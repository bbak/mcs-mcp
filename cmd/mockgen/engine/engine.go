package engine

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"mcs-mcp/internal/eventlog"
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
}

func Generate(cfg GeneratorConfig) ([]eventlog.IssueEvent, map[string]stats.StatusMetadata) {
	if cfg.Now.IsZero() {
		cfg.Now = time.Now()
	}

	mapping := map[string]stats.StatusMetadata{
		"Open":        {Tier: "Demand", Role: "active"},
		"Refinement":  {Tier: "Upstream", Role: "active"},
		"In Progress": {Tier: "Downstream", Role: "active"},
		"Done":        {Tier: "Finished", Outcome: "delivered"},
	}

	var events []eventlog.IssueEvent

	// We want the last arrival to be today (cfg.Now)
	// Average arrival rate: 1 per day
	tArrival := cfg.Now.AddDate(0, 0, -cfg.Count)

	for i := 0; i < cfg.Count; i++ {
		key := fmt.Sprintf("MCSTEST-%d", i+1)

		// 1. Arrival
		arrival := tArrival.Add(time.Duration(i*24) * time.Hour)
		events = append(events, eventlog.IssueEvent{
			IssueKey: key, IssueType: "Story", EventType: eventlog.Created, ToStatus: "Open", ToStatusID: "1", Timestamp: arrival.UnixMicro(),
		})

		// 1. Determine Parameters
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
			totalDuration = weibullSample(k, lambda)
		} else {
			// Uniform baseline: 6-11 days (Residency 3.6 - 6.6)
			totalDuration = 6.0 + rand.Float64()*5.0
			if cfg.Scenario == "chaos" && rand.Float64() < 0.2 {
				totalDuration += 10 + rand.Float64()*15 // Controlled Black Swans
			}
			if cfg.Scenario == "drift" && i > cfg.Count/2 {
				totalDuration *= 2.0
			}
		}

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

		// Move to Done at 100% of totalDuration
		tDone := arrival.Add(time.Duration(totalDuration*24) * time.Hour)
		if tDone.Before(cfg.Now) {
			events = append(events, eventlog.IssueEvent{
				IssueKey: key, IssueType: "Story", EventType: eventlog.Change, FromStatus: "In Progress", FromStatusID: "3", ToStatus: "Done", ToStatusID: "4", Resolution: "Fixed", Timestamp: tDone.UnixMicro(),
			})
		}
	}

	return events, mapping
}

func weibullSample(k, lambda float64) float64 {
	u := rand.Float64()
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
		Resolutions:     map[string]string{"Fixed": "delivered"},
		StatusOrder:     []string{"Open", "Refinement", "In Progress", "Done"},
		CommitmentPoint: "In Progress",
	}

	encW := json.NewEncoder(fw)
	encW.SetIndent("", "  ")
	return encW.Encode(meta)
}
