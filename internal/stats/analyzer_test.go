package stats

import (
	"fmt"
	"mcs-mcp/internal/jira"
	"strings"
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
	mappings := map[string]StatusMetadata{
		"Open": {Tier: "Demand", Role: "active"},
	}

	enriched := EnrichStatusPersistence(results, mappings)

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
			if r.Tier != "" {
				t.Errorf("Expected empty Tier for unmapped In Dev, got %s", r.Tier)
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

func TestCalculateStatusAging(t *testing.T) {
	now := time.Now()
	wipIssues := []jira.Issue{
		{
			Key:         "WIP-1",
			IssueType:   "Story",
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
			BirthStatus: "Backlog",
		},
	}

	persistence := []StatusPersistence{
		{StatusName: "Backlog", P50: 10.0},
		{StatusName: "Refinement", P50: 2.0},
		{StatusName: "Ready for Dev", P50: 1.0},
		{StatusName: "In Dev", P50: 5.0},
		{StatusName: "Done", P50: 0.0},
	}
	// Add transitions to Backlog to make it the birth status
	for i := 0; i < 20; i++ {
		issues = append(issues, jira.Issue{
			Key: "B", Status: "Backlog", BirthStatus: "Backlog",
			Transitions: []jira.StatusTransition{{FromStatus: "Backlog", ToStatus: "Refinement"}},
		})
	}
	// Add transitions to Done to make it a terminal sink
	for i := 0; i < 10; i++ {
		issues = append(issues, jira.Issue{
			Key: "S", Status: "Done",
			Transitions: []jira.StatusTransition{{FromStatus: "In Dev", ToStatus: "Done"}},
		})
	}
	mapping, commitmentPoint := ProposeSemantics(issues, persistence)

	// Verify "Backlog" is Demand (detected as first entry point)
	if mapping["Backlog"].Tier != "Demand" {
		t.Errorf("Expected Backlog to be 'Demand', got %s", mapping["Backlog"].Tier)
	}

	// Verify "Ready for Dev" is a queue (detected by pattern)
	if mapping["Ready for Dev"].Role != "queue" {
		t.Errorf("Expected 'Ready for Dev' to have role 'queue', got %s", mapping["Ready for Dev"].Role)
	}

	// Verify "In Dev" is Downstream
	if mapping["In Dev"].Tier != "Downstream" {
		t.Errorf("Expected 'In Dev' to be 'Downstream', got %s", mapping["In Dev"].Tier)
	}

	// Verify User-specified role constraints: Demand must be 'queue'
	if mapping["Backlog"].Role != "queue" {
		t.Errorf("Expected 'Backlog' (Demand) to have role 'queue', got %s", mapping["Backlog"].Role)
	}

	if commitmentPoint != "In Dev" {
		t.Errorf("Expected commitment point 'In Dev', got %s", commitmentPoint)
	}

	// Verify "Done" has heuristic outcome "delivered"
	if mapping["Done"].Outcome != "delivered" {
		t.Errorf("Expected Done to have outcome 'delivered', got %s", mapping["Done"].Outcome)
	}
}

func TestDiscoverStatusOrder_PathBased(t *testing.T) {
	now := time.Now()
	// Scenario: A (Birth) -> B -> C -> D -> E (Done)
	// Noise: A -> D (Jump), D -> B (Backflow), D -> C (Backflow)
	issues := []jira.Issue{
		{
			Key:    "I-1",
			Status: "E",
			Transitions: []jira.StatusTransition{
				{FromStatus: "A", ToStatus: "B", Date: now.Add(10 * time.Minute)},
				{FromStatus: "B", ToStatus: "C", Date: now.Add(20 * time.Minute)},
				{FromStatus: "C", ToStatus: "D", Date: now.Add(30 * time.Minute)},
				{FromStatus: "D", ToStatus: "E", Date: now.Add(40 * time.Minute)},
			},
			BirthStatus: "A",
		},
		{
			Key:    "I-2",
			Status: "E",
			Transitions: []jira.StatusTransition{
				{FromStatus: "A", ToStatus: "D", Date: now.Add(15 * time.Minute)}, // Jump to D
				{FromStatus: "D", ToStatus: "B", Date: now.Add(25 * time.Minute)}, // Backflow to B
				{FromStatus: "B", ToStatus: "C", Date: now.Add(35 * time.Minute)},
				{FromStatus: "C", ToStatus: "E", Date: now.Add(45 * time.Minute)},
			},
			BirthStatus: "A",
		},
	}

	order := DiscoverStatusOrder(issues)

	// A should be first (Birth)
	if order[0] != "A" {
		t.Errorf("Expected first status 'A', got %s", order[0])
	}

	// B should be after A (most frequent successor of A is B: 1 vs D: 1, tied but B is successor of I-1 first?)
	// Actually A->B is 1, A->D is 1. Tie-breaker alphabetical B < D.
	if order[1] != "B" {
		t.Errorf("Expected second status 'B', got %s", order[1])
	}
}

func TestTierDiscovery_RefiningScenario(t *testing.T) {
	now := time.Now()
	// User Scenario: Open (Birth) -> refining -> awaiting development -> developing -> Done
	issues := []jira.Issue{
		{
			Key:            "I-1",
			Status:         "Done",
			StatusCategory: "done",
			Transitions: []jira.StatusTransition{
				{FromStatus: "Open", ToStatus: "refining", Date: now.Add(-4 * time.Hour)},
				{FromStatus: "refining", ToStatus: "awaiting development", Date: now.Add(-3 * time.Hour)},
				{FromStatus: "awaiting development", ToStatus: "developing", Date: now.Add(-2 * time.Hour)},
				{FromStatus: "developing", ToStatus: "Done", Date: now.Add(-1 * time.Hour)},
			},
			BirthStatus: "Open",
		},
		{Key: "M-1", Status: "Open", StatusCategory: "to-do", BirthStatus: "Open"},
		{Key: "M-2", Status: "refining", StatusCategory: "to-do", BirthStatus: "refining"},
		{Key: "M-3", Status: "awaiting development", StatusCategory: "indeterminate", BirthStatus: "awaiting development"},
		{Key: "M-4", Status: "developing", StatusCategory: "indeterminate", BirthStatus: "developing"},
	}

	persistence := []StatusPersistence{
		{StatusName: "Open"},
		{StatusName: "refining"},
		{StatusName: "awaiting development"},
		{StatusName: "developing"},
		{StatusName: "Done"},
	}

	proposal, _ := ProposeSemantics(issues, persistence)
	order := DiscoverStatusOrder(issues)

	// Verify Tiers
	if proposal["Open"].Tier != "Demand" {
		t.Errorf("Open should be Demand, got %s", proposal["Open"].Tier)
	}
	if proposal["refining"].Tier != "Upstream" {
		t.Errorf("refining should be Upstream (category to-do), got %s", proposal["refining"].Tier)
	}
	if proposal["developing"].Tier != "Downstream" {
		t.Errorf("developing should be Downstream (category indeterminate), got %s", proposal["developing"].Tier)
	}

	// Verify Roles
	if proposal["awaiting development"].Role != "queue" {
		t.Errorf("awaiting development should be queue, got %s", proposal["awaiting development"].Role)
	}

	// Verify Order (Backbone path)
	expectedOrder := []string{"Open", "refining", "awaiting development", "developing", "Done"}
	for i, s := range expectedOrder {
		if order[i] != s {
			t.Errorf("At index %d expected %s, got %s", i, s, order[i])
		}
	}
}

func TestDiscoverStatusOrder_ShortcutAvoidance(t *testing.T) {
	now := time.Now()
	// Scenario: Refining has two exits:
	// 1. Refining -> Done (Shortcut/Outlier): 10 issues
	// 2. Refining -> Developing (Active path): 8 issues
	// Greedy would pick Done. We expect it to pick Developing.

	var issues []jira.Issue
	// 10 Shortcutters
	for i := 0; i < 10; i++ {
		issues = append(issues, jira.Issue{
			Key:            "S",
			Status:         "Done",
			ResolutionDate: &now,
			Transitions: []jira.StatusTransition{
				{FromStatus: "Refining", ToStatus: "Done", Date: now},
			},
			BirthStatus: "Refining",
		})
	}
	// 8 Active pathers
	for i := 0; i < 8; i++ {
		issues = append(issues, jira.Issue{
			Key:    "A",
			Status: "Developing",
			Transitions: []jira.StatusTransition{
				{FromStatus: "Refining", ToStatus: "Developing", Date: now},
			},
			BirthStatus: "Refining",
		})
	}

	order := DiscoverStatusOrder(issues)

	// We expect Refining -> Developing -> Done (orphaned appended last)
	if len(order) < 2 {
		t.Fatalf("Expected at least 2 statuses, got %d", len(order))
	}
	if order[0] != "Refining" {
		t.Errorf("Expected Refining first, got %s", order[0])
	}
	if order[1] != "Developing" {
		t.Errorf("Expected Developing second (avoided Done shortcut), got %s", order[1])
	}
}

func TestProposeSemantics_ProbabilisticFinished(t *testing.T) {
	now := time.Now()
	// Scenario: UAT has some cancelled tickets (Resolutions) but is primarily an active stage.
	// Total reachability of UAT: 20 items.
	// Resolved in UAT: 2 items (10% resolution density).
	// Threshold is 20%. UAT should NOT be Finished.

	var issues []jira.Issue
	// 18 items pass through UAT to Prod
	for i := 0; i < 18; i++ {
		issues = append(issues, jira.Issue{
			Key:            "P",
			Status:         "Prod",
			ResolutionDate: &now,
			Transitions: []jira.StatusTransition{
				{FromStatus: "UAT", ToStatus: "Prod", Date: now},
			},
		})
	}
	// 2 items stop in UAT
	for i := 0; i < 2; i++ {
		issues = append(issues, jira.Issue{
			Key:            "C",
			Status:         "UAT",
			ResolutionDate: &now,
		})
	}

	persistence := []StatusPersistence{
		{StatusName: "UAT"},
		{StatusName: "Prod"},
	}

	proposal, _ := ProposeSemantics(issues, persistence)

	if proposal["UAT"].Tier == "Finished" {
		t.Errorf("UAT should be Downstream (10%% density), not Finished")
	}
	if proposal["Prod"].Tier != "Finished" {
		t.Errorf("Prod should be Finished, got %s", proposal["Prod"].Tier)
	}
}

func TestSelectDiscoverySample_Filtering(t *testing.T) {
	now := time.Now()
	oneYearAgo := now.AddDate(-1, 0, 0)
	twoYearsOld := now.AddDate(-2, 0, 0)
	ancient := now.AddDate(-5, 0, 0)

	var issues []jira.Issue
	// 50 Recent (1y)
	for i := 0; i < 50; i++ {
		issues = append(issues, jira.Issue{
			Key:     fmt.Sprintf("R-%d", i),
			Created: oneYearAgo.Add(time.Hour * time.Duration(i)),
			Updated: now,
		})
	}
	// 100 Medium (2y)
	for i := 0; i < 100; i++ {
		issues = append(issues, jira.Issue{
			Key:     fmt.Sprintf("M-%d", i),
			Created: twoYearsOld.Add(time.Hour * time.Duration(i)),
			Updated: now.Add(-time.Hour),
		})
	}
	// 100 Ancient (5y)
	for i := 0; i < 100; i++ {
		issues = append(issues, jira.Issue{
			Key:     fmt.Sprintf("A-%d", i),
			Created: ancient.Add(time.Hour * time.Duration(i)),
			Updated: now.Add(-2 * time.Hour),
		})
	}

	// target 200. Since we have 50 in 1y (which is < 100), we should expand to 3y.
	// 2y is within 3y. Ancient (5y) should be completely filtered out.
	sample := SelectDiscoverySample(issues, 200)

	if len(sample) != 150 { // Should only have Recent (50) + Medium (100)
		t.Errorf("Expected 150 items, got %d", len(sample))
	}

	for _, iss := range sample {
		if strings.HasPrefix(iss.Key, "A-") {
			t.Errorf("Ancient issue %s should have been filtered out", iss.Key)
		}
	}
}
func TestAnalyzeProbe_ResolutionDensity(t *testing.T) {
	now := time.Now()
	issues := []jira.Issue{
		{Key: "I-1", Resolution: "Fixed", ResolutionDate: &now},
		{Key: "I-2", Resolution: "Fixed", ResolutionDate: &now},
		{Key: "I-3"}, // Not resolved
	}

	summary := AnalyzeProbe(issues, 10, nil)

	if summary.Sample.ResolutionDensity != 0.67 {
		t.Errorf("Expected resolution density 0.67, got %f", summary.Sample.ResolutionDensity)
	}
}

func TestProposeSemantics_OutcomeHeuristics(t *testing.T) {
	now := time.Now()
	// Provide issues where these statuses are terminal sinks
	issues := []jira.Issue{
		{Key: "I-1", Status: "Done", ResolutionDate: &now},
		{Key: "I-2", Status: "Cancelled", ResolutionDate: &now},
		{Key: "I-3", Status: "Rejected", ResolutionDate: &now},
	}
	// Add more transitions to satisfy "isTerminalSink" (into > 5 and into > out*4)
	for i := 0; i < 10; i++ {
		issues = append(issues, jira.Issue{
			Key: "S", Status: "Done",
			Transitions: []jira.StatusTransition{{FromStatus: "In Dev", ToStatus: "Done"}},
		})
	}

	persistence := []StatusPersistence{
		{StatusName: "Done"},
		{StatusName: "Cancelled"},
		{StatusName: "Rejected"},
	}
	mapping, _ := ProposeSemantics(issues, persistence)

	// Done -> Delivered (Default or Keyword)
	if mapping["Done"].Outcome != "delivered" {
		t.Errorf("Expected Done -> delivered, got %s", mapping["Done"].Outcome)
	}
	// Cancelled -> Abandoned (Keyword)
	if mapping["Cancelled"].Outcome != "abandoned" {
		t.Errorf("Expected Cancelled -> abandoned, got %s", mapping["Cancelled"].Outcome)
	}
}
func TestCalculateDiscoveryCutoff(t *testing.T) {
	now := time.Now()
	earliest := now.AddDate(0, 0, -100)

	// Goal: 5 deliveries. Output should be the 5th one.
	issues := []jira.Issue{
		{Key: "I-1", Status: "Done", ResolutionDate: func() *time.Time { tt := earliest.AddDate(0, 0, 10); return &tt }()},
		{Key: "I-2", Status: "Done", ResolutionDate: func() *time.Time { tt := earliest.AddDate(0, 0, 20); return &tt }()},
		{Key: "I-3", Status: "Done", ResolutionDate: func() *time.Time { tt := earliest.AddDate(0, 0, 30); return &tt }()},
		{Key: "I-4", Status: "Done", ResolutionDate: func() *time.Time { tt := earliest.AddDate(0, 0, 40); return &tt }()},
		{Key: "I-5", Status: "Done", ResolutionDate: func() *time.Time { tt := earliest.AddDate(0, 0, 50); return &tt }()},
		{Key: "I-0", Status: "Backlog"}, // No resolution
	}

	isFinished := map[string]bool{"Done": true}
	cutoff := CalculateDiscoveryCutoff(issues, isFinished)

	if cutoff == nil {
		t.Fatal("Expected non-nil cutoff")
	}

	expected := earliest.AddDate(0, 0, 50)
	if !cutoff.Equal(expected) {
		t.Errorf("Expected cutoff %v, got %v", expected, *cutoff)
	}

	// Test with fewer than 5 deliveries
	smallIssues := issues[:3]
	if nilCutoff := CalculateDiscoveryCutoff(smallIssues, isFinished); nilCutoff != nil {
		t.Errorf("Expected nil cutoff for sparse deliveries, got %v", *nilCutoff)
	}
}

func TestFilterIssuesByResolutionWindow_WithCutoff(t *testing.T) {
	now := time.Now()
	cutoff := now.AddDate(0, 0, -10)
	windowDays := 20 // Window starts at -20, but cutoff is at -10

	issues := []jira.Issue{
		{Key: "I-1", ResolutionDate: func() *time.Time { tt := now.AddDate(0, 0, -5); return &tt }()},  // Keep
		{Key: "I-2", ResolutionDate: func() *time.Time { tt := now.AddDate(0, 0, -15); return &tt }()}, // Drop (before cutoff)
		{Key: "I-3", ResolutionDate: func() *time.Time { tt := now.AddDate(0, 0, -25); return &tt }()}, // Drop (before window)
	}

	filtered := FilterIssuesByResolutionWindow(issues, windowDays, cutoff)

	if len(filtered) != 1 {
		t.Errorf("Expected 1 filtered issue, got %d", len(filtered))
	}
	if filtered[0].Key != "I-1" {
		t.Errorf("Expected I-1, got %s", filtered[0].Key)
	}
}
