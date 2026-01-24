package simulation

import (
	"mcs-mcp/internal/jira"
	"time"
)

// Histogram tracks daily throughput counts.
type Histogram struct {
	Counts []int
}

// NewHistogram creates a histogram from a list of resolved issues.
// resolutions: if provided, only items with these resolutions are counted.
func NewHistogram(issues []jira.Issue, startTime, endTime time.Time, resolutions []string) *Histogram {
	// Create map for fast resolution lookup
	resMap := make(map[string]bool)
	for _, r := range resolutions {
		resMap[r] = true
	}

	// Calculate number of days in range
	days := int(endTime.Sub(startTime).Hours()/24) + 1
	if days <= 0 {
		return &Histogram{Counts: []int{0}}
	}

	buckets := make([]int, days)

	for _, issue := range issues {
		if issue.ResolutionDate == nil {
			continue
		}

		// Filter by resolution if requested
		if len(resolutions) > 0 && !resMap[issue.Resolution] {
			continue
		}

		// Calculate which day bucket this falls into
		dayIdx := int(issue.ResolutionDate.Sub(startTime).Hours() / 24)
		if dayIdx >= 0 && dayIdx < days {
			buckets[dayIdx]++
		}
	}

	return &Histogram{Counts: buckets}
}
