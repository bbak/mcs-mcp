package eventlog

import (
	"mcs-mcp/internal/jira"
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
				if item.Field == "status" {
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
	if dto.Changelog != nil {
		for _, history := range dto.Changelog.Histories {
			tsObj, err := jira.ParseTime(history.Created)
			if err != nil {
				continue
			}
			ts := tsObj.UnixMicro()

			// Tracking signals within this history entry (transaction)
			moveSignal := false

			for _, item := range history.Items {
				switch item.Field {
				case "status":
					events = append(events, IssueEvent{
						IssueKey:     issueKey,
						IssueType:    issueType,
						EventType:    Transitioned,
						Timestamp:    ts,
						FromStatus:   item.FromString,
						FromStatusID: item.From,
						ToStatus:     item.ToString,
						ToStatusID:   item.To,
					})
				case "resolution":
					if item.ToString != "" {
						events = append(events, IssueEvent{
							IssueKey:   issueKey,
							IssueType:  issueType,
							EventType:  Resolved,
							Timestamp:  ts,
							Resolution: item.ToString,
						})
					} else {
						// Case 1: Resolution explicitly cleared in Jira
						events = append(events, IssueEvent{
							IssueKey:  issueKey,
							IssueType: issueType,
							EventType: Unresolved,
							Timestamp: ts,
						})
					}
				case "Key", "project":
					moveSignal = true
				}
			}

			if moveSignal {
				events = append(events, IssueEvent{
					IssueKey:  issueKey,
					IssueType: issueType,
					EventType: Moved,
					Timestamp: ts,
				})
			}
		}
	}

	// 4. Resolution Event (Snapshot fallback)
	if dto.Fields.ResolutionDate != "" {
		resTime, err := jira.ParseTime(dto.Fields.ResolutionDate)
		if err == nil {
			ts := resTime.UnixMicro()
			resName := dto.Fields.Resolution.Name

			// De-duplication check:
			// If we already have a Resolved event for this resolution within a 2s grace period, skip fallback.
			// Jira's API snapshots and history can have slight precision offsets.
			duplicate := false
			const gracePeriod = 2000000 // 2 seconds in microseconds

			for _, e := range events {
				if e.EventType == Resolved && e.Resolution == resName {
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
					EventType:  Resolved,
					Timestamp:  ts,
					Resolution: resName,
				})
			}
		}
	}

	return events
}
