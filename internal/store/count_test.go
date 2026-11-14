package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCountNotes(t *testing.T) {
	root := t.TempDir()
	files := []string{
		"inbox/a.md", "inbox/_daily/2026-01-01.md", "projects/p/b.md", "areas/c.MD",
		"resources/d.md", "archives/e.md",
		"inbox/readme.txt",        // not a note
		"other/f.md",              // outside the PARA folders
		".para/trash/g.md",        // hidden
		"projects/.obsidian/h.md", // hidden
		"notes.md",                // at the root
	}
	for _, f := range files {
		p := filepath.Join(root, f)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	}
	assert.Equal(t, 6, CountNotes(root))
}

func TestCountNotesMissingRoot(t *testing.T) {
	assert.Zero(t, CountNotes(filepath.Join(t.TempDir(), "nope")))
}

func TestCountNotesIgnoresSymlinks(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "x.md"), []byte("x"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "inbox"), 0o755))
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "inbox", "link")))
	require.NoError(t, os.Symlink(filepath.Join(outside, "x.md"), filepath.Join(root, "inbox", "x.md")))
	assert.Zero(t, CountNotes(root), "a link is never followed or counted")
}
