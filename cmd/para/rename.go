package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/tkilb/para/internal/links"
	"github.com/tkilb/para/internal/store"
	"github.com/tkilb/para/internal/ui/para"
	"github.com/tkilb/para/internal/ui/popup"
	"github.com/tkilb/para/internal/undo"
)

// startRename carries out a rename asked for by the PARA panel. When other
// notes reference the note it first asks, saying how many it will update.
func (m model) startRename(req *para.RenameRequest) model {
	if n := len(m.links.ReferencedBy(req.Rel)); !req.IsDir && n > 0 {
		prompt := fmt.Sprintf("Rename %s to %s? This updates %s that reference it.",
			filepath.Base(req.Rel), req.Name, noteCount(n))
		return m.ask(prompt, func(m model) model { return m.doRename(req) })
	}
	return m.doRename(req)
}

// doRename renames, records one undo entry, moves the cursor to the renamed
// item and reports anything that could not be updated.
func (m model) doRename(req *para.RenameRequest) model {
	act, res, err := m.renameAction(req)
	if err != nil {
		return m.notify(popup.Error, capitalize(fmt.Sprintf("rename failed: %s.", err)))
	}
	m.history.Push(act)
	newRel := filepath.Join(filepath.Dir(req.Rel), req.Name)
	m.content = m.content.Renamed(req.Rel, newRel)
	m.para = m.para.Reload()
	if p, ok := m.para.Reveal(newRel); ok {
		m.para = p
	}
	m = m.refreshLinks()
	return m.warnRename(res)
}

// renameAction does the rename. An image is only moved, because it has no
// title and no note links to it.
func (m model) renameAction(req *para.RenameRequest) (undo.Action, links.RenameResult, error) {
	if !req.IsImage {
		return links.Rename(m.store, m.links, req.Rel, req.IsDir, req.Typed)
	}
	act, err := m.store.Move(req.Rel, filepath.Join(filepath.Dir(req.Rel), req.Name))
	return act, links.RenameResult{}, err
}

// warnRename reports what a rename could not update.
func (m model) warnRename(res links.RenameResult) model {
	if res.TitleKept {
		m = m.notify(popup.Warning, "The note's properties could not be edited, so its title property was left as it was.")
	}
	if res.Skipped > 0 {
		m = m.notify(popup.Warning, fmt.Sprintf("%s could not be updated and still link to the old name.", noteCount(res.Skipped)))
	}
	return m
}

// followTitle renames a note just edited in the editor to match its title
// property, when the title was changed, adding _1, _2 and so on if the name is
// taken. The rename is one undo entry; the edit itself is not undoable.
func (m model) followTitle(rel string) model {
	got, err := links.RenameToTitle(m.store, m.links, rel)
	switch {
	case errors.Is(err, store.ErrNotFound):
		return m // the editor moved or removed the file: nothing to follow
	case errors.Is(err, links.ErrUnusableTitle):
		return m.notify(popup.Warning, "The title has no characters usable in a file name, so the note was not renamed.")
	case err != nil:
		return m.notify(popup.Error, capitalize(fmt.Sprintf("could not rename the note to match its title: %s.", err)))
	case got.CaseOnly:
		return m.notify(popup.Info, "The title differs from the file name only by case, so the note was not renamed.")
	case !got.Changed:
		return m
	}
	m.history.Push(got.Action)
	m.content = m.content.Renamed(rel, got.Rel)
	m.para = m.para.Reload()
	if p, ok := m.para.Reveal(got.Rel); ok {
		m.para = p
	}
	m = m.refreshLinks()
	if got.Collided {
		m = m.notify(popup.Info, "That name was taken, so the note is now "+strings.TrimSuffix(filepath.Base(got.Rel), ".md")+".")
	}
	return m.warnRename(got.Result)
}

// noteCount words a number of notes.
func noteCount(n int) string {
	if n == 1 {
		return "1 note"
	}
	return fmt.Sprintf("%d notes", n)
}
