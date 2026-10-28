package render

import (
	"flag"
	"os"
	"testing"
	"time"

	"github.com/troyronda/claude-export-md/internal/export"
)

var update = flag.Bool("update", false, "rewrite golden files")

func renderer(t *testing.T) *Renderer {
	t.Helper()
	r, err := New(DefaultTemplate(), Options{Human: "Robin", Assistant: "Claude", Location: time.UTC})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestGolden(t *testing.T) {
	e, err := export.Open("../export/testdata/export")
	if err != nil {
		t.Fatal(err)
	}
	r := renderer(t)
	projects, _ := e.Projects()
	i := 0
	err = e.Conversations(func(c *export.Conversation) error {
		i++
		updated := ""
		if i == 2 {
			updated = "2026-05-01"
		}
		d := r.Doc(c, projects[c.ProjectID()], "2026-10-28", updated)
		got, err := r.Render(d)
		if err != nil {
			return err
		}
		golden := "testdata/" + FileName(d)
		if *update {
			return os.WriteFile(golden, got, 0o644)
		}
		want, err := os.ReadFile(golden)
		if err != nil {
			return err
		}
		if string(got) != string(want) {
			t.Errorf("%s differs:\n%s", golden, got)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestQuote(t *testing.T) {
	for in, want := range map[string]string{
		"plain":          "'plain'",
		"it's":           "'it''s'",
		"two\nlines":     "'two lines'",
		"  spaced  out ": "'spaced out'",
		"":               "''",
	} {
		if got := Quote(in); got != want {
			t.Errorf("Quote(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestSlug(t *testing.T) {
	for in, want := range map[string]string{
		"Planning a garden shed": "planning-a-garden-shed",
		"Café — déjà vu!":        "cafe-deja-vu",
		"":                       "untitled",
		"日本語":                    "untitled",
		"C++ & Go: notes/ideas?": "c-go-notes-ideas",
		"a very long title that goes on and on and on past the sixty character limit": "a-very-long-title-that-goes-on-and-on-and-on-past-the-sixty",
	} {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
		if len(Slug(in)) > 60 {
			t.Errorf("Slug(%q) longer than 60", in)
		}
	}
}

func TestFence(t *testing.T) {
	if got := Fence("a\n````\nb\n", "md"); got != "`````md\na\n````\nb\n`````" {
		t.Errorf("Fence = %q", got)
	}
	if got := Fence("x", ""); got != "```\nx\n```" {
		t.Errorf("Fence = %q", got)
	}
}

func TestCustomTemplate(t *testing.T) {
	r, err := New("{{.Title}}|{{len .Turns}}", Options{Human: "H", Assistant: "A"})
	if err != nil {
		t.Fatal(err)
	}
	c := &export.Conversation{UUID: "x", Messages: []export.Message{{UUID: "a", Sender: "human", Content: []export.Block{{Type: "text", Text: "hi"}}}}}
	got, _ := r.Render(r.Doc(c, "", "2026-01-01", ""))
	if string(got) != "Untitled|1\n" {
		t.Errorf("got %q", got)
	}
}
