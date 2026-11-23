package intake

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tkilb/para/internal/store"
)

func TestParseNames(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"empty", "", nil},
		{"blank", "  , ,  ", nil},
		{"one", "Tyler Kilburn", []string{"Tyler Kilburn"}},
		{"kept as typed", "Tyler K., amber d", []string{"Tyler K.", "amber d"}},
		{"trimmed", "  Ann Lee  ,Bo Ray ", []string{"Ann Lee", "Bo Ray"}},
		{"duplicates ignoring case", "Ann Lee, ann lee, Bo", []string{"Ann Lee", "Bo"}},
		{"inner spaces kept", "Mary Jane  Watson", []string{"Mary Jane Watson"}},
		{"control characters removed", "Ann\tLee\nSmith", []string{"Ann Lee Smith"}},
		{"yaml punctuation survives", `O'Brien: "Pat", Lee [Jr]`, []string{`O'Brien: "Pat"`, "Lee [Jr]"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ParseNames(tt.input))
		})
	}
}

func TestCaptureInWithPeopleWritesThem(t *testing.T) {
	_, s := newStore(t)
	people := []string{"Tyler K.", `O'Brien: "Pat"`}
	res, err := CaptureInWithPeople(s, time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC), "", store.KindIdea, SourceAdhoc, "Idea", "body", nil, people)
	require.NoError(t, err)
	note, err := s.ReadNote(res.Rel)
	require.NoError(t, err)
	require.NoError(t, note.FrontmatterErr)
	got := note.Frontmatter.Get("people")
	require.NotNil(t, got)
	require.Len(t, got.Content, 2)
	assert.Equal(t, "Tyler K.", got.Content[0].Value)
	assert.Equal(t, `O'Brien: "Pat"`, got.Content[1].Value)
}
