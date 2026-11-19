package config

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDigestDefaults(t *testing.T) {
	const nb = "notebooks:\n  - /tmp\n"
	home := t.TempDir()
	want := Digest{
		Enabled: false,
		Command: "claude",
		Model:   "sonnet",
		Timeout: 5 * time.Minute,
		// Prompt and MergePrompt stay empty: the built-in prompts are used.
	}
	for _, in := range []string{nb, nb + "digest:\n", nb + "digest: {}\n"} {
		cfg, err := Parse([]byte(in), home)
		require.NoError(t, err)
		assert.Equal(t, want, cfg.Digest, "absent, null and empty all give the defaults, and import is off")
	}
}

func TestParseDigest(t *testing.T) {
	const nb = "notebooks:\n  - /tmp\n"
	home := t.TempDir()
	tests := []struct {
		name, yaml string
		check      func(t *testing.T, d Digest)
	}{
		{"enabled", "digest:\n  enabled: true\n", func(t *testing.T, d Digest) {
			assert.True(t, d.Enabled)
			assert.Equal(t, "sonnet", d.Model, "the rest keeps its defaults")
		}},
		{"command and model", "digest:\n  command: /opt/bin/claude\n  model: opus\n", func(t *testing.T, d Digest) {
			assert.Equal(t, "/opt/bin/claude", d.Command)
			assert.Equal(t, "opus", d.Model)
		}},
		{"full model name", "digest:\n  model: claude-sonnet-5-5\n", func(t *testing.T, d Digest) {
			assert.Equal(t, "claude-sonnet-5-5", d.Model)
		}},
		{"timeout", "digest:\n  timeout: 90s\n", func(t *testing.T, d Digest) {
			assert.Equal(t, 90*time.Second, d.Timeout)
		}},
		{"timeout in minutes", "digest:\n  timeout: 10m\n", func(t *testing.T, d Digest) {
			assert.Equal(t, 10*time.Minute, d.Timeout)
		}},
		{"prompt paths expand the home directory", "digest:\n  prompt: ~/p/main.md\n  merge_prompt: ~/p/merge.md\n", func(t *testing.T, d Digest) {
			assert.Equal(t, filepath.Join(home, "p", "main.md"), d.Prompt)
			assert.Equal(t, filepath.Join(home, "p", "merge.md"), d.MergePrompt)
		}},
		{"absolute prompt path", "digest:\n  prompt: /etc/para/main.md\n", func(t *testing.T, d Digest) {
			assert.Equal(t, "/etc/para/main.md", d.Prompt)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Parse([]byte(nb+tt.yaml), home)
			require.NoError(t, err)
			tt.check(t, cfg.Digest)
		})
	}
}

func TestParseDigestRejectsMistakes(t *testing.T) {
	const nb = "notebooks:\n  - /tmp\n"
	tests := []struct {
		name, yaml, wantErr string
	}{
		{"not a mapping", "digest: yes\n", "digest must be a mapping"},
		{"a list", "digest: [a]\n", "digest must be a mapping"},
		{"unknown key", "digest:\n  enable: true\n", `digest entry "enable" is not known`},
		{"enabled is not a boolean", "digest:\n  enabled: maybe\n", "digest enabled must be true or false"},
		{"empty command", "digest:\n  command: \"\"\n", "digest command must be"},
		{"command with a space", "digest:\n  command: claude -p\n", "digest command must be"},
		{"empty model", "digest:\n  model: \"\"\n", "digest model must be"},
		{"model that looks like a flag", "digest:\n  model: --dangerously-skip-permissions\n", "digest model must be"},
		{"model with a space", "digest:\n  model: two words\n", "digest model must be"},
		{"timeout not a duration", "digest:\n  timeout: soon\n", "digest timeout must be"},
		{"timeout without a unit", "digest:\n  timeout: 30\n", "digest timeout must be"},
		{"zero timeout", "digest:\n  timeout: 0s\n", "digest timeout must be"},
		{"negative timeout", "digest:\n  timeout: -5m\n", "digest timeout must be"},
		{"timeout over an hour", "digest:\n  timeout: 2h\n", "digest timeout must be"},
		{"empty prompt path", "digest:\n  prompt: \"\"\n", "digest prompt must be"},
		{"prompt is a list", "digest:\n  prompt: [a]\n", "digest prompt must be"},
		{"empty merge prompt path", "digest:\n  merge_prompt: \"\"\n", "digest merge_prompt must be"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(nb+tt.yaml), t.TempDir())
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
