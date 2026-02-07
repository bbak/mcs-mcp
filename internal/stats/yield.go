package stats

import (
	"math"
	"strings"

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
		// 1. Determine Outcome
		outcome := resolutions[issue.Resolution]
		if outcome == "" {
			if m, ok := GetMetadataRobust(mappings, issue.StatusID, issue.Status); ok {
				outcome = m.Outcome
			}
		}

		// 2. Handle Outcome
		if outcome == "delivered" {
			yield.DeliveredCount++
		} else if strings.HasPrefix(outcome, "abandoned") {
			yield.AbandonedCount++

			// 3. Attribute to Tier (Explicit vs Heuristic)
			tier := "Demand"
			if strings.Contains(outcome, "_") {
				// Calibration-based Attribution (e.g., 'abandoned_upstream')
				parts := strings.Split(outcome, "_")
				if len(parts[1]) > 0 {
					tier = strings.ToUpper(parts[1][:1]) + strings.ToLower(parts[1][1:])
				}
			} else {
				// Heuristic-based Attribution: use the tier of the status BEFORE it reached Finished
				if len(issue.Transitions) > 0 {
					lastStatus := issue.Transitions[len(issue.Transitions)-1].ToStatus
					if m, ok := GetMetadataRobust(mappings, "", lastStatus); ok {
						tier = m.Tier
					}
				}
			}

			// Total age in the process as the 'cost' of the loss
			age := 0.0
			for _, s := range issue.StatusResidency {
				age += float64(s) / 86400.0
			}
			lossMap[tier] = append(lossMap[tier], age)
		} else if issue.ResolutionDate != nil {
			// Legacy Fallback: any resolution counts as delivered if not mapped as abandoned
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
		switch tier {
		case "Downstream":
			severity = "High"
		case "Upstream":
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
