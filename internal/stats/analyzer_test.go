package stats

import (
	"mcs-mcp/internal/jira"
	"testing"
	"time"
)

func TestCalculateStatusPersistence(t *testing.T) {
	now := time.Now()
	issues := []jira.Issue{
		{
			Key:     "PROJ-1",
			Created: now.AddDate(0, 0, -10),
			StatusResidency: map[string]float64{
				"Created":     2.0,
				"In Progress": 7.0,
				"Done":        1.0,
			},
			ResolutionDate: func() *time.Time { t := now; return &t }(),
		},
	}

	results := CalculateStatusPersistence(issues)

	// We expect statuses: "Created", "In Progress", "Done"
	// "Created" duration: 10 - 8 = 2 days
	// "In Progress" duration: 8 - 1 = 7 days
	// "Done" duration: 1 - 0 = 1 day

	foundIP := false
	for _, r := range results {
		if r.StatusName == "In Progress" {
			foundIP = true
			if r.P50 != 7.0 {
				t.Errorf("Expected In Progress P50 to be 7.0, got %f", r.P50)
			}
		}
		if r.StatusName == "Created" {
			if r.P50 != 2.0 {
				t.Errorf("Expected Created P50 to be 2.0, got %f", r.P50)
			}
		}
	}

	if !foundIP {
		t.Error("Did not find 'In Progress' status in persistence results")
	}
}

func TestEnrichStatusPersistence(t *testing.T) {
	results := []StatusPersistence{
		{StatusName: "Open"},
		{StatusName: "In Dev"},
	}
	categories := map[string]string{
		"Open":   "to-do",
		"In Dev": "indeterminate",
	}
	mappings := map[string]StatusMetadata{
		"Open": {Tier: "Demand", Role: "active"},
	}

	enriched := EnrichStatusPersistence(results, categories, mappings)

	for _, r := range enriched {
		if r.StatusName == "Open" {
			if r.Role != "active" {
				t.Errorf("Expected Role 'active' for Open, got %s", r.Role)
			}
			if r.Tier != "Demand" {
				t.Errorf("Expected Tier 'Demand' for Open, got %s", r.Tier)
			}
			if r.Interpretation == "" {
				t.Error("Expected interpretation hint for backlog")
			}
		}
		if r.StatusName == "In Dev" {
			if r.Category != "indeterminate" {
				t.Errorf("Expected Category 'indeterminate', got %s", r.Category)
			}
		}
	}
}

func TestSumRangeDuration(t *testing.T) {
	issue := jira.Issue{
		StatusResidency: map[string]float64{
			"In Dev":  5.5,
			"Ready":   2.0,
			"Testing": 3.0,
			"Done":    1.0,
		},
	}

	rangeStatuses := []string{"In Dev", "Testing"}
	duration := SumRangeDuration(issue, rangeStatuses)

	if duration != 8.5 {
		t.Errorf("Expected duration 8.5, got %f", duration)
	}

	// Non-existent status should be ignored
	duration = SumRangeDuration(issue, []string{"In Dev", "Blocked"})
	if duration != 5.5 {
		t.Errorf("Expected duration 5.5, got %f", duration)
	}
}

func TestCalculateProcessYield(t *testing.T) {
	now := time.Now()
	issues := []jira.Issue{
		{
			// Delivered from Downstream
			Key:             "PROJ-1",
			Resolution:      "Fixed",
			Transitions:     []jira.StatusTransition{{ToStatus: "In Progress", Date: now}},
			StatusResidency: map[string]float64{"In Progress": 5.0},
			ResolutionDate:  &now,
		},
		{
			// Abandoned from Upstream
			Key:             "PROJ-2",
			Resolution:      "Won't Do",
			Transitions:     []jira.StatusTransition{{ToStatus: "Refinement", Date: now}},
			StatusResidency: map[string]float64{"Refinement": 10.0},
			ResolutionDate:  &now,
		},
	}

	mappings := map[string]StatusMetadata{
		"Refinement":  {Tier: "Upstream", Role: "active"},
		"In Progress": {Tier: "Downstream", Role: "active"},
	}

	resolutions := map[string]string{
		"Fixed":    "delivered",
		"Won't Do": "abandoned",
	}

	yield := CalculateProcessYield(issues, mappings, resolutions)

	if yield.DeliveredCount != 1 {
		t.Errorf("Expected 1 delivered, got %d", yield.DeliveredCount)
	}
	if yield.AbandonedCount != 1 {
		t.Errorf("Expected 1 abandoned, got %d", yield.AbandonedCount)
	}

	foundUpstream := false
	for _, lp := range yield.LossPoints {
		if lp.Tier == "Upstream" {
			foundUpstream = true
			if lp.Count != 1 {
				t.Errorf("Expected 1 abandoned in Upstream, got %d", lp.Count)
			}
		}
	}
	if !foundUpstream {
		t.Error("Did not find Upstream loss point")
	}
}
