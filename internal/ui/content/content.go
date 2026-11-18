// Package content implements the Content panel: a read-only plain-text view of the
// selected note, with its frontmatter hidden and long lines wrapped to the
// panel width. A cursor line keeps your place, and whole lines can be
// selected, as in PARA (space picks a line, v and V select or unselect a
// range), for extracting them into a new note. A wrapped line is one line.
package content

import (
	"fmt"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/tkilb/para/internal/textsafe"
	"github.com/tkilb/para/internal/ui/panel"
)

// dirColor is the color of a folder name, as in PARA.
var dirColor = lipgloss.Color("6")

// ListItem is one entry of a folder listing: a badge (blank for a folder) in
// the kind's color, then the name.
type ListItem struct {
	Badge string
	Color lipgloss.Color
	Name  string
	Dir   bool
}

const (
	// defaultHeight is the page size until the first Resize.
	defaultHeight = 10
)

var (
	dimStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	errorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	// cursorStyle is the cursor line, as the cursor row in a list panel;
	// pickStyle a selected line; both together use the teal of the search
	// match under the cursor.
	cursorStyle = lipgloss.NewStyle().Bold(true).Background(panel.SelectedRowBackground)
	pickStyle   = lipgloss.NewStyle().Background(lipgloss.Color("3")).Foreground(lipgloss.Color("0"))
	bothStyle   = lipgloss.NewStyle().Bold(true).Background(lipgloss.Color("6")).Foreground(lipgloss.Color("0"))
)

// visualMode says whether a range is being selected, unselected, or neither.
type visualMode int

const (
	visualOff visualMode = iota
	visualSelect
	visualUnselect
)

// visualState anchors a visual-mode range. While it is active, the selection
// is base plus (or minus) every line between anchor and the cursor.
type visualState struct {
	mode   visualMode
	anchor int
	base   map[int]bool
}

// Preview is a picture for Content to show in place of text. Render draws it
// into a box of cols x rows cells, one string per row. Cleanup is written
// before the next thing shown, to remove a picture the terminal still holds.
type Preview struct {
	Render  func(cols, rows int) []string
	Caption string
	Cleanup string
}

// Model is the Content panel state. Methods return updated copies.
type Model struct {
	// rel identifies the note shown, so switching notes resets the scroll
	// while reloading the same note keeps it.
	rel  string
	text string // sanitized, unwrapped
	err  error

	// srcLines are the lines of text as written. lines are those wrapped to
	// the width, srcOf maps each wrapped row to its source line, and
	// firstRow and lastRow give the rows each source line covers.
	// listing is set while the panel shows a folder's entries; item i styles
	// source line i.
	listing []ListItem
	// rendered is the note drawn as Markdown, one string per row, and
	// roffset the first row shown. It is read-only: the cursor and selection
	// belong to the plain view. plain is the reader's choice of the plain
	// view (the m key); kicked says a selection key left the rendered view
	// for the note shown, and is reset when another note is loaded.
	rendered []string
	roffset  int
	// rcursor is the cursor row of the rendered view, which keeps your place
	// in a long note. It is not a selection.
	rcursor int
	// front holds the note's frontmatter lines, fences included, while they
	// are shown above the text (zoomed Content); the source lines are then front
	// followed by the text. frontRows are the same lines as rendered rows, with
	// a blank row after, and frontSrc the source line of each of those rows.
	front     []string
	frontRows []string
	frontSrc  []int
	plain     bool
	kicked    bool
	// kinds is the color kind of each source line in the plain view.
	kinds             []lineKind
	srcLines          []string
	lines             []string
	srcOf             []int
	firstRow, lastRow []int

	offset int // index of the first visible row
	cursor int // the cursor's source line
	// picked holds the selected source lines when no range is open. It is
	// replaced, never changed in place.
	picked map[int]bool
	visual visualState
	// yanked is how many lines the last yank copied, shown in the footer
	// until the next key press.
	yanked int
	// places remembers the cursor line of each note shown this session, so
	// coming back to a note puts you where you were.
	places map[string]int

	width, height int

	// preview is the picture shown in place of text, and previewRows its rows
	// at the current size. staleImage is what to write to remove the
	// picture once something else is shown.
	preview     *Preview
	previewRows []string
	staleImage  string

	// covered hides a picture while a popup is open: a terminal image draws
	// over any text, so it would hide the popup.
	covered bool

	// tabLabels are the tabs drawn in the title, and tabActive the one shown.
	// They belong to the caller, so they survive a reload.
	tabLabels []panel.TabLabel
	tabActive int
}

// New returns an empty panel.
func New() Model {
	return Model{height: defaultHeight, places: map[string]int{}}
}

// Resize sets the content area size, re-wrapping when the width changed.
func (m Model) Resize(width, height int) Model {
	rewrap := width != m.width
	resized := rewrap || max(height, 1) != m.height
	m.width, m.height = width, max(height, 1)
	if m.preview != nil {
		if resized {
			m.previewRows = m.preview.Render(m.width, m.height)
		}
		return m
	}
	if rewrap {
		m = m.fromRendered().wrap()
	}
	m = m.showRendered()
	return m.showCursor()
}

// Load shows text for the note at rel, without the blank lines around it.
// Reloading the note already shown keeps the cursor (and the selection, while
// the text is unchanged); another note starts where you left it last time, or
// at the top.
func (m Model) Load(rel, text string) Model {
	return m.load(rel, nil, text, nil)
}

// LoadNote is Load with the note's frontmatter lines (fences included) to
// show above the text, or none.
func (m Model) LoadNote(rel string, front []string, text string) Model {
	return m.load(rel, front, text, nil)
}

// load is Load for text that may be a folder listing (items not nil).
func (m Model) load(rel string, front []string, text string, items []ListItem) Model {
	m = m.dropPreview()
	text = textsafe.Sanitize(strings.Trim(text, "\r\n"))
	front = cleanFront(front)
	unchanged := rel == m.rel && text == m.text && slices.Equal(front, m.front) && m.err == nil && items == nil && m.listing == nil && m.rendered != nil
	if rel == m.rel && text != m.text {
		m = m.fromRendered() // keep the place in the note while its text changes
	}
	if rel != m.rel {
		m = m.remember()
		m.cursor, m.offset = m.places[rel]+len(front), 0
		m.roffset, m.kicked = 0, false
		m.picked, m.visual = nil, visualState{}
	} else if text != m.text {
		m.picked, m.visual = nil, visualState{}
	}
	m.rel, m.err, m.text, m.listing, m.front = rel, nil, text, items, front
	if !unchanged {
		m = m.wrap()
	}
	m.cursor = min(m.cursor, max(len(m.srcLines)-1, 0))
	return m.showCursor()
}

// LoadListing shows the entries of the folder at rel, one per line: the
// badge in its color, then the name, which is cyan for a folder.
func (m Model) LoadListing(rel string, items []ListItem) Model {
	lines := make([]string, len(items))
	clean := make([]ListItem, len(items))
	for i, it := range items {
		it.Name = textsafe.SingleLine(it.Name)
		if it.Dir {
			it.Name += "/"
		}
		clean[i] = it
		lines[i] = it.Badge + " " + it.Name
	}
	return m.load(rel, nil, strings.Join(lines, "\n"), clean)
}

// Fail shows err in place of the note at rel.
func (m Model) Fail(rel string, err error) Model {
	m = m.dropPreview()
	m = m.remember()
	m = m.reset()
	m.rel, m.err = rel, err
	return m
}

// Clear empties the panel.
func (m Model) Clear() Model {
	m = m.remember()
	return m.reset()
}

// Renamed moves the remembered place of the note at oldRel to newRel, so a
// note keeps your place when it is renamed. Call it before the panel loads
// the note under its new name.
func (m Model) Renamed(oldRel, newRel string) Model {
	m = m.remember()
	if place, ok := m.places[oldRel]; ok {
		m.places[newRel] = place
		delete(m.places, oldRel)
	}
	if m.rel == oldRel {
		m.rel = newRel
	}
	return m
}

// remember keeps the cursor of the note shown for later.
func (m Model) remember() Model {
	m = m.fromRendered()
	if m.rel != "" && m.err == nil {
		m.places[m.rel] = max(m.cursor-len(m.front), 0) // counted from the text, not the frontmatter
	}
	return m
}

// reset drops everything shown, keeping the size and the remembered places.
func (m Model) reset() Model {
	m = m.dropPreview()
	return Model{width: m.width, height: m.height, places: m.places, plain: m.plain, staleImage: m.staleImage,
		tabLabels: m.tabLabels, tabActive: m.tabActive}
}

// Covered returns the panel set to leave its picture out of the next View,
// and to remove one the terminal still holds. Use it on the copy drawn while a
// popup is open; the panel's state is not changed.
func (m Model) Covered() Model {
	m.covered = true
	return m
}

// SetTabs shows labels as tabs in the panel title, with the one at active
// highlighted, or no tabs when labels is empty. The caller decides what each
// tab shows; the panel only draws them.
func (m Model) SetTabs(labels []panel.TabLabel, active int) Model {
	m.tabLabels, m.tabActive = labels, active
	return m
}

// LoadPreview shows a picture for rel in place of text. A picture shown
// before is replaced.
func (m Model) LoadPreview(rel string, p Preview) Model {
	m = m.remember()
	m = m.reset()
	m.rel, m.preview = rel, &p
	m.staleImage = ""
	m.previewRows = p.Render(m.width, m.height)
	return m
}

// dropPreview forgets the picture shown, if any, and remembers what removes
// it from the terminal.
func (m Model) dropPreview() Model {
	if m.preview == nil {
		return m
	}
	if m.preview.Cleanup != "" {
		m.staleImage = m.preview.Cleanup
	}
	m.rel, m.preview, m.previewRows = "", nil, nil
	return m
}

// SetFront shows the given frontmatter lines (fences included) above the text,
// or none, for the note already shown, keeping your place in the text. Any
// selection is cleared, because line numbers move.
func (m Model) SetFront(front []string) Model {
	front = cleanFront(front)
	if slices.Equal(front, m.front) {
		return m
	}
	m = m.fromRendered()
	m.cursor = max(m.cursor-len(m.front), 0) + len(front)
	m.picked, m.visual = nil, visualState{}
	m.front = front
	if m.rel == "" || m.err != nil || m.listing != nil {
		return m
	}
	m = m.wrap()
	m.cursor = min(m.cursor, max(len(m.srcLines)-1, 0))
	return m.showCursor()
}

// Rel returns the note shown, or "" when the panel is empty.
func (m Model) Rel() string { return m.rel }

// wrap lays text out for the current width.
func (m Model) wrap() Model {
	m.lines, m.srcLines, m.srcOf, m.firstRow, m.lastRow, m.rendered, m.kinds = nil, nil, nil, nil, nil, nil, nil
	m.frontRows, m.frontSrc = nil, nil
	if (m.text == "" && len(m.front) == 0) || m.width <= 0 {
		return m
	}
	var body []string
	if m.text != "" {
		body = strings.Split(m.text, "\n")
	}
	m.srcLines = append(append([]string(nil), m.front...), body...)
	if m.listing == nil {
		m.kinds = append(frontKinds(m.front), classify(body)...)
	}
	if !m.plain && m.listing == nil {
		m.frontRows, m.frontSrc = renderFront(m.front, m.width)
		m.rendered = append(append([]string(nil), m.frontRows...), renderMarkdown(m.text, m.width)...)
		if len(m.rendered) == 0 {
			m.rendered = nil
		}
		m.rcursor = m.rowForSrc(m.cursor)
		m = m.showRendered()
	}
	for i, line := range m.srcLines {
		m.firstRow = append(m.firstRow, len(m.lines))
		for _, row := range strings.Split(ansi.Wrap(line, m.width, ""), "\n") {
			m.lines = append(m.lines, row)
			m.srcOf = append(m.srcOf, i)
		}
		m.lastRow = append(m.lastRow, len(m.lines)-1)
	}
	return m
}

// clamp keeps offset within the scrollable range.
func (m Model) clamp(offset int) int {
	return min(max(offset, 0), max(len(m.lines)-m.height, 0))
}

// showCursor scrolls so the cursor line is in view (its first row, when it is
// taller than the panel).
func (m Model) showCursor() Model {
	if len(m.firstRow) == 0 {
		m.offset = 0
		return m
	}
	first, last := m.firstRow[m.cursor], m.lastRow[m.cursor]
	switch {
	case first < m.offset:
		m.offset = first
	case last >= m.offset+m.height:
		m.offset = max(last-m.height+1, first)
	}
	m.offset = m.clamp(m.offset)
	return m
}

// moveTo puts the cursor on source line i, clamped, and scrolls to it.
func (m Model) moveTo(i int) Model {
	if len(m.srcLines) == 0 {
		return m
	}
	m.cursor = min(max(i, 0), len(m.srcLines)-1)
	return m.showCursor()
}

// halfPage is how many rows ctrl+u and ctrl+d move: half the panel, as in vim.
func (m Model) halfPage() int { return max(m.height/2, 1) }

// moveRows moves the cursor to the line shown delta rows away from its first
// row, for page movement.
func (m Model) moveRows(delta int) Model {
	if len(m.firstRow) == 0 {
		return m
	}
	row := min(max(m.firstRow[m.cursor]+delta, 0), len(m.srcOf)-1)
	return m.moveTo(m.srcOf[row])
}

// isSelected reports whether source line i is selected, counting an open
// range.
func (m Model) isSelected(i int) bool {
	if m.visual.mode == visualOff {
		return m.picked[i]
	}
	lo, hi := min(m.visual.anchor, m.cursor), max(m.visual.anchor, m.cursor)
	if i >= lo && i <= hi {
		return m.visual.mode == visualSelect
	}
	return m.visual.base[i]
}

// selection returns the selected source lines in order.
func (m Model) selection() []int {
	var out []int
	for i := range m.srcLines {
		if m.isSelected(i) {
			out = append(out, i)
		}
	}
	return out
}

// commit makes the open range part of the selection and closes it.
func (m Model) commit() Model {
	if m.visual.mode == visualOff {
		return m
	}
	picked := map[int]bool{}
	for _, i := range m.selection() {
		picked[i] = true
	}
	m.picked, m.visual = picked, visualState{}
	return m
}

// HasSelection reports whether any line is selected.
func (m Model) HasSelection() bool { return len(m.selection()) > 0 }

// Active reports whether Escape has something to close: an open range or a
// selection.
func (m Model) Active() bool { return m.visual.mode != visualOff || len(m.picked) > 0 }

// Escape closes an open range keeping what it selected, and otherwise clears
// the selection.
func (m Model) Escape() Model {
	if m.visual.mode != visualOff {
		return m.commit()
	}
	return m.ClearSelection()
}

// ClearSelection drops the selection and any open range.
func (m Model) ClearSelection() Model {
	m.picked, m.visual = nil, visualState{}
	return m
}

// SelectedText returns the selected lines in file order, as shown (control
// characters already removed). Lines that were not next to each other are
// separated by a blank line, so the gaps stay visible. It is "" when nothing
// is selected.
func (m Model) SelectedText() string {
	var groups []string
	var run []string
	prev := -2
	for _, i := range m.selection() {
		if i != prev+1 && len(run) > 0 {
			groups = append(groups, strings.Join(run, "\n"))
			run = nil
		}
		run = append(run, m.srcLines[i])
		prev = i
	}
	if len(run) > 0 {
		groups = append(groups, strings.Join(run, "\n"))
	}
	return strings.Join(groups, "\n\n")
}

// groupCount is the number of runs of adjacent selected lines.
func (m Model) groupCount() int {
	groups, prev := 0, -2
	for _, i := range m.selection() {
		if i != prev+1 {
			groups++
		}
		prev = i
	}
	return groups
}

// YankText returns what a yank copies and how many lines that is: the
// selection, or the cursor line when nothing is selected.
func (m Model) YankText() (text string, lines int) {
	if m.HasSelection() {
		return m.SelectedText(), len(m.selection())
	}
	if len(m.srcLines) == 0 {
		return "", 0
	}
	return m.srcLines[m.cursor], 1
}

// Yanked records that n lines were copied: the selection is cleared, as a
// yank does in PARA, and the footer says so until the next key press.
func (m Model) Yanked(n int) Model {
	m = m.ClearSelection()
	m.yanked = n
	return m
}

// toggleVisual opens a range in mode, or closes it when it is already open in
// that mode (the other mode switches).
func (m Model) toggleVisual(mode visualMode) Model {
	switch m.visual.mode {
	case mode:
		return m.commit()
	case visualOff:
		m.visual = visualState{mode: mode, anchor: m.cursor, base: m.picked}
	default:
		m.visual.mode = mode
	}
	return m
}

// Update handles a key press while the panel has focus.
func (m Model) Update(msg tea.KeyMsg) Model {
	m.yanked = 0
	if m.Rendered() {
		return m.updateRendered(msg)
	}
	switch msg.String() {
	case "up":
		return m.moveTo(m.cursor - 1)
	case "down":
		return m.moveTo(m.cursor + 1)
	case "ctrl+u":
		return m.moveRows(-m.halfPage())
	case "ctrl+d":
		return m.moveRows(m.halfPage())
	case "pgup":
		return m.moveRows(-m.height)
	case "pgdown":
		return m.moveRows(m.height)
	case "g", "home":
		return m.moveTo(0)
	case "G", "end":
		return m.moveTo(len(m.srcLines) - 1)
	case " ":
		if m.visual.mode != visualOff || len(m.srcLines) == 0 {
			return m
		}
		picked := make(map[int]bool, len(m.picked)+1)
		for i := range m.picked {
			picked[i] = true
		}
		if picked[m.cursor] {
			delete(picked, m.cursor)
		} else {
			picked[m.cursor] = true
		}
		m.picked = picked
		return m.moveTo(m.cursor + 1)
	case "m": // back to the reading view; a selection cannot go with it
		if m.listing != nil {
			return m
		}
		m = m.ClearSelection()
		m.plain, m.kicked = false, false
		return m.wrap()
	case "{", "}":
		return m.moveTo(paragraph(m.cursor, len(m.srcLines), braceDir(msg.String()), m.blankLine))
	case "v":
		return m.toggleVisual(visualSelect)
	case "V":
		return m.toggleVisual(visualUnselect)
	case "ctrl+a":
		picked := make(map[int]bool, len(m.srcLines))
		for i := range m.srcLines {
			picked[i] = true
		}
		m.picked, m.visual = picked, visualState{}
		return m
	}
	return m
}

// View renders the panel into outerWidth x outerHeight, border included,
// titled "Content" whether it shows a note or a folder.
func (m Model) View(outerWidth, outerHeight int, focused bool) string {
	width, height := panel.InnerSize(outerWidth, outerHeight)

	var footer panel.Footer
	var content string
	switch {
	case m.err != nil:
		content = errorStyle.Render(ansi.Truncate(m.err.Error(), width, "…"))
	case m.preview != nil && m.covered:
		content = m.preview.Cleanup
		footer.Left = m.preview.Caption
	case m.preview != nil:
		content = strings.Join(m.previewRows, "\n")
		footer.Left = m.preview.Caption
	case m.rel != "" && len(m.lines) == 0:
		content = dimStyle.Render("(empty note)")
	case m.Rendered():
		end := min(m.roffset+height, len(m.rendered))
		rows := make([]string, 0, end-min(m.roffset, end))
		for i := min(m.roffset, end); i < end; i++ {
			rows = append(rows, m.renderedRow(i, width, focused))
		}
		content = strings.Join(rows, "\n")
		if len(m.rendered) > height {
			footer.Right = fmt.Sprintf("%d%%", min(((m.rcursor+1)*100)/len(m.rendered), 100))
		}
	default:
		end := min(m.offset+height, len(m.lines))
		rows := make([]string, 0, end-min(m.offset, end))
		for i := min(m.offset, end); i < end; i++ {
			rows = append(rows, m.row(i, width, focused))
		}
		content = strings.Join(rows, "\n")
		if len(m.lines) > height {
			footer.Right = fmt.Sprintf("%d/%d", m.cursor+1, len(m.srcLines))
		}
		footer.Left = m.status()
	}
	// The cleanup goes on the first row, which changed when the picture left,
	// so the terminal removes the picture as that row is drawn.
	content = m.staleImage + content
	if len(m.tabLabels) > 0 {
		tabs := panel.FitTabs(m.tabLabels, m.tabActive, width)
		return panel.FrameFittedTabs('0', tabs, content, width, height, focused, footer)
	}
	return panel.Frame("0 Content", content, width, height, focused, footer)
}

// status is the bottom-left text: the range mode and the selection size.
func (m Model) status() string {
	var parts []string
	if m.yanked > 0 {
		parts = append(parts, fmt.Sprintf("%d yanked", m.yanked))
	}
	switch m.visual.mode {
	case visualSelect:
		parts = append(parts, "VISUAL")
	case visualUnselect:
		parts = append(parts, "UNSELECT")
	}
	if n := len(m.selection()); n > 0 {
		sel := fmt.Sprintf("%d sel", n)
		if g := m.groupCount(); g > 1 {
			sel += fmt.Sprintf(" in %d groups", g)
		}
		parts = append(parts, sel)
	}
	return strings.Join(parts, " ")
}

// row draws wrapped row i: the cursor line is highlighted while the panel has
// focus, and selected lines always are.
func (m Model) row(i, width int, focused bool) string {
	src := m.srcOf[i]
	cursor, selected := focused && src == m.cursor, m.isSelected(src)
	if !cursor && !selected {
		return m.styled(i)
	}
	text := m.lines[i] + strings.Repeat(" ", max(width-ansi.StringWidth(m.lines[i]), 0))
	switch {
	case cursor && selected:
		return bothStyle.Render(text)
	case cursor:
		return cursorStyle.Render(text)
	}
	return pickStyle.Render(text)
}

// styled draws wrapped row i in its colors: a folder listing by entry, a note
// in the plain view by the kind of its line (see linecolor.go).
func (m Model) styled(i int) string {
	line, src := m.lines[i], m.srcOf[i]
	if m.listing == nil {
		if src < len(m.kinds) {
			return colorRow(line, m.kinds[src])
		}
		return line
	}
	if src >= len(m.listing) {
		return line
	}
	it := m.listing[src]
	name := lipgloss.NewStyle()
	if it.Dir {
		name = name.Foreground(dirColor)
	}
	if i != m.firstRow[src] {
		return name.Render(line) // a wrapped continuation has no badge
	}
	w := ansi.StringWidth(it.Badge)
	return lipgloss.NewStyle().Foreground(it.Color).Render(ansi.Truncate(line, w, "")) +
		name.Render(ansi.TruncateLeft(line, w, ""))
}

// RowAt returns the source line drawn at content row y (0 is the first row
// under the top border). It reports false past the last row.
func (m Model) RowAt(y int) (int, bool) {
	if m.Rendered() {
		return 0, false // a rendered row has no source line to pick
	}
	row := m.offset + y
	if y < 0 || y >= m.height || row >= len(m.srcOf) {
		return 0, false
	}
	return m.srcOf[row], true
}

// SelectRow puts the cursor on source line i, as moving with the arrow keys
// does.
func (m Model) SelectRow(i int) Model {
	m.yanked = 0
	if m.Rendered() {
		m.kicked = true
	}
	return m.moveTo(i)
}
