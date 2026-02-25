package visuals

import (
	"fmt"
	"math"
	"strings"

	"mcs-mcp/internal/simulation"
	"mcs-mcp/internal/stats"
)

// GenerateXmRChart creates a Mermaid xychart-beta for Process Stability (Individuals and Moving Range).
func GenerateXmRChart(result stats.StabilityResult) string {
	if len(result.XmR.Values) == 0 {
		return ""
	}

	var labels []string
	var values []string
	var averages []string
	var unpls []string

	for i, v := range result.XmR.Values {
		labels = append(labels, fmt.Sprintf("%d", i+1))
		values = append(values, fmt.Sprintf("%.1f", v))
		averages = append(averages, fmt.Sprintf("%.1f", result.XmR.Average))
		unpls = append(unpls, fmt.Sprintf("%.1f", result.XmR.UNPL))
	}

	var sb strings.Builder
	sb.WriteString("```mermaid\n")
	sb.WriteString("xychart-beta\n")
	sb.WriteString("    title \"Process Behavior (XmR)\"\n")
	sb.WriteString(fmt.Sprintf("    x-axis [%s]\n", strings.Join(labels, ", ")))

	// Dynamically scale Y-axis based on max value to give breathing room above the UNPL
	maxY := result.XmR.UNPL * 1.2
	for _, v := range result.XmR.Values {
		if v > maxY {
			maxY = v * 1.1
		}
	}
	sb.WriteString(fmt.Sprintf("    y-axis \"Cycle Time (Days)\" 0 --> %d\n", int(math.Ceil(maxY))))

	sb.WriteString(fmt.Sprintf("    line [%s]\n", strings.Join(values, ", ")))
	sb.WriteString(fmt.Sprintf("    line [%s]\n", strings.Join(averages, ", ")))
	sb.WriteString(fmt.Sprintf("    line [%s]\n", strings.Join(unpls, ", ")))
	sb.WriteString("```")
	return sb.String()
}

// GenerateThroughputChart creates a Mermaid bar chart for Delivery Cadence over time buckets.
func GenerateThroughputChart(throughput []int, metadata []map[string]string) string {
	if len(throughput) == 0 || len(metadata) == 0 {
		return ""
	}

	var labels []string
	var values []string

	maxVal := 0
	for i, count := range throughput {
		if i < len(metadata) {
			labels = append(labels, fmt.Sprintf("\"%s\"", metadata[i]["label"]))
			values = append(values, fmt.Sprintf("%d", count))
			if count > maxVal {
				maxVal = count
			}
		}
	}

	var sb strings.Builder
	sb.WriteString("```mermaid\n")
	sb.WriteString("xychart-beta\n")
	sb.WriteString("    title \"Delivery Cadence (Throughput)\"\n")
	sb.WriteString(fmt.Sprintf("    x-axis [%s]\n", strings.Join(labels, ", ")))
	sb.WriteString(fmt.Sprintf("    y-axis \"Items Delivered\" 0 --> %d\n", maxVal+int(math.Max(1, float64(maxVal)*0.2))))
	sb.WriteString(fmt.Sprintf("    bar [%s]\n", strings.Join(values, ", ")))
	sb.WriteString("```")
	return sb.String()
}

// GenerateAgingChart creates a Mermaid bar chart showing the age of currently active items.
func GenerateAgingChart(aging []stats.InventoryAge) string {
	if len(aging) == 0 {
		return ""
	}

	var labels []string
	var values []string
	maxVal := 0.0

	// Limit to 20 items to avoid overwhelming the text chart context
	limit := len(aging)
	if limit > 20 {
		limit = 20
	}

	for i := 0; i < limit; i++ {
		item := aging[i]

		val := 0.0
		if item.AgeSinceCommitment != nil {
			val = *item.AgeSinceCommitment
		} else {
			val = item.TotalAgeSinceCreation
		}

		labels = append(labels, fmt.Sprintf("\"%s\"", item.Key))
		values = append(values, fmt.Sprintf("%.1f", val))
		if val > maxVal {
			maxVal = val
		}
	}

	var sb strings.Builder
	sb.WriteString("```mermaid\n")
	sb.WriteString("xychart-beta\n")
	sb.WriteString("    title \"WIP Aging (Top 20 Active Items)\"\n")
	sb.WriteString(fmt.Sprintf("    x-axis [%s]\n", strings.Join(labels, ", ")))
	sb.WriteString(fmt.Sprintf("    y-axis \"Age (Days)\" 0 --> %d\n", int(math.Ceil(maxVal*1.1))))
	sb.WriteString(fmt.Sprintf("    bar [%s]\n", strings.Join(values, ", ")))
	sb.WriteString("```")
	return sb.String()
}

// GenerateEvolutionChart creates a Mermaid Three-Way XmR chart showing drift of subgroup averages.
func GenerateEvolutionChart(result stats.ThreeWayResult) string {
	if len(result.Subgroups) == 0 {
		return ""
	}

	var labels []string
	var values []string
	var averages []string
	var unpls []string
	var lnpls []string

	for _, sg := range result.Subgroups {
		labels = append(labels, fmt.Sprintf("\"%s\"", sg.Label))
		values = append(values, fmt.Sprintf("%.1f", sg.Average))
		averages = append(averages, fmt.Sprintf("%.1f", result.AverageChart.Average))
		unpls = append(unpls, fmt.Sprintf("%.1f", result.AverageChart.UNPL))
		lnpls = append(lnpls, fmt.Sprintf("%.1f", result.AverageChart.LNPL))
	}

	maxY := result.AverageChart.UNPL * 1.2
	for _, sg := range result.Subgroups {
		if sg.Average > maxY {
			maxY = sg.Average * 1.1
		}
	}

	var sb strings.Builder
	sb.WriteString("```mermaid\n")
	sb.WriteString("xychart-beta\n")
	sb.WriteString("    title \"Process Evolution (Subgroup Averages)\"\n")
	sb.WriteString(fmt.Sprintf("    x-axis [%s]\n", strings.Join(labels, ", ")))
	sb.WriteString(fmt.Sprintf("    y-axis \"Avg Cycle Time\" 0 --> %d\n", int(math.Ceil(maxY))))
	sb.WriteString(fmt.Sprintf("    line [%s]\n", strings.Join(values, ", ")))
	sb.WriteString(fmt.Sprintf("    line [%s]\n", strings.Join(averages, ", ")))
	sb.WriteString(fmt.Sprintf("    line [%s]\n", strings.Join(unpls, ", ")))
	sb.WriteString(fmt.Sprintf("    line [%s]\n", strings.Join(lnpls, ", ")))
	sb.WriteString("```")
	return sb.String()
}

// GenerateWIPRunChart creates a Mermaid xychart-beta for Historical WIP bounding.
func GenerateWIPRunChart(result stats.WIPStabilityResult) string {
	if len(result.RunChart) == 0 {
		return ""
	}

	var labels []string
	var values []string
	var unpls []string
	var lnpls []string

	// XmR Process limits are scalar across the entire run chart for visual context
	unpl := fmt.Sprintf("%.1f", result.XmR.UNPL)
	lnpl := fmt.Sprintf("%.1f", result.XmR.LNPL)

	// Subsample points if the chart is too wide for Mermaid's layout engine
	// Typically Mermaid xychart starts overflowing/overlapping text around 60 points
	subsampleRate := 1
	if len(result.RunChart) > 60 {
		subsampleRate = int(math.Ceil(float64(len(result.RunChart)) / 60.0))
	}

	for i, point := range result.RunChart {
		if i%subsampleRate == 0 || i == len(result.RunChart)-1 {
			labels = append(labels, fmt.Sprintf("\"%s\"", point.Date.Format("Jan02")))
			values = append(values, fmt.Sprintf("%d", point.Count))
			unpls = append(unpls, unpl)
			lnpls = append(lnpls, lnpl)
		}
	}

	maxY := result.XmR.UNPL * 1.2
	for _, p := range result.RunChart {
		if float64(p.Count) > maxY {
			maxY = float64(p.Count) * 1.1
		}
	}

	var sb strings.Builder
	sb.WriteString("```mermaid\n")
	sb.WriteString("xychart-beta\n")
	sb.WriteString("    title \"Work-In-Progress (WIP) Stability\"\n")
	sb.WriteString(fmt.Sprintf("    x-axis [%s]\n", strings.Join(labels, ", ")))
	sb.WriteString(fmt.Sprintf("    y-axis \"Active Items\" 0 --> %d\n", int(math.Ceil(maxY))))
	sb.WriteString(fmt.Sprintf("    line [%s]\n", strings.Join(values, ", ")))
	sb.WriteString(fmt.Sprintf("    line [%s]\n", strings.Join(unpls, ", ")))
	sb.WriteString(fmt.Sprintf("    line [%s]\n", strings.Join(lnpls, ", ")))
	sb.WriteString("```")
	return sb.String()
}

// GenerateYieldPie creates a Mermaid Pie chart indicating abandonment points.
func GenerateYieldPie(result stats.ProcessYield) string {
	if result.TotalIngested == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("```mermaid\n")
	sb.WriteString("pie title Process Yield (Flow Efficiency)\n")
	sb.WriteString(fmt.Sprintf("    \"Delivered\" : %d\n", result.DeliveredCount))

	for _, loss := range result.LossPoints {
		sb.WriteString(fmt.Sprintf("    \"Abandoned (%s)\" : %d\n", loss.Tier, loss.Count))
	}
	sb.WriteString("```")
	return sb.String()
}

// GeneratePersistenceChart creates a Mermaid bar chart showing the median residency per status.
func GeneratePersistenceChart(persistence []stats.StatusPersistence) string {
	if len(persistence) == 0 {
		return ""
	}

	var labels []string
	var values []string
	maxVal := 0.0

	for _, stat := range persistence {
		// Replace spaces to help mermaid rendering
		safeName := strings.ReplaceAll(stat.StatusName, " ", "_")
		labels = append(labels, fmt.Sprintf("\"%s\"", safeName))
		values = append(values, fmt.Sprintf("%.1f", stat.P50))
		if stat.P50 > maxVal {
			maxVal = stat.P50
		}
	}

	var sb strings.Builder
	sb.WriteString("```mermaid\n")
	sb.WriteString("xychart-beta\n")
	sb.WriteString("    title \"Status Persistence (Median Days)\"\n")
	sb.WriteString(fmt.Sprintf("    x-axis [%s]\n", strings.Join(labels, ", ")))
	sb.WriteString(fmt.Sprintf("    y-axis \"Median Days\" 0 --> %d\n", int(math.Ceil(maxVal*1.2))))
	sb.WriteString(fmt.Sprintf("    bar [%s]\n", strings.Join(values, ", ")))
	sb.WriteString("```")
	return sb.String()
}

// GenerateSimulationCDF creates a Mermaid bar chart showing the cumulative probability distribution of the forecast.
func GenerateSimulationCDF(percentiles simulation.Percentiles, mode string) string {
	yAxisLabel := "Days (Duration)"
	if mode == "scope" {
		yAxisLabel = "Items Delivered (Scope)"
	}

	labels := []string{
		"\"10% (Aggressive)\"",
		"\"30% (Unlikely)\"",
		"\"50% (Coin Toss)\"",
		"\"70% (Probable)\"",
		"\"85% (Likely)\"",
		"\"90% (Conservative)\"",
		"\"95% (Safe)\"",
		"\"98% (Certain)\"",
	}

	values := []string{
		fmt.Sprintf("%.0f", percentiles.Aggressive),
		fmt.Sprintf("%.0f", percentiles.Unlikely),
		fmt.Sprintf("%.0f", percentiles.CoinToss),
		fmt.Sprintf("%.0f", percentiles.Probable),
		fmt.Sprintf("%.0f", percentiles.Likely),
		fmt.Sprintf("%.0f", percentiles.Conservative),
		fmt.Sprintf("%.0f", percentiles.Safe),
		fmt.Sprintf("%.0f", percentiles.AlmostCertain),
	}

	maxVal := percentiles.AlmostCertain
	if mode == "scope" {
		maxVal = percentiles.Aggressive // In scope, 10% confidence produces the highest hypothetical output
	}

	if maxVal == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("```mermaid\n")
	sb.WriteString("xychart-beta\n")
	sb.WriteString("    title \"Monte Carlo Simulation (Cumulative Probability)\"\n")
	sb.WriteString(fmt.Sprintf("    x-axis [%s]\n", strings.Join(labels, ", ")))
	sb.WriteString(fmt.Sprintf("    y-axis \"%s\" 0 --> %d\n", yAxisLabel, int(math.Ceil(maxVal*1.1))))
	sb.WriteString(fmt.Sprintf("    bar [%s]\n", strings.Join(values, ", ")))
	sb.WriteString("```")
	return sb.String()
}
