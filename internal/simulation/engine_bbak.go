package simulation

import (
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/rs/zerolog/log"
	"mcs-mcp/internal/stats"
)

// BbakEngine is a regime-aware sampling engine that enhances the baseline
// with a 5-step SPA (Sample-Path Approach) pipeline: convergence gating,
// regime boundary detection, outlier filtering, adaptive windowing, and
// post-histogram WIP/aging resampling.
//
// Falls back to crude behavior if prerequisites are missing (no commitment
// point or insufficient residence time data).
type BbakEngine struct{}

func (b *BbakEngine) Name() string { return "bbak" }

func (b *BbakEngine) Run(req ForecastRequest) (Result, error) {
	finished := req.Finished
	windowStart := req.WindowStart
	windowEnd := req.WindowEnd

	var spaDiagnostics *stats.SPAPipelineResult

	// SPA pipeline requires a commitment point and workflow mappings.
	if req.CommitmentPoint != "" && len(req.WorkflowMappings) > 0 {
		// Run SPA over full history (not just the windowed finished issues)
		fullWindow := stats.NewAnalysisWindow(time.Time{}, req.Clock, "day", req.DiscoveryCutoff)
		spaItems := stats.ExtractResidenceItems(req.AllIssues, req.CommitmentPoint,
			req.StatusWeights, req.WorkflowMappings, fullWindow.Start)

		if len(spaItems) > 0 {
			spaRT := stats.ComputeResidenceTimeSeries(spaItems, fullWindow)
			spaCfg := stats.DefaultSPAPipelineConfig()

			window := stats.NewAnalysisWindow(windowStart, windowEnd, "day", req.DiscoveryCutoff)
			spaDiagnostics = stats.RunSPAPipeline(
				spaRT, spaItems, finished,
				windowStart, windowEnd,
				req.CommitmentPoint, req.StatusWeights, req.WorkflowMappings,
				window, spaCfg,
			)

			// Override finished issues and window with pipeline output
			finished = spaDiagnostics.FilteredFinished
			windowStart = spaDiagnostics.EffectiveStart

			log.Info().
				Str("engine", "bbak").
				Time("effective_start", spaDiagnostics.EffectiveStart).
				Int("outliers_removed", spaDiagnostics.OutlierCount).
				Str("convergence", spaDiagnostics.ConvergenceStatus).
				Msg("SPA pipeline applied")
		}
	}

	// Build histogram from (possibly refined) inputs
	h := NewHistogram(finished, windowStart, windowEnd, req.IssueTypes, req.WorkflowMappings, req.Resolutions)

	// Post-histogram WIP/aging resampling
	if spaDiagnostics != nil {
		spaCfg := stats.DefaultSPAPipelineConfig()
		if tpOverall, ok := h.Meta["throughput_overall"].(float64); ok && tpOverall > 0 {
			wipScale := stats.ComputeWIPScaleFactor(spaDiagnostics.RTSummary, tpOverall, spaCfg)
			combinedFactor := wipScale * spaDiagnostics.DivergenceScaleFactor
			if combinedFactor != 1.0 {
				resampleRNG := rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 0))
				if req.SimulationSeed != 0 {
					resampleRNG = rand.New(rand.NewPCG(uint64(req.SimulationSeed), 1))
				}
				h.Resample(combinedFactor, resampleRNG)
				spaDiagnostics.WIPScaleFactor = combinedFactor
			}
		}
	}

	engine := NewEngine(h)
	if req.SimulationSeed != 0 {
		engine.SetSeed(req.SimulationSeed)
	}

	// Resolve distribution
	var dist map[string]float64
	if len(req.MixOverrides) > 0 {
		dist = req.MixOverrides
	} else if histDist, ok := h.Meta["type_distribution"].(map[string]float64); ok {
		dist = histDist
	}

	var res Result
	switch req.Mode {
	case "scope":
		res = engine.RunMultiTypeScopeSimulation(req.TargetDays, DefaultTrials, req.IssueTypes, dist, true)
	case "duration":
		res = engine.RunMultiTypeDurationSimulation(req.Targets, dist, DefaultTrials, true)
	default:
		return Result{}, fmt.Errorf("unknown simulation mode: %q", req.Mode)
	}

	// Attach SPA diagnostics to result context
	if spaDiagnostics != nil {
		if res.Context == nil {
			res.Context = make(map[string]any)
		}
		res.Context["spa_pipeline"] = spaDiagnostics
		if spaDiagnostics.DivergenceWarning != "" {
			res.Warnings = append(res.Warnings, spaDiagnostics.DivergenceWarning)
		}
		boundaryStr := "no"
		if spaDiagnostics.RegimeBoundaryRespected {
			boundaryStr = "yes"
		}
		res.Insights = append(res.Insights, fmt.Sprintf(
			"Adaptive window: %d departures over %d days (window: %s to %s, regime boundary respected: %s), %d outliers removed, convergence: %s, WIP scale: %.2f.",
			spaDiagnostics.AdaptiveWindowDepartures,
			spaDiagnostics.AdaptiveWindowDays,
			spaDiagnostics.EffectiveStart.Format("2006-01-02"),
			spaDiagnostics.EffectiveEnd.Format("2006-01-02"),
			boundaryStr,
			spaDiagnostics.OutlierCount,
			spaDiagnostics.ConvergenceStatus,
			spaDiagnostics.WIPScaleFactor,
		))
	}

	return res, nil
}
