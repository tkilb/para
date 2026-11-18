package store

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/tkilb/para/internal/undo"
)

// sidecarExt ends the name of a note's sidecar folder: plan.md keeps its
// images in plan.assets, beside it. Moving, copying, trashing and deleting
// the note do the same to the folder, so the two never part.
const sidecarExt = ".assets"

// SidecarOf returns the root-relative path of the sidecar folder of the note
// at rel, whether or not it exists, and "" when rel is not a note.
func SidecarOf(rel string) string {
	if !isNote(filepath.Base(rel)) || strings.HasSuffix(rel, "/") {
		return ""
	}
	return rel[:len(rel)-len(noteExt)] + sidecarExt
}

// sidecarFolder is the sidecar of the note at path, which must be a note.
func sidecarFolder(path string) string {
	return path[:len(path)-len(noteExt)] + sidecarExt
}

// existingSidecar returns the sidecar folder of the note at path when there
// is one. A file or a symlink named like a sidecar is not one.
func existingSidecar(path string) (string, bool) {
	if !isNote(filepath.Base(path)) {
		return "", false
	}
	side := sidecarFolder(path)
	info, err := os.Lstat(side)
	return side, err == nil && info.IsDir()
}

// sidecarPair returns the sidecars of from and to when from is a note with a
// sidecar and to is a note name too.
func sidecarPair(from, to string) (side, toSide string, ok bool) {
	if !isNote(filepath.Base(to)) {
		return "", "", false
	}
	if side, ok = existingSidecar(from); !ok {
		return "", "", false
	}
	return side, sidecarFolder(to), true
}

// together runs the operation on a note and then on its sidecar, as one undo
// entry. When the second fails the first is undone, so a failure leaves
// nothing half done.
func together(description string, note, side func() (undo.Action, error)) (undo.Action, error) {
	noteAct, err := note()
	if err != nil {
		return undo.Action{}, err
	}
	sideAct, err := side()
	if err != nil {
		if rbErr := noteAct.Undo(); rbErr != nil {
			err = errors.Join(err, rbErr)
		}
		return undo.Action{}, err
	}
	return undo.Group(description, noteAct, sideAct), nil
}

// sidecarsIn maps each note in des that has a sidecar folder beside it to the
// folder's name.
func sidecarsIn(des []fs.DirEntry) map[string]string {
	dirs := map[string]bool{}
	for _, de := range des {
		if de.IsDir() && strings.HasSuffix(de.Name(), sidecarExt) {
			dirs[de.Name()] = true
		}
	}
	if len(dirs) == 0 {
		return nil
	}
	out := map[string]string{}
	for _, de := range des {
		if de.IsDir() || !isNote(de.Name()) {
			continue
		}
		if side := noteTitle(de.Name()) + sidecarExt; dirs[side] {
			out[de.Name()] = side
		}
	}
	return out
}

// countFiles counts the visible files directly inside dir.
func countFiles(dir string) int {
	des, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, de := range des {
		if !strings.HasPrefix(de.Name(), ".") && de.Type().IsRegular() {
			n++
		}
	}
	return n
}

// ownedSidecar reports whether the folder name inside dir is the sidecar of a
// note that sits beside it.
func ownedSidecar(dir, name string) bool {
	if !strings.HasSuffix(name, sidecarExt) {
		return false
	}
	des, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, de := range des {
		if !de.IsDir() && isNote(de.Name()) && noteTitle(de.Name())+sidecarExt == name {
			return true
		}
	}
	return false
}
