// Package intake creates the notes that arrive in the inbox: a line of
// quick capture, or a file imported as a transcript or as a note. Every
// intake note gets a uid, a title equal to its file name, a kind, a source,
// a date and a tags list. Nothing here reads or writes outside the notebook
// except one plain read of the file being imported.
package intake

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tkilb/para/internal/store"
	"github.com/tkilb/para/internal/undo"
)

const (
	// SourceAdhoc is the built-in default source of a note added by hand; the
	// config can change the list the Add Note form offers.
	SourceAdhoc = "adhoc"
	// SourceExtract is the source of a note extracted from another note.
	SourceExtract = "extract"
	// SourceDaily is the source of a daily note para creates.
	SourceDaily = "daily"

	inboxDir     = "inbox"
	archivesDir  = "archives"
	dailyDir     = "_daily"
	dayLayout    = "2006-01-02"
	noteExt      = ".md"
	maxNameTries = 1000
	// maxStemBytes keeps a file name, with its extension and a number added
	// for a taken name, inside the 255 bytes most file systems allow, even
	// for a title of four-byte characters.
	maxStemBytes = 200
)

// Errors never echo a path, because they are shown in the interface.
var (
	ErrEmpty      = errors.New("a title is needed")
	ErrNoFreeName = errors.New("could not find a free name")
	ErrNoText     = errors.New("no text selected")
	ErrNoKind     = errors.New("a kind is needed")
)

// Result describes a note that was created.
type Result struct {
	// Rel is the new note's path relative to the notebook root.
	Rel string
	// Action undoes (and redoes) the creation.
	Action undo.Action
	// Unlinked is set when an extracted note could not link back to its
	// origin, because the origin's name has characters a link cannot hold.
	Unlinked bool
	// Existed is set by Daily when the note was already there, so nothing was
	// created and there is nothing to undo.
	Existed bool
}

// Name returns the file name of an intake note titled title: the cleaned
// title with .md, or "" when nothing usable is left.
func Name(title string) string {
	clean := store.CleanTitle(title)
	for len(clean) > maxStemBytes {
		_, size := utf8.DecodeLastRuneInString(clean)
		clean = strings.TrimRight(clean[:len(clean)-size], " .")
	}
	if clean == "" {
		return ""
	}
	return clean + noteExt
}

// Capture creates an inbox note named after its title, holding body, with the
// given kind (KindUnset for none), source and tags. A missing title is
// ErrEmpty.
func Capture(s store.Store, now time.Time, kind store.Kind, source, title, body string, tags []string) (Result, error) {
	return CaptureIn(s, now, "", kind, source, title, body, tags)
}

// CaptureIn is Capture for the folder dir, relative to the notebook root,
// such as "projects/alpha". An empty dir means the inbox. A missing folder is
// created, and one outside the notebook is refused.
func CaptureIn(s store.Store, now time.Time, dir string, kind store.Kind, source, title, body string, tags []string) (Result, error) {
	if dir == "" {
		dir = inboxDir
	}
	name := Name(title)
	if name == "" {
		return Result{}, ErrEmpty
	}
	text := ""
	if body = strings.TrimSpace(body); body != "" {
		text = body + "\n"
	}
	return createUniqueIn(s, dir, name, func(final string) ([]byte, error) {
		return store.IntakeNote(final, kind, source, now.Format(dayLayout), text, tags)
	})
}

// createUnique creates an inbox note named name, or "name 2", "name 3" and
// so on when that is taken. contents makes the bytes for the name finally
// used.
func createUnique(s store.Store, name string, contents func(final string) ([]byte, error)) (Result, error) {
	return createUniqueIn(s, inboxDir, name, contents)
}

// createUniqueIn is createUnique for the folder dir, relative to the notebook
// root. The store refuses a folder that leaves the notebook.
func createUniqueIn(s store.Store, dir, name string, contents func(final string) ([]byte, error)) (Result, error) {
	stem := strings.TrimSuffix(name, filepath.Ext(name))
	ext := filepath.Ext(name)
	for n := 1; n <= maxNameTries; n++ {
		final := name
		if n > 1 {
			final = fmt.Sprintf("%s %d%s", stem, n, ext)
		}
		data, err := contents(final)
		if err != nil {
			return Result{}, err
		}
		rel := filepath.Join(dir, final)
		act, err := s.Create(rel, data)
		if errors.Is(err, store.ErrExists) {
			continue
		}
		if err != nil {
			return Result{}, err
		}
		return Result{Rel: rel, Action: act}, nil
	}
	return Result{}, ErrNoFreeName
}

// SplitText splits text, such as pasted from the clipboard, into a title and
// a body: the title is its first non-blank line, trimmed, and the body is
// what follows it with leading blank lines dropped. A single line is all
// title.
func SplitText(text string) (title, body string) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		rest := strings.TrimSpace(strings.Join(lines[i+1:], "\n"))
		return strings.TrimSpace(line), rest
	}
	return "", ""
}

// Extract creates an inbox note holding text taken from the note at origin,
// with the chosen kind. It links back to the origin with a related property,
// except when the origin is a dated daily note: that will be deleted, so the
// new note records source "daily YYYY-MM-DD" as plain text instead. The origin
// is never changed.
func Extract(s store.Store, now time.Time, kind store.Kind, origin, title, text string) (Result, error) {
	name := Name(title)
	text = strings.TrimSpace(text)
	switch {
	case kind == store.KindUnset:
		return Result{}, ErrNoKind
	case name == "":
		return Result{}, ErrEmpty
	case text == "":
		return Result{}, ErrNoText
	}

	source := SourceExtract
	var related []string
	var unlinked bool
	if date, daily := store.DailyDate(origin); daily {
		source = "daily " + date
	} else if link := originLink(origin); link != "" {
		related = []string{link}
	} else {
		unlinked = true
	}
	res, err := createUnique(s, name, func(final string) ([]byte, error) {
		return store.ExtractedNote(final, kind, source, now.Format(dayLayout), text+"\n", related)
	})
	res.Unlinked = unlinked
	return res, err
}

// originLink is the wikilink to the note at rel, or "" when its name has
// characters a link cannot hold.
func originLink(rel string) string {
	base := filepath.Base(rel)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	if stem == "" || strings.ContainsAny(stem, "|#[]^") {
		return ""
	}
	return "[[" + stem + "]]"
}

// Daily returns the daily note for the day of t: the dated note
// (YYYY-MM-DD.md) in inbox/_daily, or else the archived one in archives/_daily.
// When neither exists, an empty inbox/_daily note with a uid, a title, a date
// and no kind is created, and Result.Action undoes that.
func Daily(s store.Store, t time.Time) (Result, error) {
	return DailyFrom(s, t, nil)
}

// DailyFrom is Daily with a template: when it has to create the note, the
// note is made from template, the text of a template file, with the day
// filled in (see store.TemplatedNote). A nil template gives the empty note.
// A note that exists is never changed.
func DailyFrom(s store.Store, t time.Time, template []byte) (Result, error) {
	name := t.Format(dayLayout) + noteExt
	for _, dir := range []string{inboxDir, archivesDir} {
		rel := filepath.ToSlash(filepath.Join(dir, dailyDir, name))
		_, err := s.ReadNote(rel)
		if err == nil {
			return Result{Rel: rel, Existed: true}, nil
		}
		if !errors.Is(err, store.ErrNotFound) {
			return Result{}, err
		}
	}
	rel := filepath.ToSlash(filepath.Join(inboxDir, dailyDir, name))
	var data []byte
	var err error
	if template != nil {
		data, err = store.TemplatedNote(template, name, SourceDaily, t)
	} else {
		data, err = store.IntakeNote(name, store.KindUnset, SourceDaily, t.Format(dayLayout), "", nil)
	}
	if err != nil {
		return Result{}, err
	}
	act, err := s.Create(rel, data)
	if err != nil {
		return Result{}, err
	}
	return Result{Rel: rel, Action: act}, nil
}

// Clip creates a note for a web page in the folder dir (relative to the
// notebook root; empty means the inbox): named after title (or, when the
// title has nothing usable, after the address), with kind link and the
// address as its source, and an empty body. u must come from clip.ParseURL.
func Clip(s store.Store, now time.Time, dir string, u *url.URL, title string) (Result, error) {
	if dir == "" {
		dir = inboxDir
	}
	name := Name(title)
	if name == "" {
		name = Name(addressTitle(u))
	}
	if name == "" {
		return Result{}, ErrEmpty
	}
	address := u.String()
	return createUniqueIn(s, dir, name, func(final string) ([]byte, error) {
		return store.IntakeNote(final, store.KindLink, address, now.Format(dayLayout), "", nil)
	})
}

// addressTitle names a page by its address: the host, then the path
// segments, such as "example.com - docs - intro".
func addressTitle(u *url.URL) string {
	parts := []string{u.Hostname()}
	for _, seg := range strings.Split(u.Path, "/") {
		if seg != "" {
			parts = append(parts, seg)
		}
	}
	return strings.Join(parts, " - ")
}
