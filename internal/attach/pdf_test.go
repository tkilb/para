package attach

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tkilb/para/internal/loader"
)

var tinyPDF = []byte("%PDF-1.4\n1 0 obj<<>>endobj\ntrailer<<>>\n%%EOF\n")

func TestImportPDFFileIsSavedUnchanged(t *testing.T) {
	root, inbox := intakeRoot(t)
	s := newStoreAt(root)
	put(t, filepath.Join(inbox, "Paper.PDF"), tinyPDF)
	res, handled, err := Import(context.Background(), s, fakeSource{}, "projects/alpha", filepath.Join(inbox, "Paper.PDF"), inbox, when)
	require.NoError(t, err)
	assert.True(t, handled)
	assert.Equal(t, "projects/alpha/Paper.pdf", res.Rel)
	got, err := os.ReadFile(filepath.Join(root, res.Rel))
	require.NoError(t, err)
	assert.Equal(t, tinyPDF, got, "a PDF is stored byte for byte")
	assert.FileExists(t, filepath.Join(inbox, "Paper.PDF"), "the original is untouched")

	again, _, err := Import(context.Background(), s, fakeSource{}, "projects/alpha", filepath.Join(inbox, "Paper.PDF"), inbox, when)
	require.NoError(t, err)
	assert.Equal(t, "projects/alpha/Paper 2.pdf", again.Rel, "never overwrites")

	require.NoError(t, res.Action.Undo())
	assert.NoFileExists(t, filepath.Join(root, res.Rel))
}

func TestImportPDFAllowsJunkBeforeTheHeader(t *testing.T) {
	root, inbox := intakeRoot(t)
	put(t, filepath.Join(inbox, "a.pdf"), append([]byte("junk\n"), tinyPDF...))
	_, handled, err := Import(context.Background(), newStoreAt(root), fakeSource{}, "projects/alpha", filepath.Join(inbox, "a.pdf"), inbox, when)
	require.NoError(t, err)
	assert.True(t, handled)
}

func TestImportRefusesAFileThatIsNotAPDF(t *testing.T) {
	root, inbox := intakeRoot(t)
	put(t, filepath.Join(inbox, "fake.pdf"), []byte("just text, not a pdf"))
	_, handled, err := Import(context.Background(), newStoreAt(root), fakeSource{}, "projects/alpha", filepath.Join(inbox, "fake.pdf"), inbox, when)
	require.ErrorIs(t, err, ErrNotPDF)
	assert.True(t, handled, "a .pdf name is never sent to the text import")
	assert.NotContains(t, err.Error(), inbox, "the message shows no path")
	entries, readErr := os.ReadDir(filepath.Join(root, "projects", "alpha"))
	require.NoError(t, readErr)
	assert.Empty(t, entries)
}

func TestImportPDFFileErrors(t *testing.T) {
	root, inbox := intakeRoot(t)
	_, handled, err := Import(context.Background(), newStoreAt(root), fakeSource{}, "projects/alpha", filepath.Join(inbox, "none.pdf"), inbox, when)
	require.ErrorIs(t, err, loader.ErrNotFound)
	assert.True(t, handled)

	require.NoError(t, os.Mkdir(filepath.Join(inbox, "dir.pdf"), 0o755))
	_, _, err = Import(context.Background(), newStoreAt(root), fakeSource{}, "projects/alpha", filepath.Join(inbox, "dir.pdf"), inbox, when)
	require.ErrorIs(t, err, loader.ErrNotFile)
}

func TestProbePDF(t *testing.T) {
	_, inbox := intakeRoot(t)
	put(t, filepath.Join(inbox, "ok.pdf"), tinyPDF)
	put(t, filepath.Join(inbox, "bad.pdf"), []byte("nope"))
	hint, ok := ProbeFile(filepath.Join(inbox, "ok.pdf"), inbox)
	assert.True(t, ok)
	assert.Equal(t, fmt.Sprintf("PDF file, %d bytes, saved as is", len(tinyPDF)), hint)
	hint, ok = ProbeFile(filepath.Join(inbox, "bad.pdf"), inbox)
	assert.True(t, ok)
	assert.Equal(t, "PDF file, not a PDF", hint)
	_, ok = ProbeFile(filepath.Join(inbox, "missing.pdf"), inbox)
	assert.False(t, ok)
}
