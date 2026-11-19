package marks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadMissingFileIsNoMarks(t *testing.T) {
	got, exists, err := Load(t.TempDir())
	require.NoError(t, err)
	assert.False(t, exists)
	assert.Empty(t, got)
}

func TestSaveLoadRoundTripKeepsOrder(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, Save(root, []string{"u3", "u1", "u2"}))
	got, exists, err := Load(root)
	require.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, []string{"u3", "u1", "u2"}, got)
}

func TestSaveCleansAndBounds(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{"blank and repeated dropped", []string{"a", "", "a", "b"}, []string{"a", "b"}},
		{"oversized dropped", []string{strings.Repeat("x", maxUIDLen+1), "ok"}, []string{"ok"}},
		{"whitespace in a uid dropped", []string{"a b", "c\nd", "e"}, []string{"e"}},
		{"cap at Max", manyUIDs(Max + 10), manyUIDs(Max)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			require.NoError(t, Save(root, tc.in))
			got, _, err := Load(root)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestLoadToleratesHandEditedFile(t *testing.T) {
	root := t.TempDir()
	body := "# my marks\n\n  u1  \r\nu1\n" + strings.Repeat("y", maxUIDLen+1) + "\nu2\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, FileName), []byte(body), 0o644))
	got, exists, err := Load(root)
	require.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, []string{"u1", "u2"}, got)
}

func TestSaveEmptyKeepsAnEmptyFile(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, Save(root, []string{"a"}))
	require.NoError(t, Save(root, nil))
	got, exists, err := Load(root)
	require.NoError(t, err)
	assert.True(t, exists, "an emptied file still records that marks were migrated")
	assert.Empty(t, got)
}

func TestSaveLeavesNoTempFilesAndReplacesInPlace(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, Save(root, []string{"a"}))
	require.NoError(t, Save(root, []string{"b"}))
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, FileName, entries[0].Name())
}

func TestSaveMissingNotebookFails(t *testing.T) {
	err := Save(filepath.Join(t.TempDir(), "gone"), []string{"a"})
	assert.Error(t, err)
}

func manyUIDs(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = "u" + strings.Repeat("0", 3) + string(rune('a'+i%26)) + strings.Repeat("z", i/26)
	}
	return out
}
