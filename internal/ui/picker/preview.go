package picker

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/tkilb/para/internal/ui/panel"
)

const (
	ellipsis = "…"
	// previewRows is the text rows the preview shows; the list gets the rest.
	previewRows = 4
	// listBox is the rows a list box takes besides its items: the field and
	// the two borders. A preview box has the two borders.
	listBox, previewBox = 3, 2
	// paneGap is the blank row between the list and the preview.
	paneGap = 1
	// unknownListRows is the list's rows before the terminal height is known.
	unknownListRows = 8
)

// stackedRows is how many item rows the list shows and how many text rows
// the preview shows when the two are stacked on a terminal screenHeight rows
// tall: the preview has a fixed height and the list takes what is left.
func stackedRows(screenHeight int) (list, preview int) {
	if screenHeight <= 0 {
		return unknownListRows, previewRows
	}
	budget := screenHeight - boxMargin - listBox - paneGap - previewBox - previewRows
	return min(max(budget, minListRows), largeMaxListRows), previewRows
}

// previewPane draws the highlighted item's Preview, wrapped to the pane and
// cut with an ellipsis when it does not fit, in a frame width columns wide
// inside and height rows tall.
func (m Model) previewPane(width, height int) string {
	it, ok := m.Chosen()
	if !ok || it.Preview == "" {
		return panel.Frame(m.previewName, "", width, height, false)
	}
	wrapped := lipgloss.NewStyle().Width(width).Render(it.Preview)
	lines := strings.Split(wrapped, "\n")
	if len(lines) > height {
		lines = lines[:height]
		lines[height-1] = ansi.Truncate(lines[height-1], max(width-1, 1), ellipsis)
	}
	return panel.Frame(m.previewName, strings.Join(lines, "\n"), width, height, false)
}
