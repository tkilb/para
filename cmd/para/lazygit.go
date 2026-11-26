package main

import (
	"errors"
	"fmt"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tkilb/para/internal/ui/popup"
)

const lazygitCommand = "lazygit"

// lazygitDoneMsg arrives when lazygit exits; err is nil on success.
type lazygitDoneMsg struct{ err error }

// openLazygit runs lazygit in the open notebook's folder, handing the terminal
// over until it exits, so a commit needs no cd. A missing lazygit or notebook
// is reported as a warning and nothing is suspended.
func (m model) openLazygit() (model, tea.Cmd) {
	if m.activeNotebook < 0 || m.activeNotebook >= len(m.notebooks) {
		return m.notify(popup.Warning, "There is no open notebook to run lazygit in."), nil
	}
	path, err := exec.LookPath(lazygitCommand)
	if err != nil {
		return m.notify(popup.Warning, "lazygit is not installed or not on your PATH."), nil
	}
	cmd := exec.Command(path)
	cmd.Dir = m.notebooks[m.activeNotebook].Path
	m = m.saveOrWarn()
	return m, tea.ExecProcess(cmd, func(err error) tea.Msg { return lazygitDoneMsg{err: err} })
}

// finishLazygit refreshes from disk, since lazygit may have changed, pulled or
// checked out files, and reads the git state again for the status bar.
func (m model) finishLazygit(msg lazygitDoneMsg) (model, tea.Cmd) {
	m.para = m.para.Reload()
	m.skills = m.skills.Reload()
	m = m.refreshLinks()
	var exitErr *exec.ExitError
	if msg.err != nil && !errors.As(msg.err, &exitErr) {
		m = m.notify(popup.Error, fmt.Sprintf("Could not run lazygit: %s.", msg.err))
	}
	m, syncCmd := m.forceSync()
	return m, tea.Batch(tea.ClearScreen, syncCmd)
}
