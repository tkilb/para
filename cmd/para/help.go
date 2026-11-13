package main

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tkilb/para/internal/ui/helppopup"
	"github.com/tkilb/para/internal/ui/statusbar"
)

// helpGlobalBindings are the keys that work whatever panel has focus. Panel
// switching is movement, so it is under Navigation, as in lazygit.
var helpGlobalBindings = []statusbar.Binding{
	{Key: "s", Label: "open the scratch pad (_scratch.md in the notebook root) in $EDITOR"},
	{Key: "S", Label: "pin the selected note as a sticky (or unpin it)"},
	{Key: "ctrl+f", Label: "find a note by name or path anywhere in the notebook (best file name matches first)"},
	{Key: "ctrl+/ or F", Label: "search text in the notes, grep style, filtered by Space and Dir (tab switches fields; opens the note at the line)"},
	{Key: "B", Label: "choose the notebook to open (your place in each is remembered)"},
	{Key: "A", Label: "Add Note: add a note in the folder PARA shows (tab, ctrl+s saves)"},
	{Key: "I", Label: "import the clipboard, a web page or a file with the agent; on an IMPORT_ folder, run it again"},
	{Key: "U", Label: "clip a web page: a title (optional) and a URL; saved in the folder PARA shows"},
	{Key: "o", Label: "open the web address in the note's source in the browser"},
	{Key: "ctrl+t", Label: "append a template to the end of the selected note (u undoes it)"},
	{Key: "ctrl+y", Label: "glossary: fuzzy-pick a term from glossary.yaml in the notebook root (enter shows it; ctrl+n adds, ctrl+e edits, ctrl+x deletes)"},
	{Key: "b/t/T", Label: "open yesterday's/today's/tomorrow's daily note"},
	{Key: "u", Label: "undo"},
	{Key: "ctrl+r", Label: "redo"},
	{Key: "ctrl+g", Label: "open lazygit in the notebook's folder (para returns when you quit it)"},
	{Key: "ctrl+l", Label: "refresh from disk"},
	{Key: "ctrl+z", Label: "suspend"},
	{Key: "+", Label: "zoom/unzoom the focused panel"},
	{Key: "?", Label: "help"},
	{Key: "q", Label: "quit"},
}

var helpNavigationBindings = []statusbar.Binding{
	{Key: "0-4", Label: "jump to panel"},
	{Key: "tab/shift+tab", Label: "next/prev panel"},
}

// helpParaBindings is the full PARA list; the status bar shows a trimmed one.
var helpParaBindings = []statusbar.Binding{
	{Key: "↑/↓", Label: "move"},
	{Key: "←", Label: "go to the parent directory"},
	{Key: "→/enter", Label: "open a directory or note"},
	{Key: "gg/G", Label: "first/last entry"},
	{Key: "ctrl+u/d, pgup/pgdn", Label: "page up/down"},
	{Key: "[ ]", Label: "previous/next tab"},
	{Key: "space", Label: "select and move down"},
	{Key: "v/V", Label: "visual select/unselect"},
	{Key: "ctrl+a", Label: "select all"},
	{Key: "esc", Label: "leave visual mode, search, selection"},
	{Key: "y/x", Label: "yank/cut"},
	{Key: "Y/X", Label: "cancel yank/cut"},
	{Key: "p/P", Label: "paste/paste over"},
	{Key: "d/D", Label: "trash/delete permanently"},
	{Key: "a", Label: "new note (end with / for folder)"},
	{Key: "r", Label: "rename (updates references)"},
	{Key: "K", Label: "set the note's kind"},
	{Key: "m", Label: "move the note to a tab (keys i p a r A, or arrows)"},
	{Key: "e", Label: "edit in $EDITOR"},
	{Key: "/", Label: "search names"},
	{Key: "n/N", Label: "next/previous match"},
	{Key: ".", Label: "show hidden files"},
}

var helpStickiesBindings = []statusbar.Binding{
	{Key: "↑/↓", Label: "choose a sticky"},
	{Key: "enter", Label: "jump to the note"},
	{Key: "d", Label: "unpin the sticky"},
	{Key: "esc/←", Label: "back to PARA"},
}

var helpRelatedBindings = []statusbar.Binding{
	{Key: "↑/↓", Label: "move"},
	{Key: "enter", Label: "jump to the note"},
	{Key: "l", Label: "add a reference (pick a note)"},
	{Key: "d", Label: "remove the selected reference"},
	{Key: "esc/←", Label: "back to PARA"},
}

var helpContentBindings = []statusbar.Binding{
	{Key: "↑/↓", Label: "move the cursor line (your place is remembered)"},
	{Key: "ctrl+d/u", Label: "half page down/up"},
	{Key: "pgdn/pgup", Label: "page down/up"},
	{Key: "g/G", Label: "first/last line"},
	{Key: "{ }", Label: "previous/next paragraph (blank line)"},
	{Key: "space", Label: "pick the line and move down"},
	{Key: "v/V", Label: "select/unselect a range as you move"},
	{Key: "ctrl+a", Label: "select all"},
	{Key: "m", Label: "switch between the rendered Markdown view and plain text"},
	{Key: "y", Label: "yank (copy) the selection, or the line, to the clipboard"},
	{Key: "enter", Label: "extract the selection into a new note"},
	{Key: "r", Label: "rename this note (updates references)"},
	{Key: "e", Label: "edit this note in $EDITOR"},
	{Key: "esc", Label: "close the range, clear the selection, then leave"},
}

var helpInfoBindings = []statusbar.Binding{
	{Key: "↑/↓", Label: "scroll"},
	{Key: "esc/←", Label: "back to PARA"},
}

// helpSections returns the help popup's content for the focused panel: its
// own keys (Local), the keys that work everywhere (Global) and panel
// movement (Navigation). A panel with no keys of its own has no Local
// section.
func (m model) helpSections() []helppopup.Section {
	sections := make([]helppopup.Section, 0, 3)
	if local := m.helpLocalBindings(); len(local) > 0 {
		sections = append(sections, helppopup.Section{Title: "Local", Bindings: local})
	}
	return append(sections,
		helppopup.Section{Title: "Global", Bindings: helpGlobalBindings},
		helppopup.Section{Title: "Navigation", Bindings: helpNavigationBindings},
	)
}

func (m model) helpLocalBindings() []statusbar.Binding {
	switch m.focus {
	case panelStickies:
		return helpStickiesBindings
	case panelPARA:
		return helpParaBindings
	case panelRelated:
		return helpRelatedBindings
	case panelContent:
		return helpContentBindings
	case panelInfo:
		return helpInfoBindings
	}
	return nil
}

// updateHelp handles a key while the help popup is open. While the "/" prompt
// is open, keys type into it; otherwise the up and down arrows scroll, "/"
// opens the prompt, n and N step through matches, gg and G go to the top and
// bottom, esc clears the search and then closes, ? closes, and other keys are
// ignored.
func (m model) updateHelp(msg tea.KeyMsg) model {
	if m.helpSearch.Typing {
		return m.helpPrompt(msg)
	}
	pendingG := m.helpPendingG
	m.helpPendingG = false
	switch msg.String() {
	case "g":
		if pendingG { // gg: the top
			m.helpScroll = 0
		} else {
			m.helpPendingG = true
		}
	case "G":
		m.helpScroll = helppopup.MaxScroll(m.helpSections(), m.height)
	case "up":
		m.helpScroll = max(m.helpScroll-1, 0)
	case "down":
		m.helpScroll = min(m.helpScroll+1, helppopup.MaxScroll(m.helpSections(), m.height))
	case "/":
		m.helpSearch.Typing, m.helpSearch.Input = true, ""
	case "n":
		m = m.helpStep(1)
	case "N":
		m = m.helpStep(-1)
	case "esc":
		if m.helpSearch.Active() {
			m.helpSearch = helppopup.NoSearch
			break
		}
		m = m.closeHelp()
	case "?":
		m = m.closeHelp()
	}
	return m
}

// closeHelp hides the popup and forgets its scroll and search.
func (m model) closeHelp() model {
	m.showingHelp, m.helpScroll, m.helpSearch, m.helpPendingG = false, 0, helppopup.NoSearch, false
	return m
}

// helpPrompt handles a key in the popup's "/" prompt: enter finds the first
// match at or after the top of the view, esc cancels and clears the search,
// and text and backspace edit the query.
func (m model) helpPrompt(msg tea.KeyMsg) model {
	switch msg.Type {
	case tea.KeyEsc:
		m.helpSearch = helppopup.NoSearch
	case tea.KeyEnter:
		m = m.helpFind()
	case tea.KeyBackspace:
		if r := []rune(m.helpSearch.Input); len(r) > 0 {
			m.helpSearch.Input = string(r[:len(r)-1])
		}
	case tea.KeyRunes, tea.KeySpace:
		m.helpSearch.Input += string(msg.Runes)
	}
	return m
}

// helpFind closes the prompt and jumps to the first match of what was typed.
// An empty or unmatched query leaves the previous search as it was.
func (m model) helpFind() model {
	query := strings.TrimSpace(m.helpSearch.Input)
	m.helpSearch.Typing, m.helpSearch.Input = false, ""
	line := helppopup.Step(helppopup.Matches(m.helpSections(), query), m.helpScroll, 0)
	if query == "" || line < 0 {
		return m
	}
	m.helpSearch.Query = query
	return m.helpShow(line)
}

// helpStep moves to the next (dir 1) or previous (dir -1) match of the
// active query, wrapping; it does nothing without a query.
func (m model) helpStep(dir int) model {
	if !m.helpSearch.Active() {
		return m
	}
	from := m.helpSearch.Current
	if from < 0 {
		from = m.helpScroll
	}
	line := helppopup.Step(helppopup.Matches(m.helpSections(), m.helpSearch.Query), from, dir)
	if line < 0 {
		return m
	}
	return m.helpShow(line)
}

// helpShow makes line the current match and scrolls to it when it is out of
// view.
func (m model) helpShow(line int) model {
	m.helpSearch.Current = line
	visible := helppopup.VisibleRows(m.height)
	if line < m.helpScroll || line >= m.helpScroll+visible {
		m.helpScroll = min(line, helppopup.MaxScroll(m.helpSections(), m.height))
	}
	return m
}
