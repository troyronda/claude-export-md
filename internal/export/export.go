// Package export reads a claude.ai data export: the per-category zips, or the
// folders and files they unpack to. Conversations are stream-decoded one at a
// time, so a large conversations.json is never held in memory whole.
package export

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Export is an export folder on disk. It is only ever read.
type Export struct {
	dir string
}

// Open checks that dir looks like a claude.ai export.
func Open(dir string) (*Export, error) {
	fi, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	if !fi.IsDir() {
		return nil, fmt.Errorf("%s is not a folder", dir)
	}
	e := &Export{dir: dir}
	srcs, err := e.sources("conversations")
	if err != nil {
		return nil, err
	}
	if len(srcs) == 0 {
		return nil, fmt.Errorf("no conversations found in %s (expected conversations.json or conversations-*.zip)", dir)
	}
	return e, nil
}

// Conversation is one claude.ai conversation as the export records it.
type Conversation struct {
	UUID      string    `json:"uuid"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Messages  []Message `json:"chat_messages"`

	// Not present in every export version; empty when the export doesn't link
	// conversations to projects.
	ProjectUUID string   `json:"project_uuid"`
	Project     *Project `json:"project"`
}

// Message is one turn.
type Message struct {
	UUID        string       `json:"uuid"`
	Parent      string       `json:"parent_message_uuid"`
	Sender      string       `json:"sender"` // "human" or "assistant"
	Text        string       `json:"text"`
	CreatedAt   time.Time    `json:"created_at"`
	Content     []Block      `json:"content"`
	Attachments []Attachment `json:"attachments"`
	Files       []File       `json:"files"`
}

// Block is one content block of a message.
type Block struct {
	Type  string          `json:"type"` // text, thinking, tool_use, tool_result, image, document, ...
	Text  string          `json:"text"`
	Name  string          `json:"name"` // tool name, for tool_use
	Input json.RawMessage `json:"input"`
}

// Attachment is a file whose extracted text the export includes.
type Attachment struct {
	FileName         string `json:"file_name"`
	FileType         string `json:"file_type"`
	ExtractedContent string `json:"extracted_content"`
}

// File is an uploaded file the export names but doesn't include.
type File struct {
	FileUUID string `json:"file_uuid"`
	FileName string `json:"file_name"`
}

// Project is a claude.ai project.
type Project struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
}

// ProjectID returns the conversation's project uuid, or "".
func (c *Conversation) ProjectID() string {
	if c.Project != nil && c.Project.UUID != "" {
		return c.Project.UUID
	}
	return c.ProjectUUID
}

// Branch returns the messages on the conversation's current branch, oldest
// first. Editing a message forks the conversation; the export keeps every
// fork, linked by parent_message_uuid. The current branch is the path from the
// root to the most recently created message. If the messages carry no usable
// parent links, they are returned in export order.
func (c *Conversation) Branch() []Message {
	if len(c.Messages) == 0 {
		return nil
	}
	byID := make(map[string]int, len(c.Messages))
	for i, m := range c.Messages {
		if m.UUID != "" {
			byID[m.UUID] = i
		}
	}
	leaf := len(c.Messages) - 1
	for i, m := range c.Messages {
		if m.CreatedAt.After(c.Messages[leaf].CreatedAt) {
			leaf = i
		}
	}
	var path []Message
	seen := map[int]bool{}
	for i, ok := leaf, true; ok && !seen[i]; i, ok = byID[c.Messages[i].Parent] {
		seen[i] = true
		path = append(path, c.Messages[i])
	}
	if len(path) == 1 && len(c.Messages) > 1 {
		// No parent links: keep export order.
		return c.Messages
	}
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	return path
}

// Conversations calls fn for each conversation, decoding one at a time.
func (e *Export) Conversations(fn func(*Conversation) error) error {
	srcs, err := e.sources("conversations")
	if err != nil {
		return err
	}
	for _, s := range srcs {
		err := s.open(func(r io.Reader) error {
			return eachInArray(r, func(dec *json.Decoder) error {
				var c Conversation
				if err := dec.Decode(&c); err != nil {
					return err
				}
				return fn(&c)
			})
		})
		if err != nil {
			return fmt.Errorf("%s: %w", s.name, err)
		}
	}
	return nil
}

// Projects returns project names by uuid.
func (e *Export) Projects() (map[string]string, error) {
	srcs, err := e.sources("projects")
	if err != nil {
		return nil, err
	}
	names := map[string]string{}
	for _, s := range srcs {
		err := s.open(func(r io.Reader) error {
			return eachObject(r, func(dec *json.Decoder) error {
				var p Project
				if err := dec.Decode(&p); err != nil {
					return err
				}
				if p.UUID != "" {
					names[p.UUID] = p.Name
				}
				return nil
			})
		})
		if err != nil {
			return nil, fmt.Errorf("%s: %w", s.name, err)
		}
	}
	return names, nil
}

// AccountFirstName returns the first word of the account holder's name from
// light_metadata's users.json, or "" if the export doesn't include it.
func (e *Export) AccountFirstName() (string, error) {
	srcs, err := e.sources("light_metadata")
	if err != nil {
		return "", err
	}
	var name string
	for _, s := range srcs {
		if filepath.Base(s.name) != "users.json" {
			continue
		}
		err := s.open(func(r io.Reader) error {
			return eachObject(r, func(dec *json.Decoder) error {
				var u struct {
					FullName string `json:"full_name"`
				}
				if err := dec.Decode(&u); err != nil {
					return err
				}
				if f := strings.Fields(u.FullName); name == "" && len(f) > 0 {
					name = f[0]
				}
				return nil
			})
		})
		if err != nil {
			return "", fmt.Errorf("%s: %w", s.name, err)
		}
	}
	return name, nil
}

// source is one JSON file, on disk or inside a zip.
type source struct {
	name string
	open func(func(io.Reader) error) error
}

// sources finds the JSON files for a category. Unpacked files are preferred;
// zips (<category>-NNN.zip) are read only when no unpacked files exist, so a
// folder holding both is never read twice.
func (e *Export) sources(category string) ([]source, error) {
	var files []string
	if category == "conversations" {
		files, _ = filepath.Glob(filepath.Join(e.dir, "conversations*.json"))
	}
	dirs, _ := filepath.Glob(filepath.Join(e.dir, category+"*"))
	for _, d := range dirs {
		if fi, err := os.Stat(d); err != nil || !fi.IsDir() {
			continue
		}
		err := filepath.WalkDir(d, func(p string, de os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !de.IsDir() && strings.HasSuffix(p, ".json") {
				files = append(files, p)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(files)
	var out []source
	for _, f := range files {
		out = append(out, source{name: f, open: func(fn func(io.Reader) error) error {
			r, err := os.Open(f)
			if err != nil {
				return err
			}
			defer r.Close()
			return fn(r)
		}})
	}
	if len(out) > 0 {
		return out, nil
	}
	zips, _ := filepath.Glob(filepath.Join(e.dir, category+"-*.zip"))
	sort.Strings(zips)
	for _, z := range zips {
		zr, err := zip.OpenReader(z)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", z, err)
		}
		names := make([]*zip.File, 0, len(zr.File))
		for _, zf := range zr.File {
			if !zf.FileInfo().IsDir() && strings.HasSuffix(zf.Name, ".json") {
				names = append(names, zf)
			}
		}
		sort.Slice(names, func(i, j int) bool { return names[i].Name < names[j].Name })
		for _, zf := range names {
			out = append(out, source{name: z + ":" + zf.Name, open: func(fn func(io.Reader) error) error {
				r, err := zf.Open()
				if err != nil {
					return err
				}
				defer r.Close()
				return fn(r)
			}})
		}
		// The zip.ReadCloser stays open for the life of the process; exports
		// have a handful of parts.
	}
	return out, nil
}

// eachInArray calls fn once per element of a top-level JSON array, leaving the
// decoder positioned on that element.
func eachInArray(r io.Reader, fn func(*json.Decoder) error) error {
	dec := json.NewDecoder(r)
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if d, ok := tok.(json.Delim); !ok || d != '[' {
		return errors.New("expected a JSON array")
	}
	for dec.More() {
		if err := fn(dec); err != nil {
			return err
		}
	}
	_, err = dec.Token()
	return err
}

// eachObject handles a file holding either one object or an array of objects.
func eachObject(r io.Reader, fn func(*json.Decoder) error) error {
	var raw json.RawMessage
	if err := json.NewDecoder(r).Decode(&raw); err != nil {
		return err
	}
	trimmed := strings.TrimSpace(string(raw))
	if strings.HasPrefix(trimmed, "[") {
		return eachInArray(strings.NewReader(trimmed), fn)
	}
	return fn(json.NewDecoder(strings.NewReader(trimmed)))
}
