package simulation

import (
	"mcs-mcp/internal/jira"
	"mcs-mcp/internal/stats"
	"time"
)

// ForecastRequest encapsulates all inputs needed by any engine to produce a forecast.
// Each engine owns the full pipeline from raw issues to Result.
type ForecastRequest struct {
	// Mode: "duration" or "scope"
	Mode string

	// Issue sets (pre-filtered by the handler's AnalysisSession)
	AllIssues []jira.Issue
	Finished  []jira.Issue
	WIP       []jira.Issue

	// Sampling window
	WindowStart     time.Time
	WindowEnd       time.Time
	DiscoveryCutoff time.Time

	// Duration mode
	Targets     map[string]int
	MixOverrides map[string]float64

	// Scope mode
	TargetDays int

	// Filters
	IssueTypes []string

	// Workflow context
	CommitmentPoint  string
	StatusWeights    map[string]int
	WorkflowMappings map[string]stats.StatusMetadata
	Resolutions      map[string]string

	// Determinism
	SimulationSeed int64

	// Clock override (evaluation date)
	Clock time.Time
}

// ForecastEngine is the interface that all simulation engines must implement.
type ForecastEngine interface {
	// Name returns the unique identifier for this engine (e.g., "crude", "bbak").
	Name() string

	// Run executes the full forecast pipeline and returns a Result.
	Run(req ForecastRequest) (Result, error)
}
