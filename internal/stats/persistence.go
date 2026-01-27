package stats

import (
	"math"
	"mcs-mcp/internal/jira"
	"sort"
)

// StatusPersistence provides historical residency analysis for a single status.
type StatusPersistence struct {
	StatusName     string  `json:"statusName"`
	Count          int     `json:"count"`
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

// CalculateStatusPersistence analyzes how long items spend in each status.
func CalculateStatusPersistence(issues []jira.Issue) []StatusPersistence {
	statusDurations := make(map[string][]float64)

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
		if len(durations) == 0 {
			continue
		}
		sort.Float64s(durations)
		n := len(durations)
		results = append(results, StatusPersistence{
			StatusName: status,
			Count:      n,
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

		// Interpretation Hint
		switch s.Role {
		case "queue":
			s.Interpretation = "This is a queue/waiting stage. Persistence here is 'Flow Debt'."
		case "active":
			if s.Tier == "Demand" {
				s.Interpretation = "This is item storage; high persistence is expected."
			} else {
				s.Interpretation = "This is a working stage. High persistence indicates a bottleneck."
			}
		case "ignore":
			s.Interpretation = "This status is ignored in most process diagnostics."
		}
	}
	return results
}
