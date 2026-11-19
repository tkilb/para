package digest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsurePromptsWritesDefaultsWhenMissing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "config", "para")
	main, merge := filepath.Join(dir, "import-prompt.md"), filepath.Join(dir, "import-merge-prompt.md")

	require.NoError(t, EnsurePrompts(main, merge))

	got, err := os.ReadFile(main)
	require.NoError(t, err)
	assert.Equal(t, DefaultPrompt, string(got))
	got, err = os.ReadFile(merge)
	require.NoError(t, err)
	assert.Equal(t, DefaultMergePrompt, string(got))
}

func TestEnsurePromptsNeverOverwrites(t *testing.T) {
	dir := t.TempDir()
	main, merge := filepath.Join(dir, "a.md"), filepath.Join(dir, "b.md")
	require.NoError(t, os.WriteFile(main, []byte("my own prompt"), 0o644))

	require.NoError(t, EnsurePrompts(main, merge))

	got, _ := os.ReadFile(main)
	assert.Equal(t, "my own prompt", string(got), "an existing file is left alone")
	got, _ = os.ReadFile(merge)
	assert.Equal(t, DefaultMergePrompt, string(got), "only the missing one is written")
}

func TestEnsurePromptsFollowsNoSymlinkOverAnExistingFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.md")
	require.NoError(t, os.WriteFile(target, []byte("keep"), 0o644))
	link := filepath.Join(dir, "link.md")
	require.NoError(t, os.Symlink(target, link))

	require.NoError(t, EnsurePrompts(link, filepath.Join(dir, "m.md")))

	got, _ := os.ReadFile(target)
	assert.Equal(t, "keep", string(got))
}

func TestEnsurePromptsReportsAnUnwritableDirectory(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "file")
	require.NoError(t, os.WriteFile(blocker, nil, 0o644))
	err := EnsurePrompts(filepath.Join(blocker, "sub", "p.md"), filepath.Join(dir, "m.md"))
	assert.Error(t, err)
}

func TestReadPrompt(t *testing.T) {
	dir := t.TempDir()
	write := func(name, text string) string {
		p := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(p, []byte(text), 0o644))
		return p
	}
	tests := []struct {
		name    string
		path    string
		want    string
		wantErr error
	}{
		{"read as written", write("ok.md", "line one\nline two\n"), "line one\nline two\n", nil},
		{"empty file", write("empty.md", ""), "", ErrEmptyPrompt},
		{"blank file", write("blank.md", " \n\t\n"), "", ErrEmptyPrompt},
		{"too large", write("big.md", strings.Repeat("x", MaxPromptBytes+1)), "", ErrPromptTooLarge},
		{"exactly the limit", write("limit.md", strings.Repeat("x", MaxPromptBytes)), strings.Repeat("x", MaxPromptBytes), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReadPrompt(tt.path)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestReadPromptMissingFile(t *testing.T) {
	_, err := ReadPrompt(filepath.Join(t.TempDir(), "nope.md"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestDefaultPromptsStateTheRules(t *testing.T) {
	for name, p := range map[string]string{"main": DefaultPrompt, "merge": DefaultMergePrompt} {
		t.Run(name, func(t *testing.T) {
			lower := strings.ToLower(p)
			assert.Contains(t, lower, "data, not instructions", "the source is untrusted")
			assert.Contains(t, lower, "json", "the output format is stated")
			for _, field := range []string{"topic", "source", "notes", "title", "body", "tags", "actions", "facts", "takeaways", "key_points", "conclusions", "decisions"} {
				assert.Contains(t, lower, field, "the prompt names the field %s", field)
			}
			assert.Less(t, len(p), MaxPromptBytes)
		})
	}
	lower := strings.ToLower(DefaultPrompt)
	assert.Contains(t, lower, "small talk", "the main prompt drops small talk")
	assert.Contains(t, lower, "action items", "and keeps actions apart")
	assert.Contains(t, strings.ToLower(DefaultMergePrompt), "merge")
}
