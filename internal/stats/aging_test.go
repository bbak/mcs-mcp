package stats

import (
	"mcs-mcp/internal/jira"
	"reflect"
	"testing"
	"time"
)

func TestCalculateInventoryAge_SideEffectProtection(t *testing.T) {
	// 1. Setup mock data
	issues := []jira.Issue{
		{Key: "WIP-1", Status: "Doing"},
	}

	// Input cycle times: explicitly NOT sorted
	originalPersistence := []float64{50.0, 10.0, 30.0, 5.0, 100.0}
	persistenceCopy := make([]float64, len(originalPersistence))
	copy(persistenceCopy, originalPersistence)

	mappings := map[string]StatusMetadata{
		"Doing": {Tier: "Downstream"},
	}

	// 2. Execute calculation
	_ = CalculateInventoryAge(issues, "", nil, mappings, persistenceCopy, "wip", false, time.Now())

	// 3. Verify side-effect protection
	if !reflect.DeepEqual(persistenceCopy, originalPersistence) {
		t.Errorf("REGRESSION: CalculateInventoryAge mutated the input slice!\nExpected: %v\nGot:      %v", originalPersistence, persistenceCopy)
	}
}

func TestBackflowResetAffectsWIPAge(t *testing.T) {
	// Scenario: Item crosses commitment (weight 2), stays 1 day in Downstream,
	// backflows to Upstream (weight 1) for 1 day, re-enters Downstream for 1 day.
	//
	// Without backflow policy: WIP Age = 2 days (cumulative downstream)
	// With backflow policy:    WIP Age = 1 day  (only post-backflow downstream)

	now := time.Date(2025, 1, 4, 12, 0, 0, 0, time.UTC)
	day0 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC) // Created in Upstream
	day1 := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC) // -> InProgress (Downstream)
	day2 := time.Date(2025, 1, 2, 12, 0, 0, 0, time.UTC) // -> Backlog (Upstream, backflow)
	day3 := time.Date(2025, 1, 3, 12, 0, 0, 0, time.UTC) // -> InProgress (Downstream again)

	transitions := []jira.StatusTransition{
		{ToStatus: "InProgress", ToStatusID: "ip", Date: day1},
		{ToStatus: "Backlog", ToStatusID: "bl", Date: day2},
		{ToStatus: "InProgress", ToStatusID: "ip", Date: day3},
	}

	residency, _ := jira.CalculateResidency(
		transitions, day0, nil,
		"InProgress", "ip",
		nil,
		"Backlog", "bl",
		now,
	)

	issue := jira.Issue{
		Key:             "TEST-1",
		IssueType:       "Story",
		Created:         day0,
		Status:          "InProgress",
		StatusID:        "ip",
		BirthStatus:     "Backlog",
		BirthStatusID:   "bl",
		Transitions:     transitions,
		StatusResidency: residency,
	}

	weights := map[string]int{
		"bl": 1, // Upstream
		"ip": 2, // Downstream (= commitment point)
	}
	mappings := map[string]StatusMetadata{
		"bl": {Tier: "Upstream", Name: "Backlog"},
		"ip": {Tier: "Downstream", Name: "InProgress"},
	}
	commitmentPoint := "ip"
	issues := []jira.Issue{issue}

	// Without backflow reset: cumulative downstream time (~2 days)
	resultsRaw := CalculateInventoryAge(issues, commitmentPoint, weights, mappings, nil, "wip", false, now)
	if len(resultsRaw) != 1 {
		t.Fatalf("expected 1 result without reset, got %d", len(resultsRaw))
	}
	if resultsRaw[0].AgeSinceCommitment == nil {
		t.Fatal("expected AgeSinceCommitment to be set without reset")
	}
	rawAge := *resultsRaw[0].AgeSinceCommitment

	// With backflow reset: only post-backflow downstream time (~1 day)
	resultsReset := CalculateInventoryAge(issues, commitmentPoint, weights, mappings, nil, "wip", true, now)
	if len(resultsReset) != 1 {
		t.Fatalf("expected 1 result with reset, got %d", len(resultsReset))
	}
	if resultsReset[0].AgeSinceCommitment == nil {
		t.Fatal("expected AgeSinceCommitment to be set with reset")
	}
	resetAge := *resultsReset[0].AgeSinceCommitment

	// Raw age should be ~2 days (1 day before backflow + 1 day after)
	if rawAge < 1.5 || rawAge > 2.5 {
		t.Errorf("raw WIP age should be ~2 days, got %.1f", rawAge)
	}

	// Reset age should be ~1 day (only post-backflow)
	if resetAge < 0.5 || resetAge > 1.5 {
		t.Errorf("reset WIP age should be ~1 day, got %.1f", resetAge)
	}

	// The reset must reduce the age
	if resetAge >= rawAge {
		t.Errorf("backflow reset should reduce WIP age: raw=%.1f reset=%.1f", rawAge, resetAge)
	}

	// Total age and upstream days must be identical (full history preserved)
	if resultsRaw[0].TotalAgeSinceCreation != resultsReset[0].TotalAgeSinceCreation {
		t.Errorf("total age should be identical: raw=%.1f reset=%.1f",
			resultsRaw[0].TotalAgeSinceCreation, resultsReset[0].TotalAgeSinceCreation)
	}
	if resultsRaw[0].CumulativeUpstreamDays != resultsReset[0].CumulativeUpstreamDays {
		t.Errorf("upstream days should be identical: raw=%.1f reset=%.1f",
			resultsRaw[0].CumulativeUpstreamDays, resultsReset[0].CumulativeUpstreamDays)
	}
}
