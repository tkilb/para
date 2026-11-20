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
		"a.svg": true, "A.SVG": true, ".svg": false,
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
		ext     string
		want    string
		wantErr error
	}{
		{"extension added", "diagram", ".png", "diagram.png", nil},
		{"extension kept", "diagram.png", ".png", "diagram.png", nil},
		{"upper case extension kept", "diagram.PNG", ".png", "diagram.PNG", nil},
		{"spaces trimmed", "  my pic  ", ".png", "my pic.png", nil},
		{"a note extension is not special", "idea.md", ".png", "idea.md.png", nil},
		{"svg extension added", "logo", ".svg", "logo.svg", nil},
		{"svg extension kept", "logo.svg", ".svg", "logo.svg", nil},
		{"svg upper case extension kept", "logo.SVG", ".svg", "logo.SVG", nil},
		{"an svg is not turned into a png", "logo.png", ".svg", "logo.png.svg", nil},
		{"a png is not turned into an svg", "logo.svg", ".png", "logo.svg.png", nil},
		{"upper case svg ext argument", "logo", ".SVG", "logo.svg", nil},
		{"upper case svg ext argument keeps name", "logo.svg", ".SVG", "logo.svg", nil},
		{"a name with dots", "a.b", ".svg", "a.b.svg", nil},
		{"an upper case svg is not turned into a png", "logo.SVG", ".png", "logo.SVG.png", nil},
		{"an unknown extension means png", "logo", "", "logo.png", nil},
		{"empty", "  ", ".png", "", ErrEmptyName},
		{"only the extension", ".png", ".png", "", ErrEmptyName},
		{"only the svg extension", ".svg", ".svg", "", ErrEmptyName},
		{"slash", "a/b", ".png", "", ErrBadName},
		{"trailing slash", "a/", ".png", "", ErrBadName},
		{"dot dot", "..", ".png", "", ErrBadName},
		{"nul byte", "a\x00b", ".png", "", ErrNulByte},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CleanRenamedImageName(tt.input, tt.ext)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestImageExtAndStem(t *testing.T) {
	tests := []struct{ name, ext, stem string }{
		{"a.png", ".png", "a"},
		{"A.PNG", ".png", "A"},
		{"a b.Svg", ".svg", "a b"},
		{"logo.svg", ".svg", "logo"},
		{"a.svg.png", ".png", "a.svg"},
		{"a.jpg", "", "a.jpg"},
		{"a.md", "", "a.md"},
		{".svg", "", ".svg"},
		{"svg", "", "svg"},
		{"", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.ext, ImageExt(tt.name))
			assert.Equal(t, tt.stem, ImageStem(tt.name))
		})
	}
}

func TestListIncludesSVGImages(t *testing.T) {
	root, _ := newRoot(t)
	writeTree(t, root, "areas/a.svg", "areas/B.SVG", "areas/c.png", "areas/.h.svg")

	got, err := NewMarkdown(root).List("areas", ListOptions{IncludeImages: true})
	require.NoError(t, err)
	assert.Equal(t, []string{"a.svg", "B.SVG", "c.png"}, names(got))
	for _, e := range got {
		assert.True(t, e.IsImage, e.Name)
	}
	plain, err := NewMarkdown(root).List("areas", ListOptions{})
	require.NoError(t, err)
	assert.Empty(t, plain)
}

func TestSVGSurvivesMoveCopyTrashAndUndo(t *testing.T) {
	root, _ := newRoot(t)
	writeTree(t, root, "inbox/logo.svg", "projects/")
	s := NewMarkdown(root)

	move, err := s.Move("inbox/logo.svg", "projects/logo.svg")
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(root, "projects", "logo.svg"))
	require.NoError(t, move.Undo())
	assert.FileExists(t, filepath.Join(root, "inbox", "logo.svg"))

	trash, err := s.Trash("inbox/logo.svg")
	require.NoError(t, err)
	assert.NoFileExists(t, filepath.Join(root, "inbox", "logo.svg"))
	require.NoError(t, trash.Undo())
	assert.FileExists(t, filepath.Join(root, "inbox", "logo.svg"))
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

func TestImageDisplayNames(t *testing.T) {
	tests := []struct {
		name  string
		names []string
		want  []string
	}{
		{"empty", []string{}, []string{}},
		{"no collision", []string{"a.png", "b.svg"}, []string{"a", "b"}},
		{"png and svg collide", []string{"foo.png", "foo.svg"}, []string{"foo", "foo.svg"}},
		{"collision in either order", []string{"foo.svg", "foo.png"}, []string{"foo.svg", "foo"}},
		{"case differences collide", []string{"Foo.PNG", "foo.svg"}, []string{"Foo", "foo.svg"}},
		{"upper case svg extension", []string{"foo.png", "foo.SVG"}, []string{"foo", "foo.svg"}},
		{"different stems do not collide", []string{"foo.png", "foo2.svg"}, []string{"foo", "foo2"}},
		{"svg does not collide with a note", []string{"foo.md", "foo.svg"}, []string{"foo.md", "foo"}},
		{"non-images pass through", []string{"x.md", "dir", "a.jpg"}, []string{"x.md", "dir", "a.jpg"}},
		{"svg in the stem of a png", []string{"a.svg.png", "a.svg"}, []string{"a.svg", "a"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ImageDisplayNames(tt.names))
		})
	}
}
