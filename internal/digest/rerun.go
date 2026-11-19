package digest

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/tkilb/para/internal/clip"
	"github.com/tkilb/para/internal/loader"
	"github.com/tkilb/para/internal/store"
	"github.com/tkilb/para/internal/undo"
)

// ErrNotImport reports a folder that is not an import folder: its name does
// not start with FolderPrefix, or it has no _source.md.
var ErrNotImport = errors.New("not an import folder")

// Import runs one whole import: it begins the folder (so the source is
// saved first), asks dg for the digest of the source, and finishes the
// folder. When the agent fails, the folder with its _source.md is kept: the
// returned Result then holds its path and an Action that undoes it, and the
// error is the agent's. A source that is only white space is ErrEmptySource
// and writes nothing.
func Import(ctx context.Context, s store.Store, now time.Time, kind store.Kind, dg Digester, src loader.Source) (Result, error) {
	f, err := Begin(s, now, kind, src)
	if err != nil {
		return Result{}, err
	}
	d, err := dg.Digest(ctx, storedText(src.Text))
	if err != nil {
		return Result{Rel: f.Rel, Action: f.Action(), KindMissing: kind == store.KindUnset}, err
	}
	return f.Finish(d)
}

// FindImportFolder returns the import folder that rel is, or is inside: the
// nearest folder on the path whose name starts with FolderPrefix (any case), so the
// cursor on a file in an import folder finds it. A final part ending in .md
// is a note, not a folder. It only reads the path; Rerun checks the folder.
func FindImportFolder(rel string) (string, bool) {
	parts := strings.Split(filepath.ToSlash(filepath.Clean(rel)), "/")
	for i := len(parts) - 1; i >= 0; i-- {
		name := parts[i]
		if i == len(parts)-1 && strings.EqualFold(filepath.Ext(name), ".md") {
			continue
		}
		if hasFolderPrefix(name) {
			return filepath.Join(parts[:i+1]...), true
		}
	}
	return "", false
}

// Rerun runs the agent again over the _source.md of the import folder at rel
// and replaces what was generated: every note in the folder except
// _source.md goes to the trash, and the new files are written in their
// place. The folder keeps its name, the files keep the original import day,
// and a folder moved into PARA works as well as one in the inbox. Folders
// inside it and files that are not notes are left alone.
//
// The old files are trashed only once the agent has returned something
// usable, so a failed or empty run (ErrEmpty) leaves the folder exactly as
// it was. The whole re-run is one undo entry that brings the old files back.
// When a file fails to write, Result still holds an Action for everything
// done so far.
func Rerun(ctx context.Context, s store.Store, now time.Time, kind store.Kind, dg Digester, rel string) (Result, error) {
	src, day, err := readImport(s, now, rel)
	if err != nil {
		return Result{}, err
	}
	d, err := dg.Digest(ctx, storedText(src.Text))
	if err != nil {
		return Result{}, err
	}
	if len(usableNotes(d.Notes)) == 0 && len(cleanActions(d.Actions)) == 0 && len(cleanFacts(d.Facts)) == 0 && len(cleanConclusions(d.Conclusions)) == 0 {
		return Result{}, ErrEmpty
	}
	trashed, err := trashGenerated(s, rel)
	if err != nil {
		return Result{}, err
	}
	f := &Folder{Rel: rel, s: s, now: day, kind: kind, src: src, keepName: true}
	res, err := f.Finish(d)
	res.Action = undo.Group("re-run "+filepath.Base(rel), append(trashed, f.acts...)...)
	return res, err
}

// readImport checks that rel is an import folder and reads its source: the
// text, where it came from, and the day it was imported.
func readImport(s store.Store, now time.Time, rel string) (loader.Source, time.Time, error) {
	if !hasFolderPrefix(filepath.Base(rel)) {
		return loader.Source{}, time.Time{}, ErrNotImport
	}
	n, err := s.ReadNote(filepath.Join(rel, SourceFile))
	if errors.Is(err, store.ErrNotFound) {
		return loader.Source{}, time.Time{}, ErrNotImport
	}
	if err != nil {
		return loader.Source{}, time.Time{}, err
	}
	if strings.TrimSpace(n.Body) == "" {
		return loader.Source{}, time.Time{}, ErrEmptySource
	}
	src := loader.Source{Kind: loader.File, Text: n.Body}
	day := now
	if fm := n.Frontmatter; fm != nil {
		if node := fm.Get("source"); node != nil {
			if _, err := clip.ParseURL(node.Value); err == nil {
				src.Kind, src.Origin = loader.Web, strings.TrimSpace(node.Value)
			}
		}
		if node := fm.Get("date"); node != nil {
			if t, err := time.ParseInLocation(dayLayout, node.Value, time.Local); err == nil {
				day = t
			}
		}
	}
	return src, day, nil
}

// trashGenerated moves every note directly in the folder, except _source.md,
// to the trash. When one fails it puts back the ones already moved.
func trashGenerated(s store.Store, rel string) ([]undo.Action, error) {
	entries, err := s.List(rel, store.ListOptions{})
	if err != nil {
		return nil, err
	}
	var acts []undo.Action
	for _, e := range entries {
		if e.IsDir || strings.EqualFold(e.Name, SourceFile) {
			continue
		}
		act, err := s.Trash(e.Rel)
		if err != nil {
			_ = undo.Group("", acts...).Undo()
			return nil, err
		}
		acts = append(acts, act)
	}
	return acts, nil
}
