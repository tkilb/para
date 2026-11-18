package para

import (
	"fmt"
	"strings"

	"github.com/tkilb/para/internal/store"
)

// RenameRequest is a validated rename of the item at Rel to Name, the new
// file or folder name (a note's name includes .md, an image's .png).
type RenameRequest struct {
	Rel   string
	IsDir bool
	// IsImage is set for a PNG image, which is renamed on its own: no title
	// property and no references to update.
	IsImage bool
	// Typed is what the user typed; Name is it cleaned.
	Typed, Name string
}

// askRename opens the rename prompt for the hovered item, starting with its
// current name (a note's without .md).
func (m Model) askRename() (Model, Effect) {
	e, ok := m.Selected()
	if !ok {
		return m, Effect{}
	}
	name := e.Name
	switch {
	case e.IsImage:
		name = rowName(e)
	case !e.IsDir:
		name = strings.TrimSuffix(strings.TrimSuffix(name, ".md"), ".MD")
	}
	return m, Effect{Prompt: &Prompt{Title: "Rename", Initial: name, Op: OpRename}}
}

// rename validates the typed name and hands the rename to the root model. A
// name that is the same as the current one does nothing.
func (m Model) rename(text string) (Model, Effect) {
	e, ok := m.Selected()
	if !ok {
		return m, Effect{}
	}
	clean := func() (string, error) { return store.CleanRenamedName(text, e.IsDir) }
	if e.IsImage {
		clean = func() (string, error) { return store.CleanRenamedImageName(text) }
	}
	name, err := clean()
	if err != nil {
		return m, Effect{Err: fmt.Errorf("rename: %w", err)}
	}
	if name == e.Name {
		return m, Effect{Info: "The name is unchanged."}
	}
	return m, Effect{Rename: &RenameRequest{Rel: e.Rel, IsDir: e.IsDir, IsImage: e.IsImage, Typed: text, Name: name}}
}
