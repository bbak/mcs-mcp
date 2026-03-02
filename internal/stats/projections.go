package stats

import (
	"cmp"
	"mcs-mcp/internal/eventlog"
	"mcs-mcp/internal/jira"
	"slices"
	"time"
)

// ProjectScope identifies and reconstructs issues relevant to a specific analysis window.
// It returns four sets aligned with meta-workflow tiers:
// 1. Finished: Items that reached their final 'Finished' state WITHIN the window.
// 2. Downstream: Items in active execution tiers at the window's END point.
// 3. Upstream: Items in refinement or analysis tiers at the window's END point.
// 4. Demand: Items existing in the initial entry tier at the window's END point.
func ProjectScope(events []eventlog.IssueEvent, window AnalysisWindow, commitmentPoint string, mappings map[string]StatusMetadata, resolutions map[string]string, issueTypes []string) ([]jira.Issue, []jira.Issue, []jira.Issue, []jira.Issue) {
	typeMap := make(map[string]bool)
	for _, t := range issueTypes {
		typeMap[t] = true
	}

	grouped := make(map[string][]eventlog.IssueEvent)
	for _, e := range events {
		if !window.End.IsZero() && e.Timestamp > window.End.UnixMicro() {
			continue
		}
		if len(issueTypes) > 0 && !typeMap[e.IssueType] {
			continue
		}
		grouped[e.IssueKey] = append(grouped[e.IssueKey], e)
	}

	finishedMap := make(map[string]bool)
	for name, m := range mappings {
		if m.Tier == "Finished" {
			finishedMap[name] = true
		}
	}

	var finished []jira.Issue
	var downstream []jira.Issue
	var upstream []jira.Issue
	var demand []jira.Issue

	for _, issueEvents := range grouped {
		issue := eventlog.ReconstructIssue(issueEvents, window.End)
		if issue.IsSubtask {
			continue
		}

		DetermineOutcome(&issue, resolutions, mappings)

		// 1. Was it resolved WITHIN the window?
		if issue.OutcomeDate != nil {
			if !issue.OutcomeDate.Before(window.Start) && !issue.OutcomeDate.After(window.End) {
				finished = append(finished, issue)
			}
			continue
		}

		// 2. Classify by Tier at the end of the window
		tier := DetermineTier(issue, commitmentPoint, mappings)
		switch tier {
		case "Downstream":
			downstream = append(downstream, issue)
		case "Upstream":
			upstream = append(upstream, issue)
		case "Demand", "Unknown":
			demand = append(demand, issue)
		}
	}

	// Chronological Sort: All analytical consumers expect deterministic time-ordered data.
	sortByDate := func(issues []jira.Issue) {
		slices.SortFunc(issues, func(a, b jira.Issue) int {
			dateA := a.Updated
			if a.OutcomeDate != nil {
				dateA = *a.OutcomeDate
			}
			dateB := b.Updated
			if b.OutcomeDate != nil {
				dateB = *b.OutcomeDate
			}
			return dateA.Compare(dateB)
		})
	}

	sortByDate(finished)
	sortByDate(downstream)
	sortByDate(upstream)
	sortByDate(demand)

	return finished, downstream, upstream, demand
}

// DiscoverDatasetBoundaries performs a lightweight scan of the event log to find
// temporal boundaries and the total number of unique items.
func DiscoverDatasetBoundaries(events []eventlog.IssueEvent) (first, last time.Time, total int) {
	uniqueKeys := make(map[string]bool)
	var minTS, maxTS int64

	for _, e := range events {
		uniqueKeys[e.IssueKey] = true
		if minTS == 0 || e.Timestamp < minTS {
			minTS = e.Timestamp
		}
		if e.Timestamp > maxTS {
			maxTS = e.Timestamp
		}
	}

	if minTS != 0 {
		first = time.UnixMicro(minTS)
	}
	if maxTS != 0 {
		last = time.UnixMicro(maxTS)
	}
	return first, last, len(uniqueKeys)
}

// ProjectNeutralSample selects a recent sample of work items from the event log
// and reconstructs them without applying tier-based filtering.
func ProjectNeutralSample(events []eventlog.IssueEvent, targetSize int) []jira.Issue {
	// 1. Group events by key
	grouped := make(map[string][]eventlog.IssueEvent)
	latestTS := make(map[string]int64)
	for _, e := range events {
		grouped[e.IssueKey] = append(grouped[e.IssueKey], e)
		if e.Timestamp > latestTS[e.IssueKey] {
			latestTS[e.IssueKey] = e.Timestamp
		}
	}

	// 2. Sort keys by latest activity descending
	type keyTS struct {
		key string
		ts  int64
	}
	var sortedKeys []keyTS
	for k, ts := range latestTS {
		sortedKeys = append(sortedKeys, keyTS{k, ts})
	}
	slices.SortFunc(sortedKeys, func(a, b keyTS) int {
		return cmp.Compare(b.ts, a.ts)
	})

	// 3. Take top targetSize and reconstruct
	limit := targetSize
	if len(sortedKeys) < limit {
		limit = len(sortedKeys)
	}

	var sample []jira.Issue
	for i := 0; i < limit; i++ {
		key := sortedKeys[i].key
		issueEvents := grouped[key]
		// Reconstruct without mappings or tier filters
		issue := eventlog.ReconstructIssue(issueEvents, time.Time{})
		if !issue.IsSubtask {
			sample = append(sample, issue)
		}
	}

	return sample
}

// WIPItem represents an active work item derived from the event log.
type WIPItem struct {
	IssueKey           string
	IssueType          string
	CurrentStatus      string
	CommitmentDate     time.Time
	AgeSinceCommitment float64 // Days
}

// BuildWIPProjection identifies active items based on a commitment point and current mappings.
func BuildWIPProjection(events []eventlog.IssueEvent, commitmentPoint string, mappings map[string]StatusMetadata, referenceDate time.Time) []WIPItem {
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
func BuildThroughputProjection(events []eventlog.IssueEvent, mappings map[string]StatusMetadata) []ThroughputBucket {
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
		if e.EventType != eventlog.Created {
			if e.ToStatus != "" || e.ToStatusID != "" {
				// Use PreferID logic inline
				statusKey := e.ToStatusID
				if statusKey == "" {
					statusKey = e.ToStatus
				}

				if m, ok := mappings[statusKey]; ok && (m.Tier == "Finished" || m.Role == "terminal") && m.Outcome == "delivered" {
					nowDelivered = true
				} else if m, ok := mappings[e.ToStatus]; ok && (m.Tier == "Finished" || m.Role == "terminal") && m.Outcome == "delivered" {
					nowDelivered = true
				}
			}

			// Fallback: If it has a resolution but we didn't map the status, count it if it's not explicitly missing
			if !nowDelivered && (e.Resolution != "" || e.ResolutionID != "") {
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

	slices.SortFunc(result, func(a, b ThroughputBucket) int {
		return a.Date.Compare(b.Date)
	})

	return result
}

// GetStratifiedThroughput aggregates resolved items into time buckets, both pooled and stratified by type.
func GetStratifiedThroughput(issues []jira.Issue, window AnalysisWindow) StratifiedThroughput {
	buckets := window.Subdivide()
	pooled := make([]int, len(buckets))
	byType := make(map[string][]int)

	for _, issue := range issues {
		if !IsDelivered(issue) {
			continue
		}

		if issue.OutcomeDate == nil {
			continue
		}

		idx := window.FindBucketIndex(*issue.OutcomeDate)
		if idx < 0 || idx >= len(buckets) {
			continue
		}

		pooled[idx]++
		if _, ok := byType[issue.IssueType]; !ok {
			byType[issue.IssueType] = make([]int, len(buckets))
		}
		byType[issue.IssueType][idx]++
	}

	return StratifiedThroughput{
		Pooled: pooled,
		ByType: byType,
	}
}
