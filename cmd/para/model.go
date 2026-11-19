package main

import (
	"os"
	"strings"
	"time"

	"github.com/atotto/clipboard"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/tkilb/para/internal/attach"
	"github.com/tkilb/para/internal/browser"
	"github.com/tkilb/para/internal/clip"
	"github.com/tkilb/para/internal/config"
	"github.com/tkilb/para/internal/gitsync"
	"github.com/tkilb/para/internal/glossary"
	"github.com/tkilb/para/internal/imgpreview"
	"github.com/tkilb/para/internal/links"
	"github.com/tkilb/para/internal/store"
	"github.com/tkilb/para/internal/ui/clipform"
	"github.com/tkilb/para/internal/ui/content"
	"github.com/tkilb/para/internal/ui/grep"
	"github.com/tkilb/para/internal/ui/helppopup"
	"github.com/tkilb/para/internal/ui/info"
	"github.com/tkilb/para/internal/ui/noteform"
	"github.com/tkilb/para/internal/ui/panel"
	"github.com/tkilb/para/internal/ui/para"
	"github.com/tkilb/para/internal/ui/picker"
	"github.com/tkilb/para/internal/ui/popup"
	"github.com/tkilb/para/internal/ui/related"
	"github.com/tkilb/para/internal/ui/statusbar"
	"github.com/tkilb/para/internal/ui/title"
	"github.com/tkilb/para/internal/undo"
)

// panelID identifies a panel. Its value is the number key that focuses it,
// so Content is 0 and the empty [1] pane is 1, matching the titles in the grid.
type panelID int

const (
	panelContent panelID = iota
	panelBlank
	panelPARA
	panelRelated
	panelInfo
)

var panelNames = map[panelID]string{
	panelContent: "Content",
	panelBlank:   "[1]",
	panelPARA:    "PARA",
	panelRelated: "Related",
	panelInfo:    "Info",
}

// focusCycle is the Tab/Shift+Tab order: the left column top to bottom,
// then the right column top to bottom. The Title panel is not in it.
var focusCycle = []panelID{panelBlank, panelPARA, panelRelated, panelContent, panelInfo}

// isPanel reports whether n is the number of a focusable panel.
func isPanel(n int) bool {
	for _, p := range focusCycle {
		if int(p) == n {
			return true
		}
	}
	return false
}

func (p panelID) next() panelID { return focusCycle[(p.cycleIndex()+1)%len(focusCycle)] }

func (p panelID) prev() panelID {
	return focusCycle[(p.cycleIndex()-1+len(focusCycle))%len(focusCycle)]
}

func (p panelID) cycleIndex() int {
	for i, c := range focusCycle {
		if c == p {
			return i
		}
	}
	return 0
}

// globalBindings are always shown in the status bar, after any bindings of
// the focused panel.
var globalBindings = []statusbar.Binding{
	{Key: "0-4", Label: "focus panel"},
	{Key: "s", Label: "scratch"},
	{Key: "M", Label: "marks"},
	{Key: "B", Label: "notebook"},
	{Key: "ctrl+f", Label: "find note"},
	{Key: "ctrl+y", Label: "glossary"},
	{Key: "tab", Label: "next panel"},
	{Key: "+", Label: "zoom"},
	{Key: "u", Label: "undo"},
	{Key: "?", Label: "help"},
	{Key: "ctrl+g", Label: "lazygit"},
	{Key: "ctrl+l", Label: "refresh"},
	{Key: "ctrl+z", Label: "suspend"},
	{Key: "q", Label: "quit"},
}

var paraBindings = []statusbar.Binding{
	{Key: "↑/↓", Label: "move"},
	{Key: "←/→", Label: "up/open"},
	{Key: "[ ]", Label: "tab"},
	{Key: "space", Label: "select"},
	{Key: "y/x/p", Label: "yank/cut/paste"},
	{Key: "d/D", Label: "trash/delete"},
	{Key: "a", Label: "new"},
	{Key: "r", Label: "rename"},
	{Key: "e", Label: "edit"},
	{Key: "mm", Label: "mark"},
	{Key: "/", Label: "search"},
	{Key: "n/N", Label: "next/prev match"},
	{Key: "v", Label: "visual"},
	{Key: ".", Label: "hidden"},
	{Key: "enter", Label: "open"},
}

var relatedBindings = []statusbar.Binding{
	{Key: "↑/↓", Label: "move"},
	{Key: "l", Label: "add reference"},
	{Key: "d", Label: "remove reference"},
	{Key: "enter", Label: "jump to note"},
	{Key: "esc/←", Label: "back to PARA"},
}

var infoBindings = []statusbar.Binding{
	{Key: "↑/↓", Label: "scroll"},
	{Key: "esc/←", Label: "back to PARA"},
}

var contentBindings = []statusbar.Binding{
	{Key: "↑/↓", Label: "move"},
	{Key: "ctrl+d/u", Label: "half page"},
	{Key: "{ }", Label: "paragraph"},
	{Key: "space", Label: "pick line"},
	{Key: "v", Label: "select lines"},
	{Key: "m", Label: "rendered/plain"},
	{Key: "y", Label: "yank"},
	{Key: "r", Label: "rename"},
	{Key: "e", Label: "edit"},
	{Key: "enter", Label: "extract"},
	{Key: "esc/←", Label: "back to PARA"},
}

type model struct {
	// imageClip reads an image from the system clipboard, and imageProto is
	// how the terminal draws an image.
	imageClip  attach.Source
	imageProto imgpreview.Protocol
	// bundle holds the file names of the images bundled with the note shown,
	// and bundleTab says which Content tab is open: 0 is the note and i is
	// bundle[i-1].
	bundle    []string
	bundleTab int

	store   store.Store
	focus   panelID
	para    para.Model
	content content.Model
	info    info.Model
	related related.Model
	// markArmed is set by a first m in PARA: a second m, as the very next
	// key, marks the note; any other key clears it.
	markArmed bool
	// markUIDs are the uids of the notes marked with a bullet in PARA.
	markUIDs []string
	// notebooks are the configured notebooks and activeNotebook the index of
	// the open one; purged holds the notebook roots whose old dailies were
	// removed this session, and retentionDays is the daily retention.
	notebooks      []config.Notebook
	activeNotebook int
	// gitSync is the open notebook's git state (see gitsync.Status); nil when
	// it is not in a repository or is not read yet.
	gitSync *gitsync.Status
	// syncGate spaces out the reads that follow changes to the notes, and
	// syncDirty is set when the notes changed since Update last looked.
	syncGate      *gitsync.Gate
	syncDirty     bool
	purged        map[string]bool
	retentionDays int
	title         title.Model
	// links indexes the related properties; nil until it could be built.
	links *links.Index
	shown shownNote
	// shownFront is the shown note's frontmatter lines, fences included, or
	// none when it has no properties block. Content shows them while zoomed.
	shownFront    []string
	width, height int
	// history is shared by pointer: it holds the session's undo/redo stack.
	history *undo.Stack
	// popups queues notifications; the front one blocks input until a key
	// dismisses it.
	popups popup.Model
	// pending is a yes/no question waiting for an answer.
	pending *para.Confirm
	// input is an open one-line text prompt.
	input *input
	// picker is the open fuzzy picker for adding a reference to the note at
	// pickerFor.
	picker *picker.Model
	// grep is the open text search.
	grep      *grep.Model
	pickerFor string
	pickerOp  pickerOp
	// pickerKind is the kind the note had when the kind picker opened.
	pickerKind store.Kind
	// extract is the extraction in progress, if any.
	extract *extractState
	// fetcher gets page titles for URL clips; clipping counts the fetches
	// still running.
	fetcher *clip.Fetcher
	// openURL hands a web address to the browser.
	openURL  func(string) error
	clipping int
	// writeClipboard copies text to the system clipboard (a yank).
	writeClipboard func(string) error
	// sources are the sources the Add Note form offers, from the config.
	sources []string
	// templatesDir is the folder of note templates (see package templates).
	templatesDir string
	// digest is the agentic import configuration; importing counts the
	// imports running (at most one) and importLabel says what the status bar
	// shows while it runs. status is a message that clears itself, and
	// statusID tells a stale clearing timer from the current one.
	digest      config.Digest
	importing   int
	importLabel string
	status      string
	statusID    int
	// clipForm is the open web clip form.
	clipForm *clipform.Model
	// form is the open quick-capture form; formSave handles its texts.
	form     *noteform.Model
	formSave func(model, noteform.Values) (model, tea.Cmd)
	// question is a yes/no question owned by the root model, with what to do
	// on yes (the para panel's own questions use pending instead).
	question *question
	// session saves and restores the position between runs.
	session session
	// showingHelp is set while the help popup is open, scrolled by helpScroll.
	showingHelp bool
	helpScroll  int
	// helpSearch is the "/" search inside the help popup.
	helpSearch helppopup.Search
	// zoomed is set while only the focused panel is drawn, at full size.
	zoomed bool
	// helpPendingG is set after one g in the popup, waiting for the second.
	helpPendingG bool
	// saveErr is a failure to save on quit, reported after the screen closes.
	saveErr error
}

// shownNote identifies what the Content and Info panels last loaded. A changed
// modification time means the file changed on disk, and a changed item
// count means a folder listed differently (for example after toggling
// hidden files).
type shownNote struct {
	rel         string
	mod         time.Time
	items       int
	attachments int
}

func initialModel(s store.Store) model {
	return model{
		imageClip: attach.NewClipboard(), imageProto: imgpreview.Detect(os.Getenv),
		store: s, focus: panelPARA, para: para.New(s),
		content: content.New(), info: info.New(), related: related.New(), title: title.New(), history: &undo.Stack{}, purged: map[string]bool{},
		syncGate: gitsync.NewGate(syncDebounce), helpSearch: helppopup.NoSearch, sources: config.DefaultSources, writeClipboard: clipboard.WriteAll, fetcher: clip.NewFetcher(), openURL: browser.Open,
	}
}

// syncSelection makes Content and Info show what is under the PARA cursor,
// reading the note once, and only when the selection or the file changed.
// A folder shows its name and item count in Info and its contents in Content.
func (m model) syncSelection() model {
	return m.loadSelection().syncTabs()
}

// loadSelection is syncSelection without the tabs. A different selection
// forgets the bundle tab; the note branch sets the bundle again.
func (m model) loadSelection() model {
	entry, ok := m.para.Selected()
	if !ok {
		m.content, m.info, m.shown, m.shownFront = m.content.Clear(), m.info.Clear(), shownNote{}, nil
		m.bundle, m.bundleTab = nil, 0
		m.related = m.related.Clear()
		m.title = m.title.Set("")
		return m
	}
	now := shownNote{rel: entry.Rel, mod: entry.ModTime, items: entry.ItemCount, attachments: entry.Attachments}
	if now == m.shown {
		return m
	}
	m.shown, m.shownFront = now, nil
	m.bundle, m.bundleTab = nil, 0

	if entry.IsDir {
		m.title = m.title.SetDir(entry.Name)
		m.content = m.showDirContent(entry)
		m.related = m.related.Clear()
		m.info = m.info.ShowDir(entry.Rel, entry.Name, entry.ItemCount)
		return m
	}
	if entry.IsImage {
		return m.showImage(entry)
	}
	m.related = m.related.Show(entry.Rel, m.links.References(entry.Rel), m.links.ReferencedBy(entry.Rel))
	m = m.conformNote(entry)
	note, err := m.store.ReadNote(entry.Rel)
	if err != nil {
		m.title = m.title.Set(entry.Name)
		m.content = m.content.Fail(entry.Rel, err)
		m.info = m.info.ShowError(entry.Rel, err)
		return m
	}
	m.title = m.title.SetNote(note.Kind, note.Title)
	m.shownFront = frontLines(note)
	m.content = m.content.LoadNote(entry.Rel, m.wantFront(), note.Body)
	m.info = m.info.ShowNote(entry.Rel, note)
	m.bundle = m.bundledImages(entry)
	return m
}

// conformNote makes the note being viewed standard on disk (see
// store.Markdown.Conform): viewing a note is what brings it in line, so every
// note you look at or open ends up in the standard order with the standard
// properties. A note that is standard already is not written, and one whose
// properties cannot be read is left alone.
func (m model) conformNote(entry store.Entry) model {
	if _, err := m.store.Conform(entry.Rel); err != nil {
		return m.notify(popup.Warning, "Could not make "+strings.TrimSuffix(entry.Name, ".md")+" follow the property standard: "+err.Error()+".")
	}
	return m
}

// frontLines is the note's properties block as written, fences included, or
// nil when it has none.
func frontLines(n store.Note) []string {
	if !n.HasFrontmatter {
		return nil
	}
	lines := []string{"---"}
	if n.RawFrontmatter != "" {
		lines = append(lines, strings.Split(n.RawFrontmatter, "\n")...)
	}
	return append(lines, "---")
}

// wantFront is the frontmatter Content should show: the shown note's while Content
// is zoomed, since Title and Info are out of sight then, else none.
func (m model) wantFront() []string {
	if m.zoomed && m.focus == panelContent {
		return m.shownFront
	}
	return nil
}

// showDirContent fills Content with the contents of the folder entry, or clears it
// when the folder is empty or cannot be read.
func (m model) showDirContent(entry store.Entry) content.Model {
	entries, err := m.store.List(entry.Rel, m.para.ListOptions())
	if err != nil || len(entries) == 0 {
		return m.content.Clear()
	}
	return m.content.LoadListing(entry.Rel, m.dirItems(entries))
}

func (m model) Init() tea.Cmd {
	_, cmd := m.forceSync()
	return cmd
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.update(msg)
	after := next.(model)
	if after.width > 0 {
		after = after.resizePanels()
	}
	after = after.syncSelection()
	after.content = after.content.SetFront(after.wantFront())
	if after.syncDirty {
		after.syncDirty = false
		cmd = tea.Batch(cmd, after.requestSync())
	}
	return after, cmd
}

// grid is the panel layout for the current size. While zoomed, the focused
// panel takes the whole grid.
func (m model) grid() grid {
	g := computeGrid(m.width, m.height-1)
	if m.zoomed {
		return g.zoomed(m.focus, m.width, m.height-1)
	}
	return g
}

// stepFocus moves the focus with step, skipping panels the compact layout
// hides.
func (m model) stepFocus(step func(panelID) panelID) panelID {
	g := m.grid()
	p := step(m.focus)
	for i := 0; i < len(focusCycle) && !g.shows(p); i++ {
		p = step(p)
	}
	return p
}

// resizePanels gives PARA, Content and Info the size the grid leaves them.
// A focus the compact layout hides moves to PARA.
func (m model) resizePanels() model {
	g := m.grid()
	if !g.shows(m.focus) {
		m.focus = panelPARA
	}
	_, rows := panel.InnerSize(g.leftWidth, g.paraHeight)
	m.para = m.para.Resize(rows)
	bw, bh := panel.InnerSize(g.rightWidth, g.contentHeight)
	m.content = m.content.Resize(bw, bh)
	iw, ih := panel.InnerSize(g.rightWidth, g.infoHeight)
	m.info = m.info.Resize(iw, ih)
	return m
}

func (m model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		if m.form != nil {
			f := m.form.Resize(m.width, m.height)
			m.form = &f
		}
		if m.clipForm != nil {
			f := m.clipForm.Resize(m.width, m.height)
			m.clipForm = &f
		}
		m = m.resizePanels()
	case tea.FocusMsg:
		// Files may have changed while the terminal was in the background.
		// A pending question was worded for the rows on screen, so it waits.
		if m.pending == nil {
			m.para = m.para.Reload()
			m = m.refreshLinks()
		}
		return m, nil
	case syncDueMsg:
		return m, m.finishDue()
	case syncReadMsg:
		return m.finishSync(msg), nil
	case clipFetchedMsg:
		return m.finishClip(msg)
	case importDoneMsg:
		return m.finishImport(msg)
	case statusClearMsg:
		if msg.id == m.statusID {
			m.status = ""
		}
		return m, nil
	case editDoneMsg:
		return m.finishEdit(msg), tea.ClearScreen
	case glossaryEditDoneMsg:
		return m.finishGlossaryEdit(msg)
	case scratchDoneMsg:
		return m.finishScratch(msg), tea.ClearScreen
	case lazygitDoneMsg:
		return m.finishLazygit(msg)
	case tea.ResumeMsg:
		// Returning from ctrl+z: repaint everything on the alternate screen.
		return m, tea.ClearScreen
	case tea.KeyMsg:
		return m.updateKey(msg)
	case tea.MouseMsg:
		return m.updateMouse(msg)
	default:
		// Cursor blink and similar messages belong to the open prompt.
		if m.picker != nil {
			p, cmd, _ := m.picker.Update(msg)
			m.picker = &p
			return m, cmd
		}
		if m.grep != nil {
			g, cmd, _ := m.grep.Update(msg)
			m.grep = &g
			return m, cmd
		}
		if m.form != nil {
			f, cmd := m.form.Blink(msg)
			m.form = &f
			return m, cmd
		}
		if m.clipForm != nil {
			f, cmd := m.clipForm.Blink(msg)
			m.clipForm = &f
			return m, cmd
		}
		if m.input != nil {
			var cmd tea.Cmd
			m.input.field, cmd = m.input.field.Update(msg)
			return m, cmd
		}
	}
	return m, nil
}

func (m model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key != "ctrl+c" {
		switch {
		case m.popups.Active():
			m.popups = m.popups.Dismiss()
			return m, nil
		case m.showingHelp:
			return m.updateHelp(msg), nil
		case m.question != nil && m.form != nil:
			return m.answerQuestion(key)
		case m.form != nil:
			return m.formKey(msg)
		case m.clipForm != nil:
			return m.clipFormKey(msg)
		case m.question != nil:
			return m.answerQuestion(key)
		case m.picker != nil:
			return m.pickerKey(msg)
		case m.grep != nil:
			return m.grepKey(msg)
		case m.pending != nil:
			return m.answer(key)
		case m.input != nil:
			return m.typeKey(msg)
		}
	}
	armed := m.markArmed
	m.markArmed = false
	if key == "m" && m.focus == panelPARA && !armed {
		m.markArmed = true // the second m reaches PARA, which marks the note
		return m, nil
	}
	var cmd tea.Cmd
	switch key {
	case "q", "ctrl+c":
		m.saveErr = m.save()
		return m, tea.Quit
	case "?":
		m.showingHelp = true
	case "A":
		m, cmd = m.startQuickAdd()
	case "I":
		m, cmd = m.importKey()
	case "B":
		m, cmd = m.openNotebookPicker()
	case "M":
		m, cmd = m.openMarks()
	case "s":
		m, cmd = m.openScratch()
	case "ctrl+f":
		m, cmd = m.openFileFinder()
	case "ctrl+_", "F":
		// ctrl+/ reaches the program as ctrl+_ ; F is for terminals that
		// send nothing useful for it.
		m, cmd = m.openTextFinder()
	case "U":
		m, cmd = m.startClip()
	case "o":
		m = m.openWeb()
	case "ctrl+t":
		m, cmd = m.startAppendTemplate()
	case "ctrl+y":
		m, cmd = m.openGlossary(glossary.Entry{})
	case "b":
		m = m.openDaily(-1)
	case "t":
		m = m.openDaily(0)
	case "T":
		m = m.openDaily(1)
	case "ctrl+g":
		return m.openLazygit()
	case "ctrl+z":
		return m.saveOrWarn(), tea.Suspend
	case "ctrl+l":
		m.para = m.para.Reload()
		m = m.refreshLinks()
		var syncCmd tea.Cmd
		m, syncCmd = m.forceSync()
		cmd = tea.Batch(tea.ClearScreen, syncCmd)
	case "u":
		m = m.runHistory("Undid", "Undo", m.history.Undo)
	case "ctrl+r":
		m = m.runHistory("Redid", "Redo", m.history.Redo)
	case "+":
		m.zoomed = !m.zoomed
	case "tab":
		m.focus = m.stepFocus(panelID.next)
	case "shift+tab":
		m.focus = m.stepFocus(panelID.prev)
	case "0", "1", "2", "3", "4":
		if p := panelID(key[0] - '0'); m.grid().shows(p) {
			m.focus = p
		}
	case "esc", "left":
		switch m.focus {
		case panelContent:
			// Closing a range or clearing the selection comes before leaving
			// the panel. On an image tab, left goes back one tab first, so it
			// leaves only from the note tab; esc always leaves.
			switch {
			case m.content.Active():
				m.content = m.content.Escape()
			case key == "left" && m.bundleTab > 0:
				m = m.openBundleTab(m.bundleTab - 1)
			default:
				m.focus = panelPARA
			}
		case panelInfo, panelRelated, panelBlank:
			m.focus = panelPARA
		case panelPARA:
			m, cmd = m.paraKey(msg)
		}
	case "enter", "right":
		if m.focus == panelRelated {
			m, cmd = m.relatedKey(msg)
			break
		}
		if m.focus == panelContent && m.content.HasSelection() && key == "enter" {
			m, cmd = m.startExtract()
			break
		}
		if m.focus == panelContent && key == "right" && len(m.bundle) > 0 {
			// Right opens the next tab and stops at the last: it never wraps.
			m = m.openBundleTab(m.bundleTab + 1)
			break
		}
		if m.focus != panelPARA {
			break
		}
		if _, ok := m.para.SelectedNote(); ok {
			// Enter goes into the folder of a note's bundled images; right,
			// and enter on a note with none, move to its Content.
			if key == "enter" {
				if p, entered := m.para.EnterBundle(); entered {
					m.para = p
					break
				}
			}
			m.focus = panelContent
		} else {
			m, cmd = m.paraKey(msg)
		}
	default:
		if key == "c" && m.focus == panelPARA {
			m = m.copyPath()
			break
		}
		if key == "y" && m.focus == panelContent {
			m = m.yankContent()
			break
		}
		if (key == "[" || key == "]") && m.focus == panelContent && len(m.bundle) > 0 {
			m = m.stepBundleTab(map[string]int{"[": -1, "]": 1}[key])
			break
		}
		if (key == "r" || key == "e") && m.focus == panelContent {
			// Content shows the note under the PARA cursor, so renaming or
			// editing it is PARA's rename or edit.
			m, cmd = m.paraKey(msg)
			break
		}
		switch m.focus {
		case panelPARA:
			m, cmd = m.paraKey(msg)
		case panelRelated:
			m, cmd = m.relatedKey(msg)
		case panelContent:
			m.content = m.content.Update(msg)
		case panelInfo:
			m.info = m.info.Update(msg)
		}
	}
	return m, cmd
}

// paraKey sends a key to the PARA panel and applies what it asks for.
func (m model) paraKey(msg tea.KeyMsg) (model, tea.Cmd) {
	var eff para.Effect
	m.para, eff = m.para.Update(msg)
	return m.apply(eff)
}

func (m model) bindings() []statusbar.Binding {
	var local []statusbar.Binding
	switch m.focus {
	case panelPARA:
		local = paraBindings
	case panelRelated:
		local = relatedBindings
	case panelContent:
		local = contentBindings
		if len(m.bundle) > 0 {
			local = append([]statusbar.Binding{{Key: "←/→ [ ]", Label: "image tabs"}}, local...)
		}
	case panelInfo:
		local = infoBindings
	}
	return append(append([]statusbar.Binding(nil), local...), globalBindings...)
}

func (m model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}
	g := m.grid() // one row of the height is the status bar
	render := func(id panelID, w, h int) string {
		switch id {
		case panelPARA:
			return m.para.View(w, h, m.focus == id)
		case panelRelated:
			return m.related.View(w, h, m.focus == id)
		case panelContent:
			c := m.content
			if m.overlayOpen() {
				c = c.Covered()
			}
			return c.View(w, h, m.focus == id)
		case panelInfo:
			return m.info.View(w, h, m.focus == id)
		}
		iw, ih := panel.InnerSize(w, h)
		return panel.Frame(panelNames[id], "", iw, ih, m.focus == id)
	}

	if m.zoomed {
		zoomed := render(m.focus, g.leftWidth, m.height-1)
		view := lipgloss.JoinVertical(lipgloss.Left, zoomed, ansi.Truncate(m.statusLine(), m.width, "…"))
		return m.overlayModals(view)
	}

	if g.compact {
		compact := lipgloss.JoinHorizontal(lipgloss.Top,
			render(panelPARA, g.leftWidth, g.paraHeight),
			render(panelContent, g.rightWidth, g.contentHeight),
		)
		view := lipgloss.JoinVertical(lipgloss.Left, compact, ansi.Truncate(m.statusLine(), m.width, "…"))
		return m.overlayModals(view)
	}

	left := lipgloss.JoinVertical(lipgloss.Left,
		render(panelBlank, g.leftWidth, g.blankHeight),
		render(panelPARA, g.leftWidth, g.paraHeight),
		render(panelRelated, g.leftWidth, g.relatedHeight),
	)
	right := lipgloss.JoinVertical(lipgloss.Left,
		m.title.View(g.rightWidth, g.titleHeight),
		render(panelContent, g.rightWidth, g.contentHeight),
		render(panelInfo, g.rightWidth, g.infoHeight),
	)
	grid := lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	view := lipgloss.JoinVertical(lipgloss.Left, grid, ansi.Truncate(m.statusLine(), m.width, "…"))
	return m.overlayModals(view)
}
