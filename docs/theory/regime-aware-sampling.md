# Regime-Aware Sampling for Monte Carlo Simulation

## 1. The Forecasting Problem

Monte Carlo Simulation (MCS) for delivery forecasting works by resampling from a histogram of historical daily throughput. The implicit assumption is stationarity — every day in the sample window is equally likely to represent future behavior.

When this assumption is violated (throughput has changed), the standard response in the flow metrics community is to narrow the sampling window to "the right sample" — selecting a timeframe most likely to match the future. This is considered the art of MCS forecasting.

## 2. Why Window Selection Is Insufficient

The "right sample" framing is fundamentally limited:

- **Narrowing to recent data overfits to the latest regime.** If the process has a history of regime changes, the current regime is likely transient. Betting on it persisting is the same mistake as betting on any single regime.

- **Keeping all data dilutes the signal.** If throughput genuinely shifted, old data contaminates the distribution and produces forecasts that match neither the past nor the present.

- **You cannot know which window matches the future.** Window selection requires predicting the future process state — exactly what the forecast is supposed to answer. It's circular.

- **Meta-stable processes defeat both strategies.** Many software delivery systems are meta-stable (convergent within any window, but parameters drift between windows). Neither "use recent" nor "use all" produces reliable forecasts for these systems.

## 3. The Baseline Distribution

The simplest defensible baseline for MCS is **uniform sampling** — every day in the histogram is equally likely. This is the production default and embodies **regression to the mean**: the assumption that transient process states will revert, and the long-run behavior is the unweighted average of all observed days.

When the coherence gap slope (Section 5) indicates sustained deterioration, the sampling distribution tilts toward recent data via exponential decay. When the slope is flat or negative, α = 0 and sampling remains uniform — identical to production.

> **Note**: An earlier approach attempted to construct a "typical" distribution by detecting throughput regimes via changepoint analysis and blending them proportionally by duration. This was tried and abandoned — see Section 6b for details.

## 4. The Coherence Gap as Leading Indicator

From Sample Path Analysis, the **coherence gap** is defined as:

```text
coherence_gap(T) = w(T) - W*(T)
```

where w(T) is the average residence time (all items, including active WIP) and W*(T) is the average sojourn time (completed items only).

The coherence gap is a **leading indicator** of process change. When active WIP begins aging beyond what completed items experienced, throughput is about to deteriorate — but the throughput histogram hasn't reflected this yet. The gap detects the cause (WIP aging) before the effect (throughput drop) manifests.

**The slope of the coherence gap** provides a second-order signal — the rate of change of this leading indicator:

- **Steep positive slope**: The gap is widening rapidly. Active WIP is aging faster and faster relative to completed items. This is not a transient fluctuation — it indicates sustained, accelerating deterioration. The current regime is diverging from historical norms.

- **Gentle positive slope**: Slow drift. The process may be shifting, but the evidence for a sustained change is weak. Regression to the mean remains plausible.

- **Flat / near-zero slope**: Stable. Whether the gap itself is large or small, its rate of change is zero. The process is in a steady state (possibly a degraded one, but not actively worsening).

- **Negative slope**: Recovery. Active WIP is being worked down. Recent data may be better than typical, but regression to the mean is again the safer assumption.

## 5. The Blending Dial

The coherence gap slope maps to a continuous blending factor α ∈ [0, 1] that controls how much the sampling distribution deviates from uniform toward recency-biased:

**Slope computation** (Ordinary Least Squares):

Given the coherence gap time series {g₁, g₂, ..., gₙ} at daily bucket indices {1, 2, ..., n}:

```text
β₁ = (n·Σ(i·gᵢ) - Σi·Σgᵢ) / (n·Σi² - (Σi)²)
```

This is the standard OLS slope of the coherence gap against time.

**Normalization**: The raw slope has units of "coherence gap per day," which is not comparable across projects. Normalize by the median absolute coherence gap:

```text
slope_normalized = β₁ / median(|gᵢ|)
```

This produces a dimensionless quantity expressing how fast the gap is changing relative to its typical magnitude.

**Sigmoid mapping**:

```text
α = max(0, 2 · σ(slope_normalized / scale) - 1)
```

where `σ(x) = 1 / (1 + e⁻ˣ)` is the logistic sigmoid and `scale` is a calibration parameter (default 1.0).

Properties:

- When slope_normalized ≤ 0 (flat or improving): α = 0 → uniform sampling (identical to production)
- When slope_normalized is large and positive: α → 1 → lean toward most recent data
- The transition is smooth, not binary — moderate positive slopes produce moderate α values

## 6. Weight Construction

For each day index i in the histogram (i = 0 is oldest, n-1 is most recent):

```text
w[i] = (1 - α) · 1.0 + α · exp(-λ · (n - 1 - i))
```

where λ = ln(10) / max(n - 45, 1). The 45-day reference window is calibrated so the most recent ~45 days capture the dominant share of recency weight when α = 1.

Weights are renormalized so Σ w[i] = n (preserving the expected number of samples per simulation trial, maintaining statistical power).

When α = 0: all weights are 1.0 — uniform sampling, identical to production MCS.
When α = 1: heavily recency-biased. Recent data dominates, similar to aggressive window narrowing — but arrived at through evidence (coherence gap slope), not arbitrary choice.

## 6b. Approaches Tried and Abandoned

This section documents approaches that were implemented, empirically tested, and removed. Knowing what doesn't work is as valuable as knowing what does.

### Regime-Proportional "Typical" Distribution

**What**: Detect distinct throughput regimes via changepoint analysis (smoothing, shift detection, merging) and construct a "typical" distribution that blends regimes proportionally by duration. The idea was that regression to the mean should weight each regime by how long it persisted historically.

**Implementation**: `DetectRegimes` smoothed daily throughput with a 14-day sliding window, detected changepoints where the smoothed mean shifted by >30%, confirmed only if the new level persisted ≥7 days, merged adjacent regimes with means within 15%, and capped at 5 regimes. `ComputeRegimeWeights` assigned per-day weights based on regime duration fractions at α = 0, blending with exponential recency at higher α values.

**Empirical result**: Tested on 4 Jira boards (IESFSCPL, SAPFORMETM, ARTDATA, ARTPUBCL). On 3/4 boards the coherence gap slope was negative, producing α ≈ 0 — correct fallback to uniform, but the regime layer added nothing. On 1 board α = 0.008 (negligible). At α ≈ 0, the "typical" regime-proportional distribution is mathematically identical to uniform sampling. The regime detection machinery added complexity but no differentiation from production.

**Why it failed**: The coherence gap slope already solves the problem regime detection was trying to solve. If the slope says "don't tilt" (α ≈ 0), regime structure adds nothing over uniform. If it says "tilt" (α > 0), pure exponential recency decay achieves the same effect without changepoint machinery. The regime layer was an intermediate abstraction that didn't carry its weight.

### Hardcoded α in Backtesting

**What**: The A/B backtest handler used a fixed α = 0.5 for the treatment group, because per-checkpoint coherence gap slope wasn't threaded through the `EngineFactory` callback.

**Empirical result**: On ARTPUBCL, this caused -4% accuracy degradation at higher scope levels. The hardcoded α applied recency weighting even when the slope at that checkpoint said not to — violating the core design principle that the coherence gap slope should drive activation.

**Fix**: Changed the `EngineFactory` signature to receive per-checkpoint `StationarityAssessment` and `ResidenceTimeBucket` series, enabling proper per-checkpoint slope → α computation. When the slope is flat or negative, the treatment falls back to uniform sampling (identical to control).

### Key Insight

The regime detection was solving a problem the coherence gap slope already solves more directly. The slope is the activation signal; the weights are just exponential decay. No intermediate "regime structure" is needed between them.

## 7. Why This Differs From Exponential Tilting and Block Bootstrap

Classical recency-weighted bootstrap (exponential tilting) applies a fixed exponential decay to all observations:

```text
w_classical[i] = exp(-λ · (n - 1 - i))
```

This approach differs in two fundamental ways:

1. **Data-driven activation**: The blending factor α is derived from the coherence gap slope — a quantity that has meaning within the SPA framework. It is not a tuning parameter to be calibrated by the user, but a signal computed from the data. When the coherence gap slope is flat or negative, α = 0 and sampling is uniform (identical to production MCS). The recency bias activates only when the SPA diagnostics indicate sustained directional change.

2. **Graceful degradation**: When the process is stationary (flat coherence gap), this reduces to uniform sampling — identical to classical MCS. The recency tilt activates proportionally to the strength of the non-stationarity signal. There is no binary switch, no user-selected window, and no regime detection machinery — just a continuous, data-driven dial from "trust all history equally" to "trust recent history more."

## 8. Empirical vs Formal Validation

This approach is designed for **empirical validation** via walk-forward A/B backtesting:

1. At each historical checkpoint, run the simulation twice: once with uniform sampling (control) and once with recency-weighted sampling (treatment).
2. Compare cone-hit rates (fraction of checkpoints where the actual outcome falls within the forecast confidence band).
3. The recency-weighted approach merges only if treatment accuracy ≥ control accuracy across multiple projects and checkpoint windows.

This complements **formal mathematical proof** approaches — particularly Sample Path Analysis work that could establish theoretical conditions under which slope-driven recency weighting is optimal. The empirical approach answers "does this help in practice?" while the formal approach answers "why does it help, and when?"

Specific questions that formal analysis could illuminate:

- **Under what conditions does slope-driven recency weighting minimize forecast error?** Is there a class of meta-stable processes for which this is provably optimal?
- **Is the coherence gap slope the right activation signal?** Are there other SPA-derived quantities (e.g., the rate of change of Λ(T)/Θ(T)) that would provide better discrimination?
- **Does the exponential decay have theoretical justification?** Or would a different decay function (e.g., polynomial, step-function at the inflection point) be superior for certain process classes?

## 9. Connection to Sample Path Analysis

The recency-weighted sampling approach is grounded in the SPA framework:

1. **The coherence gap** `w(T) - W*(T)` is a direct consequence of the finite Little's Law identity `L(T) = Λ(T) · w(T)`. When w(T) diverges from W*(T), the system's input-output behavior is changing — active items are experiencing different dynamics than completed items did.

2. **The convergence assessment** (converging / metastable / diverging) classifies the end behavior of the sample path. Recency-weighted sampling extends this from a diagnostic signal to an actionable input: the convergence classification informs whether uniform sampling (regression to mean) or recent data is the better basis for forecasting.

3. **The slope of the coherence gap** is a derivative of a SPA quantity over time. It measures the **acceleration** of the end effect — not just whether active items are aging beyond completed items, but whether this divergence is increasing, stable, or decreasing. This second-order signal is what distinguishes a transient fluctuation (single-point excursion in SPC terms) from a sustained shift.

4. **The slope alone is sufficient** to bridge SPA diagnostics and MCS mechanics. SPA tells you *that* the process is non-stationary and *how fast* it is changing; the sigmoid mapping converts this into a continuous blending factor α; the exponential decay weights translate α into a modified sampling distribution. No intermediate regime detection is needed (see Section 6b).

The aspiration is that formal SPA analysis will eventually provide the theoretical foundation for what we are currently validating empirically — establishing the conditions under which this approach is not just useful, but provably optimal for a defined class of processes.

### Residence-Time-Informed Per-Checkpoint Window Narrowing (Phase 3)

**What**: In experimental mode, re-run the SPA on the full history (warmup date → checkpoint
date) rather than the 90-day sampling window, then use that full-history assessment to narrow
the MCS sampling window. Two variants were tried:

1. **Geometric heuristic**: `RecommendedWindowDays` = days from the start of the final-quarter
   of the w(T) series to the window end (floor 30 days). Wired into both walk-forward per-checkpoint
   and single-run MCS experimental paths.

2. **Backward CUSUM on weekly w(T)**: Walk the weekly w(T) series backward from the most recent
   point; stop at the first CUSUM threshold breach (k=0.5σ, h=3σ, σ from d₂=1.128 moving ranges).
   The steps walked × 7 days = recommended window. Replaced the geometric heuristic in the same
   two experimental paths.

**Empirical result**: Both variants degraded walk-forward `cone_hit_rate` relative to the stable
90-day default. The CUSUM variant was tested on at least one board and showed worse accuracy than
the stable path.

**Why it failed**: Narrowing the sampling window to the most recent stationary segment is the
correct framing when the regime has genuinely shifted and will stay shifted. However, the
`!Stationary` signal fires too broadly — it catches transient fluctuations, flow imbalance, and
aging WIP, not just sustained regime changes. In those cases, narrowing the window *removes
useful data* and produces a smaller, noisier histogram that worsens forecast accuracy. The
SPA-derived `!Stationary` flag is a reliable diagnostic of current system health, but not a
reliable trigger for window narrowing.

**Key insight**: Window selection requires predicting which past data will match future behavior —
a circular problem. The coherence gap slope (Section 5) is a better activation signal because it
distinguishes *sustained directional deterioration* from transient non-stationarity. Narrowing
should activate only when the process is actively worsening, not merely non-stationary.

## References

- Stidham, S. (1972). "L = λW: A Discounted Analogue and a New Proof." *Operations Research*.
- El-Taha, M. & Stidham, S. (1999). *Sample-Path Analysis of Queueing Systems*. Springer.
- Kumar, K. (2025). "What is Residence Time." *The Polaris Flow Dispatch*.
- Kumar, K. (2025). "The Many Faces of Little's Law." *The Polaris Flow Dispatch*.
- Vose, M. D. (1991). "A Linear Algorithm for Generating Random Numbers with a Given Distribution." *IEEE Transactions on Software Engineering*.
