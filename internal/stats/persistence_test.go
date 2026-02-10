package stats

import (
	"mcs-mcp/internal/jira"
	"testing"
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
