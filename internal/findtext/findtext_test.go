package findtext

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tkilb/para/internal/store"
)

func put(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
}

func TestLinesMatchWhatContentShows(t *testing.T) {
	root := t.TempDir()
	// Content trims leading and trailing newlines, so line 0 is "first".
	put(t, root, "inbox/a.md", "---\nuid: u\ntitle: a\n---\n\n\nfirst\n\n  second  \nthird\n\n")
	res := Lines(store.NewMarkdown(root), []string{"inbox/a.md"})
	for i := range res.Lines {
		res.Lines[i].Mod = time.Time{} // the time has its own test
	}

	assert.False(t, res.Truncated)
	assert.Equal(t, []Line{
		{Rel: "inbox/a.md", Kind: store.KindUnset, Index: 0, Text: "first"},
		{Rel: "inbox/a.md", Kind: store.KindUnset, Index: 2, Text: "second"},
		{Rel: "inbox/a.md", Kind: store.KindUnset, Index: 3, Text: "third"},
	}, res.Lines, "blank lines are skipped but the index counts them")
}

func TestLinesSkipFrontmatterAndControlCharacters(t *testing.T) {
	root := t.TempDir()
	put(t, root, "inbox/a.md", "---\nuid: secret-uid\n---\nclean\x1b[31m text\n")
	res := Lines(store.NewMarkdown(root), []string{"inbox/a.md"})
	require.Len(t, res.Lines, 1)
	assert.Equal(t, "clean[31m text", res.Lines[0].Text)
}

func TestLinesCutLongLinesAtACharacterBoundary(t *testing.T) {
	root := t.TempDir()
	put(t, root, "inbox/a.md", strings.Repeat("é", maxLineRunes*2)+"\n")
	res := Lines(store.NewMarkdown(root), []string{"inbox/a.md"})
	require.Len(t, res.Lines, 1)
	assert.Equal(t, maxLineRunes, len([]rune(res.Lines[0].Text)))
}

func TestLinesStopAtTheLimit(t *testing.T) {
	root := t.TempDir()
	put(t, root, "inbox/a.md", strings.Repeat("x\n", maxLines+5))
	put(t, root, "inbox/b.md", "later\n")
	res := Lines(store.NewMarkdown(root), []string{"inbox/a.md", "inbox/b.md"})
	assert.True(t, res.Truncated)
	assert.Len(t, res.Lines, maxLines)
}

func TestLinesSkipUnreadableNotes(t *testing.T) {
	root := t.TempDir()
	put(t, root, "inbox/ok.md", "fine\n")
	res := Lines(store.NewMarkdown(root), []string{"inbox/missing.md", "inbox/ok.md"})
	require.Len(t, res.Lines, 1)
	assert.Equal(t, "fine", res.Lines[0].Text)
}

func TestInDirKeepsNotesInsideTheFolder(t *testing.T) {
	rels := []string{"projects/a.md", "projects/deep/b.md", "projects2/c.md", "areas/d.md"}
	tests := []struct {
		dir  string
		want []string
	}{
		{"projects", []string{"projects/a.md", "projects/deep/b.md"}},
		{"projects/deep", []string{"projects/deep/b.md"}},
		{"areas/", []string{"areas/d.md"}},
		{"", rels},
		{".", rels},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, InDir(rels, tt.dir), tt.dir)
	}
}

func TestUnderPathMatchesPrefixIgnoringCase(t *testing.T) {
	tests := []struct {
		rel, prefix string
		want        bool
	}{
		{"projects/deep/b.md", "", true},
		{"projects/deep/b.md", "projects/", true},
		{"projects/deep/b.md", "Projects/DEEP", true},
		{"projects/deep/b.md", "/projects/de", true},
		{"projects/deep/b.md", "areas/", false},
		{"projects/deep/b.md", "deep", false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, UnderPath(tt.rel, tt.prefix), tt.rel+" "+tt.prefix)
	}
}

func TestWithoutFileDropsLinesOfNotesNamedSo(t *testing.T) {
	lines := []Line{
		{Rel: "inbox/IMPORT_x/_source.md", Text: "a"},
		{Rel: "inbox/IMPORT_x/notes.md", Text: "b"},
		{Rel: "_source.md", Text: "c"},
		{Rel: "inbox/my_source.md", Text: "d"},
		{Rel: "inbox/_source.md/inner.md", Text: "e"},
	}
	got := WithoutFile(lines, "_source.md")

	assert.Equal(t, []Line{lines[1], lines[3], lines[4]}, got)
	assert.Len(t, lines, 5, "the input is not changed")
	assert.Equal(t, lines, WithoutFile(lines, ""), "an empty name drops nothing")
}

func TestLinesCarryTheKindOfTheirNote(t *testing.T) {
	root := t.TempDir()
	put(t, root, "inbox/f.md", "---\nkind: fact\n---\n\nsolid\n")
	put(t, root, "inbox/n.md", "plain\n")
	res := Lines(store.NewMarkdown(root), []string{"inbox/f.md", "inbox/n.md"})

	require.Len(t, res.Lines, 2)
	assert.Equal(t, store.KindFact, res.Lines[0].Kind)
	assert.Equal(t, store.KindUnset, res.Lines[1].Kind)
}

func TestOfKindKeepsOnlyThatKind(t *testing.T) {
	lines := []Line{
		{Rel: "a.md", Kind: store.KindFact},
		{Rel: "b.md", Kind: store.KindUnset},
		{Rel: "c.md", Kind: store.KindFact},
	}

	assert.Equal(t, []Line{lines[0], lines[2]}, OfKind(lines, store.KindFact))
	assert.Equal(t, []Line{lines[1]}, OfKind(lines, store.KindUnset))
	assert.Empty(t, OfKind(lines, store.KindRumor))
	assert.Len(t, lines, 3, "the input is not changed")
}

func TestChildDirsListsTheFoldersUnderAPathPrefix(t *testing.T) {
	rels := []string{
		"projects/alpha/a.md",
		"projects/alpha/deep/b.md",
		"projects/Beta/c.md",
		"projects/top.md",
		"areas/home/d.md",
		"inbox/e.md",
	}
	tests := []struct {
		name, path string
		want       []string
	}{
		{"empty path lists the top folders", "", []string{"areas", "inbox", "projects"}},
		{"a partial name filters, ignoring case", "PRO", []string{"projects"}},
		{"a trailing slash lists the children", "projects/", []string{"alpha", "Beta"}},
		{"a partial child", "projects/a", []string{"alpha"}},
		{"the parent is matched ignoring case", "Projects/b", []string{"Beta"}},
		{"a file is not a folder", "projects/t", nil},
		{"deeper", "projects/alpha/", []string{"deep"}},
		{"a leading slash is ignored", "/areas/", []string{"home"}},
		{"no match", "zzz", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ChildDirs(rels, tt.path))
		})
	}
}

func TestDropLastSegmentRemovesTheEndOfASlashPath(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", ""},
		{"a", ""},
		{"a/", ""},
		{"a/b", "a/"},
		{"a/b/", "a/"},
		{"a/b/c", "a/b/"},
		{"proj/al", "proj/"},
		{"/a/b", "/a/"},
		{"a//", ""},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			assert.Equal(t, tt.want, DropLastSegment(tt.in))
		})
	}
}

func TestLinesCarryTheModificationTimeOfTheirNote(t *testing.T) {
	root := t.TempDir()
	put(t, root, "inbox/a.md", "text\n")
	when := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	require.NoError(t, os.Chtimes(filepath.Join(root, "inbox/a.md"), when, when))
	res := Lines(store.NewMarkdown(root), []string{"inbox/a.md"})

	require.Len(t, res.Lines, 1)
	assert.True(t, res.Lines[0].Mod.Equal(when), "got %v", res.Lines[0].Mod)
}

func TestChangedSinceKeepsRecentAndUnknownTimes(t *testing.T) {
	cutoff := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	lines := []Line{
		{Rel: "old.md", Mod: cutoff.Add(-time.Second)},
		{Rel: "edge.md", Mod: cutoff},
		{Rel: "new.md", Mod: cutoff.Add(time.Hour)},
		{Rel: "unknown.md"},
	}

	assert.Equal(t, []Line{lines[1], lines[2], lines[3]}, ChangedSince(lines, cutoff))
	assert.Len(t, lines, 4, "the input is not changed")
}

func TestNotesListsOneLinePerReadableNote(t *testing.T) {
	root := t.TempDir()
	put(t, root, "inbox/f.md", "---\nkind: fact\n---\n\nsolid\nmore\n")
	put(t, root, "inbox/empty.md", "")
	when := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	require.NoError(t, os.Chtimes(filepath.Join(root, "inbox/f.md"), when, when))
	got := Notes(store.NewMarkdown(root), []string{"inbox/f.md", "inbox/missing.md", "inbox/empty.md"})

	require.Len(t, got, 2, "an unreadable note is skipped and an empty one is kept")
	assert.Equal(t, "inbox/f.md", got[0].Rel)
	assert.Equal(t, store.KindFact, got[0].Kind)
	assert.True(t, got[0].Mod.Equal(when))
	assert.Empty(t, got[0].Text)
	assert.Equal(t, store.KindUnset, got[1].Kind)
}
