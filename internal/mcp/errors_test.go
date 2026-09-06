package mcp

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"mcs-mcp/internal/jira"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// resultText extracts the text the agent actually sees.
func resultText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	var b strings.Builder
	for _, content := range result.Content {
		text, ok := content.(*mcp.TextContent)
		if !ok {
			t.Fatalf("unexpected content type %T", content)
		}
		b.WriteString(text.Text)
	}
	return b.String()
}

// TestAgentGuidance pins the distinction the agent has to act on: an
// operational failure must stop the workflow, while anything else must not
// invent a stop instruction.
func TestAgentGuidance(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantStop    bool
		wantPhrases []string
	}{
		{
			name:        "auth failure tells the user to fix credentials",
			err:         &jira.APIError{Kind: jira.KindAuth, Status: 401, Operation: "project search"},
			wantStop:    true,
			wantPhrases: []string{"JIRA_TOKEN", "rejected the credentials"},
		},
		{
			name:        "not found points at the URL and dialect",
			err:         &jira.APIError{Kind: jira.KindNotFound, Status: 404, Operation: "project search"},
			wantStop:    true,
			wantPhrases: []string{"JIRA_URL", "JIRA_DEPLOYMENT"},
		},
		{
			name:        "transport failure points at connectivity",
			err:         &jira.APIError{Kind: jira.KindTransport, Operation: "project search"},
			wantStop:    true,
			wantPhrases: []string{"could not be reached"},
		},
		{
			// A soft stop: no STOP banner, but the agent must not read "later"
			// as a few seconds and busy-retry into a rate limit.
			name:        "unavailable defers to a much later attempt without stopping hard",
			err:         &jira.APIError{Kind: jira.KindUnavailable, Status: 503, Operation: "project search"},
			wantStop:    false,
			wantPhrases: []string{"Do NOT retry now", "in an hour"},
		},
		{
			name: "an ordinary error gets no guidance",
			err:  errors.New("no workflow mapping confirmed"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := agentGuidance(tc.err)

			if len(tc.wantPhrases) == 0 {
				if got != "" {
					t.Errorf("expected no guidance, got %q", got)
				}
				return
			}
			if strings.Contains(got, "STOP.") != tc.wantStop {
				t.Errorf("STOP presence = %v, want %v; guidance: %s", !tc.wantStop, tc.wantStop, got)
			}
			for _, phrase := range tc.wantPhrases {
				if !strings.Contains(got, phrase) {
					t.Errorf("guidance missing %q; got: %s", phrase, got)
				}
			}
		})
	}
}

// TestAgentGuidance_WrappedError confirms the classification survives the
// wrapping that happens between the client and the tool layer.
func TestAgentGuidance_WrappedError(t *testing.T) {
	wrapped := fmt.Errorf("fetching projects: %w",
		&jira.APIError{Kind: jira.KindAuth, Status: 401, Operation: "project search"})

	if !strings.Contains(agentGuidance(wrapped), "STOP.") {
		t.Error("a wrapped APIError must still produce stop guidance")
	}
	if !isOperationalFailure(wrapped) {
		t.Error("a wrapped APIError must still count as an operational failure")
	}
}

// TestFormatToolError confirms the guidance reaches the agent, not just the log.
func TestFormatToolError(t *testing.T) {
	result := formatToolError(&jira.APIError{
		Kind: jira.KindAuth, Status: 401, URL: "https://jira.example.com", Operation: "project search",
	})

	if !result.IsError {
		t.Error("IsError must be set")
	}
	text := resultText(t, result)
	if !strings.Contains(text, "status 401") {
		t.Errorf("the underlying error must be preserved; got: %s", text)
	}
	if !strings.Contains(text, "STOP.") {
		t.Errorf("the guidance must be appended; got: %s", text)
	}
}
