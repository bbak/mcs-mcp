// Package jira provides the Jira domain model (Issue, StatusTransition,
// NameRegistry, SourceContext), the Client interface, and Data Center /
// Cloud API implementations. All external API boundary concerns live here.
package jira

import (
	"encoding/json"
	"strings"
	"time"
)

// Issue represents a subset of Jira issue data needed for forecasting.
type Issue struct {
	Key            string     // The unique identifier of the issue
	ProjectKey     string     // The unique identifier of the project
	IssueType      string     // The type of the issue
	Created        time.Time  // The time when the issue was created
	Updated        time.Time  // The time when the issue was last updated; IMPORTANT: when sorting, use this field.
	ResolutionDate *time.Time // The time when the issue was resolved
	Resolution     string     // Name of the resolution, empty if not resolved
	ResolutionID   string     // ID of the resolution Name, empty if not resolved
	// Note: a non-empty ResolutionID (or Resolution) implies the issue is finished, but if can also be
	// finished without a resolution being set; therefore loot at Outcome.
	Status            string           // Name of the status
	StatusID          string           // ID of the status Name
	BirthStatus       string           // Name of the birth status
	BirthStatusID     string           // ID of the birth status Name
	StatusCategory    string           // Name of the status category
	StatusResidency   map[string]int64 // Seconds spent in each status
	BlockedResidency  map[string]int64 // Total seconds spent in 'Blocked' state per status
	Transitions       []StatusTransition
	IsSubtask         bool
	IsMoved           bool
	Flagged           string
	HasSyntheticBirth bool       // True if birth date was inferred from earliest event
	Outcome           string     // Empty if not finished, else it's 'delivered' or 'abandoned'
	OutcomeDate       *time.Time // The time when the issue was delivered or abandoned
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

// idByName performs a case-insensitive reverse lookup in the given map.
func idByName(m map[string]string, name string) string {
	for id, n := range m {
		if strings.EqualFold(n, name) {
			return id
		}
	}
	return ""
}

// GetStatusID returns the ID for a given status name, or empty if not found.
func (nr *NameRegistry) GetStatusID(name string) string {
	if nr == nil {
		return ""
	}
	return idByName(nr.Statuses, name)
}

// GetResolutionID returns the ID for a given resolution name, or empty if not found.
func (nr *NameRegistry) GetResolutionID(name string) string {
	if nr == nil {
		return ""
	}
	return idByName(nr.Resolutions, name)
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
	TokenType string // "pat" (Bearer) or "api" (Basic) — how to authenticate, not which deployment
	UserEmail string // Required for Jira Cloud (api token)

	// Load Balancer Cookies
	GCILB string
	GCLB  string

	// Deployment forces the API dialect: "cloud" or "datacenter". Empty means
	// detect it — see dcClient.resolveIsCloud.
	Deployment string

	// Performance Settings
	RequestDelay time.Duration
}

// IsCloud guesses the deployment flavour from the host. It is only a fallback:
// a custom domain may front either Cloud or Data Center, so the URL cannot
// answer this reliably. Prefer dcClient.resolveIsCloud, which asks the server.
//
// Deliberately not derived from TokenType. The two answer different questions
// — TokenType is about Basic vs Bearer — and conflating them misroutes any
// deployment where the auth method does not imply the flavour, most obviously
// cookie-authenticated Data Center.
func (c Config) IsCloud() bool {
	host := strings.TrimSpace(strings.ToLower(c.BaseURL))
	return strings.Contains(host, ".atlassian.net") || strings.Contains(host, ".jira.com")
}

// hostIsConclusive reports whether the host name settles the question on its
// own. Only the Atlassian-owned domains do: they are always Cloud. Every other
// host — including a custom domain fronting Cloud — is ambiguous, so a
// heuristic answer there is a guess and must be surfaced as one.
func (c Config) hostIsConclusive() bool {
	return c.IsCloud()
}

// NewClient creates a new Jira client based on the provided configuration.
func NewClient(cfg Config) Client {
	return NewDataCenterClient(cfg)
}
