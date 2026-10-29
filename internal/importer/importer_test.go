package importer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/troyronda/claude-export-md/internal/export"
	"github.com/troyronda/claude-export-md/internal/render"
	"github.com/troyronda/claude-export-md/internal/state"
)

const shedID = "11111111-aaaa-4bbb-8ccc-000000000001"

// fixture copies the made-up export so a test can change it.
func fixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	src := "../export/testdata/export"
	err := filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dir, rel), 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dir, rel), b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func run(t *testing.T, src, out, today string, dry bool) *Report {
	t.Helper()
	e, err := export.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	r, err := render.New(render.DefaultTemplate(), render.Options{Human: "Robin", Assistant: "Claude", Location: time.UTC})
	if err != nil {
		t.Fatal(err)
	}
	rep, err := Run(Config{Export: e, Out: out, Renderer: r, Today: today, DryRun: dry})
	if err != nil {
		t.Fatal(err)
	}
	return rep
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// grow appends a message to the shed conversation.
func grow(t *testing.T, src string) {
	t.Helper()
	p := filepath.Join(src, "conversations.json")
	s := read(t, p)
	extra := `{"uuid": "m6", "parent_message_uuid": "m5", "sender": "human", "text": "", "created_at": "2026-03-03T10:00:00Z", "updated_at": "2026-03-03T10:00:00Z", "attachments": [], "files": [], "content": [{"type": "text", "text": "And a window?"}]}`
	s = strings.Replace(s, `{"type": "tool_result", "name": "create_file", "content": [{"type": "text", "text": "ok"}]}]}`,
		`{"type": "tool_result", "name": "create_file", "content": [{"type": "text", "text": "ok"}]}]},`+"\n      "+extra, 1)
	s = strings.Replace(s, `"updated_at": "2026-03-02T16:00:00.000000Z"`, `"updated_at": "2026-03-03T10:00:00Z"`, 1)
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFirstRunAndRerun(t *testing.T) {
	src, out := fixture(t), t.TempDir()
	rep := run(t, src, out, "2026-10-28", false)
	if rep.Seen != 2 || rep.New != 2 {
		t.Fatalf("first run: %+v", rep)
	}
	if rep.Branched != 1 || rep.OffBranch != 1 {
		t.Errorf("branches: %d conversations, %d messages left out; want 1 and 1", rep.Branched, rep.OffBranch)
	}
	shed := filepath.Join(out, "2026-03-02-planning-a-garden-shed.md")
	if !strings.Contains(read(t, shed), "id: '"+shedID+"'") {
		t.Error("shed file missing its id")
	}
	if _, err := os.Stat(filepath.Join(out, state.FileName)); err != nil {
		t.Error("no state file")
	}

	rep = run(t, src, out, "2026-10-29", false)
	if rep.New != 0 || rep.Updated != 0 || rep.Unchanged != 2 {
		t.Errorf("rerun: %+v", rep)
	}
	if strings.Contains(read(t, shed), "updated:") {
		t.Error("an unchanged file got an updated date")
	}
}

func TestGrownConversationIsRewritten(t *testing.T) {
	src, out := fixture(t), t.TempDir()
	run(t, src, out, "2026-10-28", false)
	grow(t, src)
	rep := run(t, src, out, "2026-11-02", false)
	if rep.Updated != 1 || rep.Unchanged != 1 {
		t.Fatalf("after growth: %+v", rep)
	}
	got := read(t, filepath.Join(out, "2026-03-02-planning-a-garden-shed.md"))
	for _, want := range []string{"added: 2026-10-28", "updated: 2026-11-02", "messages: 5", "And a window?"} {
		if !strings.Contains(got, want) {
			t.Errorf("rewritten file lacks %q", want)
		}
	}
}

func TestEditedFileIsNeverOverwritten(t *testing.T) {
	src, out := fixture(t), t.TempDir()
	run(t, src, out, "2026-10-28", false)
	shed := filepath.Join(out, "2026-03-02-planning-a-garden-shed.md")
	edited := read(t, shed) + "\nMy own note.\n"
	os.WriteFile(shed, []byte(edited), 0o644)
	grow(t, src)
	rep := run(t, src, out, "2026-11-02", false)
	if ids := rep.Skipped[SkipEdited]; len(ids) != 1 || ids[0] != shedID {
		t.Fatalf("skipped = %v", rep.Skipped)
	}
	if read(t, shed) != edited {
		t.Error("edited file was changed")
	}
}

func TestDeletedFileIsNotRecreated(t *testing.T) {
	src, out := fixture(t), t.TempDir()
	run(t, src, out, "2026-10-28", false)
	os.Remove(filepath.Join(out, "2026-03-02-planning-a-garden-shed.md"))
	rep := run(t, src, out, "2026-10-29", false)
	if len(rep.Skipped[SkipMissing]) != 1 {
		t.Errorf("skipped = %v", rep.Skipped)
	}
	if _, err := os.Stat(filepath.Join(out, "2026-03-02-planning-a-garden-shed.md")); err == nil {
		t.Error("deleted file was recreated")
	}
}

func TestNameTakenBySomeoneElse(t *testing.T) {
	src, out := fixture(t), t.TempDir()
	mine := filepath.Join(out, "2026-03-02-planning-a-garden-shed.md")
	os.WriteFile(mine, []byte("someone else's file\n"), 0o644)
	rep := run(t, src, out, "2026-10-28", false)
	if rep.New != 2 {
		t.Fatalf("%+v", rep)
	}
	if read(t, mine) != "someone else's file\n" {
		t.Error("existing file overwritten")
	}
	if _, err := os.Stat(filepath.Join(out, "2026-03-02-planning-a-garden-shed-11111111.md")); err != nil {
		t.Error("no id-suffixed file")
	}
}

func TestDryRunWritesNothing(t *testing.T) {
	src, out := fixture(t), t.TempDir()
	rep := run(t, src, out, "2026-10-28", true)
	if rep.New != 2 {
		t.Errorf("%+v", rep)
	}
	if ents, _ := os.ReadDir(out); len(ents) != 0 {
		t.Errorf("dry run wrote %d files", len(ents))
	}
	for _, l := range rep.Summary(true) {
		if strings.Contains(strings.ToLower(l), "shed") {
			t.Errorf("summary names a conversation: %q", l)
		}
	}
}

func TestInterruptedRunIsAdopted(t *testing.T) {
	src, out := fixture(t), t.TempDir()
	run(t, src, out, "2026-10-28", false)
	os.Remove(filepath.Join(out, state.FileName)) // as if the run stopped before saving
	rep := run(t, src, out, "2026-10-28", false)
	if rep.New != 2 || len(rep.Skipped) != 0 {
		t.Errorf("%+v", rep)
	}
	if _, err := os.Stat(filepath.Join(out, "2026-03-02-planning-a-garden-shed-11111111.md")); err == nil {
		t.Error("wrote a duplicate instead of adopting its own file")
	}
}

func TestFilters(t *testing.T) {
	src, out := fixture(t), t.TempDir()
	e, _ := export.Open(src)
	r, _ := render.New(render.DefaultTemplate(), render.Options{Human: "Robin", Assistant: "Claude", Location: time.UTC})
	for _, tc := range []struct {
		cfg  Config
		want int
	}{
		{Config{Since: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)}, 1},
		{Config{Project: "proj-1"}, 1},
		{Config{Project: "Garden & 'Yard'"}, 1},
		{Config{Project: "nope"}, 0},
	} {
		tc.cfg.Export, tc.cfg.Out, tc.cfg.Renderer, tc.cfg.Today, tc.cfg.DryRun = e, out, r, "2026-10-28", true
		rep, err := Run(tc.cfg)
		if err != nil {
			t.Fatal(err)
		}
		if rep.Matched != tc.want {
			t.Errorf("%+v matched %d, want %d", tc.cfg.Project, rep.Matched, tc.want)
		}
	}
}
