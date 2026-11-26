// Command para is a TUI note browser.
package main

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/tkilb/para/internal/cli"
	"github.com/tkilb/para/internal/config"
	"github.com/tkilb/para/internal/store"
	"github.com/tkilb/para/internal/ui/popup"
	"github.com/tkilb/para/internal/version"
)

// startup is what the config gives the interface and the shell commands.
type startup struct {
	store         store.Store
	root          string
	sources       []string
	templatesDir  string
	retentionDays int
	mouse         bool
	digest        config.Digest
	// notebooks are all the configured notebooks and active the index of
	// the one opened.
	notebooks []config.Notebook
	active    int
}

// openStore loads the config and opens the notebook used last, or the first
// available one when that is gone.
func openStore() (startup, error) {
	path, err := config.DefaultPath()
	if err != nil {
		return startup{}, err
	}
	cfg, err := config.Load(path)
	if err != nil {
		return startup{}, err
	}
	if err := store.SetKinds(cfg.Kinds); err != nil {
		return startup{}, err
	}
	if i, ok := cfg.Select(lastNotebook()); ok {
		nb := cfg.Notebooks[i]
		return startup{
			store: store.NewMarkdown(nb.Path), root: nb.Path,
			sources: cfg.Sources, templatesDir: cfg.TemplatesDir, retentionDays: cfg.Retention.DailyDays,
			mouse: cfg.UI.MouseEnabled(), notebooks: cfg.Notebooks, active: i, digest: cfg.Digest,
		}, nil
	}
	return startup{}, errors.New("none of the configured notebooks is an existing directory")
}

// isVersionArg reports whether args (os.Args[1:]) ask for the version
// rather than the TUI.
func isVersionArg(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "version", "--version", "-v":
		return true
	}
	return false
}

func main() {
	if isVersionArg(os.Args[1:]) {
		fmt.Printf("para %s\n", version.Version)
		return
	}
	open := func() (store.Store, []string, error) {
		st, err := openStore()
		return st.store, st.sources, err
	}
	if handled, code := cli.Run(os.Args[1:], cli.SystemEnv(open, clipboard.ReadAll)); handled {
		os.Exit(code)
	}

	boot, err := openStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "para: %v\n", err)
		os.Exit(1)
	}
	sess, sessErr := openSession(boot.root)
	if sess.db != nil {
		defer sess.db.Close()
	}
	purged, purgeErr := boot.store.PurgeDailies(boot.retentionDays, time.Now())
	start := initialModel(boot.store)
	start.sources = boot.sources
	start.templatesDir = boot.templatesDir
	start.digest = boot.digest
	start.para = start.para.WithRetention(boot.retentionDays, time.Now)
	start = start.withNotebooks(boot.notebooks, boot.active, boot.retentionDays)
	if sessErr != nil {
		start = start.notify(popup.Warning, fmt.Sprintf("Your position will not be saved: %s.", sessErr))
	}
	start = start.notifyPurge(purged, purgeErr).withSession(sess).refreshLinks()
	start.focus = panelPARA
	start.syncDirty = false // Init reads the git state itself

	// The alternate screen gives the program the full terminal buffer, so
	// resuming after ctrl+z or an external editor repaints cleanly. Focus
	// reporting lets the app refresh when the terminal regains focus (not
	// every terminal sends it; ctrl+l always works).
	opts := []tea.ProgramOption{tea.WithAltScreen(), tea.WithReportFocus()}
	if boot.mouse {
		opts = append(opts, tea.WithMouseCellMotion())
	}
	p := tea.NewProgram(start, opts...)
	final, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error running para: %v\n", err)
		os.Exit(1)
	}
	if m, ok := final.(model); ok && m.saveErr != nil {
		fmt.Fprintf(os.Stderr, "para: could not save your position: %v\n", m.saveErr)
	}
}
