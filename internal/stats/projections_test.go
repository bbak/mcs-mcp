package stats

import (
	"testing"
	"time"

	"mcs-mcp/internal/eventlog"
	"mcs-mcp/internal/jira"
)

func TestBuildWIPProjection_TimeTravel(t *testing.T) {
	// Create a dummy timeline
	now := time.Now()
	t0 := now.AddDate(0, 0, -10)
	t1 := now.AddDate(0, 0, -8) // Committed 8 days ago
	t2 := now.AddDate(0, 0, -5)
	t3 := now.AddDate(0, 0, -2) // Finished 2 days ago

	events := []eventlog.IssueEvent{
		{
			IssueKey:  "PROJ-1",
			EventType: eventlog.Created,
			ToStatus:  "Open",
			Timestamp: t0.UnixMicro(),
		},
		{
			IssueKey:  "PROJ-1",
			EventType: eventlog.Change,
			ToStatus:  "Dev",
			Timestamp: t1.UnixMicro(),
		},
		{
			IssueKey:  "PROJ-1",
			EventType: eventlog.Change,
			ToStatus:  "QA",
			Timestamp: t2.UnixMicro(),
		},
		{
			IssueKey:   "PROJ-1",
			EventType:  eventlog.Change,
			ToStatus:   "Done",
			Resolution: "Done",
			Timestamp:  t3.UnixMicro(),
		},
	}

	mappings := map[string]StatusMetadata{
		"Open": {Tier: "Demand"},
		"Dev":  {Tier: "Downstream"},
		"QA":   {Tier: "Downstream"},
		"Done": {Tier: "Finished"},
	}

	// Case 1: View at T1 + 1 day
	refDate1 := t1.Add(24 * time.Hour)
	wip1 := BuildWIPProjection(events, "Dev", mappings, refDate1)

	if len(wip1) != 1 {
		t.Fatalf("Expected 1 WIP item at T1+1, got %d", len(wip1))
	}
	if wip1[0].CurrentStatus != "Dev" {
		t.Errorf("Expected status Dev, got %s", wip1[0].CurrentStatus)
	}

	// Case 2: View at T3 + 1 day
	refDate2 := t3.Add(24 * time.Hour)
	wip2 := BuildWIPProjection(events, "Dev", mappings, refDate2)

	if len(wip2) != 0 {
		t.Errorf("Expected 0 WIP items at T3+1 (Item finished), got %d", len(wip2))
	}

	// Case 3: View at T0 + 1 hour (Before Commitment)
	refDate3 := t0.Add(1 * time.Hour)
	wip3 := BuildWIPProjection(events, "Dev", mappings, refDate3)

	if len(wip3) != 0 {
		t.Errorf("Expected 0 WIP items before commitment, got %d", len(wip3))
	}
}

func TestMapIssueFromEvents_MoveHealing(t *testing.T) {
	// Scenario: Item created in OLD_PROJ (LegacyStatus), moved to NEW_PROJ (Open)
	now := time.Now()
	t0 := now.AddDate(0, 0, -10) // Original Birth
	t2 := now.AddDate(0, 0, -2)  // Transition to 'In Progress'

	// These events simulate what TRANSFORMER produces after healing
	events := []eventlog.IssueEvent{
		{
			IssueKey:  "NEW-1",
			EventType: eventlog.Created,
			ToStatus:  "Open", // Healed entry status
			Timestamp: t0.UnixMicro(),
			IsHealed:  true,
		},
		{
			IssueKey:   "NEW-1",
			EventType:  eventlog.Change,
			FromStatus: "Open",
			ToStatus:   "In Progress",
			Timestamp:  t2.UnixMicro(),
		},
	}

	finished := map[string]bool{"Done": true}
	issue := MapIssueFromEvents(events, finished, now)

	// Verification 1: Age should be 10 days (from T0), status residency should be split correctly
	// T0 to T2 (8 days) in 'Open'
	// T2 to now (2 days) in 'In Progress'

	openRes := issue.StatusResidency["Open"]
	ipRes := issue.StatusResidency["In Progress"]

	// Expected seconds (approx allowed)
	expectedOpen := int64(t2.Sub(t0).Seconds())
	expectedIP := int64(now.Sub(t2).Seconds())

	// Tolerance of 1s
	if openRes < expectedOpen-1 || openRes > expectedOpen+1 {
		t.Errorf("Expected ~%d seconds in Open, got %d", expectedOpen, openRes)
	}
	if ipRes < expectedIP-1 || ipRes > expectedIP+1 {
		t.Errorf("Expected ~%d seconds in In Progress, got %d", expectedIP, ipRes)
	}

	// Verification 2: LegacyStatus (from before move) must NOT exist
	if _, exists := issue.StatusResidency["LegacyStatus"]; exists {
		t.Errorf("LegacyStatus should have been healed away")
	}
}

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

	events := eventlog.TransformIssue(dto)

	mappings := map[string]StatusMetadata{
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
