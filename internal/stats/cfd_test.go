package stats

import (
	"mcs-mcp/internal/jira"
	"testing"
	"time"
)

func TestCalculateCFDData(t *testing.T) {
	now := time.Now()
	monday := SnapToStart(now, "week")
	t1 := monday.AddDate(0, 0, 1) // Tuesday
	t2 := monday.AddDate(0, 0, 2) // Wednesday

	issues := []jira.Issue{
		{
			Key:         "I1",
			IssueType:   "Story",
			BirthStatus: "To Do",
			Created:     monday,
			Transitions: []jira.StatusTransition{
				{ToStatus: "In Progress", Date: t1},
				{ToStatus: "Done", Date: t2},
			},
		},
		{
			Key:         "I2",
			IssueType:   "Bug",
			BirthStatus: "To Do",
			Created:     t1,
			Transitions: []jira.StatusTransition{
				{ToStatus: "In Progress", Date: t2},
			},
		},
	}

	// 1 week window
	window := NewAnalysisWindow(monday, monday.AddDate(0, 0, 6), "day", time.Time{})

	res := CalculateCFDData(issues, window, nil)

	if len(res.Buckets) != 7 {
		t.Fatalf("Expected 7 buckets, got %d", len(res.Buckets))
	}

	// Monday:
	// I1: To Do
	// I2: Not born
	if res.Buckets[0].ByIssueType["Story"]["To Do"] != 1 {
		t.Errorf("Monday Story/To Do mismatch: got %d", res.Buckets[0].ByIssueType["Story"]["To Do"])
	}

	// Tuesday:
	// I1: In Progress
	// I2: To Do
	if res.Buckets[1].ByIssueType["Story"]["In Progress"] != 1 {
		t.Errorf("Tuesday Story/In Progress mismatch: got %d", res.Buckets[1].ByIssueType["Story"]["In Progress"])
	}
	if res.Buckets[1].ByIssueType["Bug"]["To Do"] != 1 {
		t.Errorf("Tuesday Bug/To Do mismatch: got %d", res.Buckets[1].ByIssueType["Bug"]["To Do"])
	}

	// Wednesday:
	// I1: Done
	// I2: In Progress
	if res.Buckets[2].ByIssueType["Story"]["Done"] != 1 {
		t.Errorf("Wednesday Story/Done mismatch: got %d", res.Buckets[2].ByIssueType["Story"]["Done"])
	}
	if res.Buckets[2].ByIssueType["Bug"]["In Progress"] != 1 {
		t.Errorf("Wednesday Bug/In Progress mismatch: got %d", res.Buckets[2].ByIssueType["Bug"]["In Progress"])
	}
}
