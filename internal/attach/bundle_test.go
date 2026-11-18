package attach

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tkilb/para/internal/store"
)

// bundleRoot makes a notebook with a note and a loose image.
func bundleRoot(t *testing.T) (string, store.Store) {
	t.Helper()
	root, s := newStore(t)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "inbox"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "projects", "alpha", "plan.md"), []byte("# plan\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "inbox", "pic.png"), tinyPNG(t), 0o644))
	return root, s
}

func TestBundleCopiesTheImageIntoTheSidecar(t *testing.T) {
	root, s := bundleRoot(t)
	res, err := Bundle(s, "projects/alpha/plan.md", "inbox/pic.png", false)
	require.NoError(t, err)
	assert.Equal(t, "projects/alpha/plan.assets/pic.png", res.Rel)
	assert.FileExists(t, filepath.Join(root, "inbox", "pic.png"), "a yank leaves the original")
	assert.FileExists(t, filepath.Join(root, res.Rel))

	entries, err := s.List("projects/alpha", store.ListOptions{})
	require.NoError(t, err)
	require.Len(t, entries, 1, "the sidecar folder is hidden")
	assert.Equal(t, 1, entries[0].Attachments)

	require.NoError(t, res.Action.Undo())
	assert.NoFileExists(t, filepath.Join(root, res.Rel))
	assert.NoDirExists(t, filepath.Join(root, "projects", "alpha", "plan.assets"), "undo leaves no empty folder")
	require.NoError(t, res.Action.Redo())
	assert.FileExists(t, filepath.Join(root, res.Rel))
}

func TestBundleMovesTheImageForACut(t *testing.T) {
	root, s := bundleRoot(t)
	res, err := Bundle(s, "projects/alpha/plan.md", "inbox/pic.png", true)
	require.NoError(t, err)
	assert.NoFileExists(t, filepath.Join(root, "inbox", "pic.png"))
	assert.FileExists(t, filepath.Join(root, res.Rel))

	require.NoError(t, res.Action.Undo())
	assert.FileExists(t, filepath.Join(root, "inbox", "pic.png"), "undo puts the image back")
	assert.NoDirExists(t, filepath.Join(root, "projects", "alpha", "plan.assets"))
}

func TestBundleNeverOverwrites(t *testing.T) {
	root, s := bundleRoot(t)
	first, err := Bundle(s, "projects/alpha/plan.md", "inbox/pic.png", false)
	require.NoError(t, err)
	second, err := Bundle(s, "projects/alpha/plan.md", "inbox/pic.png", false)
	require.NoError(t, err)
	assert.Equal(t, "projects/alpha/plan.assets/pic.png", first.Rel)
	assert.Equal(t, "projects/alpha/plan.assets/pic 2.png", second.Rel)
	assert.FileExists(t, filepath.Join(root, first.Rel))
}

func TestBundleFollowsTheNote(t *testing.T) {
	root, s := bundleRoot(t)
	_, err := Bundle(s, "projects/alpha/plan.md", "inbox/pic.png", false)
	require.NoError(t, err)
	_, err = s.Move("projects/alpha/plan.md", "projects/alpha/idea.md")
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(root, "projects", "alpha", "idea.assets", "pic.png"))
}

func TestBundleRefuses(t *testing.T) {
	_, s := bundleRoot(t)
	_, err := Bundle(s, "projects/alpha/plan.md", "inbox/pic.png", false)
	require.NoError(t, err)
	tests := []struct {
		name  string
		note  string
		image string
		want  error
	}{
		{"the target is a folder", "projects/alpha", "inbox/pic.png", ErrNotNote},
		{"the target is an image", "inbox/pic.png", "inbox/pic.png", ErrNotNote},
		{"the item is a note", "projects/alpha/plan.md", "projects/alpha/plan.md", ErrNotImage},
		{"the note is missing", "projects/alpha/none.md", "inbox/pic.png", store.ErrNotFound},
		{"the image is missing", "projects/alpha/plan.md", "inbox/none.png", store.ErrNotFound},
		{"already in this sidecar", "projects/alpha/plan.md", "projects/alpha/plan.assets/pic.png", ErrAlreadyBundled},
		{"outside the notebook", "projects/alpha/plan.md", "../pic.png", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Bundle(s, tt.note, tt.image, true)
			require.Error(t, err)
			if tt.want != nil {
				assert.ErrorIs(t, err, tt.want)
			}
		})
	}
}
