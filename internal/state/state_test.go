package state

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func open(t *testing.T) (*DB, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "state")
	db, err := Open(dir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db, dir
}

func TestDir(t *testing.T) {
	home := t.TempDir()
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"override wins", map[string]string{"PARA_STATE_DIR": "/o", "XDG_STATE_HOME": "/x"}, "/o"},
		{"xdg state home", map[string]string{"XDG_STATE_HOME": "/x"}, "/x/para"},
		{"relative xdg is ignored", map[string]string{"XDG_STATE_HOME": "rel"}, filepath.Join(home, ".local", "state", "para")},
		{"home fallback", nil, filepath.Join(home, ".local", "state", "para")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", home)
			t.Setenv("PARA_STATE_DIR", "")
			t.Setenv("XDG_STATE_HOME", "")
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			got, err := Dir()
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestOpenCreatesPrivateDirAndFile(t *testing.T) {
	_, dir := open(t)
	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
	info, err = os.Stat(filepath.Join(dir, "state.db"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestEmptyStateLoadsAsZero(t *testing.T) {
	db, _ := open(t)
	s, err := db.LoadSession()
	require.NoError(t, err)
	assert.Equal(t, Session{}, s)
	nb, err := db.LoadNotebook("/n")
	require.NoError(t, err)
	assert.Equal(t, Notebook{}, nb)
}

func TestSessionRoundTripAndOverwrite(t *testing.T) {
	db, _ := open(t)
	require.NoError(t, db.SaveSession(Session{Notebook: "/a", Focus: 2}))
	require.NoError(t, db.SaveSession(Session{Notebook: "/b", Focus: 0}))
	got, err := db.LoadSession()
	require.NoError(t, err)
	assert.Equal(t, Session{Notebook: "/b", Focus: 0}, got)
}

func TestNotebookStateIsPerNotebook(t *testing.T) {
	db, _ := open(t)
	a := Notebook{ActiveTab: 1}
	a.Tabs[1] = Tab{Dir: "work/q4", Name: "plan.md"}
	a.Tabs[0] = Tab{Dir: "daily", Name: "2026-10-30.md"}
	b := Notebook{ActiveTab: 4}
	b.Tabs[4] = Tab{Dir: "old", Name: "x.md"}
	require.NoError(t, db.SaveNotebook("/a", a))
	require.NoError(t, db.SaveNotebook("/b", b))

	got, err := db.LoadNotebook("/a")
	require.NoError(t, err)
	assert.Equal(t, a, got)
	got, err = db.LoadNotebook("/b")
	require.NoError(t, err)
	assert.Equal(t, b, got)
}

func TestSaveNotebookReplacesEverything(t *testing.T) {
	db, _ := open(t)
	first := Notebook{ActiveTab: 2}
	first.Tabs[2] = Tab{Dir: "d", Name: "n.md"}
	require.NoError(t, db.SaveNotebook("/a", first))
	require.NoError(t, db.SaveNotebook("/a", Notebook{ActiveTab: 0}))

	got, err := db.LoadNotebook("/a")
	require.NoError(t, err)
	assert.Equal(t, Notebook{}, got, "an old tab does not linger")
}

func TestStateSurvivesReopen(t *testing.T) {
	db, dir := open(t)
	require.NoError(t, db.SaveSession(Session{Notebook: "/a", Focus: 3}))
	require.NoError(t, db.Close())

	again, err := Open(dir)
	require.NoError(t, err)
	defer again.Close()
	got, err := again.LoadSession()
	require.NoError(t, err)
	assert.Equal(t, Session{Notebook: "/a", Focus: 3}, got)
}

func TestLoadClampsOutOfRangeValues(t *testing.T) {
	db, _ := open(t)
	_, err := db.sql.Exec(`INSERT INTO notebook(path, active_tab) VALUES ('/a', 99)`)
	require.NoError(t, err)
	_, err = db.sql.Exec(`INSERT INTO tab(notebook, idx, dir, name) VALUES ('/a', 77, 'x', 'y'), ('/a', 1, 'ok', 'k')`)
	require.NoError(t, err)

	got, err := db.LoadNotebook("/a")
	require.NoError(t, err)
	assert.Equal(t, 0, got.ActiveTab)
	assert.Equal(t, Tab{Dir: "ok", Name: "k"}, got.Tabs[1])
}

func TestOpenRefusesANewerSchema(t *testing.T) {
	db, dir := open(t)
	_, err := db.sql.Exec(`PRAGMA user_version = 999`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	_, err = Open(dir)
	assert.ErrorIs(t, err, ErrNewerSchema)
}

func TestOpenRejectsACorruptFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "state.db"), []byte("this is not a database, just text padding padding padding"), 0o600))
	_, err := Open(dir)
	assert.Error(t, err)
}

func TestStickiesRoundTripInOrderPerNotebook(t *testing.T) {
	db, _ := open(t)
	got, err := db.LoadStickies("/a")
	require.NoError(t, err)
	assert.Empty(t, got)

	require.NoError(t, db.SaveStickies("/a", []string{"u3", "u1", "u2"}))
	require.NoError(t, db.SaveStickies("/b", []string{"x"}))

	got, err = db.LoadStickies("/a")
	require.NoError(t, err)
	assert.Equal(t, []string{"u3", "u1", "u2"}, got, "the order they were pinned in")
	got, err = db.LoadStickies("/b")
	require.NoError(t, err)
	assert.Equal(t, []string{"x"}, got)
}

func TestSaveStickiesReplacesAndCleans(t *testing.T) {
	db, _ := open(t)
	require.NoError(t, db.SaveStickies("/a", []string{"u1", "u2"}))
	require.NoError(t, db.SaveStickies("/a", []string{"u2", "", "u2", "u9"}))
	got, err := db.LoadStickies("/a")
	require.NoError(t, err)
	assert.Equal(t, []string{"u2", "u9"}, got, "blank and repeated uids are dropped")

	require.NoError(t, db.SaveStickies("/a", nil))
	got, err = db.LoadStickies("/a")
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestSaveStickiesBoundsWhatIsStored(t *testing.T) {
	db, _ := open(t)
	many := make([]string, MaxStickies+10)
	for i := range many {
		many[i] = "u" + string(rune('a'+i%26)) + string(rune('a'+i/26))
	}
	require.NoError(t, db.SaveStickies("/a", many))
	got, err := db.LoadStickies("/a")
	require.NoError(t, err)
	assert.Len(t, got, MaxStickies)

	long := string(make([]byte, 0)) + "x"
	for len(long) <= maxUIDLen {
		long += long
	}
	require.NoError(t, db.SaveStickies("/a", []string{long, "ok"}))
	got, err = db.LoadStickies("/a")
	require.NoError(t, err)
	assert.Equal(t, []string{"ok"}, got, "an absurdly long uid is dropped")
}

func TestOpenUpgradesAVersionOneDatabase(t *testing.T) {
	db, dir := open(t)
	require.NoError(t, db.SaveSession(Session{Notebook: "/a", Focus: 2}))
	_, err := db.sql.Exec(`DROP TABLE sticky; PRAGMA user_version = 1`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	db, err = Open(dir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	s, err := db.LoadSession()
	require.NoError(t, err)
	assert.Equal(t, "/a", s.Notebook, "what was saved stays")
	require.NoError(t, db.SaveStickies("/a", []string{"u1"}))
}

func TestMarksRoundTripPerNotebook(t *testing.T) {
	db, _ := open(t)
	got, err := db.LoadMarks("/a")
	require.NoError(t, err)
	assert.Empty(t, got)

	require.NoError(t, db.SaveMarks("/a", []string{"u3", "u1", "u2"}))
	require.NoError(t, db.SaveMarks("/b", []string{"x"}))

	got, err = db.LoadMarks("/a")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"u3", "u1", "u2"}, got)
	got, err = db.LoadMarks("/b")
	require.NoError(t, err)
	assert.Equal(t, []string{"x"}, got)
}

func TestSaveMarksReplacesAndCleans(t *testing.T) {
	db, _ := open(t)
	require.NoError(t, db.SaveMarks("/a", []string{"u1", "u2"}))
	require.NoError(t, db.SaveMarks("/a", []string{"u2", "", "u2", "u9"}))
	got, err := db.LoadMarks("/a")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"u2", "u9"}, got, "blank and repeated uids are dropped")

	require.NoError(t, db.SaveMarks("/a", nil))
	got, err = db.LoadMarks("/a")
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestSaveMarksBoundsWhatIsStored(t *testing.T) {
	db, _ := open(t)
	many := make([]string, MaxMarks+10)
	for i := range many {
		many[i] = fmt.Sprintf("u%d", i)
	}
	require.NoError(t, db.SaveMarks("/a", many))
	got, err := db.LoadMarks("/a")
	require.NoError(t, err)
	assert.Len(t, got, MaxMarks)

	long := strings.Repeat("x", maxUIDLen+1)
	require.NoError(t, db.SaveMarks("/a", []string{long, "ok"}))
	got, err = db.LoadMarks("/a")
	require.NoError(t, err)
	assert.Equal(t, []string{"ok"}, got, "an absurdly long uid is dropped")
}

func TestOpenUpgradesAVersionTwoDatabase(t *testing.T) {
	db, dir := open(t)
	require.NoError(t, db.SaveStickies("/a", []string{"u1"}))
	_, err := db.sql.Exec(`DROP TABLE mark; PRAGMA user_version = 2`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	db, err = Open(dir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	st, err := db.LoadStickies("/a")
	require.NoError(t, err)
	assert.Equal(t, []string{"u1"}, st, "what was saved stays")
	require.NoError(t, db.SaveMarks("/a", []string{"m1"}))
	got, err := db.LoadMarks("/a")
	require.NoError(t, err)
	assert.Equal(t, []string{"m1"}, got)
}
