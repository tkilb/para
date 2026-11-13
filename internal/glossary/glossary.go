// Package glossary reads and edits the notebook's glossary, the file
// glossary.yaml in the notebook root: one YAML mapping of term to definition,
// in the order written. A term with several meanings (a three letter acronym
// may have many) holds a list of definitions instead of one. Edits go through the YAML node tree, so comments and
// the order of the other terms survive, and the file is replaced atomically.
package glossary

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// FileName is the glossary file in the notebook root.
const FileName = "glossary.yaml"

// maxBytes bounds the glossary file, as a note is bounded.
const maxBytes = 1 << 20

var (
	// ErrFormat reports a glossary file that is not a mapping of term to definition text, or to a list of them.
	ErrFormat = errors.New("glossary.yaml must be a mapping of term to definition")
	// ErrExists reports a term the glossary already has.
	ErrExists = errors.New("that term is already in the glossary")
	// ErrNotFound reports a term the glossary does not have.
	ErrNotFound = errors.New("that term is not in the glossary")
	// ErrEmptyTerm reports a term with no text.
	ErrEmptyTerm = errors.New("the term is empty")
	// ErrEmptyDefinition reports a definition with no text.
	ErrEmptyDefinition = errors.New("the definition is empty")
	// ErrTooLarge reports a glossary file over the size limit.
	ErrTooLarge = errors.New("glossary.yaml is too large")
)

// Entry is one term and its definition.
type Entry struct {
	Term, Definition string
}

// Load returns the glossary of the notebook at root, in file order. A term
// with several meanings gives one entry per meaning, in the order written. A
// missing or empty file is an empty glossary.
func Load(root string) ([]Entry, error) {
	doc, err := read(root)
	if err != nil {
		return nil, err
	}
	m, err := mapping(doc)
	if err != nil {
		return nil, err
	}
	var out []Entry
	for i := 0; i+1 < len(m.Content); i += 2 {
		for _, d := range meanings(m.Content[i+1]) {
			out = append(out, Entry{Term: m.Content[i].Value, Definition: d.Value})
		}
	}
	return out, nil
}

// Add records a meaning of a term. A term that exists gets another meaning
// (its value becomes a list); the same term with the same definition is
// refused. The match is exact.
func Add(root, term, definition string) error {
	e, err := clean(Entry{term, definition})
	if err != nil {
		return err
	}
	return update(root, func(m *yaml.Node) error { return insert(m, e) })
}

// Replace changes the entry old to the entry new. When the term is unchanged
// the meaning is edited in place; a new term moves the meaning there, under
// an existing term if there is one. An entry that exists already is refused.
func Replace(root string, old, new Entry) error {
	new, err := clean(new)
	if err != nil {
		return err
	}
	return update(root, func(m *yaml.Node) error {
		at, n := locate(m, old)
		if n == nil {
			return ErrNotFound
		}
		if old.Term == new.Term {
			if new.Definition != old.Definition && has(m, new) {
				return ErrExists
			}
			n.Value, n.Style = new.Definition, style(new.Definition)
			return nil
		}
		if has(m, new) {
			return ErrExists
		}
		if m.Content[at+1] == n && keyAt(m, new.Term) < 0 {
			// The only meaning of a term moves to a free name: rename it in place.
			m.Content[at].Value = new.Term
			n.Value, n.Style = new.Definition, style(new.Definition)
			return nil
		}
		remove(m, at, n)
		return insert(m, new)
	})
}

// Delete removes one meaning of a term; the last meaning removes the term.
func Delete(root string, e Entry) error {
	return update(root, func(m *yaml.Node) error {
		at, n := locate(m, e)
		if n == nil {
			return ErrNotFound
		}
		remove(m, at, n)
		return nil
	})
}

// clean trims both texts and rejects empty ones.
func clean(e Entry) (Entry, error) {
	e.Term, e.Definition = strings.TrimSpace(e.Term), strings.TrimSpace(e.Definition)
	switch {
	case e.Term == "":
		return e, ErrEmptyTerm
	case e.Definition == "":
		return e, ErrEmptyDefinition
	}
	return e, nil
}

func style(s string) yaml.Style {
	if strings.Contains(s, "\n") {
		return yaml.LiteralStyle
	}
	return 0
}

func scalar(s string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s, Style: style(s)}
}

// keyAt returns the index of term's key in the mapping m, or -1.
func keyAt(m *yaml.Node, term string) int {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == term {
			return i
		}
	}
	return -1
}

// meanings is the definitions held by a value node: itself when it is text,
// its items when it is a list.
func meanings(v *yaml.Node) []*yaml.Node {
	if v.Kind == yaml.SequenceNode {
		return v.Content
	}
	return []*yaml.Node{v}
}

// locate finds the definition node of e and the index of its term's key.
func locate(m *yaml.Node, e Entry) (int, *yaml.Node) {
	at := keyAt(m, e.Term)
	if at < 0 {
		return -1, nil
	}
	for _, d := range meanings(m.Content[at+1]) {
		if d.Value == e.Definition {
			return at, d
		}
	}
	return -1, nil
}

func has(m *yaml.Node, e Entry) bool {
	_, n := locate(m, e)
	return n != nil
}

// insert adds e as a new meaning: a new key, or the term's value made (or
// kept) a list. The caller has checked that e is not there already.
func insert(m *yaml.Node, e Entry) error {
	at := keyAt(m, e.Term)
	switch {
	case at < 0:
		m.Content = append(m.Content, scalar(e.Term), scalar(e.Definition))
	case has(m, e):
		return ErrExists
	case m.Content[at+1].Kind == yaml.SequenceNode:
		seq := m.Content[at+1]
		seq.Content = append(seq.Content, scalar(e.Definition))
	default:
		m.Content[at+1] = &yaml.Node{
			Kind: yaml.SequenceNode, Tag: "!!seq",
			Content: []*yaml.Node{m.Content[at+1], scalar(e.Definition)},
		}
	}
	return nil
}

// remove takes the meaning node n out of the term whose key is at. A list left
// with one meaning goes back to plain text; one left with none removes the
// term.
func remove(m *yaml.Node, at int, n *yaml.Node) {
	v := m.Content[at+1]
	if v.Kind == yaml.SequenceNode {
		for i, d := range v.Content {
			if d == n {
				v.Content = append(v.Content[:i], v.Content[i+1:]...)
				break
			}
		}
		switch len(v.Content) {
		case 0:
		case 1:
			m.Content[at+1] = v.Content[0]
			return
		default:
			return
		}
	}
	m.Content = append(m.Content[:at], m.Content[at+2:]...)
}

// read parses the file into a document node. A missing or empty file gives a
// document holding an empty mapping.
func read(root string) (*yaml.Node, error) {
	path := filepath.Join(root, FileName)
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return emptyDoc(), nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: not a regular file", ErrFormat)
	}
	if info.Size() > maxBytes {
		return nil, ErrTooLarge
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%w (%s)", ErrFormat, err)
	}
	if doc.Kind == 0 {
		return emptyDoc(), nil
	}
	return &doc, nil
}

func emptyDoc() *yaml.Node {
	return &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}}
}

// mapping returns the top-level mapping of doc, or ErrFormat.
func mapping(doc *yaml.Node) (*yaml.Node, error) {
	if len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, ErrFormat
	}
	m := doc.Content[0]
	for i, n := range m.Content {
		switch {
		case n.Kind == yaml.ScalarNode:
		case i%2 == 1 && n.Kind == yaml.SequenceNode && allScalars(n):
		default:
			return nil, ErrFormat
		}
	}
	return m, nil
}

func allScalars(seq *yaml.Node) bool {
	for _, n := range seq.Content {
		if n.Kind != yaml.ScalarNode {
			return false
		}
	}
	return true
}

// update reads the file, applies edit to its mapping and writes the result,
// writing nothing when either step fails.
func update(root string, edit func(*yaml.Node) error) error {
	doc, err := read(root)
	if err != nil {
		return err
	}
	m, err := mapping(doc)
	if err != nil {
		return err
	}
	if err := edit(m); err != nil {
		return err
	}
	data, err := yaml.Marshal(doc)
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(root, FileName), data)
}

// writeAtomic replaces path with data through a temporary file, so a crash
// never leaves a half-written glossary.
func writeAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".glossary-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// Sorted returns a copy of entries in alphabetical order of term, ignoring
// case. The meanings of one term stay in the order given, and terms that
// differ only by case are ordered by their exact text.
func Sorted(entries []Entry) []Entry {
	out := slices.Clone(entries)
	slices.SortStableFunc(out, func(a, b Entry) int {
		if c := strings.Compare(strings.ToLower(a.Term), strings.ToLower(b.Term)); c != 0 {
			return c
		}
		return strings.Compare(a.Term, b.Term)
	})
	return out
}
