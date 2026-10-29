// Command claude-export-md converts a claude.ai data export into one Markdown
// file per conversation.
//
//	claude-export-md [flags] <export-folder> <output-folder>
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/troyronda/claude-export-md/internal/export"
	"github.com/troyronda/claude-export-md/internal/importer"
	"github.com/troyronda/claude-export-md/internal/render"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "claude-export-md:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("claude-export-md", flag.ContinueOnError)
	dryRun := fs.Bool("dry-run", false, "print counts only; write nothing")
	since := fs.String("since", "", "only conversations updated on or after this date (YYYY-MM-DD)")
	project := fs.String("project", "", "only conversations in this project (name or uuid)")
	allowTracked := fs.Bool("allow-tracked", false, "allow writing into a folder that git tracks")
	human := fs.String("human", "", "heading for your turns (default: first name from the export, else \"Me\")")
	assistant := fs.String("assistant", "Claude", "heading for Claude's turns")
	tmplPath := fs.String("template", "", "text/template file to use instead of the built-in format")
	printTmpl := fs.Bool("print-template", false, "print the built-in template and exit")
	verbose := fs.Bool("v", false, "list the ids of skipped conversations")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "usage: claude-export-md [flags] <export-folder> <output-folder>")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *printTmpl {
		fmt.Print(render.DefaultTemplate())
		return nil
	}
	if fs.NArg() != 2 {
		fs.Usage()
		return errors.New("need an export folder and an output folder")
	}
	src, err := filepath.Abs(fs.Arg(0))
	if err != nil {
		return err
	}
	out, err := filepath.Abs(fs.Arg(1))
	if err != nil {
		return err
	}
	if out == src || strings.HasPrefix(out, src+string(filepath.Separator)) {
		return errors.New("the output folder must be outside the export folder; the export is never changed")
	}

	cfg := importer.Config{Out: out, DryRun: *dryRun, Project: *project, Today: time.Now().Format(time.DateOnly)}
	if *since != "" {
		t, err := time.ParseInLocation(time.DateOnly, *since, time.Local)
		if err != nil {
			return fmt.Errorf("--since: %w", err)
		}
		cfg.Since = t
	}
	if cfg.Export, err = export.Open(src); err != nil {
		return err
	}

	tmpl := render.DefaultTemplate()
	if *tmplPath != "" {
		b, err := os.ReadFile(*tmplPath)
		if err != nil {
			return err
		}
		tmpl = string(b)
	}
	if *human == "" {
		if *human, err = cfg.Export.AccountFirstName(); err != nil {
			return err
		}
		if *human == "" {
			*human = "Me"
		}
	}
	if cfg.Renderer, err = render.New(tmpl, render.Options{Human: *human, Assistant: *assistant}); err != nil {
		return err
	}

	if !*allowTracked {
		if err := refuseTracked(out); err != nil {
			return err
		}
	}
	if !*dryRun {
		if err := os.MkdirAll(out, 0o755); err != nil {
			return err
		}
	}

	rep, err := importer.Run(cfg)
	if rep != nil {
		for _, l := range rep.Summary(*dryRun) {
			fmt.Println(l)
		}
		if *project != "" && rep.ProjectLinks == 0 {
			fmt.Println("note: this export links no conversations to projects, so --project matches nothing")
		}
		if *verbose {
			for reason, ids := range rep.Skipped {
				for _, id := range ids {
					fmt.Printf("skipped %s: %s\n", id, reason)
				}
			}
		}
	}
	return err
}

// refuseTracked returns an error if out is inside a git work tree and is not
// ignored there, or if git tracks any file under it. Conversations are
// private; a folder git would commit is the wrong place for them.
func refuseTracked(out string) error {
	dir := out
	for {
		if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil
		}
		dir = parent
	}
	inside, err := exec.Command("git", "-C", dir, "rev-parse", "--is-inside-work-tree").Output()
	if err != nil || strings.TrimSpace(string(inside)) != "true" {
		return nil // not in a git work tree
	}
	tracked, err := exec.Command("git", "-C", dir, "ls-files", "--", out).Output()
	if err != nil {
		return fmt.Errorf("checking git: %w", err)
	}
	if len(strings.TrimSpace(string(tracked))) > 0 {
		return errors.New("git tracks files in the output folder; refusing (use --allow-tracked to override)")
	}
	probe := filepath.Join(out, "probe.md")
	err = exec.Command("git", "-C", dir, "check-ignore", "-q", "--no-index", probe).Run()
	var ee *exec.ExitError
	switch {
	case err == nil:
		return nil // ignored
	case errors.As(err, &ee) && ee.ExitCode() == 1:
		return errors.New("the output folder is inside a git repository and not ignored, so its files would be committed; add it to .gitignore or use --allow-tracked")
	default:
		return fmt.Errorf("checking git: %w", err)
	}
}
