package jira

import (
	"time"
)

// Issue represents a subset of Jira issue data needed for forecasting.
type Issue struct {
	Key            string
	IssueType      string
	ResolutionDate *time.Time
	Resolution     string
	Status         string
	// More fields like changelog will be added later
}

// Client is the interface for interacting with Jira.
type Client interface {
	SearchIssues(jql string, startAt int, maxResults int) ([]Issue, int, error)
	GetProject(key string) (interface{}, error)
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
