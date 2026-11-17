package main

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tkilb/para/internal/links"
	"github.com/tkilb/para/internal/ui/popup"
	"github.com/tkilb/para/internal/ui/related"
)

// relatedKey sends a key to the Related panel and applies what it asks for:
// a jump selects the note in PARA and moves focus there.
func (m model) relatedKey(msg tea.KeyMsg) (model, tea.Cmd) {
	var eff related.Effect
	m.related, eff = m.related.Update(msg)
	if eff.Info != "" {
		m = m.notify(popup.Info, eff.Info)
	}
	switch {
	case eff.Add:
		return m.openReferencePicker()
	case eff.Remove != "":
		return m.askRemoval(m.related.Subject(), eff.Remove), nil
	}
	if eff.Jump != "" {
		para, ok := m.para.Reveal(eff.Jump)
		if !ok {
			return m.notify(popup.Warning, "That note is outside the PARA folders."), nil
		}
		m.para, m.focus = para, panelPARA
	}
	return m, nil
}

// refreshLinks brings the link index up to date, building it when it does
// not exist yet, and makes the panels reload the selected note so Related
// shows the new links. A failure keeps the old index and is reported.
func (m model) refreshLinks() model {
	var err error
	if m.links == nil {
		m.links, err = links.Build(m.store)
	} else {
		err = m.links.Refresh(m.store)
	}
	if err != nil {
		m = m.notify(popup.Warning, fmt.Sprintf("Could not read the related links: %s.", err))
	}
	m.shown = shownNote{}
	m.syncDirty = true // the notes changed, so the git state may have
	return m
}
