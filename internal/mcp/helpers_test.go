package mcp

import (
	"errors"
	"testing"

	"mcs-mcp/internal/jira"
)

type mockJiraClient struct {
	jira.Client
	getBoard           func(id int) (interface{}, error)
	getBoardConfig     func(id int) (interface{}, error)
	getFilter          func(id string) (interface{}, error)
	getProjectStatuses func(key string) (interface{}, error)
}

func (m *mockJiraClient) GetBoard(id int) (interface{}, error) {
	if m.getBoard != nil {
		return m.getBoard(id)
	}
	return nil, nil
}

func (m *mockJiraClient) GetBoardConfig(id int) (interface{}, error) {
	if m.getBoardConfig != nil {
		return m.getBoardConfig(id)
	}
	return nil, nil
}

func (m *mockJiraClient) GetFilter(id string) (interface{}, error) {
	if m.getFilter != nil {
		return m.getFilter(id)
	}
	return nil, nil
}

func (m *mockJiraClient) GetProjectStatuses(key string) (interface{}, error) {
	if m.getProjectStatuses != nil {
		return m.getProjectStatuses(key)
	}
	return nil, nil
}

func TestResolveSourceContext_SafeAssertions(t *testing.T) {
	s := &Server{
		jira: &mockJiraClient{
			getBoard: func(id int) (interface{}, error) {
				// Malformed response: not a map
				return []interface{}{"not", "a", "map"}, nil
			},
		},
	}

	_, err := s.resolveSourceContext("123", "board")
	if err == nil {
		t.Error("expected error for malformed board response, got nil")
	}
	if err.Error() != "invalid board config response format from Jira" {
		t.Errorf("unexpected error message: %v", err)
	}

	s.jira.(*mockJiraClient).getBoard = func(id int) (interface{}, error) {
		// Board metadata missing filter, but has location
		return map[string]interface{}{
			"id":       123,
			"location": map[string]interface{}{"projectKey": "PROJ"},
		}, nil
	}
	s.jira.(*mockJiraClient).getBoardConfig = func(id int) (interface{}, error) {
		// Board config has filter
		return map[string]interface{}{
			"filter": map[string]interface{}{"id": "456"},
		}, nil
	}
	s.jira.(*mockJiraClient).getFilter = func(id string) (interface{}, error) {
		return map[string]interface{}{"jql": "project = PROJ"}, nil
	}

	ctx, err := s.resolveSourceContext("123", "board")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx.JQL != "(project = PROJ) AND issuetype not in subtaskIssueTypes()" {
		t.Errorf("unexpected JQL: %s", ctx.JQL)
	}
	if ctx.PrimaryProject != "PROJ" {
		t.Errorf("unexpected project key: %s", ctx.PrimaryProject)
	}

	s.jira.(*mockJiraClient).getBoard = func(id int) (interface{}, error) {
		return map[string]interface{}{"id": 123}, nil
	}
	s.jira.(*mockJiraClient).getBoardConfig = func(id int) (interface{}, error) {
		return nil, errors.New("config error")
	}

	_, err = s.resolveSourceContext("123", "board")
	if err == nil {
		t.Error("expected error when filter missing in metadata and config retrieval fails, got nil")
	}

	s.jira.(*mockJiraClient).getBoard = func(id int) (interface{}, error) {
		return map[string]interface{}{
			"filter": map[string]interface{}{"id": "456"},
		}, nil
	}
	s.jira.(*mockJiraClient).getFilter = func(id string) (interface{}, error) {
		// Filter response not a map
		return "not a map", nil
	}

	_, err = s.resolveSourceContext("123", "board")
	if err == nil {
		t.Error("expected error for malformed filter response, got nil")
	}

	// Test JQL stripping with ORDER BY
	s.jira.(*mockJiraClient).getBoard = func(id int) (interface{}, error) {
		return map[string]interface{}{
			"filter": map[string]interface{}{"id": "456"},
		}, nil
	}
	s.jira.(*mockJiraClient).getFilter = func(id string) (interface{}, error) {
		return map[string]interface{}{"jql": "project = PROJ ORDER BY created DESC"}, nil
	}

	ctx, err = s.resolveSourceContext("123", "board")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expectedJQL := "(project = PROJ) AND issuetype not in subtaskIssueTypes()"
	if ctx.JQL != expectedJQL {
		t.Errorf("expected JQL %q, got %q", expectedJQL, ctx.JQL)
	}
}

func TestGetStatusCategories_SafeAssertions(t *testing.T) {
	s := &Server{
		jira: &mockJiraClient{
			getProjectStatuses: func(key string) (interface{}, error) {
				// Malformed response: not a slice
				return map[string]interface{}{"not": "a slice"}, nil
			},
		},
	}

	// Should not panic
	cats := s.getStatusCategories([]string{"PROJ"})
	if len(cats) != 0 {
		t.Errorf("expected 0 categories, got %d", len(cats))
	}

	s.jira.(*mockJiraClient).getProjectStatuses = func(key string) (interface{}, error) {
		// Malformed element in slice: not a map
		return []interface{}{"not a map"}, nil
	}

	// Should not panic
	cats = s.getStatusCategories([]string{"PROJ"})
	if len(cats) != 0 {
		t.Errorf("expected 0 categories, got %d", len(cats))
	}

	s.jira.(*mockJiraClient).getProjectStatuses = func(key string) (interface{}, error) {
		return []interface{}{
			map[string]interface{}{
				"statuses": "not a slice",
			},
		}, nil
	}

	// Should not panic
	cats = s.getStatusCategories([]string{"PROJ"})
	if len(cats) != 0 {
		t.Errorf("expected 0 categories, got %d", len(cats))
	}
}
