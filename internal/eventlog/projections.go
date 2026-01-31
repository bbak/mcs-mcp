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
func BuildWIPProjection(events []IssueEvent, commitmentPoint string, mappings map[string]stats.StatusMetadata) []WIPItem {
	type state struct {
		key            string
		issueType      string
		currentStatus  string
		commitmentDate int64
		isFinished     bool
	}
	states := make(map[string]*state)

	for _, e := range events {
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
		}

		if m, ok := mappings[s.currentStatus]; ok && m.Tier == "Finished" {
			s.isFinished = true
		}
	}

	var wip []WIPItem
	now := time.Now().UnixMicro()
	for _, s := range states {
		if s.commitmentDate != 0 && !s.isFinished {
			wip = append(wip, WIPItem{
				IssueKey:           s.key,
				IssueType:          s.issueType,
				CurrentStatus:      s.currentStatus,
				CommitmentDate:     time.UnixMicro(s.commitmentDate),
				AgeSinceCommitment: float64(now-s.commitmentDate) / 86400000000.0, // 86.4B micros in a day
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
			if m, ok := mappings[e.ToStatus]; ok && m.Tier == "Finished" && m.Outcome == "delivered" {
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
func ReconstructIssue(events []IssueEvent, finishedStatuses map[string]bool) jira.Issue {
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

	for _, e := range events {
		switch e.EventType {
		case Created:
			issue.Created = time.UnixMicro(e.Timestamp)
			issue.Status = e.ToStatus
		case Transitioned:
			issue.Transitions = append(issue.Transitions, jira.StatusTransition{
				FromStatus: e.FromStatus,
				ToStatus:   e.ToStatus,
				Date:       time.UnixMicro(e.Timestamp),
			})
			issue.Status = e.ToStatus
		case Resolved:
			ts := time.UnixMicro(e.Timestamp)
			issue.ResolutionDate = &ts
			issue.Resolution = e.Resolution
		case Moved:
			issue.IsMoved = true
			lastMoveDate = e.Timestamp
		}
	}

	issue.StatusResidency = CalculateResidencyFromEvents(events, issue.Created, issue.ResolutionDate, issue.Status, finishedStatuses, lastMoveDate)

	return issue
}

// CalculateResidencyFromEvents computes residency times from an event stream.
func CalculateResidencyFromEvents(events []IssueEvent, created time.Time, resolved *time.Time, currentStatus string, finished map[string]bool, lastMove int64) map[string]int64 {
	residency := make(map[string]int64)
	if len(events) == 0 {
		return residency
	}

	var transitions []IssueEvent
	for _, e := range events {
		if e.EventType == Transitioned || e.EventType == Created {
			if lastMove == 0 || e.Timestamp >= lastMove {
				transitions = append(transitions, e)
			}
		}
	}

	if len(transitions) == 0 {
		return residency
	}

	for i := 0; i < len(transitions)-1; i++ {
		curr := transitions[i]
		next := transitions[i+1]
		duration := (next.Timestamp - curr.Timestamp) / 1000000 // microseconds to seconds
		if duration <= 0 {
			duration = 1
		}
		residency[curr.ToStatus] += duration
	}

	last := transitions[len(transitions)-1]
	finalMicros := time.Now().UnixMicro()
	if resolved != nil {
		finalMicros = resolved.UnixMicro()
	} else if finished[currentStatus] {
		finalMicros = last.Timestamp
	}
	duration := (finalMicros - last.Timestamp) / 1000000
	if duration <= 0 {
		duration = 1
	}
	residency[last.ToStatus] += duration

	return residency
}
