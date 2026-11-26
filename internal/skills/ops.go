package skills

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tkilb/para/internal/store"
	"github.com/tkilb/para/internal/undo"
)

const maxDuplicates = 9999

var (
	// ErrNoFreeName reports that every "_N" duplicate name is taken.
	ErrNoFreeName = errors.New("no free name for the copy")
	// ErrUnchangedName reports a rename to the same name.
	ErrUnchangedName = errors.New("the name is unchanged")
)

// CreateInStore creates a new skill file in .skills/ with a starter template
// and returns the filename and undo.Action.
func CreateInStore(st store.Store, inputName string, mode Mode) (string, undo.Action, error) {
	filename, stem, err := CleanName(inputName)
	if err != nil {
		return "", undo.Action{}, err
	}
	rel := filepath.Join(DirName, filename)
	tmpl := StarterTemplate(stem, mode)
	act, err := st.Create(rel, []byte(tmpl))
	if err != nil {
		return "", undo.Action{}, err
	}
	return filename, act, nil
}

// RenameInStore renames oldFilename to newInput inside .skills/, updating the
// frontmatter name property to match the new stem, and returns one undo.Action.
func RenameInStore(st store.Store, oldFilename, newInput string) (string, undo.Action, error) {
	oldClean, oldStem, err := CleanName(oldFilename)
	if err != nil {
		return "", undo.Action{}, err
	}
	newClean, newStem, err := CleanName(newInput)
	if err != nil {
		return "", undo.Action{}, err
	}
	if oldClean == newClean {
		return "", undo.Action{}, ErrUnchangedName
	}

	oldRel := filepath.Join(DirName, oldClean)
	newRel := filepath.Join(DirName, newClean)

	var acts []undo.Action
	moveAct, err := st.Move(oldRel, newRel)
	if err != nil {
		return "", undo.Action{}, err
	}
	acts = append(acts, moveAct)

	// Update frontmatter name if valid frontmatter exists.
	if editAct, err := updateSkillName(st, newRel, oldStem, newStem, true); err == nil && editAct.Undo != nil {
		acts = append(acts, editAct)
	}

	return newClean, undo.Group("rename "+oldClean+" to "+newClean, acts...), nil
}

// updateSkillName rewrites the frontmatter name property of the skill at rel.
func updateSkillName(st store.Store, rel, oldStem, newStem string, always bool) (undo.Action, error) {
	abs, err := st.Path(rel)
	if err != nil {
		return undo.Action{}, err
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return undo.Action{}, err
	}
	preMod := info.ModTime()
	before, err := os.ReadFile(abs)
	if err != nil {
		return undo.Action{}, err
	}
	n := store.ParseNote(filepath.Base(rel), before)
	if !n.HasFrontmatter || n.FrontmatterErr != nil || n.Frontmatter == nil {
		return undo.Action{}, nil
	}
	cur := getScalar(n.Frontmatter, "name")
	if !always && cur != "" && cur != oldStem {
		return undo.Action{}, nil
	}
	n.Frontmatter.SetString("name", newStem)
	raw, err := n.Frontmatter.Marshal()
	if err != nil {
		return undo.Action{}, err
	}
	body := strings.TrimLeft(n.Body, "\r\n")
	var after []byte
	after = append(after, "---\n"...)
	after = append(after, raw...)
	after = append(after, "---\n\n"...)
	after = append(after, body...)

	if err := os.WriteFile(abs, after, info.Mode().Perm()); err != nil {
		return undo.Action{}, err
	}
	var postMod time.Time
	if postInfo, err := os.Lstat(abs); err == nil {
		postMod = postInfo.ModTime()
	}
	return undo.Action{
		Description: "update skill name " + filepath.Base(rel),
		Undo: func() error {
			if err := os.WriteFile(abs, before, info.Mode().Perm()); err != nil {
				return err
			}
			_ = os.Chtimes(abs, preMod, preMod)
			return nil
		},
		Redo: func() error {
			if err := os.WriteFile(abs, after, info.Mode().Perm()); err != nil {
				return err
			}
			if !postMod.IsZero() {
				_ = os.Chtimes(abs, postMod, postMod)
			}
			return nil
		},
	}, nil
}

// TrashInStore moves the given skill filenames into .para/trash as a single
// undoable action. On error, any already-trashed items are rolled back.
func TrashInStore(st store.Store, filenames []string) (undo.Action, error) {
	var done []undo.Action
	for _, fn := range filenames {
		clean, _, err := CleanName(fn)
		if err != nil {
			if len(done) > 0 {
				_ = undo.Group("rollback", done...).Undo()
			}
			return undo.Action{}, err
		}
		rel := filepath.Join(DirName, clean)
		act, err := st.Trash(rel)
		if err != nil {
			if len(done) > 0 {
				_ = undo.Group("rollback", done...).Undo()
			}
			return undo.Action{}, err
		}
		done = append(done, act)
	}
	if len(done) == 0 {
		return undo.Action{}, nil
	}
	desc := "trash " + filenames[0]
	if len(filenames) > 1 {
		desc = fmt.Sprintf("trash %d skills", len(filenames))
	}
	return undo.Group(desc, done...), nil
}

// DeleteInStore permanently deletes the given skill filenames from .skills/.
func DeleteInStore(st store.Store, filenames []string) error {
	for i, fn := range filenames {
		clean, _, err := CleanName(fn)
		if err != nil {
			return err
		}
		rel := filepath.Join(DirName, clean)
		if err := st.Delete(rel); err != nil {
			return fmt.Errorf("delete failed after %d of %d: %w", i, len(filenames), err)
		}
	}
	return nil
}

// PasteItem describes one planned skill paste.
type PasteItem struct {
	SrcFilename string
	DstFilename string
	Collides    bool
	Duplicate   bool
}

// PastePlan is a resolved paste operation against a destination store.
type PastePlan struct {
	Items      []PasteItem
	Missing    int
	Skipped    int
	Collisions int
}

// PlanPaste resolves pasting filenames from srcStore into dstStore.
func PlanPaste(srcStore, dstStore store.Store, sameNotebook bool, filenames []string, cut bool) (PastePlan, error) {
	var plan PastePlan
	if srcStore == nil || dstStore == nil {
		return plan, errors.New("no store available")
	}
	for _, fn := range filenames {
		clean, stem, err := CleanName(fn)
		if err != nil {
			return plan, err
		}
		srcRel := filepath.Join(DirName, clean)
		ok, err := srcStore.Exists(srcRel)
		if err != nil {
			return plan, err
		}
		if !ok {
			plan.Missing++
			continue
		}
		if sameNotebook && cut {
			plan.Skipped++
			continue
		}
		if sameNotebook && !cut {
			dupFile, err := freeDuplicateFilename(dstStore, stem)
			if err != nil {
				return plan, err
			}
			plan.Items = append(plan.Items, PasteItem{
				SrcFilename: clean,
				DstFilename: dupFile,
				Duplicate:   true,
			})
			continue
		}
		dstRel := filepath.Join(DirName, clean)
		collides, err := dstStore.Exists(dstRel)
		if err != nil {
			return plan, err
		}
		if collides {
			plan.Collisions++
		}
		plan.Items = append(plan.Items, PasteItem{
			SrcFilename: clean,
			DstFilename: clean,
			Collides:    collides,
		})
	}
	return plan, nil
}

// ExecutePaste runs a PastePlan, returning the grouped undo.Action and the
// first destination filename created.
func ExecutePaste(srcStore, dstStore store.Store, plan PastePlan, cut bool) (undo.Action, string, error) {
	var done []undo.Action
	fail := func(err error) (undo.Action, string, error) {
		if len(done) > 0 {
			if rbErr := undo.Group("rollback", done...).Undo(); rbErr != nil {
				err = errors.Join(err, fmt.Errorf("rollback: %w", rbErr))
			}
		}
		return undo.Action{}, "", err
	}

	var firstDst string
	for _, it := range plan.Items {
		srcRel := filepath.Join(DirName, it.SrcFilename)
		dstRel := filepath.Join(DirName, it.DstFilename)

		srcPath, err := srcStore.Path(srcRel)
		if err != nil {
			return fail(err)
		}
		data, err := os.ReadFile(srcPath)
		if err != nil {
			return fail(err)
		}

		// If duplicating (e.g. foo.md -> foo_1.md), update the skill's name to match the new stem.
		if it.Duplicate {
			newStem := stemOf(it.DstFilename)
			if parsed, pErr := Parse(it.SrcFilename, data); pErr == nil || errors.Is(pErr, ErrEmptyBody) {
				parsed.Name = newStem
				parsed.Filename = it.DstFilename
				data = Format(parsed)
			}
		}

		if it.Collides {
			trashAct, err := dstStore.Trash(dstRel)
			if err != nil {
				return fail(err)
			}
			done = append(done, trashAct)
		}

		createAct, err := dstStore.Create(dstRel, data)
		if err != nil {
			return fail(err)
		}
		done = append(done, createAct)

		if cut {
			cutAct, err := srcStore.Trash(srcRel)
			if err != nil {
				return fail(err)
			}
			done = append(done, cutAct)
		}

		if firstDst == "" {
			firstDst = it.DstFilename
		}
	}

	if len(done) == 0 {
		return undo.Action{}, "", nil
	}
	desc := "paste " + firstDst
	if len(plan.Items) > 1 {
		desc = fmt.Sprintf("paste %d skills", len(plan.Items))
	}
	return undo.Group(desc, done...), firstDst, nil
}

func freeDuplicateFilename(st store.Store, stem string) (string, error) {
	for n := 1; n <= maxDuplicates; n++ {
		candidate := fmt.Sprintf("%s_%d%s", stem, n, ext)
		taken, err := st.Exists(filepath.Join(DirName, candidate))
		if err != nil {
			return "", err
		}
		if !taken {
			return candidate, nil
		}
	}
	return "", ErrNoFreeName
}
