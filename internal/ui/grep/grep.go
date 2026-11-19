// Package grep implements the text search popup: a query box and a list of
// matching lines on the left, and a preview of the note around the chosen
// line on the right, as the Snacks grep picker in Neovim does. The matching
// is exact words, ignoring case (see internal/search).
package grep

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/tkilb/para/internal/digest"
	"github.com/tkilb/para/internal/findtext"
	"github.com/tkilb/para/internal/search"
	"github.com/tkilb/para/internal/store"
)

const (
	// maxHits bounds the lines listed; the count still says how many matched.
	maxHits   = 500
	charLimit = 100
)

// Outcome says how a key press ended the popup, if it did.
type Outcome int

const (
	// Pending means the popup is still open.
	Pending Outcome = iota
	// Chosen means enter picked the highlighted line.
	Chosen
	// Cancelled means esc closed the popup.
	Cancelled
)

// Loader returns the lines of the note at rel, as the Content panel shows
// them, for the preview.
type Loader func(rel string) []string

// Space is a folder of the notebook the search can be limited to.
type Space struct {
	Label, Dir string
}

// Options sets the filters the popup opens with.
type Options struct {
	// Spaces are the folders to choose from; "All" is added before them.
	Spaces []Space
	// Space is the Dir of the space chosen at first ("" for all).
	Space string
	// Dir is the folder path inside the space that the search starts in.
	Dir string
}

// focus is the field the keys edit.
type focus int

const (
	focusQuery focus = iota
	focusSpace
	focusKind
	focusDir
	focusCount
)

const allLabel = "All"

// kindAll stands for every kind in the Kind filter. It is not a valid kind,
// so it cannot clash with one the config adds.
const kindAll store.Kind = ""

// Model is the popup state. Methods return updated copies.
type Model struct {
	field  textinput.Model
	lines  []findtext.Line
	scoped []findtext.Line // lines inside the space and the path filter

	spaces   []Space // spaces[0] is All
	space    int
	kinds    []store.Kind // kinds[0] is kindAll, the last is store.KindUnset
	kind     int
	dirField textinput.Model
	focus    focus
	capped   bool // the lines were cut at findtext.MaxLines
	// showSource lists the lines of import transcripts (_source.md), which
	// are left out until ctrl+s turns them on.
	showSource bool
	load       Loader

	res    search.Result
	cursor int

	// preview is the note of the highlighted line; scroll moves the view of
	// it away from the line, in lines.
	previewRel string
	preview    []string
	scroll     int

	screenW, screenH int
}

// New returns the popup over lines, which capped says were cut short, opened
// with the filters in opts. load reads a note for the preview. The command
// blinks the query's cursor.
func New(lines []findtext.Line, capped bool, load Loader, opts Options) (Model, tea.Cmd) {
	field := textinput.New()
	field.CharLimit = charLimit
	field.Prompt = "> "
	field.PromptStyle = promptStyle
	field.Focus()

	dir := textinput.New()
	dir.CharLimit = charLimit
	dir.Prompt = ""
	dir.Placeholder = "(whole space)"

	dir.SetValue(opts.Dir)
	m := Model{field: field, dirField: dir, lines: lines, capped: capped, load: load}
	m.spaces = append([]Space{{Label: allLabel}}, opts.Spaces...)
	for i, sp := range m.spaces {
		if i > 0 && sp.Dir == opts.Space {
			m.space = i
		}
	}
	m.kinds = append(append([]store.Kind{kindAll}, store.Kinds()...), store.KindUnset)
	m.scoped = m.scope()
	return m, textinput.Blink
}

// scope returns the lines inside the chosen space and path filter. The path
// is inside the space, or from the notebook root when the space is All.
func (m Model) scope() []findtext.Line {
	lines := m.lines
	if !m.showSource {
		lines = findtext.WithoutFile(lines, digest.SourceFile)
	}
	if k := m.kinds[m.kind]; k != kindAll {
		lines = findtext.OfKind(lines, k)
	}
	prefix := strings.TrimLeft(strings.TrimSpace(m.dirField.Value()), "/")
	if d := m.spaces[m.space].Dir; d != "" {
		prefix = d + "/" + prefix
	}
	if prefix == "" {
		return lines
	}
	var out []findtext.Line
	for _, l := range lines {
		if findtext.UnderPath(l.Rel, prefix) {
			out = append(out, l)
		}
	}
	return out
}

// rescope applies the filters and searches again.
func (m Model) rescope() Model {
	m.scoped = m.scope()
	m.res = search.Find(m.scoped, m.field.Value(), maxHits)
	m.cursor, m.scroll = 0, 0
	return m.syncPreview()
}

// setFocus moves the keyboard to field f.
func (m Model) setFocus(f focus) (Model, tea.Cmd) {
	m.focus = f
	m.field.Blur()
	m.dirField.Blur()
	switch f {
	case focusQuery:
		return m, m.field.Focus()
	case focusDir:
		return m, m.dirField.Focus()
	}
	return m, nil
}

// pickSpace moves the chosen space delta places, wrapping.
func (m Model) pickSpace(delta int) Model {
	m.space = (m.space + delta + len(m.spaces)) % len(m.spaces)
	return m.rescope()
}

// pickKind moves the chosen kind delta places, wrapping.
func (m Model) pickKind(delta int) Model {
	m.kind = (m.kind + delta + len(m.kinds)) % len(m.kinds)
	return m.rescope()
}

// WithScreen tells the popup how big the terminal is, so paging and the
// preview scroll move by what the panes show.
func (m Model) WithScreen(w, h int) Model {
	m.screenW, m.screenH = w, h
	return m
}

// Chosen returns the highlighted line, if there is one.
func (m Model) Chosen() (search.Hit, bool) {
	if m.cursor < 0 || m.cursor >= len(m.res.Hits) {
		return search.Hit{}, false
	}
	return m.res.Hits[m.cursor], true
}

// Update handles a message: the arrows move through the lines, page up and
// down move by a page, ctrl+d and ctrl+u scroll the preview, tab and
// shift+tab move between the query, space, kind and dir fields, ctrl+s shows or
// hides the lines of _source.md files, enter chooses,
// esc cancels, and anything else edits the field in focus (left, right, h, l
// and space change the space or kind).
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd, Outcome) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "esc":
			return m, nil, Cancelled
		case "enter":
			if _, ok := m.Chosen(); ok {
				return m, nil, Chosen
			}
			return m, nil, Pending
		case "up", "ctrl+p":
			return m.move(-1), nil, Pending
		case "down", "ctrl+n":
			return m.move(1), nil, Pending
		case "pgup":
			return m.move(-m.pageRows()), nil, Pending
		case "pgdown":
			return m.move(m.pageRows()), nil, Pending
		case "ctrl+u":
			return m.scrollPreview(-m.pageRows() / 2), nil, Pending
		case "ctrl+d":
			return m.scrollPreview(m.pageRows() / 2), nil, Pending
		case "ctrl+s":
			m.showSource = !m.showSource
			return m.rescope(), nil, Pending
		case "tab":
			m, cmd := m.setFocus((m.focus + 1) % focusCount)
			return m, cmd, Pending
		case "shift+tab":
			m, cmd := m.setFocus((m.focus + focusCount - 1) % focusCount)
			return m, cmd, Pending
		}
		if m.focus == focusSpace || m.focus == focusKind {
			pick := m.pickSpace
			if m.focus == focusKind {
				pick = m.pickKind
			}
			switch k.String() {
			case "left", "h", "shift+left":
				return pick(-1), nil, Pending
			case "right", "l", " ", "shift+right":
				return pick(1), nil, Pending
			}
			return m, nil, Pending
		}
	}
	if m.focus == focusDir {
		before := m.dirField.Value()
		var cmd tea.Cmd
		m.dirField, cmd = m.dirField.Update(msg)
		if m.dirField.Value() != before {
			m = m.rescope()
		}
		return m, cmd, Pending
	}
	before := m.field.Value()
	var cmd tea.Cmd
	m.field, cmd = m.field.Update(msg)
	if m.field.Value() != before {
		m.res = search.Find(m.scoped, m.field.Value(), maxHits)
		m.cursor, m.scroll = 0, 0
		m = m.syncPreview()
	}
	return m, cmd, Pending
}

// move puts the highlight delta lines away, clamped.
func (m Model) move(delta int) Model {
	m.cursor = min(max(m.cursor+delta, 0), max(len(m.res.Hits)-1, 0))
	m.scroll = 0
	return m.syncPreview()
}

// scrollPreview moves the preview delta lines away from the highlighted line.
func (m Model) scrollPreview(delta int) Model {
	m.scroll += delta
	return m
}

// syncPreview loads the note of the highlighted line when it changed.
func (m Model) syncPreview() Model {
	h, ok := m.Chosen()
	if !ok {
		m.previewRel, m.preview = "", nil
		return m
	}
	if h.Rel != m.previewRel {
		m.previewRel, m.preview = h.Rel, m.load(h.Rel)
	}
	return m
}
