package eventlog

import (
	"fmt"
	"testing"
	"time"

	"mcs-mcp/internal/jira"
)

type MockJiraClient struct {
	SearchIssuesFunc func(jql string, startAt, maxResults int) (*jira.SearchResponse, error)
}

func (m *MockJiraClient) FindProjects(query string) ([]interface{}, error) { return nil, nil }
func (m *MockJiraClient) FindBoards(projectKey, nameFilter string) ([]interface{}, error) {
	return nil, nil
}
func (m *MockJiraClient) GetBoard(id int) (interface{}, error)                   { return nil, nil }
func (m *MockJiraClient) GetIssueWithHistory(key string) (*jira.IssueDTO, error) { return nil, nil }
func (m *MockJiraClient) GetProject(key string) (interface{}, error)             { return nil, nil }
func (m *MockJiraClient) GetProjectStatuses(key string) (interface{}, error)     { return nil, nil }
func (m *MockJiraClient) GetBoardConfig(id int) (interface{}, error)             { return nil, nil }
func (m *MockJiraClient) GetFilter(id string) (interface{}, error)               { return nil, nil }
func (m *MockJiraClient) SearchIssues(jql string, startAt int, maxResults int) (*jira.SearchResponse, error) {
	return nil, nil
}
func (m *MockJiraClient) SearchIssuesWithHistory(jql string, startAt, maxResults int) (*jira.SearchResponse, error) {
	return m.SearchIssuesFunc(jql, startAt, maxResults)
}

func TestLogProvider_HistoryExpansion(t *testing.T) {
	store := NewEventStore()
	mockJira := &MockJiraClient{}
	p := NewLogProvider(mockJira, store, "")

	sourceID := "PROJ-1"
	jql := "project = PROJ"

	// 1. Setup initial state: One issue with events
	now := time.Now().Truncate(time.Minute)
	initialEvents := []IssueEvent{
		{IssueKey: "PROJ-1", Timestamp: now.UnixMicro(), EventType: "Created"},
	}
	store.Append(sourceID, initialEvents)

	omrc, _ := store.GetMostRecentUpdates(sourceID)
	if omrc.UnixMicro() != now.UnixMicro() {
		t.Errorf("Expected OMRC %v, got %v", now.UnixMicro(), omrc.UnixMicro())
	}

	// 2. Mock Expansion
	past := now.Add(-24 * time.Hour)
	expandJQL := fmt.Sprintf("(%s) AND updated <= \"%s\" ORDER BY updated DESC", jql, now.Format("2006-01-02 15:04"))
	catchUpJQL := fmt.Sprintf("(%s) AND updated > \"%s\" ORDER BY updated ASC", jql, now.Format("2006-01-02 15:04"))

	mockJira.SearchIssuesFunc = func(q string, startAt, maxResults int) (*jira.SearchResponse, error) {
		if q == expandJQL {
			return &jira.SearchResponse{
				Issues: []jira.IssueDTO{
					{
						Key: "PROJ-2",
						Fields: jira.FieldsDTO{
							Updated: past.Format("2006-01-02T15:04:05.000-0700"),
							Created: past.Format("2006-01-02T15:04:05.000-0700"),
						},
						Changelog: &jira.ChangelogDTO{Histories: []jira.HistoryDTO{}},
					},
				},
			}, nil
		}
		if q == catchUpJQL {
			return &jira.SearchResponse{Issues: []jira.IssueDTO{}}, nil
		}
		return nil, fmt.Errorf("unexpected JQL: %s", q)
	}

	// Execute Expansion
	fetched, usedOMRC, err := p.ExpandHistory(sourceID, jql, 1)
	if err != nil {
		t.Fatalf("ExpandHistory failed: %v", err)
	}
	if fetched != 1 {
		t.Errorf("Expected 1 fetched, got %d", fetched)
	}
	if usedOMRC.UnixMicro() != now.UnixMicro() {
		t.Errorf("Expected used OMRC %v, got %v", now.UnixMicro(), usedOMRC.UnixMicro())
	}

	// 3. Verify Store
	if store.Count(sourceID) != 2 {
		t.Errorf("Expected count 2, got %d", store.Count(sourceID))
	}

	newOMRC, _ := store.GetMostRecentUpdates(sourceID)
	if newOMRC.UnixMicro() != past.UnixMicro() {
		t.Errorf("Expected new OMRC %v, got %v", past.UnixMicro(), newOMRC.UnixMicro())
	}
}

func TestLogProvider_MergeStrategy(t *testing.T) {
	store := NewEventStore()
	sourceID := "PROJ-1"
	now := time.Now().Truncate(time.Minute)

	// 1. Initial event for PROJ-1
	store.Append(sourceID, []IssueEvent{
		{IssueKey: "PROJ-1", Timestamp: now.UnixMicro(), EventType: "Created"},
	})

	// 2. Merge fresh history for PROJ-1
	later := now.Add(1 * time.Hour)
	freshEvents := []IssueEvent{
		{IssueKey: "PROJ-1", Timestamp: now.UnixMicro(), EventType: "Created"},
		{IssueKey: "PROJ-1", Timestamp: later.UnixMicro(), EventType: "Change", ToStatus: "In Progress"},
	}

	store.Merge(sourceID, freshEvents)

	// 3. Verify
	events := store.GetEventsInRange(sourceID, time.Time{}, time.Now().Add(2*time.Hour))
	if len(events) != 2 {
		t.Errorf("Expected 2 events, got %d", len(events))
	}
	if events[1].ToStatus != "In Progress" {
		t.Errorf("Expected 'In Progress', got '%s'", events[1].ToStatus)
	}
}
