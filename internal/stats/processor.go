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

	for _, h := range changelog.Histories {
		hDate, dateErr := jira.ParseTime(h.Created)
		if dateErr != nil {
			continue
		}

		for _, itm := range h.Items {
			switch itm.Field {
			case "status":
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

			case "Key", "project":
				if lastMoveDate == nil || hDate.After(*lastMoveDate) {
					lastMoveDate = &hDate
				}
			}
		}
	}

	sort.Slice(transitions, func(a, b int) bool {
		return transitions[a].Date.Before(transitions[b].Date)
	})

	if lastMoveDate != nil {
		newAllTrans := []fullTransition{}
		for _, t := range allTrans {
			if !t.Date.Before(*lastMoveDate) {
				newAllTrans = append(newAllTrans, t)
			}
		}
		allTrans = newAllTrans

		newTransitions := []jira.StatusTransition{}
		for _, t := range transitions {
			if !t.Date.Before(*lastMoveDate) {
				newTransitions = append(newTransitions, t)
			}
		}
		transitions = newTransitions
	}

	residency := make(map[string]int64)
	if len(allTrans) > 0 {
		initialStatus := allTrans[0].From
		if initialStatus == "" {
			initialStatus = allTrans[0].To
		}
		anchorDate := created
		if lastMoveDate != nil {
			anchorDate = *lastMoveDate
		}
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
		anchorDate := created
		if lastMoveDate != nil {
			anchorDate = *lastMoveDate
		}
		finalDate := time.Now()
		if finishedStatuses[currentStatus] {
			finalDate = anchorDate
		}
		duration := int64(finalDate.Sub(anchorDate).Seconds())
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

// GetDailyThroughput returns count of items resolved each day in the requested window.
// It never counts items resolved earlier than the specified cutoff date.
func GetDailyThroughput(issues []jira.Issue, windowDays int, mappings map[string]StatusMetadata, resolutionMappings map[string]string, deliveredOnly bool, cutoff time.Time) []int {
	if len(issues) == 0 {
		return nil
	}

	now := time.Now()
	// Normalize to start of today
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	minDate := today.AddDate(0, 0, -windowDays+1)

	// If a cutoff is provided, we respect the LATEST of minDate and cutoff
	effectiveMinDate := minDate
	if !cutoff.IsZero() && cutoff.After(effectiveMinDate) {
		effectiveMinDate = cutoff
	}

	daily := make([]int, windowDays)

	for _, issue := range issues {
		var resDate time.Time
		isDelivered := true

		if issue.ResolutionDate != nil {
			resDate = *issue.ResolutionDate
			if deliveredOnly {
				// Primary: Resolution Mapping
				if outcome, ok := resolutionMappings[issue.Resolution]; ok {
					if outcome != "delivered" {
						isDelivered = false
					}
				} else {
					// Fallback: If no resolution mapping, we check the status mapping
					if m, ok := GetMetadataRobust(mappings, issue.StatusID, issue.Status); ok && m.Tier == "Finished" {
						if m.Outcome != "" && m.Outcome != "delivered" {
							isDelivered = false
						}
					}
					// If neither mapping specifies outcome, we trust the ResolutionDate presence (legacy/default)
				}
			}
		} else if m, ok := GetMetadataRobust(mappings, issue.StatusID, issue.Status); ok && m.Tier == "Finished" {
			// Fallback Finished logic
			resDate = issue.Updated
			if deliveredOnly && m.Outcome != "" && m.Outcome != "delivered" {
				isDelivered = false
			}
		}

		if resDate.IsZero() || !isDelivered {
			continue
		}

		d := int(resDate.Sub(minDate).Hours() / 24)
		if d >= 0 && d < windowDays {
			daily[d]++
		}
	}
	return daily
}
