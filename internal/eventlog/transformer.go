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

	// 1. Initial State from Snapshots (Fallback/Starting Point)
	// We'll walk history BACKWARDS to find where the issue entered our scope.
	initialStatus := dto.Fields.Status.Name
	initialStatusID := dto.Fields.Status.ID
	initialFlagged := extractFlaggedValue(dto.Fields.Flagged)

	// Infer target project key from current issue key (e.g., "PROJ" from "PROJ-123")
	targetProjectKey := issueKey
	if idx := strings.Index(issueKey, "-"); idx > 0 {
		targetProjectKey = issueKey[:idx]
	}

	stopProcessing := false
	if dto.Changelog != nil {
		// Pass 0: Ensure histories are chronological (ascending) first so we can reliably reverse.
		// Jira API often returns them descending, but we'll sort them to be sure.
		sort.Slice(dto.Changelog.Histories, func(i, j int) bool {
			t1, _ := jira.ParseTime(dto.Changelog.Histories[i].Created)
			t2, _ := jira.ParseTime(dto.Changelog.Histories[j].Created)
			return t1.Before(t2)
		})

		// 2. Process Histories BACKWARDS (Latest to Oldest)
		for i := len(dto.Changelog.Histories) - 1; i >= 0; i-- {
			history := dto.Changelog.Histories[i]
			tsObj, err := jira.ParseTime(history.Created)
			if err != nil {
				continue
			}
			ts := tsObj.UnixMicro()

			var statusItem *jira.ItemDTO
			var resItem *jira.ItemDTO
			var flaggedItem *jira.ItemDTO
			isRelevantMove := false

			for j := range history.Items {
				item := &history.Items[j]
				if strings.EqualFold(item.Field, "status") {
					statusItem = item
				} else if strings.EqualFold(item.Field, "resolution") {
					resItem = item
				} else if strings.EqualFold(item.Field, "Flagged") {
					flaggedItem = item
				} else if strings.EqualFold(item.Field, "Key") {
					if strings.HasPrefix(item.To, targetProjectKey+"-") || strings.HasPrefix(item.ToString, targetProjectKey+"-") {
						isRelevantMove = true
					}
				}
			}

			suppressStatus := false
			// Condition 1: Terminal Move (Entering Project with Workflow Boundary)
			// If we see a move into our project, this is where the item "arrived".
			if isRelevantMove {
				if statusItem != nil {
					initialStatus = statusItem.ToString
					initialStatusID = statusItem.To
					suppressStatus = true
				} else if len(events) > 0 {
					// Fallback: Use the status from the chronologically next status change.
					// We must walk back through already extracted events because 'Flagged' events
					// do not contain status information and might have been emitted later.
					for j := len(events) - 1; j >= 0; j-- {
						if events[j].FromStatus != "" || events[j].ToStatus != "" {
							// We use FromStatus because we are looking at the state *before* that change.
							// For a resolution-only change, FromStatus might be empty if only resolution changed,
							// but usually it's populated for status transitions.
							initialStatus = events[j].FromStatus
							initialStatusID = events[j].FromStatusID
							break
						}
					}
				}

				// If we hit a boundary, we also need to know the flagged state at arrival.
				if flaggedItem != nil {
					initialFlagged = flaggedItem.ToString
				}

				stopProcessing = true
			} else {
				if statusItem != nil {
					// Condition 2: Normal Transition - trace back the "From" state for non-moved issues
					initialStatus = statusItem.FromString
					initialStatusID = statusItem.From
				}
				if flaggedItem != nil {
					initialFlagged = flaggedItem.FromString
				}
			}

			// Condition 3: Standard Transitions Emit
			// We emit status and resolution changes.
			// If it's a move boundary, we suppress the status change (it's folded into 'Created' status).
			if (statusItem != nil && !suppressStatus) || resItem != nil {
				event := IssueEvent{
					IssueKey:  issueKey,
					IssueType: issueType,
					EventType: Change,
					Timestamp: ts,
				}

				if statusItem != nil && !suppressStatus {
					event.FromStatus = statusItem.FromString
					event.FromStatusID = statusItem.From
					event.ToStatus = statusItem.ToString
					event.ToStatusID = statusItem.To
				}

				if resItem != nil {
					if resItem.ToString != "" {
						event.Resolution = resItem.ToString
						event.IsUnresolved = false
					} else {
						event.IsUnresolved = true
						event.Resolution = ""
					}
				}
				events = append(events, event)
			}

			// Condition 4: Flagged Changes Emit
			if flaggedItem != nil {
				events = append(events, IssueEvent{
					IssueKey:  issueKey,
					IssueType: issueType,
					EventType: Flagged,
					Timestamp: ts,
					Flagged:   flaggedItem.ToString,
				})
			}

			if stopProcessing {
				// Only break if we've finished the entire "cluster" of events for this specific timestamp.
				// This handles cases where a move and a transition are recorded with identical timestamps.
				if i == 0 || dto.Changelog.Histories[i-1].Created != history.Created {
					break
				}
			}
		}
	}

	// 3. Anchoring the 'Created' event
	// This event represents the point where the clock starts, even if the issue is years old.
	// We use the original biological 'Created' timestamp but with the status we derived from the stop-point.
	createdTime, _ := jira.ParseTime(dto.Fields.Created)
	createdEvent := IssueEvent{
		IssueKey:   issueKey,
		IssueType:  issueType,
		EventType:  Created,
		Timestamp:  createdTime.UnixMicro(),
		ToStatus:   initialStatus, // This is now our "Arrival" or "Biological Birth" status
		ToStatusID: initialStatusID,
		Flagged:    initialFlagged,
		IsHealed:   stopProcessing, // Flag that we hit a boundary
	}

	// Add the created event to the list
	events = append(events, createdEvent)

	// 4. Handle Snapshot Resolution (Fallthrough/De-duplication)
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

	// 5. Finalize: Standardize Chronological Order
	sort.Slice(events, func(i, j int) bool {
		// Strict grouping: Created event always comes first if timestamps are identical
		if events[i].Timestamp != events[j].Timestamp {
			return events[i].Timestamp < events[j].Timestamp
		}
		// Tie-breaker for identical timestamps
		if events[i].EventType == Created && events[j].EventType != Created {
			return true
		}
		if events[j].EventType == Created && events[i].EventType != Created {
			return false
		}
		// Preserve order for other types (arbitrary but stable)
		return false
	})

	return events
}

func extractFlaggedValue(val any) string {
	if val == nil {
		return ""
	}
	// Jira Agile Flagged field is usually an array of objects: [{"id":"...","value":"Impediment"}]
	// For simplicity, we just want to know if it's "blocked" or not.
	slice, ok := val.([]any)
	if ok {
		if len(slice) == 0 {
			return ""
		}
		// Look at first item
		first := slice[0]
		if s, ok := first.(string); ok {
			return s
		}
		if m, ok := first.(map[string]any); ok {
			if v, ok := m["value"].(string); ok {
				return v
			}
		}
	}
	if s, ok := val.(string); ok {
		return s
	}
	return ""
}
