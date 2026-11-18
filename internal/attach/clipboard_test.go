package attach

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// call is one command a fake runner was asked to run.
type call struct{ name, args string }

// fakeRun answers commands from replies, keyed by "name args", and records
// every call. A missing key is an error, like a command that fails.
func fakeRun(replies map[string]string, calls *[]call) Runner {
	return func(_ context.Context, name string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		*calls = append(*calls, call{name, joined})
		out, ok := replies[name+" "+joined]
		if !ok {
			return nil, errors.New("exit status 1")
		}
		return []byte(out), nil
	}
}

func linux(env map[string]string, tools []string, run Runner) Clipboard {
	return Clipboard{
		GOOS:   "linux",
		Getenv: func(k string) string { return env[k] },
		LookPath: func(name string) (string, error) {
			for _, t := range tools {
				if t == name {
					return "/usr/bin/" + name, nil
				}
			}
			return "", errors.New("not found")
		},
		Run: run,
	}
}

func TestImageOnWayland(t *testing.T) {
	var calls []call
	c := linux(map[string]string{"WAYLAND_DISPLAY": "wayland-0", "DISPLAY": ":0"}, []string{"wl-paste", "xclip"}, fakeRun(map[string]string{
		"wl-paste --list-types":                       "text/plain\nimage/jpeg\nimage/png\n",
		"wl-paste --no-newline --type image/png":      "PNGBYTES",
		"wl-paste --no-newline --type image/jpeg":     "JPEGBYTES",
		"xclip -selection clipboard -t TARGETS -o":    "never used",
		"xclip -selection clipboard -t image/png -o":  "never used",
		"xclip -selection clipboard -t image/jpeg -o": "never used",
	}, &calls))
	got, err := c.Image(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "PNGBYTES", string(got), "PNG is preferred when several image types are offered")
	for _, cl := range calls {
		assert.Equal(t, "wl-paste", cl.name, "xclip is not touched on Wayland")
	}
}

func TestImageOnX11(t *testing.T) {
	var calls []call
	c := linux(map[string]string{"DISPLAY": ":0"}, []string{"xclip"}, fakeRun(map[string]string{
		"xclip -selection clipboard -t TARGETS -o":    "TARGETS\nimage/webp\nUTF8_STRING\n",
		"xclip -selection clipboard -t image/webp -o": "WEBPBYTES",
	}, &calls))
	got, err := c.Image(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "WEBPBYTES", string(got), "any supported image type is read, and converted later")
}

func TestImageFailuresOnLinux(t *testing.T) {
	text := map[string]string{"wl-paste --list-types": "text/plain\n"}
	tests := []struct {
		name  string
		env   map[string]string
		tools []string
		reply map[string]string
		want  error
	}{
		{"no image on the clipboard", map[string]string{"WAYLAND_DISPLAY": "w"}, []string{"wl-paste"}, text, ErrNoImage},
		{"wl-paste not installed", map[string]string{"WAYLAND_DISPLAY": "w"}, nil, nil, ErrNoTool},
		{"xclip not installed", map[string]string{"DISPLAY": ":0"}, nil, nil, ErrNoTool},
		{"no graphical session", map[string]string{}, []string{"wl-paste", "xclip"}, nil, ErrClipboard},
		{"the tool fails", map[string]string{"WAYLAND_DISPLAY": "w"}, []string{"wl-paste"}, map[string]string{}, ErrClipboard},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls []call
			_, err := linux(tt.env, tt.tools, fakeRun(tt.reply, &calls)).Image(context.Background())
			require.ErrorIs(t, err, tt.want)
		})
	}
}

func TestWaylandSessionFallsBackToXclip(t *testing.T) {
	var calls []call
	c := linux(map[string]string{"WAYLAND_DISPLAY": "w", "DISPLAY": ":0"}, []string{"xclip"}, fakeRun(map[string]string{
		"xclip -selection clipboard -t TARGETS -o":   "image/png\n",
		"xclip -selection clipboard -t image/png -o": "PNGBYTES",
	}, &calls))
	got, err := c.Image(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "PNGBYTES", string(got))
}

func TestImageOnMac(t *testing.T) {
	var calls []call
	encoded := base64.StdEncoding.EncodeToString([]byte("PNGBYTES")) + "\n"
	run := func(_ context.Context, name string, args ...string) ([]byte, error) {
		calls = append(calls, call{name, fmt.Sprint(args[:2])})
		return []byte(encoded), nil
	}
	c := Clipboard{GOOS: "darwin", Run: run}
	got, err := c.Image(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "PNGBYTES", string(got))
	require.Len(t, calls, 1)
	assert.Equal(t, "osascript", calls[0].name)
	assert.Equal(t, "[-l JavaScript]", calls[0].args)
}

func TestImageOnMacFailures(t *testing.T) {
	tests := []struct {
		name string
		out  string
		err  error
		want error
	}{
		{"no image on the clipboard", "\n", nil, ErrNoImage},
		{"osascript fails", "", errors.New("exit status 1"), ErrClipboard},
		{"output is not base64", "%%%not base64%%%", nil, ErrClipboard},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run := func(context.Context, string, ...string) ([]byte, error) { return []byte(tt.out), tt.err }
			_, err := Clipboard{GOOS: "darwin", Run: run}.Image(context.Background())
			require.ErrorIs(t, err, tt.want)
		})
	}
}

func TestImageOnUnsupportedSystem(t *testing.T) {
	_, err := Clipboard{GOOS: "plan9"}.Image(context.Background())
	require.ErrorIs(t, err, ErrClipboard)
}

func TestErrorsNeverShowClipboardContent(t *testing.T) {
	const secret = "SECRET-CLIPBOARD-DATA"
	run := func(context.Context, string, ...string) ([]byte, error) {
		return []byte(secret), errors.New("failed: " + secret)
	}
	_, err := linux(map[string]string{"WAYLAND_DISPLAY": "w"}, []string{"wl-paste"}, run).Image(context.Background())
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "SECRET")
}
