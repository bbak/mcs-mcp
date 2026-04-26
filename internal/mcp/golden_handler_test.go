package mcp

import (
	"testing"
)

// TestHandlers_Golden captures the analytical payload of each MCP handler against
// the canonical fixture (simulated_events.jsonl) and compares it to a committed
// baseline. Run with -update to regenerate baselines after intentional changes.
//
// Subtests run sequentially on a shared server (one server = one session),
// mirroring real-world usage. Do NOT add t.Parallel() inside subtests.
func TestHandlers_Golden(t *testing.T) {
	checkFixtureHash(t)

	srv := newGoldenServer(t)

	cases := []struct {
		name string
		run  func() (any, error)
	}{
		{
			"analyze_cycle_time",
			func() (any, error) {
				return srv.handleGetCycleTimeAssessment(testProject, testBoard, "", "", nil, 0, 0)
			},
		},
		{
			"analyze_throughput",
			func() (any, error) {
				return srv.handleGetDeliveryCadence(testProject, testBoard, "week", false)
			},
		},
		{
			"analyze_status_persistence",
			func() (any, error) {
				return srv.handleGetStatusPersistence(testProject, testBoard)
			},
		},
		{
			"analyze_work_item_age",
			func() (any, error) {
				return srv.handleGetAgingAnalysis(testProject, testBoard, "wip", "")
			},
		},
		{
			"analyze_process_stability",
			func() (any, error) {
				return srv.handleGetProcessStability(testProject, testBoard, true)
			},
		},
		{
			"analyze_wip_stability",
			func() (any, error) {
				return srv.handleAnalyzeWIPStability(testProject, testBoard)
			},
		},
		{
			"analyze_wip_age_stability",
			func() (any, error) {
				return srv.handleAnalyzeWIPAgeStability(testProject, testBoard)
			},
		},
		{
			"analyze_process_evolution",
			func() (any, error) {
				return srv.handleGetProcessEvolution(testProject, testBoard, "month")
			},
		},
		{
			"analyze_yield",
			func() (any, error) {
				return srv.handleGetProcessYield(testProject, testBoard)
			},
		},
		{
			"analyze_flow_debt",
			func() (any, error) {
				return srv.handleGetFlowDebt(testProject, testBoard, "week")
			},
		},
		{
			"generate_cfd_data",
			func() (any, error) {
				return srv.handleGetCFDData(testProject, testBoard, "")
			},
		},
		{
			"analyze_residence_time",
			func() (any, error) {
				return srv.handleAnalyzeResidenceTime(testProject, testBoard, nil, "day")
			},
		},
		{
			"forecast_monte_carlo_scope",
			func() (any, error) {
				return srv.handleRunSimulation(
					testProject, testBoard,
					"scope",
					false, 0, 60, "", // targetDays=60
					"", nil, false,
					90, "", "", nil, nil,
				)
			},
		},
		{
			"forecast_monte_carlo_duration",
			func() (any, error) {
				return srv.handleRunSimulation(
					testProject, testBoard,
					"duration",
					true, 0, 0, "", // includeExistingBacklog=true
					"", nil, true, // includeWIP=true
					90, "", "", nil, nil,
				)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := tc.run()
			if err != nil {
				t.Fatalf("%s: handler error: %v", tc.name, err)
			}
			env, ok := res.(ResponseEnvelope)
			if !ok {
				t.Fatalf("%s: expected ResponseEnvelope, got %T", tc.name, res)
			}
			assertGolden(t, tc.name, env)
		})
	}

	if *update {
		writeFixtureHash(t)
	}
}
