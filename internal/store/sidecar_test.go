package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bundle makes a note with a sidecar folder holding two images.
func bundle(t *testing.T, root, dir, stem string) {
	t.Helper()
	writeTree(t, root,
		filepath.Join(dir, stem+".md"),
		filepath.Join(dir, stem+".assets", "a.png"),
		filepath.Join(dir, stem+".assets", "b.png"))
}

func TestSidecarOf(t *testing.T) {
	tests := map[string]string{
		"inbox/plan.md":  "inbox/plan.assets",
		"inbox/Plan.MD":  "inbox/Plan.assets",
		"plan.md":        "plan.assets",
		"inbox/pic.png":  "",
		"inbox/plan.md/": "",
		"inbox/plan":     "",
		"":               "",
	}
	for rel, want := range tests {
		assert.Equal(t, want, SidecarOf(rel), rel)
	}
}

func TestMoveCarriesTheSidecar(t *testing.T) {
	root, _ := newRoot(t)
	bundle(t, root, "inbox", "plan")
	s := NewMarkdown(root)

	act, err := s.Move("inbox/plan.md", "projects/idea.md")
	require.NoError(t, err)
	assert.NoFileExists(t, filepath.Join(root, "inbox", "plan.md"))
	assert.NoDirExists(t, filepath.Join(root, "inbox", "plan.assets"))
	assert.FileExists(t, filepath.Join(root, "projects", "idea.md"))
	assert.FileExists(t, filepath.Join(root, "projects", "idea.assets", "a.png"), "the sidecar is renamed with the note")
	assert.FileExists(t, filepath.Join(root, "projects", "idea.assets", "b.png"))

	require.NoError(t, act.Undo(), "one undo puts both back")
	assert.FileExists(t, filepath.Join(root, "inbox", "plan.md"))
	assert.FileExists(t, filepath.Join(root, "inbox", "plan.assets", "a.png"))
	assert.NoDirExists(t, filepath.Join(root, "projects", "idea.assets"))

	require.NoError(t, act.Redo())
	assert.FileExists(t, filepath.Join(root, "projects", "idea.assets", "b.png"))
	assert.FileExists(t, filepath.Join(root, "projects", "idea.md"))
}

func TestMoveRefusesWhenTheSidecarNameIsTaken(t *testing.T) {
	root, _ := newRoot(t)
	bundle(t, root, "inbox", "plan")
	writeTree(t, root, "projects/idea.assets/other.png")
	s := NewMarkdown(root)

	_, err := s.Move("inbox/plan.md", "projects/idea.md")
	require.ErrorIs(t, err, ErrExists)
	assert.FileExists(t, filepath.Join(root, "inbox", "plan.md"), "nothing moved")
	assert.FileExists(t, filepath.Join(root, "inbox", "plan.assets", "a.png"))
	assert.NoFileExists(t, filepath.Join(root, "projects", "idea.md"))
	assert.FileExists(t, filepath.Join(root, "projects", "idea.assets", "other.png"), "the other folder is untouched")
}

func TestMoveLeavesUnpairedItemsAlone(t *testing.T) {
	root, _ := newRoot(t)
	writeTree(t, root, "inbox/plain.md", "inbox/ghost.assets/x.png", "inbox/pic.png")
	s := NewMarkdown(root)

	_, err := s.Move("inbox/plain.md", "projects/plain.md")
	require.NoError(t, err)
	_, err = s.Move("inbox/pic.png", "projects/pic.png")
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(root, "inbox", "ghost.assets", "x.png"), "a folder with no note is not a sidecar")
}

func TestMoveIgnoresASidecarThatIsNotAFolder(t *testing.T) {
	root, _ := newRoot(t)
	writeTree(t, root, "inbox/plan.md", "inbox/plan.assets")
	_, err := NewMarkdown(root).Move("inbox/plan.md", "projects/plan.md")
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(root, "inbox", "plan.assets"), "a file named like a sidecar stays")
}

func TestTrashCarriesTheSidecar(t *testing.T) {
	root, _ := newRoot(t)
	bundle(t, root, "inbox", "plan")
	s := NewMarkdown(root)

	act, err := s.Trash("inbox/plan.md")
	require.NoError(t, err)
	assert.NoFileExists(t, filepath.Join(root, "inbox", "plan.md"))
	assert.NoDirExists(t, filepath.Join(root, "inbox", "plan.assets"))

	require.NoError(t, act.Undo())
	assert.FileExists(t, filepath.Join(root, "inbox", "plan.md"))
	assert.FileExists(t, filepath.Join(root, "inbox", "plan.assets", "a.png"))

	require.NoError(t, act.Redo())
	assert.NoDirExists(t, filepath.Join(root, "inbox", "plan.assets"))
}

func TestCopyCarriesTheSidecar(t *testing.T) {
	root, _ := newRoot(t)
	bundle(t, root, "inbox", "plan")
	s := NewMarkdown(root)

	act, _, err := s.Copy("inbox/plan.md", "projects/plan copy.md")
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(root, "inbox", "plan.md"), "the source stays")
	assert.FileExists(t, filepath.Join(root, "inbox", "plan.assets", "a.png"))
	got, err := os.ReadFile(filepath.Join(root, "projects", "plan copy.assets", "b.png"))
	require.NoError(t, err)
	assert.Equal(t, []byte("x"), got)

	require.NoError(t, act.Undo())
	assert.NoFileExists(t, filepath.Join(root, "projects", "plan copy.md"))
	assert.NoDirExists(t, filepath.Join(root, "projects", "plan copy.assets"))
	assert.FileExists(t, filepath.Join(root, "inbox", "plan.assets", "a.png"), "undo leaves the source alone")
}

func TestCopyRefusesWhenTheSidecarNameIsTaken(t *testing.T) {
	root, _ := newRoot(t)
	bundle(t, root, "inbox", "plan")
	writeTree(t, root, "projects/copy.assets/other.png")

	_, _, err := NewMarkdown(root).Copy("inbox/plan.md", "projects/copy.md")
	require.ErrorIs(t, err, ErrExists)
	assert.NoFileExists(t, filepath.Join(root, "projects", "copy.md"), "no half copy")
}

func TestDeleteCarriesTheSidecar(t *testing.T) {
	root, _ := newRoot(t)
	bundle(t, root, "inbox", "plan")
	writeTree(t, root, "inbox/ghost.assets/x.png")
	s := NewMarkdown(root)

	require.NoError(t, s.Delete("inbox/plan.md"))
	assert.NoFileExists(t, filepath.Join(root, "inbox", "plan.md"))
	assert.NoDirExists(t, filepath.Join(root, "inbox", "plan.assets"))
	assert.DirExists(t, filepath.Join(root, "inbox", "ghost.assets"), "another folder is not touched")
}

func TestListHidesASidecarAndCountsItsImages(t *testing.T) {
	root, _ := newRoot(t)
	bundle(t, root, "inbox", "plan")
	writeTree(t, root, "inbox/plan.assets/.hidden.png", "inbox/plan.assets/notes.txt", "inbox/solo.md", "inbox/ghost.assets/x.png")

	got, err := NewMarkdown(root).List("inbox", ListOptions{IncludeImages: true})
	require.NoError(t, err)
	assert.Equal(t, []string{"ghost.assets", "plan.md", "solo.md"}, names(got),
		"the sidecar of an existing note is hidden; a folder with no note is listed")
	byName := map[string]Entry{}
	for _, e := range got {
		byName[e.Name] = e
	}
	assert.Equal(t, 3, byName["plan.md"].Attachments, "every visible file in the sidecar counts")
	assert.Equal(t, 0, byName["solo.md"].Attachments)
}

func TestListHidesTheSidecarWithoutImageOption(t *testing.T) {
	root, _ := newRoot(t)
	bundle(t, root, "inbox", "plan")
	got, err := NewMarkdown(root).List("inbox", ListOptions{})
	require.NoError(t, err)
	assert.Equal(t, []string{"plan.md"}, names(got), "code that lists notes never sees a sidecar")
}

func TestFolderItemCountSkipsSidecars(t *testing.T) {
	root, _ := newRoot(t)
	bundle(t, root, "areas/sub", "plan")
	got, err := NewMarkdown(root).List("areas", ListOptions{IncludeImages: true})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, 1, got[0].ItemCount, "only the note")
}

func TestInspectCountsSidecarFilesAsUnlisted(t *testing.T) {
	root, _ := newRoot(t)
	bundle(t, root, "areas/sub", "plan")
	di, err := NewMarkdown(root).Inspect("areas/sub")
	require.NoError(t, err)
	assert.Equal(t, 4, di.Items, "note, sidecar folder and its two images")
	assert.Equal(t, 3, di.Unlisted, "the sidecar and its images are not listed")
}

func TestRenameNoteWithSidecarKeepsTheCountWithTheNewName(t *testing.T) {
	root, _ := newRoot(t)
	bundle(t, root, "inbox", "plan")
	s := NewMarkdown(root)
	_, err := s.Move("inbox/plan.md", "inbox/idea.md")
	require.NoError(t, err)

	got, err := s.List("inbox", ListOptions{})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "idea.md", got[0].Name)
	assert.Equal(t, 2, got[0].Attachments)
}
