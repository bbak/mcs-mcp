package eventlog

import (
	"mcs-mcp/internal/jira"
	"mcs-mcp/internal/stats"
	"sort"
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

		switch e.EventType {
		case Created:
			s.currentStatus = e.ToStatus
		case Transitioned:
			s.currentStatus = e.ToStatus
			if s.commitmentDate == 0 && e.ToStatus == commitmentPoint {
				s.commitmentDate = e.Timestamp
			}
		case Resolved, Closed:
			s.isFinished = true
		case Unresolved:
			s.isFinished = false
		}

		// Reactive check: If this was a transition, update finished state based on target status (Case 2 & 3)
		if e.EventType == Transitioned || e.EventType == Created {
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

	for _, e := range events {
		isDelivery := e.EventType == Resolved
		if !isDelivery && e.EventType == Transitioned {
			if m, ok := mappings[e.ToStatus]; ok && (m.Tier == "Finished" || m.Role == "terminal") && m.Outcome == "delivered" {
				isDelivery = true
			}
		}

		if isDelivery {
			dateStr := time.UnixMicro(e.Timestamp).Format("2006-01-02")
			counts[dateStr]++
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

	resAtStart := 0
	for _, e := range events {
		if e.EventType == Resolved {
			resAtStart++
		}
	}

	first := events[0]
	issue := jira.Issue{
		Key:             first.IssueKey,
		IssueType:       first.IssueType,
		StatusResidency: make(map[string]int64),
	}
	issue.ProjectKey = stats.ExtractProjectKey(first.IssueKey)

	var lastMoveDate int64

	// Group by timestamp to handle "transactions" (concurrent events)
	for i := 0; i < len(events); {
		j := i
		ts := events[i].Timestamp
		for j < len(events) && events[j].Timestamp == ts {
			j++
		}

		// Apply all events in this transaction
		hadResolved := false
		for k := i; k < j; k++ {
			e := events[k]
			issue.Updated = time.UnixMicro(e.Timestamp)

			switch e.EventType {
			case Created:
				issue.Created = time.UnixMicro(e.Timestamp)
				issue.Status = e.ToStatus
				issue.StatusID = e.ToStatusID
			case Transitioned:
				issue.Transitions = append(issue.Transitions, jira.StatusTransition{
					FromStatus:   e.FromStatus,
					FromStatusID: e.FromStatusID,
					ToStatus:     e.ToStatus,
					ToStatusID:   e.ToStatusID,
					Date:         time.UnixMicro(e.Timestamp),
				})
				issue.Status = e.ToStatus
				issue.StatusID = e.ToStatusID
			case Resolved:
				resTS := time.UnixMicro(e.Timestamp)
				issue.ResolutionDate = &resTS
				issue.Resolution = e.Resolution
				hadResolved = true
			case Unresolved:
				issue.ResolutionDate = nil
				issue.Resolution = ""
			case Moved:
				issue.IsMoved = true
				lastMoveDate = e.Timestamp
			}
		}

		// Evaluated final state after the transaction
		// Reactive check: If we are in a non-terminal status, implicitly clear resolution (Case 2 & 3)
		// UNLESS we just got a Resolved event in this same transaction (trusting the change-set).
		if len(finishedStatuses) > 0 && !finishedStatuses[issue.Status] && !hadResolved {
			issue.ResolutionDate = nil
			issue.Resolution = ""
		}

		i = j
	}

	// We pass time.Time{} as referenceDate to behave as "Now" for backward compatibility
	// effectively relying on CalculateResidencyFromEvents default behavior or we can pass Now explicitly.
	// Actually, ReconstructIssue doesn't have a reference date argument, so it inevitably uses "Now" for open items.
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
		if (e.EventType == Transitioned || e.EventType == Created) && (lastMove == 0 || e.Timestamp >= lastMove) {
			if initialStatus == "" && e.EventType == Created {
				initialStatus = e.ToStatus
			} else if initialStatus == "" && e.EventType == Transitioned {
				initialStatus = e.FromStatus
			}

			if e.EventType == Transitioned {
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
