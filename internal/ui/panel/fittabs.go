package panel

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	// tabPrefixLen is the width of the "[0]-" that starts a tab title.
	tabPrefixLen = len("[0]-")
	// minTabText is the narrowest an active tab is cut to, "ab…", before the
	// other tabs are dropped to give it room.
	minTabText = 3
)

// TabLabel is the text of one tab: Full is its name and Short what stands for
// it when there is no room, such as a number.
type TabLabel struct {
	Full, Short string
}

// FitTabs chooses the text of each tab so the row fits the title of a panel
// width columns wide, using the first of these that fits:
//
//  1. every tab by its full name;
//  2. the active tab by its full name and the others by their short form;
//  3. the active tab cut with "…" to the room the short forms leave;
//  4. the active tab alone, cut to the width.
//
// The result is for FrameFittedTabs. It is nil when labels is empty.
func FitTabs(labels []TabLabel, active, width int) []Tab {
	if len(labels) == 0 {
		return nil
	}
	active = min(max(active, 0), len(labels)-1)
	budget := width - leadDashes - tabPrefixLen

	build := func(text func(i int, l TabLabel) string) []Tab {
		tabs := make([]Tab, len(labels))
		for i, l := range labels {
			tabs[i] = Tab{Label: text(i, l), Active: i == active}
		}
		return tabs
	}
	full := build(func(_ int, l TabLabel) string { return l.Full })
	if tabsWidth(full) <= budget {
		return full
	}
	short := build(func(i int, l TabLabel) string {
		if i == active {
			return l.Full
		}
		return l.Short
	})
	if tabsWidth(short) <= budget {
		return short
	}
	room := budget - (tabsWidth(short) - ansi.StringWidth(labels[active].Full))
	if room >= minTabText {
		short[active].Label = ansi.Truncate(labels[active].Full, room, "…")
		return short
	}
	return []Tab{{Label: ansi.Truncate(labels[active].Full, max(budget, 1), "…"), Active: true}}
}

// FrameFittedTabs is like FrameTabs for tabs already fitted to the width by
// FitTabs, so their text is drawn as given instead of being shrunk to first
// letters.
func FrameFittedTabs(number rune, tabs []Tab, body string, width, height int, focused bool, footer ...Footer) string {
	lineStyle := lipgloss.NewStyle().Foreground(borderColor(focused))
	prefix := "[" + string(number) + "]-"
	budget := width - leadDashes - len(prefix)
	var top string
	if tabsWidth(tabs) > budget {
		top = topBorder(prefix+tabsPlain(tabs), width, focused, lineStyle)
	} else {
		top = renderTabsBorder(prefix, tabs, budget, focused, lineStyle)
	}
	return frameBody(top, firstFooter(footer), body, width, height, lineStyle)
}
