package attach

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tkilb/para/internal/imgfmt"
	"github.com/tkilb/para/internal/store"
)

// fakeSource is a clipboard that returns fixed bytes.
type fakeSource struct {
	data []byte
	err  error
}

func (f fakeSource) Image(context.Context) ([]byte, error) { return f.data, f.err }

func tinyPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	img.Set(1, 1, color.NRGBA{R: 255, A: 255})
	var b bytes.Buffer
	require.NoError(t, png.Encode(&b, img))
	return b.Bytes()
}

func tinyJPEG(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	require.NoError(t, jpeg.Encode(&b, image.NewNRGBA(image.Rect(0, 0, 8, 8)), nil))
	return b.Bytes()
}

var when = time.Date(2026, 11, 18, 9, 5, 7, 0, time.Local)

func newStore(t *testing.T) (string, store.Store) {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "projects", "alpha"), 0o755))
	return root, store.NewMarkdown(root)
}

// newStoreAt opens the notebook at root.
func newStoreAt(root string) store.Store { return store.NewMarkdown(root) }

func TestPasteSavesADatedPNG(t *testing.T) {
	root, s := newStore(t)
	data := tinyPNG(t)
	res, err := Paste(context.Background(), s, fakeSource{data: data}, "projects/alpha", when)
	require.NoError(t, err)
	assert.Equal(t, "projects/alpha/2026-11-18 09-05-07.png", res.Rel)
	got, err := os.ReadFile(filepath.Join(root, res.Rel))
	require.NoError(t, err)
	assert.Equal(t, data, got)
}

func TestPasteConvertsOtherFormats(t *testing.T) {
	root, s := newStore(t)
	res, err := Paste(context.Background(), s, fakeSource{data: tinyJPEG(t)}, "projects/alpha", when)
	require.NoError(t, err)
	got, err := os.ReadFile(filepath.Join(root, res.Rel))
	require.NoError(t, err)
	_, format, err := image.Decode(bytes.NewReader(got))
	require.NoError(t, err)
	assert.Equal(t, "png", format, "a JPEG on the clipboard is stored as PNG")
}

func TestPasteNeverOverwrites(t *testing.T) {
	root, s := newStore(t)
	first, err := Paste(context.Background(), s, fakeSource{data: tinyPNG(t)}, "projects/alpha", when)
	require.NoError(t, err)
	second, err := Paste(context.Background(), s, fakeSource{data: tinyPNG(t)}, "projects/alpha", when)
	require.NoError(t, err)
	third, err := Paste(context.Background(), s, fakeSource{data: tinyPNG(t)}, "projects/alpha", when)
	require.NoError(t, err)
	assert.Equal(t, "projects/alpha/2026-11-18 09-05-07.png", first.Rel)
	assert.Equal(t, "projects/alpha/2026-11-18 09-05-07 2.png", second.Rel)
	assert.Equal(t, "projects/alpha/2026-11-18 09-05-07 3.png", third.Rel)
	assert.FileExists(t, filepath.Join(root, first.Rel))
}

func TestPasteUndoAndRedo(t *testing.T) {
	root, s := newStore(t)
	res, err := Paste(context.Background(), s, fakeSource{data: tinyPNG(t)}, "projects/alpha", when)
	require.NoError(t, err)
	path := filepath.Join(root, res.Rel)

	require.NoError(t, res.Action.Undo())
	assert.NoFileExists(t, path)
	require.NoError(t, res.Action.Redo())
	assert.FileExists(t, path)
}

func TestPasteFailuresWriteNothing(t *testing.T) {
	tests := []struct {
		name string
		src  fakeSource
		want error
	}{
		{"no image", fakeSource{err: ErrNoImage}, ErrNoImage},
		{"clipboard unreadable", fakeSource{err: ErrClipboard}, ErrClipboard},
		{"not an image", fakeSource{data: []byte("plain text")}, imgfmt.ErrUnsupported},
		{"a damaged image", fakeSource{data: tinyPNG(t)[:20]}, imgfmt.ErrCorrupt},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, s := newStore(t)
			_, err := Paste(context.Background(), s, tt.src, "projects/alpha", when)
			require.True(t, errors.Is(err, tt.want), "got %v", err)
			entries, err := os.ReadDir(filepath.Join(root, "projects", "alpha"))
			require.NoError(t, err)
			assert.Empty(t, entries)
		})
	}
}

func TestPasteRefusesAFolderOutsideTheNotebook(t *testing.T) {
	_, s := newStore(t)
	_, err := Paste(context.Background(), s, fakeSource{data: tinyPNG(t)}, "../elsewhere", when)
	require.Error(t, err)
}
