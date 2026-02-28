package stats

import (
	"cmp"
	"math"
	"mcs-mcp/internal/jira"
	"slices"
	"time"
)

// StatusAgeAnalysis represents the risk of a single active item's residence in its current step.
type StatusAgeAnalysis struct {
	Key            string  `json:"key"`
	Type           string  `json:"type"`
	Status         string  `json:"status"`
	DaysInStatus   float64 `json:"daysInStatus"`
	Percentile     int     `json:"percentile"`       // e.g., 85 if it's at the P85 level
	IsAgingOutlier bool    `json:"is_aging_outlier"` // true if DaysInStatus > P85
}

// InventoryAge represents the process-wide risk of a single item.
type InventoryAge struct {
	Key                      string   `json:"key"`
	Type                     string   `json:"type"`
	Status                   string   `json:"status"`
	Tier                     string   `json:"tier"`                                // Terminal Tier context
	IsCompleted              bool     `json:"is_completed"`                        // True if in 'Finished' tier
	TotalAgeSinceCreation    float64  `json:"total_age_since_creation_days"`       // Caps at entry to Finished tier
	AgeSinceCommitment       *float64 `json:"age_since_commitment_days,omitempty"` // WIP Age (since last commitment)
	CumulativeWIPDays        float64  `json:"cumulative_wip_days"`                 // Total time spent in Downstream statuses
	AgeInCurrentStatus       float64  `json:"age_in_current_status_days"`
	CumulativeUpstreamDays   float64  `json:"cumulative_upstream_days"`
	CumulativeDownstreamDays float64  `json:"cumulative_downstream_days"`
	Percentile               int      `json:"percentile"` // Relative to historical distribution
	IsAgingOutlier           bool     `json:"is_aging_outlier"`
}

// AgingResult is the top-level response for inventory aging analysis.
type AgingResult struct {
	Items    []InventoryAge `json:"items"`
	Warnings []string       `json:"warnings,omitempty"`
	Guidance []string       `json:"_guidance,omitempty"`
}

// CalculateStatusAging identifies active items and compares their residence in current step to history.
func CalculateStatusAging(wipIssues []jira.Issue, persistence []StatusPersistence, evaluationTime time.Time) []StatusAgeAnalysis {
	var results []StatusAgeAnalysis

	pMap := make(map[string]StatusPersistence)
	for _, p := range persistence {
		pMap[PreferID(p.StatusID, p.StatusName)] = p
	}

	for _, issue := range wipIssues {
		var seconds int64
		if len(issue.Transitions) > 0 {
			seconds = int64(evaluationTime.Sub(issue.Transitions[len(issue.Transitions)-1].Date).Seconds())
		} else {
			seconds = int64(evaluationTime.Sub(issue.Created).Seconds())
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
			Status:       issue.Status,
			DaysInStatus: daysDisplay,
		}

		// ID-first lookup matching the pMap keying above.
		if p, ok := pMap[PreferID(issue.StatusID, issue.Status)]; ok {
			if daysRaw > p.P95 {
				analysis.Percentile = 95
				analysis.IsAgingOutlier = true
			} else if daysRaw > p.P85 {
				analysis.Percentile = 85
				analysis.IsAgingOutlier = true
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

	slices.SortFunc(results, func(a, b StatusAgeAnalysis) int {
		return cmp.Compare(b.DaysInStatus, a.DaysInStatus)
	})

	return results
}

// CalculateInventoryAge identifies active items and calculates age (WIP or Total) and percentile.
func CalculateInventoryAge(wipIssues []jira.Issue, startStatus string, statusWeights map[string]int, mappings map[string]StatusMetadata, persistence []float64, agingType string, evaluationTime time.Time) []InventoryAge {
	var results []InventoryAge

	// 1. Copy and Sort historical values for percentile calculation
	// We MUST copy here to avoid modifying the caller's slice (side-effect protection).
	sortedPersistence := make([]float64, len(persistence))
	copy(sortedPersistence, persistence)
	slices.Sort(sortedPersistence)

	getP := func(days float64) int {
		if len(sortedPersistence) == 0 {
			return 0
		}
		for i, v := range sortedPersistence {
			if v > days {
				return int(float64(i) / float64(len(sortedPersistence)) * 100)
			}
		}
		return 100
	}

	for _, issue := range wipIssues {
		// 0. Determine Tier Context
		currentTier := "Demand"
		isFinished := false
		if m, ok := GetMetadataRobust(mappings, issue.StatusID, issue.Status); ok {
			currentTier = m.Tier
			if m.Tier == "Finished" || m.Role == "terminal" {
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
				stepSeconds = int64(evaluationTime.Sub(lastTransDate).Seconds())
			}
		} else {
			if isFinished {
				stepSeconds = 0
			} else {
				stepSeconds = int64(evaluationTime.Sub(issue.Created).Seconds())
			}
		}
		stepDays := math.Ceil((float64(stepSeconds)/86400.0)*10) / 10

		// 2. Tier Residency & Total Age (Stop the clock)
		var upstreamDays, downstreamDays, totalDays float64

		// Re-evaluate residency loop with ID robustness
		for status, seconds := range issue.StatusResidency {
			days := float64(seconds) / 86400.0
			totalDays += days

			// Try to find if this status name corresponds to an ID in our current mapping
			// Since StatusResidency ONLY has name, we must use Name-based lookup or a Name->ID map.
			// But mappings map can contain Names too.
			if m, ok := GetMetadataRobust(mappings, "", status); ok {
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
		totalAgeRaw := totalDays

		if agingType == "total" {
			ageRaw = totalAgeRaw
		} else {
			// WIP Age is cumulative downstream residency (Nave-aligned)
			ageRaw = downstreamDays

			// Identify if it has started (ever reached commitment)
			// It is started if:
			// 1. It is explicitly in the Downstream or Finished tier
			// 2. Its current status weight is >= the startStatus weight
			isStarted := false
			if currentTier == "Downstream" || currentTier == "Finished" {
				isStarted = true
			} else if startStatus != "" {
				commitmentWeight, okC := GetWeightRobust(statusWeights, "", startStatus)
				currentWeight, okW := GetWeightRobust(statusWeights, issue.StatusID, issue.Status)
				if okC && okW && currentWeight >= commitmentWeight {
					isStarted = true
				}
			}

			if isStarted {
				rounded := math.Ceil(ageRaw*10) / 10
				ageSinceCommitment = &rounded
			}
		}

		if ageSinceCommitment == nil && agingType != "total" {
			continue
		}

		analysis := InventoryAge{
			Key:                      issue.Key,
			Type:                     issue.IssueType,
			Status:                   issue.Status,
			Tier:                     currentTier,
			IsCompleted:              isFinished,
			AgeInCurrentStatus:       stepDays,
			TotalAgeSinceCreation:    math.Round(totalAgeRaw*10) / 10,
			AgeSinceCommitment:       ageSinceCommitment,
			CumulativeWIPDays:        math.Round(downstreamDays*10) / 10,
			CumulativeUpstreamDays:   math.Round(upstreamDays*10) / 10,
			CumulativeDownstreamDays: math.Round(downstreamDays*10) / 10,
		}

		if agingType == "total" {
			analysis.Percentile = getP(ageRaw)
		} else if ageSinceCommitment != nil {
			analysis.Percentile = getP(ageRaw)
			if len(sortedPersistence) > 0 && !isFinished {
				p85 := sortedPersistence[int(float64(len(sortedPersistence))*0.85)]
				if ageRaw > p85 {
					analysis.IsAgingOutlier = true
				}
			}
		}

		results = append(results, analysis)
	}

	slices.SortFunc(results, func(a, b InventoryAge) int {
		if a.AgeSinceCommitment != nil && b.AgeSinceCommitment != nil {
			return cmp.Compare(*b.AgeSinceCommitment, *a.AgeSinceCommitment)
		}
		return cmp.Compare(b.AgeInCurrentStatus, a.AgeInCurrentStatus)
	})

	return results
}
