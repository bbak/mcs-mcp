package stats

import (
	"mcs-mcp/internal/jira"
	"testing"
	"time"
)

func TestCalculateBlockedResidency(t *testing.T) {
	now := time.Now()

	// Status Segments:
	// S1: [0, 10h]
	// S2: [10h, 20h]
	segments := []StatusSegment{
		{Status: "S1", Start: now, End: now.Add(10 * time.Hour)},
		{Status: "S2", Start: now.Add(10 * time.Hour), End: now.Add(20 * time.Hour)},
	}

	// Blocked Intervals:
	// B1: [5h, 15h] - Spans both statuses
	// B2: [18h, 19h] - Within S2
	intervals := []Interval{
		{Start: now.Add(5 * time.Hour), End: now.Add(15 * time.Hour)},
		{Start: now.Add(18 * time.Hour), End: now.Add(19 * time.Hour)},
	}

	res := CalculateBlockedResidency(segments, intervals)

	// S1 should have 5h blocked (from 5h to 10h)
	if res["S1"] != 5*3600 {
		t.Errorf("Expected S1 blocked residency to be 18000s (5h), got %d", res["S1"])
	}

	// S2 should have 5h (from 10h to 15h) + 1h (from 18h to 19h) = 6h
	if res["S2"] != 6*3600 {
		t.Errorf("Expected S2 blocked residency to be 21600s (6h), got %d", res["S2"])
	}
}

func TestCalculateBlockedResidency_EdgeCases(t *testing.T) {
	now := time.Now()

	segments := []StatusSegment{
		{Status: "S1", Start: now, End: now.Add(10 * time.Hour)},
	}

	s := func(h int) time.Time { return now.Add(time.Duration(h) * time.Hour) }

	tests := []struct {
		name     string
		blocked  []Interval
		expected int64
	}{
		{
			name:     "starts_before_status",
			blocked:  []Interval{{Start: s(-5), End: s(5)}},
			expected: 5 * 3600,
		},
		{
			name:     "ends_after_status",
			blocked:  []Interval{{Start: s(5), End: s(15)}},
			expected: 5 * 3600,
		},
		{
			name:     "encompasses_status",
			blocked:  []Interval{{Start: s(-5), End: s(15)}},
			expected: 10 * 3600,
		},
		{
			name:     "multiple_intervals_in_status",
			blocked:  []Interval{{Start: s(1), End: s(2)}, {Start: s(4), End: s(6)}},
			expected: 3 * 3600,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := CalculateBlockedResidency(segments, tt.blocked)
			if res["S1"] != tt.expected {
				t.Errorf("Expected %d, got %d", tt.expected, res["S1"])
			}
		})
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
