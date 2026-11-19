package config

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"go.yaml.in/yaml/v3"
)

const (
	defaultDigestCommand = "claude"
	defaultDigestModel   = "sonnet"
	defaultDigestTimeout = 5 * time.Minute

	// maxDigestTimeout caps digest.timeout so a hung agent cannot hold an
	// import for long.
	maxDigestTimeout = time.Hour
)

// Digest configures the agentic import: the command that runs the agent, the
// model and timeout, and the two prompt files. Import is off until Enabled is
// set, and setting it is the user's consent to send imported content to the
// command.
type Digest struct {
	Enabled bool
	// Command is the Claude Code CLI to run: a name on the PATH or a path.
	Command string
	// Model is the model name or alias passed to the command.
	Model   string
	Timeout time.Duration
	// Prompt and MergePrompt are optional files holding the whole main and
	// merge prompts. Empty means the prompts built into para, which cannot
	// drift from the code; a file is a deliberate override.
	Prompt      string
	MergePrompt string
}

// parseDigest reads the digest section: a mapping with the keys enabled,
// command, model, timeout, prompt and merge_prompt. Any other key is an
// error that names it. An absent or empty section gives the defaults, with
// import off and the built-in prompts. home expands "~".
func parseDigest(n *yaml.Node, home string) (Digest, error) {
	d := Digest{
		Command: defaultDigestCommand,
		Model:   defaultDigestModel,
		Timeout: defaultDigestTimeout,
	}
	if n.Kind == 0 || (n.Kind == yaml.ScalarNode && n.Tag == "!!null") {
		return d, nil
	}
	if n.Kind != yaml.MappingNode {
		return d, errors.New("digest must be a mapping, for example: digest: {enabled: true, model: sonnet}")
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		name, val := n.Content[i].Value, n.Content[i+1]
		if err := d.set(name, val, home); err != nil {
			return Digest{}, err
		}
	}
	return d, nil
}

func (d *Digest) set(name string, val *yaml.Node, home string) error {
	var err error
	switch name {
	case "enabled":
		if val.Kind != yaml.ScalarNode || val.Tag != "!!bool" {
			return errors.New("digest enabled must be true or false")
		}
		err = val.Decode(&d.Enabled)
	case "command":
		d.Command, err = word(name, val, false)
	case "model":
		d.Model, err = word(name, val, true)
	case "timeout":
		d.Timeout, err = parseDigestTimeout(val)
	case "prompt":
		d.Prompt, err = promptPath(name, val, home)
	case "merge_prompt":
		d.MergePrompt, err = promptPath(name, val, home)
	default:
		return fmt.Errorf("digest entry %q is not known; the entries are enabled, command, model, timeout, prompt and merge_prompt", name)
	}
	return err
}

// word reads a one-word scalar with no white space or control characters.
// With notFlag it also refuses a leading "-", so a value can never be read
// as a command-line flag.
func word(key string, n *yaml.Node, notFlag bool) (string, error) {
	s := strings.TrimSpace(n.Value)
	bad := n.Kind != yaml.ScalarNode || s == "" || strings.IndexFunc(s, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r)
	}) >= 0 || (notFlag && strings.HasPrefix(s, "-"))
	if bad {
		return "", fmt.Errorf("digest %s must be one word with no spaces, for example: %s", key, wordExample(key))
	}
	return s, nil
}

func wordExample(key string) string {
	if key == "command" {
		return defaultDigestCommand
	}
	return defaultDigestModel
}

func parseDigestTimeout(n *yaml.Node) (time.Duration, error) {
	errBad := fmt.Errorf("digest timeout must be a duration with a unit from 1s to %s, for example: 5m", maxDigestTimeout)
	if n.Kind != yaml.ScalarNode {
		return 0, errBad
	}
	t, err := time.ParseDuration(strings.TrimSpace(n.Value))
	if err != nil || t <= 0 || t > maxDigestTimeout {
		return 0, errBad
	}
	return t, nil
}

func promptPath(key string, n *yaml.Node, home string) (string, error) {
	p := strings.TrimSpace(n.Value)
	if n.Kind != yaml.ScalarNode || p == "" {
		return "", fmt.Errorf("digest %s must be the path of a file, for example: ~/.config/para/import-prompt.md", key)
	}
	return expandHome(p, home), nil
}
