package stats

import (
	"mcs-mcp/internal/jira"
	"testing"
	"time"
)

func TestGetStratifiedThroughput(t *testing.T) {
	now := time.Now()
	// Monday of current week
	monday := SnapToStart(now, "week")

	issues := []jira.Issue{
		{Key: "S1", IssueType: "Story", ResolutionDate: &now},                                                               // This week
		{Key: "B1", IssueType: "Bug", ResolutionDate: &now},                                                                 // This week
		{Key: "S2", IssueType: "Story", ResolutionDate: func() *time.Time { tt := monday.AddDate(0, 0, -2); return &tt }()}, // Last week (Saturday)
	}

	mappings := map[string]StatusMetadata{
		"Done": {Tier: "Finished", Outcome: "delivered"},
	}
	resolutions := map[string]string{
		"Fixed": "delivered",
	}

	// 2-week window ending now
	window := NewAnalysisWindow(monday.AddDate(0, 0, -7), now, "week", time.Time{})

	res := GetStratifiedThroughput(issues, window, resolutions, mappings)

	// Since window starts at monday-7 and ends at now, it should have 2 buckets: Last Week and This Week.
	if len(res.Pooled) != 2 {
		t.Fatalf("Expected 2 buckets, got %d", len(res.Pooled))
	}

	// This week (Index 1): S1 + B1 = 2
	if res.Pooled[1] != 2 {
		t.Errorf("Expected 2 items in current week, got %d", res.Pooled[1])
	}
	// Last week (Index 0): S2 = 1
	if res.Pooled[0] != 1 {
		t.Errorf("Expected 1 item in last week, got %d", res.Pooled[0])
	}

	// Stratified check
	if res.ByType["Story"][1] != 1 || res.ByType["Bug"][1] != 1 {
		t.Errorf("Stratified counts for current week mismatch: %+v", res.ByType)
	}
	if res.ByType["Story"][0] != 1 {
		t.Errorf("Stratified counts for last week mismatch: %+v", res.ByType)
	}
}

func TestGetStratifiedThroughput_DayBucket(t *testing.T) {
	now := time.Now()
	today := SnapToStart(now, "day")

	issues := []jira.Issue{
		{Key: "I1", IssueType: "Story", ResolutionDate: &now},
	}

	window := NewAnalysisWindow(today, now, "day", time.Time{})
	res := GetStratifiedThroughput(issues, window, nil, nil)

	if len(res.Pooled) != 1 {
		t.Fatalf("Expected 1 bucket, got %d", len(res.Pooled))
	}
	if res.Pooled[0] != 1 {
		t.Errorf("Expected 1 item, got %d", res.Pooled[0])
	}
}
