package stats_test

import (
	"fmt"
	"mcs-mcp/internal/jira"
	"mcs-mcp/internal/stats"
	"mcs-mcp/internal/stats/discovery"
	"reflect"
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

	results := stats.CalculateStatusPersistence(issues)

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
	results := []stats.StatusPersistence{
		{StatusName: "Open"},
		{StatusName: "In Dev"},
	}
	mappings := map[string]stats.StatusMetadata{
		"Open": {Tier: "Demand", Role: "active"},
	}

	enriched := stats.EnrichStatusPersistence(results, mappings)

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
	duration := stats.SumRangeDuration(issue, rangeStatuses)

	if duration != 8.5 {
		t.Errorf("Expected duration 8.5, got %f", duration)
	}

	// Non-existent status should be ignored
	duration = stats.SumRangeDuration(issue, []string{"In Dev", "Blocked"})
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

	persistence := []stats.StatusPersistence{
		{StatusName: "Development", P50: 1.0, P85: 5.0},
	}

	results := stats.CalculateStatusAging(wipIssues, persistence, time.Now())

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
	mappings := map[string]stats.StatusMetadata{
		"In Dev": {Tier: "Downstream", Role: "active"},
	}

	// Test WIP Age
	results := stats.CalculateInventoryAge(wipIssues, "In Dev", statusWeights, mappings, history, "wip", time.Now())

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
	resultsTotal := stats.CalculateInventoryAge(wipIssues, "In Dev", statusWeights, mappings, history, "total", time.Now())
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
			Key:      "PROJ-1",
			Status:   "In Dev",
			StatusID: "4",
			Transitions: []jira.StatusTransition{
				{FromStatus: "Backlog", FromStatusID: "1", ToStatus: "Refinement", ToStatusID: "2", Date: now.AddDate(0, 0, -5)},
				{FromStatus: "Refinement", FromStatusID: "2", ToStatus: "Ready for Dev", ToStatusID: "3", Date: now.AddDate(0, 0, -3)},
				{FromStatus: "Ready for Dev", FromStatusID: "3", ToStatus: "In Dev", ToStatusID: "4", Date: now.AddDate(0, 0, -2)},
			},
			BirthStatus:   "Backlog",
			BirthStatusID: "1",
		},
	}

	persistence := []stats.StatusPersistence{
		{StatusID: "1", StatusName: "Backlog", P50: 10.0},
		{StatusID: "2", StatusName: "Refinement", P50: 2.0},
		{StatusID: "3", StatusName: "Ready for Dev", P50: 1.0},
		{StatusID: "4", StatusName: "In Dev", P50: 5.0},
		{StatusID: "5", StatusName: "Done", P50: 0.0},
	}
	// Add transitions to Backlog to make it the birth status
	for i := 0; i < 20; i++ {
		issues = append(issues, jira.Issue{
			Key: "B", Status: "Backlog", StatusID: "1", BirthStatus: "Backlog", BirthStatusID: "1",
			Transitions: []jira.StatusTransition{{FromStatus: "Backlog", FromStatusID: "1", ToStatus: "Refinement", ToStatusID: "2"}},
		})
	}
	// Add transitions to Done to make it a terminal sink
	for i := 0; i < 10; i++ {
		issues = append(issues, jira.Issue{
			Key: "S", Status: "Done", StatusID: "5",
			Transitions: []jira.StatusTransition{{FromStatus: "In Dev", FromStatusID: "4", ToStatus: "Done", ToStatusID: "5"}},
		})
	}
	mapping, commitmentPoint := discovery.ProposeSemantics(issues, persistence)

	// Verify Backlog (ID "1") is Demand (detected as first entry point)
	if mapping["1"].Tier != "Demand" {
		t.Errorf("Expected Backlog (1) to be 'Demand', got %s", mapping["1"].Tier)
	}

	// Verify Ready for Dev (ID "3") is a queue (detected by pattern)
	if mapping["3"].Role != "queue" {
		t.Errorf("Expected 'Ready for Dev' (3) to have role 'queue', got %s", mapping["3"].Role)
	}

	// Verify In Dev (ID "4") is Downstream
	if mapping["4"].Tier != "Downstream" {
		t.Errorf("Expected 'In Dev' (4) to be 'Downstream', got %s", mapping["4"].Tier)
	}

	// Verify User-specified role constraints: Demand must be 'queue'
	if mapping["1"].Role != "queue" {
		t.Errorf("Expected 'Backlog' (1) (Demand) to have role 'queue', got %s", mapping["1"].Role)
	}

	if commitmentPoint != "4" {
		t.Errorf("Expected commitment point '4' (In Dev), got %s", commitmentPoint)
	}

	// Verify Done (ID "5") has heuristic outcome "delivered"
	if mapping["5"].Outcome != "delivered" {
		t.Errorf("Expected Done (5) to have outcome 'delivered', got %s", mapping["5"].Outcome)
	}
}

func TestDiscoverStatusOrder_PathBased(t *testing.T) {
	issues := []jira.Issue{
		{
			Key:           "I-1",
			BirthStatus:   "To Do",
			BirthStatusID: "1",
			Status:        "Done",
			StatusID:      "3",
			Transitions: []jira.StatusTransition{
				{FromStatus: "To Do", FromStatusID: "1", ToStatus: "In Progress", ToStatusID: "2"},
				{FromStatus: "In Progress", FromStatusID: "2", ToStatus: "Done", ToStatusID: "3"},
			},
		},
		{
			Key:           "I-2",
			BirthStatus:   "To Do",
			BirthStatusID: "1",
			Status:        "In Progress",
			StatusID:      "2",
			Transitions: []jira.StatusTransition{
				{FromStatus: "To Do", FromStatusID: "1", ToStatus: "In Progress", ToStatusID: "2"},
			},
		},
	}

	order := discovery.DiscoverStatusOrder(issues)
	expected := []string{"1", "2", "3"}

	if !reflect.DeepEqual(order, expected) {
		t.Errorf("Expected %v, got %v", expected, order)
	}
}

func TestDiscoverStatusOrder_Shortcuts(t *testing.T) {
	// Scenario: Many items skip "Analysis" and go straight from "To Do" to "Development".
	// But "Analysis" still happens before "Development" for some, and never after.
	issues := []jira.Issue{
		{
			Key:           "I-1",
			BirthStatus:   "To Do",
			BirthStatusID: "1",
			Transitions: []jira.StatusTransition{
				{FromStatus: "To Do", FromStatusID: "1", ToStatus: "Development", ToStatusID: "3"},
			},
		},
		{
			Key:           "I-2",
			BirthStatus:   "To Do",
			BirthStatusID: "1",
			Transitions: []jira.StatusTransition{
				{FromStatus: "To Do", FromStatusID: "1", ToStatus: "Development", ToStatusID: "3"},
			},
		},
		{
			Key:           "I-3",
			BirthStatus:   "To Do",
			BirthStatusID: "1",
			Transitions: []jira.StatusTransition{
				{FromStatus: "To Do", FromStatusID: "1", ToStatus: "Analysis", ToStatusID: "2"},
				{FromStatus: "Analysis", FromStatusID: "2", ToStatus: "Development", ToStatusID: "3"},
			},
		},
	}

	order := discovery.DiscoverStatusOrder(issues)
	// Even though To Do -> Development is more frequent than To Do -> Analysis,
	// Analysis globally precedes Development (it never happens after it).
	// So Analysis should be between To Do and Development.
	expected := []string{"1", "2", "3"}

	if !reflect.DeepEqual(order, expected) {
		t.Errorf("Expected %v, got %v", expected, order)
	}
}

func TestDiscoverStatusOrder_ComplexPath_Scenario(t *testing.T) {
	// Mimics the user's reported "hijacked" path
	// Expected IDs: 1(To Do), 2(Analysis), 3(Arch Design), 4(Refinement), 5(Ready for Dev), 6(In Dev), 7(Validation), 8(Ready to Deploy), 9(In Releasing), 10(Done)
	issues := []jira.Issue{
		// Item 1: Perfect happy path
		{
			BirthStatus: "To Do", BirthStatusID: "1",
			Transitions: []jira.StatusTransition{
				{FromStatus: "To Do", FromStatusID: "1", ToStatus: "Analysis", ToStatusID: "2"},
				{FromStatus: "Analysis", FromStatusID: "2", ToStatus: "Architecture Design", ToStatusID: "3"},
				{FromStatus: "Architecture Design", FromStatusID: "3", ToStatus: "Refinement", ToStatusID: "4"},
				{FromStatus: "Refinement", FromStatusID: "4", ToStatus: "Ready for Development", ToStatusID: "5"},
				{FromStatus: "Ready for Development", FromStatusID: "5", ToStatus: "In Development", ToStatusID: "6"},
				{FromStatus: "In Development", FromStatusID: "6", ToStatus: "Validation", ToStatusID: "7"},
				{FromStatus: "Validation", FromStatusID: "7", ToStatus: "Ready to Deploy", ToStatusID: "8"},
				{FromStatus: "Ready to Deploy", FromStatusID: "8", ToStatus: "In Releasing", ToStatusID: "9"},
				{FromStatus: "In Releasing", FromStatusID: "9", ToStatus: "Done", ToStatusID: "10"},
			},
		},
		// Item 2: Shortcut "To Do -> Validation" (The Hijacker)
		{
			BirthStatus: "To Do", BirthStatusID: "1",
			Transitions: []jira.StatusTransition{
				{FromStatus: "To Do", FromStatusID: "1", ToStatus: "Validation", ToStatusID: "7"},
				{FromStatus: "Validation", FromStatusID: "7", ToStatus: "Done", ToStatusID: "10"},
			},
		},
		// Item 3: Another shortcut "To Do -> Ready for Development"
		{
			BirthStatus: "To Do", BirthStatusID: "1",
			Transitions: []jira.StatusTransition{
				{FromStatus: "To Do", FromStatusID: "1", ToStatus: "Ready for Development", ToStatusID: "5"},
				{FromStatus: "Ready for Development", FromStatusID: "5", ToStatus: "Done", ToStatusID: "10"},
			},
		},
	}

	order := discovery.DiscoverStatusOrder(issues)

	// Check specific critical orderings (by ID)
	indexOf := func(s string) int {
		for i, v := range order {
			if v == s {
				return i
			}
		}
		return -1
	}

	checks := [][]string{
		{"1", "2"},  // To Do before Analysis
		{"2", "3"},  // Analysis before Architecture Design
		{"3", "4"},  // Architecture Design before Refinement
		{"4", "5"},  // Refinement before Ready for Development
		{"5", "7"},  // Ready for Development before Validation
		{"7", "10"}, // Validation before Done
	}

	for _, pair := range checks {
		i1 := indexOf(pair[0])
		i2 := indexOf(pair[1])
		if i1 == -1 || i2 == -1 || i1 >= i2 {
			t.Errorf("Ordering failure: Expected ID %s before ID %s. Full order: %v", pair[0], pair[1], order)
		}
	}
}

func TestTierDiscovery_RefiningScenario(t *testing.T) {
	now := time.Now()
	// User Scenario: Open (Birth) -> refining -> awaiting development -> developing -> Done
	issues := []jira.Issue{
		{
			Key: "I-1", Status: "Done", StatusID: "5", StatusCategory: "done",
			Transitions: []jira.StatusTransition{
				{FromStatus: "Open", FromStatusID: "1", ToStatus: "refining", ToStatusID: "2", Date: now.Add(-4 * time.Hour)},
				{FromStatus: "refining", FromStatusID: "2", ToStatus: "awaiting development", ToStatusID: "3", Date: now.Add(-3 * time.Hour)},
				{FromStatus: "awaiting development", FromStatusID: "3", ToStatus: "developing", ToStatusID: "4", Date: now.Add(-2 * time.Hour)},
				{FromStatus: "developing", FromStatusID: "4", ToStatus: "Done", ToStatusID: "5", Date: now.Add(-1 * time.Hour)},
			},
			BirthStatus: "Open", BirthStatusID: "1",
		},
		{Key: "M-1", Status: "Open", StatusID: "1", StatusCategory: "to-do", BirthStatus: "Open", BirthStatusID: "1"},
		{Key: "M-2", Status: "refining", StatusID: "2", StatusCategory: "to-do", BirthStatus: "refining", BirthStatusID: "2"},
		{Key: "M-3", Status: "awaiting development", StatusID: "3", StatusCategory: "indeterminate", BirthStatus: "awaiting development", BirthStatusID: "3"},
		{Key: "M-4", Status: "developing", StatusID: "4", StatusCategory: "indeterminate", BirthStatus: "developing", BirthStatusID: "4"},
	}

	persistence := []stats.StatusPersistence{
		{StatusID: "1", StatusName: "Open"},
		{StatusID: "2", StatusName: "refining"},
		{StatusID: "3", StatusName: "awaiting development"},
		{StatusID: "4", StatusName: "developing"},
		{StatusID: "5", StatusName: "Done"},
	}

	proposal, _ := discovery.ProposeSemantics(issues, persistence)
	order := discovery.DiscoverStatusOrder(issues)

	// Verify Tiers (keyed by ID)
	if proposal["1"].Tier != "Demand" {
		t.Errorf("Open (1) should be Demand, got %s", proposal["1"].Tier)
	}
	if proposal["2"].Tier != "Upstream" {
		t.Errorf("refining (2) should be Upstream, got %s", proposal["2"].Tier)
	}
	if proposal["4"].Tier != "Downstream" {
		t.Errorf("developing (4) should be Downstream, got %s", proposal["4"].Tier)
	}

	// Verify Roles (keyed by ID)
	if proposal["3"].Role != "queue" {
		t.Errorf("awaiting development (3) should be queue, got %s", proposal["3"].Role)
	}

	// Verify Order (Backbone path by ID)
	expectedOrder := []string{"1", "2", "3", "4", "5"}
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
			Key: "S", Status: "Done", StatusID: "3",
			ResolutionDate: &now,
			Transitions: []jira.StatusTransition{
				{FromStatus: "Refining", FromStatusID: "1", ToStatus: "Done", ToStatusID: "3", Date: now},
			},
			BirthStatus: "Refining", BirthStatusID: "1",
		})
	}
	// 8 Active pathers
	for i := 0; i < 8; i++ {
		issues = append(issues, jira.Issue{
			Key: "A", Status: "Developing", StatusID: "2",
			Transitions: []jira.StatusTransition{
				{FromStatus: "Refining", FromStatusID: "1", ToStatus: "Developing", ToStatusID: "2", Date: now},
			},
			BirthStatus: "Refining", BirthStatusID: "1",
		})
	}

	order := discovery.DiscoverStatusOrder(issues)

	// We expect 1(Refining) -> 2(Developing) -> 3(Done)
	if len(order) < 2 {
		t.Fatalf("Expected at least 2 statuses, got %d", len(order))
	}
	if order[0] != "1" {
		t.Errorf("Expected 1 (Refining) first, got %s", order[0])
	}
	if order[1] != "2" {
		t.Errorf("Expected 2 (Developing) second (avoided Done shortcut), got %s", order[1])
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
			Key: "P", Status: "Prod", StatusID: "2",
			ResolutionDate: &now,
			Transitions: []jira.StatusTransition{
				{FromStatus: "UAT", FromStatusID: "1", ToStatus: "Prod", ToStatusID: "2", Date: now},
			},
		})
	}
	// 2 items stop in UAT
	for i := 0; i < 2; i++ {
		issues = append(issues, jira.Issue{
			Key: "C", Status: "UAT", StatusID: "1",
			ResolutionDate: &now,
		})
	}

	persistence := []stats.StatusPersistence{
		{StatusID: "1", StatusName: "UAT"},
		{StatusID: "2", StatusName: "Prod"},
	}

	proposal, _ := discovery.ProposeSemantics(issues, persistence)

	if proposal["1"].Tier == "Finished" {
		t.Errorf("UAT (1) should be Downstream (10%% density), not Finished")
	}
	if proposal["2"].Tier != "Finished" {
		t.Errorf("Prod (2) should be Finished, got %s", proposal["2"].Tier)
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
	sample := discovery.SelectDiscoverySample(issues, 200)

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

	summary := discovery.AnalyzeProbe(issues, 10)

	if summary.Sample.ResolutionDensity != 0.67 {
		t.Errorf("Expected resolution density 0.67, got %f", summary.Sample.ResolutionDensity)
	}
}

func TestProposeSemantics_OutcomeHeuristics(t *testing.T) {
	now := time.Now()
	// Provide issues where these statuses are terminal sinks
	issues := []jira.Issue{
		{Key: "I-1", Status: "Done", StatusID: "1", ResolutionDate: &now},
		{Key: "I-2", Status: "Cancelled", StatusID: "2", ResolutionDate: &now},
		{Key: "I-3", Status: "Rejected", StatusID: "3", ResolutionDate: &now},
	}
	// Add more transitions to satisfy "isTerminalSink" (into > 5 and into > out*4)
	for i := 0; i < 10; i++ {
		issues = append(issues, jira.Issue{
			Key: "S", Status: "Done", StatusID: "1",
			Transitions: []jira.StatusTransition{{FromStatus: "In Dev", FromStatusID: "4", ToStatus: "Done", ToStatusID: "1"}},
		})
	}

	persistence := []stats.StatusPersistence{
		{StatusID: "1", StatusName: "Done"},
		{StatusID: "2", StatusName: "Cancelled"},
		{StatusID: "3", StatusName: "Rejected"},
	}
	mapping, _ := discovery.ProposeSemantics(issues, persistence)

	// Done (1) -> Delivered (Default or Keyword)
	if mapping["1"].Outcome != "delivered" {
		t.Errorf("Expected Done (1) -> delivered, got %s", mapping["1"].Outcome)
	}
	// Cancelled (2) -> Abandoned (Keyword)
	if mapping["2"].Outcome != "abandoned" {
		t.Errorf("Expected Cancelled (2) -> abandoned, got %s", mapping["2"].Outcome)
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
	cutoff := discovery.CalculateDiscoveryCutoff(issues, isFinished)

	if cutoff == nil {
		t.Fatal("Expected non-nil cutoff")
	}

	expected := earliest.AddDate(0, 0, 50)
	if !cutoff.Equal(expected) {
		t.Errorf("Expected cutoff %v, got %v", expected, *cutoff)
	}

	// Test with fewer than 5 deliveries
	smallIssues := issues[:3]
	if nilCutoff := discovery.CalculateDiscoveryCutoff(smallIssues, isFinished); nilCutoff != nil {
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

	filtered := stats.FilterIssuesByResolutionWindow(issues, windowDays, cutoff)

	if len(filtered) != 1 {
		t.Errorf("Expected 1 filtered issue, got %d", len(filtered))
	}
	if filtered[0].Key != "I-1" {
		t.Errorf("Expected I-1, got %s", filtered[0].Key)
	}
}
