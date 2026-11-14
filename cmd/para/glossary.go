package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tkilb/para/internal/glossary"
	"github.com/tkilb/para/internal/textsafe"
	"github.com/tkilb/para/internal/ui/picker"
	"github.com/tkilb/para/internal/ui/popup"
)

const (
	glossaryTitle = "Glossary"
	glossaryHint  = "enter show  ctrl+n add  ctrl+e edit  ctrl+x delete  esc cancel"
)

// notebookRoot is the folder of the open notebook, or "" when none is open.
func (m model) notebookRoot() string {
	if m.activeNotebook < 0 || m.activeNotebook >= len(m.notebooks) {
		return ""
	}
	return m.notebooks[m.activeNotebook].Path
}

// glossaryFailed tells the user a glossary read or write failed.
func (m model) glossaryFailed(what string, err error) model {
	return m.notify(popup.Error, capitalize(fmt.Sprintf("%s failed: %s.", what, err)))
}

// glossaryValue is the picker value of an entry: a term can have several
// meanings, so the definition is part of the identity (a NUL byte cannot be
// in either text).
func glossaryValue(e glossary.Entry) string { return e.Term + valueSep + e.Definition }

// parseGlossaryValue is the inverse of glossaryValue.
func parseGlossaryValue(v string) glossary.Entry {
	term, def, _ := strings.Cut(v, valueSep)
	return glossary.Entry{Term: term, Definition: def}
}

// openGlossary lists the entries in glossary.yaml of the open notebook, in
// alphabetical order of term (the file's own order is left alone), the
// highlight on the entry given (when it is there). An empty glossary still
// opens, so the first term can be added from it.
func (m model) openGlossary(at glossary.Entry) (model, tea.Cmd) {
	root := m.notebookRoot()
	if root == "" {
		return m.notify(popup.Info, "Open a notebook first."), nil
	}
	entries, err := glossary.Load(root)
	if err != nil {
		return m.glossaryFailed("reading the glossary", err), nil
	}
	entries = glossary.Sorted(entries)
	items := make([]picker.Item, len(entries))
	cursor := 0
	for i, e := range entries {
		items[i] = picker.Item{
			Label:   textsafe.Sanitize(e.Term),
			Detail:  firstLine(textsafe.Sanitize(e.Definition)),
			Preview: glossary.Spaced(textsafe.Sanitize(e.Definition)),
			Value:   glossaryValue(e),
		}
		if e == at {
			cursor = i
		}
	}
	p, cmd := picker.New(glossaryTitle, items)
	p = p.Large().WithPreview("Definition").WithHint(glossaryHint).WithCursor(cursor)
	m.picker, m.pickerOp = &p, pickGlossary
	return m, cmd
}

// firstLine is s up to its first line break.
func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}

// glossaryKey handles the glossary's own keys while its picker is open:
// ctrl+n adds a term, ctrl+e edits the highlighted one, ctrl+x deletes it.
// handled is false for every other key, which the picker takes as usual.
func (m model) glossaryKey(msg tea.Msg) (next model, cmd tea.Cmd, handled bool) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil, false
	}
	switch key.String() {
	case "ctrl+n":
		m.picker = nil
		next, cmd = m.askGlossaryTerm(glossary.Entry{})
		return next, cmd, true
	case "ctrl+e", "ctrl+x":
		item, has := m.picker.Chosen()
		if !has {
			return m, nil, true // nothing highlighted: the key does nothing
		}
		m.picker = nil
		if key.String() == "ctrl+x" {
			return m.askGlossaryDelete(parseGlossaryValue(item.Value)), nil, true
		}
		next, cmd = m.askGlossaryTerm(parseGlossaryValue(item.Value))
		return next, cmd, true
	}
	return m, nil, false
}

// showGlossaryEntry shows the whole definition of an entry in a popup and
// puts the glossary back, so a long definition can be read in full.
func (m model) showGlossaryEntry(e glossary.Entry) (model, tea.Cmd) {
	m = m.notify(popup.Info, textsafe.Sanitize(e.Term)+"\n\n"+textsafe.Sanitize(e.Definition))
	return m.openGlossary(e)
}

// askGlossaryTerm opens $EDITOR on a buffer holding the term and its
// definition (see glossary.Buffer). With an old entry (old.Term not empty) it
// replaces that entry, which pre-fills the buffer; with none it adds a new
// one, which is another meaning when the term exists.
func (m model) askGlossaryTerm(old glossary.Entry) (model, tea.Cmd) {
	return m.editGlossaryBuffer(old)
}

// saveGlossaryEntry writes the entry (replacing old, when it names one) and
// goes back to the glossary with the highlight on it. The glossary is part of
// the notebook, so git state is read again.
func (m model) saveGlossaryEntry(old, e glossary.Entry) (model, tea.Cmd) {
	var err error
	if old.Term == "" {
		err = glossary.Add(m.notebookRoot(), e.Term, e.Definition)
	} else {
		err = glossary.Replace(m.notebookRoot(), old, e)
	}
	if err != nil {
		return m.glossaryFailed("saving the term", err), nil
	}
	m.syncDirty = true
	e.Term, e.Definition = strings.TrimSpace(e.Term), strings.TrimSpace(e.Definition)
	return m.openGlossary(e)
}

// askGlossaryDelete asks before deleting an entry, which cannot be undone
// with u (the glossary is not part of the undo history).
func (m model) askGlossaryDelete(e glossary.Entry) model {
	prompt := fmt.Sprintf("Delete %q from the glossary? This cannot be undone.", textsafe.Sanitize(e.Term+": "+firstLine(e.Definition)))
	return m.ask(prompt, func(m model) model {
		if err := glossary.Delete(m.notebookRoot(), e); err != nil {
			return m.glossaryFailed("deleting the term", err)
		}
		m.syncDirty = true
		next, _ := m.openGlossary(glossary.Entry{})
		return next
	})
}
