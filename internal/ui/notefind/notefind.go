// Package notefind implements the note finder popup: the fuzzy picker of the
// notes' paths, with the filter boxes and the ctrl+f popup of the text search
// above it (see internal/ui/filterbar). A change to a filter replaces the
// notes the picker lists; what is typed stays.
package notefind

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/tkilb/para/internal/findtext"
	"github.com/tkilb/para/internal/ui/filterbar"
	"github.com/tkilb/para/internal/ui/picker"
)

const hint = "enter open  tab fields  ctrl+f filters  esc cancel"

// Outcome says how a key press ended the popup, if it did.
type Outcome int

const (
	// Pending means the popup is still open.
	Pending Outcome = iota
	// Chosen means enter picked the highlighted note.
	Chosen
	// Cancelled means esc closed the popup.
	Cancelled
)

// ItemFunc makes the picker item for a note, given its line (see
// findtext.Notes).
type ItemFunc func(note findtext.Line) picker.Item

// Model is the popup state. Methods return updated copies.
type Model struct {
	picker picker.Model
	bar    filterbar.Bar
	item   ItemFunc
	// limit is the most notes listed, taken from the start of the order they
	// came in; zero lists them all.
	limit int
}

// New returns the popup over notes, one line for each note in the order to
// list them (see findtext.Notes), opened with the filters in opts. The
// command blinks the query's cursor.
func New(title string, notes []findtext.Line, item ItemFunc, opts filterbar.Options) (Model, tea.Cmd) {
	m := Model{bar: filterbar.New(notes, opts), item: item}
	p, cmd := picker.New(title, m.items())
	m.picker = p.Large().Tall().WithHint(hint)
	return m, cmd
}

// Recent limits the list to the first limit notes that pass the filters, so
// notes given newest first list the most recent ones, and keeps that order
// among matches of the same quality while typing.
func (m Model) Recent(limit int) Model {
	m.limit = limit
	m.picker = m.picker.KeepOrder().SetItems(m.items())
	return m
}

// items are the picker items of the notes that pass the filters, at most the
// limit.
func (m Model) items() []picker.Item {
	scoped := findtext.First(m.bar.Scope(), m.limit)
	items := make([]picker.Item, len(scoped))
	for i, l := range scoped {
		items[i] = m.item(l)
	}
	return items
}

// Chosen returns the highlighted item, if there is one.
func (m Model) Chosen() (picker.Item, bool) { return m.picker.Chosen() }

// Update handles a message: the filter boxes and their popup take their keys
// (see filterbar.Update), and the rest goes to the picker, which moves,
// chooses, cancels and edits the query. screenH is the terminal's height.
func (m Model) Update(msg tea.Msg, screenH int) (Model, tea.Cmd, Outcome) {
	before := m.bar.Focus()
	bar, cmd, res := m.bar.Update(msg)
	m.bar = bar
	if res != filterbar.Ignored {
		if res == filterbar.Changed {
			m.picker = m.picker.SetItems(m.items())
		}
		if now := m.bar.Focus(); now != before {
			var focusCmd tea.Cmd
			m.picker, focusCmd = m.picker.SetFocused(now == filterbar.FieldQuery)
			cmd = tea.Batch(cmd, focusCmd)
		}
		return m, cmd, Pending
	}
	p, pcmd, out := m.picker.WithScreenHeight(screenH - filterbar.RowHeight).Update(msg)
	m.picker = p
	switch out {
	case picker.Chosen:
		return m, pcmd, Chosen
	case picker.Cancelled:
		return m, pcmd, Cancelled
	}
	return m, pcmd, Pending
}

// View draws the popup for a terminal screenW x screenH: the filter boxes
// above the picker, as wide as it is.
func (m Model) View(screenW, screenH int) string {
	h := screenH - filterbar.RowHeight
	list := m.picker.WithScreenHeight(h).WithTopRight(m.bar.Since()).View(screenW, h)
	width := m.picker.BoxWidth(screenW)
	left := width / 2
	view := lipgloss.JoinVertical(lipgloss.Left, m.bar.Row(left, width-left), list)
	return m.bar.Overlay(view, left, width-left, lipgloss.Height(list))
}
