package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/tkilb/para/internal/config"
	"github.com/tkilb/para/internal/state"
	"github.com/tkilb/para/internal/store"
	"github.com/tkilb/para/internal/ui/content"
	"github.com/tkilb/para/internal/ui/info"
	"github.com/tkilb/para/internal/ui/para"
	"github.com/tkilb/para/internal/ui/picker"
	"github.com/tkilb/para/internal/ui/popup"
	"github.com/tkilb/para/internal/ui/related"
	"github.com/tkilb/para/internal/ui/stickies"
	"github.com/tkilb/para/internal/ui/title"
	"github.com/tkilb/para/internal/undo"
)

// openNotebookTitle is the title of the notebook picker.
const openNotebookTitle = "Notebook"

// unavailableColor dims a notebook whose folder is missing.
const unavailableColor = lipgloss.Color("245")

// withNotebooks records the configured notebooks, with the one at active
// open. Its old daily notes are purged by the caller already.
func (m model) withNotebooks(notebooks []config.Notebook, active int, retentionDays int) model {
	m.notebooks, m.activeNotebook, m.retentionDays = notebooks, active, retentionDays
	if active >= 0 && active < len(notebooks) {
		m.purged[notebooks[active].Path] = true
	}
	return m
}

// notebookName is the name of the open notebook, shown in the status bar.
func (m model) notebookName() string {
	if m.activeNotebook < 0 || m.activeNotebook >= len(m.notebooks) {
		return ""
	}
	return m.notebooks[m.activeNotebook].Name
}

// openNotebookPicker lists the configured notebooks, the highlight on the
// open one. Each row gives the notebook's path and how many notes it holds,
// and says when it is the open one or its folder is missing.
func (m model) openNotebookPicker() (model, tea.Cmd) {
	items := make([]picker.Item, len(m.notebooks))
	for i, nb := range m.notebooks {
		var parts []string
		item := picker.Item{Label: nb.Name, Value: strconv.Itoa(i)}
		if nb.Available {
			n := store.CountNotes(nb.Path)
			parts = append(parts, fmt.Sprintf("%d %s", n, plural(n)))
		} else {
			parts = append(parts, "unavailable")
			item.Color = unavailableColor
		}
		parts = append(parts, shortPath(nb.Path))
		if i == m.activeNotebook {
			parts = append([]string{"open"}, parts...)
		}
		item.Detail = strings.Join(parts, " · ")
		items[i] = item
	}
	p, cmd := picker.New(openNotebookTitle, items)
	p = p.WithCursor(m.activeNotebook)
	m.picker, m.pickerOp = &p, pickNotebook
	return m, cmd
}

// chooseNotebook acts on the notebook picked in the picker.
func (m model) chooseNotebook(value string) (model, tea.Cmd) {
	i, err := strconv.Atoi(value)
	if err != nil || i < 0 || i >= len(m.notebooks) || i == m.activeNotebook {
		return m, nil
	}
	return m.switchNotebook(i).forceSync()
}

// shortPath shows the home directory as "~".
func shortPath(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if p == home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(p, home+string(filepath.Separator)); ok {
		return filepath.Join("~", rest)
	}
	return p
}

// switchNotebook opens the notebook at index i of the config. The position in
// the one being left is saved first, and the one being opened is restored to
// where it was. Everything tied to the old notebook starts over: the undo
// history (its actions name files in the old one), the link index, what Content
// remembers and the yank buffer. The first time a notebook is opened in a
// session, its archived daily notes past the retention period are removed.
func (m model) switchNotebook(i int) model {
	if i < 0 || i >= len(m.notebooks) {
		return m
	}
	nb := m.notebooks[i]
	if st, err := os.Stat(nb.Path); err != nil || !st.IsDir() {
		return m.notify(popup.Warning, "That notebook's folder does not exist, so it cannot be opened.")
	}
	m = m.saveOrWarn()

	m.store = store.NewMarkdown(nb.Path)
	var purged int
	var purgeErr error
	if !m.purged[nb.Path] {
		m.purged[nb.Path] = true
		purged, purgeErr = m.store.PurgeDailies(m.retentionDays, time.Now())
	}
	m.history, m.links, m.shown = &undo.Stack{}, nil, shownNote{}
	m.stickyUIDs, m.markUIDs, m.stickies = nil, nil, stickies.New()
	m.content, m.info, m.related, m.title = content.New(), info.New(), related.New(), title.New()
	m.para = para.New(m.store).WithRetention(m.retentionDays, time.Now)
	m.session.notebook = nb.Path
	m = m.loadStickies().loadMarks().restorePosition().resizePanels()
	m.activeNotebook = i
	m.gitSync = nil
	m.focus = panelPARA
	return m.notifyPurge(purged, purgeErr).refreshLinks()
}

// restorePosition puts PARA back where it was in the open notebook.
func (m model) restorePosition() model {
	if m.session.db == nil {
		return m
	}
	nb, err := m.session.db.LoadNotebook(m.session.notebook)
	if err != nil {
		return m.notify(popup.Warning, fmt.Sprintf("Could not restore your last position: %s.", err))
	}
	m.para = m.para.Restore(nb)
	return m
}

// lastNotebook returns the path of the notebook used last, or "" when there
// is none or the saved state cannot be read.
func lastNotebook() string {
	dir, err := state.Dir()
	if err != nil {
		return ""
	}
	db, err := state.Open(dir)
	if err != nil {
		return ""
	}
	defer db.Close()
	sess, err := db.LoadSession()
	if err != nil {
		return ""
	}
	return sess.Notebook
}
