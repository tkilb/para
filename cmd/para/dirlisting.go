package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/tkilb/para/internal/store"
	"github.com/tkilb/para/internal/ui/content"
	"github.com/tkilb/para/internal/ui/kindstyle"
)

// dirItems turns the entries of a folder into Content's listing, in the order
// PARA lists them: a folder has a blank badge, a note has its kind's badge
// and color as in PARA, an image has the IMG badge, and ".md" or ".png" is
// dropped from a name.
func (m model) dirItems(entries []store.Entry) []content.ListItem {
	items := make([]content.ListItem, len(entries))
	for i, e := range entries {
		if e.IsDir {
			items[i] = content.ListItem{Badge: strings.Repeat(" ", kindstyle.BadgeWidth), Name: e.Name, Dir: true}
			continue
		}
		if e.IsImage {
			badge, color := kindstyle.ImageStyle()
			items[i] = content.ListItem{Badge: badge, Color: color, Name: e.Name[:len(e.Name)-len(".png")]}
			continue
		}
		kind := store.KindUnset
		if n, err := m.store.ReadNote(e.Rel); err == nil {
			kind = n.Kind
		}
		badge, color := kindstyle.Badge(kind)
		items[i] = content.ListItem{Badge: badge, Color: lipgloss.Color(color), Name: strings.TrimSuffix(e.Name, ".md")}
	}
	return items
}
