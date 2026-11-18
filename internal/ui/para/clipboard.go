package para

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/tkilb/para/internal/attach"
	"github.com/tkilb/para/internal/store"
	"github.com/tkilb/para/internal/undo"
)

type clipMode int

const (
	clipNone clipMode = iota
	clipYank
	clipCut
)

// clip is the yank/cut buffer. It belongs to the whole panel, so it
// survives directory and tab changes, and is replaced, never mutated.
type clip struct {
	mode clipMode
	// rels are notebook-relative paths, in list order.
	rels []string
	set  map[string]bool
}

func newClip(mode clipMode, entries []store.Entry) clip {
	c := clip{mode: mode, set: make(map[string]bool, len(entries))}
	for _, e := range entries {
		c.rels = append(c.rels, e.Rel)
		c.set[e.Rel] = true
	}
	return c
}

// targets returns the selected rows' entries, or the hovered one when
// nothing is selected.
func (m Model) targets() []store.Entry {
	t := m.tabs[m.active]
	var out []store.Entry
	for _, r := range t.rows {
		if t.selected[r.entry.Name] {
			out = append(out, r.entry)
		}
	}
	if len(out) == 0 && t.cursor < len(t.rows) {
		out = append(out, t.rows[t.cursor].entry)
	}
	return out
}

// setClip fills the buffer from the targets, replacing what was there, and
// clears the selection.
func (m Model) setClip(mode clipMode) (Model, Effect) {
	entries := m.targets()
	if len(entries) == 0 {
		return m, Effect{}
	}
	m.clip = newClip(mode, entries)
	t := &m.tabs[m.active]
	t.selected, t.visual = nil, visualState{}
	return m, Effect{}
}

// cancelClip empties the buffer.
func (m Model) cancelClip() (Model, Effect) {
	m.clip = clip{}
	return m, Effect{}
}

// currentDir is the notebook-relative path of the directory on screen.
func (m Model) currentDir() string {
	return filepath.Join(tabDefs[m.active].dir, m.tabs[m.active].dir)
}

// pasteItem is one planned paste.
type pasteItem struct {
	src, dst string
	collides bool
}

// pastePlan is the buffer resolved against the current directory.
type pastePlan struct {
	items   []pasteItem
	missing int // sources that vanished and are dropped
	skipped int // cut items already in this directory
}

func (p pastePlan) collisions() int {
	n := 0
	for _, it := range p.items {
		if it.collides {
			n++
		}
	}
	return n
}

func (m Model) planPaste() (pastePlan, error) {
	var plan pastePlan
	dir := m.currentDir()
	for _, src := range m.clip.rels {
		ok, err := m.store.Exists(src)
		if err != nil {
			return plan, err
		}
		if !ok {
			plan.missing++
			continue
		}
		dst := filepath.Join(dir, filepath.Base(src))
		if m.clip.mode == clipCut && dst == src {
			plan.skipped++
			continue
		}
		if m.clip.mode == clipYank && dst == src {
			// Copying an item into its own directory makes a duplicate.
			if dst, err = m.freeName(dst); err != nil {
				return plan, err
			}
			plan.items = append(plan.items, pasteItem{src: src, dst: dst})
			continue
		}
		collides, err := m.store.Exists(dst)
		if err != nil {
			return plan, err
		}
		plan.items = append(plan.items, pasteItem{src: src, dst: dst, collides: collides})
	}
	return plan, nil
}

// maxDuplicates bounds the search for a free "_N" name.
const maxDuplicates = 9999

// errNoFreeName reports that every "_N" name up to maxDuplicates is taken.
var errNoFreeName = errors.New("no free name for the copy")

// freeName returns rel with the first "_N" suffix (before a ".md" or ".png"
// extension) that nothing occupies yet: a.md becomes a_1.md, then a_2.md.
func (m Model) freeName(rel string) (string, error) {
	stem, ext := rel, ""
	for _, e := range []string{".md", ".png"} {
		if strings.HasSuffix(strings.ToLower(rel), e) {
			stem, ext = rel[:len(rel)-len(e)], rel[len(rel)-len(e):]
			break
		}
	}
	for n := 1; n <= maxDuplicates; n++ {
		candidate := fmt.Sprintf("%s_%d%s", stem, n, ext)
		taken, err := m.store.Exists(candidate)
		if err != nil {
			return "", err
		}
		if !taken {
			return candidate, nil
		}
	}
	return "", errNoFreeName
}

// paste puts the buffer into the current directory. Without overwrite a
// name collision cancels the whole paste. With overwrite and collisions it
// asks first, unless confirmed; replaced items go to the trash, and the
// whole paste is one undo entry.
func (m Model) paste(overwrite, confirmed bool) (Model, Effect) {
	if m.clip.mode == clipNone {
		return m, Effect{Info: "Nothing to paste."}
	}
	if eff, ok := m.askBundle(overwrite); ok {
		return m, eff
	}
	return m.pasteHere(overwrite, confirmed)
}

// bundleTarget is the note under the cursor when the buffer holds exactly one
// image, so the image could be bundled with the note.
func (m Model) bundleTarget() (store.Entry, bool) {
	if len(m.clip.rels) != 1 || !store.IsImage(filepath.Base(m.clip.rels[0])) {
		return store.Entry{}, false
	}
	return m.SelectedNote()
}

// askBundle asks whether to bundle the image in the buffer with the note under
// the cursor or paste it beside, when that is the situation.
func (m Model) askBundle(overwrite bool) (Effect, bool) {
	note, ok := m.bundleTarget()
	if !ok {
		return Effect{}, false
	}
	beside := OpPasteBeside
	if overwrite {
		beside = OpPasteBesideOverwrite
	}
	return Effect{Confirm: &Confirm{
		Prompt: fmt.Sprintf("Bundle %s with %s, or paste it beside the note?", filepath.Base(m.clip.rels[0]), rowName(note)),
		Op:     OpBundlePaste,
		Alt:    beside,
		Hint:   "(y/enter) bundle  (n) paste beside  (esc) cancel",
	}}, true
}

// bundlePaste puts the image in the buffer into the sidecar folder of the
// note under the cursor, as one undo entry.
func (m Model) bundlePaste() (Model, Effect) {
	note, ok := m.bundleTarget()
	if !ok {
		return m, Effect{Err: errors.New("bundle: select the note to bundle with")}
	}
	res, err := attach.Bundle(m.store, note.Rel, m.clip.rels[0], m.clip.mode == clipCut)
	if err != nil {
		return m, Effect{Err: fmt.Errorf("bundle failed: %w", err)}
	}
	m.clip = clip{}
	m = m.Reload()
	return m, Effect{Action: &res.Action}
}

// pasteHere pastes the buffer into the current directory.
func (m Model) pasteHere(overwrite, confirmed bool) (Model, Effect) {
	plan, err := m.planPaste()
	if err != nil {
		return m, Effect{Err: fmt.Errorf("paste: %w", err)}
	}

	if n := plan.collisions(); n > 0 {
		switch {
		case !overwrite:
			return m, Effect{Err: fmt.Errorf("paste cancelled: %s here (P overwrites)", alreadyExist(n))}
		case !confirmed:
			return m, Effect{Confirm: &Confirm{
				Prompt: fmt.Sprintf("Overwrite %s? Replaced items go to the trash.", count(n)),
				Op:     OpOverwritePaste,
			}}
		}
	}

	actions, kept, err := m.execute(plan)
	if err != nil {
		return m, Effect{Err: fmt.Errorf("paste failed: %w", err)}
	}
	m.clip = clip{}
	m = m.Reload()
	return m, pasteResult(plan, actions, kept)
}

// execute runs the plan, trashing replaced items first. On the first error
// it undoes what it already did, so a failed paste changes nothing.
// It also returns how many copied notes kept their source's uid.
func (m Model) execute(plan pastePlan) ([]undo.Action, int, error) {
	var done []undo.Action
	var kept int
	fail := func(err error) ([]undo.Action, int, error) {
		if len(done) > 0 {
			if rbErr := undo.Group("rollback", done...).Undo(); rbErr != nil {
				err = errors.Join(err, fmt.Errorf("rollback: %w", rbErr))
			}
		}
		return nil, 0, err
	}
	for _, it := range plan.items {
		if it.collides {
			a, err := m.store.Trash(it.dst)
			if err != nil {
				return fail(err)
			}
			done = append(done, a)
		}
		var a undo.Action
		var err error
		if m.clip.mode == clipCut {
			a, err = m.store.Move(it.src, it.dst)
		} else {
			var res store.CopyResult
			a, res, err = m.store.Copy(it.src, it.dst)
			kept += res.KeptUID
		}
		if err != nil {
			return fail(err)
		}
		done = append(done, a)
	}
	return done, kept, nil
}

func pasteResult(plan pastePlan, actions []undo.Action, kept int) Effect {
	var warn string
	if plan.missing > 0 {
		warn = fmt.Sprintf("%s no longer exist and were dropped from the paste.", count(plan.missing))
		if plan.missing == 1 {
			warn = "1 item no longer exists and was dropped from the paste."
		}
	}
	if kept > 0 {
		warn = strings.TrimSpace(warn + " " + keptUIDWarning(kept))
	}
	if len(plan.items) == 0 {
		if plan.skipped > 0 {
			return Effect{Info: "Already in this directory.", Warn: warn}
		}
		return Effect{Info: "Nothing was pasted.", Warn: warn}
	}
	group := undo.Group("paste "+describe(len(plan.items), filepath.Base(plan.items[0].dst)), actions...)
	return Effect{Action: &group, Warn: warn}
}

// ApplyConfirm carries out an accepted Confirm.
func (m Model) ApplyConfirm(op ConfirmOp) (Model, Effect) {
	switch op {
	case OpOverwritePaste:
		return m.paste(true, true)
	case OpTrash:
		return m.trash()
	case OpDelete:
		return m.deletePermanently()
	case OpBundlePaste:
		return m.bundlePaste()
	case OpPasteBeside:
		return m.pasteHere(false, false)
	case OpPasteBesideOverwrite:
		return m.pasteHere(true, false)
	}
	return m, Effect{}
}

// describe names one item, or counts several.
func describe(n int, first string) string {
	if n == 1 {
		return first
	}
	return count(n)
}

// alreadyExist words n collisions, e.g. "1 item already exists".
func alreadyExist(n int) string {
	if n == 1 {
		return "1 item already exists"
	}
	return fmt.Sprintf("%d items already exist", n)
}

// keptUIDWarning explains that some copies could not be given a new uid.
func keptUIDWarning(n int) string {
	if n == 1 {
		return "1 copied note could not be rewritten, so it kept its source's uid."
	}
	return fmt.Sprintf("%d copied notes could not be rewritten, so they kept their source's uid.", n)
}

func count(n int) string {
	if n == 1 {
		return "1 item"
	}
	return fmt.Sprintf("%d items", n)
}
