package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tkilb/para/internal/editor"
	"github.com/tkilb/para/internal/glossary"
	"github.com/tkilb/para/internal/ui/popup"
)

const (
	// glossaryDraftPattern names the temporary file a term is edited in.
	glossaryDraftPattern = "para-glossary-*.md"
	draftPerm            = 0o600
)

// glossaryEditDoneMsg arrives when the editor opened on a glossary buffer
// exits. path is the draft file, which is read and removed, and old is the
// entry being replaced (empty for a new term).
type glossaryEditDoneMsg struct {
	old  glossary.Entry
	path string
	err  error
}

// editGlossaryBuffer writes the entry to a private temporary file as a
// Term and Definition buffer and opens $EDITOR on it, handing the terminal
// over until it exits.
func (m model) editGlossaryBuffer(old glossary.Entry) (model, tea.Cmd) {
	path, err := writeGlossaryDraft(glossary.Buffer(old))
	if err != nil {
		return m.glossaryFailed("opening the editor", err), nil
	}
	cmd, err := editor.Command(path, os.Environ())
	if err != nil {
		_ = os.Remove(path)
		return m.glossaryFailed("opening the editor", err), nil
	}
	return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
		return glossaryEditDoneMsg{old: old, path: path, err: err}
	})
}

// writeGlossaryDraft creates the draft file holding text and returns its path.
func writeGlossaryDraft(text string) (string, error) {
	f, err := os.CreateTemp("", glossaryDraftPattern)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if err := os.Chmod(f.Name(), draftPerm); err != nil {
		_ = os.Remove(f.Name())
		return "", err
	}
	if _, err := f.WriteString(text); err != nil {
		_ = os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// finishGlossaryEdit saves what the editor left in the draft. An editor that
// exits with an error cancels the edit, and a buffer with no term says so;
// either way the glossary opens again unchanged.
func (m model) finishGlossaryEdit(msg glossaryEditDoneMsg) (model, tea.Cmd) {
	data, readErr := os.ReadFile(msg.path)
	_ = os.Remove(msg.path)
	back := func(m model) (model, tea.Cmd) {
		next, cmd := m.openGlossary(msg.old)
		return next, tea.Batch(cmd, tea.ClearScreen)
	}
	if msg.err != nil {
		return back(m.notify(popup.Error, capitalize(fmt.Sprintf("could not run the editor: %s.", msg.err))))
	}
	if readErr != nil {
		return m.glossaryFailed("reading the glossary buffer", readErr), tea.ClearScreen
	}
	e, err := glossary.ParseBuffer(string(data))
	if err != nil {
		return back(m.notify(popup.Warning, capitalize(err.Error())+"."))
	}
	next, cmd := m.saveGlossaryEntry(msg.old, e)
	return next, tea.Batch(cmd, tea.ClearScreen)
}
