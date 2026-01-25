package stats

import (
	"mcs-mcp/internal/jira"
	"time"
)

// MetadataSummary provides a high-level overview of a Jira data source.
type MetadataSummary struct {
	TotalIssues            int            `json:"totalIssues"`
	SampleSize             int            `json:"sampleSize"`
	IssueTypes             map[string]int `json:"issueTypes"`
	Statuses               map[string]int `json:"statuses"`
	ResolutionNames        map[string]int `json:"resolutionNames"`
	ResolutionRate         float64        `json:"resolutionRate"`
	FirstResolution        *time.Time     `json:"firstResolution,omitempty"`
	LastResolution         *time.Time     `json:"lastResolution,omitempty"`
	AverageCycleTime       float64        `json:"averageCycleTime,omitempty"` // Days
	AvailableStatuses      interface{}    `json:"availableStatuses,omitempty"`
	HistoricalReachability map[string]int `json:"historicalReachability,omitempty"` // How many issues visited each status
	CommitmentPointHints   []string       `json:"commitmentPointHints,omitempty"`
}

// AnalyzeProbe performs a preliminary analysis on a sample of issues.
func AnalyzeProbe(issues []jira.Issue, totalCount int) MetadataSummary {
	summary := MetadataSummary{
		TotalIssues:            totalCount,
		SampleSize:             len(issues),
		IssueTypes:             make(map[string]int),
		Statuses:               make(map[string]int),
		ResolutionNames:        make(map[string]int),
		HistoricalReachability: make(map[string]int),
	}

	if len(issues) == 0 {
		return summary
	}

	resolvedCount := 0
	var first, last *time.Time

	for _, issue := range issues {
		summary.IssueTypes[issue.IssueType]++
		summary.Statuses[issue.Status]++
		if issue.Resolution != "" {
			summary.ResolutionNames[issue.Resolution]++
		}

		// Track reachability from transitions
		visited := make(map[string]bool)
		// Current status is visited
		visited[issue.Status] = true
		for _, t := range issue.Transitions {
			visited[t.ToStatus] = true
		}
		for status := range visited {
			summary.HistoricalReachability[status]++
		}

		if issue.ResolutionDate != nil {
			resolvedCount++
			if first == nil || issue.ResolutionDate.Before(*first) {
				first = issue.ResolutionDate
			}
			if last == nil || issue.ResolutionDate.After(*last) {
				last = issue.ResolutionDate
			}
		}
	}

	summary.ResolutionRate = float64(resolvedCount) / float64(len(issues))
	summary.FirstResolution = first
	summary.LastResolution = last

	return summary
}
