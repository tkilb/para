package attach

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tkilb/para/internal/imgfmt"
)

const (
	svgDoc    = `<svg xmlns="http://www.w3.org/2000/svg" width="4" height="4"><rect width="4" height="4"/></svg>`
	svgScript = `<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`
	svgBroken = `<svg xmlns="http://www.w3.org/2000/svg"><rect `
)

func TestImportSVGFiles(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		content []byte
		stored  string
	}{
		{"an svg file", "Logo.svg", []byte(svgDoc), "projects/alpha/Logo.svg"},
		{"upper case extension", "Logo.SVG", []byte(svgDoc), "projects/alpha/Logo.svg"},
		{"a png name that holds svg", "fake.png", []byte(svgDoc), "projects/alpha/fake.svg"},
		{"an svg name that holds png", "real.svg", nil, "projects/alpha/real.png"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, inbox := intakeRoot(t)
			content := tt.content
			if content == nil {
				content = tinyPNG(t)
			}
			put(t, filepath.Join(inbox, tt.file), content)
			res, handled, err := Import(context.Background(), newStoreAt(root), fakeSource{}, "projects/alpha", filepath.Join(inbox, tt.file), inbox, when)
			require.NoError(t, err)
			assert.True(t, handled)
			assert.Equal(t, tt.stored, res.Rel)
			got, err := os.ReadFile(filepath.Join(root, res.Rel))
			require.NoError(t, err)
			assert.Equal(t, content, got, "the bytes are stored as they came")
		})
	}
}

func TestImportRefusesAnUnsafeSVGFile(t *testing.T) {
	root, inbox := intakeRoot(t)
	path := filepath.Join(inbox, "evil.svg")
	put(t, path, []byte(svgScript))
	_, handled, err := Import(context.Background(), newStoreAt(root), fakeSource{}, "projects/alpha", path, inbox, when)
	require.ErrorIs(t, err, imgfmt.ErrUnsafe)
	assert.True(t, handled, "it never falls through to the text import")
	assert.NotContains(t, err.Error(), inbox)
	entries, err := os.ReadDir(filepath.Join(root, "projects", "alpha"))
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestImportSVGFromClipboardText(t *testing.T) {
	root, inbox := intakeRoot(t)
	text := "\n  " + svgDoc + "\n"
	res, handled, err := Import(context.Background(), newStoreAt(root), fakeSource{err: ErrNoImage, text: text}, "projects/alpha", "", inbox, when)
	require.NoError(t, err)
	assert.True(t, handled)
	assert.Equal(t, "projects/alpha/2026-11-18 09-05-07.svg", res.Rel)
	got, err := os.ReadFile(filepath.Join(root, res.Rel))
	require.NoError(t, err)
	assert.Equal(t, text, string(got), "the text is stored unchanged")
}

func TestImportSVGFromClipboardImageData(t *testing.T) {
	root, inbox := intakeRoot(t)
	res, handled, err := Import(context.Background(), newStoreAt(root), fakeSource{data: []byte(svgDoc), text: "ignored"}, "projects/alpha", "", inbox, when)
	require.NoError(t, err)
	assert.True(t, handled)
	assert.Equal(t, "projects/alpha/2026-11-18 09-05-07.svg", res.Rel)
}

func TestImportClipboardTextThatIsNotAnSVGIsNotHandled(t *testing.T) {
	tests := map[string]string{
		"plain text":            "buy milk",
		"empty":                 "",
		"html":                  "<html><body><svg xmlns=\"http://www.w3.org/2000/svg\"/></body></html>",
		"an svg without a name": `<svg><rect/></svg>`,
		"other xml":             `<?xml version="1.0"?><note>svg</note>`,
		"broken markup":         "<div><svg",
	}
	for name, text := range tests {
		t.Run(name, func(t *testing.T) {
			root, inbox := intakeRoot(t)
			_, handled, err := Import(context.Background(), newStoreAt(root), fakeSource{err: ErrNoImage, text: text}, "projects/alpha", "", inbox, when)
			require.NoError(t, err)
			assert.False(t, handled)
		})
	}
	root, inbox := intakeRoot(t)
	_, handled, err := Import(context.Background(), newStoreAt(root), fakeSource{err: ErrNoImage, textErr: errors.New("boom")}, "projects/alpha", "", inbox, when)
	require.NoError(t, err)
	assert.False(t, handled, "an unreadable text clipboard is left to the text import")
}

func TestImportClipboardTextSVGThatIsRefused(t *testing.T) {
	tests := []struct {
		name string
		text string
		want error
	}{
		{"unsafe", svgScript, imgfmt.ErrUnsafe},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, inbox := intakeRoot(t)
			_, handled, err := Import(context.Background(), newStoreAt(root), fakeSource{err: ErrNoImage, text: tt.text}, "projects/alpha", "", inbox, when)
			require.ErrorIs(t, err, tt.want)
			assert.True(t, handled)
			assert.NotContains(t, err.Error(), "alert")
			entries, rerr := os.ReadDir(filepath.Join(root, "projects", "alpha"))
			require.NoError(t, rerr)
			assert.Empty(t, entries)
		})
	}
}

func TestProbeSVG(t *testing.T) {
	_, inbox := intakeRoot(t)
	put(t, filepath.Join(inbox, "logo.svg"), []byte(svgDoc))
	put(t, filepath.Join(inbox, "evil.svg"), []byte(svgScript))
	put(t, filepath.Join(inbox, "odd.png"), []byte(svgDoc))
	put(t, filepath.Join(inbox, "text.svg"), []byte("hello"))

	files := []struct {
		input string
		want  string
		ok    bool
	}{
		{"logo.svg", savedAs("SVG file"), true},
		{"odd.png", savedAs("SVG file"), true},
		{"evil.svg", "SVG file, not safe to store", true},
		{"text.svg", "", false},
	}
	for _, tt := range files {
		got, ok := ProbeFile(tt.input, inbox)
		assert.Equal(t, tt.ok, ok, tt.input)
		assert.Equal(t, tt.want, got, tt.input)
	}

	clips := []struct {
		name string
		src  fakeSource
		want string
		ok   bool
	}{
		{"text", fakeSource{err: ErrNoImage, text: svgDoc}, savedAs("SVG from the clipboard"), true},
		{"image data", fakeSource{data: []byte(svgDoc)}, savedAs("SVG from the clipboard"), true},
		{"unsafe", fakeSource{err: ErrNoImage, text: svgScript}, "SVG from the clipboard, not safe to store", true},
		{"damaged", fakeSource{err: ErrNoImage, text: svgBroken}, "", false},
		{"plain text", fakeSource{err: ErrNoImage, text: "hello"}, "", false},
	}
	for _, tt := range clips {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ProbeClipboard(context.Background(), tt.src)
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestProbeFileReadsAtMostMaxBytes(t *testing.T) {
	_, inbox := intakeRoot(t)
	big := svgDoc[:len(svgDoc)-6] + strings.Repeat(" ", imgfmt.MaxBytes) + "</svg>"
	put(t, filepath.Join(inbox, "big.svg"), []byte(big))
	got, ok := ProbeFile("big.svg", inbox)
	assert.True(t, ok)
	assert.Equal(t, "SVG file, too large", got)
}

// savedAs is the hint for the valid SVG svgDoc.
func savedAs(label string) string {
	return fmt.Sprintf("%s, %d bytes, saved as is", label, len(svgDoc))
}
