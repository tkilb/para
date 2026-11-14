package glossary

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBufferRoundTrip(t *testing.T) {
	tests := []Entry{
		{"NAM", "National Account Manager"},
		{"PARA", "Projects, Areas,\nResources, Archives\n\nand more"},
		{"blank", ""},
		{"", ""},
	}
	for _, e := range tests {
		got, err := ParseBuffer(Buffer(e))
		if e.Term == "" {
			assert.ErrorIs(t, err, ErrEmptyTerm, "an empty buffer for a new term has no term")
			continue
		}
		require.NoError(t, err)
		assert.Equal(t, e, got)
	}
}

func TestParseBuffer(t *testing.T) {
	tests := []struct {
		name, text string
		want       Entry
		wantErr    error
	}{
		{"labels and text", "Term: NAM\nDefinition: one\n", Entry{"NAM", "one"}, nil},
		{"definition on following lines", "Term: NAM\nDefinition:\nline one\nline two\n", Entry{"NAM", "line one\nline two"}, nil},
		{"first line and more", "Term: NAM\nDefinition: a\nb\n", Entry{"NAM", "a\nb"}, nil},
		{"spaces trimmed", "Term:   NAM  \nDefinition:   x  \n", Entry{"NAM", "x"}, nil},
		{"blank definition", "Term: NAM\nDefinition:\n", Entry{"NAM", ""}, nil},
		{"definition line deleted", "Term: NAM\n", Entry{"NAM", ""}, nil},
		{"a Term: line inside the definition stays", "Term: A\nDefinition: x\nTerm: B\n", Entry{"A", "x\nTerm: B"}, nil},
		{"text above the term is ignored", "# note\nTerm: A\nDefinition: x\n", Entry{"A", "x"}, nil},
		{"windows line ends", "Term: A\r\nDefinition: x\r\ny\r\n", Entry{"A", "x\ny"}, nil},
		{"empty term", "Term:\nDefinition: x\n", Entry{}, ErrEmptyTerm},
		{"empty buffer", "", Entry{}, ErrEmptyTerm},
		{"no term line", "Definition: x\n", Entry{}, ErrEmptyTerm},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseBuffer(tt.text)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSpaced(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"one line is unchanged", "one", "one"},
		{"empty", "", ""},
		{"first break is doubled", "a\nb\nc", "a\n\nb\nc"},
		{"only the first break", "a\nb\nc\nd", "a\n\nb\nc\nd"},
		{"an existing blank line stays", "a\n\nb\nc", "a\n\nb\nc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, Spaced(tt.in))
		})
	}
}
