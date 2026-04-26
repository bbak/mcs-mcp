package stats

import (
	"testing"
	"time"
)

func TestLastCompleteBucketEnd(t *testing.T) {
	loc := time.UTC
	cases := []struct {
		name   string
		now    time.Time
		bucket string
		want   time.Time
	}{
		{
			name:   "mid-month → previous month end",
			now:    time.Date(2024, 4, 26, 12, 0, 0, 0, loc),
			bucket: "month",
			want:   time.Date(2024, 4, 1, 0, 0, 0, 0, loc).Add(-time.Nanosecond),
		},
		{
			name:   "last day of month at noon → that month's end",
			now:    time.Date(2024, 3, 31, 23, 59, 59, 999999999, loc),
			bucket: "month",
			want:   time.Date(2024, 4, 1, 0, 0, 0, 0, loc).Add(-time.Nanosecond),
		},
		{
			name:   "wed → preceding Sunday end",
			now:    time.Date(2024, 4, 24, 12, 0, 0, 0, loc), // Wednesday
			bucket: "week",
			want:   time.Date(2024, 4, 22, 0, 0, 0, 0, loc).Add(-time.Nanosecond), // Sunday end
		},
		{
			name:   "sunday end exactly → that same Sunday",
			now:    time.Date(2024, 4, 21, 23, 59, 59, 999999999, loc),
			bucket: "week",
			want:   time.Date(2024, 4, 22, 0, 0, 0, 0, loc).Add(-time.Nanosecond),
		},
		{
			name:   "day bucket mid-day → previous day end",
			now:    time.Date(2024, 4, 26, 12, 0, 0, 0, loc),
			bucket: "day",
			want:   time.Date(2024, 4, 26, 0, 0, 0, 0, loc).Add(-time.Nanosecond),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := LastCompleteBucketEnd(tc.now, tc.bucket)
			if !got.Equal(tc.want) {
				t.Fatalf("LastCompleteBucketEnd(%v, %q) = %v, want %v", tc.now, tc.bucket, got, tc.want)
			}
		})
	}
}
