package discovery

import (
	"mcs-mcp/internal/jira"
	"slices"
	"time"
)

// SelectDiscoverySample filters a set of issues to provide a 200-item "healthy" subset for discovery.
func SelectDiscoverySample(issues []jira.Issue, targetSize int) []jira.Issue {
	if len(issues) <= targetSize {
		return issues
	}

	// 1. Sort by Updated DESC to ensure we get most recent activity
	slices.SortStableFunc(issues, func(a, b jira.Issue) int {
		return b.Updated.Compare(a.Updated)
	})

	now := time.Now()
	oneYearAgo := now.AddDate(-1, 0, 0)
	twoYearsAgo := now.AddDate(-2, 0, 0)
	threeYearsAgo := now.AddDate(-3, 0, 0)

	var pool1y []jira.Issue
	for _, iss := range issues {
		if !iss.Created.Before(oneYearAgo) {
			pool1y = append(pool1y, iss)
		}
	}

	// 2. Check if we have enough 1y items
	if len(pool1y) >= targetSize {
		return pool1y[:targetSize]
	}

	// 3. Expansion Logic
	var fallbackPool []jira.Issue
	limitDate := twoYearsAgo
	if len(pool1y) < 100 {
		limitDate = threeYearsAgo
	}

	for _, iss := range issues {
		// Only consider items OLDER than 1y but NEWER than limit
		if iss.Created.Before(oneYearAgo) && iss.Created.After(limitDate) {
			fallbackPool = append(fallbackPool, iss)
		}
	}

	// Union
	result := append([]jira.Issue{}, pool1y...)
	remaining := targetSize - len(result)
	if remaining > 0 {
		if len(fallbackPool) > remaining {
			result = append(result, fallbackPool[:remaining]...)
		} else {
			result = append(result, fallbackPool...)
		}
	}

	return result
}
