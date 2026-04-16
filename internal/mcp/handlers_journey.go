package mcp

import (
	"fmt"
	"math"
	"time"

	"github.com/rs/zerolog/log"
	"mcs-mcp/internal/eventlog"
	"mcs-mcp/internal/jira"
	"mcs-mcp/internal/stats"
)

func (s *Server) handleGetItemJourney(projectKey string, boardID int, issueKey string) (any, error) {
	if err := s.anchorContext(projectKey, boardID); err != nil {
		return nil, err
	}

	ctx, err := s.resolveSourceContext(projectKey, boardID)
	if err != nil {
		return nil, err
	}
	sourceID := getCombinedID(projectKey, boardID)

	// 1. Try to find in existing memory log
	events := s.events.GetEventsForIssue(sourceID, issueKey)

	// 2. Fallback to context-locked hydration if not found
	if len(events) == 0 {
		lockedJQL := fmt.Sprintf("(%s) AND key = %s", ctx.JQL, issueKey)
		_, reg, err := s.events.Hydrate(sourceID, projectKey, lockedJQL, s.activeRegistry)
		if err != nil {
			return nil, err
		}
		s.activeRegistry = reg
		if err := s.saveWorkflow(projectKey, boardID); err != nil {
			log.Warn().Err(err).Msg("Failed to persist workflow metadata to disk")
		}
		events = s.events.GetEventsForIssue(sourceID, issueKey)
	}

	if len(events) == 0 {
		return nil, fmt.Errorf("issue %s not found on the current Project (%s) and Board (%d)", issueKey, projectKey, boardID)
	}

	// Finished statuses for reconstruction
	finishedMap := make(map[string]bool)
	for status, m := range s.activeMapping {
		if m.Tier == stats.TierFinished {
			finishedMap[status] = true
		}
	}

	issue := eventlog.ReconstructIssue(events, s.Clock())

	type JourneyStep struct {
		Status string  `json:"status"`
		Days   float64 `json:"days"`
	}
	var steps []JourneyStep

	// Reconstruct path for display
	if len(issue.Transitions) > 0 {
		birthStatus := issue.BirthStatus
		if birthStatus == "" {
			birthStatus = issue.Transitions[0].FromStatus
		}

		firstDuration := issue.Transitions[0].Date.Sub(issue.Created).Seconds()
		steps = append(steps, JourneyStep{
			Status: birthStatus,
			Days:   math.Round((firstDuration/86400.0)*10) / 10,
		})

		for i := 0; i < len(issue.Transitions)-1; i++ {
			duration := issue.Transitions[i+1].Date.Sub(issue.Transitions[i].Date).Seconds()
			steps = append(steps, JourneyStep{
				Status: issue.Transitions[i].ToStatus,
				Days:   math.Round((duration/86400.0)*10) / 10,
			})
		}

		var finalDate time.Time
		if issue.ResolutionDate != nil {
			finalDate = *issue.ResolutionDate
		} else {
			finalDate = s.Clock()
		}
		lastTrans := issue.Transitions[len(issue.Transitions)-1]
		finalDuration := finalDate.Sub(lastTrans.Date).Seconds()
		steps = append(steps, JourneyStep{
			Status: lastTrans.ToStatus,
			Days:   math.Round((finalDuration/86400.0)*10) / 10,
		})
	}

	residencyDays := make(map[string]float64)
	for st, sec := range issue.StatusResidency {
		residencyDays[st] = math.Round((float64(sec)/86400.0)*10) / 10
	}

	blockedDays := make(map[string]float64)
	for st, sec := range issue.BlockedResidency {
		blockedDays[st] = math.Round((float64(sec)/86400.0)*10) / 10
	}

	tierBreakdown := make(map[string]map[string]any)
	for status, sec := range issue.StatusResidency {
		tier := "Unknown"
		if s.activeMapping != nil {
			if m, ok := s.activeMapping[status]; ok {
				tier = m.Tier
			}
		}
		if _, ok := tierBreakdown[tier]; !ok {
			tierBreakdown[tier] = map[string]any{
				"days":         0.0,
				"blocked_days": 0.0,
				"statuses":     []string{},
			}
		}
		data := tierBreakdown[tier]
		data["days"] = data["days"].(float64) + math.Round((float64(sec)/86400.0)*10)/10

		if bSec, ok := issue.BlockedResidency[status]; ok {
			data["blocked_days"] = data["blocked_days"].(float64) + math.Round((float64(bSec)/86400.0)*10)/10
		}

		data["statuses"] = append(data["statuses"].([]string), status)
		tierBreakdown[tier] = data
	}

	res := map[string]any{
		"key":            issue.Key,
		"residency":      residencyDays,
		"blocked_time":   blockedDays,
		"path":           steps,
		"tier_breakdown": tierBreakdown,
		"warnings":       []string{},
	}

	guidance := []string{
		"The 'path' shows chronological flow, while 'residency' shows cumulative totals.",
	}

	return WrapResponse(res, projectKey, boardID, nil, s.getQualityWarnings([]jira.Issue{issue}), guidance), nil
}
