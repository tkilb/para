package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/tkilb/para/internal/attach"
	"github.com/tkilb/para/internal/digest"
	"github.com/tkilb/para/internal/editor"
	"github.com/tkilb/para/internal/loader"
	"github.com/tkilb/para/internal/skills"
	"github.com/tkilb/para/internal/store"
	"github.com/tkilb/para/internal/textsafe"
	"github.com/tkilb/para/internal/ui/para"
	"github.com/tkilb/para/internal/ui/picker"
	"github.com/tkilb/para/internal/ui/popup"
	skillsui "github.com/tkilb/para/internal/ui/skills"
)

const (
	skillsPickerTitle = "Skills"
)

type skillNoteDoneMsg struct {
	store  store.Store
	result skills.NoteResult
	err    error
}

type skillInteractiveDoneMsg struct {
	err error
}

type skillEditDoneMsg struct {
	path string
	err  error
}

// summonSkills opens a fuzzy picker to select a skill to execute.
func (m model) summonSkills() (model, tea.Cmd) {
	loaded, err := skills.List(m.notebookPath())
	if err != nil {
		return m.notify(popup.Warning, capitalize(fmt.Sprintf("could not list skills: %s.", err))), nil
	}
	if len(loaded) == 0 {
		return m.notify(popup.Info, "No skills in .skills/ (focus [1] and press a to add one)."), nil
	}

	items := make([]picker.Item, len(loaded))
	for i, s := range loaded {
		detail := fmt.Sprintf("[%s]", s.Mode)
		if s.Description != "" {
			detail += " " + textsafe.SingleLine(s.Description)
		}
		items[i] = picker.Item{
			Label:  textsafe.SingleLine(s.Name),
			Detail: detail,
			Value:  s.Filename,
		}
	}

	p, cmd := picker.New(skillsPickerTitle, items)
	m.picker, m.pickerOp = &p, pickSkill
	return m, cmd
}

// onSkillChosen handles selecting a skill (by filename) from the picker.
func (m model) onSkillChosen(filename string) (model, tea.Cmd) {
	s, err := skills.Load(m.notebookPath(), filename)
	if errors.Is(err, skills.ErrEmptyBody) {
		return m.notify(popup.Warning, "That skill has no instructions yet. Focus [1] and press e to edit it."), nil
	}
	if err != nil {
		return m.notify(popup.Error, capitalize(fmt.Sprintf("could not load skill: %s.", err))), nil
	}
	return m.executeSkill(s)
}

// executeSkill dispatches the skill according to its mode.
func (m model) executeSkill(s skills.Skill) (model, tea.Cmd) {
	if strings.TrimSpace(s.Body) == "" {
		return m.notify(popup.Warning, "That skill has no instructions yet. Press e to edit it."), nil
	}
	switch s.Mode {
	case skills.ModeImport:
		return m.openImportPromptForSkill(s)
	case skills.ModeInteractive:
		return m.runInteractiveSkill(s)
	case skills.ModeNote:
		return m.runNoteSkill(s)
	default:
		return m.runNoteSkill(s)
	}
}

// runInteractiveSkill launches claude/agy interactively with prompt & note pre-fed.
func (m model) runInteractiveSkill(s skills.Skill) (model, tea.Cmd) {
	if m.agent == skills.AgentNone || m.agentPath == "" {
		return m.notify(popup.Warning, "Neither claude nor agy CLI was found on PATH."), nil
	}

	var noteRel, noteContent string
	if entry, ok := m.para.SelectedNote(); ok {
		noteRel = entry.Rel
		if p, err := m.store.Path(entry.Rel); err == nil {
			if data, err := os.ReadFile(p); err == nil {
				noteContent = string(data)
			}
		}
	}

	prompt := skills.BuildInteractivePrompt(s, noteRel, noteContent)
	runner := skills.Runner{Agent: m.agent, BinPath: m.agentPath}
	cmd := runner.InteractiveCmd(m.notebookPath(), prompt)

	m = m.saveOrWarn()
	return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
		return skillInteractiveDoneMsg{err: err}
	})
}

// finishSkillInteractive handles terminal handover return.
func (m model) finishSkillInteractive(msg skillInteractiveDoneMsg) (model, tea.Cmd) {
	m.para = m.para.Reload()
	m = m.refreshLinks()
	m.skills = m.skills.Reload()
	m.focus = panelPARA
	if msg.err != nil {
		m = m.notify(popup.Error, fmt.Sprintf("Agent interactive session exited with error: %s.", msg.err))
	}
	m, syncCmd := m.forceSync()
	return m, tea.Batch(tea.ClearScreen, syncCmd)
}

// runNoteSkill executes the skill non-interactively and saves a timestamped note.
func (m model) runNoteSkill(s skills.Skill) (model, tea.Cmd) {
	if m.agent == skills.AgentNone || m.agentPath == "" {
		return m.notify(popup.Warning, "Neither claude nor agy CLI was found on PATH."), nil
	}

	entry, ok := m.para.SelectedNote()
	if !ok {
		return m.notify(popup.Info, "Select a note to run this skill against."), nil
	}

	notePath, err := m.store.Path(entry.Rel)
	if err != nil {
		return m.notify(popup.Error, fmt.Sprintf("Could not find note: %s.", err)), nil
	}
	data, err := os.ReadFile(notePath)
	if err != nil {
		return m.notify(popup.Error, fmt.Sprintf("Could not read note: %s.", err)), nil
	}

	m.skillRunning = true
	m.skillLabel = fmt.Sprintf("Running %s…", s.Name)

	st := m.store
	nbRoot := m.notebookPath()
	runner := skills.Runner{Agent: m.agent, BinPath: m.agentPath}
	relPath := entry.Rel
	content := string(data)

	return m, func() tea.Msg {
		res, err := runner.RunNoteInStore(context.Background(), st, nbRoot, relPath, content, s, time.Now())
		return skillNoteDoneMsg{store: st, result: res, err: err}
	}
}

// finishSkillNote handles completion of non-interactive skill execution.
func (m model) finishSkillNote(msg skillNoteDoneMsg) (model, tea.Cmd) {
	m.skillRunning = false
	m.skillLabel = ""

	if msg.result.Action.Undo != nil {
		m.history.Push(msg.result.Action)
	}
	if msg.err != nil {
		return m.notify(popup.Error, fmt.Sprintf("Skill failed: %s.", msg.err)), nil
	}

	if msg.store == m.store {
		m.para = m.para.Reload()
		m = m.refreshLinks()
		if p, ok := m.para.Reveal(msg.result.Rel); ok {
			m.para = p
		}
		m.focus = panelPARA
		m.syncDirty = true
	}

	return m.notify(popup.Info, fmt.Sprintf("Created note %s.", filepath.Base(msg.result.Rel))), nil
}

// openImportPromptForSkill prompts for input source to import using a custom skill prompt.
func (m model) openImportPromptForSkill(s skills.Skill) (model, tea.Cmd) {
	if m.importing > 0 {
		return m.notify(popup.Info, "An import is already running."), nil
	}
	inbox, _ := m.store.Path("inbox")
	label := fmt.Sprintf("Import with %s · Blank: clipboard · URL: web page · else: file", s.Name)
	in, cmd := m.openInput(&para.Prompt{Title: "Import with " + s.Name, Label: label})
	clipImage, hasClipImage := attach.ProbeClipboard(context.Background(), m.imageClip)
	in.hint = func(text string) string {
		if hasClipImage && loader.Classify(text).Kind == loader.Clipboard {
			return clipImage
		}
		if hint, ok := attach.ProbeFile(text, inbox); ok {
			return hint
		}
		return loader.Describe(text, inbox, clipboard.ReadAll)
	}
	in.complete = func(text string) (string, []string) { return loader.Complete(text, inbox) }
	in.accept = func(m model, text string) (model, tea.Cmd) {
		return m.startImportWithSkillPrompt(text, inbox, s.Body, s.Name)
	}
	in.refresh()
	m.input = in
	return m, cmd
}

func (m model) startImportWithSkillPrompt(text, inbox, customPrompt, skillName string) (model, tea.Cmd) {
	if m.importing > 0 {
		return m.notify(popup.Info, "An import is already running."), nil
	}
	if res, handled, err := attach.Import(context.Background(), m.store, m.imageClip, m.para.Dir(), text, inbox, time.Now()); handled {
		return m.finishImage(res, err), nil
	}
	if !m.digest.Enabled {
		return m.notify(popup.Info, importOffNotice), nil
	}
	in := loader.Classify(text)
	fetcher := m.fetcher
	load := func(ctx context.Context) (loader.Source, error) {
		switch in.Kind {
		case loader.Clipboard:
			return loader.ReadClipboard(clipboard.ReadAll)
		case loader.Web:
			return loader.ReadURL(ctx, fetcher, in.Value)
		}
		return loader.ReadFile(loader.ResolveFile(in.Value, inbox))
	}
	return m.runImportWithCustomPrompt("Importing ("+skillName+")…", customPrompt, func(ctx context.Context, dg digest.Digester, st store.Store, kind store.Kind) (digest.Result, error, bool) {
		src, err := load(ctx)
		if err != nil {
			return digest.Result{}, err, true
		}
		res, err := digest.Import(ctx, st, time.Now(), kind, dg, src)
		return res, err, false
	})
}

// skillsKey handles keys when the [1] Skills panel is focused.
func (m model) skillsKey(msg tea.KeyMsg) (model, tea.Cmd) {
	var eff skillsui.Effect
	m.skills, eff = m.skills.Update(msg)
	return m.applySkillsEffect(eff)
}

func (m model) applySkillsEffect(eff skillsui.Effect) (model, tea.Cmd) {
	if eff.Action != nil {
		m.history.Push(*eff.Action)
		m.syncDirty = true
	}
	if eff.Info != "" {
		m = m.notify(popup.Info, eff.Info)
	}
	if eff.Warn != "" {
		m = m.notify(popup.Warning, eff.Warn)
	}
	if eff.Err != nil {
		m = m.notify(popup.Error, capitalize(eff.Err.Error())+".")
	}
	if eff.Leave {
		m.focus = panelPARA
	}
	if eff.CopyPath != "" {
		if err := m.writeClipboard(eff.CopyPath); err != nil {
			m = m.notify(popup.Error, capitalize(fmt.Sprintf(
				"could not copy to the clipboard: %s (it needs pbcopy on macOS, or xclip, xsel or wl-copy on Linux).", err)))
		} else {
			m = m.notify(popup.Info, "Copied "+eff.CopyPath)
		}
	}
	if eff.Execute != nil {
		return m.executeSkill(*eff.Execute)
	}
	if eff.EditPath != "" {
		return m.openSkillEditor(eff.EditPath)
	}
	if eff.Prompt != nil {
		return m.openSkillsInput(eff.Prompt)
	}
	if eff.Confirm != nil {
		op := eff.Confirm.Op
		m = m.askDanger(eff.Confirm.Prompt, eff.Confirm.Danger, func(m model) (model, tea.Cmd) {
			var nextEff skillsui.Effect
			m.skills, nextEff = m.skills.ApplyConfirm(op)
			return m.applySkillsEffect(nextEff)
		})
	}
	return m, nil
}

func (m model) openSkillsInput(p *skillsui.Prompt) (model, tea.Cmd) {
	in, cmd := m.openInput(&para.Prompt{
		Title:       p.Title,
		Label:       p.Label,
		Initial:     p.Initial,
		Placeholder: p.Placeholder,
		Inline:      p.Inline,
	})
	op := p.Op
	cancelOp := p.CancelOp
	in.accept = func(m model, text string) (model, tea.Cmd) {
		var eff skillsui.Effect
		m.skills, eff = m.skills.ApplyInput(op, text)
		return m.applySkillsEffect(eff)
	}
	if cancelOp != 0 {
		in.cancel = func(m model) (model, tea.Cmd) {
			var eff skillsui.Effect
			m.skills, eff = m.skills.ApplyInput(cancelOp, "")
			return m.applySkillsEffect(eff)
		}
	}
	m.input = in
	return m, cmd
}

func (m model) openSkillEditor(path string) (model, tea.Cmd) {
	cmd, err := editor.Command(path, os.Environ())
	if err != nil {
		return m.notify(popup.Error, fmt.Sprintf("Could not open editor: %s.", err)), nil
	}
	m = m.saveOrWarn()
	return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
		return skillEditDoneMsg{path: path, err: err}
	})
}

func (m model) finishSkillEdit(msg skillEditDoneMsg) (model, tea.Cmd) {
	m.skills = m.skills.Reload()
	m.syncDirty = true
	if msg.err != nil {
		m = m.notify(popup.Error, fmt.Sprintf("Editor exited with error: %s.", msg.err))
	}
	return m, tea.ClearScreen
}
