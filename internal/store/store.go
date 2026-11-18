package store

import (
	"errors"
	"syscall"
	"time"

	"github.com/tkilb/para/internal/undo"
)

// Errors returned by Store implementations. Like the path-safety errors,
// they never echo paths because they are shown in the status bar.
var (
	ErrNotFound   = errors.New("not found")
	ErrUnreadable = errors.New("cannot read")
	ErrTooLarge   = errors.New("note is too large")
	// ErrNothingToAppend reports text with no content to append.
	ErrNothingToAppend = errors.New("nothing to append")
)

// errNotDir is what the OS reports for a path below a regular file.
var errNotDir = syscall.ENOTDIR

// maxNoteBytes bounds how much of a single note is read into memory.
const maxNoteBytes = 10 << 20

// MaxNoteBytes is the largest file para reads as a note.
const MaxNoteBytes = maxNoteBytes

// Entry is one row of a directory listing.
type Entry struct {
	Name string
	// Rel is the path relative to the notebook root.
	Rel     string
	IsDir   bool
	ModTime time.Time
	// ItemCount is the number of entries the directory would list under the
	// same options; it is zero for notes.
	ItemCount int
	// Attachments is the number of files in a note's sidecar folder, zero
	// when it has none and for anything but a note.
	Attachments int
	// IsImage is set for a PNG file, listed only when ListOptions.IncludeImages
	// is set. It is false for notes and directories.
	IsImage bool
}

// ListOptions controls which entries List returns.
type ListOptions struct {
	// ShowHidden includes names starting with ".". The .obsidian directory
	// is never listed.
	ShowHidden bool
	// IncludeImages also lists .png files. It is off by default so that code
	// which expects every listed file to be a note never sees an image.
	IncludeImages bool
}

// Store is the notes backend: a notebook of markdown files.
type Store interface {
	// List returns the directories and .md notes in dir (relative to the
	// notebook root), plus .png files when opts.IncludeImages is set:
	// directories first, then files, each sorted by name. A missing
	// directory lists as empty.
	List(dir string, opts ListOptions) ([]Entry, error)
	// ReadNote reads and parses the note at rel (relative to the root).
	ReadNote(rel string) (Note, error)

	// Inspect counts what is inside a directory, listed or not.
	Inspect(rel string) (DirInfo, error)
	// Delete permanently removes a file or directory tree. Not undoable.
	Delete(rel string) error
	// PurgeDailies permanently deletes archived daily notes that are
	// retentionDays or more days old, and returns how many. Not undoable.
	PurgeDailies(retentionDays int, today time.Time) (int, error)
	// Path returns the absolute path of rel (relative to the root), after the
	// same path-safety checks as every other operation.
	Path(rel string) (string, error)
	// Exists reports whether anything is at rel (relative to the root).
	Exists(rel string) (bool, error)

	// The write operations take names relative to the notebook root, never
	// overwrite, create missing folders on demand, and return one undo
	// action for the whole operation.

	// Create writes a new file containing data.
	Create(rel string, data []byte) (undo.Action, error)
	// MakeDir creates a new directory.
	MakeDir(rel string) (undo.Action, error)
	// Move renames a file or directory.
	Move(src, dst string) (undo.Action, error)
	// Edit changes the properties of the note at rel: edit modifies them,
	// then the note gets a uid when it has none and its title set to its
	// file name, and everything else is kept. A note whose properties cannot
	// be parsed yields ErrFrontmatterInvalid and is left untouched. Undo
	// restores the exact previous bytes, and refuses (ErrChanged) when the
	// note was changed meanwhile.
	Edit(rel string, edit func(*Frontmatter)) (undo.Action, error)
	// Append adds text to the end of the note at rel after one blank line,
	// leaving every existing byte as it is. Undo restores the exact previous
	// bytes, and refuses (ErrChanged) when the note was changed meanwhile.
	Append(rel, text string) (undo.Action, error)
	// Conform makes the note at rel standard on disk (see
	// Markdown.Conform), reporting whether it wrote anything.
	Conform(rel string) (bool, error)
	// Copy copies a file or a directory tree, giving every note in the copy
	// a fresh uid and a title equal to its file name.
	Copy(src, dst string) (undo.Action, CopyResult, error)
	// Trash moves a file or directory into .para/trash.
	Trash(rel string) (undo.Action, error)
}
