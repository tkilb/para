// Package grep implements the text search popup: a query box and a list of
// matching lines on the left, and a preview of the note around the chosen
// line on the right, as the Snacks grep picker in Neovim does. The matching
// is exact words, ignoring case (see internal/search).
package grep

import (
	"strings"
	"time"

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

// sinceOption is one choice of the "Modified since" filter. Within is how far
// back a note may have changed, zero for any time.
type sinceOption struct {
	Label  string
	Within time.Duration
}

const day = 24 * time.Hour

// sinceOptions are the choices of the "Modified since" filter; the first is
// no filter.
var sinceOptions = []sinceOption{
	{"Any time", 0},
	{"Past 24 hours", day},
	{"Past 7 days", 7 * day},
	{"Past 30 days", 30 * day},
	{"Past year", 365 * day},
}

// The rows of the filters popup.
const (
	rowSince = iota
	rowSource
	filterCount
)

// kindAll stands for every kind in the Kind filter. It is not a valid kind,
// so it cannot clash with one the config adds.
const kindAll store.Kind = ""

// Model is the popup state. Methods return updated copies.
type Model struct {
	field textinput.Model
	lines []findtext.Line
	rels  []string // the notes of lines, once each, for completing the dir
	// hintSel is the folder chosen in the dir suggestions with the arrows, or
	// -1 when none is.
	hintSel int
	scoped  []findtext.Line // lines inside the space and the path filter

	spaces   []Space // spaces[0] is All
	space    int
	kinds    []store.Kind // kinds[0] is kindAll, the last is store.KindUnset
	kind     int
	dirField textinput.Model
	focus    focus
	capped   bool // the lines were cut at findtext.MaxLines
	// showSource lists the lines of import transcripts (_source.md), which
	// are left out until the filters popup (ctrl+f) turns them on.
	showSource bool
	// since is the choice in sinceOptions: how recently a note changed.
	since int
	// filtersOpen is set while the filters popup is shown, and filterRow is
	// the row of it the keys act on.
	filtersOpen bool
	filterRow   int
	load        Loader

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
	m := Model{field: field, dirField: dir, lines: lines, capped: capped, load: load, hintSel: -1}
	m.spaces = append([]Space{{Label: allLabel}}, opts.Spaces...)
	for i, sp := range m.spaces {
		if i > 0 && sp.Dir == opts.Space {
			m.space = i
		}
	}
	m.rels = noteRels(lines)
	m.kinds = append(append([]store.Kind{kindAll}, store.Kinds()...), store.KindUnset)
	m.scoped = m.scope()
	return m, textinput.Blink
}

// noteRels returns the paths of the notes in lines, once each, in order.
func noteRels(lines []findtext.Line) []string {
	var rels []string
	for i, l := range lines {
		if i == 0 || l.Rel != lines[i-1].Rel {
			rels = append(rels, l.Rel)
		}
	}
	return rels
}

// dirHints returns the folders that could come next in the dir field: the
// ones inside the chosen space (or the notebook, for All) that start with
// what is typed after its last slash.
func (m Model) dirHints() []string {
	typed := strings.TrimLeft(strings.TrimSpace(m.dirField.Value()), "/")
	if d := m.spaces[m.space].Dir; d != "" {
		return findtext.ChildDirs(m.rels, d+"/"+typed)
	}
	return findtext.ChildDirs(m.rels, typed)
}

// acceptHint puts the chosen suggestion in the dir field, with a slash after
// it, ready to list the folders inside it.
func (m Model) acceptHint() Model {
	names := m.dirHints()
	if m.hintSel < 0 || m.hintSel >= len(names) {
		return m
	}
	typed := strings.TrimLeft(strings.TrimSpace(m.dirField.Value()), "/")
	m.dirField.SetValue(typed[:strings.LastIndex(typed, "/")+1] + names[m.hintSel] + "/")
	m.dirField.CursorEnd() // SetValue keeps the old position when it still fits
	m.hintSel = -1
	return m.rescope()
}

// setDir replaces the dir field and searches again.
func (m Model) setDir(v string) Model {
	m.dirField.SetValue(v)
	m.dirField.CursorEnd()
	m.hintSel = -1
	return m.rescope()
}

// moveHint moves the choice in the dir suggestions delta places: from none to
// the first with down, to the last with up, and back to none past either end.
func (m Model) moveHint(delta int) Model {
	n := len(m.dirHints())
	m.hintSel += delta
	switch {
	case m.hintSel < -1:
		m.hintSel = n - 1
	case m.hintSel >= n:
		m.hintSel = -1
	}
	return m
}

// suggesting reports whether the dir suggestions are open.
func (m Model) suggesting() bool {
	return m.focus == focusDir && len(m.dirHints()) > 0
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
	if w := sinceOptions[m.since].Within; w > 0 {
		lines = findtext.ChangedSince(lines, time.Now().Add(-w))
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
	m.focus, m.hintSel = f, -1
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

// pickSpace moves the chosen space delta places, wrapping. The dir is a path
// inside the space, so it is cleared.
func (m Model) pickSpace(delta int) Model {
	m.space = (m.space + delta + len(m.spaces)) % len(m.spaces)
	m.dirField.SetValue("")
	m.hintSel = -1
	return m.rescope()
}

// pickKind moves the chosen kind delta places, wrapping.
func (m Model) pickKind(delta int) Model {
	m.kind = (m.kind + delta + len(m.kinds)) % len(m.kinds)
	return m.rescope()
}

// filtersKey handles a key while the filters popup is open: up, down and tab
// move between its rows, left, right and space change the row, and enter, esc
// and ctrl+f close it. Other keys do nothing, so typing never reaches the
// query behind it.
func (m Model) filtersKey(k tea.KeyMsg) Model {
	switch k.String() {
	case "esc", "enter", "ctrl+f":
		m.filtersOpen = false
	case "up", "shift+tab", "ctrl+p":
		m.filterRow = (m.filterRow + filterCount - 1) % filterCount
	case "down", "tab", "ctrl+n":
		m.filterRow = (m.filterRow + 1) % filterCount
	case "left", "h", "shift+left":
		return m.changeFilter(-1)
	case "right", "l", " ", "shift+right":
		return m.changeFilter(1)
	}
	return m
}

// changeFilter moves the filter in the current row delta places, wrapping,
// and searches again.
func (m Model) changeFilter(delta int) Model {
	switch m.filterRow {
	case rowSince:
		n := len(sinceOptions)
		m.since = (m.since + delta + n) % n
	case rowSource:
		m.showSource = !m.showSource
	}
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
// shift+tab move between the query, space, kind and dir fields (up and down in dir choose from its folder suggestions and enter fills one in;
// ctrl+u clears the dir and ctrl+w removes its last folder name), ctrl+f opens the
// filters popup, enter chooses,
// esc cancels, and anything else edits the field in focus (left, right, h, l
// and space change the space or kind).
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd, Outcome) {
	if k, ok := msg.(tea.KeyMsg); ok {
		if m.filtersOpen {
			return m.filtersKey(k), nil, Pending
		}
		if m.focus == focusDir {
			switch k.String() {
			case "ctrl+u":
				return m.setDir(""), nil, Pending
			case "ctrl+w":
				return m.setDir(findtext.DropLastSegment(m.dirField.Value())), nil, Pending
			}
		}
		if m.suggesting() {
			switch k.String() {
			case "up", "ctrl+p":
				return m.moveHint(-1), nil, Pending
			case "down", "ctrl+n":
				return m.moveHint(1), nil, Pending
			case "enter":
				if m.hintSel >= 0 {
					return m.acceptHint(), nil, Pending
				}
			}
		}
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
		case "ctrl+f":
			m.filtersOpen, m.filterRow = true, rowSince
			return m, nil, Pending
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
			m.hintSel = -1
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
