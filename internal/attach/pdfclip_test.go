package attach

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fileClip is a clipboard that holds a copied file as well as its thumbnail,
// which is what Finder puts there when a file is copied.
type fileClip struct {
	fakeSource
	path    string
	fileErr error
}

func (f fileClip) File(context.Context) (string, error) { return f.path, f.fileErr }

func TestImportQuotedPDFPath(t *testing.T) {
	root, inbox := intakeRoot(t)
	name := "Car Care Acquisition Partner Brief updated 11-20-26.pdf"
	put(t, filepath.Join(inbox, name), tinyPDF)
	for label, typed := range map[string]string{
		"single quotes": "'" + filepath.Join(inbox, name) + "'",
		"double quotes": `"` + filepath.Join(inbox, name) + `"`,
	} {
		t.Run(label, func(t *testing.T) {
			res, handled, err := Import(context.Background(), newStoreAt(root), fakeSource{}, "projects/alpha", typed, inbox, when)
			require.NoError(t, err)
			assert.True(t, handled)
			assert.Contains(t, res.Rel, "Car Care Acquisition Partner Brief updated 11-20-26")
		})
	}
}

func TestProbeQuotedPDFPath(t *testing.T) {
	_, inbox := intakeRoot(t)
	put(t, filepath.Join(inbox, "a b.pdf"), tinyPDF)
	_, ok := ProbeFile("'"+filepath.Join(inbox, "a b.pdf")+"'", inbox)
	assert.True(t, ok)
}

func TestImportClipboardWithACopiedPDFSavesThePDFNotTheThumbnail(t *testing.T) {
	root, inbox := intakeRoot(t)
	put(t, filepath.Join(inbox, "Brief.pdf"), tinyPDF)
	clip := fileClip{fakeSource: fakeSource{data: tinyPNG(t)}, path: filepath.Join(inbox, "Brief.pdf")}

	res, handled, err := Import(context.Background(), newStoreAt(root), clip, "projects/alpha", "", inbox, when)
	require.NoError(t, err)
	assert.True(t, handled)
	assert.Equal(t, "projects/alpha/Brief.pdf", res.Rel)
	got, err := os.ReadFile(filepath.Join(root, res.Rel))
	require.NoError(t, err)
	assert.Equal(t, tinyPDF, got)
	entries, err := os.ReadDir(filepath.Join(root, "projects", "alpha"))
	require.NoError(t, err)
	assert.Len(t, entries, 1, "the thumbnail is not saved as well")
}

func TestImportClipboardWithACopiedImageFileSavesTheFile(t *testing.T) {
	root, inbox := intakeRoot(t)
	put(t, filepath.Join(inbox, "Whiteboard.jpg"), tinyJPEG(t))
	clip := fileClip{fakeSource: fakeSource{data: tinyPNG(t)}, path: filepath.Join(inbox, "Whiteboard.jpg")}
	res, handled, err := Import(context.Background(), newStoreAt(root), clip, "projects/alpha", "", inbox, when)
	require.NoError(t, err)
	assert.True(t, handled)
	assert.Equal(t, "projects/alpha/Whiteboard.png", res.Rel)
}

func TestImportClipboardWithACopiedNonPDFFileFallsBack(t *testing.T) {
	root, inbox := intakeRoot(t)
	put(t, filepath.Join(inbox, "notes.txt"), []byte("hello"))
	clip := fileClip{fakeSource: fakeSource{data: tinyPNG(t)}, path: filepath.Join(inbox, "notes.txt")}
	res, handled, err := Import(context.Background(), newStoreAt(root), clip, "projects/alpha", "", inbox, when)
	require.NoError(t, err)
	assert.True(t, handled)
	assert.Equal(t, "projects/alpha/2026-11-18 09-05-07.png", res.Rel, "the clipboard image is saved as before")
}

func TestImportClipboardFileLookupFailureFallsBack(t *testing.T) {
	root, inbox := intakeRoot(t)
	clip := fileClip{fakeSource: fakeSource{data: tinyPNG(t)}, fileErr: ErrClipboard}
	res, handled, err := Import(context.Background(), newStoreAt(root), clip, "projects/alpha", "", inbox, when)
	require.NoError(t, err)
	assert.True(t, handled)
	assert.Contains(t, res.Rel, ".png")
}

func TestImportClipboardCopiedPDFThatIsNotAPDFIsAnErrorWithoutAPath(t *testing.T) {
	root, inbox := intakeRoot(t)
	put(t, filepath.Join(inbox, "fake.pdf"), []byte("not a pdf"))
	clip := fileClip{fakeSource: fakeSource{data: tinyPNG(t)}, path: filepath.Join(inbox, "fake.pdf")}
	_, handled, err := Import(context.Background(), newStoreAt(root), clip, "projects/alpha", "", inbox, when)
	require.ErrorIs(t, err, ErrNotPDF)
	assert.True(t, handled)
	assert.NotContains(t, err.Error(), inbox)
}

func TestProbeClipboardDescribesACopiedPDF(t *testing.T) {
	_, inbox := intakeRoot(t)
	put(t, filepath.Join(inbox, "Brief.pdf"), tinyPDF)
	clip := fileClip{fakeSource: fakeSource{data: tinyPNG(t)}, path: filepath.Join(inbox, "Brief.pdf")}
	hint, ok := ProbeClipboard(context.Background(), clip)
	assert.True(t, ok)
	assert.Contains(t, hint, "PDF file")
	assert.Contains(t, hint, "saved as is")
}

func TestClipboardFileOnMac(t *testing.T) {
	tests := []struct {
		name    string
		out     string
		runErr  bool
		want    string
		wantErr error
	}{
		{"a file url is decoded", "file:///Users/me/Car%20Care%20Brief.pdf\n", false, "/Users/me/Car Care Brief.pdf", nil},
		{"nothing there", "\n", false, "", ErrNoFile},
		{"not a file url", "https://example.com/a.pdf", false, "", ErrNoFile},
		{"a remote host is refused", "file://evil.example.com/share/a.pdf", false, "", ErrNoFile},
		{"localhost is fine", "file://localhost/tmp/a.pdf", false, "/tmp/a.pdf", nil},
		{"a bad escape", "file:///tmp/%zz.pdf", false, "", ErrNoFile},
		{"osascript fails", "", true, "", ErrClipboard},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Clipboard{GOOS: "darwin", Run: func(_ context.Context, name string, args ...string) ([]byte, error) {
				if tt.runErr {
					return nil, errors.New("exit status 1")
				}
				assert.Equal(t, "osascript", name)
				return []byte(tt.out), nil
			}}
			got, err := c.File(context.Background())
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestClipboardFileOnLinux(t *testing.T) {
	var calls []call
	c := linux(map[string]string{"WAYLAND_DISPLAY": "wayland-0"}, []string{"wl-paste"}, fakeRun(map[string]string{
		"wl-paste --list-types":                      "text/uri-list\ntext/plain\n",
		"wl-paste --no-newline --type text/uri-list": "# comment\r\nfile:///home/me/Car%20Care.pdf\r\nfile:///home/me/other.pdf\r\n",
	}, &calls))
	got, err := c.File(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "/home/me/Car Care.pdf", got, "the first file")

	none := linux(map[string]string{"WAYLAND_DISPLAY": "wayland-0"}, []string{"wl-paste"}, fakeRun(map[string]string{
		"wl-paste --list-types": "text/plain\n",
	}, &calls))
	_, err = none.File(context.Background())
	assert.ErrorIs(t, err, ErrNoFile)
}
