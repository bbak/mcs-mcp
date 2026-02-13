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
			isRelevantMove := false
			hasWorkflowChange := false

			for j := range history.Items {
				item := &history.Items[j]
				if strings.EqualFold(item.Field, "status") {
					statusItem = item
				} else if strings.EqualFold(item.Field, "resolution") {
					resItem = item
				} else if strings.EqualFold(item.Field, "Key") {
					if strings.HasPrefix(item.To, targetProjectKey+"-") || strings.HasPrefix(item.ToString, targetProjectKey+"-") {
						isRelevantMove = true
					}
				} else if strings.EqualFold(item.Field, "workflow") && !strings.EqualFold(item.FromString, item.ToString) {
					hasWorkflowChange = true
				}
			}

			// Condition 1: Terminal Move (Entering Project with Workflow Boundary)
			// If we see a move into our project AND Workflow change, this is where the item "arrived".
			if isRelevantMove && hasWorkflowChange {
				if statusItem != nil {
					initialStatus = statusItem.FromString
					initialStatusID = statusItem.From
				} else if len(events) > 0 {
					// Fallback: Use the "From" side of the chronologically next change
					nextEvent := events[len(events)-1]
					initialStatus = nextEvent.FromStatus
					initialStatusID = nextEvent.FromStatusID
				}

				stopProcessing = true
			} else if statusItem != nil {
				// Condition 2: Normal Transition - trace back the "From" state for non-moved issues
				initialStatus = statusItem.FromString
				initialStatusID = statusItem.From
			}

			// Condition 3: Standard Transitions Emit
			// We emit ALL status and resolution changes we encounter.
			// Because we anchor the biological birth to the FromStatus at the boundary,
			// the boundary transition itself is correctly captured as a Change event.
			if statusItem != nil || resItem != nil {
				event := IssueEvent{
					IssueKey:  issueKey,
					IssueType: issueType,
					EventType: Change,
					Timestamp: ts,
				}

				if statusItem != nil {
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
		if events[i].EventType == Created {
			return true
		}
		if events[j].EventType == Created {
			return false
		}
		return false
	})

	return events
}
