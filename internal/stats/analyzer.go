package stats

import (
	"mcs-mcp/internal/jira"
	"time"

	"github.com/rs/zerolog/log"
)

// SumRangeDuration calculates the total time spent in a list of statuses for a given issue.
func SumRangeDuration(issue jira.Issue, rangeStatuses []string) float64 {
	var total float64
	for _, status := range rangeStatuses {
		if s, ok := issue.StatusResidency[status]; ok {
			total += float64(s) / 86400.0
		}
	}
	return total
}

// FilterDelivered returns only items that have a 'delivered' outcome.
func FilterDelivered(issues []jira.Issue) []jira.Issue {
	var delivered []jira.Issue
	for _, issue := range issues {
		if IsDelivered(issue) {
			delivered = append(delivered, issue)
		}
	}
	return delivered
}

// IsDelivered returns true if the issue has a 'delivered' outcome.
func IsDelivered(issue jira.Issue) bool {
	return issue.Outcome == "delivered"
}

// HasExited returns true if the issue has left the system (any non-empty outcome).
func HasExited(issue jira.Issue) bool {
	return issue.Outcome != ""
}

// isFinishedInMapping checks, via ID-first then name fallback, whether a status is in the Finished tier.
func isFinishedInMapping(statusID, statusName string, mappings map[string]StatusMetadata) bool {
	if m, ok := mappings[statusID]; ok {
		return m.Tier == "Finished"
	}
	return mappings[statusName].Tier == "Finished"
}

// DetermineOutcome identifies the terminal outcome of an issue (e.g., "delivered", "abandoned")
// using a two-step fallback approach and mutates the issue to store the Outcome and OutcomeDate.
func DetermineOutcome(issue *jira.Issue, resolutions map[string]string, mappings map[string]StatusMetadata) {
	// Short-Circuit: bare-metal discovery before mapping configuration
	if len(resolutions) == 0 && len(mappings) == 0 {
		return
	}

	issue.Outcome = ""
	issue.OutcomeDate = nil

	// 1. Primary Signal: Jira Resolution
	if issue.ResolutionID != "" {
		issue.OutcomeDate = issue.ResolutionDate
		if outcome, ok := resolutions[issue.ResolutionID]; ok {
			issue.Outcome = outcome
		} else {
			if len(resolutions) > 0 {
				log.Warn().Str("issue", issue.Key).Str("resolution", issue.Resolution).Str("resolutionId", issue.ResolutionID).Msg("Explicit resolution lacked mapping. Defaulting to 'delivered'")
			}
			issue.Outcome = "delivered"
		}
		return
	} else if issue.Resolution != "" {
		issue.OutcomeDate = issue.ResolutionDate
		if outcome, ok := resolutions[issue.Resolution]; ok {
			issue.Outcome = outcome
		} else {
			if len(resolutions) > 0 {
				log.Warn().Str("issue", issue.Key).Str("resolution", issue.Resolution).Msg("Explicit resolution lacked mapping. Defaulting to 'delivered'")
			}
			issue.Outcome = "delivered"
		}
		return
	}

	// 2. Fallback: Workflow Status Mapping
	isFinished := isFinishedInMapping(issue.StatusID, issue.Status, mappings)
	if isFinished {
		if m, ok := mappings[issue.StatusID]; ok && m.Outcome != "" {
			issue.Outcome = m.Outcome
		} else if m, ok := mappings[issue.Status]; ok && m.Outcome != "" {
			issue.Outcome = m.Outcome
		} else {
			issue.Outcome = "delivered" // Default optimistic if mapped as finished but no outcome assigned
		}
	}

	if isFinished {
		// Synthesize OutcomeDate from transition history
		var streakStart *time.Time
		for i := len(issue.Transitions) - 1; i >= 0; i-- {
			evt := issue.Transitions[i]
			if !isFinishedInMapping(evt.ToStatusID, evt.ToStatus, mappings) {
				break
			}
			streakStart = &evt.Date
		}

		if streakStart != nil {
			issue.OutcomeDate = streakStart
		} else if isFinishedInMapping(issue.BirthStatusID, issue.BirthStatus, mappings) {
			issue.OutcomeDate = &issue.Created
		} else {
			issue.OutcomeDate = &issue.Updated
		}
	}
}

// SynthesizeResolutionDate infers a resolution date for items that are in a 'Finished' tier
// but lack an explicit Jira resolution date. This is required for temporal calculations.
func SynthesizeResolutionDate(issue *jira.Issue, mappings map[string]StatusMetadata) {
	if issue.ResolutionDate != nil {
		return
	}

	if !isFinishedInMapping(issue.StatusID, issue.Status, mappings) {
		return
	}

	// Synthesize ResolutionDate (streak start)
	var streakStart *time.Time
	for i := len(issue.Transitions) - 1; i >= 0; i-- {
		evt := issue.Transitions[i]
		if !isFinishedInMapping(evt.ToStatusID, evt.ToStatus, mappings) {
			break
		}
		streakStart = &evt.Date
	}

	if streakStart != nil {
		issue.ResolutionDate = streakStart
	} else if isFinishedInMapping(issue.BirthStatusID, issue.BirthStatus, mappings) {
		issue.ResolutionDate = &issue.Created
	} else {
		issue.ResolutionDate = &issue.Updated
	}
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
// The caller must provide now to allow time-travel semantics via Server.Clock().
func FilterIssuesByResolutionWindow(issues []jira.Issue, days int, cutoff time.Time, now time.Time) []jira.Issue {
	if days <= 0 && cutoff.IsZero() {
		return issues
	}

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
// The caller must provide now to allow time-travel semantics via Server.Clock().
func ApplyBackflowPolicy(issues []jira.Issue, weights map[string]int, commitmentWeight int, now time.Time) []jira.Issue {
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
			now,
		)
		clean = append(clean, newIssue)
	}
	return clean
}
