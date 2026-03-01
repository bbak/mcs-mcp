package stats

import (
	"mcs-mcp/internal/jira"
	"time"
)

// FilterDelivered returns only items that have a 'delivered' outcome.
func FilterDelivered(issues []jira.Issue, resolutions map[string]string, mappings map[string]StatusMetadata) []jira.Issue {
	var delivered []jira.Issue
	for _, issue := range issues {
		if IsDelivered(issue, resolutions) {
			delivered = append(delivered, issue)
		}
	}
	return delivered
}

// IsDelivered returns true if the issue has a 'delivered' outcome.
// It relies on explicit Jira resolutions or the synthesized resolution from Layer 3.
func IsDelivered(issue jira.Issue, resolutions map[string]string) bool {
	// 1. Primary Signal: Jira Resolution
	if issue.ResolutionID != "" {
		if outcome, ok := resolutions[issue.ResolutionID]; ok {
			return outcome == "delivered"
		}
	}
	if issue.Resolution != "" {
		if issue.Resolution == "Synthetic-delivered" {
			return true
		}
		if issue.Resolution == "Synthetic-abandoned" {
			return false
		}
		if outcome, ok := resolutions[issue.Resolution]; ok {
			return outcome == "delivered"
		}
		// Hardcoded fallbacks if map is incomplete
		if issue.Resolution == "Fixed" || issue.Resolution == "Done" || issue.Resolution == "Complete" {
			return true
		}
	}

	return false
}

// DetermineOutcome interprets the item's state based on metadata mappings to finalize
// its terminal properties. Specifically, if the item is 'Finished' but lacks a Jira ResolutionDate,
// it falls back to synthesizing the date based on when it entered the continuous finished streak.
func DetermineOutcome(issue jira.Issue, mappings map[string]StatusMetadata) jira.Issue {
	isFinished := false

	// Check if already formally resolved
	if issue.ResolutionDate != nil {
		isFinished = true
	} else if m, ok := mappings[issue.StatusID]; ok && m.Tier == "Finished" {
		isFinished = true
	} else {
		// Fallback for name
		for name, m := range mappings {
			if m.Tier == "Finished" && name == issue.Status {
				isFinished = true
				break
			}
		}
	}

	if isFinished && issue.ResolutionDate == nil {
		// Synthesize ResolutionDate (streak start)
		var streakStart *time.Time
		for i := len(issue.Transitions) - 1; i >= 0; i-- {
			evt := issue.Transitions[i]
			isPreviousFin := false
			if m, ok := mappings[evt.ToStatusID]; ok && m.Tier == "Finished" {
				isPreviousFin = true
			} else {
				for name, m := range mappings {
					if m.Tier == "Finished" && name == evt.ToStatus {
						isPreviousFin = true
						break
					}
				}
			}

			if !isPreviousFin {
				break
			}
			streakStart = &evt.Date
		}

		if streakStart != nil {
			issue.ResolutionDate = streakStart
		} else if mappings[issue.BirthStatusID].Tier == "Finished" || mappings[issue.BirthStatus].Tier == "Finished" {
			issue.ResolutionDate = &issue.Created
		} else {
			issue.ResolutionDate = &issue.Updated
		}

		// Inject Outcome so downstream IsDelivered doesn't need to consult mappings
		if m, ok := mappings[issue.StatusID]; ok && m.Outcome != "" {
			issue.Resolution = "Synthetic-" + m.Outcome
		} else {
			for name, m := range mappings {
				if name == issue.Status && m.Outcome != "" {
					issue.Resolution = "Synthetic-" + m.Outcome
					break
				}
			}
		}
		if issue.Resolution == "" {
			issue.Resolution = "Synthetic-delivered" // Default optimistic
		}
	}

	return issue
}

// DetermineTier identifies whether an issue is in Demand, Upstream, Downstream, or Finished
// based on the provided commitment point and mappings.
func DetermineTier(issue jira.Issue, commitmentPoint string, mappings map[string]StatusMetadata) string {
	if mappings[issue.StatusID].Tier == "Finished" || mappings[issue.Status].Tier == "Finished" {
		return "Finished"
	}
	// Simplified active tier logic based on metadata (the UI specifies the tier for each status)
	if m, ok := mappings[issue.StatusID]; ok && m.Tier != "" {
		return m.Tier
	}
	if m, ok := mappings[issue.Status]; ok && m.Tier != "" {
		return m.Tier
	}
	return "Unknown"
}

// Dataset binds a SourceContext with its fetched and processed issues.
type Dataset struct {
	Context   jira.SourceContext `json:"context"`
	Issues    []jira.Issue       `json:"issues"`
	FetchedAt time.Time          `json:"fetchedAt"`
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
			if w, ok := weights[statusID]; ok && w < commitmentWeight {
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
			if w, ok := weights[newIssue.StatusID]; ok && w >= commitmentWeight {
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
			newIssue.Status,
			newIssue.StatusID,
			nil, // finishedStatuses not needed if we are just rebuilding from trans
			issue.Transitions[lastBackflowIdx].ToStatus,
			issue.Transitions[lastBackflowIdx].ToStatusID,
			time.Now(),
		)
		clean = append(clean, newIssue)
	}
	return clean
}
