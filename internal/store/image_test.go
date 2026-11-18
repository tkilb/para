package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListLeavesImagesOutByDefault(t *testing.T) {
	root, _ := newRoot(t)
	writeTree(t, root, "areas/note.md", "areas/pic.png")

	got, err := NewMarkdown(root).List("areas", ListOptions{})
	require.NoError(t, err)
	assert.Equal(t, []string{"note.md"}, names(got), "only the panel asks for images")
}

func TestListIncludesImagesWhenAsked(t *testing.T) {
	root, _ := newRoot(t)
	writeTree(t, root, "areas/b.md", "areas/a.png", "areas/C.PNG", "areas/doc.pdf", "areas/anim.gif",
		"areas/.hidden.png", "areas/sub/")

	got, err := NewMarkdown(root).List("areas", ListOptions{IncludeImages: true})
	require.NoError(t, err)
	assert.Equal(t, []string{"sub", "a.png", "b.md", "C.PNG"}, names(got),
		"PNG files sort with the notes; other file types and hidden images stay out")

	byName := map[string]Entry{}
	for _, e := range got {
		byName[e.Name] = e
	}
	assert.True(t, byName["a.png"].IsImage)
	assert.True(t, byName["C.PNG"].IsImage, "the extension is matched in any case")
	assert.False(t, byName["b.md"].IsImage)
	assert.False(t, byName["sub"].IsImage)
}

func TestListShowsHiddenImagesWhenAsked(t *testing.T) {
	root, _ := newRoot(t)
	writeTree(t, root, "areas/.hidden.png")

	got, err := NewMarkdown(root).List("areas", ListOptions{IncludeImages: true, ShowHidden: true})
	require.NoError(t, err)
	assert.Equal(t, []string{".hidden.png"}, names(got))
}

func TestItemCountFollowsTheImageOption(t *testing.T) {
	root, _ := newRoot(t)
	writeTree(t, root, "areas/sub/one.md", "areas/sub/two.png")

	s := NewMarkdown(root)
	plain, err := s.List("areas", ListOptions{})
	require.NoError(t, err)
	withImages, err := s.List("areas", ListOptions{IncludeImages: true})
	require.NoError(t, err)
	assert.Equal(t, 1, plain[0].ItemCount)
	assert.Equal(t, 2, withImages[0].ItemCount)
}

func TestInspectCountsAnImageAsListed(t *testing.T) {
	root, _ := newRoot(t)
	writeTree(t, root, "areas/sub/one.md", "areas/sub/two.png", "areas/sub/three.pdf")

	di, err := NewMarkdown(root).Inspect("areas/sub")
	require.NoError(t, err)
	assert.Equal(t, 3, di.Items)
	assert.Equal(t, 1, di.Unlisted, "only the PDF is unlisted")
}

func TestCountNotesIgnoresImages(t *testing.T) {
	root, _ := newRoot(t)
	writeTree(t, root, "areas/one.md")
	before := CountNotes(root)
	writeTree(t, root, "areas/two.png")
	assert.Equal(t, before, CountNotes(root), "an image is not a note")
}

func TestIsImage(t *testing.T) {
	tests := map[string]bool{
		"a.png": true, "A.PNG": true, "a b.Png": true,
		"a.jpg": false, "a.md": false, "png": false, "": false, "a.png.md": false,
	}
	for name, want := range tests {
		assert.Equal(t, want, IsImage(name), name)
	}
}

func TestCleanRenamedImageName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr error
	}{
		{"extension added", "diagram", "diagram.png", nil},
		{"extension kept", "diagram.png", "diagram.png", nil},
		{"upper case extension kept", "diagram.PNG", "diagram.PNG", nil},
		{"spaces trimmed", "  my pic  ", "my pic.png", nil},
		{"a note extension is not special", "idea.md", "idea.md.png", nil},
		{"empty", "  ", "", ErrEmptyName},
		{"only the extension", ".png", "", ErrEmptyName},
		{"slash", "a/b", "", ErrBadName},
		{"trailing slash", "a/", "", ErrBadName},
		{"dot dot", "..", "", ErrBadName},
		{"nul byte", "a\x00b", "", ErrNulByte},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CleanRenamedImageName(tt.input)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestImageSurvivesMoveCopyTrashAndUndo(t *testing.T) {
	root, _ := newRoot(t)
	writeTree(t, root, "inbox/pic.png", "projects/")
	s := NewMarkdown(root)
	want := []byte("x")

	move, err := s.Move("inbox/pic.png", "projects/pic.png")
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(root, "projects", "pic.png"))
	require.NoError(t, move.Undo())
	assert.FileExists(t, filepath.Join(root, "inbox", "pic.png"))

	_, _, err = s.Copy("inbox/pic.png", "projects/copy.png")
	require.NoError(t, err)
	got, err := os.ReadFile(filepath.Join(root, "projects", "copy.png"))
	require.NoError(t, err)
	assert.Equal(t, want, got, "a copied image is byte for byte the same")

	trash, err := s.Trash("inbox/pic.png")
	require.NoError(t, err)
	assert.NoFileExists(t, filepath.Join(root, "inbox", "pic.png"))
	require.NoError(t, trash.Undo())
	assert.FileExists(t, filepath.Join(root, "inbox", "pic.png"))
}
