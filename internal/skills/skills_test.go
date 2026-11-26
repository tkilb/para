package skills

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name        string
		filename    string
		content     string
		expectSkill Skill
		expectErr   bool
	}{
		{
			name:     "full frontmatter",
			filename: "summarize.md",
			content: `---
name: Custom Summarizer
description: A helpful summarizer
mode: note
---

Summarize the following note clearly.`,
			expectSkill: Skill{
				Name:        "Custom Summarizer",
				Description: "A helpful summarizer",
				Mode:        ModeNote,
				Body:        "Summarize the following note clearly.",
				Filename:    "summarize.md",
				Rel:         filepath.Join(DirName, "summarize.md"),
			},
		},
		{
			name:     "frontmatter with triple dash inside value",
			filename: "dashes.md",
			content: `---
name: Dash --- Skill
description: Has --- inside description
mode: note
---
Prompt body with --- inside.`,
			expectSkill: Skill{
				Name:        "Dash --- Skill",
				Description: "Has --- inside description",
				Mode:        ModeNote,
				Body:        "Prompt body with --- inside.",
				Filename:    "dashes.md",
				Rel:         filepath.Join(DirName, "dashes.md"),
			},
		},
		{
			name:     "interactive mode",
			filename: "chat.md",
			content: `---
mode: interactive
description: Chat with assistant
---
Interactive session instructions.`,
			expectSkill: Skill{
				Name:        "chat",
				Description: "Chat with assistant",
				Mode:        ModeInteractive,
				Body:        "Interactive session instructions.",
				Filename:    "chat.md",
				Rel:         filepath.Join(DirName, "chat.md"),
			},
		},
		{
			name:     "import mode",
			filename: "transcribe.md",
			content: `---
mode: import
---
Import instructions.`,
			expectSkill: Skill{
				Name:     "transcribe",
				Mode:     ModeImport,
				Body:     "Import instructions.",
				Filename: "transcribe.md",
				Rel:      filepath.Join(DirName, "transcribe.md"),
			},
		},
		{
			name:     "no frontmatter defaults to note",
			filename: "review.md",
			content:  "Review this note for grammar and style.",
			expectSkill: Skill{
				Name:     "review",
				Mode:     ModeNote,
				Body:     "Review this note for grammar and style.",
				Filename: "review.md",
				Rel:      filepath.Join(DirName, "review.md"),
			},
		},
		{
			name:      "empty body returns ErrEmptyBody",
			filename:  "empty.md",
			content:   "---\nname: Empty\n---\n",
			expectErr: true,
		},
		{
			name:     "invalid mode fails",
			filename: "bad.md",
			content: `---
mode: unknown
---
Body text.`,
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := Parse(tt.filename, []byte(tt.content))
			if tt.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectSkill, s)
			}
		})
	}
}

func TestCleanNameAndPathSafety(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantFile  string
		wantStem  string
		expectErr bool
	}{
		{name: "plain name", input: "summarize", wantFile: "summarize.md", wantStem: "summarize"},
		{name: "with md ext", input: "summarize.md", wantFile: "summarize.md", wantStem: "summarize"},
		{name: "trimmed whitespace", input: "  my skill  ", wantFile: "my skill.md", wantStem: "my skill"},
		{name: "empty", input: "   ", expectErr: true},
		{name: "dot", input: ".", expectErr: true},
		{name: "dotdot", input: "..", expectErr: true},
		{name: "parent traversal", input: "../outside", expectErr: true},
		{name: "subdirectory slash", input: "sub/skill", expectErr: true},
		{name: "trailing slash directory", input: "folder/", expectErr: true},
		{name: "nul byte", input: "bad\x00name", expectErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, stem, err := CleanName(tt.input)
			if tt.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantFile, file)
				assert.Equal(t, tt.wantStem, stem)
			}
		})
	}

	tempDir := t.TempDir()
	_, err := Load(tempDir, "../evil.md")
	require.Error(t, err)
	_, err = Save(tempDir, Skill{Name: "../evil", Body: "x"})
	require.Error(t, err)
	err = Delete(tempDir, "../evil.md")
	require.Error(t, err)
}

func TestStoreOperations(t *testing.T) {
	tempDir := t.TempDir()

	// Initially list should be empty
	list, err := List(tempDir)
	require.NoError(t, err)
	assert.Empty(t, list)

	// Save skill 1
	s1 := Skill{
		Name:        "b-skill",
		Description: "Second skill",
		Mode:        ModeNote,
		Body:        "Prompt B",
	}
	p1, err := Save(tempDir, s1)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tempDir, ".skills", "b-skill.md"), p1)

	// Save skill 2
	s2 := Skill{
		Name:        "a-skill",
		Description: "First skill",
		Mode:        ModeInteractive,
		Body:        "Prompt A",
	}
	_, err = Save(tempDir, s2)
	require.NoError(t, err)

	// List should return sorted by name (a-skill, b-skill)
	list, err = List(tempDir)
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, "a-skill", list[0].Name)
	assert.Equal(t, ModeInteractive, list[0].Mode)
	assert.Equal(t, "b-skill", list[1].Name)
	assert.Equal(t, ModeNote, list[1].Mode)

	// Load loaded
	loaded, err := Load(tempDir, "b-skill")
	require.NoError(t, err)
	assert.Equal(t, "b-skill", loaded.Name)
	assert.Equal(t, "Prompt B", loaded.Body)

	// An empty-body skill file (e.g. starter template before body is added) is still listed
	starterPath := filepath.Join(tempDir, ".skills", "c-starter.md")
	require.NoError(t, os.WriteFile(starterPath, []byte(StarterTemplate("c-starter", ModeNote)), 0o644))
	list, err = List(tempDir)
	require.NoError(t, err)
	require.Len(t, list, 3)
	assert.Equal(t, "c-starter", list[2].Name)

	// Delete
	err = Delete(tempDir, "b-skill.md")
	require.NoError(t, err)

	list, err = List(tempDir)
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, "a-skill", list[0].Name)

	// Deleting again should return ErrNotFound
	err = Delete(tempDir, "b-skill.md")
	require.ErrorIs(t, err, ErrNotFound)
}

func TestFormatAndStarterTemplateSpecialYAMLChars(t *testing.T) {
	s := Skill{
		Name:        "my-skill: part #1",
		Description: "desc: with 'quotes' and #hash",
		Mode:        ModeNote,
		Body:        "My instructions",
	}
	out := Format(s)
	parsed, err := Parse("my-skill.md", out)
	require.NoError(t, err)
	assert.Equal(t, s.Name, parsed.Name)
	assert.Equal(t, s.Description, parsed.Description)
	assert.Equal(t, s.Mode, parsed.Mode)
	assert.Equal(t, s.Body, parsed.Body)

	tmpl := StarterTemplate("new: one #2", ModeInteractive)
	assert.Contains(t, tmpl, "mode: interactive")
	parsedTmpl, err := Parse("new.md", []byte(tmpl+"Some body"))
	require.NoError(t, err)
	assert.Equal(t, "new: one #2", parsedTmpl.Name)
	assert.Equal(t, ModeInteractive, parsedTmpl.Mode)
}

func TestStoreEdgeCases(t *testing.T) {
	tempDir := t.TempDir()

	// Load non-existent
	_, err := Load(tempDir, "missing")
	require.ErrorIs(t, err, ErrNotFound)

	// Skill file over size limit
	skillsDir := filepath.Join(tempDir, ".skills")
	require.NoError(t, os.MkdirAll(skillsDir, 0o755))
	largeFile := filepath.Join(skillsDir, "large.md")
	f, err := os.Create(largeFile)
	require.NoError(t, err)
	require.NoError(t, f.Truncate(2<<20)) // 2 MB
	require.NoError(t, f.Close())

	_, err = Load(tempDir, "large.md")
	require.ErrorIs(t, err, ErrTooLarge)
}
