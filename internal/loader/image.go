package loader

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/tkilb/para/internal/imgfmt"
)

// imageExts are the file extensions of the images para brings in. The bytes
// are checked later by imgfmt, never trusted from the name.
var imageExts = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".webp": true, ".avif": true}

// IsImagePath reports whether path names an image file para can bring in, by
// its extension.
func IsImagePath(path string) bool {
	return imageExts[strings.ToLower(filepath.Ext(path))]
}

// ImageFile is an image read from disk, still in its own format.
type ImageFile struct {
	// Name is the file name without its extension.
	Name string
	Data []byte
}

// ReadImage reads the regular file at path, which may be anywhere (a leading
// "~" is the home directory). It is no larger than imgfmt.MaxBytes. The file
// is never changed. Errors never show the path.
func ReadImage(path string) (ImageFile, error) {
	path = expandHome(path)
	if path == "" || strings.ContainsRune(path, 0) {
		return ImageFile{}, ErrNotFound
	}
	info, err := os.Stat(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return ImageFile{}, ErrNotFound
	case err != nil:
		return ImageFile{}, ErrUnreadable
	case !info.Mode().IsRegular():
		return ImageFile{}, ErrNotFile
	case info.Size() > imgfmt.MaxBytes:
		return ImageFile{}, ErrTooLarge
	}
	f, err := os.Open(path)
	if err != nil {
		return ImageFile{}, ErrUnreadable
	}
	defer f.Close()
	// Check the opened file, not only the path, in case it changed between.
	if fi, err := f.Stat(); err != nil || !fi.Mode().IsRegular() {
		return ImageFile{}, ErrNotFile
	}
	data, err := io.ReadAll(io.LimitReader(f, imgfmt.MaxBytes+1))
	switch {
	case err != nil:
		return ImageFile{}, ErrUnreadable
	case len(data) > imgfmt.MaxBytes:
		return ImageFile{}, ErrTooLarge
	}
	return ImageFile{Name: fileName(path), Data: data}, nil
}

// ExpandHome replaces a leading "~" or "~/" with the home directory.
func ExpandHome(p string) string { return expandHome(p) }
