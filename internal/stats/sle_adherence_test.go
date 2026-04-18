package stats

import (
	"mcs-mcp/internal/jira"
	"testing"
	"time"
)

func mkIssue(key string, finished time.Time) jira.Issue {
	t := finished
	return jira.Issue{Key: key, OutcomeDate: &t, ResolutionDate: &t}
}

func mkWeeklyWindow(start time.Time, weeks int) AnalysisWindow {
	// End must land inside the last desired week (weeks*7 - 1 days from start),
	// because SnapToEnd extends to the Sunday following the supplied end timestamp.
	end := start.AddDate(0, 0, 7*weeks-1)
	return NewAnalysisWindow(start, end, "week", time.Time{})
}

func TestComputeSLEAdherence_Empty(t *testing.T) {
	w := mkWeeklyWindow(time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC), 4)
	res := ComputeSLEAdherence(nil, nil, 10, 85, "derived_p85", w)
	if res.OverallRate != 0 {
		t.Errorf("expected 0 overall rate for empty input, got %v", res.OverallRate)
	}
	if len(res.Buckets) != 0 {
		t.Errorf("expected no buckets, got %d", len(res.Buckets))
	}
	if res.SLESource != "derived_p85" {
		t.Errorf("expected sle_source pass-through, got %q", res.SLESource)
	}
}

func TestComputeSLEAdherence_AllInside(t *testing.T) {
	start := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC) // Monday
	w := mkWeeklyWindow(start, 4)

	issues := []jira.Issue{
		mkIssue("A-1", start.AddDate(0, 0, 1)),
		mkIssue("A-2", start.AddDate(0, 0, 8)),
		mkIssue("A-3", start.AddDate(0, 0, 15)),
		mkIssue("A-4", start.AddDate(0, 0, 22)),
	}
	cts := []float64{3, 5, 7, 4}

	res := ComputeSLEAdherence(issues, cts, 10.0, 85, "derived_p85", w)

	if res.OverallRate != 1.0 {
		t.Errorf("expected overall rate 1.0, got %v", res.OverallRate)
	}
	for _, b := range res.Buckets {
		if b.DeliveredCount > 0 && b.AttainmentRate != 1.0 {
			t.Errorf("bucket %s: expected 1.0 attainment, got %v", b.BucketLabel, b.AttainmentRate)
		}
		if b.BreachCount != 0 {
			t.Errorf("bucket %s: expected zero breaches, got %d", b.BucketLabel, b.BreachCount)
		}
	}
}

func TestComputeSLEAdherence_MixedBreaches(t *testing.T) {
	start := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	w := mkWeeklyWindow(start, 1)

	// 10 items in week 1 — 3 breach the 10d SLE.
	issues := make([]jira.Issue, 10)
	cts := []float64{2, 4, 6, 8, 10, 12, 14, 18, 9, 7}
	for i := range issues {
		issues[i] = mkIssue("M-"+time.Duration(i).String(), start.AddDate(0, 0, 1))
	}

	res := ComputeSLEAdherence(issues, cts, 10.0, 85, "derived_p85", w)

	if res.OverallRate != 0.7 {
		t.Errorf("expected overall rate 0.70, got %v", res.OverallRate)
	}
	if len(res.Buckets) != 1 {
		t.Fatalf("expected 1 bucket, got %d", len(res.Buckets))
	}
	b := res.Buckets[0]
	if b.DeliveredCount != 10 || b.BreachCount != 3 {
		t.Errorf("bucket counts: delivered=%d breaches=%d (want 10/3)", b.DeliveredCount, b.BreachCount)
	}
	if b.MaxCycleTimeDays != 18 {
		t.Errorf("expected max CT 18, got %v", b.MaxCycleTimeDays)
	}
	if b.P95BreachMagDays <= 0 {
		t.Errorf("expected positive P95 breach magnitude, got %v", b.P95BreachMagDays)
	}
}

func TestComputeSLEAdherence_UserSourceNoExpectedRate(t *testing.T) {
	start := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	w := mkWeeklyWindow(start, 1)
	issues := []jira.Issue{mkIssue("U-1", start.AddDate(0, 0, 1))}
	cts := []float64{5}

	// User passes a fixed duration without a percentile → ExpectedRate stays 0.
	res := ComputeSLEAdherence(issues, cts, 14.0, 0, "user", w)
	if res.ExpectedRate != 0 {
		t.Errorf("expected 0 ExpectedRate when slePercentile=0, got %v", res.ExpectedRate)
	}
	if res.SLEDurationDays != 14 {
		t.Errorf("expected 14d SLE, got %v", res.SLEDurationDays)
	}
}

func TestComputeSLEAdherence_BucketBoundary(t *testing.T) {
	start := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC) // Monday
	w := mkWeeklyWindow(start, 2)

	// One item finishes on Sunday 23:59 (last second of week 1), one on Monday 00:00 (first of week 2).
	endOfWeek1 := time.Date(2026, 1, 11, 23, 59, 59, 0, time.UTC)
	startOfWeek2 := time.Date(2026, 1, 12, 0, 0, 0, 0, time.UTC)
	issues := []jira.Issue{
		mkIssue("E-1", endOfWeek1),
		mkIssue("E-2", startOfWeek2),
	}
	cts := []float64{3, 4}

	res := ComputeSLEAdherence(issues, cts, 10.0, 85, "derived_p85", w)
	if len(res.Buckets) != 2 {
		t.Fatalf("expected 2 buckets, got %d", len(res.Buckets))
	}
	if res.Buckets[0].DeliveredCount != 1 {
		t.Errorf("week 1 should have 1 item, got %d", res.Buckets[0].DeliveredCount)
	}
	if res.Buckets[1].DeliveredCount != 1 {
		t.Errorf("week 2 should have 1 item, got %d", res.Buckets[1].DeliveredCount)
	}
}
