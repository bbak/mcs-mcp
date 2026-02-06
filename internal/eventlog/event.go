package eventlog

// EventType defines the objective nature of a Jira state change.
type EventType string

const (
	// Created indicates the initial creation of the work item (snapshot-derived).
	Created EventType = "Created"
	// Change indicates a state change from the Jira changelog (history-derived).
	// It may contain multiple field updates (status, resolution, etc.) atomic to one history entry.
	Change EventType = "Change"
)

// IssueEvent represents one or more atomic field changes from a Jira update.
// It is the primary unit of the event-sourced log.
type IssueEvent struct {
	// IssueKey is the Jira key (e.g., PROJ-123).
	IssueKey string `json:"issueKey"`
	// IssueType is the Jira item type (e.g., Story, Bug).
	IssueType string `json:"issueType"`
	// EventType is the origin of the event (Created or Change).
	EventType EventType `json:"eventType"`
	// Timestamp is the physical time the event occurred in Jira (Unix microseconds).
	Timestamp int64 `json:"ts"`

	// Status transitions (optional)
	FromStatus   string `json:"fromStatus,omitempty"`
	FromStatusID string `json:"fromStatusId,omitempty"`
	ToStatus     string `json:"toStatus,omitempty"`
	ToStatusID   string `json:"toStatusId,omitempty"`

	// Resolution changes (optional)
	// Resolution is populated for "Resolved" signals.
	// IsUnresolved is true for explicit "Unresolved" signals (resolution cleared).
	Resolution   string `json:"resolution,omitempty"`
	IsUnresolved bool   `json:"isUnresolved,omitempty"`

	// IsHealed indicates if the event was synthetically created/modified during history healing.
	IsHealed bool `json:"isHealed,omitempty"`

	// Metadata stores extensible Jira fields that might be relevant for projections.
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}
