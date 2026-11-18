package main

import (
	"fmt"
	"path"
	"strconv"

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

// bundleLabels are the Content tab labels for the note shown: the note, then
// a number for each bundled image. Numbers keep the labels short and plain; the
// name of the open image is in the caption.
func (m model) bundleLabels() []string {
	if len(m.bundle) == 0 {
		return nil
	}
	labels := []string{"note"}
	for i := range m.bundle {
		labels = append(labels, strconv.Itoa(i+1))
	}
	return labels
}

// syncTabs draws the tabs for the bundle of the note shown, or none.
func (m model) syncTabs() model {
	m.content = m.content.SetTabs(m.bundleLabels(), m.bundleTab)
	return m
}

// stepBundleTab opens the next (delta 1) or previous (delta -1) Content tab,
// wrapping around as the PARA tabs do. Tab 0 is the note and the others are its
// bundled images.
func (m model) stepBundleTab(delta int) model {
	entry, ok := m.para.SelectedNote()
	if !ok || len(m.bundle) == 0 {
		return m
	}
	n := len(m.bundle) + 1
	m.bundleTab = (m.bundleTab + delta + n) % n
	if m.bundleTab == 0 {
		note, err := m.store.ReadNote(entry.Rel)
		if err != nil {
			m.content = m.content.Fail(entry.Rel, err)
		} else {
			m.content = m.content.LoadNote(entry.Rel, m.wantFront(), note.Body)
		}
		return m.syncTabs()
	}
	name := m.bundle[m.bundleTab-1]
	label := fmt.Sprintf("%d/%d %s", m.bundleTab, len(m.bundle), name)
	m.content = m.imagePreview(path.Join(store.SidecarOf(entry.Rel), name), label)
	return m.syncTabs()
}

// imagePreview returns Content showing a scaled preview of the image at rel.
// label, when set, starts the caption.
func (m model) imagePreview(rel, label string) content.Model {
	file, err := m.store.Path(rel)
	if err != nil {
		return m.content.Fail(rel, err)
	}
	pic, err := imgpreview.Open(file)
	if err != nil {
		return m.content.Fail(rel, err)
	}
	caption := pic.Caption()
	if label != "" {
		caption = label + ", " + caption
	}
	proto := m.imageProto
	return m.content.LoadPreview(rel, content.Preview{
		Render:  func(cols, rows int) []string { return pic.Render(proto, cols, rows) },
		Caption: caption,
		Cleanup: imgpreview.Cleanup(proto),
	})
}

// showImage fills Content with a scaled preview of the image entry, and
// clears the panels that only describe notes.
func (m model) showImage(entry store.Entry) model {
	m.related = m.related.Clear()
	m.info = m.info.Clear()
	m.title = m.title.Set(entry.Name)
	m.content = m.imagePreview(entry.Rel, "")
	return m
}
