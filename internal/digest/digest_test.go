package digest

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testSources = []string{"adhoc", "confluence", "meeting", "slack", "web"}

func parse(t *testing.T, v any) (Digest, error) {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return Parse(data, testSources)
}

type obj = map[string]any

func note(title, body string, tags ...string) obj {
	return obj{"title": title, "body": body, "tags": tags}
}

func TestParseValid(t *testing.T) {
	d, err := parse(t, obj{
		"topic":  "Pricing review",
		"source": "meeting",
		"notes": []obj{
			note("Tier pricing", "Three tiers: basic, plus, pro.", "pricing", "tiers"),
			note("Renewal rules", "Renewals lock for a year."),
		},
		"actions": []string{"Send the price sheet"},
	})
	require.NoError(t, err)
	assert.Equal(t, "Pricing review", d.Topic)
	assert.Equal(t, "meeting", d.Source)
	require.Len(t, d.Notes, 2)
	assert.Equal(t, Note{Title: "Tier pricing", Body: "Three tiers: basic, plus, pro.", Tags: []string{"pricing", "tiers"}}, d.Notes[0])
	assert.Equal(t, "Renewal rules", d.Notes[1].Title)
	assert.Equal(t, []string{"Send the price sheet"}, d.Actions)
}

func TestParseFacts(t *testing.T) {
	d, err := parse(t, obj{"facts": []string{"  Renewals lock\nfor a year ", "", strings.Repeat("y", MaxFactRunes+50), "Tier\x1b[0m pro"}})
	require.NoError(t, err, "facts alone are not empty")
	assert.Empty(t, d.Notes)
	require.Len(t, d.Facts, 3)
	assert.Equal(t, "Renewals lock for a year", d.Facts[0])
	assert.Len(t, []rune(d.Facts[1]), MaxFactRunes)
	assert.Equal(t, "Tier[0m pro", d.Facts[2])

	var many []string
	for i := range MaxFacts + 5 {
		many = append(many, strings.Repeat("a", i+1))
	}
	d, err = parse(t, obj{"facts": many})
	require.NoError(t, err)
	assert.Len(t, d.Facts, MaxFacts)
}

func TestParseConclusions(t *testing.T) {
	d, err := parse(t, obj{"conclusions": []string{"  Renewals lock\nfor a year ", "", strings.Repeat("y", MaxConclusionRunes+50), "Tier\x1b[0m pro wins"}})
	require.NoError(t, err, "conclusions alone are not empty")
	assert.Empty(t, d.Notes)
	require.Len(t, d.Conclusions, 3)
	assert.Equal(t, "Renewals lock for a year", d.Conclusions[0])
	assert.Len(t, []rune(d.Conclusions[1]), MaxConclusionRunes)
	assert.Equal(t, "Tier[0m pro wins", d.Conclusions[2])

	var many []string
	for i := range MaxConclusions + 5 {
		many = append(many, strings.Repeat("a", i+1))
	}
	d, err = parse(t, obj{"conclusions": many})
	require.NoError(t, err)
	assert.Len(t, d.Conclusions, MaxConclusions)
}

func TestParseBlankConclusionsIsEmpty(t *testing.T) {
	_, err := parse(t, obj{"conclusions": []string{" "}})
	assert.ErrorIs(t, err, ErrEmpty)
}

func TestParseActionsOnlyIsNotEmpty(t *testing.T) {
	d, err := parse(t, obj{"notes": []obj{}, "actions": []string{"Call Sam"}})
	require.NoError(t, err)
	assert.Empty(t, d.Notes)
	assert.Equal(t, []string{"Call Sam"}, d.Actions)
}

func TestParseRejects(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want error
	}{
		{"not json", "Sure! Here are your notes", ErrBadJSON},
		{"empty input", "", ErrBadJSON},
		{"json array", `[1,2]`, ErrBadJSON},
		{"wrong field type", `{"notes": "text"}`, ErrBadJSON},
		{"trailing text", `{"notes":[{"title":"A","body":"b"}]} and more`, ErrBadJSON},
		{"two objects", `{"topic":"a"}{"topic":"b"}`, ErrBadJSON},
		{"no notes or actions", `{"topic":"x","notes":[],"actions":[]}`, ErrEmpty},
		{"only unusable notes", `{"notes":[{"title":"///","body":"b"},{"title":"T","body":"  "}]}`, ErrEmpty},
		{"too large", `{"topic":"` + strings.Repeat("a", MaxInputBytes) + `"}`, ErrTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.in), testSources)
			assert.ErrorIs(t, err, tt.want)
		})
	}
}

func TestParseErrorsNeverEchoTheInput(t *testing.T) {
	_, err := Parse([]byte(`{"notes": "SECRET-MEETING-TEXT"}`), testSources)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "SECRET")
	_, err = Parse([]byte("SECRET-MEETING-TEXT"), testSources)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "SECRET")
}

func TestParseIgnoresUnknownFields(t *testing.T) {
	d, err := parse(t, obj{"mood": "upbeat", "notes": []obj{{"title": "A", "body": "b", "color": "red"}}})
	require.NoError(t, err)
	assert.Len(t, d.Notes, 1)
}

func TestParseSource(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"known", "slack", "slack"},
		{"case folded to the configured spelling", "Confluence", "confluence"},
		{"padded", "  meeting ", "meeting"},
		{"unknown falls back", "carrier-pigeon", "adhoc"},
		{"missing falls back", "", "adhoc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, err := parse(t, obj{"source": tt.in, "notes": []obj{note("A", "b")}})
			require.NoError(t, err)
			assert.Equal(t, tt.want, d.Source)
		})
	}
}

func TestParseSourceWithNoConfiguredList(t *testing.T) {
	data, _ := json.Marshal(obj{"source": "slack", "notes": []obj{note("A", "b")}})
	d, err := Parse(data, nil)
	require.NoError(t, err)
	assert.Equal(t, "adhoc", d.Source)
}

func TestParseTitles(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"unsafe characters removed", `Q3/Q4: "plan"`, "Q3Q4 plan"},
		{"newline collapses", "Line one\nline two", "Line one line two"},
		{"escape and brackets stripped", "Hi\x1b[2J there", "Hi2J there"},
		{"path traversal is just text", "../../etc/passwd", "etcpasswd"},
		{"reserved source name", "_source", "source"},
		{"reserved actions name", "__actions", "actions"},
		{"cut to the title limit", strings.Repeat("a", 200), strings.Repeat("a", 80)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, err := parse(t, obj{"notes": []obj{note(tt.in, "body")}})
			require.NoError(t, err)
			require.Len(t, d.Notes, 1)
			assert.Equal(t, tt.want, d.Notes[0].Title)
		})
	}
}

func TestParseDuplicateTitlesGetASuffix(t *testing.T) {
	d, err := parse(t, obj{"notes": []obj{note("Pricing", "a"), note("pricing", "b"), note("PRICING", "c"), note("Other", "d")}})
	require.NoError(t, err)
	var titles []string
	for _, n := range d.Notes {
		titles = append(titles, n.Title)
	}
	assert.Equal(t, []string{"Pricing", "pricing 2", "PRICING 3", "Other"}, titles)
}

func TestParseBodies(t *testing.T) {
	d, err := parse(t, obj{"notes": []obj{
		note("A", "line\x1b[31m red\x00\n\nsecond"),
		note("B", strings.Repeat("x", MaxBodyRunes+500)),
	}})
	require.NoError(t, err)
	assert.Equal(t, "line[31m red\n\nsecond", d.Notes[0].Body)
	assert.Len(t, []rune(d.Notes[1].Body), MaxBodyRunes)
}

func TestParseTags(t *testing.T) {
	d, err := parse(t, obj{"notes": []obj{note("A", "b", "#pricing", "Pricing", "bad tag", "ok_tag", "123", "has,comma", "", "a/b")}})
	require.NoError(t, err)
	assert.Equal(t, []string{"pricing", "ok_tag", "a/b"}, d.Notes[0].Tags, "hash dropped, duplicates and invalid tags removed")
}

func TestParseCapsTagsNotesAndActions(t *testing.T) {
	var tags []string
	for i := range MaxTags + 5 {
		tags = append(tags, "t"+strings.Repeat("a", i+1))
	}
	var notes []obj
	for i := range MaxNotes + 5 {
		notes = append(notes, note("Note "+strings.Repeat("a", i+1), "body", tags...))
	}
	var actions []string
	for i := range MaxActions + 5 {
		actions = append(actions, "Do thing "+strings.Repeat("a", i+1))
	}
	d, err := parse(t, obj{"notes": notes, "actions": actions})
	require.NoError(t, err)
	assert.Len(t, d.Notes, MaxNotes)
	assert.Len(t, d.Notes[0].Tags, MaxTags)
	assert.Len(t, d.Actions, MaxActions)
}

func TestParseActions(t *testing.T) {
	d, err := parse(t, obj{"actions": []string{"  Call Sam\nthis week ", "", "  ", strings.Repeat("y", MaxActionRunes+50), "Call\x1b[0m Sam"}})
	require.NoError(t, err)
	require.Len(t, d.Actions, 3)
	assert.Equal(t, "Call Sam this week", d.Actions[0])
	assert.Len(t, []rune(d.Actions[1]), MaxActionRunes)
	assert.Equal(t, "Call[0m Sam", d.Actions[2])
}

func TestParseTopicIsCleanedAndMayBeEmpty(t *testing.T) {
	d, err := parse(t, obj{"topic": "Pricing/Review: Q3", "notes": []obj{note("A", "b")}})
	require.NoError(t, err)
	assert.Equal(t, "PricingReview Q3", d.Topic)

	d, err = parse(t, obj{"notes": []obj{note("A", "b")}})
	require.NoError(t, err)
	assert.Empty(t, d.Topic, "the writer falls back to the source name")
}

func TestFakeDigester(t *testing.T) {
	want := Digest{Topic: "T", Notes: []Note{{Title: "A", Body: "b"}}}
	f := &Fake{Result: want}
	var d Digester = f

	got, err := d.Digest(context.Background(), "some source text")
	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.Equal(t, []string{"some source text"}, f.Calls)

	boom := errors.New("backend down")
	f = &Fake{Err: boom}
	_, err = f.Digest(context.Background(), "x")
	assert.ErrorIs(t, err, boom)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = (&Fake{Result: want}).Digest(ctx, "x")
	assert.ErrorIs(t, err, context.Canceled, "a cancelled context wins")
}
