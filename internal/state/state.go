// Package state persists the session between runs in a small SQLite
// database: which notebook was open, which panel had focus, and for each
// notebook its active PARA tab and, per tab, the current directory and the
// note under the cursor. Everything in it is a convenience; a missing or
// unreadable database never blocks the app.
package state

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

const (
	// TabCount is the number of PARA tabs.
	TabCount = 5
	// MaxStickies is how many stickies a notebook keeps.
	MaxStickies = 50
	// maxUIDLen bounds a saved uid, so a damaged value cannot grow the file.
	maxUIDLen = 128

	dbFile        = "state.db"
	dirPerm       = 0o700
	filePerm      = 0o600
	schemaVersion = 3
	busyTimeoutMS = 5000
)

// ErrNewerSchema reports a database written by a newer para.
var ErrNewerSchema = errors.New("state database is from a newer version")

// Session is what is remembered across notebooks.
type Session struct {
	// Notebook is the root path of the last notebook used.
	Notebook string
	// Focus is the number of the focused panel.
	Focus int
}

// Tab is one PARA tab's position.
type Tab struct {
	// Dir is the current directory relative to the tab root.
	Dir string
	// Name is the entry under the cursor, kept by name so it survives
	// changes to the directory.
	Name string
}

// Notebook is the remembered position inside one notebook.
type Notebook struct {
	ActiveTab int
	Tabs      [TabCount]Tab
}

// DB is an open state database.
type DB struct {
	sql *sql.DB
}

// Dir returns the directory holding the database: PARA_STATE_DIR, else
// $XDG_STATE_HOME/para when that is an absolute path, else
// ~/.local/state/para.
func Dir() (string, error) {
	if d := os.Getenv("PARA_STATE_DIR"); d != "" {
		return d, nil
	}
	if x := os.Getenv("XDG_STATE_HOME"); filepath.IsAbs(x) {
		return filepath.Join(x, "para"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home directory: %w", err)
	}
	return filepath.Join(home, ".local", "state", "para"), nil
}

// Open opens the database in dir, creating the directory and file when
// missing and bringing the schema up to date.
func Open(dir string) (*DB, error) {
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return nil, fmt.Errorf("create state directory: %w", err)
	}
	path := filepath.Join(dir, dbFile)
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open state database: %w", err)
	}
	// One connection keeps the pragmas below in effect for every statement.
	conn.SetMaxOpenConns(1)
	db := &DB{sql: conn}
	if err := db.init(path); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return db, nil
}

func (d *DB) init(path string) error {
	if _, err := d.sql.Exec(fmt.Sprintf(`PRAGMA busy_timeout = %d`, busyTimeoutMS)); err != nil {
		return fmt.Errorf("open state database: %w", err)
	}
	var version int
	if err := d.sql.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("read state database: %w", err)
	}
	if version > schemaVersion {
		return ErrNewerSchema
	}
	if version < schemaVersion {
		if err := d.migrate(); err != nil {
			return err
		}
	}
	if err := os.Chmod(path, filePerm); err != nil {
		return fmt.Errorf("restrict state database: %w", err)
	}
	return nil
}

func (d *DB) migrate() error {
	_, err := d.sql.Exec(fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS session (
	id       INTEGER PRIMARY KEY CHECK (id = 1),
	notebook TEXT NOT NULL,
	focus    INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS notebook (
	path       TEXT PRIMARY KEY,
	active_tab INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS tab (
	notebook TEXT NOT NULL,
	idx      INTEGER NOT NULL,
	dir      TEXT NOT NULL,
	name     TEXT NOT NULL,
	PRIMARY KEY (notebook, idx)
);
CREATE TABLE IF NOT EXISTS sticky (
	notebook TEXT NOT NULL,
	pos      INTEGER NOT NULL,
	uid      TEXT NOT NULL,
	PRIMARY KEY (notebook, pos)
);
CREATE TABLE IF NOT EXISTS mark (
	notebook TEXT NOT NULL,
	uid      TEXT NOT NULL,
	PRIMARY KEY (notebook, uid)
);
PRAGMA user_version = %d;`, schemaVersion))
	if err != nil {
		return fmt.Errorf("prepare state database: %w", err)
	}
	return nil
}

// Close closes the database.
func (d *DB) Close() error { return d.sql.Close() }

// LoadSession returns the saved session, or the zero Session when none.
func (d *DB) LoadSession() (Session, error) {
	var s Session
	err := d.sql.QueryRow(`SELECT notebook, focus FROM session WHERE id = 1`).Scan(&s.Notebook, &s.Focus)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, nil
	}
	if err != nil {
		return Session{}, fmt.Errorf("load session: %w", err)
	}
	return s, nil
}

// SaveSession replaces the saved session.
func (d *DB) SaveSession(s Session) error {
	_, err := d.sql.Exec(`
INSERT INTO session (id, notebook, focus) VALUES (1, ?, ?)
ON CONFLICT(id) DO UPDATE SET notebook = excluded.notebook, focus = excluded.focus`,
		s.Notebook, s.Focus)
	if err != nil {
		return fmt.Errorf("save session: %w", err)
	}
	return nil
}

// LoadNotebook returns the saved position in the notebook at path, or the
// zero Notebook when none. Values outside the valid range are dropped, so a
// damaged database cannot index past the tabs.
func (d *DB) LoadNotebook(path string) (Notebook, error) {
	var nb Notebook
	err := d.sql.QueryRow(`SELECT active_tab FROM notebook WHERE path = ?`, path).Scan(&nb.ActiveTab)
	if errors.Is(err, sql.ErrNoRows) {
		return Notebook{}, nil
	}
	if err != nil {
		return Notebook{}, fmt.Errorf("load notebook state: %w", err)
	}
	if nb.ActiveTab < 0 || nb.ActiveTab >= TabCount {
		nb.ActiveTab = 0
	}
	rows, err := d.sql.Query(`SELECT idx, dir, name FROM tab WHERE notebook = ?`, path)
	if err != nil {
		return Notebook{}, fmt.Errorf("load notebook state: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var idx int
		var t Tab
		if err := rows.Scan(&idx, &t.Dir, &t.Name); err != nil {
			return Notebook{}, fmt.Errorf("load notebook state: %w", err)
		}
		if idx >= 0 && idx < TabCount {
			nb.Tabs[idx] = t
		}
	}
	if err := rows.Err(); err != nil {
		return Notebook{}, fmt.Errorf("load notebook state: %w", err)
	}
	return nb, nil
}

// SaveNotebook replaces the saved position in the notebook at path, all or
// nothing.
func (d *DB) SaveNotebook(path string, nb Notebook) error {
	tx, err := d.sql.Begin()
	if err != nil {
		return fmt.Errorf("save notebook state: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`
INSERT INTO notebook (path, active_tab) VALUES (?, ?)
ON CONFLICT(path) DO UPDATE SET active_tab = excluded.active_tab`, path, nb.ActiveTab); err != nil {
		return fmt.Errorf("save notebook state: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM tab WHERE notebook = ?`, path); err != nil {
		return fmt.Errorf("save notebook state: %w", err)
	}
	for i, t := range nb.Tabs {
		if t == (Tab{}) {
			continue
		}
		if _, err := tx.Exec(`INSERT INTO tab (notebook, idx, dir, name) VALUES (?, ?, ?, ?)`,
			path, i, t.Dir, t.Name); err != nil {
			return fmt.Errorf("save notebook state: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("save notebook state: %w", err)
	}
	return nil
}

// LoadStickies returns the uids of the notes pinned in the notebook at path,
// in the order they were pinned. Blank or oversized values are dropped.
func (d *DB) LoadStickies(path string) ([]string, error) {
	rows, err := d.sql.Query(`SELECT uid FROM sticky WHERE notebook = ? ORDER BY pos`, path)
	if err != nil {
		return nil, fmt.Errorf("load stickies: %w", err)
	}
	defer rows.Close()
	var uids []string
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			return nil, fmt.Errorf("load stickies: %w", err)
		}
		uids = append(uids, uid)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load stickies: %w", err)
	}
	return cleanStickies(uids), nil
}

// SaveStickies replaces the stickies of the notebook at path, all or nothing.
// Blank, oversized and repeated uids are dropped and at most MaxStickies are
// kept.
func (d *DB) SaveStickies(path string, uids []string) error {
	tx, err := d.sql.Begin()
	if err != nil {
		return fmt.Errorf("save stickies: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM sticky WHERE notebook = ?`, path); err != nil {
		return fmt.Errorf("save stickies: %w", err)
	}
	for i, uid := range cleanStickies(uids) {
		if _, err := tx.Exec(`INSERT INTO sticky (notebook, pos, uid) VALUES (?, ?, ?)`, path, i, uid); err != nil {
			return fmt.Errorf("save stickies: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("save stickies: %w", err)
	}
	return nil
}

// cleanStickies drops blank, oversized and repeated uids, keeps the order and
// stops at MaxStickies.
func cleanStickies(uids []string) []string { return cleanUIDs(uids, MaxStickies) }

// cleanUIDs drops blank, oversized and repeated uids, keeps the order and
// stops at limit.
func cleanUIDs(uids []string, limit int) []string {
	seen := make(map[string]bool, len(uids))
	var out []string
	for _, uid := range uids {
		if uid == "" || len(uid) > maxUIDLen || seen[uid] {
			continue
		}
		seen[uid] = true
		if out = append(out, uid); len(out) == limit {
			break
		}
	}
	return out
}
