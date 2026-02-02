package mcp

import (
	"mcs-mcp/internal/jira"
	"mcs-mcp/internal/stats"
)

type AnalysisContext struct {
	ProjectKey               string
	BoardID                  int
	SourceID                 string
	StatusWeights            map[string]int
	WorkflowMappings         map[string]stats.StatusMetadata
	FinishedStatuses         map[string]bool
	CommitmentPoint          string
	CommitmentPointIsDefault bool
	StatusOrder              []string
}

func (s *Server) prepareAnalysisContext(projectKey string, boardID int, issues []jira.Issue) *AnalysisContext {
	sourceID := getCombinedID(projectKey, boardID)
	statusWeights := s.getStatusWeights(issues)

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
	commitment, found := s.getEarliestCommitment(projectKey, boardID, issues)

	return &AnalysisContext{
		ProjectKey:               projectKey,
		BoardID:                  boardID,
		SourceID:                 sourceID,
		StatusWeights:            statusWeights,
		WorkflowMappings:         mappings,
		FinishedStatuses:         finished,
		CommitmentPoint:          commitment,
		CommitmentPointIsDefault: found,
		StatusOrder:              s.statusOrderings[sourceID],
	}
}
