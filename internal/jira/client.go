package jira

import (
	"encoding/json"
	"strings"
	"time"
)

// Issue represents a subset of Jira issue data needed for forecasting.
type Issue struct {
	Key               string
	ProjectKey        string
	IssueType         string
	Created           time.Time
	Updated           time.Time
	ResolutionDate    *time.Time
	Resolution        string
	ResolutionID      string
	Status            string
	StatusID          string
	BirthStatus       string
	BirthStatusID     string
	StatusCategory    string
	StatusResidency   map[string]int64 // Seconds spent in each status
	BlockedResidency  map[string]int64 // Total seconds spent in 'Blocked' state per status
	Transitions       []StatusTransition
	IsSubtask         bool
	IsMoved           bool
	Flagged           string
	HasSyntheticBirth bool // True if birth date was inferred from earliest event
}

// SourceContext formalizes the analytical "Center of Gravity" for a tool call.
type SourceContext struct {
	ProjectKey string
	BoardID    int
	JQL        string
	FetchedAt  time.Time
}

// StatusTransition represents a change in an issue's status.
type StatusTransition struct {
	FromStatus   string
	FromStatusID string
	ToStatus     string
	ToStatusID   string
	Date         time.Time
}

// NameRegistry provides a mapping from Jira IDs to their stable (untranslated) names,
// separated by entity type to ensure cohesion and avoid ID collisions.
type NameRegistry struct {
	Statuses    map[string]string `json:"statuses"`
	Resolutions map[string]string `json:"resolutions"`
}

// UnmarshalJSON handles both the new structured format and the legacy prefixed map format.
func (nr *NameRegistry) UnmarshalJSON(data []byte) error {
	// Try new format first
	type alias NameRegistry
	var aux alias
	if err := json.Unmarshal(data, &aux); err == nil && (len(aux.Statuses) > 0 || len(aux.Resolutions) > 0) {
		*nr = NameRegistry(aux)
		return nil
	}

	// Fallback to legacy map format
	var legacy map[string]string
	if err := json.Unmarshal(data, &legacy); err != nil {
		return err
	}

	nr.Statuses = make(map[string]string)
	nr.Resolutions = make(map[string]string)
	for k, v := range legacy {
		if strings.HasPrefix(k, "s:") {
			nr.Statuses[strings.TrimPrefix(k, "s:")] = v
		} else if strings.HasPrefix(k, "r:") {
			nr.Resolutions[strings.TrimPrefix(k, "r:")] = v
		}
	}
	return nil
}

// GetIDByName returns the ID for a given status name, or empty if not found.
func (nr *NameRegistry) GetStatusID(name string) string {
	if nr == nil {
		return ""
	}
	for id, n := range nr.Statuses {
		if strings.EqualFold(n, name) {
			return id
		}
	}
	return ""
}

// GetResolutionID returns the ID for a given resolution name, or empty if not found.
func (nr *NameRegistry) GetResolutionID(name string) string {
	if nr == nil {
		return ""
	}
	for id, n := range nr.Resolutions {
		if strings.EqualFold(n, name) {
			return id
		}
	}
	return ""
}

// GetStatusName returns the name for a status ID, or empty if not found.
func (nr *NameRegistry) GetStatusName(id string) string {
	if nr == nil {
		return ""
	}
	return nr.Statuses[id]
}

// GetResolutionName returns the name for a resolution ID, or empty if not found.
func (nr *NameRegistry) GetResolutionName(id string) string {
	if nr == nil {
		return ""
	}
	return nr.Resolutions[id]
}

// Client is the interface for interacting with Jira.
type Client interface {
	SearchIssues(jql string, startAt int, maxResults int) (*SearchResponse, error)
	GetIssueWithHistory(key string) (*IssueDTO, error)
	GetProject(key string) (any, error)
	GetProjectStatuses(key string) (any, error)
	GetBoard(id int) (any, error)
	GetBoardConfig(id int) (any, error)
	GetFilter(id string) (any, error)
	FindProjects(query string) ([]any, error)
	FindBoards(projectKey string, nameFilter string) ([]any, error)
	GetRegistry(projectKey string) (*NameRegistry, error)
}

// Config holds the authentication and connection settings for Jira.
type Config struct {
	BaseURL string

	// Data Center Cookies
	XsrfToken  string
	SessionID  string
	RememberMe string

	// Token Authentication
	Token     string
	TokenType string // "pat" or "api"
	UserEmail string // Required for Jira Cloud (api token)

	// Load Balancer Cookies
	GCILB string
	GCLB  string

	// Performance Settings
	RequestDelay time.Duration
}

// NewClient creates a new Jira client based on the provided configuration.
func NewClient(cfg Config) Client {
	return NewDataCenterClient(cfg)
}
