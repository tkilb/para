package main

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/tkilb/para/internal/attach"
	"github.com/tkilb/para/internal/config"
	"github.com/tkilb/para/internal/digest"
	"github.com/tkilb/para/internal/loader"
	"github.com/tkilb/para/internal/store"
	"github.com/tkilb/para/internal/ui/para"
	"github.com/tkilb/para/internal/ui/popup"
)

// statusDuration is how long a success message stays in the status bar.
const statusDuration = 5 * time.Second

// importDoneMsg arrives when an import or a re-run ends.
type importDoneMsg struct {
	// store is the notebook it ran in, which may no longer be the open one.
	store  store.Store
	rerun  bool
	result digest.Result
	err    error
	// early is set when it failed before anything was written.
	early bool
}

// statusClearMsg arrives statusDuration after a status message was shown.
type statusClearMsg struct{ id int }

// importKey handles I. On an IMPORT_ folder, or anything inside one, it runs
// that import again at once; anywhere else it asks what to import.
//
// With the agent import off, I still saves an image, which never goes to the
// agent; only text, web pages and re-runs need digest.enabled.
func (m model) importKey() (model, tea.Cmd) {
	if m.importing > 0 {
		return m.notify(popup.Info, "An import is already running."), nil
	}
	if folder, ok := m.cursorImportFolder(); ok {
		if !m.digest.Enabled {
			return m.notify(popup.Info, importOffNotice), nil
		}
		return m.startRerun(folder)
	}
	return m.openImportPrompt()
}

// pasteAttachment handles ctrl+v: it saves an image or PDF from the clipboard
// in the folder PARA shows, by the same logic as a blank import (I). Anything
// else on the clipboard is left alone, because text needs the agent import.
func (m model) pasteAttachment() model {
	inbox, _ := m.store.Path("inbox")
	res, handled, err := attach.Import(context.Background(), m.store, m.imageClip, m.para.Dir(), "", inbox, time.Now())
	if !handled {
		return m.notify(popup.Info, "No image or PDF on the clipboard.")
	}
	return m.finishImage(res, err)
}

// importOffNotice says why a text import did not run.
const importOffNotice = "Import is off. Set digest: enabled: true in the config file to turn it on. Imported content is then sent to the agent command."

// cursorImportFolder is the import folder under the PARA cursor, or the one
// PARA is showing.
func (m model) cursorImportFolder() (string, bool) {
	if entry, ok := m.para.Selected(); ok {
		if folder, found := digest.FindImportFolder(entry.Rel); found {
			return folder, true
		}
	}
	return digest.FindImportFolder(m.para.Dir())
}

// openImportPrompt opens the one box that takes the clipboard (empty), a web
// address or a file path, with a preview line and Tab completion.
func (m model) openImportPrompt() (model, tea.Cmd) {
	inbox, _ := m.store.Path("inbox")
	enabled := m.digest.Enabled
	label := "Blank: clipboard · URL: web page · else: file, or an image (Tab completes)"
	if !enabled {
		label = "Blank: clipboard image or SVG text · else: an image file (text import is off) (Tab completes)"
	}
	in, cmd := m.openInput(&para.Prompt{Title: "Import", Label: label})
	clipImage, hasClipImage := attach.ProbeClipboard(context.Background(), m.imageClip)
	in.hint = func(text string) string {
		if hasClipImage && loader.Classify(text).Kind == loader.Clipboard {
			return clipImage
		}
		if hint, ok := attach.ProbeFile(text, inbox); ok {
			return hint
		}
		if !enabled {
			return "not an image: text import is off (digest.enabled)"
		}
		return loader.Describe(text, inbox, clipboard.ReadAll)
	}
	in.complete = func(text string) (string, []string) { return loader.Complete(text, inbox) }
	in.accept = func(m model, text string) (model, tea.Cmd) { return m.startImport(text, inbox) }
	in.refresh()
	m.input = in
	return m, cmd
}

// startImport begins an import of what was typed: the source is read and
// the agent run in the background. An image, from the clipboard or a file, is
// instead saved at once in the folder PARA shows: a PNG, or an SVG as it is.
func (m model) startImport(text, inbox string) (model, tea.Cmd) {
	// An image is saved as it is, without the agent.
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
	return m.runImport("Importing…", false, func(ctx context.Context, dg digest.Digester, st store.Store, kind store.Kind) (digest.Result, error, bool) {
		src, err := load(ctx)
		if err != nil {
			return digest.Result{}, err, true
		}
		res, err := digest.Import(ctx, st, time.Now(), kind, dg, src)
		return res, err, false
	})
}

// startRerun runs the import in folder again.
func (m model) startRerun(folder string) (model, tea.Cmd) {
	return m.runImport("Re-running "+filepath.Base(folder)+"…", true, func(ctx context.Context, dg digest.Digester, st store.Store, kind store.Kind) (digest.Result, error, bool) {
		res, err := digest.Rerun(ctx, st, time.Now(), kind, dg, folder)
		return res, err, false
	})
}

// runImport shows label in the status bar and returns the command that runs
// the import in the background: it builds the agent from the config, then
// calls run. The notebook and settings are captured now, so switching
// notebooks meanwhile cannot send the files to the wrong one.
func (m model) runImport(label string, rerun bool, run func(context.Context, digest.Digester, store.Store, store.Kind) (digest.Result, error, bool)) (model, tea.Cmd) {
	m.importing++
	m.importLabel = label
	st, cfg, sources, kind := m.store, m.digest, m.sources, digest.ImportKind()
	return m, func() tea.Msg {
		ctx := context.Background()
		dg, err := newDigester(cfg, sources)
		if err != nil {
			return importDoneMsg{store: st, rerun: rerun, err: err, early: true}
		}
		res, err, early := run(ctx, dg, st, kind)
		return importDoneMsg{store: st, rerun: rerun, result: res, err: err, early: early}
	}
}

// newDigester builds the agent from the config: the main prompt for each
// pass over the source and the merge prompt to combine the parts of a large
// one. A prompt is the one built into para unless the config names a file.
func newDigester(cfg config.Digest, sources []string) (digest.Digester, error) {
	prompt, err := digest.ResolvePrompt(cfg.Prompt, digest.DefaultPrompt)
	if err != nil {
		return nil, fmt.Errorf("the import prompt: %w", err)
	}
	mergePrompt, err := digest.ResolvePrompt(cfg.MergePrompt, digest.DefaultMergePrompt)
	if err != nil {
		return nil, fmt.Errorf("the merge prompt: %w", err)
	}
	part := digest.CLI{Command: cfg.Command, Model: cfg.Model, Timeout: cfg.Timeout, Prompt: prompt, Sources: sources}
	merge := part
	merge.Prompt = mergePrompt
	return digest.Chunked{Part: part, Merge: merge}, nil
}

// finishImport records the undo entry, refreshes the panels and reports how it
// went: a success message that clears itself, or why it failed and what is
// left.
func (m model) finishImport(msg importDoneMsg) (model, tea.Cmd) {
	m.importing = max(m.importing-1, 0)
	res := msg.result
	if res.Action.Undo != nil {
		m.history.Push(res.Action)
	}
	if msg.store == m.store {
		m.para = m.para.Reload()
		m = m.refreshLinks()
	}
	for _, w := range res.Warnings {
		m = m.notify(popup.Warning, capitalize(w)+".")
	}
	if res.KindMissing && (msg.err == nil || res.Rel != "") {
		m = m.notify(popup.Warning, "The import kind is not in your config, so the files have no kind. Add an entry named import under kinds.")
	}
	if msg.err != nil {
		return m.notify(popup.Error, importFailure(msg)), nil
	}
	verb := "Imported"
	if msg.rerun {
		verb = "Re-ran"
	}
	return m.setStatus(fmt.Sprintf("%s: %s (%s)", verb, filepath.Base(res.Rel), fileCount(len(res.Files))))
}

// importFailure says why an import failed and what was kept.
func importFailure(msg importDoneMsg) string {
	why := capitalize(msg.err.Error()) + "."
	switch {
	case msg.early:
		return "Could not import: " + why
	case msg.rerun && msg.result.Rel == "":
		return "Could not re-run the import: " + why + " The folder is unchanged."
	case msg.result.Rel == "":
		return "Could not import: " + why
	}
	return fmt.Sprintf("The import did not finish: %s The source is saved in %s; press I on that folder to try again.", why, filepath.ToSlash(msg.result.Rel))
}

func fileCount(n int) string {
	if n == 1 {
		return "1 file"
	}
	return fmt.Sprintf("%d files", n)
}

// setStatus shows text in the status bar until statusDuration passes.
func (m model) setStatus(text string) (model, tea.Cmd) {
	m.statusID++
	m.status = text
	id := m.statusID
	return m, tea.Tick(statusDuration, func(time.Time) tea.Msg { return statusClearMsg{id: id} })
}
