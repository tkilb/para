package attach

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tkilb/para/internal/loader"
)

// intakeRoot is a notebook whose inbox holds some files to import.
func intakeRoot(t *testing.T) (root, inbox string) {
	t.Helper()
	root, _ = newStore(t)
	inbox = filepath.Join(root, "inbox")
	require.NoError(t, os.MkdirAll(inbox, 0o755))
	return root, inbox
}

func put(t *testing.T, path string, data []byte) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, data, 0o644))
}

func TestImportABlankInputSavesTheClipboardImage(t *testing.T) {
	root, inbox := intakeRoot(t)
	s := newStoreAt(root)
	data := tinyPNG(t)
	res, handled, err := Import(context.Background(), s, fakeSource{data: data}, "projects/alpha", "  ", inbox, when)
	require.NoError(t, err)
	assert.True(t, handled)
	assert.Equal(t, "projects/alpha/2026-11-18 09-05-07.png", res.Rel)
	got, err := os.ReadFile(filepath.Join(root, res.Rel))
	require.NoError(t, err)
	assert.Equal(t, data, got)
}

func TestImportAnEmptyClipboardFallsThroughToText(t *testing.T) {
	root, inbox := intakeRoot(t)
	s := newStoreAt(root)
	for name, err := range map[string]error{"no image": ErrNoImage, "no tool": ErrNoTool, "unreadable": ErrClipboard} {
		t.Run(name, func(t *testing.T) {
			_, handled, gotErr := Import(context.Background(), s, fakeSource{err: err}, "projects/alpha", "", inbox, when)
			require.NoError(t, gotErr)
			assert.False(t, handled, "the text import takes over")
		})
	}
	entries, err := os.ReadDir(filepath.Join(root, "projects", "alpha"))
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestImportADamagedClipboardImageIsAnError(t *testing.T) {
	root, inbox := intakeRoot(t)
	_, handled, err := Import(context.Background(), newStoreAt(root), fakeSource{data: tinyPNG(t)[:20]}, "projects/alpha", "", inbox, when)
	require.Error(t, err)
	assert.True(t, handled, "an image was there, so the text import must not run")
}

func TestImportAnImageFileKeepsItsName(t *testing.T) {
	root, inbox := intakeRoot(t)
	s := newStoreAt(root)
	put(t, filepath.Join(inbox, "Whiteboard.JPG"), tinyJPEG(t))
	res, handled, err := Import(context.Background(), s, fakeSource{}, "projects/alpha", filepath.Join(inbox, "Whiteboard.JPG"), inbox, when)
	require.NoError(t, err)
	assert.True(t, handled)
	assert.Equal(t, "projects/alpha/Whiteboard.png", res.Rel)
	got, err := os.ReadFile(filepath.Join(root, res.Rel))
	require.NoError(t, err)
	_, format, err := image.Decode(bytes.NewReader(got))
	require.NoError(t, err)
	assert.Equal(t, "png", format, "a JPEG is stored as PNG")
	assert.FileExists(t, filepath.Join(inbox, "Whiteboard.JPG"), "the original is never changed")
}

func TestImportABareNameIsFoundInTheInbox(t *testing.T) {
	root, inbox := intakeRoot(t)
	put(t, filepath.Join(inbox, "diagram.png"), tinyPNG(t))
	res, handled, err := Import(context.Background(), newStoreAt(root), fakeSource{}, "projects/alpha", "diagram", inbox, when)
	require.NoError(t, err)
	assert.True(t, handled)
	assert.Equal(t, "projects/alpha/diagram.png", res.Rel)
}

func TestImportNeverOverwrites(t *testing.T) {
	root, inbox := intakeRoot(t)
	s := newStoreAt(root)
	path := filepath.Join(inbox, "pic.png")
	put(t, path, tinyPNG(t))
	first, _, err := Import(context.Background(), s, fakeSource{}, "projects/alpha", path, inbox, when)
	require.NoError(t, err)
	second, _, err := Import(context.Background(), s, fakeSource{}, "projects/alpha", path, inbox, when)
	require.NoError(t, err)
	assert.Equal(t, "projects/alpha/pic.png", first.Rel)
	assert.Equal(t, "projects/alpha/pic 2.png", second.Rel)
}

func TestImportAnUnsafeFileNameFallsBackToTheDate(t *testing.T) {
	root, inbox := intakeRoot(t)
	path := filepath.Join(inbox, "\x1b[2J.png")
	put(t, path, tinyPNG(t))
	res, handled, err := Import(context.Background(), newStoreAt(root), fakeSource{}, "projects/alpha", path, inbox, when)
	require.NoError(t, err)
	assert.True(t, handled)
	assert.NotContains(t, res.Rel, "\x1b")
}

func TestImportLeavesOtherInputToTheTextImport(t *testing.T) {
	root, inbox := intakeRoot(t)
	put(t, filepath.Join(inbox, "standup.md"), []byte("Anna: hi"))
	for name, input := range map[string]string{
		"a text file":  filepath.Join(inbox, "standup.md"),
		"a bare name":  "standup",
		"a web page":   "https://example.com/a.png",
		"a folder":     inbox,
		"a missing md": filepath.Join(inbox, "nope.md"),
	} {
		t.Run(name, func(t *testing.T) {
			_, handled, err := Import(context.Background(), newStoreAt(root), fakeSource{data: tinyPNG(t)}, "projects/alpha", input, inbox, when)
			require.NoError(t, err)
			assert.False(t, handled)
		})
	}
}

func TestImportAnImageFileThatFails(t *testing.T) {
	root, inbox := intakeRoot(t)
	put(t, filepath.Join(inbox, "fake.png"), []byte("not an image"))
	tests := map[string]error{
		filepath.Join(inbox, "missing.png"): loader.ErrNotFound,
		filepath.Join(inbox, "fake.png"):    nil,
	}
	for input, want := range tests {
		_, handled, err := Import(context.Background(), newStoreAt(root), fakeSource{}, "projects/alpha", input, inbox, when)
		require.Error(t, err)
		assert.True(t, handled, "it is an image path, so it is not text")
		if want != nil {
			assert.ErrorIs(t, err, want)
		}
		assert.NotContains(t, err.Error(), inbox, "errors never show a path")
	}
	entries, err := os.ReadDir(filepath.Join(root, "projects", "alpha"))
	require.NoError(t, err)
	assert.Empty(t, entries, "nothing is written on failure")
}

func TestProbe(t *testing.T) {
	root, inbox := intakeRoot(t)
	_ = root
	put(t, filepath.Join(inbox, "pic.png"), tinyPNG(t))
	put(t, filepath.Join(inbox, "note.md"), []byte("x"))

	text, ok := ProbeClipboard(context.Background(), fakeSource{data: tinyPNG(t)})
	assert.True(t, ok)
	assert.Equal(t, "image from the clipboard, 4 x 4 px, saved as a PNG", text)
	_, ok = ProbeClipboard(context.Background(), fakeSource{err: ErrNoImage})
	assert.False(t, ok)
	_, ok = ProbeClipboard(context.Background(), fakeSource{data: []byte("junk")})
	assert.False(t, ok)

	text, ok = ProbeFile("pic", inbox)
	assert.True(t, ok)
	assert.Equal(t, "image file, 4 x 4 px, saved as a PNG", text)
	_, ok = ProbeFile("note.md", inbox)
	assert.False(t, ok)
	_, ok = ProbeFile("https://example.com/a.png", inbox)
	assert.False(t, ok)
	_, ok = ProbeFile("missing.png", inbox)
	assert.False(t, ok)
}

func TestPNGFixturesDecode(t *testing.T) {
	_, err := png.Decode(bytes.NewReader(tinyPNG(t)))
	require.NoError(t, err)
}
