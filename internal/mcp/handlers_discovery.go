package mcp

import (
	"fmt"
	"sort"
	"time"

	"mcs-mcp/internal/eventlog"
	"mcs-mcp/internal/jira"
	"mcs-mcp/internal/stats"
)

func (s *Server) handleGetWorkflowDiscovery(projectKey string, boardID int) (interface{}, error) {
	ctx, err := s.resolveSourceContext(projectKey, boardID)
	if err != nil {
		return nil, err
	}
	sourceID := getCombinedID(projectKey, boardID)

	// Hydrate
	if err := s.events.Hydrate(sourceID, ctx.JQL); err != nil {
		return nil, err
	}

	events := s.events.GetEventsInRange(sourceID, time.Time{}, time.Now())
	domainIssues := s.reconstructIssues(events, sourceID)

	// Apply age-constrained sampling (200 healthy items)
	sample := stats.SelectDiscoverySample(domainIssues, 200)

	return s.getWorkflowDiscovery(sourceID, sample), nil
}

func (s *Server) reconstructIssues(events []eventlog.IssueEvent, sourceID string) []jira.Issue {
	finished := s.getFinishedStatuses(sourceID)

	// Group events by issue key
	grouped := make(map[string][]eventlog.IssueEvent)
	for _, e := range events {
		grouped[e.IssueKey] = append(grouped[e.IssueKey], e)
	}

	var issues []jira.Issue
	for _, issueEvents := range grouped {
		issue := eventlog.ReconstructIssue(issueEvents, finished, time.Now())
		if !issue.IsSubtask {
			issues = append(issues, issue)
		}
	}
	return issues
}

func (s *Server) getWorkflowDiscovery(sourceID string, issues []jira.Issue) interface{} {
	persistence := stats.CalculateStatusPersistence(issues)

	// Build a map of significant statuses for quick lookup
	significant := make(map[string]bool)
	for _, p := range persistence {
		significant[p.StatusName] = true
	}

	// No longer need to fetch status categories from Jira
	proposal := stats.ProposeSemantics(issues, persistence)

	finalProposal := make(map[string]stats.StatusMetadata)
	for name, meta := range proposal {
		if !significant[name] {
			continue
		}
		finalProposal[name] = meta
	}

	// Filter and Sort Discovered Order
	// We use the proposed mapping (finalProposal) to determine tiers for sorting
	rawOrder := stats.DiscoverStatusOrder(issues)
	var discoveredOrder []string
	for _, st := range rawOrder {
		if significant[st] {
			discoveredOrder = append(discoveredOrder, st)
		}
	}

	// Sort by Tier: Demand < Upstream < Downstream < Finished
	tierWeights := map[string]int{
		"Demand":     1,
		"Upstream":   2,
		"Downstream": 3,
		"Finished":   4,
	}

	sort.SliceStable(discoveredOrder, func(i, j int) bool {
		ti := finalProposal[discoveredOrder[i]].Tier
		tj := finalProposal[discoveredOrder[j]].Tier
		if tierWeights[ti] != tierWeights[tj] {
			return tierWeights[ti] < tierWeights[tj]
		}
		// Secondary sort: keep existing relative order from DiscoverStatusOrder
		return i < j
	})

	// Data Summary for context
	events := s.events.GetEventsInRange(sourceID, time.Time{}, time.Now())
	domainIssues := s.reconstructIssues(events, sourceID)
	summary := stats.AnalyzeProbe(issues, len(domainIssues), s.getFinishedStatuses(sourceID))

	return map[string]interface{}{
		"source_id":         sourceID,
		"proposed_mapping":  finalProposal,
		"discovered_order":  discoveredOrder,
		"persistence_stats": persistence,
		"data_summary":      summary,
		"_guidance": []string{
			"AI MUST verify this semantic mapping with the user via 'set_workflow_mapping' before performing deeper analysis.",
			"Tiers (Demand, Upstream, Downstream, Finished) determine the analytical scope. 'Upstream' covers refinement, 'Downstream' covers execution.",
			"Roles (active, queue, ignore) determine if the clock is running or paused.",
			"Discovery uses a SMALL recent sample (200 items) to build the map. Use 'get_status_persistence' for robust performance analysis.",
			"Persistence stats (coin_toss, likely, etc.) measure INTERNAL residency time WITHIN one status. They ARE NOT end-to-end completion forecasts.",
			"WITHOUT a confirmed mapping, follow-up diagnostics (Aging, Stability, Simulation) will produce SUBPAR results.",
		},
	}
}

func (s *Server) handleSetWorkflowMapping(projectKey string, boardID int, mapping map[string]interface{}, resolutions map[string]interface{}) (interface{}, error) {
	sourceID := getCombinedID(projectKey, boardID)
	m := make(map[string]stats.StatusMetadata)
	for k, v := range mapping {
		if vm, ok := v.(map[string]interface{}); ok {
			m[k] = stats.StatusMetadata{
				Tier:    asString(vm["tier"]),
				Role:    asString(vm["role"]),
				Outcome: asString(vm["outcome"]),
			}
		}
	}
	s.workflowMappings[sourceID] = m

	if len(resolutions) > 0 {
		rm := make(map[string]string)
		for k, v := range resolutions {
			rm[k] = asString(v)
		}
		s.resolutionMappings[sourceID] = rm
	}

	return map[string]string{"status": "success", "message": fmt.Sprintf("Stored workflow mapping for source %s", sourceID)}, nil
}

func (s *Server) handleSetWorkflowOrder(projectKey string, boardID int, order []string) (interface{}, error) {
	sourceID := getCombinedID(projectKey, boardID)
	s.statusOrderings[sourceID] = order
	return map[string]string{"status": "success", "message": fmt.Sprintf("Stored workflow order for source %s", sourceID)}, nil
}
