package glossary

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeFile(t *testing.T, root, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(root, FileName), []byte(content), 0o644))
}

func readFile(t *testing.T, root string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, FileName))
	require.NoError(t, err)
	return string(b)
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		content *string
		want    []Entry
		wantErr error
	}{
		{name: "missing file is empty", content: nil, want: nil},
		{name: "empty file is empty", content: ptr(""), want: nil},
		{
			name:    "keeps file order",
			content: ptr("zeta: last letter\nalpha: first letter\n"),
			want:    []Entry{{"zeta", "last letter"}, {"alpha", "first letter"}},
		},
		{
			name:    "multi-line definition",
			content: ptr("term: |\n  line one\n  line two\n"),
			want:    []Entry{{"term", "line one\nline two\n"}},
		},
		{
			name:    "a list holds several meanings of one term",
			content: ptr("NAM:\n  - National Account Manager\n  - Not Actual Mileage\nOG: Owner Group\n"),
			want:    []Entry{{"NAM", "National Account Manager"}, {"NAM", "Not Actual Mileage"}, {"OG", "Owner Group"}},
		},
		{name: "an empty list has no entries", content: ptr("a: []\nb: two\n"), want: []Entry{{"b", "two"}}},
		{name: "list of non-text", content: ptr("a:\n  - x: y\n"), wantErr: ErrFormat},
		{name: "not a mapping", content: ptr("- a\n- b\n"), wantErr: ErrFormat},
		{name: "non-text definition", content: ptr("term:\n  nested: x\n"), wantErr: ErrFormat},
		{name: "broken yaml", content: ptr("a: [\n"), wantErr: ErrFormat},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			if tt.content != nil {
				writeFile(t, root, *tt.content)
			}
			got, err := Load(root)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestAddCreatesFileAndRejectsExactDuplicates(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, Add(root, "PARA", "Projects, Areas, Resources, Archive"))
	require.NoError(t, Add(root, "Zettel", "A slip of paper"))

	got, err := Load(root)
	require.NoError(t, err)
	assert.Equal(t, []Entry{{"PARA", "Projects, Areas, Resources, Archive"}, {"Zettel", "A slip of paper"}}, got)

	assert.ErrorIs(t, Add(root, "PARA", "Projects, Areas, Resources, Archive"), ErrExists)
	assert.NoError(t, Add(root, "para", "another case is a different term"))
}

func TestAddSecondMeaningMakesAList(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "# terms\nNAM: National Account Manager\nOG: Owner Group\n")

	require.NoError(t, Add(root, "NAM", "Not Actual Mileage"))
	require.NoError(t, Add(root, "NAM", "Network Access Module"))

	got, err := Load(root)
	require.NoError(t, err)
	assert.Equal(t, []Entry{
		{"NAM", "National Account Manager"}, {"NAM", "Not Actual Mileage"}, {"NAM", "Network Access Module"},
		{"OG", "Owner Group"},
	}, got)
	assert.Contains(t, readFile(t, root), "# terms")
	assert.ErrorIs(t, Add(root, "NAM", "Not Actual Mileage"), ErrExists)
}

func TestInvalidTerms(t *testing.T) {
	root := t.TempDir()
	assert.ErrorIs(t, Add(root, "   ", "x"), ErrEmptyTerm)
	_, err := os.Stat(filepath.Join(root, FileName))
	assert.True(t, os.IsNotExist(err), "a rejected add must not create the file")
}

func TestABlankDefinitionIsStored(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, Add(root, "term", "  \n "))
	got, err := Load(root)
	require.NoError(t, err)
	assert.Equal(t, []Entry{{"term", ""}}, got)

	require.NoError(t, Replace(root, Entry{"term", ""}, Entry{"term", "now defined"}))
	require.NoError(t, Replace(root, Entry{"term", "now defined"}, Entry{"term", "  "}))
	got, err = Load(root)
	require.NoError(t, err)
	assert.Equal(t, []Entry{{"term", ""}}, got, "editing to blank keeps the term")
}

func TestReplace(t *testing.T) {
	const start = "# my terms\na: one\nNAM:\n  - National Account Manager\n  - Not Actual Mileage\nc: three\n"
	tests := []struct {
		name     string
		old, new Entry
		want     []Entry
		wantErr  error
	}{
		{
			name: "edit the definition in place",
			old:  Entry{"a", "one"}, new: Entry{"a", "uno"},
			want: []Entry{{"a", "uno"}, {"NAM", "National Account Manager"}, {"NAM", "Not Actual Mileage"}, {"c", "three"}},
		},
		{
			name: "edit one meaning of a list",
			old:  Entry{"NAM", "Not Actual Mileage"}, new: Entry{"NAM", "Odometer rollback"},
			want: []Entry{{"a", "one"}, {"NAM", "National Account Manager"}, {"NAM", "Odometer rollback"}, {"c", "three"}},
		},
		{
			name: "rename a single term",
			old:  Entry{"c", "three"}, new: Entry{"C2", "three"},
			want: []Entry{{"a", "one"}, {"NAM", "National Account Manager"}, {"NAM", "Not Actual Mileage"}, {"C2", "three"}},
		},
		{
			name: "rename one meaning onto an existing term joins its list",
			old:  Entry{"c", "three"}, new: Entry{"a", "three"},
			want: []Entry{{"a", "one"}, {"a", "three"}, {"NAM", "National Account Manager"}, {"NAM", "Not Actual Mileage"}},
		},
		{
			name: "rename one meaning out of a list leaves the other",
			old:  Entry{"NAM", "Not Actual Mileage"}, new: Entry{"NAM2", "Not Actual Mileage"},
			want: []Entry{{"a", "one"}, {"NAM", "National Account Manager"}, {"c", "three"}, {"NAM2", "Not Actual Mileage"}},
		},
		{name: "missing", old: Entry{"zzz", "x"}, new: Entry{"zzz", "y"}, wantErr: ErrNotFound},
		{name: "wrong definition is missing", old: Entry{"a", "two"}, new: Entry{"a", "y"}, wantErr: ErrNotFound},
		{name: "onto an identical entry", old: Entry{"c", "three"}, new: Entry{"a", "one"}, wantErr: ErrExists},
		{name: "empty term", old: Entry{"a", "one"}, new: Entry{" ", "x"}, wantErr: ErrEmptyTerm},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, root, start)
			err := Replace(root, tt.old, tt.new)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				assert.Equal(t, start, readFile(t, root), "a failed edit leaves the file alone")
				return
			}
			require.NoError(t, err)
			got, err := Load(root)
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.want, got)
			assert.Contains(t, readFile(t, root), "# my terms", "comments survive an edit")
		})
	}
}

func TestReplaceKeepsPlace(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "a: one\nb: two\nc: three\n")
	require.NoError(t, Replace(root, Entry{"b", "two"}, Entry{"B2", "second"}))
	got, err := Load(root)
	require.NoError(t, err)
	assert.Equal(t, []Entry{{"a", "one"}, {"B2", "second"}, {"c", "three"}}, got)
}

func TestDelete(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "a: one\nNAM:\n  - first\n  - second\n  - third\nb: two\n")

	require.NoError(t, Delete(root, Entry{"a", "one"}))
	require.NoError(t, Delete(root, Entry{"NAM", "second"}))
	got, err := Load(root)
	require.NoError(t, err)
	assert.Equal(t, []Entry{{"NAM", "first"}, {"NAM", "third"}, {"b", "two"}}, got)

	require.NoError(t, Delete(root, Entry{"NAM", "first"}))
	require.NoError(t, Delete(root, Entry{"NAM", "third"}))
	got, err = Load(root)
	require.NoError(t, err)
	assert.Equal(t, []Entry{{"b", "two"}}, got, "the last meaning removes the term")
	assert.NotContains(t, readFile(t, root), "NAM")

	assert.ErrorIs(t, Delete(root, Entry{"a", "one"}), ErrNotFound)
	assert.ErrorIs(t, Delete(root, Entry{"b", "other"}), ErrNotFound)
}

func TestWritesRefuseABrokenFile(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "- not\n- a mapping\n")
	assert.ErrorIs(t, Add(root, "x", "y"), ErrFormat)
	assert.ErrorIs(t, Delete(root, Entry{"x", "y"}), ErrFormat)
	assert.Equal(t, "- not\n- a mapping\n", readFile(t, root), "a file that cannot be read is left alone")
}

func TestSpecialCharactersRoundTrip(t *testing.T) {
	root := t.TempDir()
	def := "has: colon, #hash and \"quotes\"\nand a second line"
	require.NoError(t, Add(root, "key: with #chars", def))
	got, err := Load(root)
	require.NoError(t, err)
	assert.Equal(t, []Entry{{"key: with #chars", def}}, got)
}

func ptr(s string) *string { return &s }

func TestSorted(t *testing.T) {
	in := []Entry{
		{"zeta", "z"}, {"arm", "new"}, {"NAM", "second"}, {"Alpha", "a"},
		{"NAM", "first"}, {"ARM", "caps"}, {"alpha", "lower"}, {"c4l", "x"}, {"CCL", "y"},
	}
	before := append([]Entry(nil), in...)

	got := Sorted(in)

	assert.Equal(t, []Entry{
		{"Alpha", "a"}, {"alpha", "lower"}, {"ARM", "caps"}, {"arm", "new"},
		{"c4l", "x"}, {"CCL", "y"}, {"NAM", "second"}, {"NAM", "first"}, {"zeta", "z"},
	}, got, "case-insensitive by term; meanings of one term keep their order")
	assert.Equal(t, before, in, "the input is left as it was")
}
