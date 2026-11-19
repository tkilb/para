package main

import (
	"fmt"
	"path"
	"strconv"

	"github.com/tkilb/para/internal/attach"
	"github.com/tkilb/para/internal/imgpreview"
	"github.com/tkilb/para/internal/store"
	"github.com/tkilb/para/internal/textsafe"
	"github.com/tkilb/para/internal/ui/content"
	"github.com/tkilb/para/internal/ui/kindstyle"
	"github.com/tkilb/para/internal/ui/panel"
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

// bundleTabMark starts the title of each image tab, the same diamond the note
// row shows in PARA.
const bundleTabMark = "❖"

// bundleLabels are the Content tabs for the note shown: "Content" for the note
// text, then one for each bundled image named for its file without ".png" and led by a diamond.
// Where the title is too narrow for every name, the tabs that are not open
// shrink to "C" and a number, and the open name is cut to fit.
func (m model) bundleLabels() []panel.TabLabel {
	if len(m.bundle) == 0 {
		return nil
	}
	_, imageColor := kindstyle.ImageStyle() // the color of the IMG badge
	labels := []panel.TabLabel{{Full: "Content", Short: "C"}}
	for i, name := range m.bundle {
		stem := textsafe.SingleLine(name[:len(name)-len(".png")])
		labels = append(labels, panel.TabLabel{Full: bundleTabMark + stem, Short: strconv.Itoa(i + 1), MarkColor: imageColor})
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
	n := len(m.bundle) + 1
	return m.openBundleTab((m.bundleTab + delta + n) % n)
}

// openBundleTab opens Content tab number tab: 0 is the note, i the image
// bundle[i-1]. It does nothing when the note has no bundle or tab is out of
// range.
func (m model) openBundleTab(tab int) model {
	entry, ok := m.para.SelectedNote()
	if !ok || len(m.bundle) == 0 || tab < 0 || tab > len(m.bundle) {
		return m
	}
	m.bundleTab = tab
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
