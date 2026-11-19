// Package filterbar is the filter part shared by the text search and the note
// finder: the Space, Kind and Dir boxes above the list (with a dropdown of the
// folders that could come next in Dir) and the ctrl+f popup with Modified
// since and _source.md. It narrows a list of notes' lines to the ones that
// pass; the popup that owns it draws its own list and query.
package filterbar

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/tkilb/para/internal/digest"
	"github.com/tkilb/para/internal/findtext"
	"github.com/tkilb/para/internal/store"
)

const charLimit = 100

// Space is a folder of the notebook the list can be limited to.
type Space struct {
	Label, Dir string
}

// Options sets the filters the bar opens with.
type Options struct {
	// Spaces are the folders to choose from; "All" is added before them.
	Spaces []Space
	// Space is the Dir of the space chosen at first ("" for all).
	Space string
	// Dir is the folder path inside the space that the list starts in.
	Dir string
}

// Field is what the keys edit: the owner's query, or one of the bar's boxes.
type Field int

const (
	// FieldQuery is the owner's query box; the bar leaves its keys alone.
	FieldQuery Field = iota
	FieldSpace
	FieldKind
	FieldDir
	fieldCount
)

// Result says what Update did with a message.
type Result int

const (
	// Ignored means the message is the owner's: a key for the query or the
	// list (esc, enter, the arrows, paging), or any message while the query
	// is the field in focus.
	Ignored Result = iota
	// Handled means the bar used the message and the filters are as they were.
	Handled
	// Changed means a filter changed, so the owner should read Scope again.
	Changed
)

const allLabel = "All"

// kindAll stands for every kind in the Kind filter. It is not a valid kind,
// so it cannot clash with one the config adds.
const kindAll store.Kind = ""

const day = 24 * time.Hour

// sinceOption is one choice of the "Modified since" filter. Within is how far
// back a note may have changed, zero for any time.
type sinceOption struct {
	Label  string
	Within time.Duration
}

// sinceOptions are the choices of the "Modified since" filter; the first is
// no filter.
var sinceOptions = []sinceOption{
	{"Anytime", 0},
	{"7 days", 7 * day},
	{"14 days", 14 * day},
	{"30 days", 30 * day},
	{"60 days", 60 * day},
	{"90 days", 90 * day},
}

// The rows of the filters popup.
const (
	rowSince = iota
	rowSource
	rowCount
)

// Bar is the filter state. Methods return updated copies.
type Bar struct {
	records []findtext.Line
	rels    []string // the notes of records, once each, for completing the dir

	spaces []Space // spaces[0] is All
	space  int
	kinds  []store.Kind // kinds[0] is kindAll, the last is store.KindUnset
	kind   int
	dir    textinput.Model
	// hintSel is the folder chosen in the dir suggestions with the arrows, or
	// -1 when none is.
	hintSel int
	focus   Field

	// showSource keeps the lines of import transcripts (_source.md), which
	// are left out until the filters popup turns them on.
	showSource bool
	// since is the choice in sinceOptions: how recently a note changed.
	since int
	// open is set while the filters popup is shown, and row is the row of it
	// the keys act on.
	open bool
	row  int
}

// New returns a bar over records, the lines (or one line per note) to filter,
// opened with the filters in opts.
func New(records []findtext.Line, opts Options) Bar {
	dir := textinput.New()
	dir.CharLimit = charLimit
	dir.Prompt = ""
	dir.Placeholder = "(whole space)"
	dir.SetValue(opts.Dir)

	b := Bar{records: records, dir: dir, hintSel: -1}
	b.spaces = append([]Space{{Label: allLabel}}, opts.Spaces...)
	for i, sp := range b.spaces {
		if i > 0 && sp.Dir == opts.Space {
			b.space = i
		}
	}
	b.rels = noteRels(records)
	b.kinds = append(append([]store.Kind{kindAll}, store.Kinds()...), store.KindUnset)
	return b
}

// noteRels returns the paths of the notes in lines, once each, in order.
func noteRels(lines []findtext.Line) []string {
	var rels []string
	for i, l := range lines {
		if i == 0 || l.Rel != lines[i-1].Rel {
			rels = append(rels, l.Rel)
		}
	}
	return rels
}

// Focus is the field the keys edit.
func (b Bar) Focus() Field { return b.focus }

// Scope returns the records that pass every filter. The path is inside the
// space, or from the notebook root when the space is All.
func (b Bar) Scope() []findtext.Line {
	lines := b.records
	if !b.showSource {
		lines = findtext.WithoutFile(lines, digest.SourceFile)
	}
	if k := b.kinds[b.kind]; k != kindAll {
		lines = findtext.OfKind(lines, k)
	}
	if w := sinceOptions[b.since].Within; w > 0 {
		lines = findtext.ChangedSince(lines, time.Now().Add(-w))
	}
	prefix := strings.TrimLeft(strings.TrimSpace(b.dir.Value()), "/")
	if d := b.spaces[b.space].Dir; d != "" {
		prefix = d + "/" + prefix
	}
	if prefix == "" {
		return lines
	}
	var out []findtext.Line
	for _, l := range lines {
		if findtext.UnderPath(l.Rel, prefix) {
			out = append(out, l)
		}
	}
	return out
}

// dirHints returns the folders that could come next in the dir field: the
// ones inside the chosen space (or the notebook, for All) that start with
// what is typed after its last slash.
func (b Bar) dirHints() []string {
	typed := strings.TrimLeft(strings.TrimSpace(b.dir.Value()), "/")
	if d := b.spaces[b.space].Dir; d != "" {
		return findtext.ChildDirs(b.rels, d+"/"+typed)
	}
	return findtext.ChildDirs(b.rels, typed)
}

// suggesting reports whether the dir suggestions are open.
func (b Bar) suggesting() bool {
	return b.focus == FieldDir && len(b.dirHints()) > 0
}

// acceptHint puts the chosen suggestion in the dir field, with a slash after
// it, ready to list the folders inside it.
func (b Bar) acceptHint() Bar {
	names := b.dirHints()
	if b.hintSel < 0 || b.hintSel >= len(names) {
		return b
	}
	typed := strings.TrimLeft(strings.TrimSpace(b.dir.Value()), "/")
	return b.setDir(typed[:strings.LastIndex(typed, "/")+1] + names[b.hintSel] + "/")
}

// setDir replaces the dir field, with the cursor at its end (SetValue keeps
// the old position when it still fits).
func (b Bar) setDir(v string) Bar {
	b.dir.SetValue(v)
	b.dir.CursorEnd()
	b.hintSel = -1
	return b
}

// moveHint moves the choice in the dir suggestions delta places: from none to
// the first with down, to the last with up, and back to none past either end.
func (b Bar) moveHint(delta int) Bar {
	n := len(b.dirHints())
	b.hintSel += delta
	switch {
	case b.hintSel < -1:
		b.hintSel = n - 1
	case b.hintSel >= n:
		b.hintSel = -1
	}
	return b
}

// setFocus moves the keyboard to field f. The command blinks the dir cursor.
func (b Bar) setFocus(f Field) (Bar, tea.Cmd) {
	b.focus, b.hintSel = f, -1
	b.dir.Blur()
	if f == FieldDir {
		return b, b.dir.Focus()
	}
	return b, nil
}

// pickSpace moves the chosen space delta places, wrapping. The dir is a path
// inside the space, so it is cleared.
func (b Bar) pickSpace(delta int) Bar {
	b.space = (b.space + delta + len(b.spaces)) % len(b.spaces)
	b.dir.SetValue("")
	b.hintSel = -1
	return b
}

// pickKind moves the chosen kind delta places, wrapping.
func (b Bar) pickKind(delta int) Bar {
	b.kind = (b.kind + delta + len(b.kinds)) % len(b.kinds)
	return b
}

// passesToOwner reports whether a key in a box of the bar is for the owner's
// list: leaving, choosing, moving and scrolling work from every box.
func passesToOwner(key string, f Field) bool {
	switch key {
	case "esc", "enter", "up", "down", "ctrl+p", "ctrl+n", "pgup", "pgdown", "ctrl+d":
		return true
	case "ctrl+u":
		return f != FieldDir // there it clears the dir
	}
	return false
}

// Update handles a message: tab and shift+tab move between the query, space,
// kind and dir fields, ctrl+f opens the filters popup, left, right, h, l and
// space change the space or kind, up and down in dir choose from its folder
// suggestions and enter fills one in, ctrl+u clears the dir and ctrl+w
// removes its last folder name. While the popup is open it takes every key.
// Keys for the owner's list come back Ignored.
func (b Bar) Update(msg tea.Msg) (Bar, tea.Cmd, Result) {
	k, isKey := msg.(tea.KeyMsg)
	if !isKey {
		if b.focus == FieldDir {
			var cmd tea.Cmd
			b.dir, cmd = b.dir.Update(msg)
			return b, cmd, Handled
		}
		return b, nil, Ignored
	}
	key := k.String()
	if b.open {
		return b.popupKey(key)
	}
	switch key {
	case "ctrl+f":
		b.open, b.row = true, rowSince
		return b, nil, Handled
	case "tab":
		b, cmd := b.setFocus((b.focus + 1) % fieldCount)
		return b, cmd, Handled
	case "shift+tab":
		b, cmd := b.setFocus((b.focus + fieldCount - 1) % fieldCount)
		return b, cmd, Handled
	}
	if b.focus == FieldDir {
		switch key {
		case "ctrl+u":
			return b.setDir(""), nil, Changed
		case "ctrl+w":
			return b.setDir(findtext.DropLastSegment(b.dir.Value())), nil, Changed
		}
		if b.suggesting() {
			switch key {
			case "up", "ctrl+p":
				return b.moveHint(-1), nil, Handled
			case "down", "ctrl+n":
				return b.moveHint(1), nil, Handled
			case "enter":
				if b.hintSel >= 0 {
					return b.acceptHint(), nil, Changed
				}
			}
		}
	}
	if b.focus == FieldQuery || passesToOwner(key, b.focus) {
		return b, nil, Ignored
	}
	return b.boxKey(k)
}

// boxKey handles a key for the space, kind or dir box in focus.
func (b Bar) boxKey(k tea.KeyMsg) (Bar, tea.Cmd, Result) {
	if b.focus == FieldDir {
		before := b.dir.Value()
		var cmd tea.Cmd
		b.dir, cmd = b.dir.Update(k)
		if b.dir.Value() != before {
			b.hintSel = -1
			return b, cmd, Changed
		}
		return b, cmd, Handled
	}
	pick := b.pickSpace
	if b.focus == FieldKind {
		pick = b.pickKind
	}
	switch k.String() {
	case "left", "h", "shift+left":
		return pick(-1), nil, Changed
	case "right", "l", " ", "shift+right":
		return pick(1), nil, Changed
	}
	return b, nil, Handled
}

// popupKey handles a key while the filters popup is open: up, down and tab
// move between its rows, left, right and space change the row, and enter, esc
// and ctrl+f close it. Other keys do nothing, so typing never reaches the
// query behind it.
func (b Bar) popupKey(key string) (Bar, tea.Cmd, Result) {
	switch key {
	case "esc", "enter", "ctrl+f":
		b.open = false
	case "up", "shift+tab", "ctrl+p":
		b.row = (b.row + rowCount - 1) % rowCount
	case "down", "tab", "ctrl+n":
		b.row = (b.row + 1) % rowCount
	case "left", "h", "shift+left":
		return b.changeFilter(-1), nil, Changed
	case "right", "l", " ", "shift+right":
		return b.changeFilter(1), nil, Changed
	}
	return b, nil, Handled
}

// changeFilter moves the filter in the current popup row delta places,
// wrapping.
func (b Bar) changeFilter(delta int) Bar {
	switch b.row {
	case rowSince:
		n := len(sinceOptions)
		b.since = (b.since + delta + n) % n
	case rowSource:
		b.showSource = !b.showSource
	}
	return b
}
