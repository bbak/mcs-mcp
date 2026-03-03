package eventlog

import (
	"mcs-mcp/internal/jira"
	"time"
)

// ReconstructIssue builds a Domain Issue purely mechanically from a chronological stream of events.
// It aggregates state based strictly on factual occurrences, without inferring "finished" meanings.
// If referenceDate is non-zero, it is used as the "Now" for open items.
func ReconstructIssue(events []IssueEvent, referenceDate time.Time) jira.Issue {
	if len(events) == 0 {
		return jira.Issue{}
	}

	first := events[0]
	issue := jira.Issue{
		Key:               first.IssueKey,
		IssueType:         first.IssueType,
		StatusResidency:   make(map[string]int64),
		Created:           time.UnixMicro(first.Timestamp), // Defensive default in case birth event is missing
		HasSyntheticBirth: true,                            // Assume synthetic until proven otherwise
	}
	issue.ProjectKey = jira.ExtractProjectKey(first.IssueKey)

	for _, e := range events {
		issue.Updated = time.UnixMicro(e.Timestamp)

		// Signal-Aware application
		if e.EventType == Created || e.ToStatus != "" {
			if e.EventType == Created {
				issue.Created = time.UnixMicro(e.Timestamp)
				issue.HasSyntheticBirth = false // We have a real birth event!
				issue.BirthStatus = e.ToStatus
				issue.BirthStatusID = e.ToStatusID
			} else {
				issue.Transitions = append(issue.Transitions, jira.StatusTransition{
					FromStatus:   e.FromStatus,
					FromStatusID: e.FromStatusID,
					ToStatus:     e.ToStatus,
					ToStatusID:   e.ToStatusID,
					Date:         time.UnixMicro(e.Timestamp),
				})
			}
			issue.Status = e.ToStatus
			issue.StatusID = e.ToStatusID
		}

		if e.Resolution != "" {
			resTS := time.UnixMicro(e.Timestamp)
			issue.ResolutionDate = &resTS
			issue.Resolution = e.Resolution
			issue.ResolutionID = e.ResolutionID
		} else if e.IsUnresolved {
			issue.ResolutionDate = nil
			issue.Resolution = ""
			issue.ResolutionID = ""
		}

		if e.EventType == Flagged {
			issue.Flagged = e.Flagged
		}

		if e.EventType == Created {
			issue.Flagged = e.Flagged
		}

		if e.EventType == Created && e.IsHealed {
			issue.IsMoved = true
		}
	}

	// Calculate factual residency based ONLY on explicit Jira Resolution Date
	issue.StatusResidency, issue.BlockedResidency = CalculateResidencyFromEvents(events, issue.Created, issue.ResolutionDate, issue.Status, issue.StatusID, referenceDate)

	return issue
}

// CalculateResidencyFromEvents computes factual residency times from an event stream by converting to domain transitions.
func CalculateResidencyFromEvents(events []IssueEvent, created time.Time, resolved *time.Time, currentStatus, currentStatusID string, referenceDate time.Time) (map[string]int64, map[string]int64) {
	var transitions []jira.StatusTransition
	var initialStatus string
	var initialStatusID string

	// Pass 1: Extract transitions and find the earliest status
	for _, e := range events {
		if e.EventType == Created || e.ToStatus != "" {
			if initialStatus == "" {
				if e.EventType == Created {
					initialStatus = e.ToStatus
					initialStatusID = e.ToStatusID
				} else {
					initialStatus = e.FromStatus
					initialStatusID = e.FromStatusID
				}
			}

			if e.ToStatus != "" && e.EventType != Created {
				transitions = append(transitions, jira.StatusTransition{
					FromStatus:   e.FromStatus,
					FromStatusID: e.FromStatusID,
					ToStatus:     e.ToStatus,
					ToStatusID:   e.ToStatusID,
					Date:         time.UnixMicro(e.Timestamp),
				})
			}
		}
	}

	// Pass completely nil for mappings to ensure purely mechanical residency calculations
	residency, segments := jira.CalculateResidency(transitions, created, resolved, currentStatus, currentStatusID, nil, initialStatus, initialStatusID, referenceDate)

	// Friction Mapping: Extract blocked intervals and overlay them
	blockedIntervals := ExtractBlockedIntervals(events, created, resolved, referenceDate)
	blockedResidency := jira.CalculateBlockedResidency(segments, blockedIntervals)

	return residency, blockedResidency
}

// ExtractBlockedIntervals identifies periods where an item was flagged as impeded.
func ExtractBlockedIntervals(events []IssueEvent, created time.Time, resolved *time.Time, referenceDate time.Time) []jira.Interval {
	var intervals []jira.Interval
	var currentStart *time.Time

	for _, e := range events {
		if e.Timestamp > referenceDate.UnixMicro() {
			break
		}
		if resolved != nil && e.Timestamp > resolved.UnixMicro() {
			break
		}

		if e.EventType == Flagged {
			if e.Flagged != "" && currentStart == nil {
				ts := time.UnixMicro(e.Timestamp)
				currentStart = &ts
			} else if e.Flagged == "" && currentStart != nil {
				intervals = append(intervals, jira.Interval{
					Start: *currentStart,
					End:   time.UnixMicro(e.Timestamp),
				})
				currentStart = nil
			}
		}
	}

	if currentStart != nil {
		end := referenceDate
		if resolved != nil {
			end = *resolved
		}
		intervals = append(intervals, jira.Interval{
			Start: *currentStart,
			End:   end,
		})
	}

	return intervals
}

