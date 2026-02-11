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

	if dto.Changelog != nil {
		// Pass 0: Ensure histories are chronological (ascending)
		// Jira API often returns them descending, which breaks forward-scans.
		sort.Slice(dto.Changelog.Histories, func(i, j int) bool {
			t1, _ := jira.ParseTime(dto.Changelog.Histories[i].Created)
			t2, _ := jira.ParseTime(dto.Changelog.Histories[j].Created)
			return t1.Before(t2)
		})

		// Pass 1: Detective Work - Find the Entry Context (Arrival Status)
		for _, h := range dto.Changelog.Histories {
			tsObj, err := jira.ParseTime(h.Created)
			if err != nil {
				continue
			}
			ts := tsObj.UnixMicro()

			isMove := false
			var statusItem *jira.ItemDTO

			for i := range h.Items {
				item := &h.Items[i]
				if strings.EqualFold(item.Field, "Key") || strings.EqualFold(item.Field, "project") {
					isMove = true
				}
				if strings.EqualFold(item.Field, "status") {
					statusItem = item
				}
			}

			if isMove {
				lastMoveTS = ts
				// Reset entry status - we found a new move, so we look for the new arrival
				entryStatus = ""
				entryStatusID = ""
				if statusItem != nil {
					// If the move itself changed the status, that's our first arrival candidate
					entryStatus = statusItem.ToString
					entryStatusID = statusItem.To
				}
			} else if lastMoveTS != 0 && entryStatus == "" && statusItem != nil {
				// This is the first status change AFTER a move that didn't have one in the same entry.
				// Per user directive: take the fromStatus as the context entry point.
				entryStatus = statusItem.FromString
				entryStatusID = statusItem.From
			}
		}

		// Pass 2: Emission - Transform and Heal
		for _, history := range dto.Changelog.Histories {
			tsObj, err := jira.ParseTime(history.Created)
			if err != nil {
				continue
			}
			ts := tsObj.UnixMicro()

			// HEALING: Discard history before the last move
			if lastMoveTS != 0 && ts < lastMoveTS {
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
				f := strings.ToLower(item.Field)
				switch f {
				case "status":
					event.FromStatus = item.FromString
					event.FromStatusID = item.From
					event.ToStatus = item.ToString
					event.ToStatusID = item.To
					hasSignal = true
				case "resolution":
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
		originalBirth := events[0].Timestamp // This is the 'Created' event added in step 2
		if entryStatus == "" && len(events) > 1 {
			// Fallback: If we still don't have entryStatus, take the first known status transition
			for _, e := range events {
				if e.EventType == Change && e.ToStatus != "" {
					entryStatus = e.FromStatus
					entryStatusID = e.FromStatusID
					break
				}
			}
		}
		// If still empty (never moved status in new project), current status is entryStatus
		if entryStatus == "" {
			entryStatus = initialStatus
			entryStatusID = initialStatusID
		}

		// Discard the old 'Created' and any 'Change' events we might have emitted exactly at lastMoveTS
		// if they are not relevant. Actually, we just replace the first event (Created).
		events[0] = IssueEvent{
			IssueKey:   issueKey,
			IssueType:  issueType,
			EventType:  Created,
			Timestamp:  originalBirth,
			ToStatus:   entryStatus,
			ToStatusID: entryStatusID,
			IsHealed:   true,
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
