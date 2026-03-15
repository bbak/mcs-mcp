package stats

import (
	"mcs-mcp/internal/jira"
	"time"
)

// CalculateFlowDebt compares arrival rates (commitment) vs. departure rates (exits) across the window.
func CalculateFlowDebt(issues []jira.Issue, window AnalysisWindow, commitmentPoint string, weights map[string]int, resolutions map[string]string, mappings map[string]StatusMetadata) FlowDebtResult {
	buckets := window.Subdivide()
	results := make([]FlowDebtBucket, len(buckets))

	for i, start := range buckets {
		results[i] = FlowDebtBucket{
			Label: window.GenerateLabel(start),
		}
	}

	targetWeight, hasCommitment := weights[commitmentPoint]

	for _, issue := range issues {
		// 1. Calculate Arrivals
		if hasCommitment {
			var arrivalDate *time.Time

			// check birth
			if bw, ok := weights[issue.BirthStatusID]; ok && bw >= targetWeight {
				arrivalDate = &issue.Created
			} else {
				// check transitions
				for _, t := range issue.Transitions {
					if tw, ok := weights[t.ToStatusID]; ok && tw >= targetWeight {
						arrivalDate = &t.Date
						break
					}
				}
			}

			if arrivalDate != nil {
				idx := window.FindBucketIndex(*arrivalDate)
				if idx >= 0 && idx < len(results) {
					results[idx].Arrivals++
				}
			}
		}

		// 2. Calculate Departures (any exit: delivered or abandoned)
		if HasExited(issue) && issue.OutcomeDate != nil {
			idx := window.FindBucketIndex(*issue.OutcomeDate)
			if idx >= 0 && idx < len(results) {
				results[idx].Departures++
			}
		}
	}

	totalDebt := 0
	for i := range results {
		results[i].Debt = results[i].Arrivals - results[i].Departures
		totalDebt += results[i].Debt
	}

	return FlowDebtResult{
		Buckets:   results,
		TotalDebt: totalDebt,
	}
}
