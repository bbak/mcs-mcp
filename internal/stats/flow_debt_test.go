package stats

import (
	"mcs-mcp/internal/jira"
	"testing"
	"time"
)

func TestCalculateFlowDebt(t *testing.T) {
	now := time.Now()
	// Monday of current week
	monday := SnapToStart(now, "week")

	weights := map[string]int{
		"1": 1,
		"2": 2, // Commitment Point
		"3": 3,
	}

	issues := []jira.Issue{
		{
			Key:           "I1",
			BirthStatus:   "To Do",
			BirthStatusID: "1",
			Created:       monday.AddDate(0, 0, -2), // Saturday last week
			Transitions: []jira.StatusTransition{
				{ToStatus: "In Progress", ToStatusID: "2", Date: monday.AddDate(0, 0, 1)}, // Tuesday this week (Arrival)
				{ToStatus: "Done", ToStatusID: "3", Date: monday.AddDate(0, 0, 2)},        // Wednesday this week (Departure)
			},
			ResolutionDate: func() *time.Time { tt := monday.AddDate(0, 0, 2); return &tt }(),
			Resolution:     "Fixed",
			Status:         "Done",
			StatusID:       "3",
		},
		{
			Key:           "I2",
			BirthStatus:   "In Progress", // Arrives at birth
			BirthStatusID: "2",
			Created:       monday.AddDate(0, 0, -1), // Sunday last week (Arrival)
			Resolution:    "",                       // Not resolved yet
			Status:        "In Progress",
			StatusID:      "2",
		},
	}

	mappings := map[string]StatusMetadata{
		"3": {Tier: "Finished", Outcome: "delivered", Name: "Done"},
	}
	resolutions := map[string]string{
		"Fixed": "delivered",
	}

	// 2-week window ending now
	window := NewAnalysisWindow(monday.AddDate(0, 0, -7), now, "week", time.Time{})

	res := CalculateFlowDebt(issues, window, "2", weights, resolutions, mappings)

	// Buckets:
	// Index 0: Last Week (Monday to Sunday)
	// Index 1: This Week (Monday to now)

	if len(res.Buckets) != 2 {
		t.Fatalf("Expected 2 buckets, got %d", len(res.Buckets))
	}

	// I2 arrived last week (Sunday)
	if res.Buckets[0].Arrivals != 1 {
		t.Errorf("Expected 1 arrival in last week, got %d", res.Buckets[0].Arrivals)
	}
	if res.Buckets[0].Departures != 0 {
		t.Errorf("Expected 0 departures in last week, got %d", res.Buckets[0].Departures)
	}

	// I1 arrived this week (Tuesday) and departed this week (Wednesday)
	if res.Buckets[1].Arrivals != 1 {
		t.Errorf("Expected 1 arrival in this week, got %d", res.Buckets[1].Arrivals)
	}
	if res.Buckets[1].Departures != 1 {
		t.Errorf("Expected 1 departure in this week, got %d", res.Buckets[1].Departures)
	}

	if res.TotalDebt != 1 {
		t.Errorf("Expected total debt 1 (2 arrivals - 1 departure), got %d", res.TotalDebt)
	}
}
