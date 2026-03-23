# Experimental Features

This file documents active experimental features — code paths that are under hypothesis
validation and have not yet been declared stable. Experimental features are:

- **Off by default** — the operator must explicitly enable the gate.
- **Opt-in per session** — the agent/user activates them deliberately via `set_experimental`.
- **Subject to removal** — if a hypothesis is rejected, the code and this entry are deleted.
- **Documented here, not in `architecture.md`** — once an experiment graduates to stable, its
  documentation moves to `architecture.md` and the flag gate is removed.

## Lifecycle

```
hypothesis → implemented (flag-gated) → validated → stable (gate removed, docs moved)
                                       ↓
                                  rejected (code deleted, entry removed)
```

## Activation

Two steps are required — both must be satisfied for experimental paths to execute:

**Step 1 — Operator enables the gate** (in `.env` next to the binary):

```env
MCS_ALLOW_EXPERIMENTAL=true
```

When false (default), `set_experimental` returns an error and experimental paths are
unreachable regardless of what the agent requests.

**Step 2 — Agent activates experimental mode for the session**:

```
set_experimental(enabled: true)
```

This persists for the duration of the session. It is not reset when switching boards — the
user controls it explicitly. Call `set_experimental(enabled: false)` to return to stable
behavior.

---

## Active Experiments

### SPA Pipeline: Departure-Adaptive Throughput Histogram

**Status:** Active
**Tool affected:** `forecast_monte_carlo`, `forecast_backtest`
**Files:** `internal/stats/spa_pipeline.go`, `internal/simulation/histogram.go` (`Resample`), `internal/mcp/handlers_forecasting.go`, `internal/simulation/walkforward.go`

#### Hypothesis

A departure-count-based adaptive sampling window will produce more accurate forecasts
than the fixed 90-day lookback. The pipeline walks backwards through the SPA departure
series until a minimum number of completed items is reached, stopping at regime
boundaries. This naturally adapts to the board's throughput scale: high-throughput
teams get tight, recent windows; low-throughput programmes get longer windows
appropriate to their cadence.

#### What it changes

When experimental mode is active, `forecast_monte_carlo` and `forecast_backtest` run
the SPA pipeline between data retrieval and histogram construction:

| Step | Action | Effect on histogram |
|------|--------|-------------------|
| 1. Convergence gate | Checks `summary.Convergence` | Flags warning + scales if diverging |
| 2. Outlier filtering | IQR fence on sojourn times, batch removal | Removes issues if convergence improves |
| 3. Adaptive window | Walks backwards through departures (see below) | Sets effective window start |
| 4. WIP/aging adjust | λ_implied/λ_histogram ratio | Resamples histogram days |

Steps 1-3 modify which issues enter `NewHistogram()`; Step 4 resamples the histogram
after construction.

**Adaptive window algorithm (Step 3):**

The window is determined by walking backwards through the SPA departure series:

- **Phase 1**: Accumulate departures (excluding outliers from Step 2) until
  `MinSampleDepartures` (50) is reached. Regime boundaries are ignored — this
  guarantees a minimum sample size.
- **Decision point**: Check regime segments crossed during Phase 1 for stationarity
  using the Λ/Θ balance (ratio ≤ 1.3 = stationary):
  - If **any** crossed segment is stationary → the 50 departures are representative.
    Stop at the threshold point.
  - If **all** crossed segments are non-stationary → enter Phase 2.
  - If **no** boundaries were crossed → stop at the threshold point.
- **Phase 2**: Continue walking back to the next regime boundary for a clean,
  regime-aligned window edge. The resulting window naturally has more than 50
  departures.
- **Hard ceiling**: `MaxLookbackDays` (365) applies throughout both phases.

#### Activation conditions

Both layers of the experimental gate must be active:

- `MCS_ALLOW_EXPERIMENTAL=true` in `.env`
- `set_experimental(enabled: true)` called in the session
- A commitment point must be configured on the board

#### Fallback behavior

With experimental mode off, `forecast_monte_carlo` and `forecast_backtest` behave
exactly as before. The SPA pipeline code is not reached, and the histogram is built
from the standard 90-day (or user-specified) lookback window. Golden tests run with
the gate off.

#### Known limitations

- Regime detection uses first-difference sign reversals, which may be noisy on short
  series. The `MinRegimeBuckets` parameter (default: 14) guards against transient
  false positives.
- Outlier filtering is batch-based. If only a subset of IQR-flagged items are
  exceptional, the batch approach may over- or under-filter.
- WIP resampling changes the number of days in the histogram, which affects the
  engine's uniform-random draw. The distribution shape is preserved but the sample
  size changes.
- Per-segment stationarity is assessed using the Λ/Θ ratio at each regime boundary
  bucket, not a full convergence assessment. This is fast but may miss subtler
  non-stationarity patterns.
- The pipeline runs SPA over the full issue history, which adds computation time
  proportional to the board's history depth.

#### Configurable parameters

All parameters are in `stats.SPAPipelineConfig` with defaults from
`stats.DefaultSPAPipelineConfig()`:

| Parameter | Default | Purpose |
|-----------|---------|---------|
| `MinSampleDepartures` | 50 | Minimum completed items for the adaptive window |
| `MaxLookbackDays` | 365 | Hard ceiling — never sample older than this |
| `MinRegimeBuckets` | 14 | Minimum duration for a regime boundary |
| `IQRMultiplier` | 1.5 | Tukey fence multiplier for sojourn outliers |
| `ScaleClampMin` | 0.5 | Lower bound for scale factors |
| `ScaleClampMax` | 2.0 | Upper bound for scale factors |
| `ScaleSnapZone` | 0.05 | Snap to 1.0 if within this fraction |

#### Graduation criteria

1. Walk-forward backtesting shows improved forecast accuracy (higher hit rate) across
   multiple boards compared to the non-experimental path.
2. The over-forecasting bias (actual completion faster than predicted) is reduced.
3. No degradation on boards where the existing approach already performs well.
4. Pipeline diagnostics (via `spa_pipeline` in the result context) confirm that each
   step is contributing meaningfully.
5. Stable for at least 4 weeks of production use without parameter changes.
