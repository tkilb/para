package imgpreview

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func solid(w, h int, c color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

func pngBytes(t *testing.T, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	require.NoError(t, png.Encode(&b, img))
	return b.Bytes()
}

func writePNG(t *testing.T, img image.Image) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pic.png")
	require.NoError(t, os.WriteFile(path, pngBytes(t, img), 0o644))
	return path
}

func open(t *testing.T, img image.Image) Picture {
	t.Helper()
	p, err := Open(writePNG(t, img))
	require.NoError(t, err)
	return p
}

var red = color.NRGBA{R: 255, A: 255}

func TestDetect(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want Protocol
	}{
		{"nothing known", nil, Blocks},
		{"kitty by window id", map[string]string{"KITTY_WINDOW_ID": "1"}, Kitty},
		{"kitty by TERM", map[string]string{"TERM": "xterm-kitty"}, Kitty},
		{"ghostty", map[string]string{"TERM_PROGRAM": "ghostty"}, Kitty},
		{"iterm2", map[string]string{"TERM_PROGRAM": "iTerm.app"}, ITerm2},
		{"wezterm stays on blocks", map[string]string{"TERM_PROGRAM": "WezTerm"}, Blocks},
		{"tmux hides the protocol", map[string]string{"KITTY_WINDOW_ID": "1", "TMUX": "/tmp/tmux"}, Blocks},
		{"override to iterm2", map[string]string{"PARA_IMAGE_PROTOCOL": "iterm2", "TMUX": "x"}, ITerm2},
		{"override to kitty", map[string]string{"PARA_IMAGE_PROTOCOL": "KITTY"}, Kitty},
		{"override to blocks", map[string]string{"PARA_IMAGE_PROTOCOL": "blocks", "KITTY_WINDOW_ID": "1"}, Blocks},
		{"unknown override is ignored", map[string]string{"PARA_IMAGE_PROTOCOL": "sixel"}, Blocks},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, Detect(func(k string) string { return tt.env[k] }))
		})
	}
}

func TestOpenRejects(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, data []byte) string {
		p := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(p, data, 0o644))
		return p
	}
	tests := []struct {
		name string
		path string
	}{
		{"missing file", filepath.Join(dir, "missing.png")},
		{"text in a png", write("text.png", []byte("not an image"))},
		{"a damaged png", write("bad.png", pngBytes(t, solid(8, 8, red))[:30])},
		{"an empty file", write("empty.png", nil)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Open(tt.path)
			require.Error(t, err)
			assert.NotContains(t, err.Error(), dir, "errors never show paths")
		})
	}
}

func TestCaption(t *testing.T) {
	p := open(t, solid(16, 12, red))
	assert.Regexp(t, `^16 x 12 px, \d+(\.\d)? (B|KB|MB)$`, p.Caption())
}

func TestBlocksFitTheBox(t *testing.T) {
	p := open(t, solid(200, 100, red))
	lines := p.Render(Blocks, 40, 10)
	require.NotEmpty(t, lines)
	assert.LessOrEqual(t, len(lines), 10)
	for _, l := range lines {
		assert.LessOrEqual(t, ansi.StringWidth(l), 40)
	}
	assert.Contains(t, lines[0], "\x1b[38;2;255;0;0m", "the top pixel is the foreground")
	assert.Contains(t, lines[0], "▀")
	assert.True(t, strings.HasSuffix(lines[0], "\x1b[0m"), "every row resets its colors")
}

func TestBlocksKeepAspectRatioAndNeverEnlarge(t *testing.T) {
	p := open(t, solid(4, 4, red))
	lines := p.Render(Blocks, 80, 40)
	assert.Len(t, lines, 2, "a 4 x 4 image is 4 columns by 2 rows, not stretched")
	assert.Equal(t, 4, ansi.StringWidth(lines[0]))

	wide := open(t, solid(400, 100, red))
	got := wide.Render(Blocks, 40, 40)
	assert.Equal(t, 40, ansi.StringWidth(got[0]))
	assert.Len(t, got, 5, "40 x 10 pixels is 5 rows")
}

func TestBlocksTransparentPixelsKeepTheBackground(t *testing.T) {
	clear := color.NRGBA{}
	img := solid(2, 2, clear)
	img.SetNRGBA(0, 0, red)
	p := open(t, img)
	lines := p.Render(Blocks, 10, 10)
	require.Len(t, lines, 1)
	assert.Contains(t, lines[0], "\x1b[38;2;255;0;0m", "the visible top pixel is drawn")
	assert.NotContains(t, lines[0], "48;2", "a transparent bottom pixel sets no background")
	assert.Contains(t, lines[0], "\x1b[49m▀", "and clears any background left by an earlier cell")
	assert.Contains(t, lines[0], "\x1b[49m ", "a fully transparent cell is a space")
}

var kittyHead = regexp.MustCompile(`^\x1b_Ga=T,f=100,t=d,i=\d+,c=(\d+),r=(\d+),C=1,q=2,m=([01]);`)

func TestKittyPlacesAScaledPNG(t *testing.T) {
	p := open(t, solid(2000, 1000, red))
	lines := p.Render(Kitty, 40, 10)
	require.NotEmpty(t, lines)
	m := kittyHead.FindStringSubmatch(lines[0])
	require.NotNil(t, m, "the first line starts the image: %q", lines[0][:min(60, len(lines[0]))])
	assert.LessOrEqual(t, atoi(t, m[1]), 40)
	assert.LessOrEqual(t, atoi(t, m[2]), 10)
	assert.Len(t, lines, atoi(t, m[2]), "one line per row the image covers")
	for _, l := range lines[1:] {
		assert.Empty(t, l, "the other rows stay blank so text never overlaps the image")
	}

	// The payload, put back together, is a PNG no bigger than the box.
	payload := kittyPayload(t, lines[0])
	cfg, err := png.DecodeConfig(bytes.NewReader(payload))
	require.NoError(t, err)
	assert.LessOrEqual(t, cfg.Width, 40*cellWidth)
	assert.LessOrEqual(t, cfg.Height, 10*cellHeight)
	assert.Equal(t, 2*cfg.Height, cfg.Width, "the aspect ratio is kept")
	assert.Equal(t, 1, strings.Count(lines[0], "m=0"), "the last chunk ends the image")
}

func TestKittyChunksAreSmall(t *testing.T) {
	// Noise does not compress, so the payload needs several chunks.
	img := image.NewNRGBA(image.Rect(0, 0, 300, 300))
	for i := range img.Pix {
		img.Pix[i] = byte(i*7 + i/13)
	}
	lines := open(t, img).Render(Kitty, 60, 30)
	for _, seq := range strings.Split(lines[0], "\x1b\\") {
		assert.LessOrEqual(t, len(seq), 4096+80, "a chunk stays under the protocol limit")
	}
	assert.Greater(t, strings.Count(lines[0], "\x1b_G"), 1)
}

func TestKittyCleanup(t *testing.T) {
	assert.Regexp(t, `^\x1b_Ga=d,d=I,i=\d+,q=2\x1b\\$`, Cleanup(Kitty))
	assert.Empty(t, Cleanup(Blocks))
	assert.Empty(t, Cleanup(ITerm2))
}

func TestITerm2PlacesAScaledPNG(t *testing.T) {
	p := open(t, solid(2000, 1000, red))
	lines := p.Render(ITerm2, 40, 10)
	require.NotEmpty(t, lines)
	re := regexp.MustCompile(`^\x1b\]1337;File=inline=1;size=\d+;width=(\d+);height=(\d+);preserveAspectRatio=1;doNotMoveCursor=1:([A-Za-z0-9+/=]+)\x07$`)
	m := re.FindStringSubmatch(lines[0])
	require.NotNil(t, m)
	assert.LessOrEqual(t, atoi(t, m[1]), 40)
	assert.LessOrEqual(t, atoi(t, m[2]), 10)
	data, err := base64.StdEncoding.DecodeString(m[3])
	require.NoError(t, err)
	_, err = png.Decode(bytes.NewReader(data))
	require.NoError(t, err)
	for _, l := range lines[1:] {
		assert.Empty(t, l)
	}
}

func TestRenderOfATinyBoxIsEmptyNotAPanic(t *testing.T) {
	p := open(t, solid(10, 10, red))
	for _, proto := range []Protocol{Blocks, Kitty, ITerm2} {
		assert.Empty(t, p.Render(proto, 0, 5))
		assert.Empty(t, p.Render(proto, 5, 0))
	}
}

func kittyPayload(t *testing.T, line string) []byte {
	t.Helper()
	var b64 strings.Builder
	for _, seq := range strings.Split(line, "\x1b\\") {
		if _, after, ok := strings.Cut(seq, ";"); ok {
			b64.WriteString(after)
		}
	}
	data, err := base64.StdEncoding.DecodeString(b64.String())
	require.NoError(t, err)
	return data
}

func atoi(t *testing.T, s string) int {
	t.Helper()
	n := 0
	for _, r := range s {
		require.True(t, r >= '0' && r <= '9')
		n = n*10 + int(r-'0')
	}
	return n
}
