package stats

import (
	"math"
	"mcs-mcp/internal/jira"
	"sort"
	"time"
)

// StatusAgeAnalysis represents the risk of a single active item's residence in its current step.
type StatusAgeAnalysis struct {
	Key          string  `json:"key"`
	Type         string  `json:"type"`
	Summary      string  `json:"summary"`
	Status       string  `json:"status"`
	DaysInStatus float64 `json:"daysInStatus"`
	Percentile   int     `json:"percentile"` // e.g., 85 if it's at the P85 level
	IsStale      bool    `json:"isStale"`    // true if DaysInStatus > P85
}

// InventoryAgeAnalysis represents the process-wide risk of a single item.
type InventoryAgeAnalysis struct {
	Key        string   `json:"key"`
	Type       string   `json:"type"`
	Summary    string   `json:"summary"`
	Status     string   `json:"status"`
	AgeDays    *float64 `json:"age_days,omitempty"` // Time passed (Total or WIP)
	StatusAge  float64  `json:"status_age_days"`    // Time in current status
	Percentile int      `json:"percentile"`         // Relative to historical distribution
	IsStale    bool     `json:"is_stale"`
}

// CalculateStatusAging identifies active items and compares their residence in current step to history.
func CalculateStatusAging(wipIssues []jira.Issue, persistence []StatusPersistence) []StatusAgeAnalysis {
	var results []StatusAgeAnalysis

	pMap := make(map[string]StatusPersistence)
	for _, p := range persistence {
		pMap[p.StatusName] = p
	}

	for _, issue := range wipIssues {
		var seconds int64
		if len(issue.Transitions) > 0 {
			seconds = int64(time.Since(issue.Transitions[len(issue.Transitions)-1].Date).Seconds())
		} else {
			seconds = int64(time.Since(issue.Created).Seconds())
		}

		daysRaw := float64(seconds) / 86400.0
		// Ceil-based rounding for display: at least 0.1
		daysDisplay := math.Ceil(daysRaw*10) / 10
		if daysDisplay < 0.1 {
			daysDisplay = 0.1
		}

		analysis := StatusAgeAnalysis{
			Key:          issue.Key,
			Type:         issue.IssueType,
			Summary:      issue.Summary,
			Status:       issue.Status,
			DaysInStatus: daysDisplay,
		}

		if p, ok := pMap[issue.Status]; ok {
			if daysRaw > p.P95 {
				analysis.Percentile = 95
				analysis.IsStale = true
			} else if daysRaw > p.P85 {
				analysis.Percentile = 85
				analysis.IsStale = true
			} else if daysRaw > p.P70 {
				analysis.Percentile = 70
			} else if daysRaw > p.P50 {
				analysis.Percentile = 50
			} else {
				analysis.Percentile = 10
			}
		}

		results = append(results, analysis)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].DaysInStatus > results[j].DaysInStatus
	})

	return results
}

// CalculateInventoryAge identifies active items and calculates age (WIP or Total) and percentile.
func CalculateInventoryAge(wipIssues []jira.Issue, startStatus string, statusWeights map[string]int, persistence []float64, agingType string) []InventoryAgeAnalysis {
	var results []InventoryAgeAnalysis

	// Sort historical values for percentile calculation
	sort.Float64s(persistence)
	getP := func(days float64) int {
		if len(persistence) == 0 {
			return 0
		}
		for i, v := range persistence {
			if v > days {
				return int(float64(i) / float64(len(persistence)) * 100)
			}
		}
		return 100
	}

	for _, issue := range wipIssues {
		// 1. Current Step Age
		var stepSeconds int64
		if len(issue.Transitions) > 0 {
			stepSeconds = int64(time.Since(issue.Transitions[len(issue.Transitions)-1].Date).Seconds())
		} else {
			stepSeconds = int64(time.Since(issue.Created).Seconds())
		}
		stepDays := math.Ceil((float64(stepSeconds)/86400.0)*10) / 10

		// 2. Age (Process-wide)
		var ageDays *float64
		var ageRaw float64

		if agingType == "total" {
			ageRaw = time.Since(issue.Created).Hours() / 24.0
			rounded := math.Ceil(ageRaw*10) / 10
			ageDays = &rounded
		} else {
			// WIP Age logic
			commitmentWeight := 2
			if startStatus != "" {
				if w, ok := statusWeights[startStatus]; ok {
					commitmentWeight = w
				}
			}

			var earliestAfterBackflow *time.Time
			isStarted := false

			// Is current status started?
			if weight, ok := statusWeights[issue.Status]; (ok && weight >= commitmentWeight) || (startStatus == "" && ok && weight >= 2) {
				isStarted = true
			}

			// Chronological scan to find the earliest commitment after the last backflow
			for _, t := range issue.Transitions {
				weight, ok := statusWeights[t.ToStatus]
				if ok && weight < commitmentWeight {
					// Backflow! Discard previous WIP history
					earliestAfterBackflow = nil
					isStarted = false
				} else if (startStatus != "" && t.ToStatus == startStatus) || (ok && weight >= commitmentWeight) {
					if earliestAfterBackflow == nil {
						st := t.Date
						earliestAfterBackflow = &st
					}
					isStarted = true
				}
			}

			if isStarted {
				var start time.Time
				if earliestAfterBackflow != nil {
					start = *earliestAfterBackflow
				} else {
					start = issue.Created
				}
				ageRaw = time.Since(start).Hours() / 24.0
				rounded := math.Ceil(ageRaw*10) / 10
				ageDays = &rounded
			}

			// Strictly filter out items that are not currently in a WIP status
			if ageDays == nil {
				continue
			}
			currentWeight, ok := statusWeights[issue.Status]
			if ok && currentWeight < commitmentWeight {
				continue
			}
		}

		analysis := InventoryAgeAnalysis{
			Key:       issue.Key,
			Type:      issue.IssueType,
			Summary:   issue.Summary,
			Status:    issue.Status,
			StatusAge: stepDays,
			AgeDays:   ageDays,
		}

		if ageDays != nil {
			analysis.Percentile = getP(ageRaw)
			// Heuristic stale: > P85
			if len(persistence) > 0 {
				p85 := persistence[int(float64(len(persistence))*0.85)]
				if ageRaw > p85 {
					analysis.IsStale = true
				}
			}
		}

		results = append(results, analysis)
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].AgeDays != nil && results[j].AgeDays != nil {
			return *results[i].AgeDays > *results[j].AgeDays
		}
		return results[i].StatusAge > results[j].StatusAge
	})

	return results
}
