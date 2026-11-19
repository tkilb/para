package main

import (
	"cmp"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tkilb/para/internal/marks"
	"github.com/tkilb/para/internal/store"
	"github.com/tkilb/para/internal/textsafe"
	"github.com/tkilb/para/internal/ui/picker"
	"github.com/tkilb/para/internal/ui/popup"
)

const (
	marksTitle = "Marks"
	marksHint  = "enter jump to note  esc cancel"
)

// noteUID reads the uid property of the note at rel, or "" when it has none.
func (m model) noteUID(rel string) (string, error) {
	note, err := m.store.ReadNote(rel)
	if err != nil {
		return "", err
	}
	if v := note.Frontmatter.Get("uid"); v != nil {
		return strings.TrimSpace(v.Value), nil
	}
	return "", nil
}

// openMarks opens a fuzzy picker over the marked notes, with the highlight,
// centered, on the note under the PARA cursor when that note is marked. Enter
// jumps to the note. A mark whose note
// cannot be found (deleted outside para) is left out.
func (m model) openMarks() (model, tea.Cmd) {
	if m.notebookRoot() == "" {
		return m.notify(popup.Info, "Open a notebook first."), nil
	}
	var items []picker.Item
	for _, uid := range m.markUIDs {
		rel, ok := m.links.ByUID(uid)
		if !ok {
			continue
		}
		it := picker.Item{
			Label: textsafe.Sanitize(strings.TrimSuffix(filepath.ToSlash(rel), filepath.Ext(rel))),
			Name:  textsafe.Sanitize(noteName(rel)),
			Value: rel,
		}
		it.Group, it.Trim = markGroup(it.Label)
		items = append(items, it)
	}
	if len(items) == 0 {
		return m.notify(popup.Info, "No marked notes. Press m on a note to mark it."), nil
	}
	slices.SortFunc(items, compareMarks)
	p, cmd := picker.New(marksTitle, items)
	p = p.Large().Tall().Centered().WithHint(marksHint).WithCursor(m.currentMarkIndex(items))
	m.picker, m.pickerOp = &p, pickMark
	return m, cmd
}

// currentMarkIndex is the position in items of the note under the PARA
// cursor, or 0 when that note is not among them.
func (m model) currentMarkIndex(items []picker.Item) int {
	e, ok := m.para.SelectedNote()
	if !ok {
		return 0
	}
	return max(slices.IndexFunc(items, func(it picker.Item) bool { return it.Value == e.Rel }), 0)
}

// toggleMark marks the note at rel with a bullet in PARA, or clears the mark
// when it has one. A mark follows the note's uid, so a note without one is
// given one first, as any edit does (and undo can take back).
func (m model) toggleMark(rel string) model {
	uid, err := m.noteUID(rel)
	if err != nil {
		return m.notify(popup.Warning, "Could not read the note: "+err.Error()+".")
	}
	if uid != "" && slices.Contains(m.markUIDs, uid) {
		return m.setMarks(slices.DeleteFunc(slices.Clone(m.markUIDs), func(u string) bool { return u == uid }))
	}
	if len(m.markUIDs) >= marks.Max {
		return m.notify(popup.Info, fmt.Sprintf("You can mark at most %d notes. Clear a mark first.", marks.Max))
	}
	if uid == "" {
		m = m.editNote(rel, "mark", func(*store.Frontmatter) {})
		if uid, err = m.noteUID(rel); err != nil || uid == "" {
			return m
		}
	}
	return m.setMarks(append(slices.Clone(m.markUIDs), uid))
}

// setMarks replaces the marked uids, shows them in PARA and saves them.
func (m model) setMarks(uids []string) model {
	m.markUIDs = uids
	m.para = m.para.WithMarks(uids)
	return m.saveMarks()
}

// saveMarks writes the marks file of the open notebook.
func (m model) saveMarks() model {
	if err := marks.Save(m.session.notebook, m.markUIDs); err != nil {
		return m.notify(popup.Warning, fmt.Sprintf("Could not save your marks: %s.", err))
	}
	return m
}

// loadMarks reads the marks file of the open notebook into PARA. A notebook
// without the file takes the marks an older para kept in the state database,
// and writes them to the file so that happens once.
func (m model) loadMarks() model {
	m.markUIDs = nil
	m.para = m.para.WithMarks(nil)
	uids, exists, err := marks.Load(m.session.notebook)
	if err != nil {
		return m.notify(popup.Warning, fmt.Sprintf("Could not restore your marks: %s.", err))
	}
	if !exists {
		uids = m.legacyMarks()
		if len(uids) > 0 {
			if err := marks.Save(m.session.notebook, uids); err != nil {
				m = m.notify(popup.Warning, fmt.Sprintf("Could not move your marks into the notebook: %s.", err))
			}
		}
	}
	m.markUIDs = uids
	m.para = m.para.WithMarks(uids)
	return m
}

// legacyMarks returns the marks the state database holds for the open
// notebook, or none when persistence is off or the read fails.
func (m model) legacyMarks() []string {
	if m.session.db == nil {
		return nil
	}
	uids, err := m.session.db.LoadMarks(m.session.notebook)
	if err != nil {
		return nil
	}
	return uids
}

// paraOrder is the order the top folders are listed in: Inbox, then PARA.
var paraOrder = []string{"inbox", "projects", "areas", "resources", "archives"}

// markGroup is the headings a marked note sits under: its top folder, then
// its second folder when it has one (a note directly in a folder has no
// second heading), and how much of the path those headings already say.
// label is the note's path without its extension, slash separated.
func markGroup(label string) (group []string, trim int) {
	parts := strings.Split(label, "/")
	depth := min(len(parts)-1, 2) // the last part is the note, not a folder
	if depth == 0 {
		return nil, 0
	}
	group = slices.Clone(parts[:depth])
	if i := slices.Index(paraOrder, group[0]); i >= 0 {
		group[0] = strings.ToUpper(group[0][:1]) + group[0][1:]
	}
	return group, len(strings.Join(parts[:depth], "/")) + 1
}

// compareMarks orders the marked notes by top folder (Inbox, Projects,
// Areas, Resources, Archives, then any other folder, then the notebook root),
// then by path.
func compareMarks(a, b picker.Item) int {
	if c := cmp.Compare(markRank(a), markRank(b)); c != 0 {
		return c
	}
	return strings.Compare(a.Label, b.Label)
}

func markRank(it picker.Item) int {
	if len(it.Group) == 0 {
		return len(paraOrder) + 1
	}
	if i := slices.Index(paraOrder, strings.ToLower(it.Group[0])); i >= 0 {
		return i
	}
	return len(paraOrder)
}
