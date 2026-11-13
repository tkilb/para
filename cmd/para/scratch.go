package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tkilb/para/internal/editor"
	"github.com/tkilb/para/internal/ui/popup"
)

// scratchFile is the scratch pad, in the notebook root.
const scratchFile = "_scratch.md"

// scratchDoneMsg arrives when the editor opened on the scratch pad exits; err
// is nil on success.
type scratchDoneMsg struct{ err error }

// openScratch opens the scratch pad in $EDITOR, handing the terminal over
// until it exits. The editor creates the file when it saves it.
func (m model) openScratch() (model, tea.Cmd) {
	path, err := m.store.Path(scratchFile)
	if err != nil {
		return m.notify(popup.Error, fmt.Sprintf("Could not open the scratch pad: %s.", err)), nil
	}
	cmd, err := editor.Command(path, os.Environ())
	if err != nil {
		return m.notify(popup.Error, fmt.Sprintf("Could not open the scratch pad: %s.", err)), nil
	}
	return m, tea.ExecProcess(cmd, func(err error) tea.Msg { return scratchDoneMsg{err: err} })
}

// finishScratch refreshes from disk, since the scratch pad may now exist or
// have changed, and reports a failure to run the editor. Unlike a note, the
// scratch pad keeps its file name whatever its first heading says.
func (m model) finishScratch(msg scratchDoneMsg) model {
	m.para = m.para.Reload()
	m = m.refreshLinks()
	if msg.err != nil {
		return m.notify(popup.Error, fmt.Sprintf("Could not run the editor: %s.", msg.err))
	}
	return m
}
