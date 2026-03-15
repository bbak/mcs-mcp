package stats

import (
	"mcs-mcp/internal/jira"
)

// GetStratifiedThroughput aggregates resolved items into time buckets, both pooled and stratified by type.
func GetStratifiedThroughput(issues []jira.Issue, window AnalysisWindow) StratifiedThroughput {
	buckets := window.Subdivide()
	pooled := make([]int, len(buckets))
	byType := make(map[string][]int)

	for _, issue := range issues {
		if !IsDelivered(issue) {
			continue
		}

		if issue.OutcomeDate == nil {
			continue
		}

		idx := window.FindBucketIndex(*issue.OutcomeDate)
		if idx < 0 || idx >= len(buckets) {
			continue
		}

		pooled[idx]++
		if _, ok := byType[issue.IssueType]; !ok {
			byType[issue.IssueType] = make([]int, len(buckets))
		}
		byType[issue.IssueType][idx]++
	}

	return StratifiedThroughput{
		Pooled: pooled,
		ByType: byType,
	}
}
