// Package render turns a conversation into a Markdown file through a
// text/template. The default template writes the conversation format: YAML
// front matter, then one "### <speaker>" heading per turn. Callers can supply
// their own template.
package render

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strings"
	"text/template"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/troyronda/claude-export-md/internal/export"
)

//go:embed default.md.tmpl
var defaultTemplate string

// DefaultTemplate is the built-in template's text.
func DefaultTemplate() string { return defaultTemplate }

// Doc is what a template sees.
type Doc struct {
	Title    string
	ID       string
	URL      string
	Added    string // date the file first entered the folder, YYYY-MM-DD
	Created  string // conversation creation date, YYYY-MM-DD
	Updated  string // date of the last rewrite, or "" if never rewritten
	Project  string // project name, or ""
	Messages int    // messages on the current branch
	Turns    []Turn
}

// Turn is one message with its rendered body.
type Turn struct {
	Speaker string
	Body    string
}

// Options control rendering.
type Options struct {
	Human     string         // heading for the account holder's turns
	Assistant string         // heading for Claude's turns
	Location  *time.Location // time zone for dates
}

// Renderer renders conversations with one template.
type Renderer struct {
	tmpl *template.Template
	opt  Options
}

// New parses tmpl (use DefaultTemplate() for the built-in format).
func New(tmpl string, opt Options) (*Renderer, error) {
	if opt.Location == nil {
		opt.Location = time.Local
	}
	t, err := template.New("doc").Funcs(template.FuncMap{"q": Quote}).Parse(tmpl)
	if err != nil {
		return nil, err
	}
	return &Renderer{tmpl: t, opt: opt}, nil
}

// Doc builds the template data for a conversation.
func (r *Renderer) Doc(c *export.Conversation, project, added, updated string) Doc {
	title := strings.TrimSpace(c.Name)
	if title == "" {
		title = "Untitled"
	}
	branch := c.Branch()
	d := Doc{
		Title:    title,
		ID:       c.UUID,
		URL:      "https://claude.ai/chat/" + c.UUID,
		Added:    added,
		Created:  c.CreatedAt.In(r.opt.Location).Format(time.DateOnly),
		Updated:  updated,
		Project:  project,
		Messages: len(branch),
	}
	for _, m := range branch {
		body := Body(&m)
		if body == "" {
			continue
		}
		speaker := r.opt.Assistant
		if m.Sender == "human" {
			speaker = r.opt.Human
		}
		d.Turns = append(d.Turns, Turn{Speaker: speaker, Body: body})
	}
	return d
}

// Render executes the template.
func (r *Renderer) Render(d Doc) ([]byte, error) {
	var b strings.Builder
	if err := r.tmpl.Execute(&b, d); err != nil {
		return nil, err
	}
	out := b.String()
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return []byte(out), nil
}

// FileName returns "<created date>-<slug>.md".
func FileName(d Doc) string {
	return d.Created + "-" + Slug(d.Title) + ".md"
}

// Body renders a message's text, with artifact files and attachment text in
// fenced blocks. Thinking, tool calls and tool results are left out.
func Body(m *export.Message) string {
	var parts []string
	hasText := false
	for _, b := range m.Content {
		switch b.Type {
		case "text":
			if t := strings.TrimSpace(b.Text); t != "" {
				parts = append(parts, t)
				hasText = true
			}
		case "tool_use":
			if p := artifact(b); p != "" {
				parts = append(parts, p)
			}
		}
	}
	if !hasText && strings.TrimSpace(m.Text) != "" && len(parts) == 0 {
		parts = append(parts, strings.TrimSpace(m.Text))
	}
	for _, a := range m.Attachments {
		label := a.FileName
		if label == "" {
			label = "attachment"
		}
		if strings.TrimSpace(a.ExtractedContent) == "" {
			parts = append(parts, fmt.Sprintf("*Attachment: %s (content not in export)*", label))
			continue
		}
		parts = append(parts, fmt.Sprintf("**Attachment: %s**\n\n%s", label, Fence(a.ExtractedContent, "")))
	}
	for _, f := range m.Files {
		if f.FileName != "" {
			parts = append(parts, fmt.Sprintf("*File: %s (content not in export)*", f.FileName))
		}
	}
	return strings.Join(parts, "\n\n")
}

// artifact renders a tool call that created a file or artifact whose content
// the export includes.
func artifact(b export.Block) string {
	var in struct {
		Path     string `json:"path"`
		FileText string `json:"file_text"`
		Command  string `json:"command"`
		Title    string `json:"title"`
		Type     string `json:"type"`
		Language string `json:"language"`
		Content  string `json:"content"`
	}
	if len(b.Input) == 0 || json.Unmarshal(b.Input, &in) != nil {
		return ""
	}
	switch {
	case b.Name == "create_file" && in.FileText != "":
		return fmt.Sprintf("**File: %s**\n\n%s", in.Path, Fence(in.FileText, langFor(in.Path)))
	case b.Name == "artifacts" && (in.Command == "create" || in.Command == "rewrite") && in.Content != "":
		lang := in.Language
		if lang == "" {
			lang = langForType(in.Type)
		}
		title := in.Title
		if title == "" {
			title = "artifact"
		}
		return fmt.Sprintf("**Artifact: %s**\n\n%s", title, Fence(in.Content, lang))
	}
	return ""
}

var langs = map[string]string{
	".go": "go", ".py": "python", ".js": "javascript", ".jsx": "jsx", ".ts": "typescript",
	".tsx": "tsx", ".html": "html", ".css": "css", ".md": "markdown", ".json": "json",
	".yaml": "yaml", ".yml": "yaml", ".sh": "sh", ".sql": "sql", ".svg": "svg", ".toml": "toml",
}

func langFor(p string) string { return langs[strings.ToLower(path.Ext(p))] }

func langForType(t string) string {
	switch t {
	case "text/markdown":
		return "markdown"
	case "text/html":
		return "html"
	case "image/svg+xml":
		return "svg"
	case "application/vnd.ant.react":
		return "jsx"
	case "application/vnd.ant.mermaid":
		return "mermaid"
	}
	return ""
}

var backticks = regexp.MustCompile("`{3,}")

// Fence wraps s in a backtick fence longer than any backtick run inside it.
func Fence(s, lang string) string {
	n := 3
	for _, run := range backticks.FindAllString(s, -1) {
		if len(run) >= n {
			n = len(run) + 1
		}
	}
	f := strings.Repeat("`", n)
	return f + lang + "\n" + strings.TrimRight(s, "\n") + "\n" + f
}

// Quote returns s as a single-quoted YAML scalar, with single quotes doubled
// and line breaks folded to spaces.
func Quote(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// Slug makes a lowercase ASCII file-name slug of at most 60 characters.
func Slug(title string) string {
	var b strings.Builder
	dash := false
	for _, r := range norm.NFKD.String(strings.ToLower(title)) {
		switch {
		case r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			b.WriteRune(r)
			dash = false
		case unicode.Is(unicode.Mn, r):
			// drop combining accents
		default:
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	s := strings.Trim(b.String(), "-")
	if len(s) > 60 {
		cut := s[:60]
		if s[60] != '-' {
			// The cut fell inside a word: drop the partial word if one is close.
			if i := strings.LastIndexByte(cut, '-'); i > 40 {
				cut = cut[:i]
			}
		}
		s = strings.TrimRight(cut, "-")
	}
	if s == "" {
		return "untitled"
	}
	return s
}
