package loader

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsImagePath(t *testing.T) {
	tests := map[string]bool{
		"a.png": true, "a.PNG": true, "dir/a.jpg": true, "a.jpeg": true, "a.JPEG": true,
		"a.webp": true, "a.avif": true,
		"a.gif": false, "a.md": false, "a.txt": false, "png": false, "": false, "a.png.md": false,
	}
	for path, want := range tests {
		assert.Equal(t, want, IsImagePath(path), path)
	}
}

func TestReadImage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "My Diagram.JPG")
	require.NoError(t, os.WriteFile(path, []byte("jpegbytes"), 0o644))

	got, err := ReadImage(path)
	require.NoError(t, err)
	assert.Equal(t, "My Diagram", got.Name, "the name has no extension")
	assert.Equal(t, []byte("jpegbytes"), got.Data, "the bytes are returned as they are")
}

func TestReadImageExpandsHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	require.NoError(t, os.WriteFile(filepath.Join(home, "pic.png"), []byte("x"), 0o644))
	got, err := ReadImage("~/pic.png")
	require.NoError(t, err)
	assert.Equal(t, "pic", got.Name)
}

func TestReadImageFailures(t *testing.T) {
	dir := t.TempDir()
	big := filepath.Join(dir, "big.png")
	f, err := os.Create(big)
	require.NoError(t, err)
	require.NoError(t, f.Truncate(32<<20+1))
	require.NoError(t, f.Close())
	require.NoError(t, os.Mkdir(filepath.Join(dir, "folder.png"), 0o755))

	tests := []struct {
		name string
		path string
		want error
	}{
		{"missing", filepath.Join(dir, "missing.png"), ErrNotFound},
		{"empty path", "", ErrNotFound},
		{"nul byte", "a\x00.png", ErrNotFound},
		{"a folder", filepath.Join(dir, "folder.png"), ErrNotFile},
		{"too large", big, ErrTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ReadImage(tt.path)
			require.ErrorIs(t, err, tt.want)
			assert.False(t, strings.Contains(err.Error(), dir), "errors never show a path")
		})
	}
}
