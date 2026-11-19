package filterbar

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/tkilb/para/internal/ui/panel"
)

const (
	// RowHeight is the rows the boxes take: a border above and below the one
	// line in each.
	RowHeight = 3
	// maxSuggestions is the most folders the dir suggestions show at once.
	maxSuggestions = 8
	// popupWidth is the outer width of the filters popup.
	popupWidth = 44
)

var (
	promptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#e78a4e"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	selBg       = panel.SelectedRowBackground
)

// Row draws the three boxes side by side: space and kind share leftW columns,
// the width of the list below, and dir takes rightW, the width of the pane
// beside it. Both are outer widths.
func (b Bar) Row(leftW, rightW int) string {
	// A panel is two columns wider than the width it is given.
	spaceW := (leftW - 4) / 2
	return lipgloss.JoinHorizontal(lipgloss.Top,
		b.spacePane(spaceW),
		b.kindPane(leftW-4-spaceW),
		b.dirPane(rightW-2),
	)
}

// Overlay draws what hangs over view, which is Row above the list and its pane
// (paneRows tall, the list leftW wide and the pane beside it rightW wide): the
// dir suggestions under the dir box and the filters popup in the middle.
func (b Bar) Overlay(view string, leftW, rightW, paneRows int) string {
	if b.suggesting() {
		view = overlay(view, b.suggestionBox(rightW-2, paneRows-2), leftW, RowHeight)
	}
	if b.open {
		x := max((leftW+rightW-popupWidth)/2, 0)
		view = overlay(view, b.popupBox(), x, RowHeight+1)
	}
	return view
}

// spacePane draws the space filter: the chosen space between arrows.
func (b Bar) spacePane(width int) string {
	focused := b.focus == FieldSpace
	return b.choicePane("Space", b.spaces[b.space].Label, width, focused)
}

// kindPane draws the kind filter: the chosen kind between arrows.
func (b Bar) kindPane(width int) string {
	label := allLabel
	if k := b.kinds[b.kind]; k != kindAll {
		label = k.String()
	}
	return b.choicePane("Kind", label, width, b.focus == FieldKind)
}

// choicePane draws a filter with a fixed set of choices.
func (b Bar) choicePane(title, label string, width int, focused bool) string {
	body := dimStyle.Render("‹ ") + promptStyle.Render(label) + dimStyle.Render(" ›")
	footer := panel.Footer{}
	if focused {
		footer = panel.Footer{Left: "←/→ or space change", Tint: true}
	}
	return panel.Frame(title, ansi.Truncate(body, width, "…"), width, 1, focused, footer)
}

// dirPane draws the path filter, which is inside the chosen space.
func (b Bar) dirPane(width int) string {
	focused := b.focus == FieldDir
	in := b.dir
	in.Width = max(width-1, 1)
	body := dimStyle.Render("")
	if prefix := b.spaces[b.space].Dir; prefix != "" && (focused || in.Value() != "") {
		body = dimStyle.Render(prefix + "/")
		in.Width = max(width-ansi.StringWidth(prefix)-2, 1)
	}
	footer := panel.Footer{}
	if focused {
		footer = panel.Footer{Left: "path in the space", Tint: true}
	}
	return panel.Frame("Dir", ansi.Truncate(body+in.View(), width, ""), width, 1, focused, footer)
}

// suggestionBox draws the folders that could come next in the dir field as a
// dropdown, with the one chosen by the arrows highlighted. height is the
// room of the pane it covers.
func (b Bar) suggestionBox(width, height int) string {
	names := b.dirHints()
	rows := max(min(len(names), maxSuggestions, height-2), 1)
	start, end := panel.ScrollWindow(max(b.hintSel, 0), len(names), rows)
	lines := make([]string, 0, rows)
	for i := start; i < end; i++ {
		text := ansi.Truncate(names[i]+"/", max(width-2, 1), "…")
		row := text + strings.Repeat(" ", max(width-2-ansi.StringWidth(text), 0))
		if i == b.hintSel {
			row = lipgloss.NewStyle().Background(selBg).Render(row)
		}
		lines = append(lines, row)
	}
	footer := panel.Footer{Left: "↑/↓ choose  enter fill in", Tint: true}
	return panel.Frame("Folders", strings.Join(lines, "\n"), width, len(lines), true, footer)
}

// popupBox draws the filters popup: a row for each filter with its choice
// between arrows, the row the keys act on highlighted.
func (b Bar) popupBox() string {
	source := "hidden"
	if b.showSource {
		source = "shown"
	}
	rows := [rowCount][2]string{
		rowSince:  {"Modified since", sinceOptions[b.since].Label},
		rowSource: {"_source.md", source},
	}
	inner := popupWidth - 2
	lines := make([]string, 0, rowCount)
	for i, r := range rows {
		value := dimStyle.Render("‹ ") + promptStyle.Render(r[1]) + dimStyle.Render(" ›")
		gap := strings.Repeat(" ", max(inner-ansi.StringWidth(r[0])-ansi.StringWidth(value)-1, 1))
		line := ansi.Truncate(" "+r[0]+gap+value, inner, "…")
		if i == b.row {
			line = lipgloss.NewStyle().Background(selBg).Render(ansi.Strip(line))
		}
		lines = append(lines, line)
	}
	footer := panel.Footer{Left: "↑/↓ row  ←/→ change  esc close", Tint: true}
	return panel.Frame("Filters", strings.Join(lines, "\n"), inner, len(lines), true, footer)
}

// overlay draws box over the lines of base from line y, starting at column
// x, keeping what base shows on either side.
func overlay(base, box string, x, y int) string {
	baseLines, boxLines := strings.Split(base, "\n"), strings.Split(box, "\n")
	for i, b := range boxLines {
		if y+i >= len(baseLines) {
			break
		}
		line := baseLines[y+i]
		rest := ansi.TruncateLeft(line, x+ansi.StringWidth(b), "")
		baseLines[y+i] = ansi.Truncate(line, x, "") + b + rest
	}
	return strings.Join(baseLines, "\n")
}
