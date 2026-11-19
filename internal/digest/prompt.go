package digest

import (
	_ "embed"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// MaxPromptBytes is the largest prompt file read. The prompt is passed as a
// command-line argument, so it must stay well under the system limit.
const MaxPromptBytes = 64 << 10

var (
	// ErrEmptyPrompt reports a prompt file with nothing but white space.
	ErrEmptyPrompt = errors.New("the prompt file is empty")
	// ErrPromptTooLarge reports a prompt file over MaxPromptBytes.
	ErrPromptTooLarge = errors.New("the prompt file is too large")
)

// DefaultPrompt and DefaultMergePrompt are the prompts built into para. They
// are used unless the config names a file, so they move with the program and
// cannot drift from the code that reads the agent's answer.
var (
	//go:embed prompts/import-prompt.md
	DefaultPrompt string
	//go:embed prompts/import-merge-prompt.md
	DefaultMergePrompt string
)

// ResolvePrompt returns def when path is empty, and otherwise the whole file
// at path, as ReadPrompt does. A configured file that cannot be read is an
// error; it never falls back to def.
func ResolvePrompt(path, def string) (string, error) {
	if path == "" {
		return def, nil
	}
	return ReadPrompt(path)
}

// ReadPrompt reads a prompt file whole. It returns ErrEmptyPrompt for a file
// of only white space and ErrPromptTooLarge above MaxPromptBytes.
func ReadPrompt(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("read the prompt: %w", err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, MaxPromptBytes+1))
	if err != nil {
		return "", fmt.Errorf("read the prompt: %w", err)
	}
	if len(data) > MaxPromptBytes {
		return "", ErrPromptTooLarge
	}
	if strings.TrimSpace(string(data)) == "" {
		return "", ErrEmptyPrompt
	}
	return string(data), nil
}
