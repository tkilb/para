package main

import (
	"github.com/tkilb/para/internal/attach"
	"github.com/tkilb/para/internal/imgpreview"
	"github.com/tkilb/para/internal/store"
	"github.com/tkilb/para/internal/ui/content"
	"github.com/tkilb/para/internal/ui/popup"
)

// finishImage records the undo entry for an image just saved, refreshes the
// panels and puts the cursor on the new file. A failure is reported and
// nothing was written.
func (m model) finishImage(res attach.Result, err error) model {
	if err != nil {
		return m.notify(popup.Warning, "Could not save the image: "+err.Error()+".")
	}
	m.history.Push(res.Action)
	m.para = m.para.Reload()
	if p, ok := m.para.Reveal(res.Rel); ok {
		m.para = p
	}
	return m
}

// bundledImages lists the file names of the images in the note's sidecar
// folder, or nil when it has none.
func (m model) bundledImages(entry store.Entry) []string {
	if entry.Attachments == 0 {
		return nil
	}
	entries, err := m.store.List(store.SidecarOf(entry.Rel), store.ListOptions{IncludeImages: true})
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsImage {
			names = append(names, e.Name)
		}
	}
	return names
}

// showImage fills Content with a scaled preview of the image entry, and
// clears the panels that only describe notes.
func (m model) showImage(entry store.Entry) model {
	m.related = m.related.Clear()
	m.info = m.info.Clear()
	m.title = m.title.Set(entry.Name)
	path, err := m.store.Path(entry.Rel)
	if err != nil {
		m.content = m.content.Fail(entry.Rel, err)
		return m
	}
	pic, err := imgpreview.Open(path)
	if err != nil {
		m.content = m.content.Fail(entry.Rel, err)
		return m
	}
	proto := m.imageProto
	m.content = m.content.LoadPreview(entry.Rel, content.Preview{
		Render:  func(cols, rows int) []string { return pic.Render(proto, cols, rows) },
		Caption: pic.Caption(),
		Cleanup: imgpreview.Cleanup(proto),
	})
	return m
}
