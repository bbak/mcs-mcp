package stats

import (
	"mcs-mcp/internal/jira"
	"sort"
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

	sort.Slice(results, func(i, j int) bool {
		return results[i].WeekStarting.Before(results[j].WeekStarting)
	})

	return results
}
