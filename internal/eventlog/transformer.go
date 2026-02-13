package eventlog

import (
	"mcs-mcp/internal/jira"
	"sort"
	"strings"
)

// TransformIssue converts a Jira Issue DTO and its changelog into a slice of IssueEvents.
func TransformIssue(dto jira.IssueDTO) []IssueEvent {
	var events []IssueEvent
	issueKey := dto.Key
	issueType := dto.Fields.IssueType.Name

	// 1. Identify Initial Status and ID
	initialStatus := dto.Fields.Status.Name
	initialStatusID := dto.Fields.Status.ID
	if dto.Changelog != nil {
		var earliestTS int64
		for _, h := range dto.Changelog.Histories {
			tsObj, err := jira.ParseTime(h.Created)
			if err != nil {
				continue
			}
			ts := tsObj.UnixMicro()
			for _, item := range h.Items {
				if strings.EqualFold(item.Field, "status") {
					if earliestTS == 0 || ts < earliestTS {
						earliestTS = ts
						initialStatus = item.FromString
						initialStatusID = item.From
						if initialStatus == "" {
							initialStatus = item.ToString
							initialStatusID = item.To
						}
					}
				}
			}
		}
	}

	// 2. Created Event
	createdTime, err := jira.ParseTime(dto.Fields.Created)
	if err == nil {
		events = append(events, IssueEvent{
			IssueKey:   issueKey,
			IssueType:  issueType,
			EventType:  Created,
			Timestamp:  createdTime.UnixMicro(),
			ToStatus:   initialStatus,
			ToStatusID: initialStatusID,
		})
	}

	// 3. Changelog Transitions
	var lastMoveTS int64
	var entryStatus string
	var entryStatusID string
	var isDifferentWorkflow bool

	// Infer target project key from current issue key (e.g., "PROJ" from "PROJ-123")
	targetProjectKey := issueKey
	if idx := strings.Index(issueKey, "-"); idx > 0 {
		targetProjectKey = issueKey[:idx]
	}

	if dto.Changelog != nil {
		// Pass 0: Ensure histories are chronological (ascending)
		// Jira API often returns them descending, which breaks forward-scans.
		sort.Slice(dto.Changelog.Histories, func(i, j int) bool {
			t1, _ := jira.ParseTime(dto.Changelog.Histories[i].Created)
			t2, _ := jira.ParseTime(dto.Changelog.Histories[j].Created)
			return t1.Before(t2)
		})

		// Pass 1: Detective Work - Find moves and arrival context
		for _, h := range dto.Changelog.Histories {
			if tsObj, err := jira.ParseTime(h.Created); err == nil {
				ts := tsObj.UnixMicro()

				isMove := false
				wfChanged := false
				var statusItem *jira.ItemDTO

				for i := range h.Items {
					item := &h.Items[i]
					f := item.Field
					if strings.EqualFold(f, "Key") {
						// Only treat as a relevant move if it enters our target project
						if strings.HasPrefix(item.To, targetProjectKey+"-") {
							isMove = true
						}
					} else if strings.EqualFold(f, "project") {
						// Or if the project name matches (Jira sometimes uses names/IDs here)
						if strings.EqualFold(item.To, targetProjectKey) {
							isMove = true
						}
					}
					if strings.EqualFold(f, "workflow") && !strings.EqualFold(item.FromString, item.ToString) {
						wfChanged = true
					}
					if strings.EqualFold(f, "status") {
						statusItem = item
					}
				}

				if isMove {
					lastMoveTS = ts
					entryStatus = ""
					entryStatusID = ""
					isDifferentWorkflow = wfChanged

					if statusItem != nil {
						entryStatus = statusItem.ToString
						entryStatusID = statusItem.To
					}
				} else if lastMoveTS != 0 && entryStatus == "" && statusItem != nil {
					entryStatus = statusItem.FromString
					entryStatusID = statusItem.From
				}
			}
		}

		// Pass 2: Emission
		for _, history := range dto.Changelog.Histories {
			tsObj, err := jira.ParseTime(history.Created)
			if err != nil {
				continue
			}
			ts := tsObj.UnixMicro()

			// Case 2 Healing: Dropping history before move if workflow changed
			if lastMoveTS != 0 && isDifferentWorkflow && ts < lastMoveTS {
				continue
			}

			event := IssueEvent{
				IssueKey:  issueKey,
				IssueType: issueType,
				EventType: Change,
				Timestamp: ts,
			}

			hasSignal := false
			for _, item := range history.Items {
				if strings.EqualFold(item.Field, "status") {
					event.FromStatus = item.FromString
					event.FromStatusID = item.From
					event.ToStatus = item.ToString
					event.ToStatusID = item.To
					hasSignal = true
				} else if strings.EqualFold(item.Field, "resolution") {
					if item.ToString != "" {
						event.Resolution = item.ToString
						event.IsUnresolved = false
					} else {
						event.IsUnresolved = true
						event.Resolution = ""
					}
					hasSignal = true
				}
			}

			if hasSignal {
				events = append(events, event)
			}
		}
	}

	// 4. Final Healing - Apply Synthetic Birth if moved
	if lastMoveTS != 0 {
		originalBirth := events[0].Timestamp // The 'Created' event added at Step 2

		// Fallback for Entry Status
		if entryStatus == "" && len(events) > 1 {
			for _, e := range events {
				if e.EventType == Change && e.ToStatus != "" {
					entryStatus = e.FromStatus
					entryStatusID = e.FromStatusID
					break
				}
			}
		}
		if entryStatus == "" {
			entryStatus = initialStatus
			entryStatusID = initialStatusID
		}

		if isDifferentWorkflow {
			// Case 2: Anchor synthetic birth at original TS but in Arrival Status
			events[0] = IssueEvent{
				IssueKey:   issueKey,
				IssueType:  issueType,
				EventType:  Created,
				Timestamp:  originalBirth,
				ToStatus:   entryStatus,
				ToStatusID: entryStatusID,
				IsHealed:   true,
			}
		} else {
			// Case 1: Same workflow, just ensure birth points to entryStatus
			events[0].ToStatus = entryStatus
			events[0].ToStatusID = entryStatusID
			events[0].IsHealed = true
		}
	} else {
		// Normal path: ensure first event has the initial biological status
		if entryStatus == "" {
			events[0].ToStatus = initialStatus
			events[0].ToStatusID = initialStatusID
		} else {
			events[0].ToStatus = entryStatus
			events[0].ToStatusID = entryStatusID
		}
	}

	// 4. Resolution Event (Snapshot fallback)
	if dto.Fields.ResolutionDate != "" {
		resTime, err := jira.ParseTime(dto.Fields.ResolutionDate)
		if err == nil {
			ts := resTime.UnixMicro()
			resName := dto.Fields.Resolution.Name

			// De-duplication check:
			// If we already have a Change event for this resolution within a 2s grace period, skip fallback.
			duplicate := false
			const gracePeriod = 2000000 // 2 seconds in microseconds

			for _, e := range events {
				if e.Resolution == resName {
					diff := ts - e.Timestamp
					if diff < 0 {
						diff = -diff
					}
					if diff <= gracePeriod {
						duplicate = true
						break
					}
				}
			}

			if !duplicate {
				events = append(events, IssueEvent{
					IssueKey:   issueKey,
					IssueType:  issueType,
					EventType:  Change,
					Timestamp:  ts,
					Resolution: resName,
				})
			}
		}
	}

	return events
}
