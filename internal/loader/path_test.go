package loader

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tree makes files (a name ending in / is a folder) under a new directory.
func tree(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range names {
		p := filepath.Join(dir, n)
		if strings.HasSuffix(n, "/") {
			require.NoError(t, os.MkdirAll(p, 0o755))
			continue
		}
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	}
	return dir
}

func TestResolveFile(t *testing.T) {
	inbox := tree(t, "standup.md", "Zoom Call.txt", "notes/", "notes.v2.md")
	cwd := tree(t, "local.txt")
	t.Chdir(cwd)

	tests := []struct {
		name, in, want string
	}{
		{"a bare name that is in the inbox", "Zoom Call.txt", filepath.Join(inbox, "Zoom Call.txt")},
		{"a bare name without its extension", "standup", filepath.Join(inbox, "standup.md")},
		{"the extension is matched ignoring case", "STANDUP", filepath.Join(inbox, "standup.md")},
		{"an exact name wins over one with an extension", "notes.v2.md", filepath.Join(inbox, "notes.v2.md")},
		{"a folder in the inbox is not a file", "notes", "notes"},
		{"a bare name not in the inbox is left as typed", "local.txt", "local.txt"},
		{"a missing bare name is left as typed", "nothing", "nothing"},
		{"a path is left as typed", "sub/standup.md", "sub/standup.md"},
		{"an absolute path is left as typed", "/tmp/standup.md", "/tmp/standup.md"},
		{"a home path is left as typed", "~/standup.md", "~/standup.md"},
		{"an empty name", "", ""},
		{"padding is trimmed", "  standup  ", filepath.Join(inbox, "standup.md")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ResolveFile(tt.in, inbox))
		})
	}
}

func TestResolveFileWithNoInbox(t *testing.T) {
	assert.Equal(t, "standup", ResolveFile("standup", ""))
	assert.Equal(t, "standup", ResolveFile("standup", filepath.Join(t.TempDir(), "missing")))
}

func TestResolveFileTwoInboxFilesWithTheSameStemAreNotGuessed(t *testing.T) {
	inbox := tree(t, "call.md", "call.txt")
	assert.Equal(t, "call", ResolveFile("call", inbox), "ambiguous: the user must say which")
}

func TestComplete(t *testing.T) {
	home := tree(t, "Downloads/", "Documents/", "Desktop/", "notes.txt", ".hidden", "Downloads/meeting one.txt", "Downloads/meeting two.txt", "Downloads/Zoom.vtt")
	t.Setenv("HOME", home)
	inbox := tree(t, "standup 2026-11-03.md", "standup 2026-11-04.md", "Retro.md", "_daily/")

	tests := []struct {
		name        string
		in          string
		want        string
		wantMatches []string
	}{
		{"a unique file", "~/not", "~/notes.txt", []string{"notes.txt"}},
		{"a unique folder gets a slash", "~/Downl", "~/Downloads/", []string{"Downloads/"}},
		{"the shared start of several", "~/Do", "~/Do", []string{"Documents/", "Downloads/"}},
		{"the shared start grows", "~/Downloads/m", "~/Downloads/meeting ", []string{"meeting one.txt", "meeting two.txt"}},
		{"case is ignored and the real case is used", "~/downl", "~/Downloads/", []string{"Downloads/"}},
		{"inside a folder", "~/Downloads/", "~/Downloads/", []string{"meeting one.txt", "meeting two.txt", "Zoom.vtt"}},
		{"a bare home is the home folder", "~", "~/", []string{"Desktop/", "Documents/", "Downloads/", "notes.txt"}},
		{"hidden files stay hidden", "~/", "~/", []string{"Desktop/", "Documents/", "Downloads/", "notes.txt"}},
		{"hidden files show when asked for", "~/.h", "~/.hidden", []string{".hidden"}},
		{"nothing matches", "~/zzz", "~/zzz", nil},
		{"a folder that is not there", "~/nowhere/a", "~/nowhere/a", nil},
		{"a bare name completes in the inbox", "stand", "standup 2026-11-0", []string{"standup 2026-11-03.md", "standup 2026-11-04.md"}},
		{"a bare name, one match", "Ret", "Retro.md", []string{"Retro.md"}},
		{"a bare folder in the inbox", "_da", "_daily/", []string{"_daily/"}},
		{"empty input lists the inbox", "", "", []string{"_daily/", "Retro.md", "standup 2026-11-03.md", "standup 2026-11-04.md"}},
		{"an absolute path", filepath.Join(home, "Down"), filepath.Join(home, "Downloads") + "/", []string{"Downloads/"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, matches := Complete(tt.in, inbox)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.wantMatches, matches)
		})
	}
}

func TestCompleteNeverReadsOutsideWhatWasTyped(t *testing.T) {
	// A path with .. is only listed, never changed or followed beyond the OS.
	home := tree(t, "a/", "b.txt")
	t.Setenv("HOME", home)
	got, _ := Complete("~/a/../b", "")
	assert.Equal(t, "~/a/../b.txt", got)
}

func TestDescribe(t *testing.T) {
	inbox := tree(t, "standup.md")
	dir := tree(t, "small.txt", "folder/")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "kb.txt"), []byte(strings.Repeat("a", 42*1024)), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "mb.txt"), []byte(strings.Repeat("a", 3*1024*1024+512*1024)), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "big.txt"), []byte(strings.Repeat("a", MaxBytes+1)), 0o644))
	words := func(s string) func() (string, error) { return func() (string, error) { return s, nil } }

	tests := []struct {
		name, in string
		clip     func() (string, error)
		want     string
	}{
		{"the clipboard", "", words("one two  three\nfour"), "clipboard, 4 words"},
		{"one word", "  ", words("hello"), "clipboard, 1 word"},
		{"an empty clipboard", "", words("  \n"), "the clipboard is empty"},
		{"an unreadable clipboard", "", func() (string, error) { return "", errors.New("no pbpaste") }, "cannot read the clipboard"},
		{"a web address", "https://wiki.example.com/a/b?x=1", nil, "web page, wiki.example.com"},
		{"a bad web address", "https://", nil, "not a valid web address"},
		{"an address with a password", "https://user:pw@example.com/", nil, "not a valid web address"},
		{"a file in bytes", filepath.Join(dir, "small.txt"), nil, "file, 1 byte"},
		{"a file in kilobytes", filepath.Join(dir, "kb.txt"), nil, "file, 42 KB"},
		{"a file in megabytes", filepath.Join(dir, "mb.txt"), nil, "file, 3.5 MB"},
		{"a missing file", filepath.Join(dir, "none.txt"), nil, "file not found"},
		{"a folder", filepath.Join(dir, "folder"), nil, "a folder, not a file"},
		{"a file that is too large", filepath.Join(dir, "big.txt"), nil, "file is too large (the limit is 10 MB)"},
		{"an inbox file by name", "standup", nil, "file, 1 byte"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, Describe(tt.in, inbox, tt.clip))
		})
	}
}

func TestDescribeNeverShowsContent(t *testing.T) {
	read := func() (string, error) { return "SECRET-CLIPBOARD words here", nil }
	assert.NotContains(t, Describe("", "", read), "SECRET")
	assert.NotContains(t, Describe("https://secret-host.example.com/private", "", nil), "private")
}
