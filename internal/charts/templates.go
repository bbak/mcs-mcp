package charts

// toolTemplates maps MCP tool names to their JSX template filenames.
var toolTemplates = map[string]string{
	"analyze_throughput":          "throughput.jsx",
	"analyze_cycle_time":          "cycle_time.jsx",
	"analyze_status_persistence":  "status_persistence.jsx",
	"analyze_work_item_age":       "work_item_age.jsx",
	"analyze_process_stability":   "process_stability.jsx",
	"analyze_process_evolution":   "process_evolution.jsx",
	"analyze_wip_stability":       "wip_stability.jsx",
	"analyze_wip_age_stability":   "wip_age_stability.jsx",
	"analyze_flow_debt":           "flow_debt.jsx",
	"analyze_yield":               "yield.jsx",
	"analyze_residence_time":      "residence_time.jsx",
	"generate_cfd_data":           "cfd.jsx",
	"forecast_monte_carlo":        "monte_carlo.jsx",
	"forecast_backtest":           "backtest.jsx",
}

// toolTitles maps MCP tool names to human-readable chart page titles.
var toolTitles = map[string]string{
	"analyze_throughput":          "Throughput Analysis",
	"analyze_cycle_time":          "Cycle Time Analysis",
	"analyze_status_persistence":  "Status Persistence Analysis",
	"analyze_work_item_age":       "Work Item Age Analysis",
	"analyze_process_stability":   "Process Stability Analysis",
	"analyze_process_evolution":   "Process Evolution Analysis",
	"analyze_wip_stability":       "WIP Stability Analysis",
	"analyze_wip_age_stability":   "WIP Age Stability Analysis",
	"analyze_flow_debt":           "Flow Debt Analysis",
	"analyze_yield":               "Process Yield Analysis",
	"analyze_residence_time":      "Residence Time Analysis",
	"generate_cfd_data":           "Cumulative Flow Diagram",
	"forecast_monte_carlo":        "Monte Carlo Forecast",
	"forecast_backtest":           "Forecast Backtest",
}

// HasTemplate reports whether the given tool has a chart template.
func HasTemplate(toolName string) bool {
	_, ok := toolTemplates[toolName]
	return ok
}

// Title returns the human-readable page title for the given tool's chart.
// Falls back to "MCS Chart" if no specific title is defined.
func Title(toolName string) string {
	if t, ok := toolTitles[toolName]; ok {
		return t
	}
	return "MCS Chart"
}
