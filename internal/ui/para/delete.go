package para

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/tkilb/para/internal/store"
	"github.com/tkilb/para/internal/undo"
)

// askDelete asks for confirmation before trashing or permanently deleting
// the targets, warning about folders that are not empty.
func (m Model) askDelete(op ConfirmOp) (Model, Effect) {
	targets := m.targets()
	if len(targets) == 0 {
		return m, Effect{}
	}
	prompt, err := m.deletePrompt(op, targets)
	if err != nil {
		return m, Effect{Err: fmt.Errorf("inspect: %w", err)}
	}
	return m, Effect{Confirm: &Confirm{Prompt: prompt, Op: op, Danger: true}}
}

// deletePrompt words the question, e.g. "Trash folder alpha/ (12 items
// inside, 4 not shown)?". The counts come from the disk, so files the
// listing hides are included.
func (m Model) deletePrompt(op ConfirmOp, targets []store.Entry) (string, error) {
	verb := "Trash"
	if op == OpDelete {
		verb = "Permanently delete"
	}

	var inside, hidden, folders int
	for _, e := range targets {
		if !e.IsDir {
			continue
		}
		info, err := m.store.Inspect(e.Rel)
		if err != nil {
			return "", err
		}
		folders++
		inside += info.Items
		hidden += info.Unlisted
	}

	var what string
	switch {
	case len(targets) == 1 && targets[0].IsDir:
		what = "folder " + targets[0].Name + "/" + folderDetail(inside, hidden)
	case len(targets) == 1:
		what = targets[0].Name
	default:
		what = count(len(targets))
		if folders > 0 {
			what += folderDetail(inside, hidden)
		}
	}
	prompt := verb + " " + what + "?"
	if op == OpDelete {
		prompt += " This cannot be undone."
	}
	return prompt, nil
}

// folderDetail describes what folders hold, e.g. " (12 items inside, 4 not
// shown)", or " (empty)".
func folderDetail(inside, hidden int) string {
	if inside == 0 {
		return " (empty)"
	}
	parts := []string{count(inside) + " inside"}
	if hidden > 0 {
		parts = append(parts, fmt.Sprintf("%d not shown", hidden))
	}
	return " (" + strings.Join(parts, ", ") + ")"
}

// trash moves the targets to the trash as one undo entry. If a step fails,
// the earlier ones are undone.
func (m Model) trash() (Model, Effect) {
	targets := m.targets()
	var done []undo.Action
	for _, e := range targets {
		a, err := m.store.Trash(e.Rel)
		if err != nil {
			if len(done) > 0 {
				_ = undo.Group("rollback", done...).Undo()
			}
			return m.Reload(), Effect{Err: fmt.Errorf("trash failed: %w", err)}
		}
		done = append(done, a)
	}
	if len(done) == 0 {
		return m, Effect{}
	}
	desc := "trash " + describe(len(targets), targets[0].Name)
	group := undo.Group(desc, done...)
	return m.clearSelection().Reload(), Effect{Action: &group}
}

// deletePermanently removes the targets for good. It stops at the first
// failure and says how many were removed.
func (m Model) deletePermanently() (Model, Effect) {
	targets := m.targets()
	for i, e := range targets {
		if err := m.store.Delete(e.Rel); err != nil {
			return m.Reload(), Effect{Err: fmt.Errorf("delete failed after %d of %d: %w", i, len(targets), err)}
		}
	}
	return m.clearSelection().Reload(), Effect{}
}

// ApplyInput carries out a Prompt with the text the user typed.
func (m Model) ApplyInput(op InputOp, text string) (Model, Effect) {
	m, eff := m.applyInput(op, text)
	return m.syncScroll(), eff
}

func (m Model) applyInput(op InputOp, text string) (Model, Effect) {
	switch op {
	case OpCreate:
		return m.create(text)
	case OpRename:
		return m.rename(text)
	case OpSearch:
		if strings.TrimSpace(text) == "" {
			return m, Effect{}
		}
		return m.setSearch(text), Effect{}
	case OpClearSearch:
		m.tabs[m.active].search = ""
		return m, Effect{}
	}
	return m, Effect{}
}

// create makes a note, or a folder when the name ends with "/", in the
// current directory and puts the cursor on it.
func (m Model) create(text string) (Model, Effect) {
	name, isDir, err := store.CleanNewName(text)
	if err != nil {
		return m, Effect{Err: fmt.Errorf("create failed: %w", err)}
	}
	rel := filepath.Join(m.currentDir(), name)

	var act undo.Action
	if isDir {
		act, err = m.store.MakeDir(rel)
	} else {
		var content []byte
		if content, err = store.BlankNote(name); err == nil {
			act, err = m.store.Create(rel, content)
		}
	}
	if err != nil {
		return m, Effect{Err: fmt.Errorf("create failed: %w", err)}
	}
	m.tabs[m.active].keep = name
	return m.Reload(), Effect{Action: &act}
}

// reloaded re-reads the active tab, keeping each tab's directory.
func (m Model) reloaded() Model { return m.Reload() }

// clearSelection drops the active tab's selection and visual mode.
func (m Model) clearSelection() Model {
	t := &m.tabs[m.active]
	t.selected, t.visual = nil, visualState{}
	return m
}
