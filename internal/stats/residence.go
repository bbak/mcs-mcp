package stats

import (
	"math"
	"mcs-mcp/internal/jira"
	"time"
)

// ResidenceItem represents a single item's temporal footprint for sample path analysis.
type ResidenceItem struct {
	Key       string
	Type      string
	Start     time.Time  // s_i: last commitment date (backflow-reset)
	End       *time.Time // e_i: OutcomeDate, nil if still active
	PreWindow bool       // true if Start < window.Start (excluded from A(T))
}

// ResidenceTimeBucket represents one row (day or week) in the sample path time series.
type ResidenceTimeBucket struct {
	Date         time.Time `json:"date"`
	Label        string    `json:"label"`
	N            int       `json:"n"`              // instantaneous active count N(t)
	H            float64   `json:"h"`              // cumulative element-days H(T)
	L            float64   `json:"l"`              // time-average WIP = H(T) / T_days
	A            int       `json:"a"`              // cumulative arrivals (excludes pre-window)
	Lambda       float64   `json:"lambda"`          // arrival rate = A(T) / T_days
	W            float64   `json:"w"`              // avg residence time = H(T) / A(T)
	D            int       `json:"d"`              // cumulative departures
	WStar        float64   `json:"w_star"`          // avg sojourn time (completed only)
	CoherenceGap float64   `json:"coherence_gap"`   // w(T) - W*(T)
}

// ResidenceTimeSummary provides the final-value snapshot and window metadata.
type ResidenceTimeSummary struct {
	WindowStart       time.Time `json:"window_start"`
	WindowEnd         time.Time `json:"window_end"`
	TotalDays         int       `json:"total_days"`
	TotalItems        int       `json:"total_items"`         // all items (including pre-window)
	InWindowArrivals  int       `json:"in_window_arrivals"`  // A(T) — excludes pre-window
	PreWindowItems    int       `json:"pre_window_items"`    // items committed before window start
	ActiveItems       int       `json:"active_items"`        // items with no end date at window end
	Departures        int       `json:"departures"`          // D(T)
	FinalL            float64   `json:"final_l"`             // time-average WIP at end
	FinalLambda       float64   `json:"final_lambda"`        // arrival rate at end
	FinalW            float64   `json:"final_w"`             // avg residence time at end
	FinalWStar        float64   `json:"final_w_star"`        // avg sojourn time at end
	FinalCoherenceGap float64   `json:"final_coherence_gap"` // w(T) - W*(T) at end
	Convergence       string    `json:"convergence"`         // "converging", "metastable", "diverging"
}

// ResidenceTimeValidation captures the identity check L(T) = Λ(T) · w(T).
type ResidenceTimeValidation struct {
	IdentityVerified bool    `json:"identity_verified"`
	MaxDeviation     float64 `json:"max_deviation"`
}

// ResidenceTimeResult is the top-level response for sample path analysis.
type ResidenceTimeResult struct {
	Series     []ResidenceTimeBucket    `json:"series"`
	Summary    ResidenceTimeSummary     `json:"summary"`
	Validation ResidenceTimeValidation  `json:"validation"`
}

// ExtractResidenceItems walks reconstructed issues to find commitment dates using
// backflow-reset logic: the LAST transition into the commitment point status is used
// as the start anchor (s_i). This always applies backflow reset regardless of the
// server's commitmentBackflowReset configuration.
//
// Items that never reached the commitment point are excluded.
func ExtractResidenceItems(
	issues []jira.Issue,
	commitmentPoint string,
	statusWeights map[string]int,
	mappings map[string]StatusMetadata,
	windowStart time.Time,
) []ResidenceItem {
	if commitmentPoint == "" {
		return nil
	}

	commitmentWeight := 0
	if w, ok := statusWeights[commitmentPoint]; ok {
		commitmentWeight = w
	}

	var items []ResidenceItem

	for _, issue := range issues {
		// Find the LAST transition that crosses the commitment boundary (backflow reset).
		// Walk forwards to find the last time an item entered from below the commitment weight.
		var lastCommitDate *time.Time
		if commitmentWeight > 0 {
			for i := range issue.Transitions {
				t := issue.Transitions[i]
				toID := t.ToStatusID
				if toID == "" {
					toID = t.ToStatus
				}
				toWeight, toOK := statusWeights[toID]
				if !toOK {
					continue
				}

				fromID := t.FromStatusID
				if fromID == "" {
					fromID = t.FromStatus
				}
				fromWeight := 0
				if fw, ok := statusWeights[fromID]; ok {
					fromWeight = fw
				}

				// A commitment entry: moving from below commitment weight to at-or-above
				if fromWeight < commitmentWeight && toWeight >= commitmentWeight {
					d := t.Date
					lastCommitDate = &d
				}
			}
		}

		// Fallback: if no transition found but currently downstream/finished, use Created
		if lastCommitDate == nil {
			tier := DetermineTier(issue, commitmentPoint, mappings)
			if tier == "Downstream" || tier == "Finished" {
				lastCommitDate = &issue.Created
			}
		}

		// Skip items that never reached the commitment point
		if lastCommitDate == nil {
			continue
		}

		items = append(items, ResidenceItem{
			Key:       issue.Key,
			Type:      issue.IssueType,
			Start:     *lastCommitDate,
			End:       issue.OutcomeDate,
			PreWindow: lastCommitDate.Before(windowStart),
		})
	}

	return items
}

// ComputeResidenceTimeSeries builds the daily/weekly sample path and all derived
// quantities implementing the finite Little's Law identity L(T) = Λ(T) · w(T).
func ComputeResidenceTimeSeries(
	items []ResidenceItem,
	window AnalysisWindow,
) *ResidenceTimeResult {
	buckets := window.Subdivide()
	if len(buckets) == 0 {
		return &ResidenceTimeResult{}
	}

	series := make([]ResidenceTimeBucket, len(buckets))
	var cumulativeH float64
	maxDeviation := 0.0
	identityHolds := true
	const epsilon = 1e-9

	for idx, bucketDate := range buckets {
		dayEnd := SnapToEnd(bucketDate, window.Bucket)

		// N(t): count items active on this day
		n := 0
		for _, item := range items {
			// Item is active if: started <= dayEnd AND (not ended OR ended after dayEnd)
			if !item.Start.After(dayEnd) && (item.End == nil || item.End.After(dayEnd)) {
				n++
			}
		}

		// H(T): cumulative element-days
		cumulativeH += float64(n)
		tDays := float64(idx + 1) // days elapsed (1-indexed)

		// L(T) = H(T) / T
		lT := cumulativeH / tDays

		// A(T): cumulative arrivals within window (excludes pre-window items)
		aT := 0
		for _, item := range items {
			if !item.PreWindow && !item.Start.After(dayEnd) {
				aT++
			}
		}

		// Λ(T) = A(T) / T
		lambdaT := float64(aT) / tDays

		// w(T) = H(T) / A(T) — guarded
		wT := 0.0
		if aT > 0 {
			wT = cumulativeH / float64(aT)
		}

		// D(T): cumulative departures
		dT := 0
		var sojournSum float64
		for _, item := range items {
			if item.End != nil && !item.End.After(dayEnd) {
				dT++
				sojourn := item.End.Sub(item.Start).Hours() / 24.0
				sojournSum += sojourn
			}
		}

		// W*(T): average sojourn time of completed items
		wStarT := 0.0
		if dT > 0 {
			wStarT = sojournSum / float64(dT)
		}

		// Coherence gap
		coherenceGap := 0.0
		if aT > 0 && dT > 0 {
			coherenceGap = wT - wStarT
		}

		// Identity verification: |L(T) - Λ(T) · w(T)| < ε
		if aT > 0 {
			deviation := math.Abs(lT - lambdaT*wT)
			if deviation > maxDeviation {
				maxDeviation = deviation
			}
			if deviation > epsilon {
				identityHolds = false
			}
		}

		series[idx] = ResidenceTimeBucket{
			Date:         bucketDate,
			Label:        window.GenerateLabel(bucketDate),
			N:            n,
			H:            cumulativeH,
			L:            roundTo(lT, 4),
			A:            aT,
			Lambda:       roundTo(lambdaT, 4),
			W:            roundTo(wT, 4),
			D:            dT,
			WStar:        roundTo(wStarT, 4),
			CoherenceGap: roundTo(coherenceGap, 4),
		}
	}

	// Convergence assessment: trend of coherence gap over final quarter
	convergence := assessConvergence(series)

	// Count categories
	preWindowCount := 0
	activeCount := 0
	for _, item := range items {
		if item.PreWindow {
			preWindowCount++
		}
		if item.End == nil {
			activeCount++
		}
	}

	last := series[len(series)-1]
	summary := ResidenceTimeSummary{
		WindowStart:       window.Start,
		WindowEnd:         window.End,
		TotalDays:         len(buckets),
		TotalItems:        len(items),
		InWindowArrivals:  last.A,
		PreWindowItems:    preWindowCount,
		ActiveItems:       activeCount,
		Departures:        last.D,
		FinalL:            last.L,
		FinalLambda:       last.Lambda,
		FinalW:            last.W,
		FinalWStar:        last.WStar,
		FinalCoherenceGap: last.CoherenceGap,
		Convergence:       convergence,
	}

	return &ResidenceTimeResult{
		Series:  series,
		Summary: summary,
		Validation: ResidenceTimeValidation{
			IdentityVerified: identityHolds,
			MaxDeviation:     roundTo(maxDeviation, 10),
		},
	}
}

// assessConvergence evaluates the trend of the coherence gap over the final quarter
// of the series to determine if the system is converging, metastable, or diverging.
func assessConvergence(series []ResidenceTimeBucket) string {
	n := len(series)
	if n < 8 {
		return "insufficient_data"
	}

	quarterStart := n - n/4
	if quarterStart >= n-1 {
		quarterStart = n - 2
	}

	// Compare first and last coherence gaps in the final quarter
	first := series[quarterStart].CoherenceGap
	last := series[n-1].CoherenceGap

	// Calculate the trend: positive means gap is growing (diverging)
	delta := last - first

	// Use a relative threshold based on the magnitude
	threshold := 0.5 // days
	if math.Abs(first) > 1 {
		threshold = math.Abs(first) * 0.1
	}

	if delta < -threshold {
		return "converging"
	}
	if delta > threshold {
		return "diverging"
	}
	return "metastable"
}

func roundTo(val float64, places int) float64 {
	pow := math.Pow(10, float64(places))
	return math.Round(val*pow) / pow
}
