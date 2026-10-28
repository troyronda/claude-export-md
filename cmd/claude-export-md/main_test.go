package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestRefuseTracked(t *testing.T) {
	repo := t.TempDir()
	git(t, repo, "init", "-q")
	os.WriteFile(filepath.Join(repo, ".gitignore"), []byte("private/\n"), 0o644)

	if err := refuseTracked(filepath.Join(repo, "chats")); err == nil || !strings.Contains(err.Error(), "not ignored") {
		t.Errorf("unignored folder: err = %v", err)
	}
	if err := refuseTracked(filepath.Join(repo, "private", "chats")); err != nil {
		t.Errorf("ignored folder: %v", err)
	}
	if err := refuseTracked(t.TempDir()); err != nil {
		t.Errorf("outside git: %v", err)
	}

	// A folder that is ignored but already has tracked files is refused.
	os.MkdirAll(filepath.Join(repo, "private"), 0o755)
	os.WriteFile(filepath.Join(repo, "private", "a.md"), []byte("x"), 0o644)
	git(t, repo, "add", "-f", "private/a.md")
	if err := refuseTracked(filepath.Join(repo, "private")); err == nil || !strings.Contains(err.Error(), "tracks files") {
		t.Errorf("tracked files: err = %v", err)
	}
}

func TestOutputInsideExportIsRefused(t *testing.T) {
	src := "../../internal/export/testdata/export"
	err := run([]string{"--dry-run", src, filepath.Join(src, "out")})
	if err == nil || !strings.Contains(err.Error(), "outside the export") {
		t.Errorf("err = %v", err)
	}
}
