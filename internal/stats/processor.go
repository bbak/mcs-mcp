package stats

import (
	"mcs-mcp/internal/jira"
	"sort"
	"strings"
	"time"
)

// Dataset binds a SourceContext with its fetched and processed issues.
type Dataset struct {
	Context   jira.SourceContext `json:"context"`
	Issues    []jira.Issue       `json:"issues"`
	FetchedAt time.Time          `json:"fetchedAt"`
}

// MapIssue transforms a Jira DTO into a Domain Issue and calculates residency.
func MapIssue(item jira.IssueDTO, finishedStatuses map[string]bool) jira.Issue {
	issue := jira.Issue{
		Key:             item.Key,
		IssueType:       item.Fields.IssueType.Name,
		Status:          item.Fields.Status.Name,
		StatusCategory:  item.Fields.Status.StatusCategory.Key,
		Resolution:      item.Fields.Resolution.Name,
		StatusResidency: make(map[string]int64),
		IsSubtask:       item.Fields.IssueType.Subtask,
	}

	issue.ProjectKey = ExtractProjectKey(item.Key)

	if t, err := jira.ParseTime(item.Fields.Created); err == nil {
		issue.Created = t
	}

	if item.Fields.ResolutionDate != "" {
		if t, err := jira.ParseTime(item.Fields.ResolutionDate); err == nil {
			issue.ResolutionDate = &t
		}
	}

	if t, err := jira.ParseTime(item.Fields.Updated); err == nil {
		issue.Updated = t
	}

	if item.Changelog != nil {
		issue.Transitions, issue.StatusResidency, issue.IsMoved = ProcessChangelog(item.Changelog, issue.Created, issue.ResolutionDate, issue.Status, finishedStatuses)
	}

	return issue
}

// ExtractProjectKey gets the prefix from a Jira key.
func ExtractProjectKey(key string) string {
	for i := 0; i < len(key); i++ {
		if key[i] == '-' {
			return key[:i]
		}
	}
	return ""
}

// ProcessChangelog calculates residency times and transitions from a Jira changelog.
func ProcessChangelog(changelog *jira.ChangelogDTO, created time.Time, resolved *time.Time, currentStatus string, finishedStatuses map[string]bool) ([]jira.StatusTransition, map[string]int64, bool) {
	type fullTransition struct {
		From string
		To   string
		Date time.Time
	}
	var allTrans []fullTransition
	var transitions []jira.StatusTransition

	var lastMoveDate *time.Time
	var entryStatus string

	// Pass 1: Find context
	for _, h := range changelog.Histories {
		hDate, dateErr := jira.ParseTime(h.Created)
		if dateErr != nil {
			continue
		}

		isMove := false
		var statusChange *jira.ItemDTO
		for _, itm := range h.Items {
			if itm.Field == "Key" || itm.Field == "project" {
				isMove = true
			}
			if itm.Field == "status" {
				statusChange = &itm
			}
		}

		if isMove {
			lastMoveDate = &hDate
			if statusChange != nil {
				entryStatus = statusChange.ToString
			}
		} else if lastMoveDate != nil && entryStatus == "" && statusChange != nil {
			entryStatus = statusChange.FromString
		}
	}

	// Pass 2: Process
	for _, h := range changelog.Histories {
		hDate, dateErr := jira.ParseTime(h.Created)
		if dateErr != nil {
			continue
		}

		if lastMoveDate != nil && hDate.Before(*lastMoveDate) {
			continue
		}

		for _, itm := range h.Items {
			if itm.Field == "status" {
				allTrans = append(allTrans, fullTransition{
					From: itm.FromString,
					To:   itm.ToString,
					Date: hDate,
				})

				transitions = append(transitions, jira.StatusTransition{
					FromStatus: itm.FromString,
					ToStatus:   itm.ToString,
					Date:       hDate,
				})
			}
		}
	}

	sort.Slice(transitions, func(a, b int) bool {
		return transitions[a].Date.Before(transitions[b].Date)
	})

	residency := make(map[string]int64)
	if len(allTrans) > 0 {
		initialStatus := entryStatus
		if initialStatus == "" {
			initialStatus = allTrans[0].From
			if initialStatus == "" {
				initialStatus = allTrans[0].To
			}
		}

		anchorDate := created
		// If moved, the 'Synthetic Birth' happens at 'created' but uses 'initialStatus'
		// which is the first project-valid status.
		firstDuration := int64(allTrans[0].Date.Sub(anchorDate).Seconds())
		if firstDuration <= 0 {
			firstDuration = 1
		}
		residency[initialStatus] += firstDuration

		for j := 0; j < len(allTrans)-1; j++ {
			duration := int64(allTrans[j+1].Date.Sub(allTrans[j].Date).Seconds())
			if duration <= 0 {
				duration = 1
			}
			residency[allTrans[j].To] += duration
		}

		var finalDate time.Time
		if resolved != nil {
			finalDate = *resolved
		} else if finishedStatuses[currentStatus] {
			finalDate = allTrans[len(allTrans)-1].Date
		} else {
			finalDate = time.Now()
		}

		lastTrans := allTrans[len(allTrans)-1]
		finalDuration := int64(finalDate.Sub(lastTrans.Date).Seconds())
		if finalDuration <= 0 {
			finalDuration = 1
		}
		residency[lastTrans.To] += finalDuration
	} else if resolved != nil {
		duration := int64(resolved.Sub(created).Seconds())
		if duration <= 0 {
			duration = 1
		}
		residency[currentStatus] = duration
	} else {
		finalDate := time.Now()
		if finishedStatuses[currentStatus] {
			finalDate = created
		}
		duration := int64(finalDate.Sub(created).Seconds())
		if duration <= 0 {
			duration = 1
		}
		residency[currentStatus] = duration
	}

	return transitions, residency, lastMoveDate != nil
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
		newIssue.StatusResidency = CalculateResidency(
			newIssue.Transitions,
			issue.Created,
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

// CalculateResidency provides a unified way to compute status durations in seconds.
// If referenceDate is non-zero, it is used as the "Now" for open items (Time-Travel).
func CalculateResidency(transitions []jira.StatusTransition, created time.Time, resolved *time.Time, currentStatus string, finished map[string]bool, initialStatus string, referenceDate time.Time) map[string]int64 {
	residency := make(map[string]int64)

	now := time.Now()
	if !referenceDate.IsZero() {
		now = referenceDate
	}

	isFinished := func(status string) bool {
		if finished == nil {
			return false
		}
		if val, ok := finished[status]; ok {
			return val
		}
		lower := strings.ToLower(status)
		for k, v := range finished {
			if strings.ToLower(k) == lower {
				return v
			}
		}
		return false
	}

	if len(transitions) == 0 {
		var finalDate time.Time
		if resolved != nil {
			finalDate = *resolved
		} else if isFinished(currentStatus) {
			finalDate = created
		} else {
			finalDate = now
		}
		duration := int64(finalDate.Sub(created).Seconds())
		if duration <= 0 {
			duration = 1
		}
		residency[currentStatus] = duration
		return residency
	}

	// 1. Time from creation to first transition
	if initialStatus == "" {
		initialStatus = transitions[0].FromStatus
	}
	firstDuration := int64(transitions[0].Date.Sub(created).Seconds())
	if firstDuration <= 0 {
		firstDuration = 1
	}
	residency[initialStatus] = firstDuration

	// 2. Time between transitions
	for i := 0; i < len(transitions)-1; i++ {
		duration := int64(transitions[i+1].Date.Sub(transitions[i].Date).Seconds())
		if duration <= 0 {
			duration = 1
		}
		residency[transitions[i].ToStatus] += duration
	}

	// 3. Time since last transition
	var finalDate time.Time
	if resolved != nil {
		finalDate = *resolved
	} else if isFinished(currentStatus) {
		finalDate = transitions[len(transitions)-1].Date
	} else {
		finalDate = now
	}

	lastTrans := transitions[len(transitions)-1]
	finalDuration := int64(finalDate.Sub(lastTrans.Date).Seconds())
	if finalDuration <= 0 {
		finalDuration = 1
	}
	residency[lastTrans.ToStatus] += finalDuration

	return residency
}

// IsDelivered determines if an issue reached a successful final state following the precedence:
// 1. Resolution Mapping (Primary)
// 2. Status Mapping (Fallback)
//
// GOLD STANDARD: This logic is verified against Nave (Flow Portfolio) benchmarks.
// DO NOT MODIFY WITHOUT EXPLICIT RE-VERIFICATION.
func IsDelivered(issue jira.Issue, resolutions map[string]string, mappings map[string]StatusMetadata) bool {
	// If no resolution date and not in a terminal status, it's definitely not delivered
	isTerminal := false
	if m, ok := GetMetadataRobust(mappings, issue.StatusID, issue.Status); ok && m.Tier == "Finished" {
		isTerminal = true
	}

	if issue.ResolutionDate == nil && !isTerminal {
		return false
	}

	// 1. Resolution Mapping Precedence
	if resolutions != nil {
		if outcome, ok := resolutions[issue.Resolution]; ok {
			if outcome != "" {
				return outcome == "delivered"
			}
		}
	}

	// 2. Status Mapping Fallback
	if m, ok := GetMetadataRobust(mappings, issue.StatusID, issue.Status); ok && m.Tier == "Finished" {
		if m.Outcome != "" {
			return m.Outcome == "delivered"
		}
	}

	// 3. Absolute Fallback: if it has a resolution date, we assume delivered
	// unless specifically mapped as abandoned above.
	return issue.ResolutionDate != nil
}

// FilterDelivered returns only items that passed the IsDelivered check.
func FilterDelivered(issues []jira.Issue, resolutions map[string]string, mappings map[string]StatusMetadata) []jira.Issue {
	filtered := make([]jira.Issue, 0)
	for _, iss := range issues {
		if IsDelivered(iss, resolutions, mappings) {
			filtered = append(filtered, iss)
		}
	}
	return filtered
}

func GetDailyThroughput(issues []jira.Issue, window AnalysisWindow, resolutionMappings map[string]string, statusMappings map[string]StatusMetadata) []int {
	if len(issues) == 0 {
		return nil
	}

	windowDays := window.DayCount()
	daily := make([]int, windowDays)

	for _, issue := range issues {
		if !IsDelivered(issue, resolutionMappings, statusMappings) {
			continue
		}

		resDate := *issue.ResolutionDate
		// If resolution date is missing but IsDelivered is true (terminal status fallback), use Updated
		if issue.ResolutionDate == nil {
			resDate = issue.Updated
		}

		// Normalize to start of day for indexing
		d := int(resDate.Sub(window.Start).Hours() / 24)
		if d >= 0 && d < windowDays {
			daily[d]++
		}
	}
	return daily
}
