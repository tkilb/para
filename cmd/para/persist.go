package main

import (
	"fmt"

	"github.com/tkilb/para/internal/state"
	"github.com/tkilb/para/internal/ui/popup"
)

// session is the saved state for the running notebook. A nil db means
// persistence is off, because the database could not be opened.
type session struct {
	db       *state.DB
	notebook string
}

// openSession opens the state database for the notebook at root. On failure
// it returns a session with persistence off and the reason.
func openSession(root string) (session, error) {
	dir, err := state.Dir()
	if err != nil {
		return session{notebook: root}, err
	}
	db, err := state.Open(dir)
	if err != nil {
		return session{notebook: root}, err
	}
	return session{db: db, notebook: root}, nil
}

// withSession restores the saved position and focus into m and keeps s for
// saving later. A problem reading the saved state is reported as a warning;
// the app then starts from the defaults.
func (m model) withSession(s session) model {
	m.session = s
	if s.db == nil {
		return m
	}
	m = m.loadStickies().loadMarks()
	nb, err := s.db.LoadNotebook(s.notebook)
	if err != nil {
		return m.notify(popup.Warning, fmt.Sprintf("Could not restore your last position: %s.", err))
	}
	m.para = m.para.Restore(nb)
	if sess, err := s.db.LoadSession(); err == nil && isPanel(sess.Focus) {
		m.focus = panelID(sess.Focus)
	}
	return m
}

// save writes the current position and focus. It does nothing when
// persistence is off.
func (m model) save() error {
	if m.session.db == nil {
		return nil
	}
	if err := m.session.db.SaveNotebook(m.session.notebook, m.para.Snapshot()); err != nil {
		return err
	}
	return m.session.db.SaveSession(state.Session{Notebook: m.session.notebook, Focus: int(m.focus)})
}

// saveOrWarn saves and reports a failure as a warning popup.
func (m model) saveOrWarn() model {
	if err := m.save(); err != nil {
		return m.notify(popup.Warning, fmt.Sprintf("Could not save your position: %s.", err))
	}
	return m
}
