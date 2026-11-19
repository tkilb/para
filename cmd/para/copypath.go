package main

import (
	"fmt"

	"github.com/tkilb/para/internal/ui/popup"
)

// copyPath copies the absolute path of the folder or file under the PARA
// cursor to the system clipboard. A failure is shown as an error.
func (m model) copyPath() model {
	entry, ok := m.para.Selected()
	if !ok {
		return m.notify(popup.Info, "Nothing is selected to copy a path from.")
	}
	path, err := m.store.Path(entry.Rel)
	if err != nil {
		return m.notify(popup.Warning, capitalize(fmt.Sprintf("could not resolve the path: %s.", err)))
	}
	if err := m.writeClipboard(path); err != nil {
		return m.notify(popup.Error, capitalize(fmt.Sprintf(
			"could not copy to the clipboard: %s (it needs pbcopy on macOS, or xclip, xsel or wl-copy on Linux).", err)))
	}
	return m.notify(popup.Info, "Copied "+path)
}
