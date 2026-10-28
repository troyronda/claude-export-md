package export

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func collect(t *testing.T, e *Export) []*Conversation {
	t.Helper()
	var cs []*Conversation
	if err := e.Conversations(func(c *Conversation) error { cs = append(cs, c); return nil }); err != nil {
		t.Fatal(err)
	}
	return cs
}

func TestFolder(t *testing.T) {
	e, err := Open("testdata/export")
	if err != nil {
		t.Fatal(err)
	}
	cs := collect(t, e)
	if len(cs) != 2 {
		t.Fatalf("got %d conversations, want 2", len(cs))
	}
	if cs[1].ProjectID() != "proj-1" {
		t.Errorf("project id = %q", cs[1].ProjectID())
	}
	p, err := e.Projects()
	if err != nil {
		t.Fatal(err)
	}
	if p["proj-1"] != "Garden & 'Yard'" {
		t.Errorf("projects = %v", p)
	}
	name, err := e.AccountFirstName()
	if err != nil || name != "Robin" {
		t.Errorf("first name = %q, %v", name, err)
	}
}

func TestZips(t *testing.T) {
	dir := t.TempDir()
	writeZip(t, filepath.Join(dir, "conversations-000.zip"), "conversations.json", "testdata/export/conversations.json")
	writeZip(t, filepath.Join(dir, "conversations-001.zip"), "conversations.json", "testdata/export/conversations.json")
	writeZip(t, filepath.Join(dir, "projects-000.zip"), "projects/proj-1.json", "testdata/export/projects/proj-1.json")
	e, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(collect(t, e)); n != 4 {
		t.Errorf("got %d conversations from two parts, want 4", n)
	}
	if p, _ := e.Projects(); p["proj-1"] == "" {
		t.Error("project not read from zip")
	}
	if name, _ := e.AccountFirstName(); name != "" {
		t.Errorf("first name = %q, want empty without light_metadata", name)
	}
}

func TestOpenEmpty(t *testing.T) {
	if _, err := Open(t.TempDir()); err == nil || !strings.Contains(err.Error(), "no conversations") {
		t.Errorf("err = %v", err)
	}
}

func TestBranch(t *testing.T) {
	e, _ := Open("testdata/export")
	c := collect(t, e)[0]
	var ids []string
	for _, m := range c.Branch() {
		ids = append(ids, m.UUID)
	}
	if got := strings.Join(ids, ","); got != "m1,m2,m4,m5" {
		t.Errorf("branch = %s, want m1,m2,m4,m5 (the edited branch, not m3)", got)
	}
}

func TestBranchWithoutParents(t *testing.T) {
	c := &Conversation{Messages: []Message{{UUID: "a"}, {UUID: "b"}, {UUID: "c"}}}
	if n := len(c.Branch()); n != 3 {
		t.Errorf("got %d messages, want all 3 in export order", n)
	}
}

func writeZip(t *testing.T, path, name, src string) {
	t.Helper()
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	w.Write(b)
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()
}
