package para

import (
	"path/filepath"
	"strings"
)

// Dir returns the folder the panel is looking at, relative to the notebook
// root, such as "projects/alpha" (or just "inbox" at a tab root).
func (m Model) Dir() string {
	return filepath.ToSlash(filepath.Join(tabDefs[m.active].dir, m.tabs[m.active].dir))
}

// Shows reports whether the directory the panel is looking at is the one
// that holds the note at rel.
func (m Model) Shows(rel string) bool {
	dir := filepath.Join(tabDefs[m.active].dir, m.tabs[m.active].dir)
	return filepath.Dir(filepath.ToSlash(rel)) == filepath.ToSlash(dir)
}

// Reveal selects the note at rel (relative to the notebook root): it
// switches to the tab holding it, enters its directory and puts the cursor
// on it. It reports false, changing nothing, when rel is not inside a tab
// folder. Anything tied to the directory it leaves (selection, search) is
// dropped, as when leaving it by hand.
func (m Model) Reveal(rel string) (Model, bool) {
	if !filepath.IsLocal(rel) {
		return m, false
	}
	rel = filepath.ToSlash(filepath.Clean(rel))
	for i, d := range tabDefs {
		inside, ok := strings.CutPrefix(rel, d.dir+"/")
		if !ok {
			continue
		}
		dir, name := filepath.Split(inside)
		t := m.tabs[i]
		t.dir = filepath.Clean(strings.TrimSuffix(dir, "/"))
		if t.dir == "." {
			t.dir = ""
		}
		t.keep = name
		t.selected, t.visual, t.search = nil, visualState{}, ""
		m.tabs[i] = t
		m.active = i
		return m.load(i).syncScroll(), true
	}
	return m, false
}

// Location returns the folder of the active tab (such as "projects") and the
// folder inside it that the panel is in (such as "deep/er", empty at the top).
func (m Model) Location() (space, sub string) {
	return tabDefs[m.active].dir, filepath.ToSlash(m.tabs[m.active].dir)
}

// SpaceDirs returns the labels and folders of the tabs, in tab order.
func SpaceDirs() (labels, dirs []string) {
	for _, d := range tabDefs {
		labels, dirs = append(labels, d.label), append(dirs, d.dir)
	}
	return labels, dirs
}
