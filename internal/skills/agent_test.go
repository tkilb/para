package skills

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectAgent(t *testing.T) {
	tests := []struct {
		name          string
		lookups       map[string]string
		expectedAgent Agent
		expectedPath  string
		expectedErr   error
	}{
		{
			name: "claude available",
			lookups: map[string]string{
				"claude": "/usr/local/bin/claude",
			},
			expectedAgent: AgentClaude,
			expectedPath:  "/usr/local/bin/claude",
		},
		{
			name: "agy available when claude is missing",
			lookups: map[string]string{
				"agy": "/home/user/bin/agy",
			},
			expectedAgent: AgentAGY,
			expectedPath:  "/home/user/bin/agy",
		},
		{
			name: "both available prefers claude",
			lookups: map[string]string{
				"claude": "/usr/local/bin/claude",
				"agy":    "/home/user/bin/agy",
			},
			expectedAgent: AgentClaude,
			expectedPath:  "/usr/local/bin/claude",
		},
		{
			name:          "neither available",
			lookups:       map[string]string{},
			expectedAgent: AgentNone,
			expectedPath:  "",
			expectedErr:   ErrNoAgentFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockLookPath := func(file string) (string, error) {
				if p, ok := tt.lookups[file]; ok {
					return p, nil
				}
				return "", errors.New("not found")
			}

			agent, path, err := detectAgentWith(mockLookPath)
			if tt.expectedErr != nil {
				require.ErrorIs(t, err, tt.expectedErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedPath, path)
			}
			assert.Equal(t, tt.expectedAgent, agent)
		})
	}
}
