package para

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/tkilb/para/internal/textsafe"
	"github.com/tkilb/para/internal/ui/kindstyle"
	"github.com/tkilb/para/internal/ui/panel"
)

// panelNumber is the key that focuses this panel.
const panelNumber = '2'

const (
	badgeWidth = 4
)

var (
	dimStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	errorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	dirColor   = lipgloss.Color("6")
	// dirStyle is a folder row in the list, plain; the folder you are in is
	// bold in the path row (currentStyle).
	dirStyle = lipgloss.NewStyle().Foreground(dirColor)
	// overdueColor is the name color of a daily note past its retention that
	// has not been archived. It is also the count color of an empty folder.
	overdueColor = lipgloss.Color("208")
	selStyle     = lipgloss.NewStyle().Bold(true).Background(panel.SelectedRowBackground)
	// The left-edge bar of a marked row is colored as in Yazi.
	markerColors = map[mark]lipgloss.Color{
		markSelected: lipgloss.Color("3"), // yellow
		markYank:     lipgloss.Color("2"), // green
		markCut:      lipgloss.Color("1"), // red
	}
)

// mark says why a row has a left-edge bar.
type mark int

const (
	markNone mark = iota
	markSelected
	markYank
	markCut
)

// markOf picks a row's marker. The yank buffer outranks the selection,
// because it is what a paste acts on.
func (m Model) markOf(t tabState, r row) mark {
	if m.clip.set[r.entry.Rel] {
		if m.clip.mode == clipCut {
			return markCut
		}
		return markYank
	}
	if t.selected[r.entry.Name] {
		return markSelected
	}
	return markNone
}

const (
	// markerBar is drawn in the first column of a selected row.
	markerBar   = "▎"
	markerWidth = 1
	// bullet is drawn in the first column of a marked note's row, where the
	// bar goes. A selected, yanked or cut row shows its bar instead.
	bullet = "•"
)

// View renders the panel into outerWidth x outerHeight, border included.
func (m Model) View(outerWidth, outerHeight int, focused bool) string {
	width, height := panel.InnerSize(outerWidth, outerHeight)
	t := m.tabs[m.active]

	tabs := make([]panel.Tab, len(tabDefs))
	for i, d := range tabDefs {
		tabs[i] = panel.Tab{Label: d.label, Active: i == m.active}
	}

	// Inside a folder the path is the first row, so the bottom border has
	// only the search.
	left := ""
	if t.search != "" {
		left = "search: " + t.search
	}
	footer := panel.Footer{Left: left, Right: status(t, m.clip)}
	content := m.body(t, width, max(height-m.headerRows(), 1), focused)
	if m.headerRows() > 0 {
		content = breadcrumb(t.dir, width) + "\n" + content
	}
	return panel.FrameTabs(panelNumber, tabs, content, width, height, focused, footer)
}

// headerRows is how many rows above the list hold the path: one inside a
// folder, none at the tab root, where the tab name already says where you are.
func (m Model) headerRows() int {
	if m.tabs[m.active].dir == "" {
		return 0
	}
	return 1
}

// listRows is how many list rows fit: the height inside the border less the
// path row, if there is one.
func (m Model) listRows() int { return max(m.innerRows-m.headerRows(), 1) }

// The path is drawn in blue (the color the selected tab once had): the folder
// you are in is bold, the ones above it plain, and the slashes dim.
var (
	pathColor    = lipgloss.Color("4")
	currentStyle = lipgloss.NewStyle().Bold(true).Foreground(pathColor)
	parentStyle  = lipgloss.NewStyle().Foreground(pathColor)
)

// breadcrumb shows the folders below the tab root, such as "alpha/notes/" (the
// tab's own name is in the panel title), in the folder color. When it is wider than width it is cut from the left, so
// the current folder always shows.
func breadcrumb(dir string, width int) string {
	segs := strings.Split(filepath.ToSlash(dir), "/")
	var b strings.Builder
	for i, seg := range segs {
		if i > 0 {
			b.WriteString(dimStyle.Render("/"))
		}
		seg = textsafe.SingleLine(seg)
		if i == len(segs)-1 {
			b.WriteString(currentStyle.Render(seg))
		} else {
			b.WriteString(parentStyle.Render(seg))
		}
	}
	b.WriteString(dimStyle.Render("/")) // a folder, as its row in the list says
	line := b.String()
	if w := ansi.StringWidth(line); w > width && width > 1 {
		line = ansi.TruncateLeft(line, w-width+1, "…")
	}
	return line
}

// status builds the bottom-right text: the visual-mode flag, the selection
// count and the cursor position, e.g. "VISUAL 3 sel 4/9".
func status(t tabState, c clip) string {
	var parts []string
	switch t.visual.mode {
	case visualSelect:
		parts = append(parts, "VISUAL")
	case visualUnselect:
		parts = append(parts, "UNSELECT")
	}
	if n := len(t.selected); n > 0 {
		parts = append(parts, fmt.Sprintf("%d sel", n))
	}
	switch c.mode {
	case clipYank:
		parts = append(parts, fmt.Sprintf("%d yanked", len(c.rels)))
	case clipCut:
		parts = append(parts, fmt.Sprintf("%d cut", len(c.rels)))
	}
	if len(t.rows) > 0 {
		parts = append(parts, fmt.Sprintf("%d/%d", t.cursor+1, len(t.rows)))
	}
	return strings.Join(parts, " ")
}

func (m Model) body(t tabState, width, height int, focused bool) string {
	switch {
	case t.err != nil:
		return errorStyle.Render(ansi.Truncate(t.err.Error(), width, "…"))
	case len(t.rows) == 0:
		return dimStyle.Render("(empty)")
	}

	start, end := panel.ScrollWindow(t.cursor, len(t.rows), height)
	lines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		lines = append(lines, renderRow(t.rows[i], width, i == t.cursor && focused, m.markOf(t, t.rows[i]), m.marks[t.rows[i].uid], t.search))
	}
	return strings.Join(lines, "\n")
}

// renderRow draws one row: a marker column (the bar, or the bullet of a
// marked note), a kind badge (blank for
// directories), the name, and for directories an item count right-aligned.
// cursor highlights the row; mk draws the left-edge bar; marked draws the
// bullet when there is no bar; query, when the row matches it, highlights the matching text in the
// name.
func renderRow(r row, width int, cursor bool, mk mark, marked bool, query string) string {
	if !matches(r.entry, query) {
		query = ""
	}
	bar, barColor := " ", markerColors[mk]
	switch {
	case mk != markNone:
		bar = markerBar
	case marked:
		bar, barColor = bullet, overdueColor
	}
	markerStyle := lipgloss.NewStyle().Foreground(barColor)
	width -= markerWidth

	badge, badgeColor := strings.Repeat(" ", badgeWidth), lipgloss.Color("")
	name := strings.TrimSuffix(r.entry.Name, ".md")
	right := ""
	if r.entry.IsDir {
		name = r.entry.Name + "/"
		right = fmt.Sprint(r.entry.ItemCount)
	} else {
		badge, badgeColor = kindstyle.Badge(r.kind)
	}

	countStyle := dimStyle
	countSel := selStyle
	if r.entry.IsDir && r.entry.ItemCount == 0 {
		countStyle = lipgloss.NewStyle().Foreground(overdueColor)
		countSel = selStyle.Foreground(overdueColor)
	}

	nameWidth := max(width-badgeWidth-1-len(right)-1, 1)
	name = ansi.Truncate(name, nameWidth, "…")
	gap := strings.Repeat(" ", max(width-badgeWidth-1-ansi.StringWidth(name)-len(right), 0))

	if cursor {
		nameSel := selStyle
		if r.overdue {
			nameSel = selStyle.Foreground(overdueColor)
		}
		barSel := selStyle.Foreground(barColor)
		if mk == markNone && marked {
			barSel = nameSel // the bullet matches the name's color
		}
		return barSel.Render(bar) +
			selStyle.Render(badge+" ") + highlight(name, query, nameSel, true) + selStyle.Render(gap) + countSel.Render(right)
	}
	nameStyle := lipgloss.NewStyle()
	switch {
	case r.entry.IsDir:
		nameStyle = dirStyle
	case r.overdue:
		nameStyle = lipgloss.NewStyle().Foreground(overdueColor)
	}
	return markerStyle.Render(bar) + lipgloss.NewStyle().Foreground(badgeColor).Render(badge) + " " +
		highlight(name, query, nameStyle, false) + gap + countStyle.Render(right)
}

// Search highlight backgrounds, as in lazytask: teal on the cursor row so
// the row n and N jump to stands out, yellow on every other matching row.
var (
	matchOnCursor  = lipgloss.Color("6")
	matchElsewhere = lipgloss.Color("3")
)

// highlight renders s with base, drawing each case-insensitive occurrence of
// query with a highlight background. An empty query renders s unchanged.
func highlight(s, query string, base lipgloss.Style, onCursor bool) string {
	lowerS, lowerQ := strings.ToLower(s), strings.ToLower(query)
	if query == "" || len(lowerS) != len(s) || len(lowerQ) != len(query) {
		return base.Render(s)
	}
	bg := matchElsewhere
	if onCursor {
		bg = matchOnCursor
	}
	match := base.Background(bg).Foreground(lipgloss.Color("0")).Bold(false)

	var b strings.Builder
	for i := 0; i < len(s); {
		idx := strings.Index(lowerS[i:], lowerQ)
		if idx < 0 {
			b.WriteString(base.Render(s[i:]))
			break
		}
		idx += i
		if idx > i {
			b.WriteString(base.Render(s[i:idx]))
		}
		b.WriteString(match.Render(s[idx : idx+len(query)]))
		i = idx + len(query)
	}
	return b.String()
}
