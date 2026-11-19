package main

import (
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tkilb/para/internal/findtext"
	"github.com/tkilb/para/internal/store"
	"github.com/tkilb/para/internal/textsafe"
	"github.com/tkilb/para/internal/ui/filterbar"
	"github.com/tkilb/para/internal/ui/grep"
	"github.com/tkilb/para/internal/ui/notefind"
	"github.com/tkilb/para/internal/ui/para"
	"github.com/tkilb/para/internal/ui/picker"
	"github.com/tkilb/para/internal/ui/popup"
)

const (
	findFileTitle = "Find note"
	recentTitle   = "Recent notes"
	// valueSep splits a note's path from a line number in a picker value;
	// a path cannot hold a NUL byte.
	valueSep = "\x00"
)

// noteName is a note's file name without .md.
func noteName(rel string) string {
	return strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel))
}

// openFileFinder lists every note in the notebook by its path without .md
// (such as "projects/deep/Rocket Plan"), sorted by path, so what is typed
// matches the folders as well as the name, though a word that matches the
// file name ranks above one that only matches a folder. The Space, Kind and
// Dir boxes and the ctrl+f popup narrow the list, as in the text search. It
// jumps PARA to the one chosen.
func (m model) openFileFinder() (model, tea.Cmd) {
	rels := slices.Clone(m.links.Rels())
	slices.SortFunc(rels, func(a, b string) int {
		return strings.Compare(strings.ToLower(a), strings.ToLower(b))
	})
	notes := findtext.Notes(m.store, rels)
	if len(notes) == 0 {
		return m.notify(popup.Info, "There are no notes to find."), nil
	}
	item := func(n findtext.Line) picker.Item { return noteItem(n, "") }
	f, cmd := notefind.New(findFileTitle, notes, item, filterbar.Options{Spaces: filterSpaces()})
	m.notefind = &f
	return m, cmd
}

// noteItem is the picker item for a note: its path without .md, with detail
// shown dim at the right.
func noteItem(n findtext.Line, detail string) picker.Item {
	return picker.Item{
		Label:  strings.TrimSuffix(filepath.ToSlash(n.Rel), filepath.Ext(n.Rel)),
		Name:   noteName(n.Rel),
		Value:  n.Rel,
		Detail: detail,
	}
}

// recentLimit is how many notes the recents popup lists.
const recentLimit = 200

// openRecent opens the note finder over the notes changed most recently,
// latest first, at most recentLimit of them after the filters. What is typed
// narrows the list by fuzzy match, ranked by how well the name matches and,
// among equal matches, by how recently the note changed. Each row says how
// long ago the note changed. It jumps PARA to the one chosen.
func (m model) openRecent() (model, tea.Cmd) {
	notes := findtext.NewestFirst(findtext.Notes(m.store, m.links.Rels()))
	if len(notes) == 0 {
		return m.notify(popup.Info, "There are no notes yet."), nil
	}
	now := time.Now()
	item := func(n findtext.Line) picker.Item { return noteItem(n, findtext.Ago(now, n.Mod)) }
	f, cmd := notefind.New(recentTitle, notes, item, filterbar.Options{Spaces: filterSpaces()})
	f = f.Recent(recentLimit)
	m.notefind = &f
	return m, cmd
}

// filterSpaces are the five PARA folders, for the Space box of a finder.
func filterSpaces() []filterbar.Space {
	labels, dirs := para.SpaceDirs()
	spaces := make([]filterbar.Space, len(dirs))
	for i := range dirs {
		spaces[i] = filterbar.Space{Label: labels[i], Dir: dirs[i]}
	}
	return spaces
}

// notefindKey feeds a message to the open note finder and applies its
// outcome.
func (m model) notefindKey(msg tea.Msg) (model, tea.Cmd) {
	f, cmd, out := m.notefind.Update(msg, m.height)
	m.notefind = &f
	switch out {
	case notefind.Cancelled:
		m.notefind = nil
	case notefind.Chosen:
		item, _ := f.Chosen()
		m.notefind = nil
		return m.chooseFoundNote(item.Value), nil
	}
	return m, cmd
}

// openTextFinder opens the text search: type words to list the lines, in
// the notes of the notebook, that contain all of them (exact text, ignoring
// case), with a preview of the note beside the list. The space and dir
// filters above the list start at the folder PARA shows. It opens the note at
// the line chosen.
func (m model) openTextFinder() (model, tea.Cmd) {
	res := findtext.Lines(m.store, m.links.Rels())
	if len(res.Lines) == 0 {
		return m.notify(popup.Info, "There is no text to search in this notebook."), nil
	}
	space, sub := m.para.Location()
	g, cmd := grep.New(res.Lines, res.Truncated, previewLoader(m.store),
		grep.Options{Spaces: filterSpaces(), Space: space, Dir: sub})
	m.grep = &g
	return m, cmd
}

// previewLoader returns the function that reads a note's lines, as Content
// shows them, for the text search's preview.
func previewLoader(s store.Store) grep.Loader {
	return func(rel string) []string {
		n, err := s.ReadNote(rel)
		if err != nil {
			return nil
		}
		return strings.Split(textsafe.Sanitize(strings.Trim(n.Body, "\r\n")), "\n")
	}
}

// grepKey feeds a message to the open text search and applies its outcome.
func (m model) grepKey(msg tea.Msg) (model, tea.Cmd) {
	g, cmd, out := m.grep.WithScreen(m.width, m.height).Update(msg)
	m.grep = &g
	switch out {
	case grep.Cancelled:
		m.grep = nil
	case grep.Chosen:
		hit, _ := g.Chosen()
		m.grep = nil
		return m.chooseFoundNote(hit.Rel + valueSep + strconv.Itoa(hit.Index)), nil
	}
	return m, cmd
}

// chooseFoundNote opens the note chosen in a finder. A text hit also puts
// Content's cursor on the line and focuses Content; a note hit focuses PARA.
func (m model) chooseFoundNote(value string) model {
	rel, lineText, hasLine := strings.Cut(value, valueSep)
	line := -1
	if hasLine {
		if n, err := strconv.Atoi(lineText); err == nil {
			line = n
		}
	}
	para, ok := m.para.Reveal(rel)
	if !ok {
		return m.notify(popup.Warning, "That note is outside the PARA folders.")
	}
	m.para = para
	m = m.syncSelection()
	if line >= 0 {
		m.content, m.focus = m.content.SelectRow(line), panelContent
	} else {
		m.focus = panelPARA
	}
	return m
}
