package digest

import (
	"context"
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const goodAnswer = `{"topic":"Pricing review","source":"meeting","notes":[{"title":"Tier pricing","body":"Three tiers.","tags":["pricing"]}],"actions":["Send the sheet"]}`

// envelope wraps an answer the way the claude CLI does with --output-format
// json and --json-schema.
func envelope(t *testing.T, fields obj) string {
	t.Helper()
	base := obj{"type": "result", "subtype": "success", "is_error": false}
	maps.Copy(base, fields)
	b, err := json.Marshal(base)
	require.NoError(t, err)
	return string(b)
}

// fakeCommand writes an executable script that records its arguments,
// working directory and standard input in dir, then runs body.
func fakeCommand(t *testing.T, dir, body string) string {
	t.Helper()
	script := "#!/bin/sh\n" +
		`printf '%s\n' "$@" > "` + dir + `/args.txt"` + "\n" +
		`pwd > "` + dir + `/cwd.txt"` + "\n" +
		`cat > "` + dir + `/stdin.txt"` + "\n" +
		body + "\n"
	path := filepath.Join(dir, "fake-claude")
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755))
	return path
}

func answering(t *testing.T, out string) (CLI, string) {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "out.json"), []byte(out), 0o644))
	return CLI{
		Command: fakeCommand(t, dir, `cat "`+dir+`/out.json"`),
		Model:   "sonnet",
		Timeout: 10 * time.Second,
		Prompt:  "PROMPT TEXT",
		Sources: testSources,
	}, dir
}

func read(t *testing.T, dir, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, name))
	require.NoError(t, err)
	return string(b)
}

func TestCLIDigestUsesStructuredOutput(t *testing.T) {
	c, dir := answering(t, envelope(t, obj{
		"structured_output": json.RawMessage(goodAnswer),
		"result":            "ignored when structured output is present",
	}))

	d, err := c.Digest(context.Background(), "the meeting text")
	require.NoError(t, err)
	assert.Equal(t, "Pricing review", d.Topic)
	assert.Equal(t, "meeting", d.Source)
	require.Len(t, d.Notes, 1)
	assert.Equal(t, []string{"Send the sheet"}, d.Actions)
	assert.Equal(t, "the meeting text", read(t, dir, "stdin.txt"), "the source goes on standard input, unchanged")
}

func TestCLIDigestFallsBackToTheResultText(t *testing.T) {
	c, _ := answering(t, envelope(t, obj{"result": goodAnswer}))
	d, err := c.Digest(context.Background(), "x")
	require.NoError(t, err)
	assert.Equal(t, "Pricing review", d.Topic)
}

func TestCLIDigestArguments(t *testing.T) {
	c, dir := answering(t, envelope(t, obj{"structured_output": json.RawMessage(goodAnswer)}))
	c.Model = "claude-sonnet-5-5"
	_, err := c.Digest(context.Background(), "x")
	require.NoError(t, err)

	args := strings.Split(strings.TrimSuffix(read(t, dir, "args.txt"), "\n"), "\n")
	pair := func(flag string) string {
		for i, a := range args {
			if a == flag && i+1 < len(args) {
				return args[i+1]
			}
		}
		t.Fatalf("flag %s missing in %q", flag, args)
		return ""
	}
	assert.Contains(t, args, "-p")
	assert.Equal(t, "claude-sonnet-5-5", pair("--model"))
	assert.Equal(t, "", pair("--tools"), "the agent gets no tools")
	assert.Equal(t, "json", pair("--output-format"))
	assert.Equal(t, "PROMPT TEXT", pair("--system-prompt"), "the whole prompt is the system prompt")
	assert.JSONEq(t, Schema, pair("--json-schema"))
	assert.Contains(t, args, "--no-session-persistence")
	assert.Contains(t, args, "--disable-slash-commands")
	assert.Contains(t, args, "--strict-mcp-config")
	for _, a := range args {
		assert.NotContains(t, a, "the meeting text", "the source is never an argument")
	}
}

func TestCLIDigestRunsInAnEmptyTemporaryDirectory(t *testing.T) {
	c, dir := answering(t, envelope(t, obj{"structured_output": json.RawMessage(goodAnswer)}))
	_, err := c.Digest(context.Background(), "x")
	require.NoError(t, err)

	cwd := strings.TrimSpace(read(t, dir, "cwd.txt"))
	wd, err := os.Getwd()
	require.NoError(t, err)
	assert.NotEqual(t, dir, cwd)
	assert.NotEqual(t, wd, cwd, "never the project or the notebook")
	assert.Equal(t, "para-digest-", filepath.Base(cwd)[:len("para-digest-")], "a fresh directory made for the run")
	_, statErr := os.Stat(cwd)
	assert.True(t, os.IsNotExist(statErr), "the directory is removed afterwards")
}

func TestCLIDigestFailures(t *testing.T) {
	const secret = "SECRET-MEETING-TEXT"
	tests := []struct {
		name string
		out  string
		want error
	}{
		{"agent reports an error", envelope(t, obj{"is_error": true, "subtype": "error_during_execution", "result": secret}), ErrBackendFailed},
		{"not json at all", "Sorry, I cannot help with " + secret, ErrBadJSON},
		{"no answer in the envelope", envelope(t, obj{}), ErrNoOutput},
		{"null structured output and empty result", envelope(t, obj{"structured_output": nil, "result": ""}), ErrNoOutput},
		{"answer is the wrong shape", envelope(t, obj{"structured_output": json.RawMessage(`{"notes":"x"}`)}), ErrBadJSON},
		{"answer is empty", envelope(t, obj{"structured_output": json.RawMessage(`{"notes":[],"actions":[]}`)}), ErrEmpty},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := answering(t, tt.out)
			_, err := c.Digest(context.Background(), secret)
			require.ErrorIs(t, err, tt.want)
			assert.NotContains(t, err.Error(), secret, "errors never show source or answer text")
		})
	}
}

func TestCLIDigestExitCodeFailure(t *testing.T) {
	dir := t.TempDir()
	c := CLI{Command: fakeCommand(t, dir, "echo SECRET-STDERR >&2\nexit 3"), Model: "sonnet", Timeout: 5 * time.Second, Prompt: "p", Sources: testSources}
	_, err := c.Digest(context.Background(), "x")
	require.ErrorIs(t, err, ErrBackendFailed)
	assert.Contains(t, err.Error(), "3", "the exit code helps")
	assert.NotContains(t, err.Error(), "SECRET", "stderr is not shown")
}

func TestCLIDigestMissingCommand(t *testing.T) {
	c := CLI{Command: filepath.Join(t.TempDir(), "no-such-command"), Model: "sonnet", Timeout: time.Second, Prompt: "p", Sources: testSources}
	_, err := c.Digest(context.Background(), "x")
	assert.ErrorIs(t, err, ErrBackendMissing)
}

func TestCLIDigestTimeout(t *testing.T) {
	dir := t.TempDir()
	c := CLI{Command: fakeCommand(t, dir, "exec sleep 10"), Model: "sonnet", Timeout: 200 * time.Millisecond, Prompt: "p", Sources: testSources}
	start := time.Now()
	_, err := c.Digest(context.Background(), "x")
	require.ErrorIs(t, err, ErrTimeout)
	assert.Less(t, time.Since(start), 5*time.Second, "the process is killed")
}

func TestCLIDigestCancelled(t *testing.T) {
	dir := t.TempDir()
	c := CLI{Command: fakeCommand(t, dir, "exec sleep 10"), Model: "sonnet", Timeout: time.Minute, Prompt: "p", Sources: testSources}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(150 * time.Millisecond); cancel() }()
	_, err := c.Digest(ctx, "x")
	assert.ErrorIs(t, err, context.Canceled)
	assert.NotErrorIs(t, err, ErrTimeout)
}

func TestCLIDigestRefusesOversizedOutput(t *testing.T) {
	dir := t.TempDir()
	c := CLI{Command: fakeCommand(t, dir, "head -c 6000000 /dev/zero | tr '\\0' 'a'"), Model: "sonnet", Timeout: 10 * time.Second, Prompt: "p", Sources: testSources}
	_, err := c.Digest(context.Background(), "x")
	assert.ErrorIs(t, err, ErrTooLarge)
}

func TestCLIDigestRejectsUnsafeSettings(t *testing.T) {
	good := CLI{Command: "claude", Model: "sonnet", Timeout: time.Second, Prompt: "p"}
	tests := []struct {
		name string
		edit func(c *CLI)
	}{
		{"model that looks like a flag", func(c *CLI) { c.Model = "--dangerously-skip-permissions" }},
		{"empty model", func(c *CLI) { c.Model = "" }},
		{"empty command", func(c *CLI) { c.Command = "" }},
		{"empty prompt", func(c *CLI) { c.Prompt = "  " }},
		{"no timeout", func(c *CLI) { c.Timeout = 0 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := good
			tt.edit(&c)
			_, err := c.Digest(context.Background(), "x")
			assert.ErrorIs(t, err, ErrBadSettings)
		})
	}
}

func TestCLIDigestRefusesAnEmptySource(t *testing.T) {
	c, _ := answering(t, envelope(t, obj{"structured_output": json.RawMessage(goodAnswer)}))
	_, err := c.Digest(context.Background(), "  \n ")
	assert.ErrorIs(t, err, ErrEmptySource)
}

func TestSchemaMatchesTheParseContract(t *testing.T) {
	var s struct {
		Type       string                    `json:"type"`
		Required   []string                  `json:"required"`
		Properties map[string]map[string]any `json:"properties"`
	}
	require.NoError(t, json.Unmarshal([]byte(Schema), &s))
	assert.Equal(t, "object", s.Type)
	assert.ElementsMatch(t, []string{"topic", "source", "notes", "actions", "facts", "conclusions"}, s.Required)
	for _, k := range []string{"topic", "source", "notes", "actions", "facts", "conclusions"} {
		assert.Contains(t, s.Properties, k)
	}
}
