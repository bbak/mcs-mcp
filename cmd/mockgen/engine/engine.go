package engine

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"math/rand/v2"
	"mcs-mcp/internal/eventlog"
	"mcs-mcp/internal/jira"
	"mcs-mcp/internal/stats"
	"os"
	"path/filepath"
	"time"
)

type GeneratorConfig struct {
	Scenario     string
	Distribution string // "uniform" or "weibull"
	Count        int
	Now          time.Time
}

type WorkflowMetadata struct {
	SourceID        string                          `json:"source_id"`
	Mapping         map[string]stats.StatusMetadata `json:"mapping"`
	Resolutions     map[string]string               `json:"resolutions,omitempty"`
	StatusOrder     []string                        `json:"status_order,omitempty"`
	CommitmentPoint string                          `json:"commitment_point,omitempty"`
	DiscoveryCutoff *time.Time                      `json:"discovery_cutoff,omitempty"`
	NameRegistry    *jira.NameRegistry              `json:"name_registry,omitempty"`
}

func Generate(cfg GeneratorConfig) ([]eventlog.IssueEvent, map[string]stats.StatusMetadata) {
	if cfg.Now.IsZero() {
		cfg.Now = time.Now()
	}

	mapping := map[string]stats.StatusMetadata{
		"1": {Name: "Open", Tier: "Demand", Role: "active"},
		"2": {Name: "Refinement", Tier: "Upstream", Role: "active"},
		"3": {Name: "In Progress", Tier: "Downstream", Role: "active"},
		"4": {Name: "Done", Tier: "Finished", Outcome: "delivered"},
		"5": {Name: "Closed", Tier: "Finished", Outcome: "abandoned"},
	}

	// Use a deterministic source for mock data generation to ensure stable test results.
	rnd := rand.New(rand.NewPCG(42, 42))
	var events []eventlog.IssueEvent

	for i := 0; i < cfg.Count; i++ {
		key := fmt.Sprintf("MCSTEST-%d", i+1)
		k, lambda := 2.5, 9.5 // Mild: Targeted at ~5.0 day In-Progress residency
		switch cfg.Scenario {
		case "chaos":
			k = 0.8
			if cfg.Distribution == "weibull" {
				lambda = 12.0
			}
		case "drift":
			ratio := float64(i) / float64(cfg.Count)
			k = 2.5 - (1.7 * ratio) // Shift 2.5 -> 0.8
			lambda = 9.5 + (2.5 * ratio)
		}

		// 2. Sample Total Cycle Time (Duration)
		var totalDuration float64
		if cfg.Distribution == "weibull" {
			totalDuration = weibullSample(rnd, k, lambda)
		} else {
			// Uniform baseline: 6-11 days (Residency 3.6 - 6.6)
			totalDuration = 6.0 + rnd.Float64()*5.0
			if cfg.Scenario == "chaos" && rnd.Float64() < 0.2 {
				totalDuration += 10 + rnd.Float64()*15 // Controlled Black Swans
			}
			if cfg.Scenario == "drift" && i%2 == 0 {
				totalDuration *= 2.0
			}
		}

		// Calculate Arrival independently to fix bounds
		var arrival time.Time
		if float64(i)/float64(cfg.Count) < 0.60 {
			// Finished: Must arrive early enough to finish
			maxArrivalIdx := 100.0 - totalDuration
			if maxArrivalIdx < 1.0 {
				maxArrivalIdx = 1.0
			}
			offsetDays := totalDuration + rnd.Float64()*maxArrivalIdx
			arrival = cfg.Now.Add(-time.Duration(offsetDays*24) * time.Hour)
		} else {
			// WIP: Arrived more recently than totalDuration
			offsetDays := rnd.Float64() * totalDuration
			arrival = cfg.Now.Add(-time.Duration(offsetDays*24) * time.Hour)
		}

		// Add Created event
		events = append(events, eventlog.IssueEvent{
			IssueKey: key, IssueType: "Story", EventType: eventlog.Created, ToStatus: "Open", ToStatusID: "1", Timestamp: arrival.UnixMicro(),
		})

		// 3. Determine Status based on current Age
		ageDays := cfg.Now.Sub(arrival).Hours() / 24.0
		status := "Done"
		if ageDays <= totalDuration {
			// Item is WIP - determine tier by lifecycle progress
			progress := ageDays / totalDuration
			if progress < 0.15 {
				status = "Open"
			} else if progress < 0.40 {
				status = "Refinement"
			} else {
				status = "In Progress"
			}
		}

		// 4. Generate Events sequentially
		// All items start in Open at 'arrival'

		// Move to Refinement at 15% of totalDuration
		tRefinement := arrival.Add(time.Duration(totalDuration*0.15*24) * time.Hour)
		if tRefinement.Before(cfg.Now) {
			events = append(events, eventlog.IssueEvent{
				IssueKey: key, IssueType: "Story", EventType: eventlog.Change, FromStatus: "Open", FromStatusID: "1", ToStatus: "Refinement", ToStatusID: "2", Timestamp: tRefinement.UnixMicro(),
			})
		}

		// Move to In Progress at 40% of totalDuration
		tInProgress := arrival.Add(time.Duration(totalDuration*0.40*24) * time.Hour)
		if tInProgress.Before(cfg.Now) && status != "Refinement" {
			events = append(events, eventlog.IssueEvent{
				IssueKey: key, IssueType: "Story", EventType: eventlog.Change, FromStatus: "Refinement", FromStatusID: "2", ToStatus: "In Progress", ToStatusID: "3", Timestamp: tInProgress.UnixMicro(),
			})
		}

		// Move to Terminal at 100% of totalDuration
		tDone := arrival.Add(time.Duration(totalDuration*24) * time.Hour)
		if tDone.Before(cfg.Now) {
			resolution := "Fixed"
			toStatus := "Done"
			toStatusID := "4"

			// 20% of finished items are abandoned
			if rnd.Float64() < 0.2 {
				resolution = "Won't Do"
				toStatus = "Closed"
				toStatusID = "5"
			}

			events = append(events, eventlog.IssueEvent{
				IssueKey: key, IssueType: "Story", EventType: eventlog.Change, FromStatus: "In Progress", FromStatusID: "3", ToStatus: toStatus, ToStatusID: toStatusID, Resolution: resolution, Timestamp: tDone.UnixMicro(),
			})
		}
	}

	return events, mapping
}

func weibullSample(rnd *rand.Rand, k, lambda float64) float64 {
	u := rnd.Float64()
	if u == 0 {
		u = 0.0001
	}
	// X = lambda * (-ln(1-u))^(1/k)
	return lambda * math.Pow(-math.Log(1.0-u), 1.0/k)
}

func Save(outDir string, sourceID string, events []eventlog.IssueEvent, mapping map[string]stats.StatusMetadata) error {
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return err
	}

	jsonlPath := filepath.Join(outDir, fmt.Sprintf("%s.jsonl", sourceID))
	workflowPath := filepath.Join(outDir, fmt.Sprintf("%s_workflow.json", sourceID))

	// Save Events
	f, err := os.Create(jsonlPath)
	if err != nil {
		return err
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	enc := json.NewEncoder(w)
	for _, e := range events {
		enc.Encode(e)
	}
	w.Flush()

	// Save Workflow
	fw, err := os.Create(workflowPath)
	if err != nil {
		return err
	}
	defer fw.Close()

	meta := WorkflowMetadata{
		SourceID:        sourceID,
		Mapping:         mapping,
		Resolutions:     map[string]string{"1": "delivered", "2": "abandoned"},
		StatusOrder:     []string{"1", "2", "3", "4", "5"},
		CommitmentPoint: "3",
		NameRegistry: &jira.NameRegistry{
			Statuses: map[string]string{
				"1": "Open",
				"2": "Refinement",
				"3": "In Progress",
				"4": "Done",
				"5": "Closed",
			},
			Resolutions: map[string]string{
				"1": "Fixed",
				"2": "Won't Do",
				"3": "Duplicate",
			},
		},
	}

	encW := json.NewEncoder(fw)
	encW.SetIndent("", "  ")
	return encW.Encode(meta)
}
