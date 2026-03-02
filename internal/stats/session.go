package stats

import (
	"mcs-mcp/internal/eventlog"
	"mcs-mcp/internal/jira"
)

// AnalysisSession orchestrates the analytical pipeline for a single request.
// It manages data hydration, projection, and applying meta-workflow policies
// to provide a consistent set of items for various analytical tools.
type AnalysisSession struct {
	events      []eventlog.IssueEvent
	sourceID    string
	ctx         jira.SourceContext
	mappings    map[string]StatusMetadata
	resolutions map[string]string
	window      AnalysisWindow

	// Cached projections
	allIssues []jira.Issue
	delivered []jira.Issue
	wip       []jira.Issue

	isProjected bool
}

// NewAnalysisSession creates a new orchestration session.
func NewAnalysisSession(events []eventlog.IssueEvent, sourceID string, ctx jira.SourceContext, mapping map[string]StatusMetadata, resolutions map[string]string, window AnalysisWindow) *AnalysisSession {
	return &AnalysisSession{
		events:      events,
		sourceID:    sourceID,
		ctx:         ctx,
		mappings:    mapping,
		resolutions: resolutions,
		window:      window,
	}
}

// Project ensures that events are projected into domain issues for the session's window.
func (s *AnalysisSession) Project() error {
	if s.isProjected {
		return nil
	}

	// 1. Process events into basic domain issues
	finished, downstream, upstream, demand := ProjectScope(s.events, s.window, "", s.mappings, s.resolutions, nil)

	// We'll store all un-filtered items first
	s.allIssues = append(finished, append(downstream, append(upstream, demand...)...)...)
	s.wip = downstream
	s.delivered = FilterDelivered(finished)

	_ = upstream
	_ = demand

	s.isProjected = true
	return nil
}

// GetDelivered returns the set of successfully finished items in the window.
func (s *AnalysisSession) GetDelivered() []jira.Issue {
	_ = s.Project()
	return s.delivered
}

// GetWIP returns the set of items currently in progress.
func (s *AnalysisSession) GetWIP() []jira.Issue {
	_ = s.Project()
	return s.wip
}

// GetAllIssues returns all issues encountered in the window.
func (s *AnalysisSession) GetAllIssues() []jira.Issue {
	_ = s.Project()
	return s.allIssues
}

// SourceID returns the ID of the data source.
func (s *AnalysisSession) SourceID() string {
	return s.sourceID
}

// Window returns the analysis window.
func (s *AnalysisSession) Window() AnalysisWindow {
	return s.window
}
