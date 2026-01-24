package simulation

import (
	"mcs-mcp/internal/jira"
	"time"
)

// Histogram tracks daily throughput counts.
type Histogram struct {
	Counts []int
	Meta   map[string]interface{}
}

// NewHistogram creates a histogram from a list of resolved issues.
func NewHistogram(issues []jira.Issue, startTime, endTime time.Time, issueTypes []string, resolutions []string) *Histogram {
	// Create maps for fast lookup
	resMap := make(map[string]bool)
	for _, r := range resolutions {
		resMap[r] = true
	}
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
	typeDist := make(map[string]int)
	for _, issue := range issues {
		typeDist[issue.IssueType]++
	}

	analyzedCount := 0
	for _, issue := range issues {
		if issue.ResolutionDate == nil {
			continue
		}
		if len(issueTypes) > 0 && !typeMap[issue.IssueType] {
			continue
		}
		if len(resolutions) > 0 && !resMap[issue.Resolution] {
			continue
		}

		analyzedCount++
		dayIdx := int(issue.ResolutionDate.Sub(startTime).Hours() / 24)
		if dayIdx >= 0 && dayIdx < days {
			buckets[dayIdx]++
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
		"issues_total":       len(issues),
		"issues_analyzed":    analyzedCount,
		"days_in_sample":     days,
		"throughput_overall": avgAcross,
		"throughput_recent":  recentAvg,
		"type_distribution":  typeDist,
	}

	return &Histogram{
		Counts: buckets,
		Meta:   meta,
	}
}
