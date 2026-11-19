package digest

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSplit(t *testing.T) {
	para := func(c string, n int) string { return strings.Repeat(c, n) }
	tests := []struct {
		name  string
		text  string
		limit int
		want  []string
	}{
		{"short text is one part", "a\n\nb", 100, []string{"a\n\nb"}},
		{"exactly the limit is one part", para("x", 10), 10, []string{para("x", 10)}},
		{"no limit is one part", "a\n\nb\n\nc", 0, []string{"a\n\nb\n\nc"}},
		{"cuts after the last blank line that fits",
			para("a", 10) + "\n\n" + para("b", 10) + "\n\n" + para("c", 10), 30,
			[]string{para("a", 10) + "\n\n" + para("b", 10) + "\n\n", para("c", 10)}},
		{"a turn is never cut when a blank line is near",
			"Sam: " + para("a", 8) + "\n\nAnna: " + para("b", 8) + "\n\nSam: " + para("c", 8), 20,
			[]string{"Sam: " + para("a", 8) + "\n\n", "Anna: " + para("b", 8) + "\n\n", "Sam: " + para("c", 8)}},
		{"windows line endings", para("a", 10) + "\r\n\r\n" + para("b", 10), 16,
			[]string{para("a", 10) + "\r\n\r\n", para("b", 10)}},
		{"falls back to a line break",
			para("a", 10) + "\n" + para("b", 10) + "\n" + para("c", 10), 25,
			[]string{para("a", 10) + "\n" + para("b", 10) + "\n", para("c", 10)}},
		{"a very early blank line is skipped for a later line break",
			"a\n\n" + para("b", 12) + "\n" + para("c", 12), 20,
			[]string{"a\n\n" + para("b", 12) + "\n", para("c", 12)}},
		{"a white space tail joins the part before it", para("a", 10) + "\n\n" + "   \n", 12,
			[]string{para("a", 10) + "\n\n" + "   \n"}},
		{"trailing white space joins the last part", para("a", 10) + "\n\n" + para("b", 10) + "\n\n\n", 22,
			[]string{para("a", 10) + "\n\n", para("b", 10) + "\n\n\n"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Split(tt.text, tt.limit)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.text, strings.Join(got, ""), "no text is lost or changed")
		})
	}
}

func TestSplitWithNoBoundaryCutsOnACharacter(t *testing.T) {
	text := strings.Repeat("🙂", 50) // 200 bytes, no break at all
	parts := Split(text, 30)
	assert.Equal(t, text, strings.Join(parts, ""))
	for _, p := range parts {
		assert.LessOrEqual(t, len(p), 30)
		assert.NotEmpty(t, p)
		assert.True(t, strings.ToValidUTF8(p, "") == p, "a character is never cut in half")
	}
	assert.Greater(t, len(parts), 1)
}

func TestSplitPartsFitTheLimit(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 200; i++ {
		b.WriteString("Speaker: some words said in turn number " + strings.Repeat("x", i%17) + "\n\n")
	}
	text := b.String()
	parts := Split(text, 500)
	assert.Equal(t, text, strings.Join(parts, ""))
	for _, p := range parts {
		assert.LessOrEqual(t, len(p), 500)
		assert.NotEmpty(t, strings.TrimSpace(p))
	}
}

// scripted is a Digester that returns one prepared answer per call.
type scripted struct {
	answers []Digest
	errs    []error
	calls   []string
}

func (s *scripted) Digest(ctx context.Context, source string) (Digest, error) {
	if err := ctx.Err(); err != nil {
		return Digest{}, err
	}
	i := len(s.calls)
	s.calls = append(s.calls, source)
	if i < len(s.errs) && s.errs[i] != nil {
		return Digest{}, s.errs[i]
	}
	if i < len(s.answers) {
		return s.answers[i], nil
	}
	return Digest{}, errors.New("unexpected call")
}

// merger is a Merger that records what it is given.
type merger struct {
	result Digest
	err    error
	got    [][]Digest
}

func (m *merger) Merge(_ context.Context, parts []Digest) (Digest, error) {
	m.got = append(m.got, parts)
	return m.result, m.err
}

func part(topic, title string, actions ...string) Digest {
	return Digest{Topic: topic, Source: "meeting", Notes: []Note{{Title: title, Body: "body of " + title}}, Actions: actions}
}

const turn = "Sam: words\n\n"

// threeParts is a source that splits into three parts at a limit of 25.
func threeParts() (string, []string) {
	parts := []string{strings.Repeat(turn, 2), strings.Repeat(turn, 2), strings.Repeat(turn, 2)}
	return strings.Join(parts, ""), parts
}

func TestChunkedSmallSourceIsOnePass(t *testing.T) {
	p := &scripted{answers: []Digest{part("T", "A")}}
	m := &merger{}
	c := Chunked{Part: p, Merge: m, Limit: 1000}

	d, err := c.Digest(context.Background(), "a short source")
	require.NoError(t, err)
	assert.Equal(t, part("T", "A"), d)
	assert.Equal(t, []string{"a short source"}, p.calls)
	assert.Empty(t, m.got, "no merge for a source that fits")
}

func TestChunkedLargeSourceIsPartsThenAMerge(t *testing.T) {
	text, parts := threeParts()
	p := &scripted{answers: []Digest{part("One", "A"), part("Two", "B"), part("Three", "C")}}
	merged := Digest{Topic: "Merged", Source: "meeting", Notes: []Note{{Title: "All", Body: "everything"}}}
	m := &merger{result: merged}
	var progress [][2]int
	c := Chunked{Part: p, Merge: m, Limit: 25, OnPart: func(done, total int) { progress = append(progress, [2]int{done, total}) }}

	d, err := c.Digest(context.Background(), text)
	require.NoError(t, err)

	assert.Equal(t, parts, p.calls, "each part, in order")
	require.Len(t, m.got, 1)
	assert.Equal(t, []Digest{part("One", "A"), part("Two", "B"), part("Three", "C")}, m.got[0], "the merge sees every part, in order")
	assert.Equal(t, merged, d)
	assert.False(t, d.Unmerged)
	assert.Equal(t, [][2]int{{0, 3}, {1, 3}, {2, 3}, {3, 3}}, progress, "progress before the first part and after each")
}

func TestChunkedPartFailureFailsTheImport(t *testing.T) {
	text, _ := threeParts()
	boom := errors.New("backend down")
	p := &scripted{answers: []Digest{part("One", "A")}, errs: []error{nil, boom}}
	m := &merger{}
	_, err := Chunked{Part: p, Merge: m, Limit: 25}.Digest(context.Background(), text)

	require.ErrorIs(t, err, boom)
	assert.Contains(t, err.Error(), "part 2 of 3")
	assert.Len(t, p.calls, 2, "it stops at the first failure")
	assert.Empty(t, m.got)
}

func TestChunkedPartErrorsNeverShowTheSource(t *testing.T) {
	text := strings.Repeat("SECRET-LINE\n\n", 6)
	p := &scripted{errs: []error{ErrBackendFailed}}
	_, err := Chunked{Part: p, Merge: &merger{}, Limit: 25}.Digest(context.Background(), text)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "SECRET")
}

func TestChunkedAnEmptyPartIsSkipped(t *testing.T) {
	text, _ := threeParts()
	p := &scripted{answers: []Digest{part("One", "A"), {}, part("Three", "C")}, errs: []error{nil, ErrEmpty, nil}}
	m := &merger{result: part("M", "AC")}
	d, err := Chunked{Part: p, Merge: m, Limit: 25}.Digest(context.Background(), text)
	require.NoError(t, err)
	assert.Equal(t, "AC", d.Notes[0].Title)
	require.Len(t, m.got, 1)
	assert.Equal(t, []Digest{part("One", "A"), part("Three", "C")}, m.got[0], "a part of small talk adds nothing")
}

func TestChunkedOneUsablePartNeedsNoMerge(t *testing.T) {
	text, _ := threeParts()
	p := &scripted{answers: []Digest{{}, part("Two", "B"), {}}, errs: []error{ErrEmpty, nil, ErrEmpty}}
	m := &merger{}
	d, err := Chunked{Part: p, Merge: m, Limit: 25}.Digest(context.Background(), text)
	require.NoError(t, err)
	assert.Equal(t, part("Two", "B"), d)
	assert.Empty(t, m.got)
}

func TestChunkedAllPartsEmptyIsEmpty(t *testing.T) {
	text, _ := threeParts()
	p := &scripted{errs: []error{ErrEmpty, ErrEmpty, ErrEmpty}}
	_, err := Chunked{Part: p, Merge: &merger{}, Limit: 25}.Digest(context.Background(), text)
	assert.ErrorIs(t, err, ErrEmpty)
}

func TestChunkedMergeFailureKeepsTheParts(t *testing.T) {
	text, _ := threeParts()
	partA, partB := part("", "A", "Call Sam"), part("Pricing", "B", "Send sheet", "Call Sam")
	partA.Facts, partB.Facts = []string{"Fact A"}, []string{"Fact B"}
	partA.Conclusions, partB.Conclusions = []string{"Conclude A"}, []string{"Conclude B"}
	p := &scripted{answers: []Digest{
		partA,
		partB,
		{Topic: "Later", Source: "slack", Notes: []Note{{Title: "C", Body: "c"}, {Title: "D", Body: "d"}}},
	}}
	m := &merger{err: ErrBackendFailed}
	d, err := Chunked{Part: p, Merge: m, Limit: 25}.Digest(context.Background(), text)

	require.NoError(t, err, "a failed merge is not a failed import")
	assert.True(t, d.Unmerged, "the caller warns that the parts were not merged")
	var titles []string
	for _, n := range d.Notes {
		titles = append(titles, n.Title)
	}
	assert.Equal(t, []string{"A", "B", "C", "D"}, titles, "every note of every part, in order")
	assert.Equal(t, []string{"Call Sam", "Send sheet", "Call Sam"}, d.Actions)
	assert.Equal(t, []string{"Fact A", "Fact B"}, d.Facts)
	assert.Equal(t, []string{"Conclude A", "Conclude B"}, d.Conclusions)
	assert.Equal(t, "Pricing", d.Topic, "the first topic there is")
	assert.Equal(t, "meeting", d.Source, "the first source there is")
}

func TestChunkedMergeThatReturnsNothingKeepsTheParts(t *testing.T) {
	text, _ := threeParts()
	p := &scripted{answers: []Digest{part("One", "A"), part("Two", "B"), part("Three", "C")}}
	m := &merger{err: ErrEmpty}
	d, err := Chunked{Part: p, Merge: m, Limit: 25}.Digest(context.Background(), text)
	require.NoError(t, err)
	assert.True(t, d.Unmerged)
	assert.Len(t, d.Notes, 3)
}

func TestChunkedStopsWhenCancelled(t *testing.T) {
	text, _ := threeParts()
	ctx, cancel := context.WithCancel(context.Background())
	p := &scripted{answers: []Digest{part("One", "A"), part("Two", "B"), part("Three", "C")}}
	c := Chunked{Part: p, Merge: &merger{}, Limit: 25, OnPart: func(done, _ int) {
		if done == 1 {
			cancel()
		}
	}}
	_, err := c.Digest(ctx, text)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Len(t, p.calls, 1, "no more parts after cancelling")
}

func TestChunkedCancelledMergeIsNotAFallback(t *testing.T) {
	text, _ := threeParts()
	p := &scripted{answers: []Digest{part("One", "A"), part("Two", "B"), part("Three", "C")}}
	m := &merger{err: context.Canceled}
	_, err := Chunked{Part: p, Merge: m, Limit: 25}.Digest(context.Background(), text)
	assert.ErrorIs(t, err, context.Canceled, "cancelling means stop, not write the parts")
}

func TestChunkedDefaultLimit(t *testing.T) {
	p := &scripted{answers: []Digest{part("T", "A")}}
	big := strings.Repeat("word ", SinglePassBytes/5-10)
	_, err := Chunked{Part: p, Merge: &merger{}}.Digest(context.Background(), big)
	require.NoError(t, err)
	assert.Len(t, p.calls, 1, "a source under the default limit is one pass")
}

func TestMarshalParts(t *testing.T) {
	data, err := MarshalParts([]Digest{
		{Topic: "One", Source: "meeting", Notes: []Note{{Title: "A", Body: "a", Tags: []string{"x"}}}, Actions: []string{"Call"}, Facts: []string{"Fact"}, Conclusions: []string{"Go"}},
		{Topic: "Two", Source: "slack"},
	})
	require.NoError(t, err)
	var got []map[string]any
	require.NoError(t, json.Unmarshal(data, &got))
	require.Len(t, got, 2)
	assert.Equal(t, "One", got[0]["topic"])
	assert.Equal(t, "meeting", got[0]["source"])
	assert.Equal(t, []any{"Call"}, got[0]["actions"])
	assert.Equal(t, []any{"Fact"}, got[0]["facts"])
	assert.Equal(t, []any{"Go"}, got[0]["conclusions"])
	note := got[0]["notes"].([]any)[0].(map[string]any)
	assert.Equal(t, "A", note["title"])
	assert.Equal(t, []any{"x"}, note["tags"])
	assert.Equal(t, []any{}, got[1]["notes"], "empty lists are lists, not null")
	assert.Equal(t, []any{}, got[1]["actions"])
	assert.Equal(t, []any{}, got[1]["facts"])
	assert.Equal(t, []any{}, got[1]["conclusions"])
}

func TestCLIMerge(t *testing.T) {
	c, dir := answering(t, envelope(t, obj{"structured_output": json.RawMessage(goodAnswer)}))
	c.Prompt = "MERGE PROMPT"
	parts := []Digest{part("One", "A"), part("Two", "B")}

	d, err := c.Merge(context.Background(), parts)
	require.NoError(t, err)
	assert.Equal(t, "Pricing review", d.Topic)

	want, err := MarshalParts(parts)
	require.NoError(t, err)
	assert.JSONEq(t, string(want), read(t, dir, "stdin.txt"), "the partial results go on standard input as JSON")
	assert.Contains(t, read(t, dir, "args.txt"), "MERGE PROMPT")
}

func TestMergeTimeoutIsItsOwn(t *testing.T) {
	dir := t.TempDir()
	c := CLI{Command: fakeCommand(t, dir, "exec sleep 10"), Model: "sonnet", Timeout: 200 * time.Millisecond, Prompt: "p", Sources: testSources}
	_, err := c.Merge(context.Background(), []Digest{part("One", "A")})
	assert.ErrorIs(t, err, ErrTimeout)
}

func TestSplitAlwaysMakesProgress(t *testing.T) {
	done := make(chan []string, 1)
	go func() { done <- Split("🙂🙂🙂", 2) }() // a character is wider than the limit
	select {
	case parts := <-done:
		assert.Equal(t, "🙂🙂🙂", strings.Join(parts, ""))
		assert.Len(t, parts, 3, "one character per part, the least that can be cut")
	case <-time.After(2 * time.Second):
		t.Fatal("Split did not finish")
	}
}
