package store

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// obsidianDir is Obsidian's settings directory; it is never listed.
const obsidianDir = ".obsidian"

// Markdown is a Store backed by a directory of markdown files.
type Markdown struct {
	root string
}

var _ Store = (*Markdown)(nil)

// NewMarkdown returns a store rooted at the notebook directory root.
func NewMarkdown(root string) *Markdown {
	return &Markdown{root: root}
}

// Path implements Store.
func (m *Markdown) Path(rel string) (string, error) {
	return ResolveInRoot(m.root, rel)
}

// List implements Store.
func (m *Markdown) List(dir string, opts ListOptions) ([]Entry, error) {
	return m.list(dir, opts, true)
}

func (m *Markdown) list(dir string, opts ListOptions, counts bool) ([]Entry, error) {
	abs, err := ResolveInRoot(m.root, dir)
	if err != nil {
		return nil, err
	}
	des, err := os.ReadDir(abs)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, ErrUnreadable
	}

	rel := filepath.Clean(dir)
	sidecars := sidecarsIn(des)
	owned := map[string]bool{}
	for _, side := range sidecars {
		owned[side] = true
	}
	var entries []Entry
	for _, de := range des {
		if owned[de.Name()] {
			continue // a note's sidecar is shown as a count on the note
		}
		e, ok := m.entry(filepath.Join(rel, de.Name()), de, opts)
		if !ok {
			continue
		}
		if side, has := sidecars[de.Name()]; has && counts {
			e.Attachments = countFiles(filepath.Join(abs, side))
		}
		if e.IsDir && counts {
			children, _ := m.list(e.Rel, opts, false)
			e.ItemCount = len(children)
		}
		entries = append(entries, e)
	}
	sortEntries(entries)
	return entries, nil
}

// entry turns a directory entry into a listing row, reporting false for
// anything that must not be listed: hidden names, .obsidian, files that are
// neither notes nor wanted images, special files, and symlinks that leave the notebook or cannot be read.
func (m *Markdown) entry(rel string, de fs.DirEntry, opts ListOptions) (Entry, bool) {
	name := de.Name()
	if name == obsidianDir || (!opts.ShowHidden && strings.HasPrefix(name, ".")) {
		return Entry{}, false
	}

	info, err := de.Info()
	if err != nil {
		return Entry{}, false
	}
	if de.Type()&fs.ModeSymlink != 0 {
		resolved, err := ResolveInRoot(m.root, rel)
		if err != nil {
			return Entry{}, false
		}
		if info, err = os.Stat(resolved); err != nil {
			return Entry{}, false
		}
	}

	isDir := info.IsDir()
	image := !isDir && opts.IncludeImages && isImage(name)
	if !isDir && (!info.Mode().IsRegular() || (!isNote(name) && !image)) {
		return Entry{}, false
	}
	return Entry{Name: name, Rel: rel, IsDir: isDir, ModTime: info.ModTime(), IsImage: image}, true
}

// sortEntries orders directories before notes, each case-insensitively by
// name, with the exact name as the tie-break.
func sortEntries(entries []Entry) {
	sort.SliceStable(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if a.IsDir != b.IsDir {
			return a.IsDir
		}
		if la, lb := strings.ToLower(a.Name), strings.ToLower(b.Name); la != lb {
			return la < lb
		}
		return a.Name < b.Name
	})
}

// ReadNote implements Store.
func (m *Markdown) ReadNote(rel string) (Note, error) {
	abs, err := ResolveInRoot(m.root, rel)
	if err != nil {
		return Note{}, err
	}
	f, err := os.Open(abs)
	if errors.Is(err, fs.ErrNotExist) {
		return Note{}, ErrNotFound
	}
	if err != nil {
		return Note{}, ErrUnreadable
	}
	defer f.Close()

	if info, err := f.Stat(); err != nil || !info.Mode().IsRegular() {
		return Note{}, ErrUnreadable
	}
	data, err := io.ReadAll(io.LimitReader(f, maxNoteBytes+1))
	if err != nil {
		return Note{}, ErrUnreadable
	}
	if len(data) > maxNoteBytes {
		return Note{}, ErrTooLarge
	}
	return ParseNote(filepath.Base(rel), data), nil
}

// Exists implements Store. A symlink counts as existing even when dangling.
func (m *Markdown) Exists(rel string) (bool, error) {
	path, err := ResolveInRoot(m.root, rel)
	if err != nil {
		return false, err
	}
	switch _, err := os.Lstat(path); {
	case err == nil:
		return true, nil
	case errors.Is(err, fs.ErrNotExist), errors.Is(err, errNotDir):
		return false, nil
	}
	return false, ErrUnreadable
}
