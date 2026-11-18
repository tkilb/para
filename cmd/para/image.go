package main

import (
	"context"
	"errors"
	"time"

	"github.com/tkilb/para/internal/attach"
	"github.com/tkilb/para/internal/imgpreview"
	"github.com/tkilb/para/internal/store"
	"github.com/tkilb/para/internal/ui/content"
	"github.com/tkilb/para/internal/ui/popup"
)

// pasteImage saves the image on the clipboard as a PNG in the folder PARA
// shows, as one undo entry, and puts the cursor on it.
func (m model) pasteImage() model {
	res, err := attach.Paste(context.Background(), m.store, m.imageClip, m.para.Dir(), time.Now())
	switch {
	case errors.Is(err, attach.ErrNoImage):
		return m.notify(popup.Info, "The clipboard has no image.")
	case err != nil:
		return m.notify(popup.Warning, "Could not paste the image: "+err.Error()+".")
	}
	m.history.Push(res.Action)
	m.para = m.para.Reload()
	if p, ok := m.para.Reveal(res.Rel); ok {
		m.para = p
	}
	return m
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
