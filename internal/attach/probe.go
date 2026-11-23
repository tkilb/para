package attach

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"syscall"

	"github.com/tkilb/para/internal/imgfmt"
	"github.com/tkilb/para/internal/loader"
)

const (
	clipboardSVG    = "SVG from the clipboard"
	fileSVG         = "SVG file"
	clipboardRaster = "image from the clipboard"
	fileRaster      = "image file"

	// sniffBytes is how much of a file's start is read to tell an SVG from a
	// raster image when the file name does not say.
	sniffBytes = 512

	unsafeSuffix  = ", not safe to store"
	savedAsFormat = "%s, %d bytes, saved as is"

	// probeMaxBytes caps how much of an SVG file the hint reads for the safety
	// check. A larger file is checked when it is imported.
	probeMaxBytes = 4 << 20
)

// ProbeClipboard describes the image, or the SVG text, on the clipboard for the
// import box, and reports false when there is neither. An SVG that cannot be
// saved is described with the reason, so the hint does not promise a save.
func ProbeClipboard(ctx context.Context, clip Source) (string, bool) {
	ctx, cancel := context.WithTimeout(ctx, pasteTimeout)
	defer cancel()
	if raw, err := clip.Image(ctx); err == nil {
		return describeImage(raw, clipboardSVG, clipboardRaster)
	}
	text, err := clip.Text(ctx)
	if err != nil {
		return "", false
	}
	data, _, handled, err := svgFromText(text)
	switch {
	case err != nil:
		return clipboardSVG + unsafeSuffix, true
	case handled:
		return fmt.Sprintf(savedAsFormat, clipboardSVG, len(data)), true
	}
	return "", false
}

// ProbeFile describes the image file that input names for the import box, and
// reports false when it is not a readable image file. A raster file is
// described from its header. A file that is, or may be, an SVG (by its name or
// its first bytes) is read up to probeMaxBytes, because the safety check needs
// all of it; a larger one is only sized and is checked on import.
func ProbeFile(input, inboxDir string) (string, bool) {
	in := loader.Classify(input)
	if in.Kind != loader.File {
		return "", false
	}
	path := loader.ResolveFile(in.Value, inboxDir)
	if loader.IsPDFPath(path) {
		return probePDF(loader.ExpandHome(path))
	}
	if !loader.IsImagePath(path) {
		return "", false
	}
	f, size, ok := openRegular(loader.ExpandHome(path))
	if !ok {
		return "", false
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, sniffBytes)
	head, _ := r.Peek(sniffBytes)
	if !startsLikeXML(head) {
		return describeRaster(r, fileRaster)
	}
	if size > imgfmt.MaxBytes {
		return fileSVG + ", too large", true
	}
	if size > probeMaxBytes {
		return deferredSVG(size), true
	}
	data, err := io.ReadAll(io.LimitReader(r, probeMaxBytes+1))
	switch {
	case err != nil:
		return "", false
	case len(data) > probeMaxBytes:
		return deferredSVG(int64(len(data))), true
	}
	return describeSVG(data, fileSVG)
}

// deferredSVG is the hint for an SVG file too big to check while typing.
func deferredSVG(size int64) string {
	return fmt.Sprintf("%s, %d bytes, checked on import", fileSVG, size)
}

// openRegular opens path for reading when it is a regular file, following a
// symbolic link, and returns its size. It checks before opening, so a pipe or
// device is never opened, and opens without blocking, so a file swapped for a
// pipe in between cannot hang the box; the opened file is checked again.
func openRegular(path string) (*os.File, int64, bool) {
	if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
		return nil, 0, false
	}
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, 0, false
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		f.Close()
		return nil, 0, false
	}
	return f, info.Size(), true
}

// startsLikeXML reports whether data, after an optional byte order mark and
// white space, opens a tag. No raster format starts that way.
func startsLikeXML(data []byte) bool {
	data = bytes.TrimLeft(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")), " \t\r\n")
	return len(data) > 0 && data[0] == '<'
}

// describeImage describes image bytes: markup as an SVG, anything else by its
// size in pixels.
func describeImage(data []byte, svgLabel, rasterLabel string) (string, bool) {
	if startsLikeXML(data) {
		return describeSVG(data, svgLabel)
	}
	return describeRaster(bytes.NewReader(data), rasterLabel)
}

func describeRaster(r io.Reader, label string) (string, bool) {
	cfg, _, err := image.DecodeConfig(r)
	if err != nil {
		return "", false
	}
	return fmt.Sprintf("%s, %d x %d px, saved as a PNG", label, cfg.Width, cfg.Height), true
}

// describeSVG runs the safety check on data. It says how big the SVG is when it
// would be saved, and why in a few words when it would not, never quoting the
// content. It reports false for markup that is not an SVG, so the usual hint
// shows.
func describeSVG(data []byte, label string) (string, bool) {
	_, _, err := imgfmt.Prepare(data)
	switch {
	case err == nil:
		return fmt.Sprintf(savedAsFormat, label, len(data)), true
	case errors.Is(err, imgfmt.ErrUnsafe):
		return label + unsafeSuffix, true
	case errors.Is(err, imgfmt.ErrNoNamespace):
		return label + ", no SVG namespace", true
	case errors.Is(err, imgfmt.ErrCorrupt):
		return label + ", damaged", true
	case errors.Is(err, imgfmt.ErrTooLarge):
		return label + ", too large", true
	}
	return "", false
}
