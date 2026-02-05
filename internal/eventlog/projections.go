package eventlog

import (
	"mcs-mcp/internal/jira"
	"mcs-mcp/internal/stats"
	"sort"
	"strings"
	"time"
)

// WIPItem represents an active work item derived from the event log.
type WIPItem struct {
	IssueKey           string
	IssueType          string
	CurrentStatus      string
	CommitmentDate     time.Time
	AgeSinceCommitment float64 // Days
}

// BuildWIPProjection identifies active items based on a commitment point and current mappings.
func BuildWIPProjection(events []IssueEvent, commitmentPoint string, mappings map[string]stats.StatusMetadata, referenceDate time.Time) []WIPItem {
	type state struct {
		key            string
		issueType      string
		currentStatus  string
		commitmentDate int64
		isFinished     bool
	}
	states := make(map[string]*state)

	refMicros := referenceDate.UnixMicro()
	if referenceDate.IsZero() {
		refMicros = time.Now().UnixMicro()
	}

	for _, e := range events {
		if e.Timestamp > refMicros {
			continue
		}

		s, ok := states[e.IssueKey]
		if !ok {
			s = &state{key: e.IssueKey, issueType: e.IssueType}
			states[e.IssueKey] = s
		}

		// Signal-Aware update
		if e.ToStatus != "" {
			s.currentStatus = e.ToStatus
			if s.commitmentDate == 0 && e.ToStatus == commitmentPoint {
				s.commitmentDate = e.Timestamp
			}
		}

		if e.Resolution != "" {
			s.isFinished = true
		} else if e.IsUnresolved {
			s.isFinished = false
		}

		// Reactive check: Update finished state based on target status if present
		if e.ToStatus != "" {
			if m, ok := mappings[s.currentStatus]; ok {
				if m.Tier == "Finished" || m.Role == "terminal" {
					s.isFinished = true
				} else {
					s.isFinished = false
				}
			}
		}
	}

	var wip []WIPItem
	for _, s := range states {
		if s.commitmentDate != 0 && !s.isFinished {
			wip = append(wip, WIPItem{
				IssueKey:           s.key,
				IssueType:          s.issueType,
				CurrentStatus:      s.currentStatus,
				CommitmentDate:     time.UnixMicro(s.commitmentDate),
				AgeSinceCommitment: float64(refMicros-s.commitmentDate) / 86400000000.0, // 86.4B micros in a day
			})
		}
	}
	return wip
}

// ThroughputBucket represents delivery volume for a specific day.
type ThroughputBucket struct {
	Date  time.Time
	Count int
}

// BuildThroughputProjection aggregates resolution events into daily counts.
func BuildThroughputProjection(events []IssueEvent, mappings map[string]stats.StatusMetadata) []ThroughputBucket {
	counts := make(map[string]int)

	// Track the state of each issue to detect the moment it becomes 'delivered'
	type issueState struct {
		isDelivered bool
	}
	states := make(map[string]*issueState)

	for _, e := range events {
		s, ok := states[e.IssueKey]
		if !ok {
			s = &issueState{}
			states[e.IssueKey] = s
		}

		wasDelivered := s.isDelivered
		nowDelivered := false

		// Signal-Aware detection
		if e.Resolution != "" {
			nowDelivered = true
		} else if e.ToStatus != "" {
			if m, ok := mappings[e.ToStatus]; ok && (m.Tier == "Finished" || m.Role == "terminal") && m.Outcome == "delivered" {
				nowDelivered = true
			}
		}

		// Count only the first transition into a delivered state per issue
		if nowDelivered && !wasDelivered {
			dateStr := time.UnixMicro(e.Timestamp).Format("2006-01-02")
			counts[dateStr]++
			s.isDelivered = true
		} else if !nowDelivered && e.IsUnresolved {
			// If explicitly unresolved, allow it to be counted again later
			s.isDelivered = false
		}
	}

	var result []ThroughputBucket
	for dStr, count := range counts {
		t, _ := time.Parse("2006-01-02", dStr)
		result = append(result, ThroughputBucket{Date: t, Count: count})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Date.Before(result[j].Date)
	})

	return result
}

// ReconstructIssue builds a Domain Issue from a chronological stream of events.
// If referenceDate is non-zero, it is used as the "Now" for open items.
func ReconstructIssue(events []IssueEvent, finishedStatuses map[string]bool, referenceDate time.Time) jira.Issue {
	if len(events) == 0 {
		return jira.Issue{}
	}

	first := events[0]
	issue := jira.Issue{
		Key:             first.IssueKey,
		IssueType:       first.IssueType,
		StatusResidency: make(map[string]int64),
	}
	issue.ProjectKey = stats.ExtractProjectKey(first.IssueKey)

	var lastMoveDate int64

	// simplified loop: events are now packed atomic change-sets
	for _, e := range events {
		issue.Updated = time.UnixMicro(e.Timestamp)

		// Signal-Aware application
		if e.EventType == Created || e.ToStatus != "" {
			if e.EventType == Created {
				issue.Created = time.UnixMicro(e.Timestamp)
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
		} else if e.IsUnresolved {
			issue.ResolutionDate = nil
			issue.Resolution = ""
		}

		if e.IsMoved {
			issue.IsMoved = true
			lastMoveDate = e.Timestamp
		}

		// Final evaluation after applying all signals in this event
		// Guard: If we are in a status known for sure NOT to be finished, implicitly clear resolution
		// UNLESS this same event just provided a Resolution.
		if len(finishedStatuses) > 0 && e.Resolution == "" {
			isFin := finishedStatuses[issue.Status] || (issue.StatusID != "" && finishedStatuses[issue.StatusID])
			if !isFin {
				// Case-insensitive fallback for Name
				lowerStatus := strings.ToLower(issue.Status)
				for name, ok := range finishedStatuses {
					if ok && strings.ToLower(name) == lowerStatus {
						isFin = true
						break
					}
				}
			}

			if !isFin {
				issue.ResolutionDate = nil
				issue.Resolution = ""
			}
		}
	}

	issue.StatusResidency = CalculateResidencyFromEvents(events, issue.Created, issue.ResolutionDate, issue.Status, finishedStatuses, lastSinceMove(lastMoveDate), referenceDate)

	return issue
}

func lastSinceMove(lastMove int64) int64 {
	return lastMove
}

// CalculateResidencyFromEvents computes residency times from an event stream by converting to domain transitions.
func CalculateResidencyFromEvents(events []IssueEvent, created time.Time, resolved *time.Time, currentStatus string, finished map[string]bool, lastMove int64, referenceDate time.Time) map[string]int64 {
	var transitions []jira.StatusTransition

	// Track the very first "From" status if possible for the birth duration
	var initialStatus string

	for _, e := range events {
		if (e.EventType == Created || e.ToStatus != "") && (lastMove == 0 || e.Timestamp >= lastMove) {
			if initialStatus == "" && e.EventType == Created {
				initialStatus = e.ToStatus
			} else if initialStatus == "" && e.ToStatus != "" {
				initialStatus = e.FromStatus
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

	return stats.CalculateResidency(transitions, created, resolved, currentStatus, finished, initialStatus, referenceDate)
}
