package glossary

import (
	"strings"
)

const (
	termLabel       = "Term:"
	definitionLabel = "Definition:"
)

// Buffer renders e as the text edited in $EDITOR: a Term line, then the
// definition after Definition:, which may continue over any number of lines.
//
//	Term: NAM
//	Definition: National Account Manager,
//	the owner of a customer relationship.
func Buffer(e Entry) string {
	return termLabel + " " + e.Term + "\n" + definitionLabel + " " + e.Definition + "\n"
}

// ParseBuffer reads the text left by the editor. The term is the rest of the
// first Term: line. The definition is the rest of the first Definition: line
// after it and every line below, as written, so a line that starts with
// "Term:" in a definition stays in it. Text above Term: is ignored; a missing
// or blank Definition: gives a blank definition. A missing or blank term is
// ErrEmptyTerm.
func ParseBuffer(text string) (Entry, error) {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	var e Entry
	i := 0
	for ; i < len(lines); i++ {
		if rest, ok := strings.CutPrefix(lines[i], termLabel); ok {
			e.Term = strings.TrimSpace(rest)
			i++
			break
		}
	}
	if e.Term == "" {
		return Entry{}, ErrEmptyTerm
	}
	for ; i < len(lines); i++ {
		if rest, ok := strings.CutPrefix(lines[i], definitionLabel); ok {
			e.Definition = strings.TrimSpace(strings.Join(append([]string{rest}, lines[i+1:]...), "\n"))
			break
		}
	}
	return e, nil
}

// Spaced returns the definition for reading: when it has a line break after
// its first line, that break is doubled, so the first line stands apart from
// the rest. A break that is already a blank line is left alone.
func Spaced(definition string) string {
	first, rest, found := strings.Cut(definition, "\n")
	if !found || strings.HasPrefix(rest, "\n") {
		return definition
	}
	return first + "\n\n" + rest
}
