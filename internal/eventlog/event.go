package eventlog

// EventType defines the objective nature of a Jira state change.
type EventType string

const (
	// Created indicates the initial creation of the work item.
	Created EventType = "Created"
	// Transitioned indicates a status change.
	Transitioned EventType = "Transitioned"
	// Resolved indicates the application of a Jira resolution.
	Resolved EventType = "Resolved"
	// Moved indicates the item was moved to a different project or key.
	Moved EventType = "Moved"
	// Closed indicates the item has reached a terminal state in the process.
	Closed EventType = "Closed"
)

// IssueEvent represents a single atomic change in an issue's lifecycle.
// It is the primary unit of the event-sourced log.
type IssueEvent struct {
	// IssueKey is the Jira key (e.g., PROJ-123).
	IssueKey string `json:"issueKey"`
	// IssueType is the Jira item type (e.g., Story, Bug).
	IssueType string `json:"issueType"`
	// EventType is the type of change being recorded.
	EventType EventType `json:"eventType"`
	// Timestamp is the physical time the event occurred in Jira (Unix microseconds).
	Timestamp int64 `json:"ts"`

	// FromStatus is the status the item moved from (for Transitioned events).
	FromStatus string `json:"fromStatus,omitempty"`
	// ToStatus is the status the item moved to (for Transitioned events).
	ToStatus string `json:"toStatus,omitempty"`
	// Resolution is the Jira resolution name (for Resolved events).
	Resolution string `json:"resolution,omitempty"`

	// Metadata stores extensible Jira fields that might be relevant for projections.
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}
