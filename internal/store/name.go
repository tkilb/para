package store

import (
	"errors"
	"strings"
)

// ErrBadName reports a typed name that cannot be a single file or folder
// name.
var ErrBadName = errors.New("invalid name")

// CleanNewName validates a name typed into the create prompt. A trailing
// "/" makes it a folder; any other "/" is rejected, as are empty names,
// "." and "..", and NUL bytes. A note name gets ".md" unless it already
// ends with it. The returned name never has a trailing slash.
func CleanNewName(input string) (name string, isDir bool, err error) {
	name = strings.TrimSpace(input)
	if strings.ContainsRune(name, 0) {
		return "", false, ErrNulByte
	}
	if trimmed, ok := strings.CutSuffix(name, "/"); ok {
		name, isDir = strings.TrimSpace(trimmed), true
	}
	switch {
	case name == "":
		return "", false, ErrEmptyName
	case name == "." || name == "..", strings.Contains(name, "/"):
		return "", false, ErrBadName
	}
	if !isDir && !isNote(name) {
		name += noteExt
	}
	return name, isDir, nil
}

// CleanRenamedName validates a name typed into the rename prompt for an item
// that is a folder when isDir is set. It applies the rules of CleanNewName,
// except that a folder's name needs no trailing "/" (one is allowed) and a
// note's name may not have one.
func CleanRenamedName(input string, isDir bool) (string, error) {
	text := strings.TrimSpace(input)
	if isDir {
		text = strings.TrimSuffix(text, "/") + "/"
	}
	name, gotDir, err := CleanNewName(text)
	switch {
	case err != nil:
		return "", err
	case gotDir != isDir:
		return "", ErrBadName
	}
	return name, nil
}

// CleanRenamedImageName validates a name typed into the rename prompt for a
// PNG image. The name gets ".png" unless it already ends with it, so renaming
// never turns an image into a note. It rejects empty names, "." and "..",
// any "/" and NUL bytes, and a name that is only the extension.
func CleanRenamedImageName(input string) (string, error) {
	name := strings.TrimSpace(input)
	switch {
	case strings.ContainsRune(name, 0):
		return "", ErrNulByte
	case name == "", strings.EqualFold(name, imageExt):
		return "", ErrEmptyName
	case name == "." || name == "..", strings.Contains(name, "/"):
		return "", ErrBadName
	}
	if !isImage(name) {
		name += imageExt
	}
	return name, nil
}
