package attach

import (
	"bytes"
	"context"
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

func TestSaveNeverOverwrites(t *testing.T) {
	root, s := newStore(t)
	first, err := save(s, "projects/alpha", "pic", tinyPNG(t))
	require.NoError(t, err)
	second, err := save(s, "projects/alpha", "pic", tinyPNG(t))
	require.NoError(t, err)
	assert.Equal(t, "projects/alpha/pic.png", first.Rel)
	assert.Equal(t, "projects/alpha/pic 2.png", second.Rel)
	assert.FileExists(t, filepath.Join(root, first.Rel))
}

func TestSaveUndoAndRedo(t *testing.T) {
	root, s := newStore(t)
	res, err := save(s, "projects/alpha", "pic", tinyPNG(t))
	require.NoError(t, err)
	path := filepath.Join(root, res.Rel)

	require.NoError(t, res.Action.Undo())
	assert.NoFileExists(t, path)
	require.NoError(t, res.Action.Redo())
	assert.FileExists(t, path)
}

func TestSaveRefusesAFolderOutsideTheNotebook(t *testing.T) {
	_, s := newStore(t)
	_, err := save(s, "../elsewhere", "pic", tinyPNG(t))
	require.Error(t, err)
}
