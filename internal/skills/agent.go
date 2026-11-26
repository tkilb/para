package skills

import (
	"errors"
	"os/exec"
)

// Agent represents the detected AI CLI backend.
type Agent string

const (
	AgentClaude Agent = "claude"
	AgentAGY    Agent = "agy"
	AgentNone   Agent = ""
)

// ErrNoAgentFound is returned when neither claude nor agy is found on PATH.
var ErrNoAgentFound = errors.New("neither claude nor agy CLI was found on PATH")

// LookPathFunc is the signature for looking up an executable on PATH.
type LookPathFunc func(file string) (string, error)

// DetectAgent finds whether claude or agy is available on PATH,
// prioritizing claude over agy.
func DetectAgent() (Agent, string, error) {
	return detectAgentWith(exec.LookPath)
}

func detectAgentWith(lookPath LookPathFunc) (Agent, string, error) {
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	if p, err := lookPath("claude"); err == nil && p != "" {
		return AgentClaude, p, nil
	}
	if p, err := lookPath("agy"); err == nil && p != "" {
		return AgentAGY, p, nil
	}
	return AgentNone, "", ErrNoAgentFound
}
