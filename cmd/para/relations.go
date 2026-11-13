package main

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tkilb/para/internal/links"
	"github.com/tkilb/para/internal/store"
	"github.com/tkilb/para/internal/ui/picker"
	"github.com/tkilb/para/internal/ui/popup"
)

const addReferenceTitle = "Add reference"

// pickerOp says what the open picker is for.
type pickerOp int

const (
	pickReference      pickerOp = iota // add a reference to the note at pickerFor
	pickKind                           // set the kind of the note at pickerFor
	pickExtractKind                    // choose the kind of a note being extracted
	pickNotebook                       // choose the notebook to open
	pickFile                           // jump to a note found by name
	pickAppendTemplate                 // append a template to the note at pickerFor
	pickGlossary                       // browse the glossary (see glossary.go)
)

// openReferencePicker lists the notes the shown note could reference: every
// note except itself and those it already references.
func (m model) openReferencePicker() (model, tea.Cmd) {
	rel := m.related.Subject()
	taken := map[string]bool{}
	for _, l := range m.links.References(rel) {
		taken[strings.ToLower(l.Name)] = true
	}
	var items []picker.Item
	for _, n := range m.links.Notes() {
		if n.Rel == rel || taken[strings.ToLower(n.Name)] {
			continue
		}
		items = append(items, picker.Item{Label: n.Name, Detail: folderOf(n.Rel), Value: n.Name})
	}
	if len(items) == 0 {
		return m.notify(popup.Info, "There are no other notes to reference."), nil
	}
	p, cmd := picker.New(addReferenceTitle, items)
	m.picker, m.pickerFor, m.pickerOp = &p, rel, pickReference
	return m, cmd
}

// folderOf is the first folder of rel, the PARA tab the note lives in.
func folderOf(rel string) string {
	if i := strings.Index(rel, "/"); i > 0 {
		return rel[:i]
	}
	return ""
}

// pickerKey feeds a message to the open picker and applies its outcome.
func (m model) pickerKey(msg tea.Msg) (model, tea.Cmd) {
	if m.pickerOp == pickGlossary {
		if next, cmd, handled := m.glossaryKey(msg); handled {
			return next, cmd
		}
	}
	p, cmd, out := m.picker.WithScreenHeight(m.height).Update(msg)
	m.picker = &p
	switch out {
	case picker.Cancelled:
		m.picker = nil
	case picker.Chosen:
		item, _ := p.Chosen()
		m.picker = nil
		switch m.pickerOp {
		case pickNotebook:
			return m.chooseNotebook(item.Value)
		case pickFile:
			return m.chooseFoundNote(item.Value), nil
		case pickKind:
			return m.setKind(m.pickerFor, store.ParseKind(item.Value)), nil
		case pickExtractKind:
			return m.askExtractTitle(store.ParseKind(item.Value))
		case pickAppendTemplate:
			return m.appendTemplate(m.pickerFor, item.Value), nil
		case pickGlossary:
			return m.showGlossaryEntry(parseGlossaryValue(item.Value))
		}
		return m.editNote(m.pickerFor, fmt.Sprintf("add reference to %s", item.Value),
			func(fm *store.Frontmatter) { links.AddReference(fm, item.Value) }), nil
	}
	return m, cmd
}

// askRemoval asks before removing the reference name from the note at rel.
func (m model) askRemoval(rel, name string) model {
	prompt := fmt.Sprintf("Remove the reference to %q from %s?", name, strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel)))
	return m.ask(prompt, func(m model) model {
		return m.editNote(rel, "remove reference to "+name,
			func(fm *store.Frontmatter) { links.RemoveReference(fm, name) })
	})
}

// editNote changes the properties of the note at rel, records the undo
// action and refreshes the panels. A failure is shown as an error popup.
func (m model) editNote(rel, what string, edit func(*store.Frontmatter)) model {
	act, err := m.store.Edit(rel, edit)
	if err != nil {
		return m.notify(popup.Error, capitalize(fmt.Sprintf("%s failed: %s.", what, err)))
	}
	m.history.Push(act)
	m.para = m.para.Reload()
	return m.refreshLinks()
}
