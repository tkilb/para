// Package findtext prepares the lines of the notes for a search inside note
// text. A line keeps the number it has in the Content panel, so a hit can open
// the note at the right place. Nothing is written and nothing leaves memory.
package findtext

import (
	"path/filepath"
	"strings"

	"github.com/tkilb/para/internal/store"
	"github.com/tkilb/para/internal/textsafe"
)

const (
	// maxLines bounds the lines kept, so a huge notebook cannot use all the
	// memory or make the search slow.
	maxLines = 100000
	// maxLineRunes cuts a long line; the rest is not searched.
	maxLineRunes = 160
)

// MaxLines is the number of lines Lines keeps at most.
const MaxLines = maxLines

// Line is one non-blank line of a note.
type Line struct {
	// Rel is the note's path relative to the notebook root.
	Rel string
	// Kind is the kind of the note, KindUnset when it has none.
	Kind store.Kind
	// Index is the line's position in the note as the Content panel shows it
	// (properties hidden, surrounding newlines trimmed), counting blank lines.
	Index int
	// Text is the line without its surrounding white space, cut to a
	// bounded length.
	Text string
}

// Result is what Lines found.
type Result struct {
	Lines []Line
	// Truncated is set when the limit was reached and notes were left out.
	Truncated bool
}

// InDir returns the paths in rels that are inside dir, at any depth. dir is a
// slash path relative to the notebook root; an empty or "." dir keeps all.
func InDir(rels []string, dir string) []string {
	dir = strings.Trim(filepath.ToSlash(filepath.Clean(dir)), "/")
	if dir == "" || dir == "." {
		return rels
	}
	var out []string
	for _, rel := range rels {
		if strings.HasPrefix(filepath.ToSlash(rel), dir+"/") {
			out = append(out, rel)
		}
	}
	return out
}

// Lines reads the notes at rels from s and returns their non-blank lines in
// order, up to maxLines. A note that cannot be read is skipped. Control
// characters are removed, as the Content panel does, so the text is safe to draw.
func Lines(s store.Store, rels []string) Result {
	var res Result
	for _, rel := range rels {
		n, err := s.ReadNote(rel)
		if err != nil {
			continue
		}
		text := textsafe.Sanitize(strings.Trim(n.Body, "\r\n"))
		for i, raw := range strings.Split(text, "\n") {
			line := strings.TrimSpace(raw)
			if line == "" {
				continue
			}
			if len(res.Lines) >= maxLines {
				res.Truncated = true
				return res
			}
			res.Lines = append(res.Lines, Line{Rel: rel, Kind: n.Kind, Index: i, Text: cut(line)})
		}
	}
	return res
}

// cut limits line to maxLineRunes characters.
func cut(line string) string {
	if len(line) <= maxLineRunes {
		return line
	}
	r := []rune(line)
	if len(r) <= maxLineRunes {
		return line
	}
	return string(r[:maxLineRunes])
}

// UnderPath reports whether the note at rel starts with the slash path
// prefix, ignoring case. A prefix may end partway through a folder name
// ("proj" holds "projects/a.md"); an empty prefix holds every note.
func UnderPath(rel, prefix string) bool {
	rel = strings.ToLower(filepath.ToSlash(rel))
	prefix = strings.ToLower(strings.TrimLeft(filepath.ToSlash(prefix), "/"))
	return strings.HasPrefix(rel, prefix)
}

// WithoutFile returns lines without those of notes whose file name is name,
// such as an import's _source.md. The input is left as it is.
func WithoutFile(lines []Line, name string) []Line {
	if name == "" {
		return lines
	}
	out := make([]Line, 0, len(lines))
	for _, l := range lines {
		if filepath.Base(l.Rel) != name {
			out = append(out, l)
		}
	}
	return out
}

// OfKind returns the lines of notes of kind k. The input is left as it is.
func OfKind(lines []Line, k store.Kind) []Line {
	var out []Line
	for _, l := range lines {
		if l.Kind == k {
			out = append(out, l)
		}
	}
	return out
}
