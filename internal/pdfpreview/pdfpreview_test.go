package pdfpreview

import (
	"bytes"
	"fmt"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// minimalPDF returns a one-page, blank, 200 x 100 point PDF with a valid
// cross-reference table.
func minimalPDF() string {
	objs := []string{
		"<</Type/Catalog/Pages 2 0 R>>",
		"<</Type/Pages/Kids[3 0 R]/Count 1>>",
		"<</Type/Page/Parent 2 0 R/MediaBox[0 0 200 100]>>",
	}
	out := "%PDF-1.4\n"
	offsets := make([]int, len(objs))
	for i, o := range objs {
		offsets[i] = len(out)
		out += fmt.Sprintf("%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := "xref\n0 4\n0000000000 65535 f \n"
	for _, off := range offsets {
		xref += fmt.Sprintf("%010d 00000 n \n", off)
	}
	return out + xref + fmt.Sprintf("trailer\n<</Size 4/Root 1 0 R>>\nstartxref\n%d\n%%%%EOF\n", len(out))
}

func TestArgsPassThePathAsOneArgument(t *testing.T) {
	got := args("/tmp/a b.pdf")
	assert.Equal(t, "/tmp/a b.pdf", got[len(got)-1], "the path is last and no output prefix follows")
}

func TestFirstPageRefusesARelativePath(t *testing.T) {
	_, err := FirstPage("-opt.pdf")
	assert.ErrorIs(t, err, ErrUnreadable)
}

func TestFirstPage(t *testing.T) {
	if _, err := exec.LookPath(tool); err != nil {
		t.Skip("pdftoppm is not installed")
	}
	dir := t.TempDir()
	good := filepath.Join(dir, "page.pdf")
	require.NoError(t, os.WriteFile(good, []byte(minimalPDF()), 0o600))
	bad := filepath.Join(dir, "bad.pdf")
	require.NoError(t, os.WriteFile(bad, []byte("not a pdf"), 0o600))

	data, err := FirstPage(good)
	require.NoError(t, err)
	img, err := png.Decode(bytes.NewReader(data))
	require.NoError(t, err)
	assert.Equal(t, 2*img.Bounds().Dy(), img.Bounds().Dx(), "a 200 x 100 page keeps its shape")

	_, err = FirstPage(bad)
	assert.ErrorIs(t, err, ErrUnreadable)
	_, err = FirstPage(filepath.Join(dir, "missing.pdf"))
	assert.ErrorIs(t, err, ErrUnreadable)
}

func TestFirstPageWithoutTheTool(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := FirstPage(filepath.Join(t.TempDir(), "x.pdf"))
	assert.ErrorIs(t, err, ErrNoTool)
}
