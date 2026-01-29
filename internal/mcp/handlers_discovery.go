package mcp

import (
	"fmt"

	"mcs-mcp/internal/jira"
	"mcs-mcp/internal/stats"
)

func (s *Server) handleGetDataMetadata(sourceID, sourceType string) (interface{}, error) {
	ctx, err := s.resolveSourceContext(sourceID, sourceType)
	if err != nil {
		return nil, err
	}

	issues, err := s.jira.SearchIssuesWithHistory(ctx.JQL, 0, 50)
	if err != nil {
		return nil, err
	}

	finished := s.getFinishedStatuses(sourceID)
	domainIssues := make([]jira.Issue, 0, len(issues.Issues))
	for _, dto := range issues.Issues {
		issue := stats.MapIssue(dto, finished)
		if !issue.IsSubtask {
			domainIssues = append(domainIssues, issue)
		}
	}

	summary := stats.AnalyzeProbe(domainIssues, issues.Total, finished)

	return map[string]interface{}{
		"summary": summary,
		"_guidance": []string{
			"This is a DATA PROBE on a 50-item sample. Use it to understand data volume and health.",
			"SampleResolvedRatio is a diagnostic of the sample's completeness, NOT a team performance metric.",
			"Inventory counts (WIP/Backlog) are heuristics based on Jira Status Categories and your 'Finished' tier mapping.",
		},
	}, nil
}

func (s *Server) handleGetWorkflowDiscovery(sourceID, sourceType string) (interface{}, error) {
	ctx, err := s.resolveSourceContext(sourceID, sourceType)
	if err != nil {
		return nil, err
	}

	issues, err := s.jira.SearchIssuesWithHistory(ctx.JQL, 0, 200)
	if err != nil {
		return nil, err
	}

	finished := s.getFinishedStatuses(sourceID)
	domainIssues := make([]jira.Issue, 0, len(issues.Issues))
	for _, dto := range issues.Issues {
		domainIssues = append(domainIssues, stats.MapIssue(dto, finished))
	}

	return s.getWorkflowDiscovery(sourceID, domainIssues), nil
}

func (s *Server) getWorkflowDiscovery(sourceID string, issues []jira.Issue) interface{} {
	projectKeys := s.extractProjectKeys(issues)
	persistence := stats.CalculateStatusPersistence(issues)

	proposal := stats.ProposeSemantics(issues, persistence)

	// Refine proposal with status categories
	statusCats := s.getStatusCategories(projectKeys)
	for name, meta := range proposal {
		if cat, ok := statusCats[name]; ok {
			if cat == "done" {
				meta.Tier = "Finished"
				meta.Role = "active"
				proposal[name] = meta
			}
		}
	}

	return map[string]interface{}{
		"source_id":         sourceID,
		"proposed_mapping":  proposal,
		"discovered_order":  stats.DiscoverStatusOrder(issues),
		"persistence_stats": persistence,
		"_guidance": []string{
			"AI MUST verify this semantic mapping with the user before performing deeper analysis.",
			"Tiers (Demand, Upstream, Downstream, Finished) determine the analytical scope.",
			"Roles (active, queue, ignore) determine if the clock is running or paused.",
		},
	}
}

func (s *Server) handleSetWorkflowMapping(sourceID string, mapping map[string]interface{}, resolutions map[string]interface{}) (interface{}, error) {
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

func (s *Server) handleSetWorkflowOrder(sourceID string, order []string) (interface{}, error) {
	s.statusOrderings[sourceID] = order
	return map[string]string{"status": "success", "message": fmt.Sprintf("Stored workflow order for source %s", sourceID)}, nil
}
