package stats

import (
	"mcs-mcp/internal/jira"
	"slices"
	"time"
)

// DeliveryCadence represents a weekly snapshot of throughput.
type DeliveryCadence struct {
	WeekStarting   time.Time `json:"weekStarting"`
	ItemsDelivered int       `json:"itemsDelivered"`
}

// CalculateDeliveryCadence aggregates items resolved per week.
func CalculateDeliveryCadence(issues []jira.Issue, windowWeeks int) []DeliveryCadence {
	weeks := make(map[time.Time]int)

	// Start date for the window
	cutoff := time.Now().AddDate(0, 0, -windowWeeks*7)

	// Normalize time to the start of the week (Monday)
	normalize := func(t time.Time) time.Time {
		// Go's Weekday starts at Sunday=0. We want Monday to be the anchor.
		offset := int(t.Weekday()) - 1
		if offset < 0 {
			offset = 6 // Sunday
		}
		return time.Date(t.Year(), t.Month(), t.Day()-offset, 0, 0, 0, 0, t.Location())
	}

	for _, issue := range issues {
		if issue.ResolutionDate != nil && issue.ResolutionDate.After(cutoff) {
			week := normalize(*issue.ResolutionDate)
			weeks[week]++
		}
	}

	var results []DeliveryCadence
	for week, count := range weeks {
		results = append(results, DeliveryCadence{
			WeekStarting:   week,
			ItemsDelivered: count,
		})
	}

	slices.SortFunc(results, func(a, b DeliveryCadence) int {
		return a.WeekStarting.Compare(b.WeekStarting)
	})

	return results
}

// CalculateFlowDebt compares arrival rates (commitment) vs. departure rates (delivery) across the window.
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

		// 2. Calculate Departures
		if IsDelivered(issue, resolutions, mappings) && issue.ResolutionDate != nil {
			idx := window.FindBucketIndex(*issue.ResolutionDate)
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
