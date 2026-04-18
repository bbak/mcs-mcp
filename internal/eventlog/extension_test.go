package eventlog

import (
	"testing"
	"time"

	"mcs-mcp/internal/jira"
)

type MockJiraClient struct {
	SearchIssuesFunc func(jql string, startAt, maxResults int) (*jira.SearchResponse, error)
}

func (m *MockJiraClient) FindProjects(query string) ([]any, error) { return nil, nil }
func (m *MockJiraClient) FindBoards(projectKey, nameFilter string) ([]any, error) {
	return nil, nil
}
func (m *MockJiraClient) GetBoard(id int) (any, error)                           { return nil, nil }
func (m *MockJiraClient) GetIssueWithHistory(key string) (*jira.IssueDTO, error) { return nil, nil }
func (m *MockJiraClient) GetProject(key string) (any, error)                     { return nil, nil }
func (m *MockJiraClient) GetProjectStatuses(key string) (any, error)             { return nil, nil }
func (m *MockJiraClient) GetBoardConfig(id int) (any, error)                     { return nil, nil }
func (m *MockJiraClient) GetFilter(id string) (any, error)                       { return nil, nil }
func (m *MockJiraClient) SearchIssues(jql string, startAt int, maxResults int) (*jira.SearchResponse, error) {
	return m.SearchIssuesFunc(jql, startAt, maxResults)
}
func (m *MockJiraClient) GetRegistry(projectKey string) (*jira.NameRegistry, error) { return nil, nil }

func TestLogProvider_MergeStrategy(t *testing.T) {
	now := time.Now().Truncate(time.Minute)
	later := now.Add(1 * time.Hour)
	endOfTime := later.Add(1 * time.Hour)

	store := NewEventStore(func() time.Time { return endOfTime })
	sourceID := "PROJ-1"

	// 1. Initial event for PROJ-1
	store.Append(sourceID, []IssueEvent{
		{IssueKey: "PROJ-1", Timestamp: now.UnixMicro(), EventType: "Created"},
	})

	// 2. Merge fresh history for PROJ-1
	freshEvents := []IssueEvent{
		{IssueKey: "PROJ-1", Timestamp: now.UnixMicro(), EventType: "Created"},
		{IssueKey: "PROJ-1", Timestamp: later.UnixMicro(), EventType: "Change", ToStatus: "In Progress"},
	}

	store.Merge(sourceID, freshEvents)

	// 3. Verify
	events := store.GetIssuesInRange(sourceID, time.Time{}, time.Now().Add(2*time.Hour))
	if len(events) != 2 {
		t.Errorf("Expected 2 events, got %d", len(events))
	}
	if events[1].ToStatus != "In Progress" {
		t.Errorf("Expected 'In Progress', got '%s'", events[1].ToStatus)
	}
}
