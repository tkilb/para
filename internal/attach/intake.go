package attach

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tkilb/para/internal/imgfmt"
	"github.com/tkilb/para/internal/loader"
	"github.com/tkilb/para/internal/store"
	"github.com/tkilb/para/internal/textsafe"
)

// Import handles what was typed into the import box when it is an image. A
// blank input is the clipboard: when it holds an image, the image is saved.
// Anything else is a file path, and an image file (PNG, JPEG, WebP or AVIF) is
// converted to PNG and saved. The image goes into dir (relative to the
// notebook root), named for the file, or for the moment when it came from the
// clipboard.
//
// handled reports whether the input was an image, so the caller must not run
// the text import: an empty clipboard, a text file, a folder and a web address
// are not handled and write nothing. A handled input can still fail, with a
// message that shows no path.
func Import(ctx context.Context, s store.Store, clip Source, dir, input, inboxDir string, now time.Time) (res Result, handled bool, err error) {
	in := loader.Classify(input)
	switch in.Kind {
	case loader.Clipboard:
		return importClipboard(ctx, s, clip, dir, now)
	case loader.File:
		path := loader.ResolveFile(in.Value, inboxDir)
		if !loader.IsImagePath(path) {
			return Result{}, false, nil
		}
		res, err = importFile(s, dir, path, now)
		return res, true, err
	}
	return Result{}, false, nil
}

func importClipboard(ctx context.Context, s store.Store, clip Source, dir string, now time.Time) (Result, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, pasteTimeout)
	defer cancel()
	raw, err := clip.Image(ctx)
	if err != nil {
		return Result{}, false, nil // no image there: the text import takes over
	}
	data, err := imgfmt.ToPNG(raw)
	if err != nil {
		return Result{}, true, err
	}
	res, err := save(s, dir, now.Format(nameLayout), store.PNGExt, data)
	return res, true, err
}

func importFile(s store.Store, dir, path string, now time.Time) (Result, error) {
	img, err := loader.ReadImage(path)
	if err != nil {
		return Result{}, err
	}
	data, err := imgfmt.ToPNG(img.Data)
	if err != nil {
		return Result{}, err
	}
	return save(s, dir, fileStem(img.Name, now), store.PNGExt, data)
}

// fileStem is the name for a saved image taken from a file name: the name
// cleaned of control characters, or the date when nothing usable is left.
func fileStem(name string, now time.Time) string {
	clean, err := store.CleanRenamedImageName(textsafe.SingleLine(name), store.PNGExt)
	if err != nil {
		return now.Format(nameLayout)
	}
	return strings.TrimSuffix(clean, filepath.Ext(clean))
}

// ProbeClipboard describes the image on the clipboard for the import box, and
// reports false when there is none.
func ProbeClipboard(ctx context.Context, clip Source) (string, bool) {
	ctx, cancel := context.WithTimeout(ctx, pasteTimeout)
	defer cancel()
	raw, err := clip.Image(ctx)
	if err != nil {
		return "", false
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return "", false
	}
	return fmt.Sprintf("image from the clipboard, %d x %d px, saved as a PNG", cfg.Width, cfg.Height), true
}

// ProbeFile describes the image file that input names for the import box, and
// reports false when it is not a readable image file. It reads only the
// file's header.
func ProbeFile(input, inboxDir string) (string, bool) {
	in := loader.Classify(input)
	if in.Kind != loader.File {
		return "", false
	}
	path := loader.ResolveFile(in.Value, inboxDir)
	if !loader.IsImagePath(path) {
		return "", false
	}
	f, err := os.Open(loader.ExpandHome(path))
	if err != nil {
		return "", false
	}
	defer f.Close()
	if info, err := f.Stat(); err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return "", false
	}
	return fmt.Sprintf("image file, %d x %d px, saved as a PNG", cfg.Width, cfg.Height), true
}
