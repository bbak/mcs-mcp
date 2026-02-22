package mcp

import (
	"testing"
	"time"

	"mcs-mcp/internal/jira"
	"mcs-mcp/internal/stats"
)

func TestAnalyticalIntegrity_TemporalOrdering(t *testing.T) {
	// Setup Server with minimal context
	s := &Server{
		activeMapping: map[string]stats.StatusMetadata{
			"Todo":        {Tier: "Demand"},
			"In Progress": {Tier: "Downstream", Outcome: "delivered"},
			"Done":        {Tier: "Finished", Outcome: "delivered"},
		},
		activeResolutions: map[string]string{
			"Done": "delivered",
		},
		activeStatusOrder: []string{"Todo", "In Progress", "Done"},
		activeSourceID:    "PROJ_1",
	}

	now := time.Now()
	t1 := now.AddDate(0, 0, -10)
	t2 := now.AddDate(0, 0, -9)
	t3 := now.AddDate(0, 0, -8)

	// Create issues with specific resolution dates and interleaved types
	// If the order is broken (clustered by type), we will see Story (T1), Story (T3), Bug (T2)
	// If the order is correct (temporal), we will see Story (T1), Bug (T2), Story (T3)
	issues := []jira.Issue{
		{
			Key:            "PROJ-1",
			IssueType:      "Story",
			Status:         "Done",
			Resolution:     "Done",
			ResolutionDate: &t1,
			StatusResidency: map[string]int64{
				"In Progress": 86400 * 2, // 2 days
			},
		},
		{
			Key:            "PROJ-2",
			IssueType:      "Bug",
			Status:         "Done",
			Resolution:     "Done",
			ResolutionDate: &t2,
			StatusResidency: map[string]int64{
				"In Progress": 86400 * 1, // 1 day
			},
		},
		{
			Key:            "PROJ-3",
			IssueType:      "Story",
			Status:         "Done",
			Resolution:     "Done",
			ResolutionDate: &t3,
			StatusResidency: map[string]int64{
				"In Progress": 86400 * 3, // 3 days
			},
		},
	}

	// 1. Test getCycleTimes (The fix)
	cycleTimes, matchedIssues := s.getCycleTimes("PROJ", 1, issues, "In Progress", "Done", nil)

	if len(cycleTimes) != 3 {
		t.Fatalf("expected 3 cycle times, got %d", len(cycleTimes))
	}

	// Expected order: T1 (2 days), T2 (1 day), T3 (3 days)
	expectedTimes := []float64{2.0, 1.0, 3.0}
	expectedKeys := []string{"PROJ-1", "PROJ-2", "PROJ-3"}

	for i, ct := range cycleTimes {
		if ct != expectedTimes[i] {
			t.Errorf("at index %d: expected cycle time %v, got %v", i, expectedTimes[i], ct)
		}
		if matchedIssues[i].Key != expectedKeys[i] {
			t.Errorf("at index %d: expected issue key %s, got %s (REGRESSION: Chronological order lost!)", i, expectedKeys[i], matchedIssues[i].Key)
		}
	}

	// 2. Test map iteration randomness protection (re-run a few times)
	for try := 0; try < 10; try++ {
		cts, matched := s.getCycleTimes("PROJ", 1, issues, "In Progress", "Done", nil)
		if matched[1].Key != "PROJ-2" {
			t.Fatalf("map iteration or type clustering broke order on try %d", try)
		}
		if len(cts) != 3 {
			t.Fatal("issue count mismatch")
		}
	}
}
