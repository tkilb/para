package loader

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClassify(t *testing.T) {
	tests := []struct {
		name, in  string
		wantKind  Kind
		wantValue string
	}{
		{"empty means the clipboard", "", Clipboard, ""},
		{"white space means the clipboard", "  \t ", Clipboard, ""},
		{"https address", "https://example.com/a", Web, "https://example.com/a"},
		{"http address", "http://example.com", Web, "http://example.com"},
		{"scheme in capitals", "HTTPS://Example.com/A", Web, "HTTPS://Example.com/A"},
		{"address with padding", "  https://example.com/a  ", Web, "https://example.com/a"},
		{"a broken address is still a web import", "https://", Web, "https://"},
		{"home path", "~/Downloads/meeting.txt", File, "~/Downloads/meeting.txt"},
		{"absolute path", "/tmp/a b.txt", File, "/tmp/a b.txt"},
		{"padded path", "  notes.md ", File, "notes.md"},
		{"a bare name", "standup", File, "standup"},
		{"a name that starts with http", "httpd-notes.md", File, "httpd-notes.md"},
		{"single quoted path", "'/Users/me/Car Care Brief 11-20-26.pdf'", File, "/Users/me/Car Care Brief 11-20-26.pdf"},
		{"double quoted path", `"/Users/me/a b.pdf"`, File, "/Users/me/a b.pdf"},
		{"quoted path with padding", "  '/tmp/a b.pdf' ", File, "/tmp/a b.pdf"},
		{"escaped spaces", `/Users/me/Car\ Care\ Brief.pdf`, File, "/Users/me/Car Care Brief.pdf"},
		{"escaped brackets and parens", `/tmp/a\ \(1\)\ \[x\].pdf`, File, "/tmp/a (1) [x].pdf"},
		{"a quote that does not wrap the path is kept", "/tmp/it's.pdf", File, "/tmp/it's.pdf"},
		{"mismatched quotes are kept", "'/tmp/a.pdf\"", File, "'/tmp/a.pdf\""},
		{"a lone quote is kept", "'", File, "'"},
		{"a backslash before a letter is kept", `notes\todo.md`, File, `notes\todo.md`},
		{"another scheme is a file name", "ftp://host/file", File, "ftp://host/file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Classify(tt.in)
			assert.Equal(t, tt.wantKind, got.Kind)
			assert.Equal(t, tt.wantValue, got.Value)
		})
	}
}
