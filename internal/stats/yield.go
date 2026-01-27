package stats

import (
	"math"
	"mcs-mcp/internal/jira"
)

// ProcessYield represents the delivery efficiency across tiers.
type ProcessYield struct {
	TotalIngested    int                  `json:"totalIngested"`
	DeliveredCount   int                  `json:"deliveredCount"`
	AbandonedCount   int                  `json:"abandonedCount"`
	OverallYieldRate float64              `json:"overallYieldRate"`
	LossPoints       []AbandonmentInsight `json:"lossPoints"`
}

// AbandonmentInsight quantifies waste at a specific stage.
type AbandonmentInsight struct {
	Tier       string  `json:"tier"`
	Count      int     `json:"count"`
	Percentage float64 `json:"percentage"` // % of total items abandoned at this tier
	AvgAge     float64 `json:"avgAge"`     // Avg residency in that tier before abandonment
	Severity   string  `json:"severity"`   // Indicator of waste (High for Downstream)
}

// CalculateProcessYield analyzes abandonment points across tiers.
func CalculateProcessYield(issues []jira.Issue, mappings map[string]StatusMetadata, resolutions map[string]string) ProcessYield {
	yield := ProcessYield{
		TotalIngested: len(issues),
	}

	lossMap := make(map[string][]float64) // Tier -> Ages before abandonment

	for _, issue := range issues {
		isAbandoned := resolutions[issue.Resolution] == "abandoned"
		if isAbandoned {
			yield.AbandonedCount++
			// Determine which tier it was abandoned from
			// It's the Tier of the status BEFORE it reached Finished
			lastTier := "Demand"
			if len(issue.Transitions) > 0 {
				lastStatus := issue.Transitions[len(issue.Transitions)-1].ToStatus
				if m, ok := mappings[lastStatus]; ok {
					lastTier = m.Tier
				}
			}

			// Total age in the process
			age := 0.0
			for _, s := range issue.StatusResidency {
				age += float64(s) / 86400.0
			}
			lossMap[lastTier] = append(lossMap[lastTier], age)
		} else if issue.ResolutionDate != nil {
			yield.DeliveredCount++
		}
	}

	if yield.TotalIngested > 0 {
		yield.OverallYieldRate = float64(yield.DeliveredCount) / float64(yield.TotalIngested)
	}

	for _, tier := range []string{"Demand", "Upstream", "Downstream"} {
		ages := lossMap[tier]
		if len(ages) == 0 {
			continue
		}

		sum := 0.0
		for _, a := range ages {
			sum += a
		}

		severity := "Low"
		if tier == "Downstream" {
			severity = "High"
		} else if tier == "Upstream" {
			severity = "Medium"
		}

		yield.LossPoints = append(yield.LossPoints, AbandonmentInsight{
			Tier:       tier,
			Count:      len(ages),
			Percentage: float64(len(ages)) / float64(yield.TotalIngested),
			AvgAge:     math.Round((sum/float64(len(ages)))*10) / 10,
			Severity:   severity,
		})
	}

	return yield
}
