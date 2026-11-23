// Package loader reads the thing being imported, a file, the clipboard or a
// web page, into one Source of plain text for the agent. What it reads is
// untrusted: it is only ever read as text, never run, and errors never show a
// path, an address or any of the text.
package loader

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/tkilb/para/internal/store"
)

// MaxBytes is the most text one import reads: the same limit as a note.
const MaxBytes = store.MaxNoteBytes

// Kind says where a Source came from.
type Kind int

const (
	// File is a regular file on disk.
	File Kind = iota
	// Clipboard is the text on the system clipboard.
	Clipboard
	// Web is a web page fetched by address.
	Web
)

// Source is the text to import and where it came from.
type Source struct {
	Kind Kind
	// Name names the source for a folder when the agent gives no topic: the
	// file name without its extension, the page title or host, or
	// "clipboard". It is raw; the caller cleans it.
	Name string
	// Origin is the address of a web page as the user gave it; empty for a
	// file or the clipboard.
	Origin string
	Text   string
	// Truncated reports a web page cut at the size limit.
	Truncated bool
}

// Errors never echo a path, an address or text.
var (
	ErrNotFound        = errors.New("file not found")
	ErrNotFile         = errors.New("not a regular file")
	ErrTooLarge        = errors.New("the source is too large")
	ErrUnreadable      = errors.New("cannot read the file")
	ErrNotText         = errors.New("the source is not UTF-8 text")
	ErrNothingToImport = errors.New("there is no text to import")
	ErrClipboard       = errors.New("cannot read the clipboard")
	ErrNoReadableText  = errors.New("the page has no readable text; it may need JavaScript or a login, so copy its text and paste it, or save it as a file")
	ErrLoginRequired   = errors.New("the page needs a login; copy its text and paste it, or save it as a file")
)

// checkText validates data as importable text and returns it without a
// leading byte order mark: not over MaxBytes, valid UTF-8 with no NUL, and
// not only white space.
func checkText(data []byte) (string, error) {
	switch {
	case len(data) > MaxBytes:
		return "", ErrTooLarge
	case !utf8.Valid(data) || containsNUL(data):
		return "", ErrNotText
	}
	text := strings.TrimPrefix(string(data), "\xef\xbb\xbf")
	if strings.TrimSpace(text) == "" {
		return "", ErrNothingToImport
	}
	return text, nil
}

func containsNUL(data []byte) bool {
	for _, b := range data {
		if b == 0 {
			return true
		}
	}
	return false
}

// Input is what the user typed into the import box, sorted by kind.
type Input struct {
	Kind Kind
	// Value is the trimmed input; empty for the clipboard.
	Value string
}

// Classify sorts what the user typed: nothing (or white space) is the
// clipboard, a value starting with http:// or https:// is a web page, and
// anything else is a file path. It only sorts; ReadURL and ReadFile check.
func Classify(input string) Input {
	v := strings.TrimSpace(input)
	lower := strings.ToLower(v)
	switch {
	case v == "":
		return Input{Kind: Clipboard}
	case strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://"):
		return Input{Kind: Web, Value: v}
	}
	return Input{Kind: File, Value: unquotePath(v)}
}

// unquotePath undoes the quoting a terminal or file manager adds to a path
// with spaces: a pair of matching quotes around it, or a backslash before a
// space or other punctuation mark. A backslash before a letter or digit is
// part of the name, so it stays.
func unquotePath(v string) string {
	if n := len(v); n >= 2 && (v[0] == '\'' || v[0] == '"') && v[n-1] == v[0] {
		return v[1 : n-1]
	}
	if !strings.Contains(v, `\`) {
		return v
	}
	var b strings.Builder
	rs := []rune(v)
	for i := 0; i < len(rs); i++ {
		if rs[i] == '\\' && i+1 < len(rs) && !unicode.IsLetter(rs[i+1]) && !unicode.IsDigit(rs[i+1]) {
			i++
		}
		b.WriteRune(rs[i])
	}
	return b.String()
}
