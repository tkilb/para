// Package popup provides the blocking-modal widgets: notification popups
// that surface errors, warnings and information (queued, shown one at a
// time and dismissed by any key), yes/no confirmation boxes, and a text
// input box. Ported from lazytask.
package popup

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Severity controls a popup's border and title color and label.
type Severity int

const (
	Info Severity = iota
	Warning
	Error
)

// String returns the label shown in the popup's title bar.
func (s Severity) String() string {
	switch s {
	case Warning:
		return "Warning"
	case Error:
		return "Error"
	default:
		return "Info"
	}
}

func (s Severity) color() lipgloss.Color {
	switch s {
	case Warning:
		return lipgloss.Color("214") // orange
	case Error:
		return lipgloss.Color("196") // red
	default:
		return lipgloss.Color("39") // blue
	}
}

// Message is a single queued popup.
type Message struct {
	Severity Severity
	Text     string
}

// Model holds a FIFO queue of pending popup messages. The zero value is a
// ready-to-use empty queue.
type Model struct {
	queue []Message
}

// Push enqueues msg, returning the updated model.
func (m Model) Push(msg Message) Model {
	m.queue = append(append([]Message{}, m.queue...), msg)
	return m
}

// Active reports whether a popup is pending display.
func (m Model) Active() bool { return len(m.queue) > 0 }

// Current returns the front-of-queue message, if any.
func (m Model) Current() (Message, bool) {
	if len(m.queue) == 0 {
		return Message{}, false
	}
	return m.queue[0], true
}

// Dismiss removes the front-of-queue message, revealing the next one.
func (m Model) Dismiss() Model {
	if len(m.queue) == 0 {
		return m
	}
	m.queue = m.queue[1:]
	return m
}

const (
	minBoxWidth = 30
	maxBoxWidth = 60

	dismissHint = "press any key to dismiss"
	confirmHint = "(y/enter) confirm  (n/esc) cancel"
	inputHint   = "(enter) accept  (esc) cancel"

	// boxChrome is the columns a box spends on its border and padding.
	boxChrome = 4
	// titleLead is how many border characters come before a title.
	titleLead = 2
	// boxPadding is the columns of padding inside a box, left and right.
	boxPadding = 2
	// hintTrail is how many border characters follow a right-aligned hint.
	hintTrail = 1
)

var (
	// confirmColor is orange, the same as Warning.
	confirmColor = lipgloss.Color("214")
	// dangerColor is the red of Error popups, for confirmations that are
	// hard or impossible to undo.
	dangerColor = Error.color()
	inputColor  = lipgloss.Color("39")
)

// Box draws msg as a bordered box with a title colored per severity and a
// dismiss hint at the bottom, sized to fit within screenWidth.
func Box(msg Message, screenWidth int) string {
	return renderBox(msg.Severity.String(), msg.Severity.color(), dismissHint, msg.Text, screenWidth)
}

// ConfirmBox draws a yes/no confirmation box.
func ConfirmBox(text string, screenWidth int) string {
	return renderBox("Confirm", confirmColor, confirmHint, text, screenWidth)
}

// DangerConfirmBox draws a yes/no box in red, for actions that cannot be
// undone, so the elevated risk is obvious.
func DangerConfirmBox(text string, screenWidth int) string {
	return renderBox("Confirm", dangerColor, confirmHint, text, screenWidth)
}

// InputBox draws a box holding a label and an already rendered input field.
// An empty label leaves the field alone on one line.
func InputBox(title, label, field string, screenWidth int) string {
	body := field
	if label != "" {
		body = label + "\n" + field
	}
	return renderBox(title, inputColor, inputHint, body, screenWidth)
}

// FieldWidth is the width to give a text field shown in an InputBox on a
// screen screenWidth wide.
func FieldWidth(screenWidth int) int {
	return max(boxWidth(screenWidth)-boxChrome-1, 1)
}

func boxWidth(screenWidth int) int {
	w := maxBoxWidth
	if screenWidth > 0 && screenWidth-4 < w {
		w = screenWidth - 4
	}
	return max(w, minBoxWidth)
}

// renderBox lays out a bordered box with its colored title embedded in the
// top border line and its key hint in the bottom one, as the panels do,
// around the body text.
func renderBox(titleText string, color lipgloss.Color, hintText, bodyText string, screenWidth int) string {
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderTop(false).
		BorderBottom(false).
		BorderForeground(color).
		Padding(0, 1).
		Width(boxWidth(screenWidth)).
		Render(bodyText)
	width := ansi.StringWidth(strings.SplitN(box, "\n", 2)[0])
	return titleBorder(titleText, color, width) + "\n" + box + "\n" + hintBorder(hintText, color, width)
}

// titleBorder is the top border line of a box width columns wide, with the
// title after two border characters, as in a panel: "╭──Title─────╮".
func titleBorder(title string, color lipgloss.Color, width int) string {
	b := lipgloss.RoundedBorder()
	lead := b.TopLeft + strings.Repeat(b.Top, titleLead)
	room := max(width-ansi.StringWidth(lead)-ansi.StringWidth(b.TopRight), 0)
	title = ansi.Truncate(title, room, "…")
	fill := strings.Repeat(b.Top, max(room-ansi.StringWidth(title), 0))

	line := lipgloss.NewStyle().Foreground(color)
	label := lipgloss.NewStyle().Bold(true).Foreground(color).Render(title)
	return line.Render(lead) + label + line.Render(fill+b.TopRight)
}

// hintBorder is the bottom border line of a box width columns wide, with the
// key hint right-aligned, one border character from the corner, in the
// border's color: "╰────────(y) confirm─╯".
func hintBorder(hint string, color lipgloss.Color, width int) string {
	b := lipgloss.RoundedBorder()
	room := max(width-ansi.StringWidth(b.BottomLeft)-ansi.StringWidth(b.BottomRight)-hintTrail, 0)
	hint = ansi.Truncate(hint, room, "…")
	fill := strings.Repeat(b.Bottom, max(room-ansi.StringWidth(hint), 0))

	line := lipgloss.NewStyle().Foreground(color)
	return line.Render(b.BottomLeft + fill + hint + strings.Repeat(b.Bottom, hintTrail) + b.BottomRight)
}

// Overlay composites box on top of background, centered within a
// screenWidth x screenHeight area, keeping whatever of background falls
// outside the box. Both may contain ANSI styling; splicing is ANSI- and
// wide-rune-aware so escape sequences are not torn.
func Overlay(background, box string, screenWidth, screenHeight int) string {
	if screenWidth <= 0 || screenHeight <= 0 {
		return box
	}

	bgLines := strings.Split(background, "\n")
	for len(bgLines) < screenHeight {
		bgLines = append(bgLines, "")
	}

	boxLines := strings.Split(box, "\n")
	width := 0
	for _, l := range boxLines {
		width = max(width, ansi.StringWidth(l))
	}

	startRow := max((screenHeight-len(boxLines))/2, 0)
	startCol := max((screenWidth-width)/2, 0)

	out := make([]string, len(bgLines))
	copy(out, bgLines)

	for i, boxLine := range boxLines {
		row := startRow + i
		if row >= len(out) {
			continue
		}
		bgLine := out[row]
		if w := ansi.StringWidth(bgLine); w < screenWidth {
			bgLine += strings.Repeat(" ", screenWidth-w)
		}
		if w := ansi.StringWidth(boxLine); w < width {
			boxLine += strings.Repeat(" ", width-w)
		}
		out[row] = ansi.Cut(bgLine, 0, startCol) + boxLine + ansi.Cut(bgLine, startCol+width, screenWidth)
	}
	return strings.Join(out, "\n")
}
