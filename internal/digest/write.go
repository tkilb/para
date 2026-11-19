package digest

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/tkilb/para/internal/intake"
	"github.com/tkilb/para/internal/loader"
	"github.com/tkilb/para/internal/store"
	"github.com/tkilb/para/internal/textsafe"
	"github.com/tkilb/para/internal/undo"
)

const (
	// FolderPrefix starts the name of every import folder. It is upper case so
	// the folder stands out in the inbox as a reminder to follow up. Folders
	// made with the old lower case prefix are still recognised.
	FolderPrefix = "IMPORT_"
	// SourceFile holds the imported text. It starts with an underscore so it
	// sorts first.
	SourceFile = "_source.md"
	// ActionsFile holds the action items, and exists only when there are some.
	ActionsFile = "_actions.md"
	// FactsFile holds the facts, and exists only when there are some.
	FactsFile = "_facts.md"
	// TakeawaysFile holds the key points, conclusions and decisions, and
	// exists only when there are some.
	TakeawaysFile = "_takeaways.md"

	importKindName = "import"
	inboxDir       = "inbox"
	dayLayout      = "2006-01-02"
	maxNameTries   = 1000
	// placeholderSource is the source of _source.md until the agent judges it.
	placeholderSource = fallbackSource
)

// ErrFinished reports a second Finish on the same Folder.
var ErrFinished = errors.New("this import is already finished")

// ImportKind is the import kind when the configured kinds include it, and
// KindUnset when they do not, so a note is then written with no kind.
func ImportKind() store.Kind {
	for _, k := range store.Kinds() {
		if k == importKindName {
			return k
		}
	}
	return store.KindUnset
}

// Folder is an import folder in the inbox, begun with its _source.md.
type Folder struct {
	// Rel is the folder's path relative to the notebook root. Finish may
	// change it when it renames the folder to the agent's topic.
	Rel string

	s    store.Store
	now  time.Time
	kind store.Kind
	src  loader.Source
	acts []undo.Action
	done bool
	// keepName stops Finish renaming the folder, for a re-run.
	keepName bool
}

// Result describes a finished import.
type Result struct {
	// Rel is the folder, relative to the notebook root.
	Rel string
	// Files are the files written besides _source.md, in the order written.
	Files []string
	// Action undoes (and redoes) the whole import as one entry.
	Action undo.Action
	// KindMissing is set when the import kind is not configured, so the
	// files have no kind.
	KindMissing bool
	// Warnings lists steps that failed without stopping the import, such as
	// naming the folder after the topic.
	Warnings []string
}

// Begin creates inbox/IMPORT_YYYY-MM-DD_<name>/ and writes src's text to
// _source.md in it, before any agent runs, so nothing is lost if the agent
// fails. The name comes from src.Name, cleaned, with a number added when the
// folder exists. kind is the import kind (see ImportKind); a text that is
// only white space is ErrEmptySource. Nothing outside the new folder is
// touched.
func Begin(s store.Store, now time.Time, kind store.Kind, src loader.Source) (*Folder, error) {
	if strings.TrimSpace(src.Text) == "" {
		return nil, ErrEmptySource
	}
	f := &Folder{s: s, now: now, kind: kind, src: src}
	base := FolderPrefix + now.Format(dayLayout) + "_" + sourceName(src)
	rel, err := f.freeFolder(base, "")
	if err != nil {
		return nil, err
	}
	data, err := store.IntakeNote(SourceFile, kind, f.sourceProp(placeholderSource), now.Format(dayLayout), storedText(src.Text), nil)
	if err != nil {
		return nil, err
	}
	act, err := s.Create(filepath.Join(rel, SourceFile), data)
	if err != nil {
		return nil, err
	}
	f.Rel = rel
	f.acts = append(f.acts, act)
	return f, nil
}

// Action undoes (and redoes) everything written so far, for an import whose
// agent failed and so was never finished.
func (f *Folder) Action() undo.Action {
	return undo.Group("import "+filepath.Base(f.Rel), f.acts...)
}

// Finish writes the checked digest d into the folder: one file per note,
// _actions.md when there are action items, _facts.md when there are facts, _takeaways.md when there are takeaways, and the source set to the one the
// agent judged (a web import keeps its address). It then renames the folder
// to the agent's topic, or leaves its name when there is none or the rename
// fails. A note without a usable title or text is skipped. Nothing is
// written outside the folder, and no file ever replaces another.
//
// With nothing to write it returns ErrEmpty and leaves the folder as Begin
// made it. When a file fails to write, Result still holds everything done so
// far, so the caller can offer to undo it.
func (f *Folder) Finish(d Digest) (Result, error) {
	if f.done {
		return Result{}, ErrFinished
	}
	notes, actions, facts := usableNotes(d.Notes), cleanActions(d.Actions), cleanFacts(d.Facts)
	takeaways := cleanTakeaways(d.Takeaways)
	if len(notes) == 0 && len(actions) == 0 && len(facts) == 0 && takeaways.Empty() {
		return Result{}, ErrEmpty
	}
	f.done = true
	var warnings []string
	if err := f.renameToTopic(d.Topic); err != nil {
		warnings = append(warnings, "could not name the folder after its topic")
	}
	if d.Unmerged {
		warnings = append(warnings, "the parts of a large source could not be merged and are kept side by side")
	}
	src := f.sourceProp(sourceOr(d.Source))
	if f.src.Kind != loader.Web {
		if err := f.setSource(src); err != nil {
			warnings = append(warnings, "could not set the source of "+SourceFile)
		}
	}

	var files []string
	finish := func(err error) (Result, error) {
		return Result{Rel: f.Rel, Files: files, Action: f.Action(), KindMissing: f.kind == store.KindUnset, Warnings: warnings}, err
	}
	taken := map[string]bool{strings.ToLower(SourceFile): true, strings.ToLower(ActionsFile): true, strings.ToLower(FactsFile): true, strings.ToLower(TakeawaysFile): true}
	for _, n := range notes {
		name := freeName(intake.Name(n.Title), taken)
		data, err := store.IntakeNote(name, f.kind, src, f.now.Format(dayLayout), n.Body+"\n", n.Tags)
		if err == nil {
			err = f.create(name, data)
		}
		if err != nil {
			return finish(fmt.Errorf("write %s: %w", name, err))
		}
		files = append(files, name)
	}
	for _, l := range []struct {
		name  string
		items []string
		body  string
	}{{ActionsFile, actions, bulletList(actions)}, {FactsFile, facts, bulletList(facts)}, {TakeawaysFile, nil, takeawaysBody(takeaways)}} {
		if len(l.items) == 0 && l.body == "" {
			continue
		}
		data, err := store.IntakeNote(l.name, f.kind, src, f.now.Format(dayLayout), l.body, nil)
		if err == nil {
			err = f.create(l.name, data)
		}
		if err != nil {
			return finish(fmt.Errorf("write %s: %w", l.name, err))
		}
		files = append(files, l.name)
	}
	return finish(nil)
}

func (f *Folder) create(name string, data []byte) error {
	act, err := f.s.Create(filepath.Join(f.Rel, name), data)
	if err != nil {
		return err
	}
	f.acts = append(f.acts, act)
	return nil
}

// storedText is text as _source.md holds it and as the agent receives it, so
// a first run and a re-run see the same input: with a final line break.
func storedText(text string) string {
	if strings.HasSuffix(text, "\n") {
		return text
	}
	return text + "\n"
}

// sourceProp is the value of the source property: the address for a web
// import, else value.
func (f *Folder) sourceProp(value string) string {
	if f.src.Kind == loader.Web && f.src.Origin != "" {
		return f.src.Origin
	}
	return value
}

func sourceOr(s string) string {
	if strings.TrimSpace(s) == "" {
		return fallbackSource
	}
	return strings.TrimSpace(s)
}

// setSource changes the source property of _source.md.
func (f *Folder) setSource(value string) error {
	act, err := f.s.Edit(filepath.Join(f.Rel, SourceFile), func(fm *store.Frontmatter) {
		fm.SetString("source", value)
	})
	if err != nil {
		return err
	}
	f.acts = append(f.acts, act)
	return nil
}

// renameToTopic moves the folder to IMPORT_<date>_<topic>, with a number
// added when that is taken. An empty or unchanged name does nothing.
func (f *Folder) renameToTopic(topic string) error {
	name := cleanName(topic)
	if name == "" || f.keepName {
		return nil
	}
	want := FolderPrefix + f.now.Format(dayLayout) + "_" + name
	if want == filepath.Base(f.Rel) {
		return nil
	}
	rel, err := f.freeFolder(want, f.Rel)
	if err != nil || rel == f.Rel {
		return err
	}
	act, err := f.s.Move(f.Rel, rel)
	if err != nil {
		return err
	}
	f.acts = append(f.acts, act)
	f.Rel = rel
	return nil
}

// freeFolder returns the first of inbox/base, "base 2", "base 3" and so on
// that does not exist. own is the folder being renamed, which counts as free.
func (f *Folder) freeFolder(base, own string) (string, error) {
	for n := 1; n <= maxNameTries; n++ {
		name := base
		if n > 1 {
			name = fmt.Sprintf("%s %d", base, n)
		}
		rel := filepath.Join(inboxDir, name)
		if rel == own {
			return rel, nil
		}
		taken, err := f.s.Exists(rel)
		if err != nil {
			return "", err
		}
		if !taken {
			return rel, nil
		}
	}
	return "", intake.ErrNoFreeName
}

// sourceName names a folder after its source: the cleaned name, or a word for
// the kind of source when nothing is left.
func sourceName(src loader.Source) string {
	if name := cleanName(src.Name); name != "" {
		return name
	}
	switch src.Kind {
	case loader.Clipboard:
		return "clipboard"
	case loader.Web:
		return "web"
	}
	return "file"
}

// usableNotes cleans notes that did not come through Parse (a hand-made
// digest) and drops those without a title or text.
func usableNotes(in []Note) []Note {
	var out []Note
	for _, n := range in {
		title := cleanName(n.Title)
		body := strings.TrimSpace(textsafe.Sanitize(n.Body))
		if title == "" || body == "" {
			continue
		}
		out = append(out, Note{Title: title, Body: cutRunes(body, MaxBodyRunes), Tags: cleanTags(n.Tags)})
	}
	return out
}

// freeName returns name, or "stem 2.md", "stem 3.md" and so on, so that no
// two files in the folder share a name ignoring case, and records it in taken.
func freeName(name string, taken map[string]bool) string {
	stem := strings.TrimSuffix(name, ".md")
	final := name
	for n := 2; taken[strings.ToLower(final)]; n++ {
		final = fmt.Sprintf("%s %d.md", stem, n)
	}
	taken[strings.ToLower(final)] = true
	return final
}

func bulletList(items []string) string {
	var b strings.Builder
	for _, a := range items {
		b.WriteString("- " + a + "\n")
	}
	return b.String()
}

// hasFolderPrefix reports whether name starts with FolderPrefix, ignoring case.
func hasFolderPrefix(name string) bool {
	return len(name) >= len(FolderPrefix) && strings.EqualFold(name[:len(FolderPrefix)], FolderPrefix)
}

// takeawaysBody writes each section that has items under its heading.
func takeawaysBody(t Takeaways) string {
	var sections []string
	for _, s := range []struct {
		heading string
		items   []string
	}{{"Key points", t.KeyPoints}, {"Conclusions", t.Conclusions}, {"Decisions", t.Decisions}} {
		if len(s.items) > 0 {
			sections = append(sections, "## "+s.heading+"\n\n"+bulletList(s.items))
		}
	}
	return strings.Join(sections, "\n")
}
