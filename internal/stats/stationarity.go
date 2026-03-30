package stats

import (
	"fmt"
	"math"
)

// StationarityAssessment summarizes residence time signals relevant to
// forecast validity. Produced by stats, consumed by the MCP handler to
// inject warnings into Monte Carlo simulation results.
type StationarityAssessment struct {
	Convergence       string   `json:"convergence"`
	LambdaThetaRatio  float64  `json:"lambda_theta_ratio"`
	CoherenceGapRatio float64  `json:"coherence_gap_ratio"`
	Stationary        bool     `json:"stationary"`
	Warnings          []string `json:"-"`

	// Phase 2: advisory window recommendation.
	RecommendedWindowDays *int   `json:"recommended_window_days,omitempty"`
	WindowRationale       string `json:"window_rationale,omitempty"`
}

// Stationarity thresholds.
const (
	// flowImbalanceThreshold triggers a warning when Λ/Θ exceeds this value
	// (arrivals outpacing departures by 30%+).
	flowImbalanceThreshold = 1.3

	// coherenceGapThreshold triggers a warning when |CoherenceGap|/W* exceeds
	// this fraction (active WIP aging 50%+ beyond completed sojourn time).
	coherenceGapThreshold = 0.5

	// minRecommendedWindowDays is the floor for window recommendations.
	minRecommendedWindowDays = 30
)

// AssessStationarity inspects a ResidenceTimeResult for signals that the
// process is non-stationary, which would violate the implicit assumption
// behind uniform-random throughput sampling in Monte Carlo simulation.
func AssessStationarity(result *ResidenceTimeResult) *StationarityAssessment {
	if result == nil {
		return &StationarityAssessment{Stationary: true}
	}

	s := result.Summary
	a := &StationarityAssessment{
		Convergence: s.Convergence,
		Stationary:  true,
	}

	// 1. Diverging process: w(T) is still climbing.
	if s.Convergence == "diverging" {
		a.Stationary = false
		a.Warnings = append(a.Warnings,
			"CAUTION: Residence time is DIVERGING — average item age is increasing over time. "+
				"MCS assumes stationarity and may be optimistic.")
	}

	// 2. Flow imbalance: arrivals outpacing departures.
	if s.FinalTheta > 0 {
		a.LambdaThetaRatio = Round2(s.FinalLambda / s.FinalTheta)
		if a.LambdaThetaRatio > flowImbalanceThreshold {
			pct := Round2((a.LambdaThetaRatio - 1.0) * 100)
			a.Stationary = false
			a.Warnings = append(a.Warnings, fmt.Sprintf(
				"CAUTION: Arrival rate (Λ=%.2f/day) exceeds departure rate (Θ=%.2f/day) by %.0f%%. "+
					"WIP is accumulating; future throughput may be lower than historical samples suggest.",
				s.FinalLambda, s.FinalTheta, pct))
		}
	}

	// 3. Aging WIP: coherence gap large relative to completed sojourn time.
	if s.FinalWStar > 0 {
		a.CoherenceGapRatio = Round2(math.Abs(s.FinalCoherenceGap) / s.FinalWStar)
		if a.CoherenceGapRatio > coherenceGapThreshold {
			a.Stationary = false
			a.Warnings = append(a.Warnings, fmt.Sprintf(
				"CAUTION: Active WIP is aging significantly beyond completed items "+
					"(coherence gap ratio: %.2f). Harder or stalled items may remain.",
				a.CoherenceGapRatio))
		}
	}

	// Phase 2: window recommendation when non-stationary.
	if !a.Stationary && len(result.Series) >= 8 {
		tailStart := len(result.Series) - len(result.Series)/4
		inflectionDate := result.Series[tailStart].Date
		daysFromInflection := CalendarDaysBetween(inflectionDate, s.WindowEnd)

		if daysFromInflection < minRecommendedWindowDays {
			daysFromInflection = minRecommendedWindowDays
		}
		a.RecommendedWindowDays = &daysFromInflection
		a.WindowRationale = fmt.Sprintf(
			"Process divergence detected in the final quarter of the observation window (from %s). "+
				"Earlier data may not reflect current throughput.",
			inflectionDate.Format(DateFormat))
	}

	return a
}
