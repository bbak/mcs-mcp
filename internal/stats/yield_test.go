package stats

import (
	"mcs-mcp/internal/jira"
	"testing"
)

func TestCalculateProcessYield(t *testing.T) {
	mappings := map[string]StatusMetadata{
		"Open":      {Tier: "Demand"},
		"Refined":   {Tier: "Upstream"},
		"In Flight": {Tier: "Downstream"},
		"Done":      {Tier: "Finished", Outcome: "delivered"},
		"Discarded": {Tier: "Finished", Outcome: "abandoned_upstream"},
		"Cancelled": {Tier: "Finished", Outcome: "abandoned_downstream"},
	}
	resolutions := map[string]string{
		"Fixed":     "delivered",
		"Duplicate": "abandoned_upstream",
		"Won't Do":  "abandoned_downstream",
	}

	issues := []jira.Issue{
		{Key: "ISS-1", Resolution: "Fixed", IssueType: "Story"},
		{Key: "ISS-2", Resolution: "Fixed", IssueType: "Story"},
		{Key: "ISS-3", Resolution: "Duplicate", IssueType: "Story", StatusResidency: map[string]int64{"Open": 86400}},
		{Key: "ISS-4", Resolution: "Won't Do", IssueType: "Bug", StatusResidency: map[string]int64{"In Flight": 172800}},
	}

	yield := CalculateProcessYield(issues, mappings, resolutions)

	if yield.DeliveredCount != 2 {
		t.Errorf("Expected 2 delivered, got %d", yield.DeliveredCount)
	}
	if yield.AbandonedCount != 2 {
		t.Errorf("Expected 2 abandoned, got %d", yield.AbandonedCount)
	}

	foundUpstream := false
	foundDownstream := false
	for _, lp := range yield.LossPoints {
		if lp.Tier == "Upstream" && lp.Count == 1 {
			foundUpstream = true
		}
		if lp.Tier == "Downstream" && lp.Count == 1 {
			foundDownstream = true
		}
	}

	if !foundUpstream {
		t.Errorf("Expected 1 upstream loss point")
	}
	if !foundDownstream {
		t.Errorf("Expected 1 downstream loss point")
	}
}

func TestCalculateStratifiedYield(t *testing.T) {
	mappings := map[string]StatusMetadata{
		"Open": {Tier: "Demand"},
		"Done": {Tier: "Finished", Outcome: "delivered"},
	}
	resolutions := map[string]string{
		"Fixed":    "delivered",
		"Won't Do": "abandoned_demand",
	}

	issues := []jira.Issue{
		{Key: "STORY-1", Resolution: "Fixed", IssueType: "Story"},
		{Key: "STORY-2", Resolution: "Fixed", IssueType: "Story"},
		{Key: "BUG-1", Resolution: "Won't Do", IssueType: "Bug"},
	}

	strat := CalculateStratifiedYield(issues, mappings, resolutions)

	if len(strat) != 2 {
		t.Errorf("Expected 2 types, got %d", len(strat))
	}

	if strat["Story"].DeliveredCount != 2 {
		t.Errorf("Stories should have 2 delivered, got %d", strat["Story"].DeliveredCount)
	}
	if strat["Bug"].AbandonedCount != 1 {
		t.Errorf("Bugs should have 1 abandoned, got %d", strat["Bug"].AbandonedCount)
	}
}
