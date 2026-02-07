package simulation

import (
	"mcs-mcp/internal/jira"
	"mcs-mcp/internal/stats"
	"time"
)

// Histogram tracks daily throughput counts.
type Histogram struct {
	Counts []int
	Meta   map[string]interface{}
}

// NewHistogram creates a histogram from a list of resolved issues.
func NewHistogram(issues []jira.Issue, startTime, endTime time.Time, issueTypes []string, mappings map[string]stats.StatusMetadata, resolutionMappings map[string]string) *Histogram {
	// Create maps for fast lookup
	typeMap := make(map[string]bool)
	for _, t := range issueTypes {
		typeMap[t] = true
	}

	// Calculate number of days in range
	days := int(endTime.Sub(startTime).Hours()/24) + 1
	if days <= 0 {
		return &Histogram{Counts: []int{0}}
	}

	buckets := make([]int, days)
	typeCounts := make(map[string]int)
	totalDelivered := 0

	droppedByResolution := 0
	droppedByWindow := 0
	for _, issue := range issues {
		var resDate time.Time
		if !stats.IsDelivered(issue, resolutionMappings, mappings) {
			continue
		}

		if issue.ResolutionDate != nil {
			resDate = *issue.ResolutionDate
		} else {
			resDate = issue.Updated
		}

		if resDate.IsZero() {
			continue
		}

		// Fill throughput buckets based on ALL types (System Capacity)
		dayIdx := int(resDate.Sub(startTime).Hours() / 24)
		if dayIdx >= 0 && dayIdx < days {
			buckets[dayIdx]++
			typeCounts[issue.IssueType]++
			totalDelivered++
		} else {
			droppedByWindow++
		}
	}

	// Calculate normalized distribution
	typeDist := make(map[string]float64)
	if totalDelivered > 0 {
		for t, c := range typeCounts {
			typeDist[t] = float64(c) / float64(totalDelivered)
		}
	}

	avgAcross := 0.0
	recentAvg := 0.0
	totalCount := 0
	recentCount := 0
	if days > 0 {
		for i, c := range buckets {
			totalCount += c
			if i >= days-30 {
				recentCount += c
			}
		}
		avgAcross = float64(totalCount) / float64(days)
		recentDays := 30
		if days < 30 {
			recentDays = days
		}
		recentAvg = float64(recentCount) / float64(recentDays)
	}

	meta := map[string]interface{}{
		"issues_total":          len(issues),
		"issues_analyzed":       totalDelivered,
		"dropped_by_resolution": droppedByResolution,
		"dropped_by_window":     droppedByWindow,
		"days_in_sample":        days,
		"throughput_overall":    avgAcross,
		"throughput_recent":     recentAvg,
		"type_distribution":     typeDist,
		"type_counts":           typeCounts,
	}

	return &Histogram{
		Counts: buckets,
		Meta:   meta,
	}
}
