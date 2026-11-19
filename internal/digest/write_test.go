package digest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tkilb/para/internal/loader"
	"github.com/tkilb/para/internal/store"
)

var day = time.Date(2026, 11, 3, 14, 30, 0, 0, time.Local)

const importKind = store.Kind("import")

func newStore(t *testing.T) (string, store.Store) {
	t.Helper()
	root := t.TempDir()
	return root, store.NewMarkdown(root)
}

func fileSource(name, text string) loader.Source {
	return loader.Source{Kind: loader.File, Name: name, Text: text}
}

func readNote(t *testing.T, root, rel string) store.Note {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, rel))
	require.NoError(t, err)
	n := store.ParseNote(filepath.Base(rel), data)
	require.NotNil(t, n.Frontmatter, "%s has properties", rel)
	return n
}

func prop(n store.Note, key string) string { return n.Frontmatter.Get(key).Value }

func tagsOf(n store.Note) []string {
	node := n.Frontmatter.Get("tags")
	var tags []string
	if node != nil {
		for _, c := range node.Content {
			tags = append(tags, c.Value)
		}
	}
	return tags
}

func names(t *testing.T, root, rel string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, rel))
	require.NoError(t, err)
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

func TestBeginCreatesTheFolderAndSource(t *testing.T) {
	root, s := newStore(t)
	f, err := Begin(s, day, importKind, fileSource("Standup 2026-11-03", "Sam: hello\nAnna: hi"))
	require.NoError(t, err)

	assert.Equal(t, "inbox/IMPORT_2026-11-03_Standup 2026-11-03", f.Rel)
	assert.Equal(t, []string{"_source.md"}, names(t, root, f.Rel))

	n := readNote(t, root, f.Rel+"/_source.md")
	assert.Equal(t, "_source", n.Title)
	assert.Equal(t, "import", prop(n, "kind"))
	assert.Equal(t, "adhoc", prop(n, "source"), "a placeholder until the agent judges the source")
	assert.Equal(t, "2026-11-03", prop(n, "date"))
	assert.Empty(t, tagsOf(n))
	assert.Equal(t, "Sam: hello\nAnna: hi\n", n.Body, "the text, with a final line break")
}

func TestBeginKeepsTheTextExactly(t *testing.T) {
	root, s := newStore(t)
	text := "line one\r\n\r\n  indented\ttabbed\n\nlast line\n"
	f, err := Begin(s, day, importKind, fileSource("x", text))
	require.NoError(t, err)
	n := readNote(t, root, f.Rel+"/_source.md")
	assert.Equal(t, text, n.Body, "no change when it already ends in a line break")
}

func TestBeginWebSourceRecordsTheAddress(t *testing.T) {
	root, s := newStore(t)
	src := loader.Source{Kind: loader.Web, Name: "Pricing Rules", Origin: "https://wiki.example.com/pricing", Text: "# Pricing Rules\n\nbody"}
	f, err := Begin(s, day, importKind, src)
	require.NoError(t, err)
	assert.Equal(t, "inbox/IMPORT_2026-11-03_Pricing Rules", f.Rel)
	assert.Equal(t, "https://wiki.example.com/pricing", prop(readNote(t, root, f.Rel+"/_source.md"), "source"))
}

func TestBeginFolderNames(t *testing.T) {
	tests := []struct {
		name string
		src  loader.Source
		want string
	}{
		{"unsafe characters are removed", fileSource("Q3/Q4: plan", "x"), "IMPORT_2026-11-03_Q3Q4 plan"},
		{"a path trick is only text", fileSource("../../evil", "x"), "IMPORT_2026-11-03_evil"},
		{"control characters go", fileSource("a\x1b[2Jb\nc", "x"), "IMPORT_2026-11-03_a2Jb c"},
		{"a file with no usable name", fileSource("///", "x"), "IMPORT_2026-11-03_file"},
		{"clipboard", loader.Source{Kind: loader.Clipboard, Name: "clipboard", Text: "x"}, "IMPORT_2026-11-03_clipboard"},
		{"an empty clipboard name", loader.Source{Kind: loader.Clipboard, Text: "x"}, "IMPORT_2026-11-03_clipboard"},
		{"an empty web name", loader.Source{Kind: loader.Web, Origin: "https://a.example.com", Text: "x"}, "IMPORT_2026-11-03_web"},
		{"a long name is cut", fileSource(strings.Repeat("a", 300), "x"), "IMPORT_2026-11-03_" + strings.Repeat("a", 80)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, s := newStore(t)
			f, err := Begin(s, day, importKind, tt.src)
			require.NoError(t, err)
			assert.Equal(t, "inbox/"+tt.want, f.Rel)
		})
	}
}

func TestBeginTakenNameGetsANumber(t *testing.T) {
	root, s := newStore(t)
	a, err := Begin(s, day, importKind, fileSource("Standup", "one"))
	require.NoError(t, err)
	b, err := Begin(s, day, importKind, fileSource("Standup", "two"))
	require.NoError(t, err)
	c, err := Begin(s, day, importKind, fileSource("Standup", "three"))
	require.NoError(t, err)
	assert.Equal(t, "inbox/IMPORT_2026-11-03_Standup", a.Rel)
	assert.Equal(t, "inbox/IMPORT_2026-11-03_Standup 2", b.Rel)
	assert.Equal(t, "inbox/IMPORT_2026-11-03_Standup 3", c.Rel)
	assert.Equal(t, "one\n", readNote(t, root, a.Rel+"/_source.md").Body, "an earlier import is never touched")
}

func TestBeginWithoutAKindWritesNone(t *testing.T) {
	root, s := newStore(t)
	f, err := Begin(s, day, store.KindUnset, fileSource("x", "text"))
	require.NoError(t, err)
	assert.Empty(t, prop(readNote(t, root, f.Rel+"/_source.md"), "kind"), "the standard property is there but empty")
}

func TestBeginRefusesEmptyText(t *testing.T) {
	_, s := newStore(t)
	_, err := Begin(s, day, importKind, fileSource("x", "  \n "))
	assert.ErrorIs(t, err, ErrEmptySource)
}

func TestBeginActionRemovesTheFolder(t *testing.T) {
	root, s := newStore(t)
	f, err := Begin(s, day, importKind, fileSource("x", "text"))
	require.NoError(t, err)
	require.NoError(t, f.Action().Undo())
	assert.NoDirExists(t, filepath.Join(root, f.Rel), "a failed import can be undone")
	require.NoError(t, f.Action().Redo())
	assert.FileExists(t, filepath.Join(root, f.Rel, "_source.md"))
}

var sample = Digest{
	Topic:  "Pricing review",
	Source: "meeting",
	Notes: []Note{
		{Title: "Tier pricing", Body: "Three tiers: basic, plus, pro.", Tags: []string{"pricing", "tiers"}},
		{Title: "Renewal rules", Body: "Renewals lock for a year.\n\n- prorated upgrades", Tags: []string{"pricing"}},
	},
	Actions: []string{"Send the price sheet", "Review renewals with Sam"},
}

func begun(t *testing.T) (string, *Folder) {
	t.Helper()
	root, s := newStore(t)
	f, err := Begin(s, day, importKind, fileSource("Standup", "the transcript"))
	require.NoError(t, err)
	return root, f
}

func TestFinishWritesTheFiles(t *testing.T) {
	root, f := begun(t)
	res, err := f.Finish(sample)
	require.NoError(t, err)

	assert.Equal(t, "inbox/IMPORT_2026-11-03_Pricing review", res.Rel, "renamed to the agent's topic")
	assert.Equal(t, []string{"Renewal rules.md", "Tier pricing.md", "_actions.md", "_source.md"}, names(t, root, res.Rel))
	assert.Equal(t, []string{"Tier pricing.md", "Renewal rules.md", "_actions.md"}, res.Files, "in the order written, without _source.md")

	n := readNote(t, root, res.Rel+"/Tier pricing.md")
	assert.Equal(t, "Tier pricing", n.Title)
	assert.Equal(t, "import", prop(n, "kind"))
	assert.Equal(t, "meeting", prop(n, "source"))
	assert.Equal(t, "2026-11-03", prop(n, "date"))
	assert.Equal(t, []string{"pricing", "tiers"}, tagsOf(n))
	assert.Empty(t, n.Frontmatter.Get("related").Content, "an import never links")
	assert.Equal(t, "Three tiers: basic, plus, pro.\n", n.Body)

	n = readNote(t, root, res.Rel+"/Renewal rules.md")
	assert.Equal(t, "Renewals lock for a year.\n\n- prorated upgrades\n", n.Body)
}

func TestFinishWritesActionsApart(t *testing.T) {
	root, f := begun(t)
	res, err := f.Finish(sample)
	require.NoError(t, err)

	n := readNote(t, root, res.Rel+"/_actions.md")
	assert.Equal(t, "import", prop(n, "kind"))
	assert.Equal(t, "meeting", prop(n, "source"))
	assert.Equal(t, "- Send the price sheet\n- Review renewals with Sam\n", n.Body)
	for _, name := range []string{"Tier pricing.md", "Renewal rules.md"} {
		body := readNote(t, root, res.Rel+"/"+name).Body
		assert.NotContains(t, body, "price sheet", "actions never mix into the notes")
	}
}

func TestFinishWritesFactsApart(t *testing.T) {
	root, f := begun(t)
	d := sample
	d.Facts = []string{"Renewals lock for a year", "Pro tier costs 40 a seat"}
	res, err := f.Finish(d)
	require.NoError(t, err)

	n := readNote(t, root, res.Rel+"/_facts.md")
	assert.Equal(t, "import", prop(n, "kind"))
	assert.Equal(t, "- Renewals lock for a year\n- Pro tier costs 40 a seat\n", n.Body)
	assert.Contains(t, res.Files, "_facts.md")
}

func TestFinishWithoutFactsWritesNoFactsFile(t *testing.T) {
	root, f := begun(t)
	res, err := f.Finish(sample)
	require.NoError(t, err)
	assert.NoFileExists(t, filepath.Join(root, res.Rel, "_facts.md"))
	assert.NotContains(t, res.Files, "_facts.md")
}

func TestFinishNoteNamedFactsDoesNotCollide(t *testing.T) {
	root, f := begun(t)
	d := Digest{Notes: []Note{{Title: "Facts", Body: "x"}}, Facts: []string{"A fact"}}
	res, err := f.Finish(d)
	require.NoError(t, err)
	assert.Equal(t, "A fact", strings.TrimSpace(strings.TrimPrefix(readNote(t, root, res.Rel+"/_facts.md").Body, "- ")))
}

func TestFinishWritesConclusionsApart(t *testing.T) {
	root, f := begun(t)
	d := sample
	d.Conclusions = []string{"Ship in March", "Drop tier 3 because nobody buys it"}
	res, err := f.Finish(d)
	require.NoError(t, err)

	n := readNote(t, root, res.Rel+"/_conclusions.md")
	assert.Equal(t, "import", prop(n, "kind"))
	assert.Equal(t, "- Ship in March\n- Drop tier 3 because nobody buys it\n", n.Body)
	assert.Contains(t, res.Files, "_conclusions.md")
}

func TestFinishWithoutConclusionsWritesNoConclusionsFile(t *testing.T) {
	root, f := begun(t)
	res, err := f.Finish(sample)
	require.NoError(t, err)
	assert.NoFileExists(t, filepath.Join(root, res.Rel, "_conclusions.md"))
	assert.NotContains(t, res.Files, "_conclusions.md")
}

func TestFinishConclusionsAloneIsNotEmpty(t *testing.T) {
	_, f := begun(t)
	res, err := f.Finish(Digest{Conclusions: []string{"It works"}})
	require.NoError(t, err)
	assert.Equal(t, []string{"_conclusions.md"}, res.Files)
}

func TestFinishNoteNamedConclusionsDoesNotCollide(t *testing.T) {
	root, f := begun(t)
	d := Digest{Notes: []Note{{Title: "Conclusions", Body: "x"}}, Conclusions: []string{"A decision"}}
	res, err := f.Finish(d)
	require.NoError(t, err)
	assert.Equal(t, "A decision", strings.TrimSpace(strings.TrimPrefix(readNote(t, root, res.Rel+"/_conclusions.md").Body, "- ")))
}

func TestFinishWithoutActionsWritesNoActionsFile(t *testing.T) {
	root, f := begun(t)
	d := sample
	d.Actions = nil
	res, err := f.Finish(d)
	require.NoError(t, err)
	assert.NoFileExists(t, filepath.Join(root, res.Rel, "_actions.md"))
	assert.NotContains(t, res.Files, "_actions.md")
}

func TestFinishWithOnlyActions(t *testing.T) {
	root, f := begun(t)
	res, err := f.Finish(Digest{Source: "meeting", Actions: []string{"Call Sam"}})
	require.NoError(t, err)
	assert.Equal(t, []string{"_actions.md", "_source.md"}, names(t, root, res.Rel))
}

func TestFinishUpdatesTheSourceFileToTheJudgedSource(t *testing.T) {
	root, f := begun(t)
	res, err := f.Finish(sample)
	require.NoError(t, err)
	n := readNote(t, root, res.Rel+"/_source.md")
	assert.Equal(t, "meeting", prop(n, "source"))
	assert.Equal(t, "the transcript\n", n.Body, "the text is untouched")
}

func TestFinishKeepsTheAddressForAWebImport(t *testing.T) {
	root, s := newStore(t)
	src := loader.Source{Kind: loader.Web, Name: "Pricing Rules", Origin: "https://wiki.example.com/p", Text: "body text"}
	f, err := Begin(s, day, importKind, src)
	require.NoError(t, err)
	res, err := f.Finish(sample)
	require.NoError(t, err)
	for _, name := range []string{"_source.md", "Tier pricing.md", "_actions.md"} {
		assert.Equal(t, "https://wiki.example.com/p", prop(readNote(t, root, res.Rel+"/"+name), "source"), name)
	}
}

func TestFinishWithoutATopicKeepsTheSourceName(t *testing.T) {
	_, f := begun(t)
	d := sample
	d.Topic = ""
	res, err := f.Finish(d)
	require.NoError(t, err)
	assert.Equal(t, "inbox/IMPORT_2026-11-03_Standup", res.Rel)
}

func TestFinishTopicNameTakenGetsANumber(t *testing.T) {
	root, s := newStore(t)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "inbox", "IMPORT_2026-11-03_Pricing review"), 0o755))
	f, err := Begin(s, day, importKind, fileSource("Standup", "text"))
	require.NoError(t, err)
	res, err := f.Finish(sample)
	require.NoError(t, err)
	assert.Equal(t, "inbox/IMPORT_2026-11-03_Pricing review 2", res.Rel)
	assert.DirExists(t, filepath.Join(root, "inbox", "IMPORT_2026-11-03_Pricing review"), "the existing folder is untouched")
}

func TestFinishTopicIsCleanedAgain(t *testing.T) {
	_, f := begun(t)
	d := sample
	d.Topic = "../../escape: now"
	res, err := f.Finish(d)
	require.NoError(t, err)
	assert.Equal(t, "inbox/IMPORT_2026-11-03_escape now", res.Rel, "a hand-made digest cannot leave the folder")
}

func TestFinishNeverWritesOutsideTheFolder(t *testing.T) {
	root, f := begun(t)
	d := Digest{Source: "meeting", Notes: []Note{
		{Title: "../../outside", Body: "x"},
		{Title: "/etc/passwd", Body: "y"},
		{Title: `a\b`, Body: "z"},
		{Title: "_source", Body: "does not replace the source"},
		{Title: "_actions", Body: "nor the actions"},
	}}
	res, err := f.Finish(d)
	require.NoError(t, err)

	assert.Equal(t, "the transcript\n", readNote(t, root, res.Rel+"/_source.md").Body)
	for _, name := range names(t, root, res.Rel) {
		assert.False(t, strings.ContainsAny(name, `/\`), name)
	}
	assert.NoFileExists(t, filepath.Join(root, "outside.md"))
	assert.NoFileExists(t, filepath.Join(root, "inbox", "outside.md"))
	entries := names(t, root, "inbox")
	assert.Len(t, entries, 1, "only the import folder is in the inbox: %v", entries)
}

func TestFinishDuplicateOrReservedNamesGetANumber(t *testing.T) {
	root, f := begun(t)
	d := Digest{Source: "meeting", Notes: []Note{
		{Title: "Pricing", Body: "one"},
		{Title: "pricing", Body: "two"},
		{Title: "_source", Body: "three"},
	}}
	res, err := f.Finish(d)
	require.NoError(t, err, "a hand-made digest with clashing names still writes every note")
	assert.Len(t, res.Files, 3)
	assert.Len(t, names(t, root, res.Rel), 4)
}

func TestFinishDropsNotesWithNothingUsable(t *testing.T) {
	root, f := begun(t)
	d := Digest{Source: "meeting", Notes: []Note{
		{Title: "///", Body: "no title"},
		{Title: "Kept", Body: "body"},
		{Title: "Empty body", Body: "  \n"},
	}}
	res, err := f.Finish(d)
	require.NoError(t, err)
	assert.Equal(t, []string{"Kept.md", "_source.md"}, names(t, root, res.Rel))
}

func TestFinishWithNothingToWriteIsEmpty(t *testing.T) {
	_, f := begun(t)
	_, err := f.Finish(Digest{Source: "meeting"})
	assert.ErrorIs(t, err, ErrEmpty)
}

func TestFinishWithoutAKindWritesNoKindAndSaysSo(t *testing.T) {
	root, s := newStore(t)
	f, err := Begin(s, day, store.KindUnset, fileSource("x", "text"))
	require.NoError(t, err)
	res, err := f.Finish(sample)
	require.NoError(t, err)
	assert.True(t, res.KindMissing, "the caller warns that the import kind is not configured")
	assert.Empty(t, prop(readNote(t, root, res.Rel+"/Tier pricing.md"), "kind"))
}

func TestFinishIsOneUndoEntry(t *testing.T) {
	root, f := begun(t)
	res, err := f.Finish(sample)
	require.NoError(t, err)

	require.NoError(t, res.Action.Undo())
	assert.NoDirExists(t, filepath.Join(root, res.Rel))
	assert.NoDirExists(t, filepath.Join(root, "inbox", "IMPORT_2026-11-03_Standup"), "not even the first name is left")
	assert.NoDirExists(t, filepath.Join(root, "inbox"), "the import made the inbox, so undo removes it: as it was before")

	require.NoError(t, res.Action.Redo())
	assert.Equal(t, []string{"Renewal rules.md", "Tier pricing.md", "_actions.md", "_source.md"}, names(t, root, res.Rel))
	assert.Equal(t, "meeting", prop(readNote(t, root, res.Rel+"/_source.md"), "source"))
}

func TestFinishTwiceIsRefused(t *testing.T) {
	_, f := begun(t)
	_, err := f.Finish(sample)
	require.NoError(t, err)
	_, err = f.Finish(sample)
	assert.ErrorIs(t, err, ErrFinished)
}

func TestImportKind(t *testing.T) {
	t.Cleanup(store.ResetKinds)
	store.ResetKinds()
	assert.Equal(t, store.KindUnset, ImportKind(), "not configured: unset")

	require.NoError(t, store.SetKinds([]store.KindSpec{{Name: "import", Badge: "IMPT", Color: "#ffffff", Meaning: "Imported"}}))
	assert.Equal(t, importKind, ImportKind())
}
