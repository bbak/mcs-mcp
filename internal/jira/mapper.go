package jira

import (
	"slices"
	"strings"
	"time"
)

// StatusSegment represents a contiguous period in a specific status.
type StatusSegment struct {
	Status string
	Start  time.Time
	End    time.Time
}

// MapIssue transforms a Jira DTO into a Domain Issue and calculates residency.
func MapIssue(item IssueDTO, finishedStatuses map[string]bool) Issue {
	issue := Issue{
		Key:             item.Key,
		IssueType:       item.Fields.IssueType.Name,
		Status:          item.Fields.Status.Name,
		StatusID:        item.Fields.Status.ID,
		StatusCategory:  item.Fields.Status.StatusCategory.Key,
		Resolution:      item.Fields.Resolution.Name,
		StatusResidency: make(map[string]int64),
		IsSubtask:       item.Fields.IssueType.Subtask,
	}

	for i := 0; i < len(issue.Key); i++ {
		if issue.Key[i] == '-' {
			issue.ProjectKey = issue.Key[:i]
			break
		}
	}

	if t, err := ParseTime(item.Fields.Created); err == nil {
		issue.Created = t
	}

	if item.Fields.ResolutionDate != "" {
		if t, err := ParseTime(item.Fields.ResolutionDate); err == nil {
			issue.ResolutionDate = &t
		}
	}

	if t, err := ParseTime(item.Fields.Updated); err == nil {
		issue.Updated = t
	}

	if item.Changelog != nil {
		issue.Transitions, issue.StatusResidency, issue.IsMoved = ProcessChangelog(item.Changelog, issue.Created, issue.ResolutionDate, issue.Status, finishedStatuses)
	}

	// Dynamic Fallback: if in a finished status but no resolution date provided by Jira
	if issue.ResolutionDate == nil && finishedStatuses != nil && finishedStatuses[issue.Status] {
		issue.ResolutionDate = &issue.Updated
	}

	// Absolute Fallback for residency if no transitions found
	if len(issue.Transitions) == 0 {
		issue.StatusResidency, _ = CalculateResidency(nil, issue.Created, issue.ResolutionDate, issue.Status, issue.StatusID, finishedStatuses, "", "", time.Time{})
	}

	return issue
}

// ProcessChangelog calculates residency times and transitions from a Jira changelog.
func ProcessChangelog(changelog *ChangelogDTO, created time.Time, resolved *time.Time, currentStatus string, finishedStatuses map[string]bool) ([]StatusTransition, map[string]int64, bool) {
	var transitions []StatusTransition
	var lastMoveDate *time.Time
	var entryStatus string

	// Pass 1: Find context
	for _, h := range changelog.Histories {
		hDate, dateErr := ParseTime(h.Created)
		if dateErr != nil {
			continue
		}

		isMove := false
		var statusChange *ItemDTO
		for _, itm := range h.Items {
			itm := itm // capture loop var
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
		hDate, dateErr := ParseTime(h.Created)
		if dateErr != nil {
			continue
		}

		if lastMoveDate != nil && hDate.Before(*lastMoveDate) {
			continue
		}

		for _, itm := range h.Items {
			if itm.Field == "status" {
				transitions = append(transitions, StatusTransition{
					FromStatus:   itm.FromString,
					FromStatusID: itm.From,
					ToStatus:     itm.ToString,
					ToStatusID:   itm.To,
					Date:         hDate,
				})
			}
		}
	}

	slices.SortFunc(transitions, func(a, b StatusTransition) int {
		return a.Date.Compare(b.Date)
	})

	initialStatus := entryStatus
	if initialStatus == "" && len(transitions) > 0 {
		initialStatus = transitions[0].FromStatus
	}

	residency, _ := CalculateResidency(transitions, created, resolved, currentStatus, "", finishedStatuses, initialStatus, "", time.Time{})

	return transitions, residency, lastMoveDate != nil
}

// CalculateResidency provides a unified way to compute status durations in seconds.
// If referenceDate is non-zero, it is used as the "Now" for open items (Time-Travel).
func CalculateResidency(transitions []StatusTransition, created time.Time, resolved *time.Time, currentStatus, currentStatusID string, finished map[string]bool, initialStatus, initialStatusID string, referenceDate time.Time) (map[string]int64, []StatusSegment) {
	residency := make(map[string]int64)
	var segments []StatusSegment

	now := time.Now()
	if !referenceDate.IsZero() {
		now = referenceDate
	}

	isFinished := func(status, id string) bool {
		if finished == nil {
			return false
		}
		if id != "" {
			if val, ok := finished[id]; ok {
				return val
			}
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
		} else if isFinished(currentStatus, "") { // fallback for simplified call
			finalDate = created
		} else {
			finalDate = now
		}
		duration := int64(finalDate.Sub(created).Seconds())
		if duration <= 0 {
			duration = 1
		}
		key := currentStatusID
		if key == "" {
			key = currentStatus
		}
		residency[key] = duration
		segments = append(segments, StatusSegment{
			Status: currentStatus,
			Start:  created,
			End:    finalDate,
		})
		return residency, segments
	}

	// 1. Time from creation to first transition
	if initialStatus == "" {
		initialStatus = transitions[0].FromStatus
	}
	firstDuration := int64(transitions[0].Date.Sub(created).Seconds())
	if firstDuration <= 0 {
		firstDuration = 1
	}
	key := initialStatusID
	if key == "" {
		key = initialStatus
	}
	residency[key] = firstDuration
	segments = append(segments, StatusSegment{
		Status: initialStatus,
		Start:  created,
		End:    transitions[0].Date,
	})

	// 2. Time between transitions
	for i := 0; i < len(transitions)-1; i++ {
		duration := int64(transitions[i+1].Date.Sub(transitions[i].Date).Seconds())
		if duration <= 0 {
			duration = 1
		}
		key := transitions[i].ToStatusID
		if key == "" {
			key = transitions[i].ToStatus
		}
		residency[key] += duration
		segments = append(segments, StatusSegment{
			Status: transitions[i].ToStatus,
			Start:  transitions[i].Date,
			End:    transitions[i+1].Date,
		})
	}

	// 3. Time since last transition
	var finalDate time.Time
	if resolved != nil {
		finalDate = *resolved
	} else if isFinished(currentStatus, currentStatusID) {
		finalDate = transitions[len(transitions)-1].Date
	} else {
		finalDate = now
	}

	lastTrans := transitions[len(transitions)-1]
	finalDuration := int64(finalDate.Sub(lastTrans.Date).Seconds())
	if finalDuration <= 0 {
		finalDuration = 1
	}
	resKey := lastTrans.ToStatusID
	if resKey == "" {
		resKey = lastTrans.ToStatus
	}
	residency[resKey] += finalDuration
	segments = append(segments, StatusSegment{
		Status: lastTrans.ToStatus,
		Start:  lastTrans.Date,
		End:    finalDate,
	})

	return residency, segments
}
