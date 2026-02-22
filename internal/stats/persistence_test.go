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
	now := time.Now()
	issues := []jira.Issue{
		{
			Key:     "PROJ-1",
			Created: now.AddDate(0, 0, -10),
			StatusResidency: map[string]int64{
				"In Progress": 10 * 86400,
			},
			BlockedResidency: map[string]int64{
				"In Progress": 2 * 86400, // 2 days blocked
			},
		},
		{
			Key:     "PROJ-2",
			Created: now.AddDate(0, 0, -10),
			StatusResidency: map[string]int64{
				"In Progress": 10 * 86400,
			},
			BlockedResidency: map[string]int64{
				"In Progress": 4 * 86400, // 4 days blocked
			},
		},
	}

	results := CalculateStatusPersistence(issues)

	var ipStatus *StatusPersistence
	for i := range results {
		if results[i].StatusName == "In Progress" {
			ipStatus = &results[i]
		}
	}

	if ipStatus == nil {
		t.Fatal("Expected 'In Progress' status")
	}

	// BlockedCount should be 2
	if ipStatus.BlockedCount != 2 {
		t.Errorf("Expected BlockedCount 2, got %d", ipStatus.BlockedCount)
	}

	// BlockedP50 should be around 3.0 (avg of 2 and 4 index 1 of [2, 4])
	// Actually index 0.50 of 2 is 1 (bd[1]) which is 4.0
	if ipStatus.BlockedP50 != 4.0 {
		t.Errorf("Expected BlockedP50 4.0, got %f", ipStatus.BlockedP50)
	}
}
