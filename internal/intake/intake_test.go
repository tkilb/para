package intake

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tkilb/para/internal/clip"
	"github.com/tkilb/para/internal/store"
)

var now = time.Date(2026, 10, 30, 14, 30, 5, 0, time.Local)

func TestName(t *testing.T) {
	assert.Equal(t, "Call Mum.md", Name("Call Mum"))
	assert.Equal(t, "", Name("///"), "nothing usable, no name")
}

func newStore(t *testing.T) (string, store.Store) {
	t.Helper()
	root := t.TempDir()
	return root, store.NewMarkdown(root)
}

func TestCaptureWritesAnInboxNoteAndIsUndoable(t *testing.T) {
	root, s := newStore(t)

	res, err := Capture(s, now, store.KindFact, SourceAdhoc, "Water boils", "At 100C at sea level", nil)
	require.NoError(t, err)
	assert.Equal(t, "inbox/Water boils.md", res.Rel, "named from the title, no timestamp")

	data, err := os.ReadFile(filepath.Join(root, res.Rel))
	require.NoError(t, err)
	n := store.ParseNote("x.md", data)
	assert.Equal(t, store.KindFact, n.Kind)
	assert.Equal(t, "adhoc", n.Frontmatter.Get("source").Value)
	assert.Equal(t, "2026-10-30", n.Frontmatter.Get("date").Value, "the date lives in the properties")
	assert.Equal(t, "Water boils", n.Frontmatter.Get("title").Value)
	assert.Equal(t, "At 100C at sea level\n", n.Body)

	require.NoError(t, res.Action.Undo())
	assert.NoFileExists(t, filepath.Join(root, res.Rel))
	require.NoError(t, res.Action.Redo())
	assert.FileExists(t, filepath.Join(root, res.Rel))
}

func TestCaptureInSavesInTheGivenFolder(t *testing.T) {
	tests := []struct {
		name, dir, want string
	}{
		{"empty folder means the inbox", "", "inbox/Idea.md"},
		{"a project folder", "projects/alpha", "projects/alpha/Idea.md"},
		{"a folder that does not exist yet", "areas/new/deep", "areas/new/deep/Idea.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, s := newStore(t)
			res, err := CaptureIn(s, now, tt.dir, store.KindIdea, SourceAdhoc, "Idea", "body", nil)
			require.NoError(t, err)
			assert.Equal(t, tt.want, res.Rel)
			assert.FileExists(t, filepath.Join(root, res.Rel))
			require.NoError(t, res.Action.Undo())
			assert.NoFileExists(t, filepath.Join(root, res.Rel))
		})
	}
}

func TestCaptureInRefusesAFolderOutsideTheNotebook(t *testing.T) {
	_, s := newStore(t)
	_, err := CaptureIn(s, now, "../outside", store.KindIdea, SourceAdhoc, "Idea", "body", nil)
	assert.Error(t, err)
}

func TestCaptureWithoutABodyMakesAnEmptyBody(t *testing.T) {
	root, s := newStore(t)
	res, err := Capture(s, now, store.KindIdea, SourceAdhoc, "Just a title", "  ", nil)
	require.NoError(t, err)
	data, _ := os.ReadFile(filepath.Join(root, res.Rel))
	assert.Empty(t, store.ParseNote("x.md", data).Body)
}

func TestCaptureNumbersACollidingName(t *testing.T) {
	root, s := newStore(t)
	a, err := Capture(s, now, store.KindIdea, SourceAdhoc, "Same", "one", nil)
	require.NoError(t, err)
	b, err := Capture(s, now, store.KindIdea, SourceAdhoc, "Same", "two", nil)
	require.NoError(t, err)
	c, err := Capture(s, now, store.KindIdea, SourceAdhoc, "Same", "three", nil)
	require.NoError(t, err)

	assert.Equal(t, "inbox/Same.md", a.Rel)
	assert.Equal(t, "inbox/Same 2.md", b.Rel)
	assert.Equal(t, "inbox/Same 3.md", c.Rel)
	data, _ := os.ReadFile(filepath.Join(root, c.Rel))
	assert.Equal(t, "Same 3", store.ParseNote("x.md", data).Frontmatter.Get("title").Value, "the title follows the numbered name")
}

func TestCaptureRejectsAMissingTitle(t *testing.T) {
	_, s := newStore(t)
	for _, title := range []string{"", "   ", "\t", "///", "#"} {
		_, err := Capture(s, now, store.KindFact, SourceAdhoc, title, "has a body", nil)
		assert.ErrorIs(t, err, ErrEmpty, "%q", title)
	}
}

func TestCaptureKeepsTheBodyVerbatim(t *testing.T) {
	root, s := newStore(t)
	body := "---\nnot: frontmatter [[link]] # heading"
	res, err := Capture(s, now, store.KindRumor, SourceAdhoc, "T", body, nil)
	require.NoError(t, err)
	data, _ := os.ReadFile(filepath.Join(root, res.Rel))
	n := store.ParseNote("x.md", data)
	require.NoError(t, n.FrontmatterErr)
	assert.Equal(t, body+"\n", n.Body)
}

func TestParseTags(t *testing.T) {
	tests := []struct {
		in      string
		want    []string
		wantErr bool
	}{
		{"work", []string{"work"}, false},
		{"work, ideas  new", []string{"work", "ideas", "new"}, false},
		{"#work #ideas", []string{"work", "ideas"}, false},
		{"a,,b, ,c", []string{"a", "b", "c"}, false},
		{"Work work WORK", []string{"Work"}, false},
		{"ideas/new_stuff-2", []string{"ideas/new_stuff-2"}, false},
		{"héllo", []string{"héllo"}, false},
		{"", nil, false},
		{" , ", nil, false},
		{"#", nil, false},
		{"has+plus", nil, true},
		{"a\"b", nil, true},
		{"2026", nil, true},
		{"w[0]", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseTags(tt.in)
			if tt.wantErr {
				assert.ErrorIs(t, err, ErrBadTag)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCaptureWritesTagsAndOmitsAnUnsetKind(t *testing.T) {
	root, s := newStore(t)
	res, err := Capture(s, now, store.KindUnset, SourceAdhoc, "Tagged", "b", []string{"work", "ideas"})
	require.NoError(t, err)
	data, _ := os.ReadFile(filepath.Join(root, res.Rel))
	n := store.ParseNote("x.md", data)
	assert.Empty(t, n.Frontmatter.Get("kind").Value)
	assert.Len(t, n.Frontmatter.Get("tags").Content, 2)
}

func TestSplitText(t *testing.T) {
	tests := []struct{ name, in, title, body string }{
		{"one line", "Call mum about the thing", "Call mum about the thing", ""},
		{"first line and the rest", "Plan\nline two\nline three\n", "Plan", "line two\nline three"},
		{"leading blank lines are skipped", "\n\n  Plan  \nbody", "Plan", "body"},
		{"blank lines after the title are dropped", "Plan\n\n\nbody here", "Plan", "body here"},
		{"windows line endings", "Plan\r\nbody\r\n", "Plan", "body"},
		{"only blank", " \n \n", "", ""},
		{"empty", "", "", ""},
		{"body keeps its inner blank lines", "T\na\n\nb", "T", "a\n\nb"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			title, body := SplitText(tt.in)
			assert.Equal(t, tt.title, title)
			assert.Equal(t, tt.body, body)
		})
	}
}

func TestExtractFromAnOrdinaryNoteLinksBackToIt(t *testing.T) {
	root, s := newStore(t)
	origin := "inbox/Standup transcript.md"
	_, err := s.Create(origin, []byte("---\nsource: transcript\n---\nraw\n"))
	require.NoError(t, err)
	originBytes, _ := os.ReadFile(filepath.Join(root, origin))

	res, err := Extract(s, now, store.KindFact, origin, "Launch moved", "The launch moved to May.\nOwner: Sam")
	require.NoError(t, err)
	assert.Equal(t, "inbox/Launch moved.md", res.Rel)
	assert.False(t, res.Unlinked)

	data, _ := os.ReadFile(filepath.Join(root, res.Rel))
	n := store.ParseNote("x.md", data)
	assert.Equal(t, store.KindFact, n.Kind)
	assert.Equal(t, "extract", n.Frontmatter.Get("source").Value)
	assert.Equal(t, "2026-10-30", n.Frontmatter.Get("date").Value)
	rel := n.Frontmatter.Get("related")
	require.NotNil(t, rel)
	assert.Equal(t, "[[Standup transcript]]", rel.Content[0].Value)
	assert.Equal(t, "The launch moved to May.\nOwner: Sam\n", n.Body)

	after, _ := os.ReadFile(filepath.Join(root, origin))
	assert.Equal(t, originBytes, after, "the origin note is never touched")

	require.NoError(t, res.Action.Undo())
	assert.NoFileExists(t, filepath.Join(root, res.Rel))
}

func TestExtractFromADailyNoteDoesNotLinkBack(t *testing.T) {
	root, s := newStore(t)
	for _, origin := range []string{"inbox/_daily/2026-10-29.md", "archives/_daily/2026-10-29.md"} {
		res, err := Extract(s, now, store.KindIdea, origin, "From "+filepath.Dir(origin), "a thought")
		require.NoError(t, err)
		data, _ := os.ReadFile(filepath.Join(root, res.Rel))
		n := store.ParseNote("x.md", data)
		assert.Equal(t, "daily 2026-10-29", n.Frontmatter.Get("source").Value, "plain text, since the daily will be deleted")
		assert.Empty(t, n.Frontmatter.Get("related").Content, "no link to a note that will disappear")
		assert.False(t, res.Unlinked)
	}
}

func TestExtractCannotLinkToANameWithLinkBreakingCharacters(t *testing.T) {
	root, s := newStore(t)
	res, err := Extract(s, now, store.KindFact, "inbox/C# notes.md", "Point", "text")
	require.NoError(t, err)
	assert.True(t, res.Unlinked)
	data, _ := os.ReadFile(filepath.Join(root, res.Rel))
	assert.Empty(t, store.ParseNote("x.md", data).Frontmatter.Get("related").Content)
}

func TestExtractRefusals(t *testing.T) {
	_, s := newStore(t)
	_, err := Extract(s, now, store.KindFact, "inbox/a.md", "///", "text")
	assert.ErrorIs(t, err, ErrEmpty, "a title is needed")
	_, err = Extract(s, now, store.KindFact, "inbox/a.md", "Title", " \n ")
	assert.ErrorIs(t, err, ErrNoText)
	_, err = Extract(s, now, store.KindUnset, "inbox/a.md", "Title", "text")
	assert.ErrorIs(t, err, ErrNoKind, "an extract always has a kind chosen")
}

func TestExtractNumbersACollidingName(t *testing.T) {
	_, s := newStore(t)
	a, err := Extract(s, now, store.KindFact, "inbox/o.md", "Same", "one")
	require.NoError(t, err)
	b, err := Extract(s, now, store.KindFact, "inbox/o.md", "Same", "two")
	require.NoError(t, err)
	assert.Equal(t, "inbox/Same.md", a.Rel)
	assert.Equal(t, "inbox/Same 2.md", b.Rel)
}

func TestDailyCreatesTheNoteForADay(t *testing.T) {
	root, s := newStore(t)
	tomorrow := now.AddDate(0, 0, 1)

	res, err := Daily(s, tomorrow)
	require.NoError(t, err)
	assert.Equal(t, "inbox/_daily/2026-10-31.md", res.Rel)
	assert.False(t, res.Existed)

	data, err := os.ReadFile(filepath.Join(root, res.Rel))
	require.NoError(t, err)
	n := store.ParseNote("x.md", data)
	assert.Equal(t, "2026-10-31", n.Frontmatter.Get("title").Value)
	assert.Equal(t, "2026-10-31", n.Frontmatter.Get("date").Value)
	assert.Equal(t, store.KindUnset, n.Kind, "a daily has no kind")
	assert.NotEmpty(t, n.Frontmatter.Get("uid").Value)

	require.NoError(t, res.Action.Undo())
	assert.NoFileExists(t, filepath.Join(root, res.Rel))
}

func TestDailyKeepsAnExistingNote(t *testing.T) {
	root, s := newStore(t)
	path := filepath.Join(root, "inbox", "_daily", "2026-10-30.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("mine\n"), 0o644))

	res, err := Daily(s, now)
	require.NoError(t, err)
	assert.Equal(t, "inbox/_daily/2026-10-30.md", res.Rel)
	assert.True(t, res.Existed)
	got, _ := os.ReadFile(path)
	assert.Equal(t, "mine\n", string(got), "left untouched")
}

func TestDailyFindsAnArchivedNoteInsteadOfMakingANewOne(t *testing.T) {
	root, s := newStore(t)
	path := filepath.Join(root, "archives", "_daily", "2026-10-29.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("old\n"), 0o644))

	res, err := Daily(s, now.AddDate(0, 0, -1))
	require.NoError(t, err)
	assert.Equal(t, "archives/_daily/2026-10-29.md", res.Rel)
	assert.True(t, res.Existed)
	assert.NoFileExists(t, filepath.Join(root, "inbox", "_daily", "2026-10-29.md"))
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := clip.ParseURL(raw)
	require.NoError(t, err)
	return u
}

func TestClipWritesAWebNoteNamedAfterThePage(t *testing.T) {
	root, s := newStore(t)
	res, err := Clip(s, now, "", mustURL(t, "https://example.com/a/b?x=1"), "Tom: a / b?")
	require.NoError(t, err)
	assert.Equal(t, "inbox/Tom a b.md", res.Rel)

	data, err := os.ReadFile(filepath.Join(root, res.Rel))
	require.NoError(t, err)
	n := store.ParseNote("x.md", data)
	assert.Equal(t, store.KindLink, n.Kind)
	assert.Equal(t, "https://example.com/a/b?x=1", n.Frontmatter.Get("source").Value)
	assert.Equal(t, "2026-10-30", n.Frontmatter.Get("date").Value)
	assert.Equal(t, "Tom a b", n.Frontmatter.Get("title").Value)
	assert.Empty(t, n.Body, "the address lives in the properties only")

	require.NoError(t, res.Action.Undo())
	assert.NoFileExists(t, filepath.Join(root, res.Rel))
}

func TestClipFallsBackToTheAddressWithoutATitle(t *testing.T) {
	tests := []struct{ raw, title, want string }{
		{"https://example.com/docs/intro", "", "inbox/example.com - docs - intro.md"},
		{"https://example.com/", "  ", "inbox/example.com.md"},
		{"http://example.com:8080", "///", "inbox/example.com.md"},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			_, s := newStore(t)
			res, err := Clip(s, now, "", mustURL(t, tt.raw), tt.title)
			require.NoError(t, err)
			assert.Equal(t, tt.want, res.Rel)
		})
	}
}

func TestClipNumbersATakenName(t *testing.T) {
	_, s := newStore(t)
	u := mustURL(t, "https://example.com/x")
	first, err := Clip(s, now, "", u, "Same")
	require.NoError(t, err)
	second, err := Clip(s, now, "", u, "Same")
	require.NoError(t, err)
	assert.Equal(t, "inbox/Same.md", first.Rel)
	assert.Equal(t, "inbox/Same 2.md", second.Rel)
}

func TestNameFitsAFileName(t *testing.T) {
	name := Name(strings.Repeat("😀", 80))
	require.NotEmpty(t, name)
	assert.LessOrEqual(t, len(name)+len(" 999"), 255, "room for the number a taken name gets")
	assert.True(t, utf8.ValidString(name), "cut on a character boundary")
	assert.True(t, strings.HasSuffix(name, ".md"))
}

func TestClipWithAHugeTitleStillSaves(t *testing.T) {
	_, s := newStore(t)
	_, err := Clip(s, now, "", mustURL(t, "https://example.com/x"), strings.Repeat("😀", 80))
	require.NoError(t, err)
}

func TestClipSavesInTheFolderGiven(t *testing.T) {
	root, s := newStore(t)
	u := mustURL(t, "https://example.com/x")

	res, err := Clip(s, now, "projects/alpha", u, "Same")
	require.NoError(t, err)
	assert.Equal(t, "projects/alpha/Same.md", res.Rel)
	assert.FileExists(t, filepath.Join(root, "projects", "alpha", "Same.md"), "the folder is created when missing")

	again, err := Clip(s, now, "projects/alpha", u, "Same")
	require.NoError(t, err)
	assert.Equal(t, "projects/alpha/Same 2.md", again.Rel, "a taken name is numbered in that folder")

	inbox, err := Clip(s, now, "", u, "Same")
	require.NoError(t, err)
	assert.Equal(t, "inbox/Same.md", inbox.Rel, "no folder means the inbox")

	require.NoError(t, res.Action.Undo())
	assert.NoFileExists(t, filepath.Join(root, "projects", "alpha", "Same.md"))
}

func TestClipRefusesAFolderOutsideTheNotebook(t *testing.T) {
	root, s := newStore(t)
	u := mustURL(t, "https://example.com/x")
	for _, dir := range []string{"../escape", "/tmp/abs", "a/../../up", "a\x00b"} {
		_, err := Clip(s, now, dir, u, "Nope")
		assert.Error(t, err, dir)
	}
	parent := filepath.Dir(root)
	assert.NoFileExists(t, filepath.Join(parent, "escape", "Nope.md"))
	assert.NoFileExists(t, "/tmp/abs/Nope.md")
}

const dailyTemplate = "---\nuid: old-uid\ntitle: daily\nkind: \"\"\nsource: daily\ndate: 2026-10-26\ntags: [daily]\npeople: []\nrelated: []\n---\n\n# Daily Note — 2026-10-26 (Monday)\n\n## Goals\n\n- [ ] -\n"

func TestDailyFromATemplateFillsInTheDay(t *testing.T) {
	root, s := newStore(t)
	tomorrow := now.AddDate(0, 0, 1) // 2026-10-31, a Saturday

	res, err := DailyFrom(s, tomorrow, []byte(dailyTemplate))
	require.NoError(t, err)
	assert.Equal(t, "inbox/_daily/2026-10-31.md", res.Rel)
	assert.False(t, res.Existed)

	data, err := os.ReadFile(filepath.Join(root, res.Rel))
	require.NoError(t, err)
	n := store.ParseNote("x.md", data)
	assert.Equal(t, "2026-10-31", n.Frontmatter.Get("title").Value)
	assert.Equal(t, "2026-10-31", n.Frontmatter.Get("date").Value)
	assert.NotEqual(t, "old-uid", n.Frontmatter.Get("uid").Value)
	assert.Equal(t, "daily", n.Frontmatter.Get("source").Value)
	assert.Contains(t, n.Body, "# Daily Note — 2026-10-31 (Saturday)")
	assert.Contains(t, n.Body, "## Goals")

	require.NoError(t, res.Action.Undo())
	assert.NoFileExists(t, filepath.Join(root, res.Rel))
	require.NoError(t, res.Action.Redo())
	assert.FileExists(t, filepath.Join(root, res.Rel))
}

func TestDailyFromATemplateKeepsAnExistingNote(t *testing.T) {
	root, s := newStore(t)
	first, err := Daily(s, now)
	require.NoError(t, err)
	before, _ := os.ReadFile(filepath.Join(root, first.Rel))

	again, err := DailyFrom(s, now, []byte(dailyTemplate))
	require.NoError(t, err)
	assert.True(t, again.Existed)
	after, _ := os.ReadFile(filepath.Join(root, first.Rel))
	assert.Equal(t, string(before), string(after), "a template never changes a note that is there")
}

func TestDailyFromAnUnreadableTemplateCreatesNothing(t *testing.T) {
	root, s := newStore(t)
	_, err := DailyFrom(s, now, []byte("---\nkey: [oops\n---\nbody"))
	assert.ErrorIs(t, err, store.ErrFrontmatterInvalid)
	assert.NoFileExists(t, filepath.Join(root, "inbox/_daily", now.Format("2006-01-02")+".md"))
}
