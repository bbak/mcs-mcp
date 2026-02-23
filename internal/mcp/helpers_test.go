package mcp

import (
	"errors"
	"testing"

	"mcs-mcp/internal/jira"
)

type mockJiraClient struct {
	jira.Client
	getBoard           func(id int) (any, error)
	getBoardConfig     func(id int) (any, error)
	getFilter          func(id string) (any, error)
	getProjectStatuses func(key string) (any, error)
}

func (m *mockJiraClient) GetBoard(id int) (any, error) {
	if m.getBoard != nil {
		return m.getBoard(id)
	}
	return nil, nil
}

func (m *mockJiraClient) GetBoardConfig(id int) (any, error) {
	if m.getBoardConfig != nil {
		return m.getBoardConfig(id)
	}
	return nil, nil
}

func (m *mockJiraClient) GetFilter(id string) (any, error) {
	if m.getFilter != nil {
		return m.getFilter(id)
	}
	return nil, nil
}

func (m *mockJiraClient) GetProjectStatuses(key string) (any, error) {
	if m.getProjectStatuses != nil {
		return m.getProjectStatuses(key)
	}
	return nil, nil
}

func TestResolveSourceContext_SafeAssertions(t *testing.T) {
	s := &Server{
		jira: &mockJiraClient{
			getBoard: func(id int) (any, error) {
				// Malformed response: not a map
				return []any{"not", "a", "map"}, nil
			},
		},
	}

	_, err := s.resolveSourceContext("123", 456)
	if err == nil {
		t.Error("expected error for malformed board response, got nil")
	}
	if err.Error() != "invalid board config response format from Jira" {
		t.Errorf("unexpected error message: %v", err)
	}

	s.jira.(*mockJiraClient).getBoard = func(id int) (any, error) {
		// Board metadata missing filter, but has location
		return map[string]any{
			"id":       123,
			"location": map[string]any{"projectKey": "PROJ"},
		}, nil
	}
	s.jira.(*mockJiraClient).getBoardConfig = func(id int) (any, error) {
		// Board config has filter
		return map[string]any{
			"filter": map[string]any{"id": "456"},
		}, nil
	}
	s.jira.(*mockJiraClient).getFilter = func(id string) (any, error) {
		return map[string]any{"jql": "project = PROJ"}, nil
	}

	ctx, err := s.resolveSourceContext("PROJ", 123)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx.JQL != "(project = PROJ) AND issuetype not in subtaskIssueTypes()" {
		t.Errorf("unexpected JQL: %s", ctx.JQL)
	}
	if ctx.ProjectKey != "PROJ" {
		t.Errorf("unexpected project key: %s", ctx.ProjectKey)
	}

	s.jira.(*mockJiraClient).getBoard = func(id int) (any, error) {
		return map[string]any{"id": 123}, nil
	}
	s.jira.(*mockJiraClient).getBoardConfig = func(id int) (any, error) {
		return nil, errors.New("config error")
	}
	_, err = s.resolveSourceContext("123", 123)
	if err == nil {
		t.Error("expected error when filter missing in metadata and config retrieval fails, got nil")
	}

	s.jira.(*mockJiraClient).getBoard = func(id int) (any, error) {
		return map[string]any{
			"filter": map[string]any{"id": "456"},
		}, nil
	}
	s.jira.(*mockJiraClient).getFilter = func(id string) (any, error) {
		// Filter response not a map
		return "not a map", nil
	}
	_, err = s.resolveSourceContext("123", 123)
	if err == nil {
		t.Error("expected error for malformed filter response, got nil")
	}

	// Test JQL stripping with ORDER BY
	s.jira.(*mockJiraClient).getBoard = func(id int) (any, error) {
		return map[string]any{
			"filter": map[string]any{"id": "456"},
		}, nil
	}
	s.jira.(*mockJiraClient).getFilter = func(id string) (any, error) {
		return map[string]any{"jql": "project = PROJ ORDER BY created DESC"}, nil
	}

	ctx, err = s.resolveSourceContext("PROJ", 123)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expectedJQL := "(project = PROJ) AND issuetype not in subtaskIssueTypes()"
	if ctx.JQL != expectedJQL {
		t.Errorf("expected JQL %q, got %q", expectedJQL, ctx.JQL)
	}
}
