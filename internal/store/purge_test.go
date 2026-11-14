package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPurgeDailies(t *testing.T) {
	today := time.Date(2026, 11, 1, 15, 0, 0, 0, time.Local)
	tests := []struct {
		name    string
		days    int
		files   []string
		removed int
		kept    []string
	}{
		{"old archived daily goes", 30, []string{"archives/_daily/2026-09-01.md"}, 1, nil},
		{"exactly the period goes", 30, []string{"archives/_daily/2026-10-02.md"}, 1, nil},
		{"one day short stays", 30, []string{"archives/_daily/2026-10-03.md"}, 0, []string{"archives/_daily/2026-10-03.md"}},
		{"future date stays", 30, []string{"archives/_daily/2027-01-01.md"}, 0, []string{"archives/_daily/2027-01-01.md"}},
		{"other names stay", 30, []string{"archives/_daily/notes.md", "archives/_daily/2020-01-01.txt", "archives/_daily/2020-1-1.md"}, 0,
			[]string{"archives/_daily/notes.md", "archives/_daily/2020-01-01.txt", "archives/_daily/2020-1-1.md"}},
		{"other folders stay", 30, []string{"inbox/_daily/2020-01-01.md", "archives/2020-01-01.md", "archives/_daily/sub/2020-01-01.md", "areas/daily/2020-01-01.md"}, 0,
			[]string{"inbox/_daily/2020-01-01.md", "archives/2020-01-01.md", "archives/_daily/sub/2020-01-01.md", "areas/daily/2020-01-01.md"}},
		{"zero or less purges nothing", 0, []string{"archives/_daily/2020-01-01.md"}, 0, []string{"archives/_daily/2020-01-01.md"}},
		{"counts several", 30, []string{"archives/_daily/2020-01-01.md", "archives/_daily/2020-01-02.md", "archives/_daily/2026-10-31.md"}, 2,
			[]string{"archives/_daily/2026-10-31.md"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			for _, f := range tt.files {
				p := filepath.Join(root, f)
				require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
				require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
			}
			n, err := NewMarkdown(root).PurgeDailies(tt.days, today)
			require.NoError(t, err)
			assert.Equal(t, tt.removed, n)
			for _, f := range tt.kept {
				assert.FileExists(t, filepath.Join(root, f))
			}
			for _, f := range tt.files {
				if !contains(tt.kept, f) {
					assert.NoFileExists(t, filepath.Join(root, f))
				}
			}
		})
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func TestPurgeDailiesMissingFolder(t *testing.T) {
	n, err := NewMarkdown(t.TempDir()).PurgeDailies(30, time.Now())
	require.NoError(t, err)
	assert.Zero(t, n)
}

func TestPurgeDailiesLeavesSymlinksAlone(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	target := filepath.Join(outside, "keep.md")
	require.NoError(t, os.WriteFile(target, []byte("x"), 0o644))
	dir := filepath.Join(root, "archives", "_daily")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.Symlink(target, filepath.Join(dir, "2020-01-01.md")))

	n, err := NewMarkdown(root).PurgeDailies(30, time.Now())
	require.NoError(t, err)
	assert.Zero(t, n)
	assert.FileExists(t, target)
	assert.FileExists(t, filepath.Join(dir, "2020-01-01.md"))
}

func TestPurgeDailiesRefusesFolderLinkedOutside(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	victim := filepath.Join(outside, "2020-01-01.md")
	require.NoError(t, os.WriteFile(victim, []byte("x"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "archives"), 0o755))
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "archives", "_daily")))

	n, err := NewMarkdown(root).PurgeDailies(30, time.Now())
	require.Error(t, err)
	assert.Zero(t, n)
	assert.FileExists(t, victim)
}

func TestPurgeDailiesRefusesFolderLinkedInsideRoot(t *testing.T) {
	root := t.TempDir()
	inbox := filepath.Join(root, "inbox", "_daily")
	require.NoError(t, os.MkdirAll(inbox, 0o755))
	victim := filepath.Join(inbox, "2020-01-01.md")
	require.NoError(t, os.WriteFile(victim, []byte("x"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "archives"), 0o755))
	require.NoError(t, os.Symlink(inbox, filepath.Join(root, "archives", "_daily")))

	n, err := NewMarkdown(root).PurgeDailies(30, time.Now())
	require.Error(t, err)
	assert.Zero(t, n)
	assert.FileExists(t, victim)
}
