package main

import (
	tea "github.com/charmbracelet/bubbletea"
)

var (
	keyUp   = tea.KeyMsg{Type: tea.KeyUp}
	keyDown = tea.KeyMsg{Type: tea.KeyDown}
)

// hit is a terminal cell resolved to a panel and the cell's position inside
// that panel's outer frame (0,0 is its top-left border corner).
type hit struct {
	panel panelID
	x, y  int
}

// hitAt maps a terminal cell to the panel drawn there, mirroring View. It
// reports false over the Title panel (never focusable), the status bar and
// anything outside the grid.
func (m model) hitAt(x, y int) (hit, bool) {
	g := m.grid()
	if x < 0 || y < 0 {
		return hit{}, false
	}
	if m.zoomed {
		return hit{m.focus, x, y}, x < m.width && y < m.height-1
	}
	if x < g.leftWidth {
		switch {
		case y < g.stickiesHeight:
			return hit{panelStickies, x, y}, true
		case y < g.stickiesHeight+g.paraHeight:
			return hit{panelPARA, x, y - g.stickiesHeight}, true
		case y < g.stickiesHeight+g.paraHeight+g.relatedHeight:
			return hit{panelRelated, x, y - g.stickiesHeight - g.paraHeight}, true
		}
		return hit{}, false
	}
	x -= g.leftWidth
	switch {
	case y < g.titleHeight:
		return hit{}, false
	case y < g.titleHeight+g.contentHeight:
		return hit{panelContent, x, y - g.titleHeight}, true
	case y < g.titleHeight+g.contentHeight+g.infoHeight:
		return hit{panelInfo, x, y - g.titleHeight - g.contentHeight}, true
	}
	return hit{}, false
}

// modalOpen reports whether a popup, prompt or other modal has the keyboard.
func (m model) modalOpen() bool {
	return m.popups.Active() || m.showingHelp || m.form != nil || m.clipForm != nil ||
		m.question != nil || m.picker != nil || m.grep != nil || m.pending != nil || m.input != nil
}

// updateMouse handles a mouse event. While a modal is open the grid is
// out of reach: a click dismisses a notice, the wheel moves through help, a
// picker or a menu, and everything else is ignored so a stray click never
// discards typed text. On the grid, a left click focuses the panel under the
// pointer and selects the row (or PARA tab) clicked, and the wheel focuses it
// and moves its selection like the arrow keys.
func (m model) updateMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action != tea.MouseActionPress {
		return m, nil
	}
	if m.modalOpen() {
		return m.modalMouse(msg)
	}
	h, ok := m.hitAt(msg.X, msg.Y)
	if !ok {
		return m, nil
	}
	switch msg.Button {
	case tea.MouseButtonLeft:
		return m.click(h), nil
	case tea.MouseButtonWheelUp:
		m.focus = h.panel
		return m.updateKey(keyUp)
	case tea.MouseButtonWheelDown:
		m.focus = h.panel
		return m.updateKey(keyDown)
	}
	return m, nil
}

// modalMouse is updateMouse while a modal is open.
func (m model) modalMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch {
	case m.popups.Active():
		if msg.Button == tea.MouseButtonLeft {
			m.popups = m.popups.Dismiss()
		}
	case m.showingHelp || m.picker != nil || m.grep != nil:
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			return m.updateKey(keyUp)
		case tea.MouseButtonWheelDown:
			return m.updateKey(keyDown)
		}
	}
	return m, nil
}

// click focuses the clicked panel and then selects what is under the pointer.
// The row is found before the focus changes, against the layout the user saw.
func (m model) click(h hit) model {
	g := m.grid()
	row := h.y - 1 // the first content row is under the top border
	switch h.panel {
	case panelStickies:
		// While collapsed there is one row, so a click only focuses; once
		// expanded it also chooses the row clicked.
		if m.focus == panelStickies {
			if i, ok := m.stickies.RowAt(row, g.stickiesHeight); ok {
				m.stickies = m.stickies.SelectRow(i)
			}
		}
	case panelPARA:
		if h.y == 0 {
			if i, ok := m.para.TabAt(h.x, g.leftWidth); ok {
				m.para = m.para.SelectTab(i)
			}
		} else if i, ok := m.para.RowAt(row); ok {
			m.para = m.para.SelectRow(i)
		}
	case panelRelated:
		if i, ok := m.related.RowAt(row, g.relatedHeight); ok {
			m.related = m.related.SelectRow(i)
		}
	case panelContent:
		if i, ok := m.content.RowAt(row); ok {
			m.content = m.content.SelectRow(i)
		}
	}
	m.focus = h.panel
	return m
}
