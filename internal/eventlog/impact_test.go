package eventlog

import (
	"mcs-mcp/internal/jira"
	"mcs-mcp/internal/stats"
	"testing"
	"time"
)

func TestThroughputProjection_BoundaryResolved_Impact(t *testing.T) {
	// Scenario: Item moved into project and resolved 1 day ago (T_move).
	// But it was originally created 2 years ago (T_bio).

	now := time.Now()
	tBio := now.AddDate(-2, 0, 0)
	tMove := now.AddDate(0, 0, -1)

	// We use the internal/jira package to construct the DTO
	dto := jira.IssueDTO{
		Key: "GEN-1",
		Fields: jira.FieldsDTO{
			IssueType: struct {
				Name    string `json:"name"`
				Subtask bool   `json:"subtask"`
			}{Name: "Story"},
			Status: struct {
				ID             string `json:"id"`
				Name           string `json:"name"`
				StatusCategory struct {
					Key string `json:"key"`
				} `json:"statusCategory"`
			}{Name: "Done", ID: "10003"},
			Created: tBio.Format("2006-01-02T15:04:05.000-0700"),
		},
		Changelog: &jira.ChangelogDTO{
			Histories: []jira.HistoryDTO{
				{
					// Boundary Move + Status Change + Resolution
					Created: tMove.Format("2006-01-02T15:04:05.000-0700"),
					Items: []jira.ItemDTO{
						{Field: "Key", FromString: "OLD-1", ToString: "GEN-1"},
						{Field: "workflow", FromString: "old", ToString: "new"},
						{Field: "status", FromString: "Open", From: "1", ToString: "Done", To: "10003"},
						{Field: "resolution", FromString: "", ToString: "Fixed", To: "1"},
					},
				},
			},
		},
	}

	events := TransformIssue(dto)

	mappings := map[string]stats.StatusMetadata{
		"Done": {Tier: "Finished", Outcome: "delivered"},
	}

	throughput := BuildThroughputProjection(events, mappings)

	// We expect EXACTLY 1 throughput bucket.
	// If it was counted at BioBirth AND Move date, we'd have 2 (or a wrong date).
	if len(throughput) != 1 {
		t.Fatalf("Expected 1 throughput bucket, got %d. Events: %+v", len(throughput), events)
	}

	expectedDate := tMove.Format("2006-01-02")
	actualDate := throughput[0].Date.Format("2006-01-02")

	if actualDate != expectedDate {
		t.Errorf("Throughput counted at WRONG date. Expected %s (Move), got %s (BioBirth)", expectedDate, actualDate)
	}
}
