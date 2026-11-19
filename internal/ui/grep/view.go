package grep

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/tkilb/para/internal/search"
	"github.com/tkilb/para/internal/ui/filterbar"
	"github.com/tkilb/para/internal/ui/panel"
)

const (
	boxMarginCols = 8
	boxMarginRows = 6
	minWidth      = 60
	maxWidth      = 200
	minHeight     = 10
	maxHeight     = 45
	// leftPercent is the share of the width the list takes.
	leftPercent = 50
	// pathPercent is the most of a list row the path may take.
	pathPercent = 45
	// lead is how many characters before the first match stay visible when
	// a long line is cut at the left.
	lead = 10
	// beforeHit is the share of the preview kept above the highlighted line.
	beforeHit = 3
	noQuery   = "(type to search)"
	noMatch   = "(no matches)"
	hint      = "enter open  tab fields  ctrl+f filters  esc cancel"
)

// The popup's colors: the gruvbox-material palette of the Content panel.
var (
	promptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#e78a4e"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	selBg       = panel.SelectedRowBackground
	hitBg       = lipgloss.Color("238")
	matchFg     = lipgloss.Color("#a9b665")
	markFg      = lipgloss.Color("#d8a657")
)

// dims returns the outer width of the list and the preview and the inner
// height of both, for a terminal screenW x screenH.
func dims(screenW, screenH int) (left, right, inner int) {
	total := min(max(screenW-boxMarginCols, minWidth), maxWidth)
	left = total * leftPercent / 100
	outer := min(max(screenH-boxMarginRows, minHeight), maxHeight)
	return left, total - left, outer - 2 - filterbar.RowHeight
}

// pageRows is how many list rows the popup shows.
func (m Model) pageRows() int {
	_, _, inner := dims(m.screenW, m.screenH)
	return max(inner-2, 1) // the query and the rule above the list
}

// View draws the popup for a terminal screenW x screenH: the space, kind and
// dir filters on top, flush with the panes below them, then the query and the
// lines on the left and the note around the highlighted line on the right.
func (m Model) View(screenW, screenH int) string {
	left, right, inner := dims(screenW, screenH)
	panes := lipgloss.JoinHorizontal(lipgloss.Top,
		m.listPane(left-2, inner),
		m.previewPane(right-2, inner),
	)
	view := lipgloss.JoinVertical(lipgloss.Left, m.bar.Row(left, right), panes)
	return m.bar.Overlay(view, left, right, inner+2)
}

// listPane draws the query, a rule and the matching lines.
func (m Model) listPane(width, height int) string {
	count := ""
	if len(m.res.Hits) > 0 {
		count = fmt.Sprintf("%d/%d", m.cursor+1, m.res.Total)
	}
	m.field.Width = max(width-ansi.StringWidth(m.field.Prompt)-ansi.StringWidth(count)-2, 1)
	query := m.field.View()
	gap := strings.Repeat(" ", max(width-ansi.StringWidth(query)-ansi.StringWidth(count), 0))
	lines := []string{query + gap + dimStyle.Render(count), dimStyle.Render(strings.Repeat("─", width))}

	rows := max(height-len(lines), 1)
	switch {
	case strings.TrimSpace(m.field.Value()) == "":
		lines = append(lines, dimStyle.Render(noQuery))
	case len(m.res.Hits) == 0:
		lines = append(lines, dimStyle.Render(noMatch))
	default:
		start, end := panel.ScrollWindow(m.cursor, len(m.res.Hits), rows)
		for i := start; i < end; i++ {
			lines = append(lines, listRow(m.res.Hits[i], width, i == m.cursor))
		}
	}
	footer := panel.Footer{Left: hint, Tint: true}
	if m.capped {
		footer.Right = "first lines only"
	} else if m.res.Truncated {
		footer.Right = fmt.Sprintf("first %d of %d", len(m.res.Hits), m.res.Total)
	}
	return panel.Frame("Grep", strings.Join(lines, "\n"), width, height, m.bar.Focus() == filterbar.FieldQuery, footer)
}

// listRow draws one hit: the path (folders dim and italic, the note's name
// bold), the line and column, then the line's text with the matches picked
// out, cut so the first match shows.
func listRow(h search.Hit, width int, cursor bool) string {
	base := lipgloss.NewStyle()
	if cursor {
		base = base.Bold(true).Background(selBg)
	}
	dir, name := splitPath(h.Rel)
	pos := fmt.Sprintf(":%d:%d", h.Index+1, h.Col)
	dir, name = fitPath(dir, name, pos, width*pathPercent/100)

	text, spans := cutAtMatch(h.Text, h.Spans, width-ansi.StringWidth(dir+name+pos)-1)
	row := base.Foreground(lipgloss.Color("245")).Italic(true).Render(dir) +
		base.Bold(true).Render(name) +
		base.Foreground(lipgloss.Color("245")).Render(pos) + base.Render(" ") +
		styled(text, spans, base, base.Foreground(matchFg).Bold(true))
	row = ansi.Truncate(row, width, "…")
	return row + base.Render(strings.Repeat(" ", max(width-ansi.StringWidth(row), 0)))
}

// splitPath splits a note's path into its folders (ending in a slash) and
// its name without the extension.
func splitPath(rel string) (dir, name string) {
	rel = filepath.ToSlash(rel)
	dir = rel[:strings.LastIndex(rel, "/")+1]
	return dir, strings.TrimSuffix(rel[len(dir):], filepath.Ext(rel))
}

// fitPath shortens the folders, a whole folder at a time from the left (as
// "…/alpha/"), then the name, so that dir, name and pos take at most budget
// columns.
func fitPath(dir, name, pos string, budget int) (string, string) {
	folders := strings.FieldsFunc(dir, func(r rune) bool { return r == '/' })
	for dropped := 0; ansi.StringWidth(dir+name+pos) > budget && dropped < len(folders); {
		dropped++
		dir = "…/"
		if dropped < len(folders) {
			dir += strings.Join(folders[dropped:], "/") + "/"
		}
	}
	if ansi.StringWidth(dir+name+pos) > budget && len(folders) > 0 {
		dir = ""
	}
	if w := budget - ansi.StringWidth(dir+pos); ansi.StringWidth(name) > w {
		name = ansi.Truncate(name, max(w, 1), "…")
	}
	return dir, name
}

// cutAtMatch shortens text to width columns so that its first match shows:
// when the match would be off the right edge, the start is cut away (leaving
// lead characters before the match) and an ellipsis marks it. The spans are
// moved to match.
func cutAtMatch(text string, spans [][2]int, width int) (string, [][2]int) {
	if len(spans) == 0 || ansi.StringWidth(text[:spans[0][1]]) <= width {
		return text, spans
	}
	runes := utf8.RuneCountInString(text[:spans[0][0]])
	skip := max(runes-lead, 0)
	cut := 0
	for i := 0; i < skip; i++ {
		_, w := utf8.DecodeRuneInString(text[cut:])
		cut += w
	}
	const mark = "…"
	out := make([][2]int, len(spans))
	for i, s := range spans {
		out[i] = [2]int{max(s[0]-cut, 0) + len(mark), max(s[1]-cut, 0) + len(mark)}
	}
	return mark + text[cut:], out
}

// styled draws text in base with the spans in match.
func styled(text string, spans [][2]int, base, match lipgloss.Style) string {
	var b strings.Builder
	at := 0
	for _, s := range spans {
		if s[0] > at {
			b.WriteString(base.Render(text[at:s[0]]))
		}
		b.WriteString(match.Render(text[s[0]:min(s[1], len(text))]))
		at = s[1]
	}
	if at < len(text) {
		b.WriteString(base.Render(text[at:]))
	}
	return b.String()
}

// previewPane draws the note around the highlighted line, with line numbers.
func (m Model) previewPane(width, height int) string {
	h, ok := m.Chosen()
	if !ok {
		return panel.Frame("Preview", dimStyle.Render(""), width, height, false)
	}
	title := filepath.Base(h.Rel)
	n := len(m.preview)
	gutter := len(fmt.Sprint(max(n, 1)))
	textWidth := max(width-gutter-2, 1)

	m.scroll = min(max(m.scroll, -h.Index), n)
	start := h.Index - height/beforeHit + m.scroll
	start = min(max(start, 0), max(n-height, 0))
	rows := make([]string, 0, height)
	for i := start; i < min(start+height, n); i++ {
		rows = append(rows, previewRow(m.preview[i], i, gutter, textWidth, h))
	}
	return panel.Frame(title, strings.Join(rows, "\n"), width, height, false)
}

// previewRow draws line i of the note: its number, then its text. The
// highlighted line has a marker, a background and its matches picked out.
func previewRow(line string, i, gutter, width int, h search.Hit) string {
	hit := i == h.Index
	bg := lipgloss.NewStyle()
	marker := " "
	if hit {
		bg = bg.Background(hitBg)
		marker = lipgloss.NewStyle().Foreground(markFg).Render("▶")
	}
	num := bg.Foreground(lipgloss.Color("245")).Render(fmt.Sprintf("%*d ", gutter, i+1))
	line = strings.ReplaceAll(line, "\t", "    ")

	var body string
	if hit {
		spans := h.Spans
		if off := strings.Index(line, h.Text); off >= 0 && !strings.Contains(h.Text, "\t") {
			shifted := make([][2]int, len(spans))
			for j, s := range spans {
				shifted[j] = [2]int{s[0] + off, s[1] + off}
			}
			spans = shifted
		} else {
			spans = nil
		}
		match := lipgloss.NewStyle().Background(lipgloss.Color("#d8a657")).Foreground(lipgloss.Color("0")).Bold(true)
		body = styled(ansi.Truncate(line, width, ""), clampSpans(spans, width, line), bg.Bold(true), match)
	} else {
		body = ansi.Truncate(line, width, "")
	}
	row := marker + num + body
	return row + bg.Render(strings.Repeat(" ", max(width+gutter+2-ansi.StringWidth(row), 0)))
}

// clampSpans drops spans that fall outside the first width columns of line.
func clampSpans(spans [][2]int, width int, line string) [][2]int {
	shown := len(ansi.Truncate(line, width, ""))
	var out [][2]int
	for _, s := range spans {
		if s[0] < shown {
			out = append(out, [2]int{s[0], min(s[1], shown)})
		}
	}
	return out
}
