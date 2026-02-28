package stats

import (
	"cmp"
	"math"
	"mcs-mcp/internal/jira"
	"slices"
)

// TierSummary aggregates persistence metrics by meta-workflow tier.
type TierSummary struct {
	Count          int      `json:"count"`
	Median         float64  `json:"combined_median"`
	P85            float64  `json:"combined_p85"`
	Statuses       []string `json:"statuses"`
	Interpretation string   `json:"interpretation,omitempty"`
}

// PersistenceResult is the top-level response for status persistence analysis.
type PersistenceResult struct {
	Statuses        []StatusPersistence            `json:"statuses"`
	TierSummary     map[string]TierSummary         `json:"tier_summary,omitempty"`
	TypePersistence map[string][]StatusPersistence `json:"type_persistence,omitempty"`
	Warnings        []string                       `json:"warnings,omitempty"`
	Guidance        []string                       `json:"_guidance,omitempty"`
}

// CalculateStatusPersistence analyzes how long items spend in each status.
func CalculateStatusPersistence(issues []jira.Issue) []StatusPersistence {
	if len(issues) == 0 {
		return nil
	}

	statusDurations := make(map[string][]float64)
	blockedDurations := make(map[string][]float64)
	totalIssues := float64(len(issues))

	// Track the most recent name seen for each ID as a fallback for display
	idToName := make(map[string]string)

	for _, issue := range issues {
		for statusID, seconds := range issue.StatusResidency {
			// Signal-Aware: Preserve terminal statuses even if residency is < 1m.
			isTerminal := (issue.ResolutionDate != nil || issue.Resolution != "") && (issue.StatusID == statusID || issue.Status == statusID)

			if seconds >= 60 || isTerminal {
				days := float64(seconds) / 86400.0
				statusDurations[statusID] = append(statusDurations[statusID], days)
			}

			// Capture name if this key is an ID
			if issue.StatusID == statusID {
				idToName[statusID] = issue.Status
			}
		}
		for statusID, seconds := range issue.BlockedResidency {
			if seconds >= 60 {
				days := float64(seconds) / 86400.0
				blockedDurations[statusID] = append(blockedDurations[statusID], days)
			}
		}
	}

	var results []StatusPersistence
	for statusID, durations := range statusDurations {
		n := len(durations)
		share := float64(n) / totalIssues

		if share < 0.01 {
			continue
		}

		slices.Sort(durations)
		sp := StatusPersistence{
			StatusID:   statusID,
			StatusName: idToName[statusID],
			Share:      math.Round(share*1000) / 1000,
			P50:        math.Round(durations[int(float64(n)*0.50)]*10) / 10,
			P70:        math.Round(durations[int(float64(n)*0.70)]*10) / 10,
			P85:        math.Round(durations[int(float64(n)*0.85)]*10) / 10,
			P95:        math.Round(durations[int(float64(n)*0.95)]*10) / 10,
			IQR:        math.Round((durations[int(float64(n)*0.75)]-durations[int(float64(n)*0.25)])*10) / 10,
			Inner80:    math.Round((durations[int(float64(n)*0.90)]-durations[int(float64(n)*0.10)])*10) / 10,
		}

		if sp.StatusName == "" {
			sp.StatusName = statusID // Fallback
		}

		if bd, ok := blockedDurations[statusID]; ok && len(bd) > 0 {
			slices.Sort(bd)
			bn := len(bd)
			sp.BlockedCount = bn
			sp.BlockedP50 = math.Round(bd[int(float64(bn)*0.50)]*10) / 10
			sp.BlockedP85 = math.Round(bd[int(float64(bn)*0.85)]*10) / 10
		}

		results = append(results, sp)
	}

	slices.SortFunc(results, func(a, b StatusPersistence) int {
		return cmp.Compare(a.StatusName, b.StatusName)
	})

	return results
}

// EnrichStatusPersistence adds semantic context to the persistence results.
func EnrichStatusPersistence(results []StatusPersistence, mappings map[string]StatusMetadata) []StatusPersistence {
	for i := range results {
		s := &results[i]

		if m, ok := GetMetadataRobust(mappings, s.StatusID, s.StatusName); ok {
			s.Role = m.Role
			s.Tier = m.Tier
			// Restore name from metadata if it's currently an ID or empty
			if m.Name != "" && (s.StatusName == "" || s.StatusName == s.StatusID) {
				s.StatusName = m.Name
			}
		}

		// Interpretation Hint (Emphasize INTERNAL residency, not completion)
		switch s.Role {
		case "queue":
			s.Interpretation = "This is a queue/waiting stage. Residency here is 'Flow Debt'. It is NOT completion time."
		case "active":
			if s.Tier == "Demand" {
				s.Interpretation = "This is item storage; high residency here is expected and does not impact lead time."
			} else {
				s.Interpretation = "This is an active working stage. High residency indicates a local bottleneck at this step."
			}
		case "ignore":
			s.Interpretation = "This status is ignored in most process diagnostics."
		}
	}
	return results
}

// CalculateTierSummary aggregates persistence data into tiers.
func CalculateTierSummary(issues []jira.Issue, mappings map[string]StatusMetadata) map[string]TierSummary {
	// 1. Group total residency per issue, per tier
	// issueTierTotals[tier][issueKey] = totalDays
	issueTierTotals := make(map[string]map[string]float64)
	tierStatuses := make(map[string]map[string]bool)

	for _, issue := range issues {
		for status, seconds := range issue.StatusResidency {
			if seconds < 60 { // Ignore automated touch-and-go transitions < 1m
				continue
			}
			days := float64(seconds) / 86400.0

			// Resolve Tier
			tier := "Unknown"
			if m, ok := GetMetadataRobust(mappings, "", status); ok {
				tier = m.Tier
			}

			// Skip terminal tier analysis in persistence overview
			if tier == "Finished" {
				continue
			}

			if issueTierTotals[tier] == nil {
				issueTierTotals[tier] = make(map[string]float64)
			}
			issueTierTotals[tier][issue.Key] += days

			if tierStatuses[tier] == nil {
				tierStatuses[tier] = make(map[string]bool)
			}
			tierStatuses[tier][status] = true
		}
	}

	summary := make(map[string]TierSummary)
	for tier, totalsMap := range issueTierTotals {
		// Convert map to slice for distribution analysis
		durations := make([]float64, 0, len(totalsMap))
		for _, d := range totalsMap {
			durations = append(durations, d)
		}

		if len(durations) == 0 {
			continue
		}
		slices.Sort(durations)
		n := len(durations)

		statuses := []string{}
		for s := range tierStatuses[tier] {
			statuses = append(statuses, s)
		}
		slices.Sort(statuses)

		interpretation := ""
		switch tier {
		case "Demand":
			interpretation = "Total time spent in the backlog/discovery phase. High numbers here are non-blocking."
		case "Upstream":
			interpretation = "Total time spent in definition/refinement. Key indicator of 'Definition Bottlenecks'."
		case "Downstream":
			interpretation = "Total time spent in implementation/testing. This is your primary delivery capacity."
		}

		summary[tier] = TierSummary{
			Count:          n,
			Median:         math.Round(durations[int(float64(n)*0.50)]*10) / 10,
			P85:            math.Round(durations[int(float64(n)*0.85)]*10) / 10,
			Statuses:       statuses,
			Interpretation: interpretation,
		}
	}

	return summary
}

// CalculateStratifiedStatusPersistence analyzes residency per status, grouped by issue type.
func CalculateStratifiedStatusPersistence(issues []jira.Issue) map[string][]StatusPersistence {
	if len(issues) == 0 {
		return nil
	}

	byType := make(map[string][]jira.Issue)
	for _, iss := range issues {
		t := iss.IssueType
		if t == "" {
			t = "Unknown"
		}
		byType[t] = append(byType[t], iss)
	}

	res := make(map[string][]StatusPersistence)
	for t, issSlice := range byType {
		res[t] = CalculateStatusPersistence(issSlice)
	}

	return res
}
