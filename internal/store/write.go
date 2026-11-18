package store

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/tkilb/para/internal/undo"
)

// trashRel is the notebook-relative folder that holds trashed items. Each
// trashed item gets its own slot folder so names never collide.
const trashRel = ".para/trash"

const slotIDBytes = 8

// Create writes a new file at rel (relative to the root) containing data,
// creating missing folders. It never overwrites. Undo moves the file to the
// trash.
func (m *Markdown) Create(rel string, data []byte) (undo.Action, error) {
	path, err := ResolveInRoot(m.root, rel)
	if err != nil {
		return undo.Action{}, err
	}
	if err := mustNotExist(path); err != nil {
		return undo.Action{}, err
	}
	slot, err := m.newSlot(filepath.Base(path))
	if err != nil {
		return undo.Action{}, err
	}
	made, err := mkdirAllTracked(filepath.Dir(path))
	if err != nil {
		return undo.Action{}, err
	}
	if err := writeNewFile(path, data); err != nil {
		removeEmpty(made)
		return undo.Action{}, err
	}
	act, err := placed(path, slot, made, "create "+filepath.Base(path))
	if err != nil {
		_ = os.Remove(path)
		removeEmpty(made)
		return undo.Action{}, err
	}
	return act, nil
}

// MakeDir creates a new directory at rel, with missing parents. Undo
// removes it again, and refuses when it is no longer empty.
func (m *Markdown) MakeDir(rel string) (undo.Action, error) {
	path, err := ResolveInRoot(m.root, rel)
	if err != nil {
		return undo.Action{}, err
	}
	if err := mustNotExist(path); err != nil {
		return undo.Action{}, err
	}
	made, err := mkdirAllTracked(path)
	if err != nil {
		return undo.Action{}, err
	}
	return undo.Action{
		Description: "create folder " + filepath.Base(path),
		Undo: func() error {
			info, err := os.Lstat(path)
			switch {
			case errors.Is(err, fs.ErrNotExist):
				// Already gone: the folder is not there to remove.
			case err != nil || !info.IsDir():
				return ErrChanged
			default:
				// os.Remove only removes an empty folder.
				if err := os.Remove(path); err != nil {
					return ErrChanged
				}
			}
			removeEmpty(made[:max(len(made)-1, 0)])
			return nil
		},
		Redo: func() error {
			if err := mustNotExist(path); err != nil {
				return err
			}
			again, err := mkdirAllTracked(path)
			if err != nil {
				return err
			}
			made = again
			return nil
		},
	}, nil
}

// Move renames the item at src to dst, never overwriting. Undo moves it
// back. A note with a sidecar folder takes the folder along, renamed to match,
// and the move refuses when that name is taken.
func (m *Markdown) Move(src, dst string) (undo.Action, error) {
	from, to, err := m.resolvePair(src, dst)
	if err != nil {
		return undo.Action{}, err
	}
	side, toSide, ok := sidecarPair(from, to)
	if !ok {
		return m.moveAt(from, to)
	}
	if !isCaseOnlyRename(side, toSide) {
		if err := mustNotExist(toSide); err != nil {
			return undo.Action{}, err
		}
	}
	return together("move "+filepath.Base(from),
		func() (undo.Action, error) { return m.moveAt(from, to) },
		func() (undo.Action, error) { return m.moveAt(side, toSide) })
}

// moveAt moves the item at the absolute path from to to.
func (m *Markdown) moveAt(from, to string) (undo.Action, error) {
	want, err := m.existing(from)
	if err != nil {
		return undo.Action{}, err
	}
	r := &relocation{a: from, b: to, want: want}
	if err := r.forward(); err != nil {
		return undo.Action{}, err
	}
	return r.action("move " + filepath.Base(from)), nil
}

// Trash moves the item at rel into .para/trash, with the sidecar folder of a
// note. Undo restores it, and refuses when its original path is taken or the
// item changed.
func (m *Markdown) Trash(rel string) (undo.Action, error) {
	from, err := ResolveInRoot(m.root, rel)
	if err != nil {
		return undo.Action{}, err
	}
	side, ok := existingSidecar(from)
	if !ok {
		return m.trashAt(from)
	}
	return together("trash "+filepath.Base(from),
		func() (undo.Action, error) { return m.trashAt(from) },
		func() (undo.Action, error) { return m.trashAt(side) })
}

// trashAt moves the item at the absolute path from into the trash.
func (m *Markdown) trashAt(from string) (undo.Action, error) {
	want, err := m.existing(from)
	if err != nil {
		return undo.Action{}, err
	}
	slot, err := m.newSlot(filepath.Base(from))
	if err != nil {
		return undo.Action{}, err
	}
	r := &relocation{a: from, b: slot, want: want}
	if err := r.forward(); err != nil {
		return undo.Action{}, err
	}
	return r.action("trash " + filepath.Base(from)), nil
}

// Copy copies the file or folder at src to dst, never overwriting. Folders
// copy recursively with everything inside, attachments included; symlinks
// and other special files inside are skipped. Every note in the copy gets a
// fresh uid and a title equal to its file name; a note that cannot be
// rewritten is kept byte for byte and counted in the result. Undo moves the
// copy to the trash. A note with a sidecar folder is copied with it, and the
// copy refuses when the new sidecar name is taken.
func (m *Markdown) Copy(src, dst string) (undo.Action, CopyResult, error) {
	var res CopyResult
	from, to, err := m.resolvePair(src, dst)
	if err != nil {
		return undo.Action{}, res, err
	}
	side, toSide, ok := sidecarPair(from, to)
	if !ok {
		return m.copyAt(from, to)
	}
	if err := mustNotExist(toSide); err != nil {
		return undo.Action{}, res, err
	}
	act, err := together("copy "+filepath.Base(from),
		func() (undo.Action, error) {
			a, r, err := m.copyAt(from, to)
			res = r
			return a, err
		},
		func() (undo.Action, error) {
			a, _, err := m.copyAt(side, toSide)
			return a, err
		})
	return act, res, err
}

// copyAt copies the item at the absolute path from to to.
func (m *Markdown) copyAt(from, to string) (undo.Action, CopyResult, error) {
	var res CopyResult
	want, err := m.existing(from)
	if err != nil {
		return undo.Action{}, res, err
	}
	if err := mustNotExist(to); err != nil {
		return undo.Action{}, res, err
	}
	if want.isDir && isInside(from, to) {
		return undo.Action{}, res, ErrIntoItself
	}
	slot, err := m.newSlot(filepath.Base(to))
	if err != nil {
		return undo.Action{}, res, err
	}
	made, err := mkdirAllTracked(filepath.Dir(to))
	if err != nil {
		return undo.Action{}, res, err
	}
	var c copier
	if err := c.copy(from, to); err != nil {
		c.cleanup()
		removeEmpty(made)
		return undo.Action{}, res, err
	}
	if res.KeptUID, err = c.renewNotes(); err != nil {
		c.cleanup()
		removeEmpty(made)
		return undo.Action{}, res, err
	}
	act, err := placed(to, slot, made, "copy "+filepath.Base(from))
	if err != nil {
		c.cleanup()
		removeEmpty(made)
		return undo.Action{}, res, err
	}
	return act, res, nil
}

// CopyResult reports what Copy could not do for the caller to surface.
type CopyResult struct {
	// KeptUID counts notes copied byte for byte, so they still carry their
	// source's uid: their properties could not be parsed or the note was too
	// large to read.
	KeptUID int
}

// renewNotes gives every note the copier wrote a fresh uid and a title
// equal to its file name. It returns how many notes it had to leave as they
// are.
func (c *copier) renewNotes() (kept int, err error) {
	for _, path := range c.notes {
		ok, err := renewNote(path)
		if err != nil {
			return kept, err
		}
		if !ok {
			kept++
		}
	}
	return kept, nil
}

// renewNote rewrites the note at path with a fresh uid and its file name as
// title. It reports false when the note is left exactly as it is.
func renewNote(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, ErrUnreadable
	}
	if info.Size() > maxNoteBytes {
		return false, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false, ErrUnreadable
	}
	out, err := RenewIdentity(data, noteTitle(filepath.Base(path)))
	switch {
	case errors.Is(err, ErrFrontmatterInvalid):
		return false, nil
	case err != nil:
		return false, ErrUnreadable
	}
	if err := rewriteFile(path, out, info.Mode().Perm()); err != nil {
		return false, ErrUnreadable
	}
	return true, nil
}

// rewriteFile replaces the contents of path, making a read-only file
// writable for the write and restoring its mode afterward.
func rewriteFile(path string, data []byte, perm fs.FileMode) error {
	if perm&ownerWrite == 0 {
		if err := os.Chmod(path, perm|ownerWrite); err != nil {
			return err
		}
		defer func() { _ = os.Chmod(path, perm) }()
	}
	return os.WriteFile(path, data, perm)
}

// placed builds the undo action for an item just written at path: undo
// moves it to slot, redo moves it back.
func placed(path, slot string, made []string, description string) (undo.Action, error) {
	want, err := takeSnapshot(path)
	if err != nil {
		return undo.Action{}, ErrUnreadable
	}
	r := &relocation{a: slot, b: path, want: want, madeForward: made}
	return r.action(description), nil
}

// resolvePair resolves two root-relative names.
func (m *Markdown) resolvePair(src, dst string) (from, to string, err error) {
	if from, err = ResolveInRoot(m.root, src); err != nil {
		return "", "", err
	}
	if to, err = ResolveInRoot(m.root, dst); err != nil {
		return "", "", err
	}
	return from, to, nil
}

// existing snapshots path, reporting ErrNotFound when it is absent.
func (m *Markdown) existing(path string) (snapshot, error) {
	s, err := takeSnapshot(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return snapshot{}, ErrNotFound
	case err != nil:
		return snapshot{}, ErrUnreadable
	}
	return s, nil
}

// newSlot returns a fresh, not yet existing path for name inside the trash.
func (m *Markdown) newSlot(name string) (string, error) {
	var id [slotIDBytes]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", ErrUnreadable
	}
	return ResolveInRoot(m.root, filepath.Join(trashRel, hex.EncodeToString(id[:]), name))
}

// writeNewFile creates path exclusively and writes data to it.
func writeNewFile(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, filePerm)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return ErrExists
		}
		return ErrUnreadable
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		_ = os.Remove(path)
		return ErrUnreadable
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return ErrUnreadable
	}
	return nil
}

// copier copies a file or directory tree and remembers everything it
// created, so a failure removes only its own output and never touches
// anything another process put there.
type copier struct {
	created []string
	notes   []string // the .md files among them
}

// copy copies a regular file or a directory tree from to to. The top item
// must be a file or directory; nested symlinks and special files are
// skipped.
func (c *copier) copy(from, to string) error {
	info, err := os.Lstat(from)
	if err != nil {
		return ErrUnreadable
	}
	switch {
	case info.IsDir():
		return c.copyDir(from, to, info.Mode().Perm())
	case info.Mode().IsRegular():
		return c.copyFile(from, to, info.Mode().Perm())
	}
	return ErrUnsupported
}

func (c *copier) copyDir(from, to string, perm fs.FileMode) error {
	// Created with normal permissions so the children can be written even
	// when the source folder is read-only; the source mode is applied last.
	if err := os.Mkdir(to, dirPerm); err != nil {
		return ErrUnreadable
	}
	c.created = append(c.created, to)
	entries, err := os.ReadDir(from)
	if err != nil {
		return ErrUnreadable
	}
	for _, e := range entries {
		if t := e.Type(); !t.IsDir() && !t.IsRegular() {
			continue
		}
		if err := c.copy(filepath.Join(from, e.Name()), filepath.Join(to, e.Name())); err != nil {
			return err
		}
	}
	if err := os.Chmod(to, perm); err != nil {
		return ErrUnreadable
	}
	return nil
}

func (c *copier) copyFile(from, to string, perm fs.FileMode) error {
	in, err := os.Open(from)
	if err != nil {
		return ErrUnreadable
	}
	defer in.Close()
	out, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return ErrUnreadable
	}
	c.created = append(c.created, to)
	if isNote(to) {
		c.notes = append(c.notes, to)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return ErrUnreadable
	}
	if err := out.Close(); err != nil {
		return ErrUnreadable
	}
	return nil
}

// cleanup removes what copy created, newest first. A folder only goes when
// it is empty, so a file someone else added meanwhile is left alone.
func (c *copier) cleanup() {
	for i := len(c.created) - 1; i >= 0; i-- {
		if info, err := os.Lstat(c.created[i]); err == nil && info.IsDir() {
			_ = os.Chmod(c.created[i], dirPerm)
		}
		_ = os.Remove(c.created[i])
	}
}
