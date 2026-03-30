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
	N            int       `json:"n"`             // instantaneous active count N(t)
	H            float64   `json:"h"`             // cumulative element-days H(T)
	L            float64   `json:"l"`             // time-average WIP = H(T) / T_days
	A            int       `json:"a"`             // cumulative arrivals (excludes pre-window)
	Lambda       float64   `json:"lambda"`        // arrival rate = A(T) / T_days
	W            float64   `json:"w"`             // avg residence time per arrival = H(T) / A(T)
	WPrime       float64   `json:"w_prime"`       // avg residence time per departure = H(T) / D(T)
	D            int       `json:"d"`             // cumulative departures
	Theta        float64   `json:"theta"`         // departure rate = D(T) / T_days
	WStar        float64   `json:"w_star"`        // avg sojourn time of completed items
	CoherenceGap float64   `json:"coherence_gap"` // w(T) - W*(T)
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
	FinalTheta        float64   `json:"final_theta"`         // departure rate at end
	FinalW            float64   `json:"final_w"`             // avg residence time per arrival at end
	FinalWPrime       float64   `json:"final_w_prime"`       // avg residence time per departure at end
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

		// Skip items with no commitment boundary crossing — they have zero residence time.
		// This includes items still pre-commitment-point and items imported directly into
		// the Finished tier (sojourn time = 0).
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

		// w'(T) = H(T) / D(T) — avg residence time per departure; guarded
		wPrimeT := 0.0
		if dT > 0 {
			wPrimeT = cumulativeH / float64(dT)
		}

		// Θ(T) = D(T) / T — departure rate
		thetaT := float64(dT) / tDays

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
			L:            RoundTo(lT, 4),
			A:            aT,
			Lambda:       RoundTo(lambdaT, 4),
			W:            RoundTo(wT, 4),
			WPrime:       RoundTo(wPrimeT, 4),
			D:            dT,
			Theta:        RoundTo(thetaT, 4),
			WStar:        RoundTo(wStarT, 4),
			CoherenceGap: RoundTo(coherenceGap, 4),
		}
	}

	// Convergence assessment: trend of coherence gap over final quarter
	ca := assessConvergence(series)

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
		FinalTheta:        last.Theta,
		FinalW:            last.W,
		FinalWPrime:       last.WPrime,
		FinalWStar:        last.WStar,
		FinalCoherenceGap: last.CoherenceGap,
		Convergence:       ca.Label,
	}

	return &ResidenceTimeResult{
		Series:  series,
		Summary: summary,
		Validation: ResidenceTimeValidation{
			IdentityVerified: identityHolds,
			MaxDeviation:     RoundTo(maxDeviation, 10),
		},
	}
}

// convergenceAssessment holds the result of the 1/T tail regression on w(T).
// Unexported: the Label feeds into ResidenceTimeSummary.Convergence;
// Beta1 and RMSE are retained for internal algorithmic use (e.g., comparing
// convergence quality before/after data filtering).
type convergenceAssessment struct {
	Label string  // "converging", "diverging", "metastable", "insufficient_data"
	Beta1 float64 // OLS slope of w(T) ~ β₀ + β₁·(1/T); large positive = diverging
	RMSE  float64 // Root mean squared error of the regression fit
}

// assessConvergence evaluates the trend of w(T) over the tail of the series using
// a 1/T OLS regression. Theory predicts w(T) → w* as T → ∞, so a converging
// series has w(T) ≈ β₀ + β₁·(1/T) with small |β₁| and low residual noise.
//
// Convergence labels:
//   - "converging":        tail fits the 1/T model well and slope is small
//   - "diverging":         slope is large and positive (w(T) still climbing)
//   - "metastable":        tail is noisy but not clearly trending
//   - "insufficient_data": fewer than 8 valid tail buckets
func assessConvergence(series []ResidenceTimeBucket) convergenceAssessment {
	n := len(series)
	if n < 8 {
		return convergenceAssessment{Label: "insufficient_data"}
	}

	// Tail = final quarter, minimum 8 points
	tailLen := n / 4
	if tailLen < 8 {
		tailLen = 8
	}
	tail := series[n-tailLen:]

	// Collect valid tail points where w(T) > 0 (A(T) > 0)
	type point struct {
		x float64 // 1 / bucket_index (1-indexed from start of full series)
		y float64 // w(T)
	}
	var pts []point
	for i, b := range tail {
		if b.W > 0 {
			idx := float64(n - tailLen + i + 1) // 1-indexed position in full series
			pts = append(pts, point{x: 1.0 / idx, y: b.W})
		}
	}
	if len(pts) < 4 {
		return convergenceAssessment{Label: "insufficient_data"}
	}

	// OLS: fit y = β₀ + β₁·x
	np := float64(len(pts))
	var sumX, sumY, sumXX, sumXY float64
	for _, p := range pts {
		sumX += p.x
		sumY += p.y
		sumXX += p.x * p.x
		sumXY += p.x * p.y
	}
	denom := np*sumXX - sumX*sumX
	if denom == 0 {
		return convergenceAssessment{Label: "metastable"}
	}
	beta1 := (np*sumXY - sumX*sumY) / denom
	beta0 := (sumY - beta1*sumX) / np

	// RMSE of residuals
	var ssRes float64
	for _, p := range pts {
		resid := p.y - (beta0 + beta1*p.x)
		ssRes += resid * resid
	}
	rmse := math.Sqrt(ssRes / np)

	// Threshold: 50% of median w(T) in the tail (permissive, catches chaos)
	yVals := make([]float64, len(pts))
	for i, p := range pts {
		yVals[i] = p.y
	}
	medW := CalculateMedianContinuous(yVals)
	threshold := 0.5 * medW
	if threshold == 0 {
		return convergenceAssessment{Label: "metastable"}
	}

	if beta1 > threshold {
		return convergenceAssessment{Label: "diverging", Beta1: beta1, RMSE: rmse}
	}
	if math.Abs(beta1) <= threshold && rmse <= threshold {
		return convergenceAssessment{Label: "converging", Beta1: beta1, RMSE: rmse}
	}
	return convergenceAssessment{Label: "metastable", Beta1: beta1, RMSE: rmse}
}

