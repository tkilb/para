package attach

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTextOnMac(t *testing.T) {
	var calls []call
	c := Clipboard{GOOS: "darwin", Run: fakeRun(map[string]string{"pbpaste -Prefer txt": "<svg/>\n"}, &calls)}
	got, err := c.Text(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "<svg/>\n", got, "the text is returned as it is")

	c.Run = fakeRun(nil, &calls)
	_, err = c.Text(context.Background())
	assert.ErrorIs(t, err, ErrClipboard)
}

func TestTextOnLinux(t *testing.T) {
	wl := map[string]string{"WAYLAND_DISPLAY": "w"}
	x11 := map[string]string{"DISPLAY": ":0"}
	tests := []struct {
		name  string
		env   map[string]string
		tools []string
		reply map[string]string
		want  string
		err   error
	}{
		{"wayland utf-8", wl, []string{"wl-paste"}, map[string]string{
			"wl-paste --list-types":                                 "text/plain\ntext/plain;charset=utf-8\n",
			"wl-paste --no-newline --type text/plain;charset=utf-8": "UTF",
			"wl-paste --no-newline --type text/plain":               "PLAIN",
		}, "UTF", nil},
		{"x11", x11, []string{"xclip"}, map[string]string{
			"xclip -selection clipboard -t TARGETS -o":     "TARGETS\nUTF8_STRING\n",
			"xclip -selection clipboard -t UTF8_STRING -o": "X11TEXT",
		}, "X11TEXT", nil},
		{"only an image", wl, []string{"wl-paste"}, map[string]string{"wl-paste --list-types": "image/png\n"}, "", ErrNoText},
		{"no tool", wl, nil, nil, "", ErrNoTool},
		{"tool fails", wl, []string{"wl-paste"}, map[string]string{}, "", ErrClipboard},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls []call
			got, err := linux(tt.env, tt.tools, fakeRun(tt.reply, &calls)).Text(context.Background())
			if tt.err != nil {
				assert.ErrorIs(t, err, tt.err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestTextOnAnUnknownSystem(t *testing.T) {
	_, err := Clipboard{GOOS: "plan9"}.Text(context.Background())
	assert.ErrorIs(t, err, ErrClipboard)
	_, err = Clipboard{GOOS: "darwin", Run: func(context.Context, string, ...string) ([]byte, error) {
		return nil, errors.New("x")
	}}.Text(context.Background())
	assert.ErrorIs(t, err, ErrClipboard)
}

func TestImageReadsSVGData(t *testing.T) {
	var calls []call
	c := linux(map[string]string{"WAYLAND_DISPLAY": "w"}, []string{"wl-paste"}, fakeRun(map[string]string{
		"wl-paste --list-types":                      "text/plain\nimage/svg+xml\n",
		"wl-paste --no-newline --type image/svg+xml": "<svg/>",
	}, &calls))
	got, err := c.Image(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "<svg/>", string(got))
}
