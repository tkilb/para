// Package pdfpreview renders the first page of a PDF as PNG bytes by running
// pdftoppm (poppler). The file is passed as one argument, never through a
// shell, and the run has a time limit and an output limit, since a PDF is
// untrusted input. Errors never show the path.
package pdfpreview

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

const (
	// tool is the program that draws the page.
	tool = "pdftoppm"
	// timeout bounds one render.
	timeout = 10 * time.Second
	// maxOutput bounds the PNG read back from the tool.
	maxOutput = 32 << 20
	// longestSide is the size in pixels of the longer side of the page drawn.
	longestSide = 1000
)

var (
	// ErrNoTool reports that pdftoppm is not installed.
	ErrNoTool = errors.New("install poppler (pdftoppm) to preview a PDF")
	// ErrUnreadable reports a PDF that could not be drawn.
	ErrUnreadable = errors.New("cannot read the PDF")
)

// FirstPage returns page 1 of the PDF at path as PNG bytes. path must be
// absolute so it can never be taken for an option.
func FirstPage(path string) ([]byte, error) {
	if !filepath.IsAbs(path) {
		return nil, ErrUnreadable
	}
	bin, err := exec.LookPath(tool)
	if err != nil {
		return nil, ErrNoTool
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args(path)...)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, ErrUnreadable
	}
	if err := cmd.Start(); err != nil {
		return nil, ErrUnreadable
	}
	data, readErr := io.ReadAll(io.LimitReader(out, maxOutput+1))
	if len(data) > maxOutput {
		cancel() // stop the tool; it is writing more than a page should hold
	}
	waitErr := cmd.Wait()
	if readErr != nil || waitErr != nil || len(data) == 0 || len(data) > maxOutput {
		return nil, ErrUnreadable
	}
	return data, nil
}

// args returns the arguments of one render of path. With no output prefix,
// pdftoppm writes the PNG to standard output; a "-" would name a file.
func args(path string) []string {
	return []string{"-png", "-f", "1", "-l", "1", "-scale-to", strconv.Itoa(longestSide), path}
}
