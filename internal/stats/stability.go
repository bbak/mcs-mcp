package stats

import (
	"math"
	"mcs-mcp/internal/jira"
)

// XmRResult represents the output of a Process Behavior Chart analysis.
type XmRResult struct {
	Average     float64   `json:"average"`
	AmR         float64   `json:"average_moving_range"`
	UNPL        float64   `json:"upper_natural_process_limit"`
	LNPL        float64   `json:"lower_natural_process_limit"`
	Values      []float64 `json:"values,omitempty"`
	MovingRange []float64 `json:"moving_ranges,omitempty"`
	Signals     []Signal  `json:"signals"`
}

// Round2 rounds a float64 to 2 decimal places for output compactness.
// If the original value is positive, the result is floored to 0.01 to avoid
// misleading "0.00" output (same rationale as the Zero-Day Safeguard in §8.7).
func Round2(v float64) float64 {
	r := math.Round(v*100) / 100
	if r == 0 && v > 0 {
		return 0.01
	}
	return r
}

// Round rounds all numeric fields to 2 decimal places for output compactness.
// Call at the handler/output boundary — never inside math internals.
func (r *XmRResult) Round() {
	r.Average = Round2(r.Average)
	r.AmR = Round2(r.AmR)
	r.UNPL = Round2(r.UNPL)
	r.LNPL = Round2(r.LNPL)
	for i := range r.Values {
		r.Values[i] = Round2(r.Values[i])
	}
	for i := range r.MovingRange {
		r.MovingRange[i] = Round2(r.MovingRange[i])
	}
}

// Signal represents a detected special cause variation.
type Signal struct {
	Index       int    `json:"index"`
	Key         string `json:"key"`
	Type        string `json:"type"` // "outlier", "shift"
	Description string `json:"description"`
}

// CalculateXmR performs the math for an Individuals and Moving Range chart.
func CalculateXmR(values []float64) XmRResult {
	return CalculateXmRWithKeys(values, nil)
}

// CalculateXmRWithKeys performs the math for an Individuals and Moving Range chart and binds keys to signals.
func CalculateXmRWithKeys(values []float64, keys []string) XmRResult {
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
	result.Signals = detectSignals(values, result.Average, result.UNPL, result.LNPL, keys)

	return result
}

// StabilityResult represents the integrated view of process health for a specific set of data.
type StabilityResult struct {
	XmR              XmRResult `json:"xmr"`
	StabilityIndex   float64   `json:"stability_index"`    // Ratio: Expected Lead Time / Avg Cycle Time
	ExpectedLeadTime float64   `json:"expected_lead_time"` // Days: WIP / Throughput
	Signals          []Signal  `json:"signals"`
}

// Round rounds all numeric fields (including the embedded XmR) to 2 decimal places for output compactness.
func (r *StabilityResult) Round() {
	r.XmR.Round()
	r.StabilityIndex = Round2(r.StabilityIndex)
	r.ExpectedLeadTime = Round2(r.ExpectedLeadTime)
}

// LittlesLawIndex computes the stability index: currentWIP / (throughput × avgCycleTime).
// A ratio > 1.3 indicates a clogged system; < 0.7 indicates a starving system.
// Returns 0 if inputs are invalid (zero throughput or avgCycleTime).
func LittlesLawIndex(currentWIP int, throughput, avgCycleTime float64) float64 {
	if currentWIP < 0 || throughput <= 0 || avgCycleTime <= 0 {
		return 0
	}
	return math.Round(float64(currentWIP)/(throughput*avgCycleTime)*100) / 100
}

// CalculateProcessStability evaluates the system's predictability using cycle times and WIP.
func CalculateProcessStability(issues []jira.Issue, cycleTimes []float64, wipCount int, activeDays float64) StabilityResult {
	// Prepare keys for signal traceability
	keys := make([]string, len(issues))
	for i, iss := range issues {
		keys[i] = iss.Key
	}

	xmr := CalculateXmRWithKeys(cycleTimes, keys)

	stabilityIndex := 0.0
	expectedLeadTime := 0.0
	if len(cycleTimes) > 0 && activeDays > 0 {
		throughput := float64(len(cycleTimes)) / activeDays
		if throughput > 0 {
			expectedLeadTime = float64(wipCount) / throughput
			stabilityIndex = LittlesLawIndex(wipCount, throughput, xmr.Average)
		}
	}

	return StabilityResult{
		XmR:              xmr,
		StabilityIndex:   stabilityIndex,
		ExpectedLeadTime: math.Round(expectedLeadTime*10) / 10,
		Signals:          xmr.Signals,
	}
}

// CalculateStratifiedStability performs stability analysis breakdown by work item type.
func CalculateStratifiedStability(issuesByType map[string][]jira.Issue, ctByType map[string][]float64, wipByType map[string][]float64, activeDays float64) map[string]StabilityResult {
	stratified := make(map[string]StabilityResult)
	for t, issues := range issuesByType {
		cts := ctByType[t]
		if len(cts) == 0 {
			continue
		}

		wipCount := len(wipByType[t])
		stratified[t] = CalculateProcessStability(issues, cts, wipCount, activeDays)
	}
	return stratified
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

// Round rounds all numeric fields to 2 decimal places for output compactness.
func (r *ThreeWayResult) Round() {
	r.AverageChart.Round()
	for i := range r.Subgroups {
		r.Subgroups[i].Average = Round2(r.Subgroups[i].Average)
		for j := range r.Subgroups[i].Values {
			r.Subgroups[i].Values[j] = Round2(r.Subgroups[i].Values[j])
		}
	}
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

// GroupIssuesByBucket organizes issues into temporal buckets for subgroup analysis (XmR/Tactical Audit).
func GroupIssuesByBucket(issues []jira.Issue, cycleTimes []float64, window AnalysisWindow) []SubgroupStats {
	if len(issues) == 0 || len(cycleTimes) == 0 {
		return nil
	}

	groups := make(map[string]*SubgroupStats)

	for i, issue := range issues {
		if i >= len(cycleTimes) || issue.ResolutionDate == nil {
			continue
		}

		// EXCLUSION: If the bucket is partial (includes 'Now'), we exclude it
		// to avoid noise from incomplete data (The "Tuesday Problem").
		if window.IsPartial(*issue.ResolutionDate) {
			continue
		}

		bucketKey := window.GenerateLabel(*issue.ResolutionDate)

		if _, ok := groups[bucketKey]; !ok {
			groups[bucketKey] = &SubgroupStats{Label: bucketKey}
		}

		groups[bucketKey].Values = append(groups[bucketKey].Values, cycleTimes[i])
	}

	// Sort keys? Labels like "2024-W01" and "Jan 2024" sort reasonably well,
	// but for robustness we should use the chronological subdivision.
	var result []SubgroupStats
	for _, bucketStart := range window.Subdivide() {
		label := window.GenerateLabel(bucketStart)
		if g, ok := groups[label]; ok {
			sum := 0.0
			for _, v := range g.Values {
				sum += v
			}
			g.Average = sum / float64(len(g.Values))
			result = append(result, *g)
		}
	}

	return result
}

// ScatterPoint represents a single chart-ready data point for Cycle Time Scatterplot visualization.
type ScatterPoint struct {
	Date        string   `json:"date"`
	Value       float64  `json:"value"`
	MovingRange *float64 `json:"moving_range"` // nil for the first point
	Key         string   `json:"key"`
	IssueType   string   `json:"issue_type"`
}

// BuildScatterplot constructs a chart-ready scatterplot array from delivered issues and their cycle times.
// Values and moving ranges are rounded to 2 decimal places for payload compactness.
// The moving range is pooled (consecutive items regardless of type).
func BuildScatterplot(issues []jira.Issue, cycleTimes []float64) []ScatterPoint {
	if len(issues) == 0 || len(cycleTimes) == 0 {
		return nil
	}

	n := len(issues)
	if len(cycleTimes) < n {
		n = len(cycleTimes)
	}

	points := make([]ScatterPoint, 0, n)
	for i := 0; i < n; i++ {
		iss := issues[i]
		if iss.OutcomeDate == nil {
			continue
		}

		p := ScatterPoint{
			Date:      iss.OutcomeDate.Format("2006-01-02"),
			Value:     Round2(cycleTimes[i]),
			Key:       iss.Key,
			IssueType: iss.IssueType,
		}

		if len(points) > 0 {
			mr := Round2(math.Abs(cycleTimes[i] - cycleTimes[i-1]))
			p.MovingRange = &mr
		}

		points = append(points, p)
	}

	return points
}

// SystemPressureResult represents the current impediment stress on the system.
type SystemPressureResult struct {
	TotalWIP      int     `json:"total_wip"`
	FlaggedCount  int     `json:"flagged_count"`
	PressureRatio float64 `json:"pressure_ratio"` // Flagged / WIP
}

// CalculateSystemPressure quantifies the systemic impediment stress based on current WIP.
func CalculateSystemPressure(activeIssues []jira.Issue) SystemPressureResult {
	if len(activeIssues) == 0 {
		return SystemPressureResult{}
	}

	flagged := 0
	for _, iss := range activeIssues {
		if iss.Flagged != "" {
			flagged++
		}
	}

	result := SystemPressureResult{
		TotalWIP:     len(activeIssues),
		FlaggedCount: flagged,
	}

	if result.TotalWIP > 0 {
		result.PressureRatio = math.Round((float64(flagged)/float64(result.TotalWIP))*100) / 100
	}

	return result
}

func detectSignals(values []float64, avg, unpl, lnpl float64, keys []string) []Signal {
	var signals []Signal

	for i, v := range values {
		key := ""
		if i < len(keys) {
			key = keys[i]
		}

		if v > unpl {
			signals = append(signals, Signal{
				Index:       i,
				Key:         key,
				Type:        "outlier",
				Description: "Point above Upper Natural Process Limit (UNPL)",
			})
		} else if v < lnpl {
			signals = append(signals, Signal{
				Index:       i,
				Key:         key,
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
				key := ""
				if i < len(keys) {
					key = keys[i]
				}
				signals = append(signals, Signal{
					Index:       i,
					Key:         key,
					Type:        "shift",
					Description: "8 consecutive points on one side of the average identified (Process Shift)",
				})
			}
		}
	}

	return signals
}

// AnalyzeThroughputStability assesses whether the weekly/monthly delivery cadence is predictable.
// It applies Wheeler's XmR methodology to the raw throughput counts.
// Note: 0-count weeks are valid data points in throughput analysis.
func AnalyzeThroughputStability(throughput StratifiedThroughput) *XmRResult {
	if len(throughput.Pooled) == 0 {
		return nil
	}

	values := make([]float64, len(throughput.Pooled))
	for i, count := range throughput.Pooled {
		values[i] = float64(count)
	}

	res := CalculateXmR(values)
	return &res
}
