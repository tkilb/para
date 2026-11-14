// Package picker implements a fuzzy picker popup: a text field above a list
// of items that narrows as you type. Matching uses github.com/sahilm/fuzzy
// for scattered matches (in memory, no network), ranked first by how the
// words fit (see rank.go). It has no lazytask counterpart:
// lazytask's pickers reuse its list panels inside popups.
package picker

import (
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/tkilb/para/internal/ui/panel"
)

const (
	minBoxWidth = 30
	maxBoxWidth = 60
	maxListRows = 10
	// A large picker (the finders) is as wide as the terminal allows up to
	// largeMaxBoxWidth, and shows up to largeMaxListRows rows.
	largeMaxBoxWidth = 120
	largeMaxListRows = 24
	minListRows      = 3
	boxMargin        = 8 // columns and rows kept free around the box
	chromeRows       = 4 // border (2), field and gap
	charLimit        = 100
	// detailExtra widens the detail column beyond half the box.
	detailExtra  = 4
	noMatchLabel = "(no matches)"
	defaultHint  = "enter select  esc cancel"
)

var (
	dimStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	selStyle   = lipgloss.NewStyle().Bold(true).Background(panel.SelectedRowBackground)
	matchColor = lipgloss.Color("3")
)

// Item is one choice. Label is matched against the query and shown; Detail
// is shown dim at the right; Value is what the caller gets back. Name, when
// set, is the end of Label that matters most (the file name in a path): an
// item with a word matching inside its Name ranks above one whose words match
// only the rest of the Label.
type Item struct {
	Label, Detail, Value string
	Name                 string
	// Preview is the text shown in the preview pane while the item is
	// highlighted (see WithPreview).
	Preview string
	// Color, when set, draws the label in that color (except on the
	// highlighted row, which keeps the selection colors).
	Color lipgloss.Color
}

// Outcome says how a key press ended the picker, if it did.
type Outcome int

const (
	// Pending means the picker is still open.
	Pending Outcome = iota
	// Chosen means enter picked the highlighted item.
	Chosen
	// Cancelled means esc closed the picker.
	Cancelled
)

// match is an item that fits the query, with the byte offsets of its
// matched characters in Label.
type match struct {
	item int
	at   map[int]bool
}

// Model is the picker state. Methods return updated copies.
type Model struct {
	title   string
	items   []Item
	field   textinput.Model
	matches []match
	cursor  int
	// screenHeight is the terminal height, which sets how many rows a page
	// is; zero (unknown) means a full list.
	screenHeight int
	// large makes the box wider and taller, for lists of long paths.
	large bool
	// hint, when set, replaces the footer's key hint.
	hint string
	// preview shows the highlighted item's Preview under the list, in a
	// pane titled previewName.
	preview     bool
	previewName string
}

// New returns a picker over items, listed in the given order until the user
// types. The command blinks the field's cursor.
func New(title string, items []Item) (Model, tea.Cmd) {
	field := textinput.New()
	field.CharLimit = charLimit
	field.Focus()
	m := Model{title: title, items: items, field: field}
	return m.refilter(), textinput.Blink
}

// WithScreenHeight tells the picker how tall the terminal is, so page up and
// page down move by the rows the list shows.
func (m Model) WithScreenHeight(h int) Model {
	m.screenHeight = h
	return m
}

// Large makes the picker's box wider and taller than usual.
func (m Model) Large() Model {
	m.large = true
	return m
}

// WithHint replaces the key hint in the picker's footer, for a picker whose
// owner handles extra keys.
func (m Model) WithHint(h string) Model {
	m.hint = h
	return m
}

// pageRows is how many list rows fit for a terminal screenHeight rows tall.
func (m Model) pageRows(screenHeight int) int {
	if m.preview {
		list, _ := stackedRows(screenHeight)
		return list
	}
	limit := maxListRows
	if m.large {
		limit = largeMaxListRows
	}
	if screenHeight <= 0 {
		return limit
	}
	return min(max(screenHeight-boxMargin-chromeRows, minListRows), limit)
}

// WithCursor starts the highlight on the i-th listed item (before the user
// types anything), clamped to the list.
func (m Model) WithCursor(i int) Model {
	m.cursor = min(max(i, 0), max(len(m.matches)-1, 0))
	return m
}

// refilter recomputes the matches for the current text and puts the cursor
// on the best one.
func (m Model) refilter() Model {
	m.cursor = 0
	words := strings.Fields(m.field.Value())
	if len(words) == 0 {
		m.matches = make([]match, len(m.items))
		for i := range m.items {
			m.matches[i] = match{item: i}
		}
		return m
	}
	// Each word is matched on its own, so "deep rocket" finds
	// "projects/deep/Rocket Plan": an item has to match every word, ranked
	// by how well the words matched (see rank), then by their scores added
	// up, and all the matched characters are marked.
	type candidate struct {
		item    int
		quality int
		score   int
		at      map[int]bool
	}
	cands := make([]candidate, len(m.items))
	for i := range m.items {
		cands[i] = candidate{item: i}
	}
	for _, word := range words {
		itemNos := make([]int, len(cands))
		for i, c := range cands {
			itemNos[i] = c.item
		}
		ranked := m.rank(word, itemNos)
		next := make([]candidate, 0, len(ranked))
		for i, c := range cands {
			wm, ok := ranked[i]
			if !ok {
				continue
			}
			c.quality += wm.quality
			c.score += wm.score
			at := make(map[int]bool, len(c.at)+len(wm.at))
			for b := range c.at {
				at[b] = true
			}
			for _, b := range wm.at {
				at[b] = true
			}
			c.at = at
			next = append(next, c)
		}
		cands = next
	}
	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].quality != cands[j].quality {
			return cands[i].quality > cands[j].quality
		}
		return cands[i].score > cands[j].score
	})
	m.matches = make([]match, len(cands))
	for i, c := range cands {
		m.matches[i] = match{item: c.item, at: c.at}
	}
	return m
}

// Chosen returns the highlighted item, if there is one.
func (m Model) Chosen() (Item, bool) {
	if m.cursor >= len(m.matches) {
		return Item{}, false
	}
	return m.items[m.matches[m.cursor].item], true
}

// Update handles a message while the picker is open: up and down move,
// enter picks, esc cancels, page up and down (or ctrl+u and ctrl+d) move a
// page, and everything else edits the query.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd, Outcome) {
	key, isKey := msg.(tea.KeyMsg)
	if isKey {
		switch key.String() {
		case "esc":
			return m, nil, Cancelled
		case "enter":
			if _, ok := m.Chosen(); ok {
				return m, nil, Chosen
			}
			return m, nil, Pending
		case "up":
			m.cursor = max(m.cursor-1, 0)
			return m, nil, Pending
		case "down":
			m.cursor = min(m.cursor+1, max(len(m.matches)-1, 0))
			return m, nil, Pending
		case "pgup", "ctrl+u":
			m.cursor = max(m.cursor-m.pageRows(m.screenHeight), 0)
			return m, nil, Pending
		case "pgdown", "ctrl+d":
			m.cursor = min(m.cursor+m.pageRows(m.screenHeight), max(len(m.matches)-1, 0))
			return m, nil, Pending
		}
	}
	before := m.field.Value()
	var cmd tea.Cmd
	m.field, cmd = m.field.Update(msg)
	if m.field.Value() != before {
		m = m.refilter()
	}
	return m, cmd, Pending
}

// View renders the picker as a bordered box sized for a screenWidth x
// screenHeight terminal; draw it with popup.Overlay.
func (m Model) View(screenWidth, screenHeight int) string {
	widest := maxBoxWidth
	if m.large {
		widest = largeMaxBoxWidth
	}
	boxWidth := min(max(screenWidth-boxMargin, minBoxWidth), widest)
	inner := boxWidth - 2
	rows := min(m.pageRows(screenHeight), max(len(m.matches), 1))
	if m.preview {
		// A fixed height keeps the preview pane from jumping as the list
		// narrows.
		rows = m.pageRows(screenHeight)
	}

	m.field.Width = max(inner-lipgloss.Width(m.field.Prompt)-1, 1)
	lines := []string{m.field.View()}
	if len(m.matches) == 0 {
		lines = append(lines, dimStyle.Render(noMatchLabel))
	} else {
		start, end := panel.ScrollWindow(m.cursor, len(m.matches), rows)
		for i := start; i < end; i++ {
			lines = append(lines, m.row(m.matches[i], inner, i == m.cursor))
		}
	}
	hint := defaultHint
	if m.hint != "" {
		hint = m.hint
	}
	footer := panel.Footer{Left: hint, Tint: true}
	if len(m.matches) > 0 {
		footer.Right = strings.Join([]string{itoa(m.cursor + 1), itoa(len(m.matches))}, "/")
	}
	if !m.preview {
		return panel.Frame(m.title, strings.Join(lines, "\n"), inner, len(lines), true, footer)
	}
	for len(lines) < rows+1 {
		lines = append(lines, "")
	}
	list := panel.Frame(m.title, strings.Join(lines, "\n"), inner, len(lines), true, footer)
	_, previewRows := stackedRows(screenHeight)
	return lipgloss.JoinVertical(lipgloss.Left, list, "", m.previewPane(inner, previewRows))
}

// WithPreview shows the highlighted item's Preview in a pane under the list,
// titled name.
func (m Model) WithPreview(name string) Model {
	m.preview, m.previewName = true, name
	return m
}

// row draws one item: its label with the matched characters picked out, and
// its detail right-aligned.
func (m Model) row(mt match, width int, cursor bool) string {
	it := m.items[mt.item]
	detail := ansi.Truncate(it.Detail, width/2+detailExtra, "…")
	label := ansi.Truncate(it.Label, max(width-lipgloss.Width(detail)-1, 1), "…")
	gap := strings.Repeat(" ", max(width-lipgloss.Width(label)-lipgloss.Width(detail), 0))

	base := lipgloss.NewStyle()
	if it.Color != "" {
		base = base.Foreground(it.Color)
	}
	if cursor {
		base = selStyle
	}
	hit := base.Foreground(matchColor).Underline(true)
	// The folders of a path, before the item's Name, are drawn grey.
	_, nameStart := it.name()
	folders := base.Foreground(dimStyle.GetForeground())
	var b strings.Builder
	for i, r := range label {
		switch {
		case mt.at[i]:
			b.WriteString(hit.Render(string(r)))
		case i < nameStart:
			b.WriteString(folders.Render(string(r)))
		default:
			b.WriteString(base.Render(string(r)))
		}
	}
	if cursor {
		return b.String() + selStyle.Render(gap+detail)
	}
	return b.String() + gap + dimStyle.Render(detail)
}

func itoa(n int) string { return strconv.Itoa(n) }
