package browser

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommand(t *testing.T) {
	tests := []struct {
		goos string
		name string
		ok   bool
	}{
		{"darwin", "open", true},
		{"linux", "xdg-open", true},
		{"freebsd", "xdg-open", true},
		{"windows", "", false},
		{"plan9", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.goos, func(t *testing.T) {
			name, args, err := command(tt.goos, "https://example.com/a?b=1")
			if !tt.ok {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.name, name)
			assert.Equal(t, []string{"https://example.com/a?b=1"}, args, "the address is one argument, never a shell string")
		})
	}
}

func TestCommandRefusesAnythingButWebAddresses(t *testing.T) {
	for _, raw := range []string{"-a Calculator", "file:///etc/passwd", "javascript:alert(1)", "", "https://x.com/ y", "ftp://x.com"} {
		_, _, err := command("darwin", raw)
		assert.Error(t, err, raw)
	}
}

func TestFileCommand(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "pic.png")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o600))

	tests := []struct {
		name, goos, path, want string
		ok                     bool
	}{
		{"darwin", "darwin", file, "open", true},
		{"linux", "linux", file, "xdg-open", true},
		{"windows", "windows", file, "", false},
		{"relative", "darwin", "pic.png", "", false},
		{"option", "darwin", "-a Calculator", "", false},
		{"missing", "darwin", filepath.Join(dir, "none.png"), "", false},
		{"directory", "darwin", dir, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, args, err := fileCommand(tt.goos, tt.path)
			if !tt.ok {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, name)
			assert.Equal(t, []string{tt.path}, args)
		})
	}
}
