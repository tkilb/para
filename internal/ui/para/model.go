// Package para implements the PARA panel: five tabs (Inbox, Projects,
// Areas, Resources, Archives), each listing the notes under its folder in
// the notebook.
package para

import (
	"maps"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tkilb/para/internal/store"
)

type tabDef struct {
	label string
	dir   string // relative to the notebook root
}

var tabDefs = []tabDef{
	{"Inbox", "inbox"},
	{"Projects", "projects"},
	{"Areas", "areas"},
	{"Resources", "resources"},
	{"Archives", "archives"},
}

type row struct {
	entry store.Entry
	kind  store.Kind // notes only
	// uid is the note's uid property, "" for a folder or a note without one.
	uid string
	// overdue marks a daily note that should have been archived by now.
	overdue bool
}

// defaultInnerRows is the height inside the border until the first Resize.
const defaultInnerRows = 10

// tabState is one tab's listing. A tab loads when it is first shown and
// remembers its directory and cursor while another tab is active.
type tabState struct {
	loaded bool
	// dir is the current directory relative to the tab root ("" is the root).
	dir    string
	rows   []row
	cursor int
	// keep is the name to put the cursor on at the next load, when set.
	keep string
	err  error
	// selected holds the names marked in the current directory. It is
	// replaced, never mutated, on every change, and cleared when the
	// directory changes.
	selected map[string]bool
	visual   visualState
	// search is the active search query: n and N jump between the rows whose
	// names contain it, ignoring case, and the matches are highlighted. It
	// belongs to the current directory and clears when leaving it.
	search string
}

// visualMode says whether a range is being selected, unselected, or neither.
type visualMode int

const (
	visualOff visualMode = iota
	visualSelect
	visualUnselect
)

// visualState anchors a visual-mode range. While it is active, the
// selection is base plus (or minus) every row between anchor and cursor.
type visualState struct {
	mode   visualMode
	anchor int
	base   map[string]bool
}

// selectedName returns the name under the cursor, or the pending keep name
// while the tab is stale.
func (t tabState) selectedName() string {
	if !t.loaded {
		return t.keep
	}
	if t.cursor < len(t.rows) {
		return t.rows[t.cursor].entry.Name
	}
	return ""
}

// wantedName is the name the cursor should land on after a reload: one
// explicitly asked for, else the one under the cursor.
func (t tabState) wantedName() string {
	if t.keep != "" {
		return t.keep
	}
	return t.selectedName()
}

// Model is the PARA panel state. Methods return updated copies.
type Model struct {
	store      store.Store
	active     int
	tabs       [5]tabState
	showHidden bool
	// innerRows is the height inside the panel's border; the list gets that
	// less the path row, which shows only inside a folder.
	innerRows int
	// pendingG is set after a first "g", waiting for the second of "gg".
	pendingG bool
	// clip is the yank/cut buffer, shared by every tab.
	clip clip
	// retentionDays is how old an undigested daily note may get before its
	// row is marked overdue (zero never marks), measured against now.
	retentionDays int
	now           func() time.Time
	// marks holds the uids of the marked notes; their rows show a bullet. It
	// is replaced, never mutated.
	marks map[string]bool
}

// New returns a panel over s with the first tab loaded.
func New(s store.Store) Model {
	return Model{store: s, innerRows: defaultInnerRows, now: time.Now}.load(0)
}

// WithRetention marks undigested daily notes whose date is days or more before
// today as overdue, and reloads so the marks show.
func (m Model) WithRetention(days int, now func() time.Time) Model {
	m.retentionDays, m.now = days, now
	return m.Reload()
}

// WithMarks sets which notes, by uid, show a bullet. No reload is needed:
// the rows already know their uids.
func (m Model) WithMarks(uids []string) Model {
	m.marks = make(map[string]bool, len(uids))
	for _, uid := range uids {
		m.marks[uid] = true
	}
	return m
}

// Resize sets the number of rows inside the panel's border.
func (m Model) Resize(visibleRows int) Model {
	m.innerRows = max(visibleRows, 1)
	return m
}

// Reload re-reads the active tab from disk and marks the others stale. Each
// tab keeps its directory, and its cursor stays on the same name.
func (m Model) Reload() Model {
	for i, t := range m.tabs {
		m.tabs[i] = tabState{dir: t.dir, cursor: t.cursor, keep: t.wantedName(), selected: t.selected, search: t.search}
	}
	return m.load(m.active)
}

// load reads tab i's current directory and each note's kind.
func (m Model) load(i int) Model {
	t := m.nearestExisting(i, m.tabs[i])
	dir := filepath.Join(tabDefs[i].dir, t.dir)
	entries, err := m.store.List(dir, store.ListOptions{ShowHidden: m.showHidden})
	rows := make([]row, len(entries))
	for j, e := range entries {
		rows[j] = row{entry: e, kind: store.KindUnset}
		if !e.IsDir {
			rows[j].overdue = m.now != nil && store.DailyOverdue(e.Rel, m.retentionDays, m.now())
			if n, err := m.store.ReadNote(e.Rel); err == nil {
				rows[j].kind = n.Kind
				if v := n.Frontmatter.Get("uid"); v != nil {
					rows[j].uid = strings.TrimSpace(v.Value)
				}
			}
		}
	}
	t.loaded, t.rows, t.err = true, rows, err
	t.cursor = restoreCursor(rows, t.cursor, t.keep)
	t.keep = ""
	t.visual = visualState{}
	t.selected = pruned(t.selected, rows)
	m.tabs[i] = t
	return m
}

// nearestExisting climbs out of a current directory that no longer exists,
// for example one deleted outside the app, to the closest ancestor that
// does, with the cursor on the folder it left. Anything tied to the old
// directory (selection, search) is dropped, as when leaving it by hand.
func (m Model) nearestExisting(i int, t tabState) tabState {
	for t.dir != "" {
		ok, err := m.store.Exists(filepath.Join(tabDefs[i].dir, t.dir))
		if err != nil || ok {
			break
		}
		t.keep = filepath.Base(t.dir)
		if t.dir = filepath.Dir(t.dir); t.dir == "." {
			t.dir = ""
		}
		t.selected, t.visual, t.search = nil, visualState{}, ""
	}
	return t
}

// restoreCursor returns the row index named keep, or cursor clamped to the
// rows when keep is empty or gone.
func restoreCursor(rows []row, cursor int, keep string) int {
	for i, r := range rows {
		if keep != "" && r.entry.Name == keep {
			return i
		}
	}
	return min(cursor, max(len(rows)-1, 0))
}

// Update handles a key press while the panel has focus. The Effect says
// what the root model must do besides showing the updated panel.
func (m Model) Update(msg tea.KeyMsg) (Model, Effect) {
	switch msg.String() {
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
	case "/":
		return m, Effect{Prompt: &Prompt{
			Op: OpSearch, Placeholder: "search names...", Inline: true, CancelOp: OpClearSearch,
		}}
	case "r":
		return m.askRename()
	case "K":
		return m.askKind()
	case "m":
		if e, ok := m.SelectedNote(); ok {
			return m, Effect{Mark: e.Rel}
		}
		return m, Effect{}
	case "e":
		if e, ok := m.SelectedNote(); ok {
			return m, Effect{Edit: e.Rel}
		}
		return m, Effect{}
	case "a":
		return m, Effect{Prompt: &Prompt{Title: "New (end with / for folder)", Op: OpCreate}}
	}
	return m.navigate(msg), Effect{}
}

// navigate handles the keys that only move or select.
func (m Model) navigate(msg tea.KeyMsg) Model {
	pendingG := m.pendingG
	m.pendingG = false

	switch msg.String() {
	case "up":
		m.move(-1)
	case "down":
		m.move(1)
	case " ":
		m.toggleHovered()
	case "v":
		m.startVisual(visualSelect)
	case "V":
		m.startVisual(visualUnselect)
	case "ctrl+a":
		m.selectAll()
	case "esc":
		m = m.escape()
	case "left":
		m = m.goUp()
	case "right", "enter":
		m = m.enterDir()
	case "n":
		m.jumpToMatch(1, false)
	case "N":
		m.jumpToMatch(-1, false)
	case "g":
		if pendingG {
			m.moveTo(0)
		} else {
			m.pendingG = true
		}
	case "G":
		m.moveTo(len(m.tabs[m.active].rows) - 1)
	case "ctrl+u", "pgup":
		m.move(-m.listRows())
	case "ctrl+d", "pgdown":
		m.move(m.listRows())
	case ".":
		m.showHidden = !m.showHidden
		m = m.Reload()
	case "[":
		return m.switchTab(m.active - 1)
	case "]":
		return m.switchTab(m.active + 1)
	}
	return m
}

// move shifts the cursor by delta rows, clamped to the list. In visual mode
// the selection follows the cursor.
func (m *Model) move(delta int) {
	t := &m.tabs[m.active]
	t.cursor = min(max(t.cursor+delta, 0), max(len(t.rows)-1, 0))
	t.applyVisual()
}

func (m *Model) moveTo(i int) {
	m.move(i - m.tabs[m.active].cursor)
}

// enterDir descends into the directory under the cursor; it does nothing on
// a note.
func (m Model) enterDir() Model {
	t := m.tabs[m.active]
	if t.cursor >= len(t.rows) || !t.rows[t.cursor].entry.IsDir {
		return m
	}
	t.dir = filepath.Join(t.dir, t.rows[t.cursor].entry.Name)
	t.cursor, t.keep = 0, ""
	t.selected, t.visual, t.search = nil, visualState{}, ""
	m.tabs[m.active] = t
	return m.load(m.active)
}

// goUp moves to the parent directory with the cursor on the directory just
// left, as Yazi does. It does nothing at the tab root.
func (m Model) goUp() Model {
	t := m.tabs[m.active]
	if t.dir == "" {
		return m
	}
	t.keep = filepath.Base(t.dir)
	t.selected, t.visual, t.search = nil, visualState{}, ""
	if t.dir = filepath.Dir(t.dir); t.dir == "." {
		t.dir = ""
	}
	m.tabs[m.active] = t
	return m.load(m.active)
}

func (m Model) switchTab(i int) Model {
	m.active = (i + len(tabDefs)) % len(tabDefs)
	if !m.tabs[m.active].loaded {
		return m.load(m.active)
	}
	return m
}

// Selected returns the entry under the cursor, a note or a directory.
func (m Model) Selected() (store.Entry, bool) {
	t := m.tabs[m.active]
	if t.cursor >= len(t.rows) {
		return store.Entry{}, false
	}
	return t.rows[t.cursor].entry, true
}

// ListOptions is how the panel lists a folder, so another panel can list the
// same entries.
func (m Model) ListOptions() store.ListOptions {
	return store.ListOptions{ShowHidden: m.showHidden}
}

// SelectedNote returns the entry under the cursor when it is a note.
func (m Model) SelectedNote() (store.Entry, bool) {
	t := m.tabs[m.active]
	if t.cursor >= len(t.rows) || t.rows[t.cursor].entry.IsDir {
		return store.Entry{}, false
	}
	return t.rows[t.cursor].entry, true
}

// toggleHovered flips the selection of the row under the cursor and moves
// down, as Yazi's space does. It is ignored in visual mode.
func (m *Model) toggleHovered() {
	t := &m.tabs[m.active]
	if t.visual.mode != visualOff || t.cursor >= len(t.rows) {
		return
	}
	name := t.rows[t.cursor].entry.Name
	t.selected = withName(t.selected, name, !t.selected[name])
	m.move(1)
}

// startVisual enters visual mode anchored at the cursor. Pressing the same
// key again leaves it; the other key switches between select and unselect.
func (m *Model) startVisual(mode visualMode) {
	t := &m.tabs[m.active]
	if t.cursor >= len(t.rows) {
		return
	}
	if t.visual.mode == mode {
		t.visual = visualState{}
		return
	}
	t.visual = visualState{mode: mode, anchor: t.cursor, base: t.selected}
	t.applyVisual()
}

// selectAll marks every row in the current directory and leaves visual mode.
func (m *Model) selectAll() {
	t := &m.tabs[m.active]
	t.visual = visualState{}
	all := make(map[string]bool, len(t.rows))
	for _, r := range t.rows {
		all[r.entry.Name] = true
	}
	t.selected = all
}

// escape undoes the innermost thing first: visual mode (keeping the
// selection), then the active search, then the selection.
func (m Model) escape() Model {
	t := &m.tabs[m.active]
	switch {
	case t.visual.mode != visualOff:
		t.visual = visualState{}
	case t.search != "":
		t.search = ""
	default:
		t.selected = nil
	}
	return m
}

// setSearch records query as the active search and moves the cursor to the
// nearest match at or after it, wrapping around the list. The list itself
// is never filtered. A query with no match leaves the cursor where it is.
func (m Model) setSearch(query string) Model {
	m.tabs[m.active].search = strings.TrimSpace(query)
	m.jumpToMatch(1, true)
	return m
}

// jumpToMatch moves the cursor to the next row (direction 1) or previous
// row (-1) whose name matches the search, wrapping around. With
// includeCurrent the row under the cursor can match too, which is how a
// freshly committed query lands on the nearest match. It does nothing when
// no search is active or nothing matches.
func (m *Model) jumpToMatch(direction int, includeCurrent bool) {
	t := &m.tabs[m.active]
	n := len(t.rows)
	if n == 0 || t.search == "" {
		return
	}
	i := t.cursor
	if !includeCurrent {
		i += direction
	}
	i = ((i % n) + n) % n
	for range n {
		if matches(t.rows[i].entry, t.search) {
			m.moveTo(i)
			return
		}
		i = ((i+direction)%n + n) % n
	}
}

// matches reports whether the row's displayed name contains query, ignoring
// case.
func matches(e store.Entry, query string) bool {
	return strings.Contains(strings.ToLower(rowName(e)), strings.ToLower(query))
}

// rowName is the name shown for an entry: notes without ".md".
func rowName(e store.Entry) string {
	if e.IsDir {
		return e.Name
	}
	return strings.TrimSuffix(e.Name, ".md")
}

// applyVisual recomputes the selection from the visual range.
func (t *tabState) applyVisual() {
	if t.visual.mode == visualOff {
		return
	}
	lo, hi := min(t.visual.anchor, t.cursor), max(t.visual.anchor, t.cursor)
	sel := maps.Clone(t.visual.base)
	if sel == nil {
		sel = map[string]bool{}
	}
	for i := lo; i <= hi && i < len(t.rows); i++ {
		name := t.rows[i].entry.Name
		if t.visual.mode == visualSelect {
			sel[name] = true
		} else {
			delete(sel, name)
		}
	}
	t.selected = sel
}

// withName returns a copy of set with name added or removed.
func withName(set map[string]bool, name string, on bool) map[string]bool {
	c := maps.Clone(set)
	if c == nil {
		c = map[string]bool{}
	}
	if on {
		c[name] = true
	} else {
		delete(c, name)
	}
	return c
}

// pruned returns the part of set whose names still appear in rows.
func pruned(set map[string]bool, rows []row) map[string]bool {
	var out map[string]bool
	for _, r := range rows {
		if set[r.entry.Name] {
			if out == nil {
				out = map[string]bool{}
			}
			out[r.entry.Name] = true
		}
	}
	return out
}
