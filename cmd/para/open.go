package main

import (
	"path"

	"github.com/tkilb/para/internal/clip"
	"github.com/tkilb/para/internal/store"
	"github.com/tkilb/para/internal/ui/popup"
)

// openWeb sends the web address in the source property of the note under
// the PARA cursor to the default browser. A note whose source is not an http
// or https address, and a folder, only get a notice.
func (m model) openWeb() model {
	// A loose image or PDF row is not a note, so SelectedNote skips it.
	if row, found := m.para.Selected(); found && row.IsImage {
		return m.openImage(row)
	}
	entry, ok := m.para.SelectedNote()
	if ok && m.bundleTab > 0 {
		return m.openImage(entry)
	}
	if !ok {
		return m.notify(popup.Info, "Select a note with a web address in its source to open it.")
	}
	note, err := m.store.ReadNote(entry.Rel)
	if err != nil {
		return m.notify(popup.Warning, "Could not read the note: "+err.Error()+".")
	}
	var source string
	if v := note.Frontmatter.Get("source"); v != nil {
		source = v.Value
	}
	u, err := clip.ParseURL(source)
	if err != nil {
		return m.notify(popup.Info, "This note has no web address in its source.")
	}
	if err := m.openURL(u.String()); err != nil {
		return m.notify(popup.Warning, capitalize(err.Error())+".")
	}
	return m
}

// openImage opens the image on show, an image file or the open image tab of
// a note, in the system's preferred viewer.
func (m model) openImage(entry store.Entry) model {
	rel := entry.Rel
	if !entry.IsImage {
		if m.bundleTab < 1 || m.bundleTab > len(m.bundle) {
			return m
		}
		rel = path.Join(store.SidecarOf(entry.Rel), m.bundle[m.bundleTab-1])
	}
	file, err := m.store.Path(rel)
	if err != nil {
		return m.notify(popup.Warning, "Could not find the image: "+err.Error()+".")
	}
	if err := m.openFile(file); err != nil {
		return m.notify(popup.Warning, capitalize(err.Error())+".")
	}
	return m
}
