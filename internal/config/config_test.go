package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tkilb/para/internal/store"
)

func TestParse(t *testing.T) {
	home := t.TempDir()
	work := filepath.Join(home, "Notes", "work")
	require.NoError(t, os.MkdirAll(work, 0o755))
	file := filepath.Join(home, "afile")
	require.NoError(t, os.WriteFile(file, nil, 0o644))
	abs := t.TempDir()

	tests := []struct {
		name string
		yaml string
		want Config
	}{
		{
			name: "tilde expansion, order and default retention",
			yaml: "notebooks:\n  - ~/Notes/work\n  - " + abs + "\n",
			want: Config{
				Notebooks: []Notebook{
					{Path: work, Name: "work", Available: true},
					{Path: abs, Name: filepath.Base(abs), Available: true},
				},
				Retention: Retention{DailyDays: DefaultRetentionDays},
			},
		},
		{
			name: "bare tilde is the home directory",
			yaml: "notebooks: [\"~\"]\n",
			want: Config{
				Notebooks: []Notebook{{Path: home, Name: filepath.Base(home), Available: true}},
				Retention: Retention{DailyDays: DefaultRetentionDays},
			},
		},
		{
			name: "missing path is listed as unavailable",
			yaml: "notebooks:\n  - ~/Notes/gone\n",
			want: Config{
				Notebooks: []Notebook{{Path: filepath.Join(home, "Notes", "gone"), Name: "gone"}},
				Retention: Retention{DailyDays: DefaultRetentionDays},
			},
		},
		{
			name: "a file is unavailable",
			yaml: "notebooks:\n  - ~/afile\n",
			want: Config{
				Notebooks: []Notebook{{Path: file, Name: "afile"}},
				Retention: Retention{DailyDays: DefaultRetentionDays},
			},
		},
		{
			name: "trailing slash is cleaned",
			yaml: "notebooks:\n  - ~/Notes/work/\n",
			want: Config{
				Notebooks: []Notebook{{Path: work, Name: "work", Available: true}},
				Retention: Retention{DailyDays: DefaultRetentionDays},
			},
		},
		{
			name: "explicit retention",
			yaml: "notebooks:\n  - ~/Notes/work\nretention:\n  daily: 7d\n",
			want: Config{
				Notebooks: []Notebook{{Path: work, Name: "work", Available: true}},
				Retention: Retention{DailyDays: 7},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse([]byte(tt.yaml), home)
			require.NoError(t, err)
			if tt.want.Sources == nil {
				tt.want.Sources = DefaultSources
			}
			tt.want.TemplatesDir = filepath.Join(home, ".config", "para", "templates")
			if tt.want.Digest == (Digest{}) {
				tt.want.Digest = Digest{Command: "claude", Model: "sonnet", Timeout: 5 * time.Minute}
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseNoNotebooks(t *testing.T) {
	tests := []struct{ name, yaml string }{
		{"empty file", ""},
		{"no notebooks key", "retention:\n  daily: 30d\n"},
		{"empty list", "notebooks: []\n"},
		{"null", "notebooks:\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.yaml), t.TempDir())
			require.ErrorIs(t, err, ErrNoNotebooks)
			assert.Contains(t, err.Error(), ExampleConfig)
		})
	}
}

func TestParseInvalid(t *testing.T) {
	tests := []struct{ name, yaml, wantMsg string }{
		{"relative path", "notebooks:\n  - Notes/work\n", "absolute"},
		{"dot path", "notebooks:\n  - .\n", "absolute"},
		{"blank path", "notebooks:\n  - \"  \"\n", "absolute"},
		{"tilde user form", "notebooks:\n  - ~bob/Notes\n", "absolute"},
		{"malformed yaml", "notebooks: [\n", "parse config"},
		{"notebooks is a string", "notebooks: nope\n", "parse config"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.yaml), t.TempDir())
			require.Error(t, err)
			assert.NotErrorIs(t, err, ErrNoNotebooks)
			assert.Contains(t, err.Error(), tt.wantMsg)
		})
	}
}

func TestParseRetention(t *testing.T) {
	tests := []struct {
		daily   string
		want    int
		wantErr bool
	}{
		{"", DefaultRetentionDays, false},
		{"1d", 1, false},
		{"30d", 30, false},
		{"365d", 365, false},
		{"007d", 7, false},
		{"0d", 0, true},
		{"-5d", 0, true},
		{"+5d", 0, true},
		{"30", 0, true},
		{"d", 0, true},
		{"30h", 0, true},
		{"1.5d", 0, true},
		{"30D", 0, true},
		{" 30d", 0, true},
		{"30d ", 0, true},
		{"1w", 0, true},
		{"36500d", 36500, false},
		{"36501d", 0, true},
		{"9999999999d", 0, true},
		{"99999999999999999999d", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.daily, func(t *testing.T) {
			got, err := parseRetention(tt.daily)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "retention.daily")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestLoad(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	nb := filepath.Join(home, "Notes")
	require.NoError(t, os.MkdirAll(nb, 0o755))

	t.Run("reads a file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.yaml")
		require.NoError(t, os.WriteFile(path, []byte("notebooks:\n  - ~/Notes\n"), 0o600))

		got, err := Load(path)
		require.NoError(t, err)
		assert.Equal(t, []Notebook{{Path: nb, Name: "Notes", Available: true}}, got.Notebooks)
	})
	t.Run("missing file reports no notebooks", func(t *testing.T) {
		_, err := Load(filepath.Join(t.TempDir(), "absent.yaml"))
		require.ErrorIs(t, err, ErrNoNotebooks)
	})
	t.Run("unreadable path is a plain error", func(t *testing.T) {
		_, err := Load(t.TempDir()) // a directory, not a file
		require.Error(t, err)
		assert.NotErrorIs(t, err, ErrNoNotebooks)
	})
}

func TestDefaultPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got, err := DefaultPath()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, ".config", "para", "config.yaml"), got)
}

func TestParseSources(t *testing.T) {
	const nb = "notebooks:\n  - /tmp\n"
	tests := []struct {
		name, yaml string
		want       []string
	}{
		{"absent uses the defaults", nb, DefaultSources},
		{"empty list uses the defaults", nb + "sources: []\n", DefaultSources},
		{"null uses the defaults", nb + "sources:\n", DefaultSources},
		{"custom list keeps its order", nb + "sources: [call, standup, adhoc]\n", []string{"call", "standup", "adhoc"}},
		{"entries are trimmed", nb + "sources:\n  - \"  call \"\n  - web\n", []string{"call", "web"}},
		{"spaces and punctuation inside are fine", nb + "sources:\n  - team sync\n  - 1:1\n", []string{"team sync", "1:1"}},
		{"a single source", nb + "sources: [notes]\n", []string{"notes"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Parse([]byte(tt.yaml), t.TempDir())
			require.NoError(t, err)
			assert.Equal(t, tt.want, cfg.Sources)
		})
	}
}

func TestParseSourcesRejectsBadEntries(t *testing.T) {
	const nb = "notebooks:\n  - /tmp\n"
	long := ""
	for i := 0; i < maxSourceRunes+1; i++ {
		long += "x"
	}
	tests := []struct{ name, yaml string }{
		{"blank entry", nb + "sources: [call, \"  \"]\n"},
		{"duplicate", nb + "sources: [call, web, call]\n"},
		{"duplicate ignoring case", nb + "sources: [Call, web, call]\n"},
		{"control character", nb + "sources:\n  - \"a\\tb\"\n"},
		{"newline", nb + "sources:\n  - \"a\\nb\"\n"},
		{"too long", nb + "sources: [" + long + "]\n"},
		{"not a list", nb + "sources: call\n"},
		{"not strings", nb + "sources: [[a], b]\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.yaml), t.TempDir())
			require.Error(t, err)
			assert.NotErrorIs(t, err, ErrNoNotebooks)
			assert.Contains(t, err.Error(), "sources")
		})
	}
}

func TestDefaultSourcesAreNotSharedWithCallers(t *testing.T) {
	cfg, err := Parse([]byte("notebooks:\n  - /tmp\n"), t.TempDir())
	require.NoError(t, err)
	cfg.Sources[0] = "changed"
	assert.Equal(t, "adhoc", DefaultSources[0], "changing a parsed config cannot change the defaults")
}

func TestValidateSource(t *testing.T) {
	long := ""
	for i := 0; i < maxSourceRunes; i++ {
		long += "x"
	}
	for _, ok := range []string{"adhoc", "  call  ", "1:1", "team sync", "héllo", long} {
		got, err := ValidateSource(ok)
		require.NoError(t, err, ok)
		assert.Equal(t, strings.TrimSpace(ok), got)
	}
	for _, bad := range []string{"", "   ", "a\tb", "a\nb", "a\x00b", long + "x"} {
		_, err := ValidateSource(bad)
		assert.Error(t, err, "%q", bad)
	}
}

func TestParseMouse(t *testing.T) {
	tests := []struct {
		name string
		ui   string
		want bool
	}{
		{"absent means on", "", true},
		{"ui without mouse means on", "ui: {}\n", true},
		{"true", "ui:\n  mouse: true\n", true},
		{"false turns it off", "ui:\n  mouse: false\n", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Parse([]byte("notebooks: [\"~\"]\n"+tt.ui), t.TempDir())
			require.NoError(t, err)
			assert.Equal(t, tt.want, cfg.UI.MouseEnabled())
		})
	}
}

func TestParseMouseRejectsNonBoolean(t *testing.T) {
	_, err := Parse([]byte("notebooks: [\"~\"]\nui:\n  mouse: maybe\n"), t.TempDir())
	require.Error(t, err)
}

func TestSelect(t *testing.T) {
	cfg := Config{Notebooks: []Notebook{
		{Path: "/a", Name: "a", Available: false},
		{Path: "/b", Name: "b", Available: true},
		{Path: "/c", Name: "c", Available: true},
	}}
	tests := []struct {
		name string
		cfg  Config
		last string
		want int
		ok   bool
	}{
		{"last used wins", cfg, "/c", 2, true},
		{"no last: first available", cfg, "", 1, true},
		{"last no longer configured", cfg, "/gone", 1, true},
		{"last is unavailable", cfg, "/a", 1, true},
		{"none available", Config{Notebooks: []Notebook{{Path: "/a", Available: false}}}, "/a", 0, false},
		{"empty", Config{}, "", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tt.cfg.Select(tt.last)
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func kindsConfig(t *testing.T, kinds string) (Config, error) {
	t.Helper()
	return Parse([]byte("notebooks: [\"~\"]\n"+kinds), t.TempDir())
}

func TestParseKindsReadsTheArray(t *testing.T) {
	cfg, err := kindsConfig(t, `kinds:
  - name: fact
    color: "#2EA043"
  - name: Recipe
    color: "#ff8800"
    meaning: Something to cook
    badge: cook
  - name: quote
`)
	require.NoError(t, err)
	assert.Equal(t, []store.KindSpec{
		{Name: "fact", Color: "#2ea043"},
		{Name: "recipe", Color: "#ff8800", Meaning: "Something to cook", Badge: "cook"},
		{Name: "quote"},
	}, cfg.Kinds)
}

func TestParseKindsAbsentOrEmptyMeansTheBuiltIns(t *testing.T) {
	for _, in := range []string{"", "kinds: []\n", "kinds:\n"} {
		cfg, err := kindsConfig(t, in)
		require.NoError(t, err, in)
		assert.Empty(t, cfg.Kinds)
	}
}

func TestParseKindsRejectsMistakes(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"a bad color names the entry", "kinds:\n  - name: recipe\n    color: green\n", "recipe"},
		{"a missing name", "kinds:\n  - color: \"#112233\"\n", "entry 1"},
		{"a duplicate", "kinds:\n  - name: a\n  - name: A\n", "twice"},
		{"a typo in a key is not ignored", "kinds:\n  - name: recipe\n    colour: \"#112233\"\n", "colour"},
		{"not a list", "kinds: fact\n", "list"},
		{"an entry that is not a mapping", "kinds:\n  - fact\n", "entry 1"},
		{"a non-text value", "kinds:\n  - name: recipe\n    color: [1]\n", "color"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := kindsConfig(t, tt.in)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestTemplatesDirIsInTheConfigFolder(t *testing.T) {
	home := t.TempDir()
	want := filepath.Join(home, ".config", "para", "templates")
	nb := "notebooks:\n  - " + home + "\n"

	cfg, err := Parse([]byte(nb), home)
	require.NoError(t, err)
	assert.Equal(t, want, cfg.TemplatesDir, "the folder need not exist")

	cfg, err = Parse([]byte(nb+"templates:\n  daily: /elsewhere.md\n"), home)
	require.NoError(t, err)
	assert.Equal(t, want, cfg.TemplatesDir, "a templates key in the yaml is ignored")
}
