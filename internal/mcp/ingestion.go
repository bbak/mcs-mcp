package mcp

import (
	"fmt"
	"time"

	"mcs-mcp/internal/jira"
	"mcs-mcp/internal/stats"

	"github.com/rs/zerolog/log"
)

func (s *Server) ingestHistoricalIssues(sourceID string, ctx *jira.SourceContext, months int) ([]jira.Issue, int, error) {
	startTime := time.Now().AddDate(0, -months, 0)
	ingestJQL := fmt.Sprintf("(%s) AND resolutiondate >= '%s' ORDER BY resolutiondate ASC",
		ctx.JQL, startTime.Format("2006-01-02"))

	log.Debug().Str("source", sourceID).Str("jql", ingestJQL).Int("months", months).Msg("Starting historical ingestion")

	response, err := s.jira.SearchIssuesWithHistory(ingestJQL, 0, 1000)
	if err != nil {
		return nil, 0, err
	}

	if response.Total == 0 {
		return nil, 0, nil
	}

	finished := s.getFinishedStatuses(sourceID)
	issues := make([]jira.Issue, 0, len(response.Issues))
	for _, dto := range response.Issues {
		issue := stats.MapIssue(dto, finished)
		if !issue.IsSubtask {
			issues = append(issues, issue)
		}
	}

	return issues, response.Total, nil
}

func (s *Server) ingestWIPIssues(sourceID string, ctx *jira.SourceContext, withHistory bool) ([]jira.Issue, error) {
	wipJQL := fmt.Sprintf("(%s) AND resolution is EMPTY", ctx.JQL)

	var response *jira.SearchResponse
	var err error

	if withHistory {
		response, err = s.jira.SearchIssuesWithHistory(wipJQL, 0, 1000)
	} else {
		response, err = s.jira.SearchIssues(wipJQL, 0, 1000)
	}

	if err != nil {
		return nil, err
	}

	finished := s.getFinishedStatuses(sourceID)
	issues := make([]jira.Issue, 0, len(response.Issues))
	for _, dto := range response.Issues {
		issue := stats.MapIssue(dto, finished)
		if !issue.IsSubtask {
			issues = append(issues, issue)
		}
	}

	return issues, nil
}
