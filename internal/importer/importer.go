// Package importer writes one Markdown file per conversation into a folder,
// updating only files it wrote and that are unchanged since it wrote them.
package importer

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/troyronda/claude-export-md/internal/export"
	"github.com/troyronda/claude-export-md/internal/render"
	"github.com/troyronda/claude-export-md/internal/state"
)

// Config is one import run.
type Config struct {
	Export   *export.Export
	Out      string
	Renderer *render.Renderer
	Today    string    // YYYY-MM-DD, for added and updated
	Since    time.Time // zero: no filter; otherwise conversations updated on or after
	Project  string    // "" or a project name or uuid
	DryRun   bool      // count only; write nothing
}

// Skip reasons.
const (
	SkipEdited    = "edited since it was written"
	SkipMissing   = "file moved or deleted"
	SkipNameTaken = "file name taken by another file"
)

// Report holds counts and the ids of skipped conversations. It never holds
// titles or text.
type Report struct {
	Seen, Matched, New, Updated, Unchanged int
	Skipped                                map[string][]string // reason -> conversation ids
	ProjectLinks                           int                 // conversations the export links to a project
	Branched                               int                 // matching conversations with edit branches
	OffBranch                              int                 // messages on those other branches, left out
}

// Run imports every matching conversation.
func Run(cfg Config) (*Report, error) {
	rep := &Report{Skipped: map[string][]string{}}
	projects, err := cfg.Export.Projects()
	if err != nil {
		return nil, err
	}
	st, err := state.Load(cfg.Out)
	if err != nil {
		return nil, err
	}
	planned := map[string]bool{} // names claimed in this run (matters for dry runs)
	dirty := 0
	save := func() error {
		if cfg.DryRun || dirty == 0 {
			return nil
		}
		dirty = 0
		return st.Save()
	}

	err = cfg.Export.Conversations(func(c *export.Conversation) error {
		rep.Seen++
		pid := c.ProjectID()
		if pid != "" {
			rep.ProjectLinks++
		}
		if !cfg.Since.IsZero() && c.UpdatedAt.Before(cfg.Since) {
			return nil
		}
		if cfg.Project != "" && pid != cfg.Project && (pid == "" || projects[pid] != cfg.Project) {
			return nil
		}
		rep.Matched++
		if off := len(c.Messages) - len(c.Branch()); off > 0 {
			rep.Branched++
			rep.OffBranch += off
		}
		project := projects[pid]
		if project == "" && c.Project != nil {
			project = c.Project.Name
		}

		e := st.Files[c.UUID]
		if e == nil {
			d := cfg.Renderer.Doc(c, project, cfg.Today, "")
			b, err := cfg.Renderer.Render(d)
			if err != nil {
				return err
			}
			name, ok := chooseName(cfg.Out, d, b, st, planned)
			if !ok {
				rep.Skipped[SkipNameTaken] = append(rep.Skipped[SkipNameTaken], c.UUID)
				return nil
			}
			planned[name] = true
			rep.New++
			if cfg.DryRun {
				return nil
			}
			if err := state.WriteFile(filepath.Join(cfg.Out, name), b); err != nil {
				return err
			}
			st.Files[c.UUID] = &state.Entry{Path: name, SHA256: state.Hash(b), Added: cfg.Today, Source: c.UpdatedAt}
		} else {
			cur, err := os.ReadFile(filepath.Join(cfg.Out, e.Path))
			if errors.Is(err, fs.ErrNotExist) {
				rep.Skipped[SkipMissing] = append(rep.Skipped[SkipMissing], c.UUID)
				return nil
			}
			if err != nil {
				return err
			}
			if state.Hash(cur) != e.SHA256 {
				rep.Skipped[SkipEdited] = append(rep.Skipped[SkipEdited], c.UUID)
				return nil
			}
			b, err := cfg.Renderer.Render(cfg.Renderer.Doc(c, project, e.Added, e.Updated))
			if err != nil {
				return err
			}
			if bytes.Equal(b, cur) {
				rep.Unchanged++
				return nil
			}
			b, err = cfg.Renderer.Render(cfg.Renderer.Doc(c, project, e.Added, cfg.Today))
			if err != nil {
				return err
			}
			rep.Updated++
			if cfg.DryRun {
				return nil
			}
			if err := state.WriteFile(filepath.Join(cfg.Out, e.Path), b); err != nil {
				return err
			}
			e.SHA256, e.Updated, e.Source = state.Hash(b), cfg.Today, c.UpdatedAt
		}
		dirty++
		if dirty >= 100 {
			return save()
		}
		return nil
	})
	if serr := save(); err == nil {
		err = serr
	}
	for _, ids := range rep.Skipped {
		sort.Strings(ids)
	}
	return rep, err
}

// chooseName picks "<date>-<slug>.md", or "<date>-<slug>-<id prefix>.md" if
// that is taken. A file that already holds exactly the rendered bytes is
// adopted: it was written by a run that stopped before saving its record.
func chooseName(out string, d render.Doc, b []byte, st *state.State, planned map[string]bool) (string, bool) {
	base := render.FileName(d)
	short := d.ID
	if len(short) > 8 {
		short = short[:8]
	}
	alt := base[:len(base)-len(".md")] + "-" + short + ".md"
	for _, name := range []string{base, alt} {
		if planned[name] || st.Owns(name) {
			continue
		}
		cur, err := os.ReadFile(filepath.Join(out, name))
		if errors.Is(err, fs.ErrNotExist) || (err == nil && bytes.Equal(cur, b)) {
			return name, true
		}
	}
	return "", false
}

// Summary returns the report as lines of counts.
func (r *Report) Summary(dryRun bool) []string {
	verb := "written"
	if dryRun {
		verb = "would be written"
	}
	lines := []string{
		fmt.Sprintf("conversations in export: %d", r.Seen),
		fmt.Sprintf("matching filters:        %d", r.Matched),
		fmt.Sprintf("new files %s: %d", verb, r.New),
		fmt.Sprintf("updated files %s: %d", verb, r.Updated),
		fmt.Sprintf("unchanged:               %d", r.Unchanged),
		fmt.Sprintf("edited conversations:    %d (current branch written; %d messages on other branches left out)", r.Branched, r.OffBranch),
	}
	reasons := make([]string, 0, len(r.Skipped))
	for k := range r.Skipped {
		reasons = append(reasons, k)
	}
	sort.Strings(reasons)
	for _, k := range reasons {
		lines = append(lines, fmt.Sprintf("skipped (%s): %d", k, len(r.Skipped[k])))
	}
	return lines
}
