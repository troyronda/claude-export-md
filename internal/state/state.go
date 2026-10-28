// Package state records which files the importer wrote, so re-running it
// updates only its own files and never overwrites a file someone has edited.
// The record lives in the output folder as a hidden JSON file.
package state

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// FileName is the record's name inside the output folder.
const FileName = ".claude-export-md.json"

// Entry is what the importer knows about one conversation's file.
type Entry struct {
	Path    string    `json:"path"`    // relative to the output folder
	SHA256  string    `json:"sha256"`  // of the file as last written
	Added   string    `json:"added"`   // date first written, YYYY-MM-DD
	Updated string    `json:"updated"` // date last rewritten, or ""
	Source  time.Time `json:"source_updated_at"`
}

// State maps conversation ids to entries.
type State struct {
	Version int               `json:"version"`
	Files   map[string]*Entry `json:"files"`
	dir     string
}

// Load reads the record from dir, or returns an empty one.
func Load(dir string) (*State, error) {
	s := &State{Version: 1, Files: map[string]*Entry{}, dir: dir}
	b, err := os.ReadFile(filepath.Join(dir, FileName))
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, s); err != nil {
		return nil, err
	}
	if s.Files == nil {
		s.Files = map[string]*Entry{}
	}
	s.dir = dir
	return s, nil
}

// Save writes the record atomically.
func (s *State) Save() error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return WriteFile(filepath.Join(s.dir, FileName), append(b, '\n'))
}

// Owns reports whether rel is a path the record says the importer wrote.
func (s *State) Owns(rel string) bool {
	for _, e := range s.Files {
		if e.Path == rel {
			return true
		}
	}
	return false
}

// Hash returns the hex SHA-256 of b.
func Hash(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// WriteFile writes b to path through a temporary file and a rename, so a
// reader never sees a half-written file.
func WriteFile(path string, b []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
