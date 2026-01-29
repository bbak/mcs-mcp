package mcp

import (
	"mcs-mcp/internal/jira"
	"mcs-mcp/internal/stats"
)

type AnalysisContext struct {
	SourceID         string
	StatusWeights    map[string]int
	WorkflowMappings map[string]stats.StatusMetadata
	FinishedStatuses map[string]bool
	CommitmentPoint  string
	StatusOrder      []string
}

func (s *Server) prepareAnalysisContext(sourceID string, issues []jira.Issue) *AnalysisContext {
	projectKeys := s.extractProjectKeys(issues)
	statusWeights := s.getStatusWeights(projectKeys)

	mappings := s.workflowMappings[sourceID]
	if mappings == nil {
		mappings = make(map[string]stats.StatusMetadata)
	}

	// Apply known mappings to weights
	for name, metadata := range mappings {
		switch metadata.Tier {
		case "Demand":
			statusWeights[name] = 1
		case "Downstream", "Finished":
			if statusWeights[name] < 2 {
				statusWeights[name] = 2
			}
		}
	}

	finished := s.getFinishedStatuses(sourceID)

	// Determine commitment point
	commitment := s.getEarliestCommitment(sourceID)

	return &AnalysisContext{
		SourceID:         sourceID,
		StatusWeights:    statusWeights,
		WorkflowMappings: mappings,
		FinishedStatuses: finished,
		CommitmentPoint:  commitment,
		StatusOrder:      s.statusOrderings[sourceID],
	}
}
