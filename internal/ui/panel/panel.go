// Package panel provides the shared lazygit-style bordered-panel styling
// used across the panel grid: a rounded border and bold title, both
// switching to a highlight color when the panel is focused. Ported from
// lazytask.
package panel

import (
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const leadDashes = 2

const (
	// UnfocusedColor is the border/title color for a panel that does not
	// currently have focus. The empty color leaves the terminal foreground
	// in place, matching lazygit's "default" theme color.
	UnfocusedColor = lipgloss.Color("")
	// FocusedColor is the border/title color for the focused panel,
	// matching lazygit's default theme.activeBorderColor.
	FocusedColor = lipgloss.Color("2")

	// SelectedRowBackground is the background color for the selected row in a
	// list panel, matching lazygit's default theme.selectedLineBgColor.
	SelectedRowBackground = lipgloss.Color("4")
	// activeTabColor highlights the selected tab in a FrameTabs title, kept
	// distinct from FocusedColor so it stays visible in a focused panel.
	activeTabColor = lipgloss.Color("#e78a4e")

	// minWidth/minHeight guard against nonsensical (zero or negative) panel
	// sizes, e.g. before the first tea.WindowSizeMsg arrives.
	minWidth  = 10
	minHeight = 1
)

// TitleStyle returns the bold title style for a panel, colored according to
// whether it currently has focus.
func TitleStyle(focused bool) lipgloss.Style {
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(borderColor(focused))
}

func borderColor(focused bool) lipgloss.Color {
	if focused {
		return FocusedColor
	}
	return UnfocusedColor
}

// Render draws a titled, bordered panel with the given body text, sized to
// occupy outerWidth x outerHeight (the panel's total footprint, border
// included). The title is embedded in the top border line (lazygit-style,
// e.g. "╭─[1]-Notebook─────╮") so the full inner height is available to body.
func Render(title, body string, outerWidth, outerHeight int, focused bool) string {
	width, height := InnerSize(outerWidth, outerHeight)
	return Frame(title, body, width, height, focused)
}

// InnerSize converts a panel's total outer footprint (border included) into
// the content width/height available inside the border, clamped to sane
// minimums.
func InnerSize(outerWidth, outerHeight int) (width, height int) {
	width = max(outerWidth-2, minWidth)
	height = max(outerHeight-2, minHeight)
	return width, height
}

// Frame wraps body (already sized to width columns and up to height rows)
// in a rounded border of exactly width+2 x height+2, with title embedded in
// the top border line rather than as a separate content row.
//
// An optional Footer (e.g. a path on the left and a "3/10" scroll position
// on the right) is embedded in the bottom border line. Only the first is used.
func Frame(title, body string, width, height int, focused bool, footer ...Footer) string {
	return FrameTopRight(title, "", body, width, height, focused, footer...)
}

// FrameTopRight is like Frame, with topRight embedded in the top border line
// near the right corner, such as a setting that applies to the whole list. It
// is dropped when it does not fit beside the title.
func FrameTopRight(title, topRight, body string, width, height int, focused bool, footer ...Footer) string {
	lineStyle := lipgloss.NewStyle().Foreground(borderColor(focused))
	top := topBorder(title, width, focused, lineStyle)
	if topRight != "" {
		top = topBorderRight(title, topRight, width, focused, lineStyle)
	}
	return frameBody(top, firstFooter(footer), body, width, height, lineStyle)
}

// Footer is text embedded in a panel's bottom border line.
type Footer struct {
	Left, Right string
	// Tint draws the text in the border's color instead of the terminal's
	// default, for key hints that should read as part of the border.
	Tint bool
}

func firstFooter(footer []Footer) Footer {
	if len(footer) == 0 {
		return Footer{}
	}
	return footer[0]
}

// Tab describes one segment of a multi-tab panel title (see FrameTabs).
type Tab struct {
	Label  string
	Active bool
	// MarkColor, when set, colors the first character of Label, a mark such
	// as "❖" that stands apart from the name.
	MarkColor lipgloss.Color
}

// FrameTabs is like Frame, but embeds a row of tabs in the title instead of
// a single label (e.g. "╭─[2]-Inbox - Projects─╮"), styling the active tab
// distinctly from the others.
func FrameTabs(number rune, tabs []Tab, body string, width, height int, focused bool, footer ...Footer) string {
	lineStyle := lipgloss.NewStyle().Foreground(borderColor(focused))
	top := topBorderTabs(number, tabs, width, focused, lineStyle)
	return frameBody(top, firstFooter(footer), body, width, height, lineStyle)
}

// frameBody renders the border/content assembly shared by Frame and
// FrameTabs, given an already-built top border line.
func frameBody(top string, footer Footer, body string, width, height int, lineStyle lipgloss.Style) string {
	// Height/Width here size only the content area; the border characters
	// are added separately above/below/around it.
	content := lipgloss.NewStyle().Width(width).Height(height).Render(body)

	var b strings.Builder
	b.WriteString(top)
	b.WriteString("\n")
	side := lineStyle.Render("│")
	for _, line := range strings.Split(content, "\n") {
		b.WriteString(side)
		b.WriteString(line)
		b.WriteString(side)
		b.WriteString("\n")
	}
	b.WriteString(bottomBorder(footer, width, lineStyle))
	return b.String()
}

// ScrollWindow returns the [start, end) slice bounds of a scrollable list of
// totalItems entries, sized so exactly visibleRows are shown, such that
// cursor always falls within [start, end). It is a pure function of its
// arguments, so callers need not persist a scroll offset.
func ScrollWindow(cursor, totalItems, visibleRows int) (start, end int) {
	visibleRows = max(visibleRows, 1)
	if totalItems <= visibleRows {
		return 0, totalItems
	}
	start = min(max(cursor-visibleRows+1, 0), totalItems-visibleRows)
	return start, start + visibleRows
}

// ScrollOff is how many rows of context a list keeps above and below its
// cursor while scrolling, as Vim's scrolloff does.
const ScrollOff = 3

// ScrollTop returns the index of the first row to show in a list of total
// rows, rows of which fit, given the first row shown before (top) and the
// cursor. The window stays where it is while the cursor moves inside it, and
// scrolls only when the cursor comes within margin rows of an edge, so the
// cursor is not stuck on the last row. At either end of the list the cursor
// can reach the edge. The caller keeps the result to pass in next time.
func ScrollTop(top, cursor, total, rows, margin int) int {
	rows = max(rows, 1)
	margin = min(max(margin, 0), (rows-1)/2)
	if cursor < top+margin {
		top = cursor - margin
	}
	if cursor > top+rows-1-margin {
		top = cursor - (rows - 1 - margin)
	}
	return min(max(top, 0), max(total-rows, 0))
}

// Window returns the [start, end) slice bounds of a list of total items that
// shows rows of them from top, as ScrollTop chose.
func Window(top, total, rows int) (start, end int) {
	rows = max(rows, 1)
	start = min(max(top, 0), max(total-rows, 0))
	return start, min(start+rows, total)
}

// RowAtScroll maps a content row of a list (0 is the first row drawn) to the
// item index drawn there for a list drawn with ScrollTop and Window. It
// reports false past the last item.
func RowAtScroll(row, top, cursor, total, rows int) (int, bool) {
	start, end := Window(ScrollTop(top, cursor, total, rows, ScrollOff), total, rows)
	if row < 0 || start+row >= end {
		return 0, false
	}
	return start + row, true
}

// topBorder builds the top border line with the title embedded, e.g.
// "╭──[1]-Notebook────────╮". A title that does not fit within width is
// truncated to avoid overflowing the panel.
func topBorder(title string, width int, focused bool, lineStyle lipgloss.Style) string {
	label := titleLabel(title)
	if len(label) > width-leadDashes {
		label = label[:max(width-leadDashes, 0)]
	}
	fill := max(width-leadDashes-len(label), 0)

	var b strings.Builder
	b.WriteString(lineStyle.Render("╭" + strings.Repeat("─", leadDashes)))
	b.WriteString(TitleStyle(focused).Render(label))
	b.WriteString(lineStyle.Render(strings.Repeat("─", fill) + "╮"))
	return b.String()
}

// topBorderRight is topBorder with right near the right corner, one dash
// before it, e.g. "╭─Grep──────────7 days─╮". A right that does not fit with
// at least a dash between it and the title is left out.
func topBorderRight(title, right string, width int, focused bool, lineStyle lipgloss.Style) string {
	const cornerDashes = 1
	label := titleLabel(title)
	if len(label) > width-leadDashes {
		label = label[:max(width-leadDashes, 0)]
	}
	rightWidth := ansi.StringWidth(right)
	fill := width - leadDashes - len(label) - rightWidth - cornerDashes
	if fill < 1 {
		return topBorder(title, width, focused, lineStyle)
	}
	var b strings.Builder
	b.WriteString(lineStyle.Render("╭" + strings.Repeat("─", leadDashes)))
	b.WriteString(TitleStyle(focused).Render(label))
	b.WriteString(lineStyle.Render(strings.Repeat("─", fill)))
	b.WriteString(TitleStyle(focused).Render(right))
	b.WriteString(lineStyle.Render(strings.Repeat("─", cornerDashes) + "╮"))
	return b.String()
}

// titleLabel formats a panel title for the border line. Titles of the form
// "<digit> <name>" (the number-key hint, e.g. "1 Notebook") become
// "[1]-Notebook"; any other title is used as-is.
func titleLabel(title string) string {
	if len(title) > 2 && title[1] == ' ' && title[0] >= '0' && title[0] <= '9' {
		return "[" + string(title[0]) + "]-" + title[2:]
	}
	return title
}

// bottomBorder builds the bottom border line, embedding footer.Left near
// the left corner and footer.Right near the right corner, e.g.
// "╰─/daily────────3/10─╯". A Right that does not fit is dropped; a Left
// that does not fit is shortened from its start, keeping the deepest names.
func bottomBorder(footer Footer, width int, lineStyle lipgloss.Style) string {
	const cornerDashes = 1
	right := footer.Right
	if lipgloss.Width(right) > width-2*cornerDashes {
		right = ""
	}
	rightWidth := lipgloss.Width(right)
	if rightWidth > 0 {
		rightWidth += 1 // keep one dash between the two labels
	}
	left := truncateHead(footer.Left, width-2*cornerDashes-rightWidth)

	fill := max(width-2*cornerDashes-lipgloss.Width(left)-lipgloss.Width(right), 0)
	paint := func(s string) string {
		if footer.Tint {
			return lineStyle.Render(s)
		}
		return s
	}
	return lineStyle.Render("╰"+strings.Repeat("─", cornerDashes)) + paint(left) +
		lineStyle.Render(strings.Repeat("─", fill)) + paint(right) +
		lineStyle.Render(strings.Repeat("─", cornerDashes)+"╯")
}

// truncateHead shortens s to at most width columns by replacing its start
// with an ellipsis.
func truncateHead(s string, width int) string {
	if width <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	if width == 1 {
		return "…"
	}
	return "…" + string(r[len(r)-width+1:])
}

// tabSeparator joins the labels in a FrameTabs title.
const tabSeparator = " - "

// topBorderTabs builds the top border line for FrameTabs, with the active
// tab styled distinctly. It uses the first of these layouts that fits within
// width: the active tab spelled out with inactive tabs shrunk to their first
// letter; the active tab alone. If none fits, it falls back to a truncated plain label.
func topBorderTabs(number rune, tabs []Tab, width int, focused bool, lineStyle lipgloss.Style) string {
	prefix := "[" + string(number) + "]-"

	shown, budget := shownTabs(tabs, width, len(prefix))
	if shown == nil {
		return topBorder(prefix+tabsPlain(tabs), width, focused, lineStyle)
	}
	return renderTabsBorder(prefix, shown, budget, focused, lineStyle)
}

// renderTabsBorder draws the top border line for tabs that fit the budget
// columns left after the lead dashes and the prefix.
// renderTabLabel draws a tab's label in style, with its first character in
// MarkColor when that is set.
func renderTabLabel(t Tab, style lipgloss.Style) string {
	if t.MarkColor == "" || t.Label == "" {
		return style.Render(t.Label)
	}
	_, size := utf8.DecodeRuneInString(t.Label)
	mark := style.Foreground(t.MarkColor).Render(t.Label[:size])
	if rest := t.Label[size:]; rest != "" {
		return mark + style.Render(rest)
	}
	return mark
}

func renderTabsBorder(prefix string, shown []Tab, budget int, focused bool, lineStyle lipgloss.Style) string {
	inactive := lipgloss.NewStyle()
	active := lipgloss.NewStyle().Bold(true).Foreground(activeTabColor)

	var b strings.Builder
	b.WriteString(lineStyle.Render("╭" + strings.Repeat("─", leadDashes)))
	b.WriteString(TitleStyle(focused).Render(prefix))
	for i, t := range shown {
		if i > 0 {
			b.WriteString(inactive.Render(tabSeparator))
		}
		style := inactive
		if t.Active {
			style = active
		}
		b.WriteString(renderTabLabel(t, style))
	}
	fill := max(budget-tabsWidth(shown), 0)
	b.WriteString(lineStyle.Render(strings.Repeat("─", fill) + "╮"))
	return b.String()
}

// shownTabs picks the widest layout of tabs that fits a title of width
// columns after the lead dashes and a prefix of prefixLen columns. It returns
// nil when none fits, and the columns left for the layout.
func shownTabs(tabs []Tab, width, prefixLen int) ([]Tab, int) {
	budget := width - leadDashes - prefixLen
	for _, layout := range tabLayouts(tabs) {
		if tabsWidth(layout) <= budget {
			return layout, budget
		}
	}
	return nil, budget
}

// TabAt maps a column of the panel's top border (0 is the corner) to the
// index of the tab drawn there, for a title built by FrameTabs with an inner
// width of width. It reports false over the border, a separator, or when only
// the active tab is shown.
func TabAt(tabs []Tab, width, x int) (int, bool) {
	const prefixLen = len("[0]-")
	shown, _ := shownTabs(tabs, width, prefixLen)
	if shown == nil || len(shown) != len(tabs) {
		return 0, false
	}
	start := 1 + leadDashes + prefixLen
	for i, t := range shown {
		if x >= start && x < start+len(t.Label) {
			return i, true
		}
		start += len(t.Label) + len(tabSeparator)
	}
	return 0, false
}

// RowAt maps a content row of a list (0 is the first row drawn) to the item
// index drawn there, using the ScrollWindow its View uses. It reports false
// past the last item.
func RowAt(row, cursor, totalItems, visibleRows int) (int, bool) {
	start, end := ScrollWindow(cursor, totalItems, visibleRows)
	if row < 0 || start+row >= end {
		return 0, false
	}
	return start + row, true
}

// tabLayouts lists the candidate layouts of tabs, widest first: the active
// tab spelled out with the others shrunk to their first letter, then the
// active tab alone. Full labels for every tab are never used.
func tabLayouts(tabs []Tab) [][]Tab {
	compact := make([]Tab, len(tabs))
	var activeOnly []Tab
	for i, t := range tabs {
		compact[i] = t
		if !t.Active && t.Label != "" && !isNumber(t.Label) {
			compact[i].Label = t.Label[:1]
		}
		if t.Active {
			activeOnly = append(activeOnly, t)
		}
	}
	return [][]Tab{compact, activeOnly}
}

// isNumber reports whether s is only digits, a label that shrinking would
// make wrong: "12" is not "1".
func isNumber(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

func tabsWidth(tabs []Tab) int {
	total := len(tabSeparator) * max(len(tabs)-1, 0)
	for _, t := range tabs {
		total += ansi.StringWidth(t.Label)
	}
	return total
}

func tabsPlain(tabs []Tab) string {
	labels := make([]string, len(tabs))
	for i, t := range tabs {
		labels[i] = t.Label
	}
	return strings.Join(labels, tabSeparator)
}
