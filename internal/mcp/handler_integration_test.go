package mcp

import (
	"fmt"
	"mcs-mcp/cmd/mockgen/engine"
	"mcs-mcp/internal/config"
	"mcs-mcp/internal/jira"
	"mcs-mcp/internal/simulation"
	"mcs-mcp/internal/stats"
	"testing"
	"time"
)

type DummyClient struct{}

func (d *DummyClient) SearchIssues(jql string, startAt int, maxResults int) (*jira.SearchResponse, error) {
	return nil, nil
}
func (d *DummyClient) SearchIssuesWithHistory(jql string, startAt int, maxResults int) (*jira.SearchResponse, error) {
	return nil, nil
}
func (d *DummyClient) GetIssueWithHistory(key string) (*jira.IssueDTO, error) { return nil, nil }
func (d *DummyClient) GetProject(key string) (any, error)                     { return nil, nil }
func (d *DummyClient) GetProjectStatuses(key string) (any, error)             { return nil, nil }
func (d *DummyClient) GetBoard(id int) (any, error)                           { return nil, nil }
func (d *DummyClient) GetBoardConfig(id int) (any, error)                     { return nil, nil }
func (d *DummyClient) GetFilter(id string) (any, error)                       { return nil, nil }
func (d *DummyClient) FindProjects(query string) ([]any, error)               { return nil, nil }
func (d *DummyClient) FindBoards(pKey string, nFilter string) ([]any, error)  { return nil, nil }

func TestMCSTEST_Integration(t *testing.T) {
	dists := []string{"uniform", "weibull"}
	scenarios := []string{"mild", "chaos", "drift"}

	for _, dist := range dists {
		for _, scen := range scenarios {
			t.Run(fmt.Sprintf("%s_%s", dist, scen), func(t *testing.T) {
				cacheDir := t.TempDir()
				sourceID := "MCSTEST_0"
				generateMockData(scen, dist, 300, cacheDir, sourceID)

				server := NewServer(&config.AppConfig{
					CacheDir: cacheDir,
				}, &DummyClient{})

				// 1. Verify Board Details
				res, err := server.handleGetBoardDetails("MCSTEST", 0)
				if err != nil {
					t.Fatalf("Failed to get board details: %v", err)
				}
				env := res.(ResponseEnvelope)
				boardRes := env.Data.(map[string]any)
				summary := boardRes["data_summary"].(stats.MetadataSummary)

				if summary.Whole.TotalItems < 200 {
					t.Errorf("Expected at least 200 items, got %v", summary.Whole.TotalItems)
				}

				// 2. Verify Status Persistence
				pRes, err := server.handleGetStatusPersistence("MCSTEST", 0)
				if err != nil {
					t.Fatalf("Failed to get status persistence: %v", err)
				}

				pEnv := pRes.(ResponseEnvelope)
				persistenceMap := pEnv.Data.(map[string]any)
				persistence := persistenceMap["persistence"].([]stats.StatusPersistence)

				var inProgressFound bool
				for _, st := range persistence {
					if st.StatusName == "In Progress" {
						inProgressFound = true
						p50 := st.P50
						p85 := st.P85

						t.Logf("[%s/%s] In Progress: P50=%.2f, P85=%.2f", dist, scen, p50, p85)

						// Thresholds for Uniform and Weibull are slightly different but share similar bounds
						switch scen {
						case "mild":
							if p50 < 4.5 || p50 > 6.0 {
								t.Errorf("Mild P50 out of expected range (4.5-6.0): %.2f", p50)
							}
						case "chaos":
							if p85 < 8.0 {
								t.Errorf("Chaos P85 should be somewhat elevated (>8.0): %.2f", p85)
							}
						case "drift":
							if p85 < 7.0 {
								t.Errorf("Drift P85 should be elevated (>7) compared to mild: %.2f", p85)
							}
						}
					}
				}

				if !inProgressFound {
					t.Errorf("Status 'In Progress' not found in persistence data")
				}

				// 3. Verify Aging Analysis (WIP presence in Downstream)
				aRes, err := server.handleGetAgingAnalysis("MCSTEST", 0, "wip", "Downstream")
				if err != nil {
					t.Fatalf("Failed to get aging analysis: %v", err)
				}
				aEnv := aRes.(ResponseEnvelope)
				aging := aEnv.Data.(map[string]any)
				itemsFound := aging["aging"].([]stats.InventoryAge)
				if len(itemsFound) == 0 {
					t.Errorf("Expected WIP items in Downstream, got 0")
				}

				// 4. Verify WFA Accuracy (only for Weibull which is tuned for stability)
				if dist == "weibull" && scen == "mild" {
					wfaRes, err := server.handleGetForecastAccuracy("MCSTEST", 0, "scope", 0, 14, nil, 90, "", "")
					if err != nil {
						t.Fatalf("Failed to get forecast accuracy: %v", err)
					}

					wfaEnv := wfaRes.(ResponseEnvelope)
					wfa := wfaEnv.Data.(map[string]any)
					res := wfa["accuracy"].(simulation.WalkForwardResult)
					accuracy := res.AccuracyScore
					t.Logf("[%s/%s] WFA Accuracy: %.2f", dist, scen, accuracy)

					if accuracy < 0.70 {
						t.Errorf("Mild WFA Accuracy too low: %.2f (expected > 0.70)", accuracy)
					}
				}

				// 5. Verify Flow Debt
				fRes, err := server.handleGetFlowDebt("MCSTEST", 0, 26, "week")
				if err != nil {
					t.Fatalf("Failed to get flow debt: %v", err)
				}
				fEnv := fRes.(ResponseEnvelope)
				flowDebtMap := fEnv.Data.(map[string]any)
				flowDebt := flowDebtMap["flow_debt"].(stats.FlowDebtResult)

				if len(flowDebt.Buckets) == 0 {
					t.Errorf("Expected flow debt buckets, got 0")
				}
				t.Logf("[%s/%s] Flow Debt: TotalDebt=%d", dist, scen, flowDebt.TotalDebt)

				// 6. Verify CFD Data
				cRes, err := server.handleGetCFDData("MCSTEST", 0, 26)
				if err != nil {
					t.Fatalf("Failed to get CFD data: %v", err)
				}
				cEnv := cRes.(ResponseEnvelope)
				cfdMap := cEnv.Data.(map[string]any)
				cfd := cfdMap["cfd_data"].(stats.CFDResult)

				if len(cfd.Buckets) == 0 {
					t.Errorf("Expected CFD buckets, got 0")
				}
				if len(cfd.Statuses) == 0 {
					t.Errorf("Expected CFD statuses, got 0")
				}
				t.Logf("[%s/%s] CFD: Buckets=%d, Statuses=%d", dist, scen, len(cfd.Buckets), len(cfd.Statuses))
			})
		}
	}
}

func generateMockData(scenario, distribution string, count int, outDir, sourceID string) {
	cfg := engine.GeneratorConfig{
		Scenario:     scenario,
		Distribution: distribution,
		Count:        count,
		Now:          time.Now(),
	}

	events, mapping := engine.Generate(cfg)
	if err := engine.Save(outDir, sourceID, events, mapping); err != nil {
		panic(fmt.Sprintf("Failed to save mock data: %v", err))
	}
}
