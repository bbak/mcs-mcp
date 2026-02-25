package stats

import (
	"mcs-mcp/internal/jira"
	"testing"
	"time"
)

func TestCalculateWIPRunChart_Day1Rule(t *testing.T) {
	windowStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	windowEnd := time.Date(2024, 1, 3, 23, 59, 59, 0, time.UTC)
	window := NewAnalysisWindow(windowStart, windowEnd, "day", time.Time{})

	// Mappings no longer needed explicitly inside CalculateWIPRunChart

	issues := []jira.Issue{
		{
			Key: "PROJ-1",
			Transitions: []jira.StatusTransition{
				{ToStatus: "In Progress", Date: time.Date(2024, 1, 2, 10, 0, 0, 0, time.UTC)}, // Starts Day 2
				{ToStatus: "Done", Date: time.Date(2024, 1, 2, 16, 0, 0, 0, time.UTC)},        // Finishes Day 2
			},
		},
		{
			Key: "PROJ-2",
			Transitions: []jira.StatusTransition{
				{ToStatus: "In Progress", Date: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)}, // Starts Day 1
				{ToStatus: "Done", Date: time.Date(2024, 1, 3, 10, 0, 0, 0, time.UTC)},        // Finishes Day 3
			},
		},
	}

	mappings := map[string]StatusMetadata{
		"In Progress": {Tier: "Downstream"},
		"Done":        {Tier: "Finished"},
	}

	weights := map[string]int{
		"In Progress": 1,
		"Done":        2,
	}

	chart := CalculateWIPRunChart(issues, window, "In Progress", weights, mappings)

	if len(chart) != 3 {
		t.Fatalf("Expected 3 days in chart, got %d", len(chart))
	}

	// Day 1: Only PROJ-2 is active (since it didn't finish today)
	if chart[0].Count != 1 {
		t.Errorf("Day 1 expected WIP 1, got %d", chart[0].Count)
	}

	// Day 2: PROJ-2 is still active. PROJ-1 started AND finished today.
	// Due to the Day 1 rule, PROJ-1 MUST be counted.
	if chart[1].Count != 2 {
		t.Errorf("Day 2 expected WIP 2 (PROJ-2 active, PROJ-1 same-day), got %d", chart[1].Count)
	}

	// Day 3: PROJ-2 finishes during today, so by the End-Of-Day snapshot rule, it is NOT counted for Day 3.
	// (Unless it had started on Day 3 too, triggering the Day 1 rule).
	if chart[2].Count != 0 {
		t.Errorf("Day 3 expected WIP 0, got %d", chart[2].Count)
	}
}

func TestAnalyzeHistoricalWIP(t *testing.T) {
	windowStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	// 9 weeks window (8 weeks stable, 1 week spike)
	windowEnd := time.Date(2024, 3, 3, 23, 59, 59, 0, time.UTC)
	window := NewAnalysisWindow(windowStart, windowEnd, "day", time.Time{})

	// Mappings no longer needed directly

	// Create a stable baseline of 5 items
	var issues []jira.Issue
	for i := 0; i < 5; i++ {
		issues = append(issues, jira.Issue{
			Key: "PROJ-Stable",
			Transitions: []jira.StatusTransition{
				{ToStatus: "In Progress", Date: time.Date(2023, 12, 1, 0, 0, 0, 0, time.UTC)}, // Started way before
			},
		})
	}

	// Create a massive spike in Week 9
	for i := 0; i < 20; i++ {
		issues = append(issues, jira.Issue{
			Key: "PROJ-Spike",
			Transitions: []jira.StatusTransition{
				{ToStatus: "In Progress", Date: time.Date(2024, 2, 28, 10, 0, 0, 0, time.UTC)}, // Wednesday Week 9
			},
		})
	}

	mappings := map[string]StatusMetadata{
		"In Progress": {Tier: "Downstream"},
		"Done":        {Tier: "Finished"},
	}

	weights := map[string]int{
		"In Progress": 1,
		"Done":        2,
	}

	result := AnalyzeHistoricalWIP(issues, window, "In Progress", weights, mappings)

	if len(result.RunChart) != 63 {
		t.Fatalf("Expected 63 days in run chart, got %d", len(result.RunChart))
	}

	// Should have 9 samples limit calculations since it's 63 days = 9 weeks
	if len(result.XmR.Values) != 9 {
		t.Fatalf("Expected 9 weekly samples in XmR, got %d", len(result.XmR.Values))
	}

	// Since it spiked to 25 and AmR is likely 0 initially or suddenly jumps:
	// Let's just check the status is unstable
	if result.Status != "unstable" {
		t.Errorf("Expected status unstable due to the spike, got %s", result.Status)
	}

	// First two weeks should sample a WIP of 5
	if result.XmR.Values[0] != 5.0 || result.XmR.Values[1] != 5.0 {
		t.Errorf("Expected first weeks to be 5, got %v", result.XmR.Values)
	}

	// Ensure limits are projected back to the daily signals correctly
	hasOutlier := false
	for _, sig := range result.XmR.Signals {
		if sig.Type == "outlier" {
			hasOutlier = true
			break
		}
	}

	if !hasOutlier {
		t.Errorf("Expected daily signals to contain outliers")
	}
}
