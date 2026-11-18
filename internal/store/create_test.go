package store

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewUID(t *testing.T) {
	uuidV4 := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	seen := map[string]bool{}
	for range 200 {
		uid, err := NewUID()
		require.NoError(t, err)
		assert.Regexp(t, uuidV4, uid)
		assert.False(t, seen[uid], "uids are unique")
		seen[uid] = true
	}
}

func TestBlankNote(t *testing.T) {
	data, err := BlankNote("My Idea.md")
	require.NoError(t, err)

	n := ParseNote("My Idea.md", data)
	require.NoError(t, n.FrontmatterErr)
	require.NotNil(t, n.Frontmatter)
	assert.NotEmpty(t, n.Frontmatter.Get("uid").Value)
	assert.Equal(t, "My Idea", n.Frontmatter.Get("title").Value, "the title property equals the file name")
	assert.Empty(t, n.Frontmatter.Get("kind").Value, "the kind stays unset")
	assert.Equal(t, KindUnset, n.Kind)
	assert.Empty(t, n.Body)
	assert.Equal(t, "My Idea", n.Title)
}

func TestBlankNoteTitleIsAlwaysAString(t *testing.T) {
	for _, name := range []string{"2026.md", "true.md", "null.md", "a: b.md", "- dash.md", "#hash.md", "quote\".md", "it's.md", "{brace}.md", "日本語.md", "123", "trailing colon:.md"} {
		data, err := BlankNote(name)
		require.NoError(t, err, name)
		n := ParseNote(name, data)
		require.NoError(t, n.FrontmatterErr, name)
		v := n.Frontmatter.Get("title")
		assert.Equal(t, "!!str", v.Tag, name)
		assert.Equal(t, noteTitle(name), v.Value, name)
	}
}

func TestCleanNewName(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantDir bool
		wantErr error
	}{
		{"idea", "idea.md", false, nil},
		{"idea.md", "idea.md", false, nil},
		{"Idea.MD", "Idea.MD", false, nil},
		{"  spaced name  ", "spaced name.md", false, nil},
		{"notes.txt", "notes.txt.md", false, nil},
		{"a.b.c", "a.b.c.md", false, nil},
		{".hidden", ".hidden.md", false, nil},
		{"résumé ideas", "résumé ideas.md", false, nil},
		{"folder/", "folder", true, nil},
		{"folder /", "folder", true, nil},
		{"My Folder/", "My Folder", true, nil},
		{".dot/", ".dot", true, nil},
		{"v1.0/", "v1.0", true, nil},
		{"", "", false, ErrEmptyName},
		{"   ", "", false, ErrEmptyName},
		{"/", "", false, ErrEmptyName},
		{" / ", "", false, ErrEmptyName},
		{".", "", false, ErrBadName},
		{"..", "", false, ErrBadName},
		{"../", "", false, ErrBadName},
		{"a/b", "", false, ErrBadName},
		{"a/b/", "", false, ErrBadName},
		{"/abs", "", false, ErrBadName},
		{"../x", "", false, ErrBadName},
		{"x/..", "", false, ErrBadName},
		{"a//", "", false, ErrBadName},
		{"nul\x00byte", "", false, ErrNulByte},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, isDir, err := CleanNewName(tt.in)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.wantDir, isDir)
		})
	}
}

func TestInspect(t *testing.T) {
	root, _ := newRoot(t)
	base := filepath.Join(root, "projects", "p")
	put(t, filepath.Join(base, "a.md"), "a")
	put(t, filepath.Join(base, "sub", "b.md"), "b")
	put(t, filepath.Join(base, "sub", "pic.png"), "png")
	put(t, filepath.Join(base, ".hidden.md"), "h")
	put(t, filepath.Join(base, ".obsidian", "app.json"), "{}")
	put(t, filepath.Join(base, "notes.txt"), "t")
	require.NoError(t, os.MkdirAll(filepath.Join(base, "empty"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "projects", "bare"), 0o755))
	s := NewMarkdown(root)

	got, err := s.Inspect("projects/p")
	require.NoError(t, err)
	// a.md, sub, sub/b.md, sub/pic.png, .hidden.md, .obsidian, .obsidian/app.json, notes.txt, empty
	assert.Equal(t, DirInfo{Items: 9, Unlisted: 4}, got, ".hidden.md, .obsidian, its file and notes.txt; pic.png is listed")

	got, err = s.Inspect("projects/bare")
	require.NoError(t, err)
	assert.Equal(t, DirInfo{}, got)

	got, err = s.Inspect("projects/p/a.md")
	require.NoError(t, err)
	assert.Equal(t, DirInfo{}, got, "a file has nothing inside")

	_, err = s.Inspect("projects/gone")
	assert.ErrorIs(t, err, ErrNotFound)
	_, err = s.Inspect("../outside")
	assert.Error(t, err)
}

func TestInspectDoesNotFollowSymlinks(t *testing.T) {
	root, tmp := newRoot(t)
	put(t, filepath.Join(tmp, "outside", "big", "x.md"), "x")
	put(t, filepath.Join(root, "projects", "p", "a.md"), "a")
	require.NoError(t, os.Symlink(filepath.Join(tmp, "outside"), filepath.Join(root, "projects", "p", "link")))

	got, err := NewMarkdown(root).Inspect("projects/p")
	require.NoError(t, err)
	assert.Equal(t, 2, got.Items, "the link counts as one item and is not entered")
}

func TestDelete(t *testing.T) {
	root, tmp := newRoot(t)
	put(t, filepath.Join(root, "inbox", "n.md"), "x")
	put(t, filepath.Join(root, "projects", "p", "a.md"), "a")
	put(t, filepath.Join(root, "projects", "p", "sub", "pic.png"), "png")
	put(t, filepath.Join(tmp, "outside", "keep.md"), "keep")
	require.NoError(t, os.Symlink(filepath.Join(tmp, "outside"), filepath.Join(root, "projects", "p", "link")))
	s := NewMarkdown(root)

	require.NoError(t, s.Delete("inbox/n.md"))
	assert.False(t, exists(filepath.Join(root, "inbox", "n.md")))
	assert.False(t, exists(filepath.Join(root, ".para")), "permanent delete does not use the trash")

	require.NoError(t, s.Delete("projects/p"))
	assert.False(t, exists(filepath.Join(root, "projects", "p")), "the whole tree goes, attachments included")
	assert.Equal(t, "keep", read(t, filepath.Join(tmp, "outside", "keep.md")), "a symlink target outside is untouched")
}

func TestDeleteSymlinkLeavesTarget(t *testing.T) {
	root, _ := newRoot(t)
	put(t, filepath.Join(root, "inbox", "real.md"), "real")
	require.NoError(t, os.Symlink(filepath.Join(root, "inbox", "real.md"), filepath.Join(root, "inbox", "ln.md")))

	require.NoError(t, NewMarkdown(root).Delete("inbox/ln.md"))
	assert.Equal(t, "real", read(t, filepath.Join(root, "inbox", "real.md")))
}

func TestDeleteRejections(t *testing.T) {
	root, tmp := newRoot(t)
	put(t, filepath.Join(tmp, "outside", "keep.md"), "keep")
	require.NoError(t, os.Symlink(filepath.Join(tmp, "outside"), filepath.Join(root, "out")))
	s := NewMarkdown(root)

	tests := []struct {
		rel  string
		want error
	}{
		{"", ErrEmptyName},
		{".", ErrEmptyName},
		{"..", ErrParentSegment},
		{"../outside", ErrParentSegment},
		{"/etc/hosts", ErrAbsolutePath},
		{"out/keep.md", ErrEscapesRoot},
		{"inbox/missing.md", ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.rel, func(t *testing.T) {
			assert.ErrorIs(t, s.Delete(tt.rel), tt.want)
		})
	}
	assert.DirExists(t, root)
	assert.Equal(t, "keep", read(t, filepath.Join(tmp, "outside", "keep.md")))
}
