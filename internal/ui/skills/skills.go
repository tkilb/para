package skillsui

import (
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/tkilb/para/internal/skills"
	"github.com/tkilb/para/internal/store"
	"github.com/tkilb/para/internal/textsafe"
	"github.com/tkilb/para/internal/ui/panel"
	"github.com/tkilb/para/internal/undo"
)

const (
	defaultInnerRows = 7
	badgeWidth       = 4
	markerBar        = "▎"
	markerWidth      = 1
)

var (
	dimStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	errorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	selStyle   = lipgloss.NewStyle().Bold(true).Background(panel.SelectedRowBackground)

	badgeNoteColor        = lipgloss.Color("2") // green
	badgeInteractiveColor = lipgloss.Color("5") // magenta
	badgeImportColor      = lipgloss.Color("3") // yellow

	markerColors = map[mark]lipgloss.Color{
		markSelected: lipgloss.Color("3"), // yellow
		markYank:     lipgloss.Color("2"), // green
		markCut:      lipgloss.Color("1"), // red
	}

	matchOnCursor  = lipgloss.Color("6")
	matchElsewhere = lipgloss.Color("3")
)

type mark int

const (
	markNone mark = iota
	markSelected
	markYank
	markCut
)

type visualMode int

const (
	visualOff visualMode = iota
	visualSelect
	visualUnselect
)

type visualState struct {
	mode   visualMode
	anchor int
	base   map[string]bool
}

type clipMode int

const (
	clipNone clipMode = iota
	clipYank
	clipCut
)

type clipState struct {
	mode      clipMode
	srcStore  store.Store
	srcRoot   string
	filenames []string
	set       map[string]bool
}

// ConfirmOp identifies a pending yes/no confirmation for the skills panel.
type ConfirmOp int

const (
	OpTrash ConfirmOp = iota + 1
	OpDelete
	OpOverwritePaste
)

// Confirm describes a yes/no confirmation prompt requested by the skills panel.
type Confirm struct {
	Prompt string
	Op     ConfirmOp
	Danger bool
}

// InputOp identifies a text prompt requested by the skills panel.
type InputOp int

const (
	OpCreate InputOp = iota + 1
	OpRename
	OpSearch
	OpClearSearch
)

// Prompt describes a one-line text input requested by the skills panel.
type Prompt struct {
	Title       string
	Label       string
	Initial     string
	Placeholder string
	Inline      bool
	Op          InputOp
	CancelOp    InputOp
}

// Effect describes side effects the root model should perform after a skills panel update.
type Effect struct {
	Action   *undo.Action
	Info     string
	Warn     string
	Err      error
	Execute  *skills.Skill
	EditPath string
	CopyPath string
	Confirm  *Confirm
	Prompt   *Prompt
	Leave    bool
}

// Model represents the state of the [1] Skills panel.
type Model struct {
	store        store.Store
	notebookRoot string
	agent        skills.Agent
	skills       []skills.Skill
	loadErr      error
	cursor       int
	top          int
	innerRows    int
	keep         string
	pendingG     bool
	selected     map[string]bool
	visual       visualState
	clip         clipState
	search       string
}

// New creates an empty skills panel model.
func New() Model {
	return Model{innerRows: defaultInnerRows}
}

// SetNotebook updates the notebook root, store, and agent, reloading skills from disk.
// The yank/cut clipboard buffer is preserved across notebook switches so skills can
// be copied or moved between notebooks.
func (m Model) SetNotebook(st store.Store, root string, agent skills.Agent) Model {
	m.store = st
	m.notebookRoot = root
	m.agent = agent
	m.cursor = 0
	m.top = 0
	m.keep = ""
	m.selected = nil
	m.visual = visualState{}
	m.search = ""
	m.pendingG = false
	return m.Reload()
}

// Resize updates the visible inner row count for scrolling and paging.
func (m Model) Resize(visibleRows int) Model {
	m.innerRows = max(visibleRows, 1)
	return m.syncScroll()
}

func (m Model) syncScroll() Model {
	m.top = panel.ScrollTop(m.top, m.cursor, len(m.skills), max(m.innerRows, 1), panel.ScrollOff)
	return m
}

// Reload re-reads skills from .skills/ in notebookRoot, keeping the cursor on the same skill.
func (m Model) Reload() Model {
	if m.notebookRoot == "" {
		m.skills = nil
		m.loadErr = nil
		m.cursor = 0
		m.top = 0
		return m
	}
	want := m.wantedFilename()
	loaded, err := skills.List(m.notebookRoot)
	m.skills = loaded
	m.loadErr = err
	m.cursor = restoreCursor(loaded, m.cursor, want)
	m.keep = ""
	m.visual = visualState{}
	m.selected = prunedSelection(m.selected, loaded)
	return m.syncScroll()
}

func (m Model) wantedFilename() string {
	if m.keep != "" {
		return m.keep
	}
	if m.cursor >= 0 && m.cursor < len(m.skills) {
		return m.skills[m.cursor].Filename
	}
	return ""
}

func restoreCursor(list []skills.Skill, cursor int, keep string) int {
	if keep != "" {
		for i, s := range list {
			if s.Filename == keep || s.Name == keep {
				return i
			}
		}
	}
	return min(max(cursor, 0), max(len(list)-1, 0))
}

func prunedSelection(sel map[string]bool, list []skills.Skill) map[string]bool {
	if len(sel) == 0 {
		return nil
	}
	out := make(map[string]bool, len(sel))
	for _, s := range list {
		if sel[s.Filename] {
			out[s.Filename] = true
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// Skills returns the currently loaded list of skills.
func (m Model) Skills() []skills.Skill {
	return m.skills
}

// Selected returns the currently highlighted skill, if any.
func (m Model) Selected() (skills.Skill, bool) {
	if len(m.skills) == 0 || m.cursor < 0 || m.cursor >= len(m.skills) {
		return skills.Skill{}, false
	}
	return m.skills[m.cursor], true
}

// Cursor returns the cursor index.
func (m Model) Cursor() int {
	return m.cursor
}

// Count returns the number of skills loaded.
func (m Model) Count() int {
	return len(m.skills)
}

// Active reports whether visual selection, search, or multi-selection is active
// (so Esc clears that state before leaving the panel).
func (m Model) Active() bool {
	return m.visual.mode != visualOff || m.search != "" || len(m.selected) > 0
}

// Escape clears the innermost state: visual mode, then search, then selection.
func (m Model) Escape() Model {
	switch {
	case m.visual.mode != visualOff:
		m.visual = visualState{}
	case m.search != "":
		m.search = ""
	default:
		m.selected = nil
	}
	return m
}

// RowAt maps a content row index (0 is first row inside top border) to a skill index.
func (m Model) RowAt(row int) (int, bool) {
	return panel.RowAtScroll(row, m.top, m.cursor, len(m.skills), max(m.innerRows, 1))
}

// SelectRow puts the cursor on row i.
func (m Model) SelectRow(i int) Model {
	if i >= 0 && i < len(m.skills) {
		m.cursor = i
		m.applyVisual()
	}
	return m.syncScroll()
}

// Update handles a key press while the panel is focused.
func (m Model) Update(msg tea.KeyMsg) (Model, Effect) {
	m, eff := m.update(msg)
	return m.syncScroll(), eff
}

func (m Model) update(msg tea.KeyMsg) (Model, Effect) {
	pendingG := m.pendingG
	m.pendingG = false

	switch msg.String() {
	case "up", "k":
		m.move(-1)
	case "down", "j":
		m.move(1)
	case "g":
		if pendingG {
			m.moveTo(0)
		} else {
			m.pendingG = true
		}
	case "G":
		m.moveTo(len(m.skills) - 1)
	case "ctrl+u", "pgup":
		m.move(-max(m.innerRows, 1))
	case "ctrl+d", "pgdown":
		m.move(max(m.innerRows, 1))
	case " ":
		m.toggleHovered()
	case "v":
		m.startVisual(visualSelect)
	case "V":
		m.startVisual(visualUnselect)
	case "ctrl+a":
		m.selectAll()
	case "esc":
		if m.Active() {
			return m.Escape(), Effect{}
		}
		return m, Effect{Leave: true}
	case "left":
		return m, Effect{Leave: true}
	case "y":
		return m.setClip(clipYank)
	case "x":
		return m.setClip(clipCut)
	case "Y", "X":
		return m.cancelClip()
	case "p":
		return m.paste(false, false)
	case "P":
		return m.paste(true, false)
	case "d":
		return m.askDelete(OpTrash)
	case "D":
		return m.askDelete(OpDelete)
	case "a":
		return m, Effect{Prompt: &Prompt{
			Title: "New skill",
			Label: "Skill name:",
			Op:    OpCreate,
		}}
	case "r":
		return m.askRename()
	case "e":
		if s, ok := m.Selected(); ok {
			path := s.Path
			if path == "" && m.notebookRoot != "" {
				path = filepath.Join(skills.Dir(m.notebookRoot), s.Filename)
			}
			return m, Effect{EditPath: path}
		}
	case "c":
		if s, ok := m.Selected(); ok {
			path := s.Path
			if path == "" && m.notebookRoot != "" {
				path = filepath.Join(skills.Dir(m.notebookRoot), s.Filename)
			}
			return m, Effect{CopyPath: path}
		}
	case "/":
		return m, Effect{Prompt: &Prompt{
			Op:          OpSearch,
			Placeholder: "search skills...",
			Inline:      true,
			CancelOp:    OpClearSearch,
		}}
	case "n":
		m.jumpToMatch(1, false)
	case "N":
		m.jumpToMatch(-1, false)
	case "enter":
		if s, ok := m.Selected(); ok {
			cp := s
			return m, Effect{Execute: &cp}
		}
	}
	return m, Effect{}
}

func (m *Model) move(delta int) {
	if len(m.skills) == 0 {
		m.cursor = 0
		return
	}
	m.cursor = min(max(m.cursor+delta, 0), len(m.skills)-1)
	m.applyVisual()
}

func (m *Model) moveTo(i int) {
	if len(m.skills) == 0 {
		m.cursor = 0
		return
	}
	m.move(i - m.cursor)
}

func (m *Model) toggleHovered() {
	if m.visual.mode != visualOff || m.cursor >= len(m.skills) {
		return
	}
	fn := m.skills[m.cursor].Filename
	m.selected = withFilename(m.selected, fn, !m.selected[fn])
	m.move(1)
}

func (m *Model) startVisual(mode visualMode) {
	if m.cursor >= len(m.skills) {
		return
	}
	if m.visual.mode == mode {
		m.visual = visualState{}
		return
	}
	m.visual = visualState{mode: mode, anchor: m.cursor, base: m.selected}
	m.applyVisual()
}

func (m *Model) selectAll() {
	m.visual = visualState{}
	if len(m.skills) == 0 {
		return
	}
	all := make(map[string]bool, len(m.skills))
	for _, s := range m.skills {
		all[s.Filename] = true
	}
	m.selected = all
}

func (m *Model) applyVisual() {
	if m.visual.mode == visualOff {
		return
	}
	lo, hi := min(m.visual.anchor, m.cursor), max(m.visual.anchor, m.cursor)
	sel := maps.Clone(m.visual.base)
	if sel == nil {
		sel = map[string]bool{}
	}
	for i := lo; i <= hi && i < len(m.skills); i++ {
		fn := m.skills[i].Filename
		if m.visual.mode == visualSelect {
			sel[fn] = true
		} else {
			delete(sel, fn)
		}
	}
	m.selected = sel
}

func withFilename(set map[string]bool, fn string, on bool) map[string]bool {
	c := maps.Clone(set)
	if c == nil {
		c = map[string]bool{}
	}
	if on {
		c[fn] = true
	} else {
		delete(c, fn)
	}
	return c
}

func (m Model) targets() []skills.Skill {
	var out []skills.Skill
	for _, s := range m.skills {
		if m.selected[s.Filename] {
			out = append(out, s)
		}
	}
	if len(out) == 0 && m.cursor >= 0 && m.cursor < len(m.skills) {
		out = append(out, m.skills[m.cursor])
	}
	return out
}

func targetFilenames(targets []skills.Skill) []string {
	out := make([]string, len(targets))
	for i, s := range targets {
		out[i] = s.Filename
	}
	return out
}

func (m Model) setClip(mode clipMode) (Model, Effect) {
	targets := m.targets()
	if len(targets) == 0 {
		return m, Effect{}
	}
	c := clipState{
		mode:      mode,
		srcStore:  m.store,
		srcRoot:   m.notebookRoot,
		filenames: make([]string, 0, len(targets)),
		set:       make(map[string]bool, len(targets)),
	}
	for _, s := range targets {
		c.filenames = append(c.filenames, s.Filename)
		c.set[s.Filename] = true
	}
	m.clip = c
	m.selected = nil
	m.visual = visualState{}
	return m, Effect{}
}

func (m Model) cancelClip() (Model, Effect) {
	m.clip = clipState{}
	return m, Effect{}
}

func (m Model) paste(overwrite, confirmed bool) (Model, Effect) {
	if m.clip.mode == clipNone || len(m.clip.filenames) == 0 {
		return m, Effect{Info: "Nothing to paste."}
	}
	sameNotebook := m.clip.srcRoot == m.notebookRoot
	cut := m.clip.mode == clipCut
	plan, err := skills.PlanPaste(m.clip.srcStore, m.store, sameNotebook, m.clip.filenames, cut)
	if err != nil {
		return m, Effect{Err: fmt.Errorf("paste: %w", err)}
	}
	if plan.Collisions > 0 {
		if !overwrite {
			return m, Effect{Err: fmt.Errorf("paste cancelled: %s here (P overwrites)", alreadyExist(plan.Collisions))}
		}
		if !confirmed {
			return m, Effect{Confirm: &Confirm{
				Prompt: fmt.Sprintf("Overwrite %s? Replaced skills go to the trash.", countSkills(plan.Collisions)),
				Op:     OpOverwritePaste,
			}}
		}
	}
	if len(plan.Items) == 0 {
		m.clip = clipState{}
		if plan.Skipped > 0 {
			return m, Effect{Info: "Already in this notebook's .skills/."}
		}
		return m, Effect{Info: "Nothing was pasted."}
	}
	act, firstDst, err := skills.ExecutePaste(m.clip.srcStore, m.store, plan, cut)
	if err != nil {
		return m, Effect{Err: fmt.Errorf("paste failed: %w", err)}
	}
	m.clip = clipState{}
	m.keep = firstDst
	m = m.Reload()
	return m, Effect{Action: &act}
}

func (m Model) askDelete(op ConfirmOp) (Model, Effect) {
	targets := m.targets()
	if len(targets) == 0 {
		return m, Effect{}
	}
	verb := "Trash"
	if op == OpDelete {
		verb = "Permanently delete"
	}
	what := targets[0].Name
	if len(targets) > 1 {
		what = countSkills(len(targets))
	}
	prompt := fmt.Sprintf("%s %s?", verb, what)
	if op == OpDelete {
		prompt += " This cannot be undone."
	}
	return m, Effect{Confirm: &Confirm{Prompt: prompt, Op: op, Danger: true}}
}

func (m Model) askRename() (Model, Effect) {
	s, ok := m.Selected()
	if !ok {
		return m, Effect{}
	}
	initial := strings.TrimSuffix(strings.TrimSuffix(s.Filename, ".md"), ".MD")
	return m, Effect{Prompt: &Prompt{
		Title:   "Rename skill",
		Initial: initial,
		Op:      OpRename,
	}}
}

// ApplyConfirm executes an accepted confirmation for the skills panel.
func (m Model) ApplyConfirm(op ConfirmOp) (Model, Effect) {
	m, eff := m.applyConfirm(op)
	return m.syncScroll(), eff
}

func (m Model) applyConfirm(op ConfirmOp) (Model, Effect) {
	switch op {
	case OpOverwritePaste:
		return m.paste(true, true)
	case OpTrash:
		targets := m.targets()
		if len(targets) == 0 {
			return m, Effect{}
		}
		act, err := skills.TrashInStore(m.store, targetFilenames(targets))
		if err != nil {
			return m.Reload(), Effect{Err: fmt.Errorf("trash failed: %w", err)}
		}
		m.selected = nil
		m.visual = visualState{}
		return m.Reload(), Effect{Action: &act}
	case OpDelete:
		targets := m.targets()
		if len(targets) == 0 {
			return m, Effect{}
		}
		if err := skills.DeleteInStore(m.store, targetFilenames(targets)); err != nil {
			return m.Reload(), Effect{Err: err}
		}
		m.selected = nil
		m.visual = visualState{}
		return m.Reload(), Effect{}
	}
	return m, Effect{}
}

// ApplyInput executes a text prompt submitted for the skills panel.
func (m Model) ApplyInput(op InputOp, text string) (Model, Effect) {
	m, eff := m.applyInput(op, text)
	return m.syncScroll(), eff
}

func (m Model) applyInput(op InputOp, text string) (Model, Effect) {
	switch op {
	case OpCreate:
		if strings.TrimSpace(text) == "" {
			return m, Effect{}
		}
		fn, act, err := skills.CreateInStore(m.store, text, skills.ModeNote)
		if err != nil {
			return m, Effect{Err: fmt.Errorf("create skill failed: %w", err)}
		}
		m.keep = fn
		m = m.Reload()
		editPath := filepath.Join(skills.Dir(m.notebookRoot), fn)
		return m, Effect{Action: &act, EditPath: editPath}
	case OpRename:
		s, ok := m.Selected()
		if !ok {
			return m, Effect{}
		}
		newFn, act, err := skills.RenameInStore(m.store, s.Filename, text)
		if errors.Is(err, skills.ErrUnchangedName) {
			return m, Effect{Info: "The name is unchanged."}
		}
		if err != nil {
			return m, Effect{Err: fmt.Errorf("rename skill: %w", err)}
		}
		m.keep = newFn
		m = m.Reload()
		return m, Effect{Action: &act}
	case OpSearch:
		q := strings.TrimSpace(text)
		if q == "" {
			return m, Effect{}
		}
		m.search = q
		m.jumpToMatch(1, true)
		return m, Effect{}
	case OpClearSearch:
		m.search = ""
		return m, Effect{}
	}
	return m, Effect{}
}

func (m *Model) jumpToMatch(direction int, includeCurrent bool) {
	n := len(m.skills)
	if n == 0 || m.search == "" {
		return
	}
	i := m.cursor
	if !includeCurrent {
		i += direction
	}
	i = ((i % n) + n) % n
	for range n {
		if matches(m.skills[i], m.search) {
			m.moveTo(i)
			return
		}
		i = ((i+direction)%n + n) % n
	}
}

func matches(s skills.Skill, query string) bool {
	if query == "" {
		return false
	}
	q := strings.ToLower(query)
	return strings.Contains(strings.ToLower(s.Name), q) ||
		strings.Contains(strings.ToLower(s.Description), q)
}

func (m Model) markOf(s skills.Skill) mark {
	if m.clip.srcRoot == m.notebookRoot && m.clip.set[s.Filename] {
		if m.clip.mode == clipCut {
			return markCut
		}
		return markYank
	}
	if m.selected[s.Filename] {
		return markSelected
	}
	return markNone
}

// View renders the panel into outerWidth x outerHeight.
func (m Model) View(outerWidth, outerHeight int, focused bool) string {
	width, height := panel.InnerSize(outerWidth, outerHeight)
	title := "1 Skills"

	if height <= 1 || !focused {
		var line string
		agentName := string(m.agent)
		if agentName == "" {
			agentName = "no agent"
		}
		count := len(m.skills)
		switch count {
		case 0:
			line = dimStyle.Render(fmt.Sprintf("%s · 0 skills", agentName))
		case 1:
			line = fmt.Sprintf("%s · 1 skill", agentName)
		default:
			line = fmt.Sprintf("%s · %d skills", agentName, count)
		}
		line = ansi.Truncate(line, width, "…")
		return panel.Frame(title, line, width, height, focused)
	}

	left := ""
	if m.search != "" {
		left = "search: " + m.search
	}
	footer := panel.Footer{Left: left, Right: m.statusText()}

	var body string
	switch {
	case m.loadErr != nil:
		body = errorStyle.Render(ansi.Truncate(m.loadErr.Error(), width, "…"))
	case len(m.skills) == 0:
		body = dimStyle.Render("(empty — press a to add a skill)")
	default:
		top := panel.ScrollTop(m.top, m.cursor, len(m.skills), height, panel.ScrollOff)
		start, end := panel.Window(top, len(m.skills), height)
		lines := make([]string, 0, end-start)
		for i := start; i < end; i++ {
			s := m.skills[i]
			lines = append(lines, renderSkillRow(s, width, i == m.cursor && focused, m.markOf(s), m.search))
		}
		body = strings.Join(lines, "\n")
	}

	return panel.Frame(title, body, width, height, focused, footer)
}

func (m Model) statusText() string {
	var parts []string
	switch m.visual.mode {
	case visualSelect:
		parts = append(parts, "VISUAL")
	case visualUnselect:
		parts = append(parts, "UNSELECT")
	}
	if n := len(m.selected); n > 0 {
		parts = append(parts, fmt.Sprintf("%d sel", n))
	}
	switch m.clip.mode {
	case clipYank:
		parts = append(parts, fmt.Sprintf("%d yanked", len(m.clip.filenames)))
	case clipCut:
		parts = append(parts, fmt.Sprintf("%d cut", len(m.clip.filenames)))
	}
	if len(m.skills) > 0 {
		parts = append(parts, fmt.Sprintf("%d/%d", m.cursor+1, len(m.skills)))
	}
	return strings.Join(parts, " ")
}

func renderSkillRow(s skills.Skill, width int, cursor bool, mk mark, query string) string {
	if !matches(s, query) {
		query = ""
	}
	bar, barColor := " ", markerColors[mk]
	if mk != markNone {
		bar = markerBar
	}
	markerStyle := lipgloss.NewStyle().Foreground(barColor)
	width = max(width-markerWidth, 1)

	badge, badgeColor := modeBadge(s.Mode)
	name := textsafe.SingleLine(s.Name)
	right := textsafe.SingleLine(s.Description)

	availForText := max(width-badgeWidth-1, 1)
	if right != "" {
		maxRight := max(availForText/2, 0)
		if maxRight > 0 && ansi.StringWidth(name)+2 <= availForText {
			rightLimit := min(maxRight, max(availForText-ansi.StringWidth(name)-2, 0))
			right = ansi.Truncate(right, rightLimit, "…")
		} else {
			right = ""
		}
	}

	nameWidth := availForText
	if right != "" {
		nameWidth = max(availForText-ansi.StringWidth(right)-1, 1)
	}
	name = ansi.Truncate(name, nameWidth, "…")
	gap := strings.Repeat(" ", max(width-badgeWidth-1-ansi.StringWidth(name)-ansi.StringWidth(right), 0))

	if cursor {
		barSel := selStyle.Foreground(barColor)
		badgeSel := selStyle.Bold(false).Foreground(badgeColor)
		rightSel := selStyle.Bold(false).Foreground(lipgloss.Color("245"))
		return barSel.Render(bar) +
			badgeSel.Render(badge) +
			selStyle.Render(" ") +
			highlight(name, query, selStyle, true) +
			selStyle.Render(gap) +
			rightSel.Render(right)
	}

	badgeStyle := lipgloss.NewStyle().Foreground(badgeColor)
	nameStyle := lipgloss.NewStyle()
	return markerStyle.Render(bar) +
		badgeStyle.Render(badge) +
		" " +
		highlight(name, query, nameStyle, false) +
		gap +
		dimStyle.Render(right)
}

func modeBadge(mode skills.Mode) (string, lipgloss.Color) {
	switch mode {
	case skills.ModeInteractive:
		return "chat", badgeInteractiveColor
	case skills.ModeImport:
		return "imp ", badgeImportColor
	default:
		return "note", badgeNoteColor
	}
}

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

func countSkills(n int) string {
	if n == 1 {
		return "1 skill"
	}
	return fmt.Sprintf("%d skills", n)
}

func alreadyExist(n int) string {
	if n == 1 {
		return "1 skill already exists"
	}
	return fmt.Sprintf("%d skills already exist", n)
}
