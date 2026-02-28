package stats

import (
	"mcs-mcp/internal/jira"
	"testing"
	"time"
)

func TestCalculateStratifiedStatusPersistence(t *testing.T) {
	issues := []jira.Issue{
		{
			Key:       "S1",
			IssueType: "Story",
			StatusResidency: map[string]int64{
				"Development": 10 * 86400, // 10 days
			},
		},
		{
			Key:       "B1",
			IssueType: "Bug",
			StatusResidency: map[string]int64{
				"Development": 2 * 86400, // 2 days
			},
		},
		{
			Key:       "S2",
			IssueType: "Story",
			StatusResidency: map[string]int64{
				"Development": 8 * 86400, // 8 days
			},
		},
	}

	res := CalculateStratifiedStatusPersistence(issues)

	if len(res) != 2 {
		t.Fatalf("Expected 2 work item types, got %d", len(res))
	}

	// Stories check
	stories, ok := res["Story"]
	if !ok {
		t.Fatal("Expected 'Story' in results")
	}
	foundDev := false
	for _, p := range stories {
		if p.StatusName == "Development" {
			foundDev = true
			if p.P50 != 10.0 { // P50 of [8, 10] is durations[1] = 10
				t.Errorf("Expected Story Development P50 to be 10.0, got %f", p.P50)
			}
		}
	}
	if !foundDev {
		t.Error("Did not find 'Development' status for Stories")
	}

	// Bug check
	bugs, ok := res["Bug"]
	if !ok {
		t.Fatal("Expected 'Bug' in results")
	}
	foundDev = false
	for _, p := range bugs {
		if p.StatusName == "Development" {
			foundDev = true
			if p.P50 != 2.0 {
				t.Errorf("Expected Bug Development P50 to be 2.0, got %f", p.P50)
			}
		}
	}
	if !foundDev {
		t.Error("Did not find 'Development' status for Bugs")
	}
}

func TestCalculateStatusPersistence_Friction(t *testing.T) {
	// ... existing test ...
}

func TestCalculateTierSummary(t *testing.T) {
	mappings := map[string]StatusMetadata{
		"Refining": {Tier: "Upstream"},
		"Coding":   {Tier: "Downstream"},
		"Testing":  {Tier: "Downstream"},
		"Done":     {Tier: "Finished"},
		"Archived": {Tier: "Finished"},
	}

	issues := []jira.Issue{
		{
			Key: "I1",
			StatusResidency: map[string]int64{
				"Refining": 5 * 86400,
				"Coding":   10 * 86400,
				"Testing":  5 * 86400, // Total Downstream for I1: 15
				"Done":     100 * 86400,
			},
		},
		{
			Key: "I2",
			StatusResidency: map[string]int64{
				"Refining": 2 * 86400,
				"Coding":   4 * 86400,
				"Testing":  4 * 86400, // Total Downstream for I2: 8
				"Archived": 200 * 86400,
			},
		},
	}

	summary := CalculateTierSummary(issues, mappings)

	// 1. Check filtering of Finished tier
	if _, ok := summary["Finished"]; ok {
		t.Error("Expected 'Finished' tier to be filtered out of summary")
	}

	// 2. Check Upstream (I1: 5, I2: 2)
	upstream, ok := summary["Upstream"]
	if !ok {
		t.Fatal("Expected 'Upstream' tier summary")
	}
	if upstream.Count != 2 {
		t.Errorf("Expected Upstream count 2, got %d", upstream.Count)
	}
	// P85 of [2, 5] is index int(2*0.85)=1, which is 5.0
	if upstream.P85 != 5.0 {
		t.Errorf("Expected Upstream P85 to be 5.0, got %f", upstream.P85)
	}

	// 3. Check Downstream (I1: 15, I2: 8) -> Aggregation test!
	downstream, ok := summary["Downstream"]
	if !ok {
		t.Fatal("Expected 'Downstream' tier summary")
	}
	if downstream.Count != 2 {
		t.Errorf("Expected Downstream count 2 (issues), got %d", downstream.Count)
	}
	// P85 of [8, 15] is index 1, which is 15.0
	// If aggregation was NOT working, it would have [10, 5, 4, 4] -> durations sorted [4, 4, 5, 10]
	// P85 of 4 items is index int(4*0.85)=3 -> 10.0.
	// So 15.0 proves aggregation is working.
	if downstream.P85 != 15.0 {
		t.Errorf("Expected Downstream P85 to be 15.0 (summed), got %f", downstream.P85)
	}
}
func TestCalculateStatusPersistence_TerminalPreservation(t *testing.T) {
	now := time.Now()
	issues := []jira.Issue{
		{
			Key:            "KAN-4",
			Status:         "Done",
			ResolutionDate: &now,
			StatusResidency: map[string]int64{
				"To Do":       86400, // 1 day
				"In Progress": 86400, // 1 day
				"In Review":   5,     // 5 seconds (Noise)
				"Done":        2,     // 2 seconds (Terminal - should be preserved)
			},
		},
	}

	res := CalculateStatusPersistence(issues)

	foundDone := false
	foundReview := false
	for _, p := range res {
		if p.StatusName == "Done" {
			foundDone = true
		}
		if p.StatusName == "In Review" {
			foundReview = true
		}
	}

	if !foundDone {
		t.Error("Expected terminal status 'Done' to be preserved despite low residency")
	}
	if foundReview {
		t.Error("Expected intermediate status 'In Review' to be filtered out as noise (< 60s)")
	}
}
