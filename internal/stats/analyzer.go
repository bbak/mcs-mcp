package stats

import (
	"mcs-mcp/internal/jira"
)

// SumRangeDuration calculates the total time spent in a list of statuses for a given issue.
func SumRangeDuration(issue jira.Issue, rangeStatuses []string) float64 {
	var total float64
	for _, status := range rangeStatuses {
		if s, ok := GetResidencyCaseInsensitive(issue.StatusResidency, status); ok {
			total += float64(s) / 86400.0
		}
	}
	return total
}
