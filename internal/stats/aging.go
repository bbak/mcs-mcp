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
	Key                      string   `json:"key"`
	Type                     string   `json:"type"`
	Summary                  string   `json:"summary"`
	Status                   string   `json:"status"`
	Tier                     string   `json:"tier"`                                // Terminal Tier context
	IsCompleted              bool     `json:"is_completed"`                        // True if in 'Finished' tier
	TotalAgeSinceCreation    float64  `json:"total_age_since_creation_days"`       // Caps at entry to Finished tier
	AgeSinceCommitment       *float64 `json:"age_since_commitment_days,omitempty"` // WIP Age OR Final Cycle Time
	AgeInCurrentStatus       float64  `json:"age_in_current_status_days"`
	CumulativeUpstreamDays   float64  `json:"cumulative_upstream_days"`
	CumulativeDownstreamDays float64  `json:"cumulative_downstream_days"`
	Percentile               int      `json:"percentile"` // Relative to historical distribution
	IsStale                  bool     `json:"is_stale"`
}

// AgingResult is the top-level response for inventory aging analysis.
type AgingResult struct {
	Items    []InventoryAgeAnalysis `json:"items"`
	Warnings []string               `json:"warnings,omitempty"`
	Guidance []string               `json:"_guidance,omitempty"`
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
func CalculateInventoryAge(wipIssues []jira.Issue, startStatus string, statusWeights map[string]int, mappings map[string]StatusMetadata, persistence []float64, agingType string) []InventoryAgeAnalysis {
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
		// 0. Determine Tier Context
		currentTier := "Demand"
		isFinished := false
		if m, ok := mappings[issue.Status]; ok {
			currentTier = m.Tier
			if m.Tier == "Finished" {
				isFinished = true
			}
		}

		// 1. Current Step Age (Stopped for Finished)
		var stepSeconds int64
		if len(issue.Transitions) > 0 {
			lastTransDate := issue.Transitions[len(issue.Transitions)-1].Date
			if isFinished {
				stepSeconds = 0 // Transitioned into Finished, clock stops
			} else {
				stepSeconds = int64(time.Since(lastTransDate).Seconds())
			}
		} else {
			if isFinished {
				stepSeconds = 0
			} else {
				stepSeconds = int64(time.Since(issue.Created).Seconds())
			}
		}
		stepDays := math.Ceil((float64(stepSeconds)/86400.0)*10) / 10

		// 2. Tier Residency & Total Age (Stop the clock)
		var upstreamDays, downstreamDays, totalDays float64
		for status, seconds := range issue.StatusResidency {
			days := float64(seconds) / 86400.0
			totalDays += days
			if m, ok := mappings[status]; ok {
				switch m.Tier {
				case "Upstream", "Demand":
					upstreamDays += days
				case "Downstream":
					downstreamDays += days
				}
			}
		}

		// 3. Age (Process-wide)
		var ageSinceCommitment *float64
		var ageRaw float64
		totalAgeRaw := totalDays // Already pinned in StatusResidency by ProcessChangelog

		if agingType == "total" {
			ageRaw = totalAgeRaw
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

				// WIP Age / Cycle Time (Stopped for Finished)
				if isFinished && issue.ResolutionDate != nil {
					ageRaw = issue.ResolutionDate.Sub(start).Hours() / 24.0
				} else if isFinished && len(issue.Transitions) > 0 {
					// Pinned at entrance to Finished
					ageRaw = issue.Transitions[len(issue.Transitions)-1].Date.Sub(start).Hours() / 24.0
				} else {
					ageRaw = time.Since(start).Hours() / 24.0
				}

				rounded := math.Ceil(ageRaw*10) / 10
				ageSinceCommitment = &rounded
			}

			// Strictly filter out items that are not currently in a WIP status (unless Finished and we didn't filter it earlier)
			if ageSinceCommitment == nil {
				continue
			}
			currentWeight, ok := statusWeights[issue.Status]
			if ok && currentWeight < commitmentWeight && !isFinished {
				continue
			}
		}

		analysis := InventoryAgeAnalysis{
			Key:                      issue.Key,
			Type:                     issue.IssueType,
			Summary:                  issue.Summary,
			Status:                   issue.Status,
			Tier:                     currentTier,
			IsCompleted:              isFinished,
			AgeInCurrentStatus:       stepDays,
			TotalAgeSinceCreation:    math.Round(totalAgeRaw*10) / 10,
			AgeSinceCommitment:       ageSinceCommitment,
			CumulativeUpstreamDays:   math.Round(upstreamDays*10) / 10,
			CumulativeDownstreamDays: math.Round(downstreamDays*10) / 10,
		}

		if agingType == "total" {
			analysis.Percentile = getP(ageRaw)
		} else if ageSinceCommitment != nil {
			analysis.Percentile = getP(ageRaw)
			// Heuristic stale: > P85 (only for non-finished)
			if len(persistence) > 0 && !isFinished {
				p85 := persistence[int(float64(len(persistence))*0.85)]
				if ageRaw > p85 {
					analysis.IsStale = true
				}
			}
		}

		results = append(results, analysis)
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].AgeSinceCommitment != nil && results[j].AgeSinceCommitment != nil {
			return *results[i].AgeSinceCommitment > *results[j].AgeSinceCommitment
		}
		return results[i].AgeInCurrentStatus > results[j].AgeInCurrentStatus
	})

	return results
}
