package digest

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tkilb/para/internal/loader"
	"github.com/tkilb/para/internal/store"
)

var laterDay = time.Date(2026, 11, 20, 9, 0, 0, 0, time.Local)

func readFile(t *testing.T, root, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, rel))
	require.NoError(t, err)
	return string(b)
}

// imported runs a full import of a transcript with dg and returns the store,
// the notebook root and the result.
func imported(t *testing.T, dg Digester) (string, store.Store, Result) {
	t.Helper()
	root, s := newStore(t)
	res, err := Import(context.Background(), s, day, importKind, dg, fileSource("Standup", "Sam: the transcript\n\nAnna: more"))
	require.NoError(t, err)
	return root, s, res
}

func TestImportRunsTheWholeFlow(t *testing.T) {
	f := &Fake{Result: sample}
	root, s, res := imported(t, f)

	assert.Equal(t, "inbox/IMPORT_2026-11-03_Pricing review", res.Rel)
	assert.Equal(t, []string{"Renewal rules.md", "Tier pricing.md", "_actions.md", "_source.md"}, names(t, root, res.Rel))
	assert.Equal(t, []string{"Sam: the transcript\n\nAnna: more\n"}, f.Calls, "the agent gets the source text")
	_ = s
}

func TestImportKeepsTheSourceWhenTheAgentFails(t *testing.T) {
	boom := errors.New("backend down")
	root, s := newStore(t)
	res, err := Import(context.Background(), s, day, importKind, &Fake{Err: boom}, fileSource("Standup", "the transcript"))

	require.ErrorIs(t, err, boom)
	assert.Equal(t, "inbox/IMPORT_2026-11-03_Standup", res.Rel, "the fallback name, from the source")
	assert.Equal(t, []string{"_source.md"}, names(t, root, res.Rel))
	assert.Equal(t, "the transcript\n", readNote(t, root, res.Rel+"/_source.md").Body)

	require.NoError(t, res.Action.Undo(), "the caller can still undo what exists")
	assert.NoDirExists(t, filepath.Join(root, res.Rel))
}

func TestImportReportsAnEmptyAnswerButKeepsTheSource(t *testing.T) {
	root, s := newStore(t)
	res, err := Import(context.Background(), s, day, importKind, &Fake{Err: ErrEmpty}, fileSource("Standup", "hello everyone"))
	require.ErrorIs(t, err, ErrEmpty)
	assert.FileExists(t, filepath.Join(root, res.Rel, "_source.md"))
}

func TestImportWarnsWhenThePartsWereNotMerged(t *testing.T) {
	d := sample
	d.Unmerged = true
	_, _, res := imported(t, &Fake{Result: d})
	require.Len(t, res.Warnings, 1)
	assert.Contains(t, res.Warnings[0], "merge")
}

func TestImportRefusesEmptyText(t *testing.T) {
	_, s := newStore(t)
	f := &Fake{Result: sample}
	_, err := Import(context.Background(), s, day, importKind, f, fileSource("x", " \n"))
	assert.ErrorIs(t, err, ErrEmptySource)
	assert.Empty(t, f.Calls, "the agent is never called")
}

func TestFindImportFolder(t *testing.T) {
	tests := []struct {
		name, in, want string
		ok             bool
	}{
		{"the folder itself", "inbox/IMPORT_2026-11-03_Standup", "inbox/IMPORT_2026-11-03_Standup", true},
		{"a file in it", "inbox/IMPORT_2026-11-03_Standup/Tier pricing.md", "inbox/IMPORT_2026-11-03_Standup", true},
		{"a file in a folder in it", "inbox/IMPORT_2026-11-03_Standup/sub/deep/x.md", "inbox/IMPORT_2026-11-03_Standup", true},
		{"after being moved into PARA", "projects/alpha/IMPORT_2026-11-03_Standup/_source.md", "projects/alpha/IMPORT_2026-11-03_Standup", true},
		{"the nearest one wins", "inbox/import_a/import_b/x.md", "inbox/import_a/import_b", true},
		{"an ordinary note", "inbox/Note.md", "", false},
		{"an ordinary folder", "inbox/_daily", "", false},
		{"a name that only starts like it", "inbox/important/x.md", "", false},
		{"the old lower case prefix", "inbox/import_2026-11-03_Standup/x.md", "inbox/import_2026-11-03_Standup", true},
		{"the prefix inside a name", "inbox/my_import_x/x.md", "", false},
		{"the prefix alone", "inbox/import_", "inbox/import_", true},
		{"empty", "", "", false},
		{"a file named like one", "inbox/import_x.md", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := FindImportFolder(tt.in)
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRerunReplacesTheGeneratedFiles(t *testing.T) {
	root, s, first := imported(t, &Fake{Result: sample})

	second := Digest{Topic: "A different topic", Source: "slack", Notes: []Note{{Title: "New idea", Body: "fresh", Tags: []string{"new"}}}}
	f := &Fake{Result: second}
	res, err := Rerun(context.Background(), s, laterDay, importKind, f, first.Rel)
	require.NoError(t, err)

	assert.Equal(t, first.Rel, res.Rel, "the folder keeps its name, whatever the new topic")
	assert.Equal(t, []string{"New idea.md", "_source.md"}, names(t, root, res.Rel), "old files and the old actions file are gone")
	assert.Equal(t, []string{"New idea.md"}, res.Files)
	assert.Equal(t, []string{"Sam: the transcript\n\nAnna: more\n"}, f.Calls, "the agent gets _source.md again")

	n := readNote(t, root, res.Rel+"/New idea.md")
	assert.Equal(t, "slack", prop(n, "source"))
	assert.Equal(t, "2026-11-03", prop(n, "date"), "the import day, not the day of the re-run")
	assert.Equal(t, "slack", prop(readNote(t, root, res.Rel+"/_source.md"), "source"), "the judged source is updated")
	assert.Equal(t, "Sam: the transcript\n\nAnna: more\n", readNote(t, root, res.Rel+"/_source.md").Body)
}

func TestRerunUndoRestoresTheOldFiles(t *testing.T) {
	root, s, first := imported(t, &Fake{Result: sample})
	before := map[string]string{}
	for _, name := range names(t, root, first.Rel) {
		before[name] = readFile(t, root, filepath.Join(first.Rel, name))
	}

	// "Tier pricing" comes back with a different body: the same name.
	second := Digest{Source: "meeting", Notes: []Note{{Title: "Tier pricing", Body: "rewritten"}, {Title: "Extra", Body: "x"}}}
	res, err := Rerun(context.Background(), s, laterDay, importKind, &Fake{Result: second}, first.Rel)
	require.NoError(t, err)
	assert.Equal(t, "rewritten\n", readNote(t, root, res.Rel+"/Tier pricing.md").Body)

	require.NoError(t, res.Action.Undo())
	assert.Equal(t, before, snapshot(t, root, first.Rel), "every file is back exactly as it was")

	require.NoError(t, res.Action.Redo())
	assert.Equal(t, []string{"Extra.md", "Tier pricing.md", "_source.md"}, names(t, root, res.Rel))
	assert.Equal(t, "rewritten\n", readNote(t, root, res.Rel+"/Tier pricing.md").Body)
}

func snapshot(t *testing.T, root, rel string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, name := range names(t, root, rel) {
		out[name] = readFile(t, root, filepath.Join(rel, name))
	}
	return out
}

func TestRerunDoesNotTouchAnythingWhenTheAgentFails(t *testing.T) {
	root, s, first := imported(t, &Fake{Result: sample})
	before := snapshot(t, root, first.Rel)

	boom := errors.New("backend down")
	_, err := Rerun(context.Background(), s, laterDay, importKind, &Fake{Err: boom}, first.Rel)
	require.ErrorIs(t, err, boom)
	assert.Equal(t, before, snapshot(t, root, first.Rel), "a failed re-run leaves the folder exactly as it was")
}

func TestRerunDoesNotTouchAnythingWhenTheAnswerIsEmpty(t *testing.T) {
	root, s, first := imported(t, &Fake{Result: sample})
	before := snapshot(t, root, first.Rel)

	for name, d := range map[string]Digest{
		"nothing":        {},
		"unusable notes": {Notes: []Note{{Title: "///", Body: "x"}, {Title: "T", Body: " "}}},
	} {
		_, err := Rerun(context.Background(), s, laterDay, importKind, &Fake{Result: d}, first.Rel)
		assert.ErrorIs(t, err, ErrEmpty, name)
		assert.Equal(t, before, snapshot(t, root, first.Rel), name)
	}
}

func TestRerunReplacesFilesTheUserAddedButNotFoldersOrOtherFiles(t *testing.T) {
	root, s, first := imported(t, &Fake{Result: sample})
	dir := filepath.Join(root, first.Rel)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "My own note.md"), []byte("mine"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "attachments"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "attachments", "a.md"), []byte("kept"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "diagram.png"), []byte("png"), 0o644))

	_, err := Rerun(context.Background(), s, laterDay, importKind, &Fake{Result: Digest{Source: "meeting", Notes: []Note{{Title: "N", Body: "b"}}}}, first.Rel)
	require.NoError(t, err)

	assert.NoFileExists(t, filepath.Join(dir, "My own note.md"), "the import works or it does not: every note but _source.md is replaced")
	assert.FileExists(t, filepath.Join(dir, "attachments", "a.md"), "a folder is not a generated file")
	assert.FileExists(t, filepath.Join(dir, "diagram.png"), "neither is a file that is not a note")
	assert.FileExists(t, filepath.Join(dir, "_source.md"))
}

func TestRerunRefusesWhatIsNotAnImportFolder(t *testing.T) {
	root, s, first := imported(t, &Fake{Result: sample})
	f := &Fake{Result: sample}
	require.NoError(t, os.MkdirAll(filepath.Join(root, "inbox", "import_no_source"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "inbox", "ordinary"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "inbox", "ordinary", "_source.md"), []byte("x"), 0o644))

	for name, rel := range map[string]string{
		"no source file":         "inbox/import_no_source",
		"not named like one":     "inbox/ordinary",
		"missing":                "inbox/import_missing",
		"a note, not a folder":   first.Rel + "/Tier pricing.md",
		"empty":                  "",
		"outside the notebook":   "../elsewhere/import_x",
		"the notebook root":      ".",
		"the source file itself": first.Rel + "/_source.md",
	} {
		_, err := Rerun(context.Background(), s, laterDay, importKind, f, rel)
		assert.Error(t, err, name)
	}
	assert.Empty(t, f.Calls, "the agent is never called")
	assert.ErrorIs(t, func() error {
		_, err := Rerun(context.Background(), s, laterDay, importKind, f, "inbox/ordinary")
		return err
	}(), ErrNotImport)
}

func TestRerunWorksOnAFolderMovedIntoPARA(t *testing.T) {
	root, s, first := imported(t, &Fake{Result: sample})
	_, err := s.Move(first.Rel, "projects/alpha/IMPORT_2026-11-03_Pricing review")
	require.NoError(t, err)

	res, err := Rerun(context.Background(), s, laterDay, importKind, &Fake{Result: Digest{Source: "meeting", Notes: []Note{{Title: "N", Body: "b"}}}}, "projects/alpha/IMPORT_2026-11-03_Pricing review")
	require.NoError(t, err)
	assert.Equal(t, "projects/alpha/IMPORT_2026-11-03_Pricing review", res.Rel)
	assert.Equal(t, []string{"N.md", "_source.md"}, names(t, root, res.Rel))
}

func TestRerunOfAWebImportKeepsTheAddress(t *testing.T) {
	root, s := newStore(t)
	src := loader.Source{Kind: loader.Web, Name: "Pricing Rules", Origin: "https://wiki.example.com/p", Text: "page text"}
	first, err := Import(context.Background(), s, day, importKind, &Fake{Result: sample}, src)
	require.NoError(t, err)

	res, err := Rerun(context.Background(), s, laterDay, importKind, &Fake{Result: Digest{Source: "slack", Notes: []Note{{Title: "N", Body: "b"}}}}, first.Rel)
	require.NoError(t, err)
	assert.Equal(t, "https://wiki.example.com/p", prop(readNote(t, root, res.Rel+"/N.md"), "source"), "an address stays the source, whatever the agent judges")
	assert.Equal(t, "https://wiki.example.com/p", prop(readNote(t, root, res.Rel+"/_source.md"), "source"))
}

func TestRerunWithoutAKindSaysSo(t *testing.T) {
	_, s, first := imported(t, &Fake{Result: sample})
	res, err := Rerun(context.Background(), s, laterDay, store.KindUnset, &Fake{Result: sample}, first.Rel)
	require.NoError(t, err)
	assert.True(t, res.KindMissing)
}

func TestRerunStopsWhenCancelled(t *testing.T) {
	root, s, first := imported(t, &Fake{Result: sample})
	before := snapshot(t, root, first.Rel)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Rerun(ctx, s, laterDay, importKind, &Fake{Result: sample}, first.Rel)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, before, snapshot(t, root, first.Rel))
}

func TestRerunTwice(t *testing.T) {
	root, s, first := imported(t, &Fake{Result: sample})
	for i := 0; i < 2; i++ {
		_, err := Rerun(context.Background(), s, laterDay, importKind, &Fake{Result: sample}, first.Rel)
		require.NoError(t, err)
	}
	assert.Equal(t, []string{"Renewal rules.md", "Tier pricing.md", "_actions.md", "_source.md"}, names(t, root, first.Rel))
}
