package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntakeNote(t *testing.T) {
	data, err := IntakeNote("call mum.md", KindIdea, "quick", "2026-10-30", "call mum\n", nil)
	require.NoError(t, err)
	n := ParseNote("call mum.md", data)
	require.NoError(t, n.FrontmatterErr)
	assert.Equal(t, KindIdea, n.Kind)
	assert.Equal(t, "quick", n.Frontmatter.Get("source").Value)
	assert.Equal(t, "2026-10-30", n.Frontmatter.Get("date").Value)
	assert.Equal(t, "call mum", n.Frontmatter.Get("title").Value)
	assert.NotEmpty(t, n.Frontmatter.Get("uid").Value)
	assert.NotNil(t, n.Frontmatter.Get("tags"), "every intake note has a tags property")
	assert.Empty(t, n.Frontmatter.Get("tags").Content)
	assert.Equal(t, "call mum\n", n.Body)
}

func TestIntakeNoteQuotesTextSoItStaysAString(t *testing.T) {
	data, err := IntakeNote("n.md", KindFact, "2026-01-01", "2026-10-30", "x\n", nil)
	require.NoError(t, err)
	n := ParseNote("n.md", data)
	assert.Equal(t, "2026-01-01", n.Frontmatter.Get("source").Value)
	assert.Equal(t, "!!str", n.Frontmatter.Get("source").Tag, "a date-looking source is not turned into a timestamp")
	assert.Equal(t, "!!timestamp", n.Frontmatter.Get("date").Tag, "a date is written unquoted, so Obsidian reads it as a date")
}

func TestIntakeNoteTagsAndUnsetKind(t *testing.T) {
	data, err := IntakeNote("n.md", KindUnset, "quick", "2026-10-30", "", []string{"work", "ideas/new"})
	require.NoError(t, err)
	n := ParseNote("n.md", data)
	require.NoError(t, n.FrontmatterErr)
	assert.Empty(t, n.Frontmatter.Get("kind").Value, "an unset kind writes an empty kind property")
	assert.Equal(t, KindUnset, n.Kind)
	tags := n.Frontmatter.Get("tags")
	require.NotNil(t, tags)
	require.Len(t, tags.Content, 2)
	assert.Equal(t, "work", tags.Content[0].Value)
	assert.Equal(t, "ideas/new", tags.Content[1].Value)
	assert.Empty(t, n.Body)
}

func TestExtractedNote(t *testing.T) {
	data, err := ExtractedNote("Point.md", KindFact, "extract", "2026-10-30", "the text\n", []string{"[[Origin]]"})
	require.NoError(t, err)
	n := ParseNote("Point.md", data)
	require.NoError(t, n.FrontmatterErr)
	assert.Equal(t, KindFact, n.Kind)
	assert.Equal(t, "extract", n.Frontmatter.Get("source").Value)
	rel := n.Frontmatter.Get("related")
	require.NotNil(t, rel)
	require.Len(t, rel.Content, 1)
	assert.Equal(t, "[[Origin]]", rel.Content[0].Value)
	assert.Equal(t, "the text\n", n.Body)
	assert.NotNil(t, n.Frontmatter.Get("tags"))

	plain, err := ExtractedNote("P.md", KindIdea, "daily 2026-10-29", "2026-10-30", "x\n", nil)
	require.NoError(t, err)
	assert.Empty(t, ParseNote("P.md", plain).Frontmatter.Get("related").Content, "no links means an empty related property")
	assert.Equal(t, "daily 2026-10-29", ParseNote("P.md", plain).Frontmatter.Get("source").Value)
}

func TestIntakeNoteWithPeople(t *testing.T) {
	data, err := IntakeNoteWithPeople("n.md", KindUnset, "meeting", "2026-10-30", "x\n", nil, []string{"Tyler K.", "Amber D."})
	require.NoError(t, err)
	n := ParseNote("n.md", data)
	require.NoError(t, n.FrontmatterErr)
	people := n.Frontmatter.Get("people")
	require.NotNil(t, people)
	require.Len(t, people.Content, 2)
	assert.Equal(t, "Tyler K.", people.Content[0].Value)
	assert.Equal(t, "Amber D.", people.Content[1].Value)
}

func TestIntakeNoteHasNoPeopleUnlessGiven(t *testing.T) {
	data, err := IntakeNote("n.md", KindUnset, "quick", "2026-10-30", "x\n", nil)
	require.NoError(t, err)
	n := ParseNote("n.md", data)
	people := n.Frontmatter.Get("people")
	require.NotNil(t, people, "the standard property is there")
	assert.Empty(t, people.Content)
}
