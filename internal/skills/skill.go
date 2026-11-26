package skills

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/tkilb/para/internal/store"
)

// Mode represents how a skill is executed.
type Mode string

const (
	ModeNote        Mode = "note"
	ModeInteractive Mode = "interactive"
	ModeImport      Mode = "import"
)

var (
	ErrInvalidMode = errors.New("invalid skill mode (must be note, interactive, or import)")
	ErrEmptyBody   = errors.New("skill has no body or instructions")
)

var knownKeys = map[string]bool{
	"name":        true,
	"description": true,
	"mode":        true,
}

// Skill represents a notebook-associated prompt/skill definition.
type Skill struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Mode        Mode   `yaml:"mode"`
	Body        string `yaml:"-"`
	Filename    string `yaml:"-"`
	Rel         string `yaml:"-"`
	Path        string `yaml:"-"`
}

// stemOf returns filename without a trailing .md extension (case-insensitive).
func stemOf(filename string) string {
	base := filepath.Base(filename)
	if len(base) > len(ext) && strings.EqualFold(base[len(base)-len(ext):], ext) {
		return base[:len(base)-len(ext)]
	}
	return base
}

// Parse parses a skill markdown file from its content and filename.
// Even when an error is returned (such as ErrEmptyBody or ErrInvalidMode),
// the returned Skill has its Filename and fallback Name populated so the UI
// can still list, edit, or delete the file.
func Parse(filename string, data []byte) (Skill, error) {
	base := filepath.Base(filename)
	name := stemOf(base)
	s := Skill{
		Name:     name,
		Mode:     ModeNote,
		Filename: base,
		Rel:      filepath.Join(DirName, base),
	}

	n := store.ParseNote(base, data)
	if n.FrontmatterErr != nil {
		return s, fmt.Errorf("read frontmatter: %w", n.FrontmatterErr)
	}
	if n.Frontmatter != nil {
		for _, prop := range n.Frontmatter.Properties() {
			if !knownKeys[prop.Key] {
				continue
			}
			if prop.Value.Kind != yaml.ScalarNode {
				return s, fmt.Errorf("read frontmatter: %s must be text", prop.Key)
			}
			val := strings.TrimSpace(prop.Value.Value)
			switch prop.Key {
			case "name":
				if val != "" {
					s.Name = val
				}
			case "description":
				s.Description = val
			case "mode":
				if val != "" {
					s.Mode = Mode(strings.ToLower(val))
				}
			}
		}
	}

	s.Body = strings.TrimRight(strings.TrimLeft(n.Body, "\r\n"), " \t\r\n")

	switch s.Mode {
	case ModeNote, ModeInteractive, ModeImport:
		// valid
	default:
		return s, fmt.Errorf("%w: %q", ErrInvalidMode, s.Mode)
	}

	if strings.TrimSpace(s.Body) == "" {
		return s, ErrEmptyBody
	}

	return s, nil
}

// Format renders the skill to markdown bytes with properly encoded YAML frontmatter.
func Format(s Skill) []byte {
	mode := s.Mode
	if mode == "" {
		mode = ModeNote
	}
	name := strings.TrimSpace(s.Name)
	if name == "" && s.Filename != "" {
		name = stemOf(s.Filename)
	}

	fm := store.NewFrontmatter()
	fm.SetString("name", name)
	if strings.TrimSpace(s.Description) != "" {
		fm.SetString("description", strings.TrimSpace(s.Description))
	}
	fm.SetString("mode", string(mode))
	raw, _ := fm.Marshal()

	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.Write(raw)
	buf.WriteString("---\n\n")
	if s.Body != "" {
		buf.WriteString(strings.TrimRight(s.Body, "\r\n"))
		buf.WriteString("\n")
	}
	return buf.Bytes()
}

// StarterTemplate returns a markdown skeleton for creating a new skill.
func StarterTemplate(name string, mode Mode) string {
	if mode == "" {
		mode = ModeNote
	}
	name = strings.TrimSpace(stemOf(name))
	fm := store.NewFrontmatter()
	fm.SetString("name", name)
	fm.SetString("description", "")
	fm.SetString("mode", string(mode))
	raw, _ := fm.Marshal()

	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.Write(raw)
	buf.WriteString("---\n\n")
	return buf.String()
}
