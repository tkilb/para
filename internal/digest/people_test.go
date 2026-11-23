package digest

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func peopleOf(t *testing.T, root, rel string) []string {
	t.Helper()
	node := readNote(t, root, rel).Frontmatter.Get("people")
	require.NotNil(t, node, "%s has a people property", rel)
	var out []string
	for _, c := range node.Content {
		out = append(out, c.Value)
	}
	return out
}

func TestParsePeopleUseFirstNameAndLastInitial(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{"full names", []string{"Tyler Kilburn", "Amber Dean"}, []string{"Tyler K.", "Amber D."}},
		{"already in format", []string{"Howie C."}, []string{"Howie C."}},
		{"initial without a period", []string{"Tyler K"}, []string{"Tyler K."}},
		{"the last word is the surname", []string{"Mary Ann Smith"}, []string{"Mary S."}},
		{"case is tidied", []string{"justine ross"}, []string{"Justine R."}},
		{"spaces are trimmed", []string{"  Tyler   Kilburn  "}, []string{"Tyler K."}},
		{"duplicates are dropped, ignoring case", []string{"Tyler Kilburn", "tyler k.", "Tyler K"}, []string{"Tyler K."}},
		{"a first name alone is dropped, not guessed", []string{"Patrick", "Tyler Kilburn"}, []string{"Tyler K."}},
		{"blank entries are dropped", []string{"", "  ", "Amber Dean"}, []string{"Amber D."}},
		{"control characters go", []string{"Tyler\x07 Kilburn\n"}, []string{"Tyler K."}},
		{"a non-Latin name keeps its first letter", []string{"Åsa Öberg"}, []string{"Åsa Ö."}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, err := parse(t, obj{"topic": "T", "source": "meeting", "notes": []obj{note("A", "b")}, "people": tt.in})
			require.NoError(t, err)
			assert.Equal(t, tt.want, d.People)
		})
	}
}

func TestParseWithoutPeopleIsFine(t *testing.T) {
	d, err := parse(t, obj{"topic": "T", "source": "meeting", "notes": []obj{note("A", "b")}})
	require.NoError(t, err)
	assert.Empty(t, d.People)
}

func TestParseCapsPeople(t *testing.T) {
	var in []string
	for i := 0; i < MaxPeople+10; i++ {
		in = append(in, "Person"+strings.Repeat("x", i)+" Smith")
	}
	d, err := parse(t, obj{"topic": "T", "source": "meeting", "notes": []obj{note("A", "b")}, "people": in})
	require.NoError(t, err)
	assert.Len(t, d.People, MaxPeople)
}

func TestSchemaAsksForPeople(t *testing.T) {
	assert.Contains(t, Schema, `"people"`)
}

func TestDefaultPromptsAskForPeopleInTheFormat(t *testing.T) {
	for name, p := range map[string]string{"import": DefaultPrompt, "merge": DefaultMergePrompt} {
		assert.Contains(t, p, `"people"`, name)
		assert.Contains(t, p, "Tyler K.", name+" shows the format")
	}
}

func TestFinishWritesPeopleToEveryFile(t *testing.T) {
	root, s := newStore(t)
	f, err := Begin(s, day, importKind, fileSource("Kickoff", "Tyler Kilburn: hi"))
	require.NoError(t, err)
	d := Digest{
		Topic: "Kickoff", Source: "meeting",
		Notes:   []Note{{Title: "Scope", Body: "text", Tags: []string{"a"}}},
		Actions: []string{"Sam: send notes"}, Facts: []string{"A fact"}, Conclusions: []string{"A conclusion"},
		People: []string{"Tyler K.", "Amber D."},
	}
	res, err := f.Finish(d)
	require.NoError(t, err)

	want := []string{"Tyler K.", "Amber D."}
	for _, name := range append([]string{"_source.md"}, res.Files...) {
		assert.Equal(t, want, peopleOf(t, root, res.Rel+"/"+name), name)
	}
}

func TestFinishWithoutPeopleLeavesThePropertyEmpty(t *testing.T) {
	root, s := newStore(t)
	f, err := Begin(s, day, importKind, fileSource("Kickoff", "text"))
	require.NoError(t, err)
	res, err := f.Finish(Digest{Topic: "Kickoff", Source: "meeting", Notes: []Note{{Title: "Scope", Body: "text"}}})
	require.NoError(t, err)
	assert.Empty(t, peopleOf(t, root, res.Rel+"/Scope.md"))
	assert.Empty(t, peopleOf(t, root, res.Rel+"/_source.md"))
}

func TestFinishCleansPeopleFromAHandMadeDigest(t *testing.T) {
	root, s := newStore(t)
	f, err := Begin(s, day, importKind, fileSource("Kickoff", "text"))
	require.NoError(t, err)
	res, err := f.Finish(Digest{Notes: []Note{{Title: "Scope", Body: "text"}}, People: []string{"Tyler Kilburn", "tyler k"}})
	require.NoError(t, err)
	assert.Equal(t, []string{"Tyler K."}, peopleOf(t, root, res.Rel+"/Scope.md"))
}

func TestRerunReplacesPeople(t *testing.T) {
	root, s := newStore(t)
	first, err := Import(context.Background(), s, day, importKind,
		&scripted{answers: []Digest{{Topic: "Kickoff", Source: "meeting", Notes: []Note{{Title: "Scope", Body: "x"}}, People: []string{"Tyler K."}}}},
		fileSource("Kickoff", "text"))
	require.NoError(t, err)

	_, err = Rerun(context.Background(), s, day, importKind,
		&scripted{answers: []Digest{{Topic: "Kickoff", Source: "meeting", Notes: []Note{{Title: "Scope", Body: "x"}}, People: []string{"Amber D."}}}},
		first.Rel)
	require.NoError(t, err)
	assert.Equal(t, []string{"Amber D."}, peopleOf(t, root, first.Rel+"/Scope.md"))
	assert.Equal(t, []string{"Amber D."}, peopleOf(t, root, first.Rel+"/_source.md"))
}

func TestConcatUnionsPeopleOfTheParts(t *testing.T) {
	a, b := part("One", "A"), part("Two", "B")
	a.People, b.People = []string{"Tyler K.", "Amber D."}, []string{"amber d.", "Howie C."}
	assert.Equal(t, []string{"Tyler K.", "Amber D.", "Howie C."}, concat([]Digest{a, b}).People)
}

func TestMarshalPartsCarriesPeople(t *testing.T) {
	p := part("One", "A")
	p.People = []string{"Tyler K."}
	data, err := MarshalParts([]Digest{p, part("Two", "B")})
	require.NoError(t, err)
	assert.Contains(t, string(data), `"people":["Tyler K."]`)
	assert.Contains(t, string(data), `"people":[]`, "no people is an empty list, not null")
}
