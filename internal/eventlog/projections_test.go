package eventlog

import (
	"testing"
	"time"

	"mcs-mcp/internal/stats"
)

func TestBuildWIPProjection_TimeTravel(t *testing.T) {
	// Create a dummy timeline
	now := time.Now()
	t0 := now.AddDate(0, 0, -10)
	t1 := now.AddDate(0, 0, -8) // Committed 8 days ago
	t2 := now.AddDate(0, 0, -5)
	t3 := now.AddDate(0, 0, -2) // Finished 2 days ago

	events := []IssueEvent{
		{
			IssueKey:  "PROJ-1",
			EventType: Created,
			ToStatus:  "Open",
			Timestamp: t0.UnixMicro(),
		},
		{
			IssueKey:  "PROJ-1",
			EventType: Change,
			ToStatus:  "Dev",
			Timestamp: t1.UnixMicro(),
		},
		{
			IssueKey:  "PROJ-1",
			EventType: Change,
			ToStatus:  "QA",
			Timestamp: t2.UnixMicro(),
		},
		{
			IssueKey:   "PROJ-1",
			EventType:  Change,
			ToStatus:   "Done",
			Resolution: "Done",
			Timestamp:  t3.UnixMicro(),
		},
	}

	mappings := map[string]stats.StatusMetadata{
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
