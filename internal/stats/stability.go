package stats

import (
	"math"
	"mcs-mcp/internal/jira"
	"sort"
	"time"
)

// XmRResult represents the output of a Process Behavior Chart analysis.
type XmRResult struct {
	Average     float64   `json:"average"`
	AmR         float64   `json:"average_moving_range"`
	UNPL        float64   `json:"upper_natural_process_limit"`
	LNPL        float64   `json:"lower_natural_process_limit"`
	Values      []float64 `json:"values"`
	MovingRange []float64 `json:"moving_ranges"`
	Signals     []Signal  `json:"signals"`
}

// Signal represents a detected special cause variation.
type Signal struct {
	Index       int    `json:"index"`
	Type        string `json:"type"` // "outlier", "shift"
	Description string `json:"description"`
}

// CalculateXmR performs the math for an Individuals and Moving Range chart.
func CalculateXmR(values []float64) XmRResult {
	if len(values) == 0 {
		return XmRResult{}
	}

	result := XmRResult{
		Values: values,
	}

	// 1. Calculate Average
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	result.Average = sum / float64(len(values))

	// 2. Calculate Moving Ranges
	if len(values) > 1 {
		mrSum := 0.0
		result.MovingRange = make([]float64, len(values)-1)
		for i := 0; i < len(values)-1; i++ {
			mr := math.Abs(values[i+1] - values[i])
			result.MovingRange[i] = mr
			mrSum += mr
		}
		result.AmR = mrSum / float64(len(values)-1)
	}

	// 3. Calculate Limits (Wheeler's scaling constant for Individuals is 2.66)
	result.UNPL = result.Average + (2.66 * result.AmR)
	result.LNPL = math.Max(0, result.Average-(2.66*result.AmR))

	// 4. Detect Signals
	result.Signals = detectSignals(values, result.Average, result.UNPL, result.LNPL)

	return result
}

// CalculateProcessStability evaluates the system's predictability using cycle times and WIP.
func CalculateProcessStability(issues []jira.Issue, cycleTimes []float64, wipCount int) interface{} {
	xmr := CalculateXmR(cycleTimes)

	stabilityIndex := 0.0
	if len(cycleTimes) > 0 {
		avgCT := xmr.Average
		if avgCT > 0 {
			stabilityIndex = float64(wipCount) / (float64(len(cycleTimes)) / math.Max(1.0, cycleTimes[len(cycleTimes)-1]))
		}
	}

	return map[string]interface{}{
		"xmr":             xmr,
		"stability_index": stabilityIndex,
		"status":          xmr.Signals,
	}
}

// TimeStabilityResult represents the integrated view of done items vs current WIP.
type TimeStabilityResult struct {
	Baseline   XmRResult `json:"baseline"`
	WIPSignals []Signal  `json:"wip_signals"`
	Status     string    `json:"status"` // "stable", "unstable", "warning"
}

// AnalyzeTimeStability evaluates current WIP ages against a historical cycle time baseline.
func AnalyzeTimeStability(historicalCycleTimes []float64, currentWIPAges []float64) TimeStabilityResult {
	baseline := CalculateXmR(historicalCycleTimes)

	result := TimeStabilityResult{
		Baseline: baseline,
		Status:   "stable",
	}

	if len(baseline.Signals) > 0 {
		result.Status = "unstable"
	}

	// Evaluate WIP against the baseline UNPL
	for i, age := range currentWIPAges {
		if age > baseline.UNPL {
			result.WIPSignals = append(result.WIPSignals, Signal{
				Index:       i,
				Type:        "wip_outlier",
				Description: "Active WIP Age exceeds historical Upper Natural Process Limit (UNPL)",
			})
			if result.Status == "stable" {
				result.Status = "warning"
			}
		}
	}

	return result
}

// SubgroupStats represents the metrics for a single batch of data (e.g., a Month or Sprint).
type SubgroupStats struct {
	Label   string    `json:"label"`
	Average float64   `json:"average"`
	Values  []float64 `json:"values"`
}

// ThreeWayResult represents a Three-Way Process Behavior Chart (System Evolution analysis).
type ThreeWayResult struct {
	Subgroups    []SubgroupStats `json:"subgroups"`
	AverageChart XmRResult       `json:"average_chart"` // The "Third Way": XmR chart of the subgroup averages
	Status       string          `json:"status"`        // "stable", "migrating", "volatile"
}

// CalculateThreeWayXmR implements Wheeler's Three-Way Chart logic to detect process drift.
func CalculateThreeWayXmR(subgroups []SubgroupStats) ThreeWayResult {
	if len(subgroups) == 0 {
		return ThreeWayResult{}
	}

	// 1. Extract the averages to build the "Third Way" chart
	averages := make([]float64, len(subgroups))
	for i, sg := range subgroups {
		averages[i] = sg.Average
	}

	// 2. The Third Way: Calculate XmR on the averages themselves.
	avgChart := CalculateXmR(averages)

	result := ThreeWayResult{
		Subgroups:    subgroups,
		AverageChart: avgChart,
		Status:       "stable",
	}

	// 3. Interpret System Evolution
	shiftCount := 0
	outlierCount := 0
	for _, signal := range avgChart.Signals {
		if signal.Type == "shift" {
			shiftCount++
		}
		if signal.Type == "outlier" {
			outlierCount++
		}
	}

	if shiftCount > 0 {
		result.Status = "migrating"
	} else if outlierCount > 0 {
		result.Status = "volatile"
	}

	return result
}

// GroupIssuesByMonth is a helper to organize issues into monthly buckets for subgroup analysis.
func GroupIssuesByMonth(issues []jira.Issue, cycleTimes []float64) []SubgroupStats {
	if len(issues) == 0 || len(cycleTimes) == 0 {
		return nil
	}

	groups := make(map[string]*SubgroupStats)
	var keys []string

	currentMonth := time.Now().Format("2006-01")

	for i, issue := range issues {
		if i >= len(cycleTimes) || issue.ResolutionDate == nil {
			continue
		}

		monthKey := issue.ResolutionDate.Format("2006-01")

		// EXCLUSION: If the month is still "in progress" (current calendar month),
		// we exclude it from the Strategic Audit to avoid noise from incomplete data.
		if monthKey == currentMonth {
			continue
		}

		if _, ok := groups[monthKey]; !ok {
			groups[monthKey] = &SubgroupStats{Label: monthKey}
			keys = append(keys, monthKey)
		}

		groups[monthKey].Values = append(groups[monthKey].Values, cycleTimes[i])
	}

	// Sort keys chronologically
	sort.Strings(keys)

	var result []SubgroupStats
	for _, k := range keys {
		g := groups[k]
		sum := 0.0
		for _, v := range g.Values {
			sum += v
		}
		g.Average = sum / float64(len(g.Values))
		result = append(result, *g)
	}

	return result
}

func detectSignals(values []float64, avg, unpl, lnpl float64) []Signal {
	var signals []Signal

	for i, v := range values {
		if v > unpl {
			signals = append(signals, Signal{
				Index:       i,
				Type:        "outlier",
				Description: "Point above Upper Natural Process Limit (UNPL)",
			})
		} else if v < lnpl {
			signals = append(signals, Signal{
				Index:       i,
				Type:        "outlier",
				Description: "Point below Lower Natural Process Limit (LNPL)",
			})
		}
	}

	if len(values) >= 8 {
		side := 0
		count := 0
		for i, v := range values {
			currentSide := 0
			if v > avg {
				currentSide = 1
			} else if v < avg {
				currentSide = -1
			}

			if currentSide == side && currentSide != 0 {
				count++
			} else {
				side = currentSide
				count = 1
			}

			if count == 8 {
				signals = append(signals, Signal{
					Index:       i,
					Type:        "shift",
					Description: "8 consecutive points on one side of the average identified (Process Shift)",
				})
			}
		}
	}

	return signals
}
