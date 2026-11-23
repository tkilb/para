// Package attach reads an image (or the text of an SVG) from the system
// clipboard or a file and stores it in the notebook: a PNG for a raster image,
// the SVG unchanged for an SVG. It never echoes clipboard bytes or paths in an
// error, because errors are shown in the status bar.
package attach

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

var (
	// ErrNoImage reports a clipboard that holds no image.
	ErrNoImage = errors.New("the clipboard has no image")
	// ErrNoText reports a clipboard that holds no text.
	ErrNoText = errors.New("the clipboard has no text")
	// ErrNoTool reports a missing clipboard program on Linux.
	ErrNoTool = errors.New("install wl-clipboard (wl-paste) or xclip to paste images")
	// ErrNoFile reports a clipboard that holds no copied file.
	ErrNoFile = errors.New("the clipboard has no file")
	// ErrClipboard reports a clipboard that could not be read.
	ErrClipboard = errors.New("cannot read the clipboard")
)

// maxRaw caps what is read from a clipboard program. It leaves room for the
// base64 form of an image of the largest size imgfmt accepts.
const maxRaw = 48 << 20

// Runner runs a program and returns its standard output.
type Runner func(ctx context.Context, name string, args ...string) ([]byte, error)

// Clipboard reads an image or text from the system clipboard. Its fields exist so
// tests can replace the system; NewClipboard fills them for real use.
type Clipboard struct {
	GOOS     string
	Getenv   func(string) string
	LookPath func(string) (string, error)
	Run      Runner
}

// NewClipboard returns a Clipboard that uses the real system.
func NewClipboard() Clipboard {
	return Clipboard{GOOS: runtime.GOOS, Getenv: os.Getenv, LookPath: exec.LookPath, Run: execRun}
}

// imageTypes are the clipboard types para reads as an image, best first. A
// raster type other than PNG is converted to PNG afterwards; an SVG is kept.
var imageTypes = []string{"image/png", "image/jpeg", "image/webp", "image/avif", "image/svg+xml"}

// textTypes are the clipboard types para reads as text, best first. The first
// two are what Wayland offers, the last two what X11 offers.
var textTypes = []string{"text/plain;charset=utf-8", "text/plain", "UTF8_STRING", "STRING"}

// macTextArgs make pbpaste print the plain text on the clipboard.
var macTextArgs = []string{"-Prefer", "txt"}

// Text returns the plain text on the clipboard. An SVG copied from an editor
// arrives this way, not as image data.
func (c Clipboard) Text(ctx context.Context) (string, error) {
	switch c.GOOS {
	case "darwin":
		out, err := c.Run(ctx, "pbpaste", macTextArgs...)
		if err != nil {
			return "", ErrClipboard
		}
		return string(out), nil
	case "linux":
		out, err := c.linuxRead(ctx, textTypes, ErrNoText)
		return string(out), err
	}
	return "", ErrClipboard
}

// Image returns the bytes of the image on the clipboard, in whatever supported
// format the clipboard offers. The caller converts a raster image to PNG and
// keeps an SVG as it is.
func (c Clipboard) Image(ctx context.Context) ([]byte, error) {
	switch c.GOOS {
	case "darwin":
		return c.macImage(ctx)
	case "linux":
		return c.linuxRead(ctx, imageTypes, ErrNoImage)
	}
	return nil, ErrClipboard
}

// macScript prints the clipboard image as base64 PNG, or nothing. It reads a
// PNG when there is one and converts a TIFF, which is what many apps put on
// the clipboard.
const macScript = `ObjC.import('AppKit');
var pb = $.NSPasteboard.generalPasteboard;
var data = pb.dataForType('public.png');
if (data.isNil()) {
  var tiff = pb.dataForType('public.tiff');
  if (!tiff.isNil()) {
    var rep = $.NSBitmapImageRep.imageRepWithData(tiff);
    data = rep.representationUsingTypeProperties($.NSBitmapImageFileTypePNG, $());
  }
}
data.isNil() ? '' : ObjC.unwrap(data.base64EncodedStringWithOptions(0));`

func (c Clipboard) macImage(ctx context.Context) ([]byte, error) {
	out, err := c.Run(ctx, "osascript", "-l", "JavaScript", "-e", macScript)
	if err != nil {
		return nil, ErrClipboard
	}
	text := strings.TrimSpace(string(out))
	if text == "" {
		return nil, ErrNoImage
	}
	data, err := base64.StdEncoding.DecodeString(text)
	if err != nil {
		return nil, ErrClipboard
	}
	return data, nil
}

// linuxTool describes how to list and read clipboard types with one program.
type linuxTool struct {
	name string
	list []string
	read func(mime string) []string
}

var (
	wlPaste = linuxTool{
		name: "wl-paste",
		list: []string{"--list-types"},
		read: func(mime string) []string { return []string{"--no-newline", "--type", mime} },
	}
	xclip = linuxTool{
		name: "xclip",
		list: []string{"-selection", "clipboard", "-t", "TARGETS", "-o"},
		read: func(mime string) []string { return []string{"-selection", "clipboard", "-t", mime, "-o"} },
	}
)

// linuxRead reads the best of types from the clipboard, and reports none when
// the clipboard offers none of them. It uses wl-paste in a Wayland session and
// xclip in an X11 one. A Wayland session falls back to xclip when wl-paste is
// missing and XWayland offers a display.
func (c Clipboard) linuxRead(ctx context.Context, types []string, none error) ([]byte, error) {
	var candidates []linuxTool
	if c.Getenv("WAYLAND_DISPLAY") != "" {
		candidates = append(candidates, wlPaste)
	}
	if c.Getenv("DISPLAY") != "" {
		candidates = append(candidates, xclip)
	}
	if len(candidates) == 0 {
		return nil, ErrClipboard
	}
	for _, tool := range candidates {
		if _, err := c.LookPath(tool.name); err == nil {
			return c.readWith(ctx, tool, types, none)
		}
	}
	return nil, ErrNoTool
}

func (c Clipboard) readWith(ctx context.Context, tool linuxTool, types []string, none error) ([]byte, error) {
	listing, err := c.Run(ctx, tool.name, tool.list...)
	if err != nil {
		return nil, ErrClipboard
	}
	mime := pickType(string(listing), types)
	if mime == "" {
		return nil, none
	}
	data, err := c.Run(ctx, tool.name, tool.read(mime)...)
	if err != nil {
		return nil, ErrClipboard
	}
	if len(data) == 0 {
		return nil, none
	}
	return data, nil
}

// pickType returns the best of types that a newline-separated list offers.
func pickType(listing string, types []string) string {
	offered := map[string]bool{}
	for _, line := range strings.Split(listing, "\n") {
		offered[strings.TrimSpace(line)] = true
	}
	for _, t := range types {
		if offered[t] {
			return t
		}
	}
	return ""
}

// errOutputTooLarge is reported when a program prints more than maxRaw bytes.
var errOutputTooLarge = errors.New("output too large")

// execRun runs a program and returns at most maxRaw bytes of its output.
func execRun(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	out, readErr := io.ReadAll(io.LimitReader(stdout, maxRaw+1))
	if len(out) > maxRaw {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, errOutputTooLarge
	}
	if err := cmd.Wait(); err != nil {
		return nil, err
	}
	return out, readErr
}

// fileTypes are the clipboard types that list copied files, on Linux.
var fileTypes = []string{"text/uri-list"}

// macFileScript prints the address of the file copied in Finder, or nothing.
// Finder may offer a file reference ("file:///.file/id=..."), which names no
// path; filePathURL turns it into the file's real address.
const macFileScript = `ObjC.import('AppKit');
var s = $.NSPasteboard.generalPasteboard.stringForType('public.file-url');
if (s.isNil()) { ''; } else {
  var u = $.NSURL.URLWithString(s);
  var p = u.isNil() ? $() : u.filePathURL;
  ObjC.unwrap(p.isNil() ? s : p.absoluteString);
}`

// File returns the path of the file copied on the clipboard (the first, when
// several are), or ErrNoFile when none is. Copying a file in a file manager
// puts its path there beside a thumbnail image, so a caller that wants the
// file asks for it before the image.
func (c Clipboard) File(ctx context.Context) (string, error) {
	switch c.GOOS {
	case "darwin":
		out, err := c.Run(ctx, "osascript", "-l", "JavaScript", "-e", macFileScript)
		if err != nil {
			return "", ErrClipboard
		}
		return fileURLPath(string(out))
	case "linux":
		out, err := c.linuxRead(ctx, fileTypes, ErrNoFile)
		if err != nil {
			return "", err
		}
		return fileURLPath(string(out))
	}
	return "", ErrClipboard
}

// fileURLPath returns the path in the first file:// address of list, which
// is one address or a text/uri-list. Only a local file is accepted: a host
// other than localhost, another scheme or a damaged address is ErrNoFile.
func fileURLPath(list string) (string, error) {
	for _, line := range strings.Split(list, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		u, err := url.Parse(line)
		if err != nil || u.Scheme != "file" || (u.Host != "" && u.Host != "localhost") || u.Path == "" {
			return "", ErrNoFile
		}
		return u.Path, nil
	}
	return "", ErrNoFile
}
