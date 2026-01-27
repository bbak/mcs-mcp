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
func MapIssue(item jira.IssueDTO) jira.Issue {
	issue := jira.Issue{
		Key:        item.Key,
		IssueType:  item.Fields.IssueType.Name,
		Status:     item.Fields.Status.Name,
		Resolution: item.Fields.Resolution.Name,
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
		issue.Transitions, issue.StatusResidency, issue.StartedDate = ProcessChangelog(item.Changelog, issue.Created, issue.ResolutionDate)
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
func ProcessChangelog(changelog *jira.ChangelogDTO, created time.Time, resolved *time.Time) ([]jira.StatusTransition, map[string]int64, *time.Time) {
	var earliest *time.Time
	type fullTransition struct {
		From string
		To   string
		Date time.Time
	}
	var allTrans []fullTransition
	var transitions []jira.StatusTransition

	for _, h := range changelog.Histories {
		for _, itm := range h.Items {
			if itm.Field == "status" {
				if t, err := jira.ParseTime(h.Created); err == nil {
					allTrans = append(allTrans, fullTransition{
						From: itm.FromString,
						To:   itm.ToString,
						Date: t,
					})

					transitions = append(transitions, jira.StatusTransition{
						ToStatus: itm.ToString,
						Date:     t,
					})

					if earliest == nil || t.Before(*earliest) {
						st := t
						earliest = &st
					}
				}
			}
		}
	}

	// Sort ASC by date
	sort.Slice(allTrans, func(a, b int) bool {
		return allTrans[a].Date.Before(allTrans[b].Date)
	})
	sort.Slice(transitions, func(a, b int) bool {
		return transitions[a].Date.Before(transitions[b].Date)
	})

	residency := make(map[string]int64)
	if len(allTrans) > 0 {
		// 1. Initial Residency (from creation to first transition)
		initialStatus := allTrans[0].From
		if initialStatus == "" {
			initialStatus = "Created"
		}
		firstDuration := int64(allTrans[0].Date.Sub(created).Seconds())
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
		residency["Created"] = duration
	}

	return transitions, residency, earliest
}
