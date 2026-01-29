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
			StatusResidency: map[string]int64{
				"Created":     2 * 86400,
				"In Progress": 7 * 86400,
				"Done":        1 * 86400,
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
		StatusResidency: map[string]int64{
			"In Dev":  int64(5.5 * 86400),
			"Ready":   int64(2.0 * 86400),
			"Testing": int64(3.0 * 86400),
			"Done":    int64(1.0 * 86400),
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
			StatusResidency: map[string]int64{"In Progress": 5 * 86400},
			ResolutionDate:  &now,
		},
		{
			// Abandoned from Upstream
			Key:             "PROJ-2",
			Resolution:      "Won't Do",
			Transitions:     []jira.StatusTransition{{ToStatus: "Refinement", Date: now}},
			StatusResidency: map[string]int64{"Refinement": 10 * 86400},
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

func TestCalculateStatusAging(t *testing.T) {
	now := time.Now()
	wipIssues := []jira.Issue{
		{
			Key:         "WIP-1",
			IssueType:   "Story",
			Summary:     "Busy item",
			Status:      "Development",
			Transitions: []jira.StatusTransition{{ToStatus: "Development", Date: now.Add(-1 * time.Hour)}}, // Entered 1h ago
		},
	}

	persistence := []StatusPersistence{
		{StatusName: "Development", P50: 1.0, P85: 5.0},
	}

	results := CalculateStatusAging(wipIssues, persistence)

	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}

	// 1h = 1/24 days approx 0.0416. Ceil(0.04*10)/10 = 0.1
	if results[0].DaysInStatus != 0.1 {
		t.Errorf("Expected 0.1 days for 1h residency, got %f", results[0].DaysInStatus)
	}
}

func TestCalculateInventoryAgeExecution(t *testing.T) {
	now := time.Now()
	wipIssues := []jira.Issue{
		{
			Key:             "COM-1",
			Status:          "In Dev",
			Created:         now.AddDate(0, 0, -10),
			StatusResidency: map[string]int64{"In Dev": 5 * 86400, "Refinement": 3 * 86400, "Created": 2 * 86400},
			Transitions: []jira.StatusTransition{
				{ToStatus: "Refinement", Date: now.AddDate(0, 0, -8)},
				{ToStatus: "In Dev", Date: now.AddDate(0, 0, -5)}, // Commitment point!
			},
		},
		{
			Key:             "DEM-1",
			Status:          "Backlog",
			Created:         now.AddDate(0, 0, -10),
			StatusResidency: map[string]int64{"Backlog": 10 * 86400},
			// Not yet started
		},
	}

	statusWeights := map[string]int{
		"Backlog":    1,
		"Refinement": 1,
		"In Dev":     3, // Commitment
	}
	history := []float64{2.0, 5.0, 10.0}
	mappings := map[string]StatusMetadata{
		"In Dev": {Tier: "Downstream", Role: "active"},
	}

	// Test WIP Age
	results := CalculateInventoryAge(wipIssues, "In Dev", statusWeights, mappings, history, "wip")

	if len(results) != 1 {
		t.Errorf("Expected 1 result in WIP mode, got %d", len(results))
	}

	for _, r := range results {
		if r.Key == "COM-1" {
			if r.AgeSinceCommitment == nil {
				t.Error("Expected AgeSinceCommitment for COM-1 (WIP mode), got nil")
			}
		} else {
			t.Errorf("Unexpected item in WIP results: %s", r.Key)
		}
	}

	// Test Total Age
	resultsTotal := CalculateInventoryAge(wipIssues, "In Dev", statusWeights, mappings, history, "total")
	for _, r := range resultsTotal {
		if r.Key == "DEM-1" {
			if r.TotalAgeSinceCreation < 9.9 { // ~10 days
				t.Errorf("Expected TotalAgeSinceCreation around 10.0 for DEM-1, got %f", r.TotalAgeSinceCreation)
			}
		}
	}
}

func TestProposeSemantics(t *testing.T) {
	now := time.Now()
	issues := []jira.Issue{
		{
			Key:    "PROJ-1",
			Status: "In Dev",
			Transitions: []jira.StatusTransition{
				{FromStatus: "Backlog", ToStatus: "Refinement", Date: now.AddDate(0, 0, -5)},
				{FromStatus: "Refinement", ToStatus: "Ready for Dev", Date: now.AddDate(0, 0, -3)},
				{FromStatus: "Ready for Dev", ToStatus: "In Dev", Date: now.AddDate(0, 0, -2)},
			},
		},
	}

	persistence := []StatusPersistence{
		{StatusName: "Backlog", P50: 10.0},
		{StatusName: "Refinement", P50: 2.0},
		{StatusName: "Ready for Dev", P50: 1.0},
		{StatusName: "In Dev", P50: 5.0},
		{StatusName: "Done", P50: 0.0},
	}

	proposal := ProposeSemantics(issues, persistence)

	// Verify "Backlog" is Demand (detected as first entry point)
	if proposal["Backlog"].Tier != "Demand" {
		t.Errorf("Expected Backlog to be 'Demand', got %s", proposal["Backlog"].Tier)
	}

	// Verify "Ready for Dev" is a queue (detected by pattern)
	if proposal["Ready for Dev"].Role != "queue" {
		t.Errorf("Expected 'Ready for Dev' to have role 'queue', got %s", proposal["Ready for Dev"].Role)
	}

	// Verify "In Dev" is Downstream
	if proposal["In Dev"].Tier != "Downstream" {
		t.Errorf("Expected 'In Dev' to be 'Downstream', got %s", proposal["In Dev"].Tier)
	}

	// Verify User-specified role constraints: Demand must be 'queue'
	if proposal["Backlog"].Role != "queue" {
		t.Errorf("Expected 'Backlog' (Demand) to have role 'queue', got %s", proposal["Backlog"].Role)
	}
}

func TestDiscoverStatusOrder(t *testing.T) {
	now := time.Now()
	issues := []jira.Issue{
		{
			Key:    "I-1",
			Status: "D",
			Transitions: []jira.StatusTransition{
				{FromStatus: "A", ToStatus: "B", Date: now.Add(10 * time.Minute)},
				{FromStatus: "B", ToStatus: "C", Date: now.Add(20 * time.Minute)},
				{FromStatus: "C", ToStatus: "D", Date: now.Add(30 * time.Minute)},
			},
		},
		{
			Key:    "I-2",
			Status: "D",
			Transitions: []jira.StatusTransition{
				{FromStatus: "A", ToStatus: "C", Date: now.Add(15 * time.Minute)}, // Jump over B
				{FromStatus: "C", ToStatus: "B", Date: now.Add(25 * time.Minute)}, // Backflow C -> B
				{FromStatus: "B", ToStatus: "D", Date: now.Add(35 * time.Minute)},
			},
		},
	}

	order := DiscoverStatusOrder(issues)
	// Expected: A, B, C, D (based on majority/dominance)
	// A -> B (1), B -> A (0) => A before B
	// B -> C (1), C -> B (1) => Tied, alphabetic fallback
	// C -> D (1), D -> C (0) => C before D
	// Actually predecessorCount will be:
	// A: 0
	// B: 1 (A)
	// C: 1 (A)
	// D: 3 (A, B, C)

	expected := []string{"A", "B", "C", "D"}
	if len(order) != len(expected) {
		t.Fatalf("Expected %d statuses, got %d: %v", len(expected), len(order), order)
	}
	for i, s := range expected {
		if order[i] != s {
			t.Errorf("At index %d expected %s, got %s", i, s, order[i])
		}
	}
}
