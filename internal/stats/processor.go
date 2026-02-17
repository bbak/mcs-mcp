package stats

import (
	"mcs-mcp/internal/jira"
	"time"
)

// FilterDelivered returns only items that have a 'delivered' outcome.
func FilterDelivered(issues []jira.Issue, resolutions map[string]string, mappings map[string]StatusMetadata) []jira.Issue {
	var delivered []jira.Issue
	for _, issue := range issues {
		if IsDelivered(issue, resolutions, mappings) {
			delivered = append(delivered, issue)
		}
	}
	return delivered
}

// IsDelivered returns true if the issue has a 'delivered' outcome.
func IsDelivered(issue jira.Issue, resolutions map[string]string, mappings map[string]StatusMetadata) bool {
	// 1. Primary Signal: Jira Resolution
	if issue.Resolution != "" {
		if outcome, ok := resolutions[issue.Resolution]; ok {
			return outcome == "delivered"
		}
		// Hardcoded fallbacks if map is incomplete
		if issue.Resolution == "Fixed" || issue.Resolution == "Done" || issue.Resolution == "Complete" {
			return true
		}
	}

	// 2. Secondary Signal: Status Metadata
	if issue.ResolutionDate != nil {
		if m, ok := GetMetadataRobust(mappings, issue.StatusID, issue.Status); ok {
			return m.Outcome == "delivered"
		}
	}

	return false
}

// Dataset binds a SourceContext with its fetched and processed issues.
type Dataset struct {
	Context   jira.SourceContext `json:"context"`
	Issues    []jira.Issue       `json:"issues"`
	FetchedAt time.Time          `json:"fetchedAt"`
}

// Interval represents a time range.
type Interval struct {
	Start time.Time
	End   time.Time
}

// FilterTransitions returns only transitions that occurred after a specific date.
func FilterTransitions(transitions []jira.StatusTransition, since time.Time) []jira.StatusTransition {
	filtered := make([]jira.StatusTransition, 0)
	for _, t := range transitions {
		if !t.Date.Before(since) {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

// FilterIssuesByResolutionWindow returns items resolved within the last N days,
// but never earlier than the specified cutoff date (to avoid initial ingestion noise).
func FilterIssuesByResolutionWindow(issues []jira.Issue, days int, cutoff time.Time) []jira.Issue {
	if days <= 0 && cutoff.IsZero() {
		return issues
	}

	now := time.Now()
	windowStart := now.AddDate(0, 0, -days)

	// If a cutoff is provided, we take the LATEST of windowStart and cutoff
	if !cutoff.IsZero() && cutoff.After(windowStart) {
		windowStart = cutoff
	}

	var filtered []jira.Issue
	for _, iss := range issues {
		if iss.ResolutionDate != nil && !iss.ResolutionDate.Before(windowStart) {
			filtered = append(filtered, iss)
		}
	}
	return filtered
}

// ApplyBackflowPolicy resets the implementation clock if an item returns to the Demand tier.
func ApplyBackflowPolicy(issues []jira.Issue, weights map[string]int, commitmentWeight int) []jira.Issue {
	clean := make([]jira.Issue, 0, len(issues))
	for _, issue := range issues {
		lastBackflowIdx := -1
		for j, t := range issue.Transitions {
			// Weights are numeric: 1=Demand, 2=Start of Downstream.
			// Any move TO a status with weight < commitmentWeight is a backflow.
			statusID := t.ToStatusID
			if statusID == "" {
				statusID = t.ToStatus
			}
			if w, ok := GetWeightRobust(weights, t.ToStatusID, t.ToStatus); ok && w < commitmentWeight {
				lastBackflowIdx = j
			}
		}

		if lastBackflowIdx == -1 {
			clean = append(clean, issue)
			continue
		}

		newIssue := issue
		newIssue.Transitions = FilterTransitions(issue.Transitions, issue.Transitions[lastBackflowIdx].Date)
		newIssue.IsMoved = true // Treat as a reset

		// Only wipe resolution if the backflow actually re-opened the item
		// (i.e., the resolution happened BEFORE the backflow or we are now in a non-terminal status)
		isTerminal := false
		if weights != nil {
			if w, ok := GetWeightRobust(weights, newIssue.StatusID, newIssue.Status); ok && w >= commitmentWeight {
				isTerminal = true // Status is still downstream/finished
			}
		}

		if !isTerminal {
			newIssue.ResolutionDate = nil
			newIssue.Resolution = ""
		}

		// Recalculate residency starting from the backflow point
		newIssue.StatusResidency, _ = jira.CalculateResidency(
			newIssue.Transitions,
			issue.Transitions[lastBackflowIdx].Date, // Use the backflow date as the new birth anchor
			issue.ResolutionDate,
			issue.Status,
			nil, // finishedStatuses not needed if we are just rebuilding from trans
			issue.Transitions[lastBackflowIdx].ToStatus,
			time.Time{}, // Use Now
		)
		clean = append(clean, newIssue)
	}
	return clean
}

// CalculateBlockedResidency computes the overlapping time between status segments and blocked intervals.
func CalculateBlockedResidency(statusSegments []jira.StatusSegment, blockedIntervals []Interval) map[string]int64 {
	blockedResidency := make(map[string]int64)

	for _, status := range statusSegments {
		var totalBlockedSeconds int64
		for _, blocked := range blockedIntervals {
			// Find overlap between [status.Start, status.End] and [blocked.Start, blocked.End]
			overlapStart := status.Start
			if blocked.Start.After(overlapStart) {
				overlapStart = blocked.Start
			}

			overlapEnd := status.End
			if blocked.End.Before(overlapEnd) {
				overlapEnd = blocked.End
			}

			if overlapStart.Before(overlapEnd) {
				totalBlockedSeconds += int64(overlapEnd.Sub(overlapStart).Seconds())
			}
		}
		if totalBlockedSeconds > 0 {
			blockedResidency[status.Status] += totalBlockedSeconds
		}
	}

	return blockedResidency
}
