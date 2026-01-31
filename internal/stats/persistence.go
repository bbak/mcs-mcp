package stats

import (
	"math"
	"mcs-mcp/internal/jira"
	"sort"
)

// StatusPersistence provides historical residency analysis for a single status.
type StatusPersistence struct {
	StatusName     string  `json:"statusName"`
	Share          float64 `json:"share"`              // % of sample that visited this status
	Category       string  `json:"category,omitempty"` // Jira Category (To Do, In Progress, Done)
	Role           string  `json:"role,omitempty"`     // Functional Role (active, queue, ignore)
	Tier           string  `json:"tier,omitempty"`     // Meta-Workflow Tier (Demand, Upstream, Downstream, Finished)
	P50            float64 `json:"coin_toss"`          // P50
	P70            float64 `json:"probable"`           // P70
	P85            float64 `json:"likely"`             // P85
	P95            float64 `json:"safe_bet"`           // P95
	IQR            float64 `json:"iqr"`                // P75-P25
	Inner80        float64 `json:"inner_80"`           // P90-P10
	Interpretation string  `json:"interpretation,omitempty"`
}

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
	Statuses    []StatusPersistence    `json:"statuses"`
	TierSummary map[string]TierSummary `json:"tier_summary,omitempty"`
	Warnings    []string               `json:"warnings,omitempty"`
	Guidance    []string               `json:"_guidance,omitempty"`
}

// CalculateStatusPersistence analyzes how long items spend in each status.
func CalculateStatusPersistence(issues []jira.Issue) []StatusPersistence {
	if len(issues) == 0 {
		return nil
	}

	statusDurations := make(map[string][]float64)
	totalIssues := float64(len(issues))

	for _, issue := range issues {
		for status, seconds := range issue.StatusResidency {
			if seconds > 0 {
				days := float64(seconds) / 86400.0
				statusDurations[status] = append(statusDurations[status], days)
			}
		}
	}

	var results []StatusPersistence
	for status, durations := range statusDurations {
		n := len(durations)
		share := float64(n) / totalIssues

		// Filter noise: skip statuses visited by < 1% of work items
		if share < 0.01 {
			continue
		}

		sort.Float64s(durations)
		results = append(results, StatusPersistence{
			StatusName: status,
			Share:      math.Round(share*1000) / 1000,
			P50:        math.Round(durations[int(float64(n)*0.50)]*10) / 10,
			P70:        math.Round(durations[int(float64(n)*0.70)]*10) / 10,
			P85:        math.Round(durations[int(float64(n)*0.85)]*10) / 10,
			P95:        math.Round(durations[int(float64(n)*0.95)]*10) / 10,
			IQR:        math.Round((durations[int(float64(n)*0.75)]-durations[int(float64(n)*0.25)])*10) / 10,
			Inner80:    math.Round((durations[int(float64(n)*0.90)]-durations[int(float64(n)*0.10)])*10) / 10,
		})
	}

	// Sort results by status name for stability
	sort.Slice(results, func(i, j int) bool {
		return results[i].StatusName < results[j].StatusName
	})

	return results
}

// EnrichStatusPersistence adds semantic context to the persistence results.
func EnrichStatusPersistence(results []StatusPersistence, categories map[string]string, mappings map[string]StatusMetadata) []StatusPersistence {
	for i := range results {
		s := &results[i]
		if cat, ok := categories[s.StatusName]; ok {
			s.Category = cat
		}

		if m, ok := mappings[s.StatusName]; ok {
			s.Role = m.Role
			s.Tier = m.Tier
		} else {
			// Inferred defaults
			switch s.Category {
			case "to-do", "new":
				s.Tier = "Demand"
				s.Role = "active"
			case "indeterminate":
				s.Tier = "Downstream" // Conservative default
				s.Role = "active"
			case "done":
				s.Tier = "Finished"
				s.Role = "active"
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
	tierDurations := make(map[string][]float64)
	tierStatuses := make(map[string]map[string]bool)

	for _, issue := range issues {
		for status, seconds := range issue.StatusResidency {
			if seconds <= 0 {
				continue
			}
			days := float64(seconds) / 86400.0

			// Resolve Tier
			tier := "Unknown"
			if m, ok := mappings[status]; ok {
				tier = m.Tier
			}

			tierDurations[tier] = append(tierDurations[tier], days)
			if tierStatuses[tier] == nil {
				tierStatuses[tier] = make(map[string]bool)
			}
			tierStatuses[tier][status] = true
		}
	}

	summary := make(map[string]TierSummary)
	for tier, durations := range tierDurations {
		if len(durations) == 0 {
			continue
		}
		sort.Float64s(durations)
		n := len(durations)

		statuses := []string{}
		for s := range tierStatuses[tier] {
			statuses = append(statuses, s)
		}
		sort.Strings(statuses)

		interpretation := ""
		switch tier {
		case "Demand":
			interpretation = "Total time spent in the backlog/discovery phase. High numbers here are non-blocking."
		case "Upstream":
			interpretation = "Total time spent in definition/refinement. Key indicator of 'Definition Bottlenecks'."
		case "Downstream":
			interpretation = "Total time spent in implementation/testing. This is your primary delivery capacity."
		case "Finished":
			interpretation = "Total time spent in terminal statuses. Expected to be high for archived work."
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
