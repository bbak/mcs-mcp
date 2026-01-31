package eventlog

import (
	"mcs-mcp/internal/jira"
	"time"
)

// TransformIssue converts a Jira Issue DTO and its changelog into a slice of IssueEvents.
func TransformIssue(dto jira.IssueDTO) []IssueEvent {
	var events []IssueEvent
	issueKey := dto.Key
	issueType := dto.Fields.IssueType.Name

	// 1. Identify Initial Status
	// We peek at the changelog to find the earliest 'status' change.
	// The 'FromString' of that change is the true initial status.
	initialStatus := dto.Fields.Status.Name
	if dto.Changelog != nil {
		var earliestTS *time.Time
		for _, h := range dto.Changelog.Histories {
			ts, err := jira.ParseTime(h.Created)
			if err != nil {
				continue
			}
			for _, item := range h.Items {
				if item.Field == "status" {
					if earliestTS == nil || ts.Before(*earliestTS) {
						earliestTS = &ts
						initialStatus = item.FromString
						if initialStatus == "" {
							initialStatus = item.ToString
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
			Timestamp:  createdTime,
			ToStatus:   initialStatus,
			SequenceID: 0,
		})
	}

	// 2. Changelog Transitions
	if dto.Changelog != nil {
		for i, history := range dto.Changelog.Histories {
			ts, err := jira.ParseTime(history.Created)
			if err != nil {
				continue
			}

			for j, item := range history.Items {
				switch item.Field {
				case "status":
					events = append(events, IssueEvent{
						IssueKey:   issueKey,
						IssueType:  issueType,
						EventType:  Transitioned,
						Timestamp:  ts,
						FromStatus: item.FromString,
						ToStatus:   item.ToString,
						SequenceID: int64(i*100 + j + 1),
					})
				case "resolution":
					if item.ToString != "" {
						events = append(events, IssueEvent{
							IssueKey:   issueKey,
							IssueType:  issueType,
							EventType:  Resolved,
							Timestamp:  ts,
							Resolution: item.ToString,
							SequenceID: int64(i*100 + j + 1),
						})
					}
				case "Key", "project":
					events = append(events, IssueEvent{
						IssueKey:   issueKey,
						IssueType:  issueType,
						EventType:  Moved,
						Timestamp:  ts,
						SequenceID: int64(i*100 + j + 1),
					})
				}
			}
		}
	}

	// 3. Resolution Event (Snapshot fallback if no changelog)
	if dto.Fields.ResolutionDate != "" {
		resTime, err := jira.ParseTime(dto.Fields.ResolutionDate)
		if err == nil {
			events = append(events, IssueEvent{
				IssueKey:   issueKey,
				IssueType:  issueType,
				EventType:  Resolved,
				Timestamp:  resTime,
				Resolution: dto.Fields.Resolution.Name,
				SequenceID: 999999, // High sequence for snapshot-based resolution
			})
		}
	}

	return events
}
