// Package browser opens a web address in the default browser by handing it
// to the system's opener. The address is passed as a single argument, never
// through a shell.
package browser

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/tkilb/para/internal/clip"
)

// Errors never echo the address.
var (
	ErrUnsupported = errors.New("opening a web address is not supported on this system")
	ErrOpen        = errors.New("could not start the browser (needs open on macOS or xdg-open on Linux)")
)

// Open opens raw, which must be an http or https address, in the default
// browser. It returns once the opener has started, not when it exits.
func Open(raw string) error {
	name, args, err := command(runtime.GOOS, raw)
	if err != nil {
		return err
	}
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		return ErrOpen
	}
	go func() { _ = cmd.Wait() }() // reap the opener
	return nil
}

// command returns the opener for goos and its arguments. Only web addresses
// that clip.ParseURL accepts get through, so an address can never be taken
// for an option or a local file.
func command(goos, raw string) (string, []string, error) {
	u, err := clip.ParseURL(raw)
	if err != nil {
		return "", nil, err
	}
	switch goos {
	case "darwin":
		return "open", []string{u.String()}, nil
	case "linux", "freebsd", "openbsd", "netbsd":
		return "xdg-open", []string{u.String()}, nil
	}
	return "", nil, ErrUnsupported
}

// ErrNotFile is returned when OpenFile gets something other than the absolute
// path of a regular file.
var ErrNotFile = errors.New("not an openable file")

// OpenFile opens the file at path in the system's preferred application. The
// path must be absolute and name a regular file, so it can never be taken for
// an option.
func OpenFile(path string) error {
	name, args, err := fileCommand(runtime.GOOS, path)
	if err != nil {
		return err
	}
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		return ErrOpen
	}
	go func() { _ = cmd.Wait() }() // reap the opener
	return nil
}

// fileCommand returns the opener for goos and its arguments.
func fileCommand(goos, path string) (string, []string, error) {
	if !filepath.IsAbs(path) {
		return "", nil, ErrNotFile
	}
	if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
		return "", nil, ErrNotFile
	}
	switch goos {
	case "darwin":
		return "open", []string{path}, nil
	case "linux", "freebsd", "openbsd", "netbsd":
		return "xdg-open", []string{path}, nil
	}
	return "", nil, ErrUnsupported
}
