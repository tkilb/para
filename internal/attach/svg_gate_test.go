package attach

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tkilb/para/internal/imgfmt"
)

// countingSource counts how often the clipboard text is read.
type countingSource struct {
	fakeSource
	textCalls int
}

func (c *countingSource) Text(ctx context.Context) (string, error) {
	c.textCalls++
	return c.fakeSource.Text(ctx)
}

func TestClipboardTextGateMatchesPrepare(t *testing.T) {
	longComment := "<!--" + strings.Repeat("x", 2<<10) + "-->"
	tests := []struct {
		name    string
		text    string
		handled bool
		err     error
		hint    string
	}{
		{"leading BOM", "\ufeff" + svgDoc, true, nil, "valid"},
		{"leading space", " \n\t" + svgDoc, true, nil, "valid"},
		{"leading comment", "<!-- note -->" + svgDoc, true, nil, "valid"},
		{"declaration and a long comment", `<?xml version="1.0"?>` + longComment + svgDoc, true, nil, "valid"},
		{"upper case root", strings.ToUpper(svgDoc), false, nil, ""},
		{"html", "<html><body>hi</body></html>", false, nil, ""},
		{"malformed", svgBroken, false, nil, ""},
		{"no namespace", `<svg><rect/></svg>`, false, nil, ""},
		{"script", svgScript, true, imgfmt.ErrUnsafe, "SVG from the clipboard, not safe to store"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, inbox := intakeRoot(t)
			src := fakeSource{err: ErrNoImage, text: tt.text}
			_, handled, err := Import(context.Background(), newStoreAt(root), src, "projects/alpha", "", inbox, when)
			assert.Equal(t, tt.handled, handled)
			if tt.err != nil {
				assert.ErrorIs(t, err, tt.err)
			} else {
				assert.NoError(t, err)
			}
			entries, rerr := os.ReadDir(filepath.Join(root, "projects", "alpha"))
			require.NoError(t, rerr)
			if tt.handled && tt.err == nil {
				assert.Len(t, entries, 1)
			} else {
				assert.Empty(t, entries, "nothing is written")
			}
			hint, ok := ProbeClipboard(context.Background(), src)
			if tt.hint == "valid" {
				tt.hint = fmt.Sprintf("SVG from the clipboard, %d bytes, saved as is", len(tt.text))
			}
			assert.Equal(t, tt.hint != "", ok)
			assert.Equal(t, tt.hint, hint)
		})
	}
}

func TestClipboardTextIsNotReadWhenAnImageIsThere(t *testing.T) {
	root, inbox := intakeRoot(t)
	src := &countingSource{fakeSource: fakeSource{data: []byte(svgDoc), text: "x"}}
	_, handled, err := Import(context.Background(), newStoreAt(root), src, "projects/alpha", "", inbox, when)
	require.NoError(t, err)
	assert.True(t, handled)
	assert.Zero(t, src.textCalls)
}

func TestClipboardTextErrorIsNotHandled(t *testing.T) {
	root, inbox := intakeRoot(t)
	src := fakeSource{err: ErrNoImage, textErr: errors.New("boom")}
	_, handled, err := Import(context.Background(), newStoreAt(root), src, "projects/alpha", "", inbox, when)
	require.NoError(t, err)
	assert.False(t, handled)
}

func TestProbeFileNoNamespace(t *testing.T) {
	_, inbox := intakeRoot(t)
	put(t, filepath.Join(inbox, "bare.svg"), []byte(`<svg><rect/></svg>`))
	got, ok := ProbeFile("bare.svg", inbox)
	assert.True(t, ok)
	assert.Equal(t, "SVG file, no SVG namespace", got)
}

func TestProbeFileIgnoresDirectoriesAndPipes(t *testing.T) {
	_, inbox := intakeRoot(t)
	require.NoError(t, os.Mkdir(filepath.Join(inbox, "x.svg"), 0o755))
	fifo := filepath.Join(inbox, "pipe.svg")
	if err := syscall.Mkfifo(fifo, 0o644); err != nil {
		t.Skipf("no fifo: %v", err)
	}
	for _, name := range []string{"x.svg", "pipe.svg"} {
		done := make(chan struct{})
		go func() {
			defer close(done)
			got, ok := ProbeFile(name, inbox)
			assert.False(t, ok, name)
			assert.Empty(t, got, name)
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("ProbeFile blocked on %s", name)
		}
	}
}

func TestProbeFileDefersAnSVGOverTheProbeCap(t *testing.T) {
	_, inbox := intakeRoot(t)
	pad := probeMaxBytes + 1 - len(svgDoc)
	big := svgDoc[:len(svgDoc)-6] + strings.Repeat(" ", pad) + "</svg>"
	require.Equal(t, probeMaxBytes+1, len(big))
	put(t, filepath.Join(inbox, "big.svg"), []byte(big))
	got, ok := ProbeFile("big.svg", inbox)
	assert.True(t, ok)
	assert.Equal(t, "SVG file, 4194305 bytes, checked on import", got)

	pad--
	atCap := svgDoc[:len(svgDoc)-6] + strings.Repeat(" ", pad) + "</svg>"
	put(t, filepath.Join(inbox, "cap.svg"), []byte(atCap))
	got, _ = ProbeFile("cap.svg", inbox)
	assert.Contains(t, got, "saved as is", "a file at the cap is still checked")
}

func TestSymlinkedSVGIsFollowedAndKeepsTheLinkName(t *testing.T) {
	root, inbox := intakeRoot(t)
	target := filepath.Join(t.TempDir(), "real.svg")
	put(t, target, []byte(svgDoc))
	link := filepath.Join(inbox, "alias.svg")
	require.NoError(t, os.Symlink(target, link))
	hint, ok := ProbeFile("alias.svg", inbox)
	assert.True(t, ok)
	assert.Equal(t, savedAs("SVG file"), hint)
	res, handled, err := Import(context.Background(), newStoreAt(root), fakeSource{}, "projects/alpha", link, inbox, when)
	require.NoError(t, err)
	assert.True(t, handled)
	assert.Equal(t, "projects/alpha/alias.svg", res.Rel)
}
