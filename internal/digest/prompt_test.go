package digest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolvePromptUsesTheBuiltInWithoutAPath(t *testing.T) {
	got, err := ResolvePrompt("", DefaultPrompt)
	require.NoError(t, err)
	assert.Equal(t, DefaultPrompt, got, "no path means the prompt built into para, so it cannot drift")
}

func TestResolvePromptReadsAnExplicitFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mine.md")
	require.NoError(t, os.WriteFile(path, []byte("my own prompt"), 0o644))
	got, err := ResolvePrompt(path, DefaultPrompt)
	require.NoError(t, err)
	assert.Equal(t, "my own prompt", got)
}

func TestResolvePromptReportsABadFile(t *testing.T) {
	_, err := ResolvePrompt(filepath.Join(t.TempDir(), "nope.md"), DefaultPrompt)
	assert.Error(t, err, "a configured file that is missing is an error, never a silent fall back")
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
			for _, field := range []string{"topic", "source", "notes", "title", "body", "tags", "actions", "facts", "conclusions"} {
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
