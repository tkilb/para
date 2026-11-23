// Package kindstyle gives each note kind its short badge text and color, so
// every panel shows a kind the same way.
package kindstyle

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"

	"github.com/tkilb/para/internal/store"
)

// BadgeWidth is the width of every badge; shorter ones are padded.
const BadgeWidth = 4

// UnsetBadge stands in for a note whose kind is unset.
const UnsetBadge = "----"

// unsetColor is a dark gray, dim enough not to clash with the background.
var unsetColor = lipgloss.Color("238")

var styles = map[store.Kind]struct {
	badge string
	color lipgloss.Color
}{
	store.KindFact:     {"fact", lipgloss.Color("2")},
	store.KindRumor:    {"rumr", lipgloss.Color("3")},
	store.KindProposal: {"prop", lipgloss.Color("6")},
	store.KindFossil:   {"foss", lipgloss.Color("8")},
	store.KindIdea:     {"idea", lipgloss.Color("4")},
	store.KindDraft:    {"draf", lipgloss.Color("5")},
	store.KindLink:     {"link", lipgloss.Color("111")},
}

// ImageBadge is the badge of a PNG image, which has no kind property. It is
// right-aligned in the badge width, to set it apart from the note kinds.
const ImageBadge = " IMG"

// imageColor is the color of ImageBadge.
var imageColor = lipgloss.Color("13")

// PDFBadge is the badge of a PDF, set the same way as ImageBadge.
const PDFBadge = " PDF"

// AttachmentStyle returns the badge and color of the file called name: PDFBadge
// for a PDF, else the image badge.
func AttachmentStyle(name string) (string, lipgloss.Color) {
	if store.ImageExt(name) == store.PDFExt {
		return PDFBadge, imageColor
	}
	return ImageBadge, imageColor
}

// ImageStyle returns the badge and color of a PNG image.
func ImageStyle() (string, lipgloss.Color) { return ImageBadge, imageColor }

// addedColor is the color of a kind the config added without one.
var addedColor = lipgloss.Color("7")

// Badge returns the four-character badge and color for k: what the config
// gave it, else the built-in style, else for a kind the config added a badge
// made of the first letters of its name and a plain color.
func Badge(k store.Kind) (string, lipgloss.Color) {
	cfgBadge, cfgColor := store.KindStyle(k)
	builtin, isBuiltin := styles[k]
	if !isBuiltin && !configured(k) {
		return UnsetBadge, unsetColor
	}

	badge, color := builtin.badge, builtin.color
	if !isBuiltin {
		badge, color = firstRunes(k.String(), BadgeWidth), addedColor
	}
	if cfgBadge != "" {
		badge = cfgBadge
	}
	if cfgColor != "" {
		color = lipgloss.Color(cfgColor)
	}
	return fmt.Sprintf("%-*s", BadgeWidth, badge), color
}

// Color returns the color for k.
func Color(k store.Kind) lipgloss.Color {
	_, c := Badge(k)
	return c
}

// configured reports whether k is a kind the config added.
func configured(k store.Kind) bool {
	for _, known := range store.Kinds() {
		if known == k {
			return true
		}
	}
	return false
}

// firstRunes returns at most n characters of s.
func firstRunes(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}
