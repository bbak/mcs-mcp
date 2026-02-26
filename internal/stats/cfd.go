package stats

import (
	"mcs-mcp/internal/jira"
	"slices"
)

// CalculateCFDData reconstructions the status population for every day in the window.
func CalculateCFDData(issues []jira.Issue, window AnalysisWindow, mappings map[string]StatusMetadata) CFDResult {
	if len(issues) == 0 {
		return CFDResult{}
	}

	// 1. Identify all statuses and issue types present in the data
	statusSet := make(map[string]bool)
	typeSet := make(map[string]bool)
	for _, issue := range issues {
		typeSet[issue.IssueType] = true
		statusSet[issue.BirthStatus] = true
		for _, tr := range issue.Transitions {
			statusSet[tr.ToStatus] = true
		}
	}

	// 2. Prepare buckets for each day in the window
	buckets := window.Subdivide() // window.Bucket is usually "day" for CFD
	cfdBuckets := make([]CFDBucket, 0, len(buckets))

	availableIssueTypes := make([]string, 0, len(typeSet))
	for t := range typeSet {
		availableIssueTypes = append(availableIssueTypes, t)
	}
	slices.Sort(availableIssueTypes)

	// 3. For each day, reconstruct the population
	for _, bucketStart := range buckets {
		dayEnd := SnapToEnd(bucketStart, window.Bucket)
		dayEndUnix := dayEnd.UnixMicro()

		bucket := CFDBucket{
			Date:        bucketStart,
			Label:       window.GenerateLabel(bucketStart),
			ByIssueType: make(map[string]map[string]int),
		}

		// Initialize maps for each issue type
		for t := range typeSet {
			bucket.ByIssueType[t] = make(map[string]int)
		}

		for _, issue := range issues {
			// Item must be born by the end of this day
			if issue.Created.UnixMicro() > dayEndUnix {
				continue
			}

			// Find status at the end of the day
			status := getStatusAt(issue, dayEndUnix)
			if status != "" {
				bucket.ByIssueType[issue.IssueType][status]++
			}
		}

		cfdBuckets = append(cfdBuckets, bucket)
	}

	// 4. Finalize available statuses (could be sorted by backbone order if we had it)
	availableStatuses := make([]string, 0, len(statusSet))
	for s := range statusSet {
		if s != "" {
			availableStatuses = append(availableStatuses, s)
		}
	}
	// TODO: Use backbone order if possible. For now, alphabetical or by appearance.
	slices.Sort(availableStatuses)

	return CFDResult{
		Buckets:             cfdBuckets,
		Statuses:            availableStatuses,
		AvailableIssueTypes: availableIssueTypes,
	}
}

// getStatusAt determines the status of an issue at a specific microsecond timestamp.
func getStatusAt(issue jira.Issue, ts int64) string {
	// 1. If not yet born, no status
	if issue.Created.UnixMicro() > ts {
		return ""
	}

	// 2. Find the latest transition on or before ts
	currentStatus := issue.BirthStatus
	for _, tr := range issue.Transitions {
		if tr.Date.UnixMicro() <= ts {
			currentStatus = tr.ToStatus
		} else {
			// Transitions are sorted chronologically
			break
		}
	}

	return currentStatus
}
