package content

import (
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/x/ansi"
)

// renderMarkdown renders text as Markdown with glamour in the gruvbox style, wrapped to width, and
// returns one string per screen row. The result has no margins. It is for
// reading only: rendering rewrites and reflows lines, so a row cannot be
// traced to a source line, and anything that picks lines uses the plain view.
// Single newlines in the source stay line breaks, as in a note typed line by line.
// It returns nil when the text cannot be rendered.
func renderMarkdown(text string, width int) []string {
	if width <= 0 || strings.TrimSpace(text) == "" {
		return nil
	}
	r, err := glamour.NewTermRenderer(glamour.WithStyles(gruvboxStyle()), glamour.WithWordWrap(width), glamour.WithPreservedNewLines())
	if err != nil {
		return nil
	}
	out, err := r.Render(text)
	if err != nil {
		return nil
	}
	rows := strings.Split(strings.Trim(out, "\n"), "\n")
	for i, row := range rows {
		// Glamour pads every row with styled spaces; keep only what shows.
		shown := ansi.StringWidth(strings.TrimRight(ansi.Strip(row), " "))
		rows[i] = ansi.Truncate(row, min(shown, width), "")
	}
	return rows
}
