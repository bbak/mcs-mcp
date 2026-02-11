package simulation

import (
	"math"
	"mcs-mcp/internal/jira"
	"mcs-mcp/internal/stats"
	"sort"
)

// StratificationDecision holds the result of the eligibility check for a specific type.
type StratificationDecision struct {
	Type           string  `json:"type"`
	Eligible       bool    `json:"eligible"`
	Reason         string  `json:"reason"`
	Volume         int     `json:"volume"`
	P85CycleTime   float64 `json:"p85_cycle_time"`
	DistanceToPool float64 `json:"distance_to_pool"` // % difference in P85
}

// AssessStratificationNeeds analyzes a set of delivered issues to decide which types should be stratified.
func AssessStratificationNeeds(issues []jira.Issue, resolutions map[string]string, mappings map[string]stats.StatusMetadata) []StratificationDecision {
	if len(issues) == 0 {
		return nil
	}

	// 1. Group by type and calculate pooled P85
	typeGroups := make(map[string][]float64)
	var allCycleTimes []float64

	for _, iss := range issues {
		if !stats.IsDelivered(iss, resolutions, mappings) {
			continue
		}

		// Calculate Cycle Time (Resolution - Created as current best heuristic for eligibility)
		resDate := iss.Updated
		if iss.ResolutionDate != nil {
			resDate = *iss.ResolutionDate
		}
		ct := resDate.Sub(iss.Created).Hours() / 24.0

		typeGroups[iss.IssueType] = append(typeGroups[iss.IssueType], ct)
		allCycleTimes = append(allCycleTimes, ct)
	}

	if len(allCycleTimes) == 0 {
		return nil
	}

	pooledP85 := calculateP85(allCycleTimes)
	decisions := make([]StratificationDecision, 0)

	// 2. Evaluate each type
	for t, cts := range typeGroups {
		decision := StratificationDecision{
			Type:   t,
			Volume: len(cts),
		}

		decision.P85CycleTime = calculateP85(cts)

		// Distance to pool
		if pooledP85 > 0 {
			decision.DistanceToPool = (decision.P85CycleTime - pooledP85) / pooledP85
		}

		// Eligibility Criteria
		eligible := true
		reason := "Meets volume and variance criteria"

		if decision.Volume < 15 {
			eligible = false
			reason = "Volume too low (< 15 items)"
		} else if decision.DistanceToPool > -0.15 && decision.DistanceToPool < 0.15 {
			// If variance is less than 15%, it's close enough to the average to be pooled
			eligible = false
			reason = "Insufficient variance from pooled average (< 15%)"
		}

		decision.Eligible = eligible
		decision.Reason = reason
		decisions = append(decisions, decision)
	}

	return decisions
}

func calculateP85(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sort.Float64s(values)
	idx := int(float64(len(values)) * 0.85)
	if idx >= len(values) {
		idx = len(values) - 1
	}
	return values[idx]
}

// CalculateCorrelation calculates Pearson correlation between two daily throughput series.
func CalculateCorrelation(a, b []int) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	n := float64(len(a))
	sumA, sumB := 0.0, 0.0
	sumA2, sumB2 := 0.0, 0.0
	sumAB := 0.0

	for i := 0; i < len(a); i++ {
		valA := float64(a[i])
		valB := float64(b[i])
		sumA += valA
		sumB += valB
		sumA2 += valA * valA
		sumB2 += valB * valB
		sumAB += valA * valB
	}

	num := (n * sumAB) - (sumA * sumB)
	den := math.Sqrt((n*sumA2 - sumA*sumA) * (n*sumB2 - sumB*sumB))

	if den == 0 {
		return 0
	}

	return num / den
}

// DetectDependencies finds pairs with significant negative correlation (the "Tax").
// Returns a map where key 'Taxes' value (key=Taxer, value=Taxed).
func DetectDependencies(stratified map[string][]int) map[string]string {
	deps := make(map[string]string)
	types := make([]string, 0, len(stratified))
	for t := range stratified {
		types = append(types, t)
	}

	for i := 0; i < len(types); i++ {
		for j := i + 1; j < len(types); j++ {
			t1, t2 := types[i], types[j]
			corr := CalculateCorrelation(stratified[t1], stratified[t2])

			// Significant negative correlation indicates a capacity clash/dependency
			if corr < -0.6 {
				// We identify the "Taxer" as the one with higher volume/influence?
				// For now, we just pick t1 as taxer of t2 if t1 is conventionally "higher priority" or more bursty.
				// Actually, the relationship is mutual capacity drain, but usually one type (Bugs)
				// is perceived as the tax.
				deps[t1] = t2
			}
		}
	}
	return deps
}

// CalculateFatTail calculates the P98/P50 ratio for a stratum's throughput.
func CalculateFatTail(counts []int) float64 {
	if len(counts) == 0 {
		return 0
	}
	floats := make([]float64, len(counts))
	for i, c := range counts {
		floats[i] = float64(c)
	}
	sort.Float64s(floats)

	p50 := floats[int(float64(len(floats))*0.50)]
	p98 := floats[int(float64(len(floats))*0.98)]

	if p50 == 0 {
		if p98 > 0 {
			return 10.0 // Symbolic high value for sparse processes
		}
		return 1.0
	}
	return p98 / p50
}
