package stats

import (
	"fmt"
	"math"
	"time"
)

// AnalysisWindow defines the temporal context for analytical projections and diagnostics.
type AnalysisWindow struct {
	Start  time.Time `json:"start"`
	End    time.Time `json:"end"`
	Bucket string    `json:"bucket"` // "day", "week", "month"
	Cutoff time.Time `json:"cutoff"` // Steady-State floor
}

// NewAnalysisWindow creates a new window with normalized boundaries and cutoff clamping.
func NewAnalysisWindow(start, end time.Time, bucket string, cutoff time.Time) AnalysisWindow {
	// 1. Sanitize Bucket
	if bucket == "" {
		bucket = "day"
	}

	// 2. Snap to Boundaries (normalize to start/end of period)
	normStart := SnapToStart(start, bucket)
	normEnd := SnapToEnd(end, bucket)

	// 3. Apply Steady-State Cutoff (Clamping)
	if !cutoff.IsZero() && cutoff.After(normStart) {
		normStart = SnapToStart(cutoff, bucket)
	}

	return AnalysisWindow{
		Start:  normStart,
		End:    normEnd,
		Bucket: bucket,
		Cutoff: cutoff,
	}
}

// SnapToStart normalizes a timestamp to the beginning of its bucket (0:00:00).
func SnapToStart(t time.Time, bucket string) time.Time {
	if t.IsZero() {
		return t
	}
	switch bucket {
	case "month":
		return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
	case "week":
		// Snap to Monday
		weekday := int(t.Weekday())
		if weekday == 0 {
			weekday = 7 // Sunday -> 7
		}
		daysToSubtract := weekday - 1
		return time.Date(t.Year(), t.Month(), t.Day()-daysToSubtract, 0, 0, 0, 0, t.Location())
	default: // day
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	}
}

// SnapToEnd normalizes a timestamp to the very end of its bucket (23:59:59.999...).
func SnapToEnd(t time.Time, bucket string) time.Time {
	if t.IsZero() {
		return t
	}
	switch bucket {
	case "month":
		// Last nanosecond of the month
		nextMonth := time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, t.Location())
		return nextMonth.Add(-time.Nanosecond)
	case "week":
		// Last nanosecond of Sunday
		weekday := int(t.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		daysToAdd := 7 - weekday
		return time.Date(t.Year(), t.Month(), t.Day()+daysToAdd, 23, 59, 59, 999999999, t.Location())
	default: // day
		return time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 999999999, t.Location())
	}
}

// IsPartial returns true if the current bucket includes "Today", indicating incomplete data.
func (w AnalysisWindow) IsPartial(bucketStart time.Time) bool {
	now := time.Now()
	bucketEnd := SnapToEnd(bucketStart, w.Bucket)
	return (now.After(bucketStart) || now.Equal(bucketStart)) && (now.Before(bucketEnd) || now.Equal(bucketEnd))
}

// Subdivide returns a list of bucket start times within the window.
func (w AnalysisWindow) Subdivide() []time.Time {
	var buckets []time.Time
	current := w.Start

	for current.Before(w.End) {
		buckets = append(buckets, current)
		switch w.Bucket {
		case "month":
			current = current.AddDate(0, 1, 0)
		case "week":
			current = current.AddDate(0, 0, 7)
		default: // day
			current = current.AddDate(0, 0, 1)
		}
	}
	return buckets
}

// FindBucketIndex returns the index of the bucket containing t. Returns -1 if out of bounds.
func (w AnalysisWindow) FindBucketIndex(t time.Time) int {
	tNorm := SnapToStart(t, w.Bucket)
	if tNorm.Before(w.Start) || tNorm.After(w.End) {
		return -1
	}

	switch w.Bucket {
	case "month":
		return (tNorm.Year()-w.Start.Year())*12 + int(tNorm.Month()-w.Start.Month())
	case "week":
		// Use integer division on hours to avoid floating point issues
		return int(tNorm.Sub(w.Start).Hours() / (24 * 7))
	default: // day
		return int(tNorm.Sub(w.Start).Hours() / 24)
	}
}

// DayCount returns the number of calendar days in the window.
func (w AnalysisWindow) DayCount() int {
	return int(math.Ceil(w.End.Sub(w.Start).Hours() / 24.0))
}

// ActiveDayCount returns the number of days excluding partial buckets at the end.
// This prevents "Throughput Dilution" where an incomplete week/month masks real performance.
func (w AnalysisWindow) ActiveDayCount() int {
	buckets := w.Subdivide()
	activeCount := 0
	for _, b := range buckets {
		if !w.IsPartial(b) {
			switch w.Bucket {
			case "month":
				activeCount += int(math.Round(SnapToEnd(b, "month").Sub(b).Hours() / 24.0))
			case "week":
				activeCount += 7
			default:
				activeCount++
			}
		}
	}
	if activeCount == 0 {
		return w.DayCount() // Fallback
	}
	return activeCount
}

// GenerateLabel returns a human-readable label for a bucket (e.g., "Jan 2024" or "2024-W01").
func (w AnalysisWindow) GenerateLabel(t time.Time) string {
	switch w.Bucket {
	case "month":
		return t.Format("Jan 2006")
	case "week":
		year, week := t.ISOWeek()
		return fmt.Sprintf("%d-W%02d", year, week)
	default: // day
		return t.Format("2006-01-02")
	}
}
