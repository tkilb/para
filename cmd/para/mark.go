package main

import (
	"fmt"
	"slices"

	"github.com/tkilb/para/internal/state"
	"github.com/tkilb/para/internal/store"
	"github.com/tkilb/para/internal/ui/popup"
)

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
	if len(m.markUIDs) >= state.MaxMarks {
		return m.notify(popup.Info, fmt.Sprintf("You can mark at most %d notes. Clear a mark first.", state.MaxMarks))
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

// saveMarks writes the marks of the open notebook. It does nothing when
// persistence is off.
func (m model) saveMarks() model {
	if m.session.db == nil {
		return m
	}
	if err := m.session.db.SaveMarks(m.session.notebook, m.markUIDs); err != nil {
		return m.notify(popup.Warning, fmt.Sprintf("Could not save your marks: %s.", err))
	}
	return m
}

// loadMarks reads the saved marks of the open notebook into PARA.
func (m model) loadMarks() model {
	m.markUIDs = nil
	m.para = m.para.WithMarks(nil)
	if m.session.db == nil {
		return m
	}
	uids, err := m.session.db.LoadMarks(m.session.notebook)
	if err != nil {
		return m.notify(popup.Warning, fmt.Sprintf("Could not restore your marks: %s.", err))
	}
	m.markUIDs = uids
	m.para = m.para.WithMarks(uids)
	return m
}
