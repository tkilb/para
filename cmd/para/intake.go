package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tkilb/para/internal/intake"
	"github.com/tkilb/para/internal/ui/noteform"
	"github.com/tkilb/para/internal/ui/para"
	"github.com/tkilb/para/internal/ui/popup"
)

// startQuickAdd opens the Add Note form. The note is saved in the folder PARA
// is showing when the form is submitted.
func (m model) startQuickAdd() (model, tea.Cmd) {
	form, cmd, err := m.newQuickAddForm()
	if err != nil {
		m = m.notify(popup.Warning, capitalize(fmt.Sprintf("the templates could not be listed (%s).", err)))
	}
	m.form = &form
	m.formSave = func(m model, v noteform.Values) (model, tea.Cmd) {
		if strings.TrimSpace(v.Title) == "" && strings.TrimSpace(v.Body) == "" {
			return m, nil // nothing typed
		}
		tags, err := intake.ParseTags(v.Tags)
		if err != nil {
			return m.reopenForm(v, noteform.FieldTags, err)
		}
		res, err := intake.CaptureIn(m.store, time.Now(), m.para.Dir(), v.Kind, v.Source, v.Title, v.Body, tags)
		if errors.Is(err, intake.ErrEmpty) {
			return m.reopenForm(v, noteform.FieldTitle, err)
		}
		return m.finishIntake(res, err), nil
	}
	return m, cmd
}

// reopenForm puts the form back after a mistake, with a warning and what was
// typed, so a typo in the tags does not lose a long body.
func (m model) reopenForm(v noteform.Values, f noteform.Field, why error) (model, tea.Cmd) {
	form, _, listErr := m.newQuickAddForm()
	form, cmd := form.WithValues(v, f)
	m.form = &form
	if listErr != nil {
		m = m.notify(popup.Warning, capitalize(fmt.Sprintf("the templates could not be listed (%s).", listErr)))
	}
	return m.notify(popup.Warning, capitalize(why.Error())+"."), cmd
}

// discardPrompt is asked when esc would throw away a form the user changed.
const discardPrompt = "Discard the note you started?"

// formKey handles a key while the Add Note form is open: esc cancels (after
// asking, when the form has been changed), enter
// in any single-line field and ctrl+s anywhere save, and everything else
// edits the form.
func (m model) formKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	save := func() (tea.Model, tea.Cmd) {
		v, onSave := m.form.Values(), m.formSave
		m.form = nil
		return onSave(m, v)
	}
	switch msg.String() {
	case "esc":
		if m.form.Dirty() {
			return m.ask(discardPrompt, func(m model) model {
				m.form, m.formSave = nil, nil
				return m
			}), nil
		}
		m.form, m.formSave = nil, nil
		return m, nil
	case "ctrl+s":
		return save()
	case "enter":
		if m.form.FocusedField() != noteform.FieldBody {
			return save()
		}
	}
	prev := m.form.TemplateIndex()
	f, cmd := m.form.Update(msg)
	if f.TemplateIndex() != prev {
		return m.templateChanged(f, prev), cmd
	}
	m.form = &f
	return m, cmd
}

// openRootInput opens a one-line prompt, starting with initial, whose text
// goes to accept.
func (m model) openRootInput(title, initial string, accept func(model, string) (model, tea.Cmd)) (model, tea.Cmd) {
	in, cmd := m.openInput(&para.Prompt{Title: title, Initial: initial})
	in.accept = accept
	m.input = in
	return m, cmd
}

// savedPlace says where a new note went: "the Inbox" for the inbox folder
// itself, else its folder, such as "projects/alpha".
func savedPlace(rel string) string {
	if dir := filepath.ToSlash(filepath.Dir(rel)); dir != "inbox" {
		return dir
	}
	return "the Inbox"
}

// finishIntake records the undo entry for a created note and refreshes the
// panels. Nothing is said when the new note shows in the current list; when
// it does not, a notice says where it went.
func (m model) finishIntake(res intake.Result, err error) model {
	if err != nil {
		return m.notify(popup.Error, capitalize(fmt.Sprintf("could not add the note: %s.", err)))
	}
	m.history.Push(res.Action)
	m.para = m.para.Reload()
	m = m.refreshLinks()
	if res.Unlinked {
		m = m.notify(popup.Warning, "The note's name has characters a link cannot hold (| # [ ] ^), so the new note does not link back to it.")
	}
	if !m.para.Shows(res.Rel) {
		m = m.notify(popup.Info, "Saved to "+savedPlace(res.Rel)+" as "+strings.TrimSuffix(filepath.Base(res.Rel), ".md")+".")
	}
	return m
}
