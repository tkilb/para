// Package noteform implements the Add Note form: a Title, an optional
// Template chooser, a multi-line Body, Kind and Source choosers, a Tags field
// and a People field, each in its own bordered box with the
// field name in the border and the focused box highlighted, as in lazytask's
// add-task form. Tab and Shift+Tab move between the fields (not the arrows,
// which the body's text area needs for its cursor). The body scrolls within a
// fixed height, and Left and Right change the kind or source.
package noteform

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/tkilb/para/internal/store"
	"github.com/tkilb/para/internal/ui/kindstyle"
	"github.com/tkilb/para/internal/ui/panel"
)

// Field identifies one of the form's inputs, in tab order.
type Field int

const (
	// FieldTitle is the single-line title.
	FieldTitle Field = iota
	// FieldTemplate is the template chooser, present only when the form was
	// given templates (see WithTemplates).
	FieldTemplate
	// FieldBody is the multi-line body.
	FieldBody
	// FieldKind is the kind chooser.
	FieldKind
	// FieldSource is the source chooser.
	FieldSource
	// FieldTags is the single-line tag list.
	FieldTags
	// FieldPeople is the single-line, comma separated list of people.
	FieldPeople

	fieldCount
)

const (
	minWidth     = 40
	maxWidth     = 70
	screenMargin = 8 // columns and rows kept free around the form
	lineBoxRows  = 3 // a one-line box: its line plus the border
	boxChrome    = 2 // a box's top and bottom border
	lineBoxes    = 5 // the title, kind, source, tags and people boxes
	minBodyRows  = 3 // visible body lines, at the smallest
	maxBodyRows  = 8 // and at the largest
	titleLimit   = 120
	tagsLimit    = 200
	peopleLimit  = 200
	bodyLimit    = 20000
	cursorCell   = 1 // the cell a text box's cursor needs beyond its width

	titlePlaceholder  = "required"
	bodyPlaceholder   = "optional; enter starts a new line"
	tagsPlaceholder   = "optional; comma or space separated"
	peoplePlaceholder = "optional; comma separated, such as Tyler K., Amber D."
	noKindMeaning     = "No kind yet"
	noTemplate        = "none"
)

// hints are the key hints for each focused field, shown on the bottom border
// of the last box.
var hints = [fieldCount]string{
	FieldTitle:    "(tab/↓) next  (enter) save  (esc) cancel",
	FieldTemplate: "(type) jump  (←/→) change  (⌫) none  (↑/↓) move  (enter) save  (esc) cancel",
	FieldBody:     "(tab, or ↑/↓ at the ends) move  (ctrl+s) save  (esc) cancel",
	FieldKind:     "(type) jump  (←/→) change  (⌫) clear  (↑/↓) move  (enter) save  (esc) cancel",
	FieldSource:   "(type) jump  (←/→) change  (⌫) first  (↑/↓) move  (enter) save  (esc) cancel",
	FieldTags:     "(tab/↑↓) move  (enter) save  (esc) cancel",
	FieldPeople:   "(tab/↑↓) move  (enter) save  (esc) cancel",
}

// Values are what the user entered.
type Values struct {
	Title, Body string
	// Kind is KindUnset when none was chosen.
	Kind store.Kind
	// Source is the chosen source.
	Source string
	// Tags is the tags text as typed, to be parsed by the caller.
	Tags string
	// People is the people text as typed, to be parsed by the caller.
	People string
	// Template is the chosen template's name, "" for none.
	Template string
}

// Fill is what a template pre-fills. Kind, Source and Tags that are zero
// leave the form's own values alone.
type Fill struct {
	Body   string
	Kind   store.Kind
	Source string
	Tags   []string
}

// Model is the form state. Methods return updated copies.
type Model struct {
	name    string // heads the title box, e.g. "Add Note"
	title   textinput.Model
	body    textarea.Model
	tags    textinput.Model
	people  textinput.Model
	kinds   []store.Kind // unset first, then the six kinds
	kindIdx int
	// kindTyped is the letters typed in the Kind field to jump to a kind;
	// it is cleared when the focus moves.
	kindTyped string
	// tmplTyped is the same for the Template chooser.
	tmplTyped string
	// srcTyped is the same for the Source chooser.
	srcTyped string
	sources  []string
	srcIdx   int
	focused  Field
	width    int
	// templates are the names the Template chooser offers after "none"; the
	// chooser is hidden when there are none. filled is the body a template
	// last put in, to tell whether the user has edited it since.
	templates []string
	tmplIdx   int
	filled    string
	// screenWidth and screenHeight are the terminal size of the last Resize.
	screenWidth, screenHeight int
	// bodyRows is how many body lines show at once.
	bodyRows int
	// initial is what the form held when New made it, to tell whether the
	// user has changed anything (see Dirty).
	initial Values
}

// New returns an empty form named name ("Add Note") offering sources, the
// first of which is the default, sized for a screenWidth x screenHeight
// terminal, with the title focused. The command blinks the cursor.
func New(name string, sources []string, screenWidth, screenHeight int) (Model, tea.Cmd) {
	title := textinput.New()
	title.Prompt = ""
	title.Placeholder = titlePlaceholder
	title.CharLimit = titleLimit

	body := textarea.New()
	body.Prompt = ""
	body.ShowLineNumbers = false
	body.Placeholder = bodyPlaceholder
	body.CharLimit = bodyLimit
	// The text area highlights its cursor line by default; keep it flat so
	// all fields look alike.
	flat := lipgloss.NewStyle()
	body.FocusedStyle.CursorLine = flat
	body.FocusedStyle.CursorLineNumber = flat

	tags := textinput.New()
	tags.Prompt = ""
	tags.Placeholder = tagsPlaceholder
	tags.CharLimit = tagsLimit

	people := textinput.New()
	people.Prompt = ""
	people.Placeholder = peoplePlaceholder
	people.CharLimit = peopleLimit

	m := Model{
		name: name, title: title, body: body, tags: tags, people: people,
		kinds:   append([]store.Kind{store.KindUnset}, store.Kinds()...),
		sources: sources,
	}
	m = m.Resize(screenWidth, screenHeight)
	m.initial = m.Values()
	return m.focus(FieldTitle)
}

// WithValues fills the form with v and puts the keyboard on f, for showing a
// form again after a mistake without losing what was typed.
func (m Model) WithValues(v Values, f Field) (Model, tea.Cmd) {
	m.title.SetValue(v.Title)
	m.body.SetValue(v.Body)
	m.tags.SetValue(v.Tags)
	m.people.SetValue(v.People)
	for i, k := range m.kinds {
		if k == v.Kind {
			m.kindIdx = i
		}
	}
	for i, s := range m.sources {
		if s == v.Source {
			m.srcIdx = i
		}
	}
	for i, name := range m.templates {
		if name == v.Template {
			m.tmplIdx = i + 1
		}
	}
	return m.focus(f)
}

// Resize fits the form to a screenWidth x screenHeight terminal. The body
// area keeps its text and scroll position.
func (m Model) Resize(screenWidth, screenHeight int) Model {
	m.screenWidth, m.screenHeight = screenWidth, screenHeight
	m.width = min(max(screenWidth-screenMargin, minWidth), maxWidth)
	boxes := lineBoxes
	if m.hasTemplates() {
		boxes++
	}
	fixed := boxes*lineBoxRows + boxChrome
	m.bodyRows = min(max(screenHeight-screenMargin-fixed, minBodyRows), maxBodyRows)
	inner, _ := panel.InnerSize(m.width, 1)
	// A text box draws its cursor in one more cell than its width; leave that
	// cell so a full box does not wrap and make the form jump as it blinks.
	m.title.Width = inner - cursorCell
	m.tags.Width = inner - cursorCell
	m.people.Width = inner - cursorCell
	m.body.SetWidth(inner)
	m.body.SetHeight(m.bodyRows)
	return m
}

// Values returns what has been entered.
func (m Model) Values() Values {
	return Values{
		Title: m.title.Value(), Body: m.body.Value(), Kind: m.kinds[m.kindIdx],
		Source: m.sources[m.srcIdx], Tags: m.tags.Value(), People: m.people.Value(), Template: m.TemplateName(),
	}
}

// Dirty reports whether any field differs from the empty form New made, so a
// form shown again with the text typed before a mistake counts as dirty.
func (m Model) Dirty() bool { return m.Values() != m.initial }

// FocusedField reports which field has the keyboard.
func (m Model) FocusedField() Field { return m.focused }

// focus moves the keyboard to f.
func (m Model) focus(f Field) (Model, tea.Cmd) {
	m.focused = f
	m.kindTyped, m.tmplTyped, m.srcTyped = "", "", ""
	m.title.Blur()
	m.body.Blur()
	m.tags.Blur()
	m.people.Blur()
	var cmd tea.Cmd
	switch f {
	case FieldTitle:
		cmd = m.title.Focus()
	case FieldBody:
		cmd = m.body.Focus()
	case FieldTags:
		cmd = m.tags.Focus()
	case FieldPeople:
		cmd = m.people.Focus()
	}
	return m, cmd
}

// Update handles a message: tab and shift+tab move between the fields, and so
// do Up and Down (in the body only at its first or last line, so its own
// cursor keeps working); Left and Right change the kind while it has the
// focus, and everything else goes to the focused field. It does not interpret enter, esc or ctrl+s,
// which the host uses to save and cancel.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "tab":
			return m.focus(m.wrap(1))
		case "shift+tab":
			return m.focus(m.wrap(-1))
		case "up":
			if m.atEdge(-1) {
				return m.step(-1)
			}
		case "down":
			if m.atEdge(1) {
				return m.step(1)
			}
		}
	}
	var cmd tea.Cmd
	switch m.focused {
	case FieldTitle:
		m.title, cmd = m.title.Update(msg)
	case FieldBody:
		m.body, cmd = m.body.Update(msg)
		m.body = followCursor(m.body)
	case FieldTemplate:
		m.tmplIdx, m.tmplTyped = typeSelect(msg, m.templateLabels(), m.tmplIdx, m.tmplTyped, 1)
	case FieldKind:
		m.kindIdx, m.kindTyped = typeSelect(msg, m.kindLabels(), m.kindIdx, m.kindTyped, 1)
	case FieldSource:
		m.srcIdx, m.srcTyped = typeSelect(msg, m.sources, m.srcIdx, m.srcTyped, 0)
	case FieldTags:
		m.tags, cmd = m.tags.Update(msg)
	case FieldPeople:
		m.people, cmd = m.people.Update(msg)
	}
	return m, cmd
}

// atEdge reports whether an arrow key in direction dir (-1 up, 1 down) should
// leave the focused field rather than move inside it. A one-line field is
// always at its edge; the body is when its cursor is on its first (or last)
// displayed row, counting wrapped lines.
func (m Model) atEdge(dir int) bool {
	if m.focused != FieldBody {
		return true
	}
	info := m.body.LineInfo()
	if dir < 0 {
		return m.body.Line() == 0 && info.RowOffset == 0
	}
	return m.body.Line() == m.body.LineCount()-1 && info.RowOffset == info.Height-1
}

// step moves the focus one field in direction dir, staying put at the first
// and last field (unlike tab, the arrows do not wrap around).
func (m Model) step(dir int) (Model, tea.Cmd) {
	next := m.focused + Field(dir)
	if next == FieldTemplate && !m.hasTemplates() {
		next += Field(dir)
	}
	if next < 0 || next >= fieldCount {
		return m, nil
	}
	return m.focus(next)
}

// wrap returns the field after (dir 1) or before (dir -1) the focused one,
// wrapping around and skipping the Template field when it is hidden.
func (m Model) wrap(dir int) Field {
	next := (m.focused + Field(dir) + fieldCount) % fieldCount
	if next == FieldTemplate && !m.hasTemplates() {
		next = (next + Field(dir) + fieldCount) % fieldCount
	}
	return next
}

// cycle returns idx moved one step by a Left or Right key press in a list of
// n choices, wrapping around, or idx unchanged for any other message.
func cycle(msg tea.Msg, idx, n int) int {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return idx
	}
	switch key.String() {
	case "left":
		return (idx + n - 1) % n
	case "right":
		return (idx + 1) % n
	}
	return idx
}

// followCursor scrolls the text area to its cursor right away. The text
// area scrolls during Update but only loads new text into its viewport when
// it renders, so without this the cursor can sit one line below the visible
// rows until the next key press. Rendering loads the text, and an empty
// update then scrolls with it.
func followCursor(t textarea.Model) textarea.Model {
	_ = t.View()
	t, _ = t.Update(nil)
	return t
}

// Blink forwards a non-key message, such as a cursor blink, to the fields.
func (m Model) Blink(msg tea.Msg) (Model, tea.Cmd) {
	var c1, c2, c3, c4 tea.Cmd
	m.title, c1 = m.title.Update(msg)
	m.body, c2 = m.body.Update(msg)
	m.tags, c3 = m.tags.Update(msg)
	m.people, c4 = m.people.Update(msg)
	return m, tea.Batch(c1, c2, c3, c4)
}

// View draws the boxes, one above the other.
func (m Model) View() string {
	inner, _ := panel.InnerSize(m.width, 1)
	titleBox := panel.Frame(m.name+": Title", m.title.View(), inner, 1, m.focused == FieldTitle)

	bodyFooter := panel.Footer{}
	if total := m.body.LineCount(); total > m.bodyRows {
		bodyFooter.Left = fmt.Sprintf("%d/%d", m.body.Line()+1, total)
	}
	bodyBox := panel.Frame("Body", m.body.View(), inner, m.bodyRows, m.focused == FieldBody, bodyFooter)

	boxes := []string{titleBox}
	if m.hasTemplates() {
		boxes = append(boxes, panel.Frame("Template", m.templateLine(), inner, 1, m.focused == FieldTemplate))
	}
	kindBox := panel.Frame("Kind", m.kindLine(), inner, 1, m.focused == FieldKind)
	sourceBox := panel.Frame("Source", m.sourceLine(), inner, 1, m.focused == FieldSource)

	tagsBox := panel.Frame("Tags", m.tags.View(), inner, 1, m.focused == FieldTags)
	peopleFooter := panel.Footer{Right: hints[m.focused], Tint: true}
	peopleBox := panel.Frame("People", m.people.View(), inner, 1, m.focused == FieldPeople, peopleFooter)
	return strings.Join(append(boxes, bodyBox, kindBox, sourceBox, tagsBox, peopleBox), "\n")
}

// templateLine shows the chosen template, or none, between arrows.
func (m Model) templateLine() string {
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	name := noTemplate
	if m.tmplIdx > 0 {
		name = m.templates[m.tmplIdx-1]
	}
	typed := ""
	if m.tmplTyped != "" {
		typed = dim.Render("  [" + m.tmplTyped + "]")
	}
	return dim.Render("‹ ") + lipgloss.NewStyle().Bold(true).Render(name) + dim.Render(" ›") + typed
}

// sourceLine shows the chosen source between arrows.
func (m Model) sourceLine() string {
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	typed := ""
	if m.srcTyped != "" {
		typed = dim.Render("  [" + m.srcTyped + "]")
	}
	return dim.Render("‹ ") + lipgloss.NewStyle().Bold(true).Render(m.sources[m.srcIdx]) + dim.Render(" ›") + typed
}

// kindLine shows the chosen kind in its color between arrows, with what it
// means.
func (m Model) kindLine() string {
	k := m.kinds[m.kindIdx]
	meaning := k.Meaning()
	if k == store.KindUnset {
		meaning = noKindMeaning
	}
	name := lipgloss.NewStyle().Bold(true).Foreground(kindstyle.Color(k)).Render(k.String())
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	typed := ""
	if m.kindTyped != "" {
		typed = "  [" + m.kindTyped + "]"
	}
	return dim.Render("‹ ") + name + dim.Render(" ›  "+meaning+typed)
}
