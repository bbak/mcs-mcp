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
	CumulativeWIPDays        float64  `json:"cumulative_wip_days"`                 // Total time in statuses from commitment point onward, excluding Finished
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

// downstreamSince computes time spent in Downstream-tier statuses from a given date forward,
// by walking the issue's transitions chronologically. Used for backflow-aware WIP age.
func downstreamSince(issue jira.Issue, since time.Time, mappings map[string]StatusMetadata, startStatus string, now time.Time) float64 {
	var totalSeconds float64

	// Determine the status at the backflow point (the transition's ToStatus)
	// and walk forward from there.
	type segment struct {
		statusID string
		status   string
		start    time.Time
	}

	var segments []segment

	// Build segments from the backflow point onward
	for _, t := range issue.Transitions {
		if t.Date.Before(since) {
			continue
		}
		segments = append(segments, segment{
			statusID: t.ToStatusID,
			status:   t.ToStatus,
			start:    t.Date,
		})
	}

	if len(segments) == 0 {
		return 0
	}

	// Sum downstream time between consecutive segments
	for i := 0; i < len(segments)-1; i++ {
		seg := segments[i]
		dur := segments[i+1].start.Sub(seg.start).Seconds()
		t := DetermineTier(jira.Issue{StatusID: seg.statusID, Status: seg.status}, startStatus, mappings)
		if t == TierDownstream {
			totalSeconds += dur
		}
	}

	// Last segment: from last transition to now (or resolution)
	last := segments[len(segments)-1]
	endTime := now
	if issue.ResolutionDate != nil && issue.ResolutionDate.Before(now) {
		endTime = *issue.ResolutionDate
	}
	dur := endTime.Sub(last.start).Seconds()
	t := DetermineTier(jira.Issue{StatusID: last.statusID, Status: last.status}, startStatus, mappings)
	if t == TierDownstream {
		totalSeconds += dur
	}

	return totalSeconds / 86400.0
}

// CalculateInventoryAge identifies active items and calculates age (WIP or Total) and percentile.
// When commitmentBackflowReset is true and agingType is "wip", downstream residency is computed
// only from the last backflow date forward (backflow = transition to a status with weight < commitment weight).
// Total age and upstream days always reflect the full history.
func CalculateInventoryAge(wipIssues []jira.Issue, startStatus string, statusWeights map[string]int, mappings map[string]StatusMetadata, persistence []float64, agingType string, commitmentBackflowReset bool, evaluationTime time.Time) []InventoryAge {
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
		currentTier := DetermineTier(issue, startStatus, mappings)
		isFinished := false
		if currentTier == TierFinished {
			isFinished = true
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
		var upstreamDays, downstreamDays, wipDays, totalDays float64

		// Resolve commitment point weight once per issue
		commitmentWeight := 0
		if startStatus != "" {
			if w, ok := statusWeights[startStatus]; ok {
				commitmentWeight = w
			}
		}

		// Re-evaluate residency loop with ID robustness
		for status, seconds := range issue.StatusResidency {
			days := float64(seconds) / 86400.0
			totalDays += days

			// Try to find if this status name corresponds to an ID in our current mapping
			// Since StatusResidency ONLY has name, we must use Name-based lookup or a Name->ID map.
			// But mappings map can contain Names too.
			t := DetermineTier(jira.Issue{Status: status}, startStatus, mappings)
			switch t {
			case TierUpstream, TierDemand:
				upstreamDays += days
			case TierDownstream:
				downstreamDays += days
			}

			// WIP = all statuses from commitment point onward, excluding Finished
			if t != TierFinished && commitmentWeight > 0 {
				statusKey := status
				if w, ok := statusWeights[statusKey]; ok && w >= commitmentWeight {
					wipDays += days
				}
			}
		}

		// 2b. Backflow-aware downstream residency override
		// When enabled, only count downstream time from the last backflow forward.
		// Total age and upstream days remain based on full history.
		if commitmentBackflowReset && agingType == "wip" && commitmentWeight > 0 {
			lastBackflowIdx := -1
			for j, t := range issue.Transitions {
				sid := t.ToStatusID
				if sid == "" {
					sid = t.ToStatus
				}
				if w, ok := statusWeights[sid]; ok && w < commitmentWeight {
					lastBackflowIdx = j
				}
			}
			if lastBackflowIdx >= 0 {
				downstreamDays = downstreamSince(issue, issue.Transitions[lastBackflowIdx].Date, mappings, startStatus, evaluationTime)
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
			if currentTier == TierDownstream || currentTier == TierFinished {
				isStarted = true
			} else if startStatus != "" {
				commitmentWeight, okC := statusWeights[startStatus]
				currentWeight, okW := statusWeights[issue.StatusID]
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
			TotalAgeSinceCreation:    RoundTo(totalAgeRaw, 1),
			AgeSinceCommitment:       ageSinceCommitment,
			CumulativeWIPDays:        RoundTo(wipDays, 1),
			CumulativeUpstreamDays:   RoundTo(upstreamDays, 1),
			CumulativeDownstreamDays: RoundTo(downstreamDays, 1),
		}

		if agingType == "total" {
			analysis.Percentile = getP(ageRaw)
		} else if ageSinceCommitment != nil {
			analysis.Percentile = getP(ageRaw)
			if len(sortedPersistence) > 0 && !isFinished {
				p85 := CalculatePercentile(sortedPersistence, 0.85)
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

// AgingSummary provides an aggregate WIP health snapshot with risk-band distribution
// and Little's Law stability index. Designed to complement the per-item InventoryAge data.
type AgingSummary struct {
	TotalItems     int            `json:"total_items"`
	OutlierCount   int            `json:"outlier_count"`      // items > P85
	P50Threshold   float64        `json:"p50_threshold_days"` // historical cycle time P50
	P85Threshold   float64        `json:"p85_threshold_days"` // historical cycle time P85
	P95Threshold   float64        `json:"p95_threshold_days"` // historical cycle time P95
	Distribution   map[string]int `json:"distribution"`       // bucketed item counts
	StabilityIndex float64        `json:"stability_index"`    // Little's Law index
}

// CalculateAgingSummary computes aggregate WIP health metrics from individual aging data.
// cycleTimes must contain historical cycle times (unsorted is fine — will be copied and sorted).
// throughput is items/day over the analysis window.
func CalculateAgingSummary(ages []InventoryAge, cycleTimes []float64, wipCount int, throughput float64) AgingSummary {
	summary := AgingSummary{
		TotalItems: len(ages),
		Distribution: map[string]int{
			"Inconspicuous (within P50)": 0,
			"Aging (P50-P85)":            0,
			"Warning (P85-P95)":          0,
			"Extreme (>P95)":             0,
		},
	}

	if len(cycleTimes) == 0 {
		return summary
	}

	// Sort a copy to compute percentiles
	sorted := make([]float64, len(cycleTimes))
	copy(sorted, cycleTimes)
	slices.Sort(sorted)

	p50 := CalculatePercentile(sorted, 0.50)
	p85 := CalculatePercentile(sorted, 0.85)
	p95 := CalculatePercentile(sorted, 0.95)

	summary.P50Threshold = RoundTo(p50, 1)
	summary.P85Threshold = RoundTo(p85, 1)
	summary.P95Threshold = RoundTo(p95, 1)

	for _, a := range ages {
		age := 0.0
		if a.AgeSinceCommitment != nil {
			age = *a.AgeSinceCommitment
		}

		switch {
		case age < p50:
			summary.Distribution["Inconspicuous (within P50)"]++
		case age < p85:
			summary.Distribution["Aging (P50-P85)"]++
		case age < p95:
			summary.Distribution["Warning (P85-P95)"]++
		default:
			summary.Distribution["Extreme (>P95)"]++
		}

		if age > p85 {
			summary.OutlierCount++
		}
	}

	// Little's Law stability index
	if throughput > 0 && len(sorted) > 0 {
		avgCT := 0.0
		for _, ct := range sorted {
			avgCT += ct
		}
		avgCT /= float64(len(sorted))
		summary.StabilityIndex = LittlesLawIndex(wipCount, throughput, avgCT)
	}

	return summary
}
