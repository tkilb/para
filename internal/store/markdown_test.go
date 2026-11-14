package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeTree creates files (and the directories above them) under root;
// names ending in "/" create an empty directory.
func writeTree(t *testing.T, root string, names ...string) {
	t.Helper()
	for _, n := range names {
		p := filepath.Join(root, n)
		if n[len(n)-1] == '/' {
			require.NoError(t, os.MkdirAll(p, 0o755))
			continue
		}
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	}
}

func names(entries []Entry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Name
	}
	return out
}

func TestListSortsDirsFirstThenByName(t *testing.T) {
	root, _ := newRoot(t)
	writeTree(t, root, "inbox/zebra.md", "inbox/Apple.md", "inbox/banana.md", "inbox/_daily/", "inbox/Archive/", "inbox/beta/")

	got, err := NewMarkdown(root).List("inbox", ListOptions{})
	require.NoError(t, err)
	assert.Equal(t, []string{"_daily", "Archive", "beta", "Apple.md", "banana.md", "zebra.md"}, names(got))
}

func TestListFiltersFiles(t *testing.T) {
	root, _ := newRoot(t)
	writeTree(t, root, "areas/note.md", "areas/NOTE2.MD", "areas/pic.png", "areas/doc.pdf", "areas/readme.txt",
		"areas/.hidden.md", "areas/.hiddendir/", "areas/.obsidian/", "areas/sub/")

	s := NewMarkdown(root)
	got, err := s.List("areas", ListOptions{})
	require.NoError(t, err)
	assert.Equal(t, []string{"sub", "note.md", "NOTE2.MD"}, names(got))

	got, err = s.List("areas", ListOptions{ShowHidden: true})
	require.NoError(t, err)
	assert.Equal(t, []string{".hiddendir", "sub", ".hidden.md", "note.md", "NOTE2.MD"}, names(got),
		".obsidian stays excluded even when hidden files are shown")
}

func TestListEntryFields(t *testing.T) {
	root, _ := newRoot(t)
	writeTree(t, root, "projects/p1/a.md", "projects/p1/b.md", "projects/p1/img.png", "projects/p1/.dot.md",
		"projects/p1/inner/", "projects/empty/")

	s := NewMarkdown(root)
	got, err := s.List("projects", ListOptions{})
	require.NoError(t, err)
	require.Equal(t, []string{"empty", "p1", "a.md"}, names(got))

	byName := map[string]Entry{}
	for _, e := range got {
		byName[e.Name] = e
	}
	assert.Equal(t, filepath.Join("projects", "p1"), byName["p1"].Rel)
	assert.True(t, byName["p1"].IsDir)
	assert.Equal(t, 3, byName["p1"].ItemCount, "inner dir plus two listed notes; png and dotfile excluded")
	assert.Equal(t, 0, byName["empty"].ItemCount)
	assert.False(t, byName["a.md"].IsDir)
	assert.Equal(t, 0, byName["a.md"].ItemCount)
	assert.False(t, byName["a.md"].ModTime.IsZero())

	got, err = s.List("projects", ListOptions{ShowHidden: true})
	require.NoError(t, err)
	for _, e := range got {
		if e.Name == "p1" {
			assert.Equal(t, 4, e.ItemCount, "item count follows the same options")
		}
	}
}

func TestListMissingDirectoryIsEmpty(t *testing.T) {
	root, _ := newRoot(t)
	s := NewMarkdown(root)

	for _, dir := range []string{"archives", "inbox/_daily", "a/b/c"} {
		got, err := s.List(dir, ListOptions{})
		require.NoError(t, err, dir)
		assert.Empty(t, got, dir)
	}
	_, err := os.Stat(filepath.Join(root, "archives"))
	assert.True(t, os.IsNotExist(err), "browsing creates nothing")
}

func TestListFileIsUnreadable(t *testing.T) {
	root, _ := newRoot(t)
	_, err := NewMarkdown(root).List("projects/a.md", ListOptions{})
	assert.ErrorIs(t, err, ErrUnreadable)
}

func TestListSymlinks(t *testing.T) {
	root, tmp := newRoot(t)
	writeTree(t, root, "areas/real/", "areas/real.md")
	writeTree(t, tmp, "outside/secret.md", "outside/pics/")
	link := func(target, name string) {
		require.NoError(t, os.Symlink(target, filepath.Join(root, "areas", name)))
	}
	link(filepath.Join(root, "areas", "real"), "dir-link")
	link(filepath.Join(root, "areas", "real.md"), "note-link.md")
	link(filepath.Join(tmp, "outside"), "escape-dir")
	link(filepath.Join(tmp, "outside", "secret.md"), "escape-note.md")
	link(filepath.Join(tmp, "missing"), "dangling.md")
	link("loop-b", "loop-a")
	link("loop-a", "loop-b")

	got, err := NewMarkdown(root).List("areas", ListOptions{})
	require.NoError(t, err)
	assert.Equal(t, []string{"dir-link", "real", "note-link.md", "real.md"}, names(got))
}

func TestListPathSafety(t *testing.T) {
	root, tmp := newRoot(t)
	require.NoError(t, os.Symlink(filepath.Join(tmp, "outside"), filepath.Join(root, "out")))
	s := NewMarkdown(root)

	for _, dir := range []string{"", ".", "..", "../outside", "/etc", "out"} {
		_, err := s.List(dir, ListOptions{})
		assert.Error(t, err, "%q", dir)
	}
}

func TestListDoesNotCountEscapingChildren(t *testing.T) {
	root, tmp := newRoot(t)
	writeTree(t, root, "projects/p/a.md")
	require.NoError(t, os.Symlink(filepath.Join(tmp, "outside"), filepath.Join(root, "projects", "p", "esc")))

	got, err := NewMarkdown(root).List("projects", ListOptions{})
	require.NoError(t, err)
	for _, e := range got {
		if e.Name == "p" {
			assert.Equal(t, 1, e.ItemCount)
		}
	}
}
