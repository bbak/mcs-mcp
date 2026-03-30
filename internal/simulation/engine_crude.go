package simulation

import "fmt"

// CrudeEngine is the baseline Monte Carlo simulation engine.
// It builds a throughput histogram from all delivered items in a fixed window
// and runs standard Monte Carlo trials against it.
type CrudeEngine struct{}

func (c *CrudeEngine) Name() string { return "crude" }

func (c *CrudeEngine) Run(req ForecastRequest) (Result, error) {
	h := NewHistogram(req.Finished, req.WindowStart, req.WindowEnd, req.IssueTypes, req.WorkflowMappings, req.Resolutions)

	engine := NewEngine(h)
	if req.SimulationSeed != 0 {
		engine.SetSeed(req.SimulationSeed)
	}

	// Resolve distribution: explicit overrides → histogram meta
	var dist map[string]float64
	if len(req.MixOverrides) > 0 {
		dist = req.MixOverrides
	} else if histDist, ok := h.Meta["type_distribution"].(map[string]float64); ok {
		dist = histDist
	}

	switch req.Mode {
	case "scope":
		return engine.RunMultiTypeScopeSimulation(req.TargetDays, DefaultTrials, req.IssueTypes, dist, true), nil
	case "duration":
		return engine.RunMultiTypeDurationSimulation(req.Targets, dist, DefaultTrials, true), nil
	default:
		return Result{}, fmt.Errorf("unknown simulation mode: %q", req.Mode)
	}
}
