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

// Round rounds all numeric fields to 2 decimal places for output compactness.
func (y *ProcessYield) Round() {
	y.OverallYieldRate = Round2(y.OverallYieldRate)
	for i := range y.LossPoints {
		y.LossPoints[i].Percentage = Round2(y.LossPoints[i].Percentage)
		y.LossPoints[i].AvgAge = Round2(y.LossPoints[i].AvgAge)
	}
}

// CalculateProcessYield analyzes abandonment points across tiers.
func CalculateProcessYield(issues []jira.Issue, mappings map[string]StatusMetadata, resolutions map[string]string) ProcessYield {
	yield := ProcessYield{
		TotalIngested: len(issues),
	}

	lossMap := make(map[string][]float64) // Tier -> Ages before abandonment

	for _, issue := range issues {
		// 2. Handle Outcome
		if issue.Outcome == "delivered" {
			yield.DeliveredCount++
		} else if issue.Outcome == "abandoned" {
			yield.AbandonedCount++

			// 3. Attribute to Tier (Heuristic-based Attribution)
			// We walk backwards and skip Finished-tier statuses to find where the item was abandoned.
			tier := "Demand"
			for i := len(issue.Transitions) - 1; i >= 0; i-- {
				tr := issue.Transitions[i]
				t := DetermineTier(jira.Issue{Status: tr.ToStatus, StatusID: tr.ToStatusID}, "", mappings)
				if t != "Finished" && t != "Unknown" {
					tier = t
					break
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

// CalculateStratifiedYield performs yield analysis breakdown by work item type.
func CalculateStratifiedYield(issues []jira.Issue, mappings map[string]StatusMetadata, resolutions map[string]string) map[string]ProcessYield {
	groups := make(map[string][]jira.Issue)
	for _, issue := range issues {
		t := issue.IssueType
		if t == "" {
			t = "Unknown"
		}
		groups[t] = append(groups[t], issue)
	}

	stratified := make(map[string]ProcessYield)
	for t, issues := range groups {
		stratified[t] = CalculateProcessYield(issues, mappings, resolutions)
	}
	return stratified
}
