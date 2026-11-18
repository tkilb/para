// Package attach reads an image from the system clipboard and stores it as a
// PNG file in the notebook. It never echoes clipboard bytes or paths in an
// error, because errors are shown in the status bar.
package attach

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

var (
	// ErrNoImage reports a clipboard that holds no image.
	ErrNoImage = errors.New("the clipboard has no image")
	// ErrNoTool reports a missing clipboard program on Linux.
	ErrNoTool = errors.New("install wl-clipboard (wl-paste) or xclip to paste images")
	// ErrClipboard reports a clipboard that could not be read.
	ErrClipboard = errors.New("cannot read the clipboard")
)

// maxRaw caps what is read from a clipboard program. It leaves room for the
// base64 form of an image of the largest size imgfmt accepts.
const maxRaw = 48 << 20

// Runner runs a program and returns its standard output.
type Runner func(ctx context.Context, name string, args ...string) ([]byte, error)

// Clipboard reads an image from the system clipboard. Its fields exist so
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

// imageTypes are the clipboard types para reads, best first. A type other than
// PNG is converted to PNG afterwards.
var imageTypes = []string{"image/png", "image/jpeg", "image/webp", "image/avif"}

// Image returns the bytes of the image on the clipboard, in whatever supported
// format the clipboard offers. The caller converts them to PNG.
func (c Clipboard) Image(ctx context.Context) ([]byte, error) {
	switch c.GOOS {
	case "darwin":
		return c.macImage(ctx)
	case "linux":
		return c.linuxImage(ctx)
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

// linuxImage uses wl-paste in a Wayland session and xclip in an X11 one. A
// Wayland session falls back to xclip when wl-paste is missing and XWayland
// offers a display.
func (c Clipboard) linuxImage(ctx context.Context) ([]byte, error) {
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
			return c.readWith(ctx, tool)
		}
	}
	return nil, ErrNoTool
}

func (c Clipboard) readWith(ctx context.Context, tool linuxTool) ([]byte, error) {
	listing, err := c.Run(ctx, tool.name, tool.list...)
	if err != nil {
		return nil, ErrClipboard
	}
	mime := pickType(string(listing))
	if mime == "" {
		return nil, ErrNoImage
	}
	data, err := c.Run(ctx, tool.name, tool.read(mime)...)
	if err != nil {
		return nil, ErrClipboard
	}
	if len(data) == 0 {
		return nil, ErrNoImage
	}
	return data, nil
}

// pickType returns the best supported image type in a newline-separated list.
func pickType(listing string) string {
	offered := map[string]bool{}
	for _, line := range strings.Split(listing, "\n") {
		offered[strings.TrimSpace(line)] = true
	}
	for _, t := range imageTypes {
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
