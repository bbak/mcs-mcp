package stats

import (
	"mcs-mcp/internal/eventlog"
	"mcs-mcp/internal/jira"
	"sort"
	"strings"
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
		issue := MapIssueFromEvents(issueEvents, finishedMap, window.End)
		if issue.IsSubtask {
			continue
		}

		// 1. Was it resolved WITHIN the window?
		isResolved := false
		var resDate time.Time

		if issue.ResolutionDate != nil {
			isResolved = true
			resDate = *issue.ResolutionDate
		} else if m, ok := GetMetadataRobust(mappings, issue.StatusID, issue.Status); ok && m.Tier == "Finished" {
			isResolved = true
			resDate = issue.Updated
		}

		if isResolved {
			if !resDate.Before(window.Start) && !resDate.After(window.End) {
				finished = append(finished, issue)
			}
			continue
		}

		// 2. Classify by Tier at the end of the window
		if m, ok := GetMetadataRobust(mappings, issue.StatusID, issue.Status); ok {
			switch m.Tier {
			case "Downstream":
				downstream = append(downstream, issue)
			case "Upstream":
				upstream = append(upstream, issue)
			case "Demand":
				demand = append(demand, issue)
			default:
				// Fallback or items without tiers
				demand = append(demand, issue)
			}
		} else {
			// Fallback: Default to Demand if mapping is missing
			demand = append(demand, issue)
		}
	}

	// Chronological Sort: All analytical consumers expect deterministic time-ordered data.
	sortByDate := func(issues []jira.Issue) {
		sort.Slice(issues, func(i, j int) bool {
			dateI := issues[i].Updated
			if issues[i].ResolutionDate != nil {
				dateI = *issues[i].ResolutionDate
			}
			dateJ := issues[j].Updated
			if issues[j].ResolutionDate != nil {
				dateJ = *issues[j].ResolutionDate
			}
			return dateI.Before(dateJ)
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
	sort.Slice(sortedKeys, func(i, j int) bool {
		return sortedKeys[i].ts > sortedKeys[j].ts
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
		issue := MapIssueFromEvents(issueEvents, nil, time.Time{})
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
			if e.Resolution != "" {
				nowDelivered = true
			} else if e.ToStatus != "" {
				if m, ok := mappings[e.ToStatus]; ok && (m.Tier == "Finished" || m.Role == "terminal") && m.Outcome == "delivered" {
					nowDelivered = true
				}
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

// MapIssueFromEvents builds a Domain Issue from a chronological stream of events.
// If referenceDate is non-zero, it is used as the "Now" for open items.
func MapIssueFromEvents(events []eventlog.IssueEvent, finishedStatuses map[string]bool, referenceDate time.Time) jira.Issue {
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
	issue.ProjectKey = ExtractProjectKey(first.IssueKey)

	// simplified loop: events are now packed atomic change-sets
	for _, e := range events {
		issue.Updated = time.UnixMicro(e.Timestamp)

		// Signal-Aware application
		if e.EventType == eventlog.Created || e.ToStatus != "" {
			if e.EventType == eventlog.Created {
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
		} else if e.IsUnresolved {
			issue.ResolutionDate = nil
			issue.Resolution = ""
		}

		if e.EventType == eventlog.Flagged {
			issue.Flagged = e.Flagged
		}

		if e.EventType == eventlog.Created {
			issue.Flagged = e.Flagged
		}

		if e.EventType == eventlog.Created && e.IsHealed {
			issue.IsMoved = true
		}

		// Final evaluation after applying all signals in this event
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
			} else if issue.ResolutionDate == nil {
				// Synthetic Fallback
				var streakStart int64
				for i := len(events) - 1; i >= 0; i-- {
					evt := events[i]
					isPreviousFin := finishedStatuses[evt.ToStatus] || (evt.ToStatusID != "" && finishedStatuses[evt.ToStatusID])
					if !isPreviousFin {
						lowerS := strings.ToLower(evt.ToStatus)
						for name, ok := range finishedStatuses {
							if ok && strings.ToLower(name) == lowerS {
								isPreviousFin = true
								break
							}
						}
					}

					if !isPreviousFin {
						break
					}
					streakStart = evt.Timestamp
				}
				if streakStart != 0 {
					resDate := time.UnixMicro(streakStart)
					issue.ResolutionDate = &resDate
				}
			}
		}
	}

	issue.StatusResidency, issue.BlockedResidency = CalculateResidencyFromEvents(events, issue.Created, issue.ResolutionDate, issue.Status, finishedStatuses, referenceDate)

	return issue
}

// CalculateResidencyFromEvents computes residency times from an event stream by converting to domain transitions.
func CalculateResidencyFromEvents(events []eventlog.IssueEvent, created time.Time, resolved *time.Time, currentStatus string, finished map[string]bool, referenceDate time.Time) (map[string]int64, map[string]int64) {
	var transitions []jira.StatusTransition

	// Track the very first "From" status if possible for the birth duration
	var initialStatus string

	for _, e := range events {
		if e.EventType == eventlog.Created || e.ToStatus != "" {
			if initialStatus == "" && e.EventType == eventlog.Created {
				initialStatus = e.ToStatus
			} else if initialStatus == "" && e.ToStatus != "" {
				initialStatus = e.FromStatus
			}

			if e.ToStatus != "" && e.EventType != eventlog.Created {
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

	residency, segments := jira.CalculateResidency(transitions, created, resolved, currentStatus, finished, initialStatus, referenceDate)

	// Friction Mapping: Extract blocked intervals and overlay them
	blockedIntervals := ExtractBlockedIntervals(events, created, resolved, referenceDate)
	blockedResidency := CalculateBlockedResidency(segments, blockedIntervals)

	return residency, blockedResidency
}

// ExtractBlockedIntervals identifies periods where an item was flagged as impeded.
func ExtractBlockedIntervals(events []eventlog.IssueEvent, created time.Time, resolved *time.Time, referenceDate time.Time) []Interval {
	var intervals []Interval
	var currentStart *time.Time

	for _, e := range events {
		if e.Timestamp > referenceDate.UnixMicro() {
			break
		}
		if resolved != nil && e.Timestamp > resolved.UnixMicro() {
			break
		}

		if e.EventType == eventlog.Flagged {
			if e.Flagged != "" && currentStart == nil {
				ts := time.UnixMicro(e.Timestamp)
				currentStart = &ts
			} else if e.Flagged == "" && currentStart != nil {
				intervals = append(intervals, Interval{
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
		intervals = append(intervals, Interval{
			Start: *currentStart,
			End:   end,
		})
	}

	return intervals
}

// GetStratifiedThroughput aggregates resolved items into time buckets, both pooled and stratified by type.
func GetStratifiedThroughput(issues []jira.Issue, window AnalysisWindow, resolutions map[string]string, mappings map[string]StatusMetadata) StratifiedThroughput {
	buckets := window.Subdivide()
	pooled := make([]int, len(buckets))
	byType := make(map[string][]int)

	for _, issue := range issues {
		if !IsDelivered(issue, resolutions, mappings) {
			continue
		}

		if issue.ResolutionDate == nil {
			continue
		}

		idx := window.FindBucketIndex(*issue.ResolutionDate)
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
