package store

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// DirInfo summarizes everything inside a directory, listed or not.
type DirInfo struct {
	// Items counts every file and folder inside, recursively.
	Items int
	// Unlisted counts the items a listing would not show: hidden names,
	// anything inside a hidden folder, a note's sidecar folder and what is in it, and files that are neither .md notes nor .png images.
	Unlisted int
}

// Inspect counts what is inside the directory at rel. It does not follow
// symlinks. A file, rather than a directory, reports zero items.
func (m *Markdown) Inspect(rel string) (DirInfo, error) {
	path, err := ResolveInRoot(m.root, rel)
	if err != nil {
		return DirInfo{}, err
	}
	info, err := os.Lstat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return DirInfo{}, ErrNotFound
	case err != nil:
		return DirInfo{}, ErrUnreadable
	case !info.IsDir():
		return DirInfo{}, nil
	}

	var di DirInfo
	err = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == path {
			return nil
		}
		di.Items++
		if unlisted(path, p, d) {
			di.Unlisted++
		}
		return nil
	})
	if err != nil {
		return DirInfo{}, ErrUnreadable
	}
	return di, nil
}

// unlisted reports whether a listing would hide the entry p inside base.
func unlisted(base, p string, d fs.DirEntry) bool {
	rel, err := filepath.Rel(base, p)
	if err != nil {
		return true
	}
	dir := base
	for _, seg := range strings.Split(rel, string(filepath.Separator)) {
		if strings.HasPrefix(seg, ".") || ownedSidecar(dir, seg) {
			return true
		}
		dir = filepath.Join(dir, seg)
	}
	return !d.IsDir() && !isNote(d.Name()) && !isImage(d.Name())
}

// Delete permanently removes the file or directory tree at rel, and the
// sidecar folder of a note. It cannot be undone. A symlink is removed without touching its target. The notebook
// root itself is never deletable, because ResolveInRoot rejects an empty
// name.
func (m *Markdown) Delete(rel string) error {
	path, err := ResolveInRoot(m.root, rel)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(path); errors.Is(err, fs.ErrNotExist) {
		return ErrNotFound
	}
	side, hasSide := existingSidecar(path)
	if err := os.RemoveAll(path); err != nil {
		return ErrUnreadable
	}
	if hasSide {
		if err := os.RemoveAll(side); err != nil {
			return ErrUnreadable
		}
	}
	return nil
}
