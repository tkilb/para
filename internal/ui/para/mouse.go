package para

import "github.com/tkilb/para/internal/ui/panel"

// TabAt returns the tab drawn at column x of the panel's top border, for a
// panel outerWidth columns wide.
func (m Model) TabAt(x, outerWidth int) (int, bool) {
	width, _ := panel.InnerSize(outerWidth, 0)
	tabs := make([]panel.Tab, len(tabDefs))
	for i, d := range tabDefs {
		tabs[i] = panel.Tab{Label: d.label, Active: i == m.active}
	}
	return panel.TabAt(tabs, width, x)
}

// SelectTab switches to tab i, as [ and ] do.
func (m Model) SelectTab(i int) Model {
	if i < 0 || i >= len(tabDefs) {
		return m
	}
	return m.switchTab(i).syncScroll()
}

// RowAt returns the index of the row drawn at content row y (0 is the first
// row under the top border, which holds the path inside a folder).
func (m Model) RowAt(y int) (int, bool) {
	t := m.tabs[m.active]
	if t.err != nil {
		return 0, false
	}
	return panel.RowAtScroll(y-m.headerRows(), t.top, t.cursor, len(t.rows), m.listRows())
}

// SelectRow puts the cursor on row i, as moving with the arrow keys does.
func (m Model) SelectRow(i int) Model {
	m.moveTo(i)
	return m.syncScroll()
}
