package jira

import (
	"time"
)

// Issue represents a subset of Jira issue data needed for forecasting.
type Issue struct {
	Key             string
	ProjectKey      string
	IssueType       string
	Summary         string
	Created         time.Time
	Updated         time.Time
	ResolutionDate  *time.Time
	Resolution      string
	Status          string
	StatusID        string
	StatusCategory  string
	StatusResidency map[string]int64 // Seconds spent in each status
	Transitions     []StatusTransition
	IsSubtask       bool
	IsMoved         bool
}

// SourceContext formalizes the analytical "Center of Gravity" for a tool call.
type SourceContext struct {
	SourceID       string
	SourceType     string // "board" or "filter"
	JQL            string
	PrimaryProject string // Inferred or user-provided anchor
	FetchedAt      time.Time
}

// StatusTransition represents a change in an issue's status.
type StatusTransition struct {
	FromStatus   string
	FromStatusID string
	ToStatus     string
	ToStatusID   string
	Date         time.Time
}

// Client is the interface for interacting with Jira.
type Client interface {
	SearchIssues(jql string, startAt int, maxResults int) (*SearchResponse, error)
	SearchIssuesWithHistory(jql string, startAt int, maxResults int) (*SearchResponse, error)
	GetProject(key string) (interface{}, error)
	GetProjectStatuses(key string) (interface{}, error)
	GetBoard(id int) (interface{}, error)
	GetBoardConfig(id int) (interface{}, error)
	GetFilter(id string) (interface{}, error)
	FindProjects(query string) ([]interface{}, error)
	FindBoards(projectKey string, nameFilter string) ([]interface{}, error)
}

// Config holds the authentication and connection settings for Jira.
type Config struct {
	BaseURL string

	// Data Center Cookies
	XsrfToken  string
	SessionID  string
	RememberMe string

	// Token Authentication
	Token string

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
