package attach

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/tkilb/para/internal/imgfmt"
	"github.com/tkilb/para/internal/loader"
	"github.com/tkilb/para/internal/store"
	"github.com/tkilb/para/internal/textsafe"
)

// Import handles what was typed into the import box when it is an image. A
// blank input is the clipboard: when it holds an image, or text that is an SVG
// document, it is saved. Anything else is a file path, and an image file (PNG,
// JPEG, WebP, AVIF or SVG) is saved: a raster image converted to PNG, an SVG
// unchanged once it passes the safety check. The kind is decided from the
// content, not the file name. The image goes into dir (relative to the
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
		return importClipboardText(ctx, s, clip, dir, now)
	}
	data, ext, err := imgfmt.Prepare(raw)
	if err != nil {
		return Result{}, true, err
	}
	res, err := save(s, dir, now.Format(nameLayout), ext, data)
	return res, true, err
}

// importClipboardText saves the clipboard text when it is an SVG document. It
// does not handle any other text, so the text import takes over.
func importClipboardText(ctx context.Context, s store.Store, clip Source, dir string, now time.Time) (Result, bool, error) {
	text, err := clip.Text(ctx)
	if err != nil {
		return Result{}, false, nil
	}
	data, ext, handled, err := svgFromText(text)
	if !handled || err != nil {
		return Result{}, handled, err
	}
	res, err := save(s, dir, now.Format(nameLayout), ext, data)
	return res, true, err
}

// svgFromText applies to clipboard text the same gate imgfmt.Prepare applies to
// bytes: text that opens a tag, after a byte order mark and white space, is
// checked. A valid SVG is handled and returned with its extension; an unsafe
// one is handled with imgfmt.ErrUnsafe. Every other result, including a
// damaged, too large or unsupported document, is not handled, so the text
// import takes over.
func svgFromText(text string) (data []byte, ext string, handled bool, err error) {
	if !textOpensTag(text) {
		return nil, "", false, nil
	}
	data, ext, err = imgfmt.Prepare([]byte(text))
	switch {
	case err == nil:
		return data, ext, true, nil
	case errors.Is(err, imgfmt.ErrUnsafe):
		return nil, "", true, err
	}
	return nil, "", false, nil
}

// textOpensTag reports whether text, after an optional byte order mark and white
// space, starts with "<".
func textOpensTag(text string) bool {
	text = strings.TrimLeft(strings.TrimPrefix(text, "\ufeff"), " \t\r\n")
	return strings.HasPrefix(text, "<")
}

func importFile(s store.Store, dir, path string, now time.Time) (Result, error) {
	img, err := loader.ReadImage(path)
	if err != nil {
		return Result{}, err
	}
	data, ext, err := imgfmt.Prepare(img.Data)
	if err != nil {
		return Result{}, err
	}
	return save(s, dir, fileStem(img.Name, ext, now), ext, data)
}

// fileStem is the name for a saved image taken from a file name: the name
// cleaned of control characters, or the date when nothing usable is left. ext
// is the extension the image is saved with.
func fileStem(name, ext string, now time.Time) string {
	clean, err := store.CleanRenamedImageName(textsafe.SingleLine(name), ext)
	if err != nil {
		return now.Format(nameLayout)
	}
	return strings.TrimSuffix(clean, filepath.Ext(clean))
}
