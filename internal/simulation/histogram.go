package simulation

import (
	"fmt"
	"math/rand/v2"
	"mcs-mcp/internal/jira"
	"mcs-mcp/internal/stats"
	"slices"
	"time"
)

// Histogram tracks daily throughput counts.
type Histogram struct {
	Counts           []int
	StratifiedCounts map[string][]int
	Meta             map[string]any
}

// NewHistogram creates a histogram from a list of resolved issues.
func NewHistogram(issues []jira.Issue, startTime, endTime time.Time, issueTypes []string, mappings map[string]stats.StatusMetadata, resolutionMappings map[string]string) *Histogram {
	// Create maps for fast lookup
	typeMap := make(map[string]bool)
	for _, t := range issueTypes {
		typeMap[t] = true
	}

	// Calculate number of days in range
	days := int(endTime.Sub(startTime).Hours()/24) + 1
	if days <= 0 {
		return &Histogram{Counts: []int{0}, StratifiedCounts: make(map[string][]int)}
	}

	buckets := make([]int, days)
	stratified := make(map[string][]int)
	typeCounts := make(map[string]int)
	totalDelivered := 0

	droppedByOutcome := 0
	droppedByWindow := 0

	// 1. First pass: Collect delivered items and their types
	deliveredIssues := make([]jira.Issue, 0)
	for _, issue := range issues {
		if !stats.IsDelivered(issue) {
			droppedByOutcome++
			continue
		}
		deliveredIssues = append(deliveredIssues, issue)
	}

	// 2. Assess stratification eligibility
	decisions := AssessStratificationNeeds(deliveredIssues)

	// 3. Second pass: Fill buckets
	for _, issue := range deliveredIssues {
		var resDate time.Time
		if issue.ResolutionDate != nil {
			resDate = *issue.ResolutionDate
		} else {
			resDate = issue.Updated
		}

		if resDate.IsZero() {
			continue
		}

		dayIdx := int(resDate.Sub(startTime).Hours() / 24)
		if dayIdx >= 0 && dayIdx < days {
			buckets[dayIdx]++
			typeCounts[issue.IssueType]++
			totalDelivered++

			if _, ok := stratified[issue.IssueType]; !ok {
				stratified[issue.IssueType] = make([]int, days)
			}
			stratified[issue.IssueType][dayIdx]++
		} else {
			droppedByWindow++
		}
	}

	// Create meta for eligibility
	stratEligible := make(map[string]bool)
	for _, d := range decisions {
		if d.Eligible {
			stratEligible[d.Type] = true
		}
	}

	// Calculate normalized distribution
	typeDist := make(map[string]float64)
	if totalDelivered > 0 {
		types := make([]string, 0, len(typeCounts))
		for t := range typeCounts {
			types = append(types, t)
		}
		slices.Sort(types)

		for _, t := range types {
			c := typeCounts[t]
			typeDist[t] = float64(c) / float64(totalDelivered)
		}
	}

	avgAcross := 0.0
	recentAvg := 0.0
	totalCount := 0
	recentCount := 0

	firstDeliveryIdx := -1
	for i, c := range buckets {
		if c > 0 && firstDeliveryIdx == -1 {
			firstDeliveryIdx = i
		}
		totalCount += c
		if i >= days-30 {
			recentCount += c
		}
	}

	if days > 0 {
		denominator := days
		if firstDeliveryIdx != -1 {
			denominator = days - firstDeliveryIdx
		}
		avgAcross = float64(totalCount) / float64(denominator)

		recentDays := 30
		if days < 30 {
			recentDays = days
		}
		recentAvg = float64(recentCount) / float64(recentDays)
	}

	// Calculate dependencies and volatility for stratified types
	dependencies := DetectDependencies(stratified)
	volatility := make(map[string]float64)
	stratTypes := make([]string, 0, len(stratified))
	for t := range stratified {
		stratTypes = append(stratTypes, t)
	}
	slices.Sort(stratTypes)

	for _, t := range stratTypes {
		counts := stratified[t]
		volatility[t] = CalculateFatTail(counts)
	}

	// Determine modeling insight (Disclosure)
	insight := "Pooled: Overall process is homogeneous enough for single-stream modeling."
	stratCount := 0
	for _, isEligible := range stratEligible {
		if isEligible {
			stratCount++
		}
	}
	if stratCount > 0 {
		insight = fmt.Sprintf("Stratified: Modeling %d distinct delivery streams independently to capture capacity clashes and variance.", stratCount)
	}

	meta := map[string]any{
		"issues_total":                len(issues),
		"issues_analyzed":             totalDelivered,
		"dropped_by_outcome":          droppedByOutcome,
		"dropped_by_window":           droppedByWindow,
		"days_in_sample":              days,
		"throughput_overall":          avgAcross,
		"throughput_recent":           recentAvg,
		"type_distribution":           typeDist,
		"type_counts":                 typeCounts,
		"type_volatility":             volatility,
		"stratification_decisions":    decisions,
		"stratification_eligible":     stratEligible,
		"stratification_dependencies": dependencies,
		"modeling_insight":            insight,
	}

	return &Histogram{
		Counts:           buckets,
		StratifiedCounts: stratified,
		Meta:             meta,
	}
}

// Resample adjusts the histogram's effective mean throughput by duplicating or
// removing days. factor > 1.0 duplicates random days (biased toward higher counts)
// to scale up; factor < 1.0 removes random days to scale down. This preserves
// integer counts and distribution shape better than multiplying and rounding.
// StratifiedCounts are not modified — only the pooled Counts used by the engine.
func (h *Histogram) Resample(factor float64, rng *rand.Rand) {
	n := len(h.Counts)
	if n == 0 || factor == 1.0 {
		return
	}

	if factor > 1.0 {
		// Scale up: duplicate random days, biased toward higher-count days
		additions := int(float64(n) * (factor - 1.0))
		if additions == 0 {
			return
		}
		// Build weighted index: days with higher counts are more likely to be picked
		weights := make([]float64, n)
		var totalWeight float64
		for i, c := range h.Counts {
			w := float64(c) + 1.0 // +1 so zero-throughput days still have a small chance
			weights[i] = w
			totalWeight += w
		}
		for range additions {
			r := rng.Float64() * totalWeight
			var cumulative float64
			for i, w := range weights {
				cumulative += w
				if r <= cumulative {
					h.Counts = append(h.Counts, h.Counts[i])
					break
				}
			}
		}
	} else {
		// Scale down: remove random days
		removals := int(float64(n) * (1.0 - factor))
		if removals >= n {
			removals = n - 1 // keep at least one day
		}
		if removals == 0 {
			return
		}
		for range removals {
			if len(h.Counts) <= 1 {
				break
			}
			idx := rng.IntN(len(h.Counts))
			h.Counts[idx] = h.Counts[len(h.Counts)-1]
			h.Counts = h.Counts[:len(h.Counts)-1]
		}
	}
}
