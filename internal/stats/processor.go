package stats

import (
	"mcs-mcp/internal/jira"
	"sort"
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
func CalculateResidency(transitions []jira.StatusTransition, created time.Time, resolved *time.Time, currentStatus string, finishedStatuses map[string]bool, initialStatus string, referenceDate time.Time) map[string]int64 {
	residency := make(map[string]int64)

	now := time.Now()
	if !referenceDate.IsZero() {
		now = referenceDate
	}

	if len(transitions) == 0 {
		var finalDate time.Time
		if resolved != nil {
			finalDate = *resolved
		} else if finishedStatuses != nil && finishedStatuses[currentStatus] {
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
	} else if finishedStatuses != nil && finishedStatuses[currentStatus] {
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

// GetDailyThroughput returns count of items resolved each day.
func GetDailyThroughput(issues []jira.Issue) []int {
	if len(issues) == 0 {
		return nil
	}

	minDate := time.Now()
	maxDate := time.Now()

	resolvedItems := make([]jira.Issue, 0)
	for _, issue := range issues {
		if issue.ResolutionDate != nil {
			resolvedItems = append(resolvedItems, issue)
			if issue.ResolutionDate.Before(minDate) {
				minDate = *issue.ResolutionDate
			}
		}
	}

	if len(resolvedItems) == 0 {
		return nil
	}

	days := int(maxDate.Sub(minDate).Hours()/24) + 1
	daily := make([]int, days)

	for _, issue := range resolvedItems {
		d := int(issue.ResolutionDate.Sub(minDate).Hours() / 24)
		if d >= 0 && d < days {
			daily[d]++
		}
	}
	return daily
}
