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

	// Extract Project Key (e.g., PROJ-123 -> PROJ)
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
		issue.Transitions, issue.StatusResidency, issue.StartedDate = ProcessChangelog(item.Changelog, issue.Created, issue.ResolutionDate, issue.Status, finishedStatuses)
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
func ProcessChangelog(changelog *jira.ChangelogDTO, created time.Time, resolved *time.Time, currentStatus string, finishedStatuses map[string]bool) ([]jira.StatusTransition, map[string]int64, *time.Time) {
	var earliest *time.Time
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

				if earliest == nil || hDate.Before(*earliest) {
					earliest = &hDate
				}
			case "Key", "project":
				// Project Move Detection: treat the latest move as the boundary for process analysis
				if lastMoveDate == nil || hDate.After(*lastMoveDate) {
					lastMoveDate = &hDate
				}
			}
		}
	}

	sort.Slice(transitions, func(a, b int) bool {
		return transitions[a].Date.Before(transitions[b].Date)
	})

	// Apply Project Move Boundary: We discard residency data from previous projects
	// but KEEP the 'created' date of the issue for high-level Lead Time (Total Age) stats.
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
		// 1. Initial Residency (from creation/move to first transition)
		initialStatus := allTrans[0].From
		if initialStatus == "" {
			initialStatus = "Created"
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

		// 2. Intermediate Residencies
		for j := 0; j < len(allTrans)-1; j++ {
			duration := int64(allTrans[j+1].Date.Sub(allTrans[j].Date).Seconds())
			if duration <= 0 {
				duration = 1
			}
			residency[allTrans[j].To] += duration
		}

		// 3. Current/Final Residency
		var finalDate time.Time
		if resolved != nil {
			finalDate = *resolved
		} else if finishedStatuses[currentStatus] {
			// Stop the Clock: if in Finished tier, use the date of the most recent transition into it
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
		// If no transitions, we assume it stayed in the initial state (Created) until resolution
		residency["Created"] = duration
	} else {
		// No transitions and not resolved: residency in current status since creation/move
		anchorDate := created
		if lastMoveDate != nil {
			anchorDate = *lastMoveDate
		}
		finalDate := time.Now()
		if finishedStatuses[currentStatus] {
			// If created directly in a Finished status, the clock stops at creation
			finalDate = anchorDate
		}
		duration := int64(finalDate.Sub(anchorDate).Seconds())
		if duration <= 0 {
			duration = 1
		}
		residency[currentStatus] = duration
	}

	return transitions, residency, earliest
}
