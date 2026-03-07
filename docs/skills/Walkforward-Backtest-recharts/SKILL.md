---
name: backtest-chart
description: >
  Creates a dark-themed React/Recharts chart for the **Forecast Backtest** /
  Walk-Forward Analysis (mcs-mcp:forecast_backtest). Trigger on: "backtest chart",
  "walk-forward analysis", "forecast accuracy", "was the forecast reliable",
  "how accurate is the Monte Carlo", "forecast cone validation", or any
  "show/chart/plot/visualize that" follow-up after a forecast_backtest result is present.
  ONLY for forecast_backtest output (weekly checkpoints comparing actual outcomes to
  forecast cone at P50/P85/P95).
  Do NOT use for: Monte Carlo forecasts (monte-carlo-duration-chart,
  monte-carlo-scope-chart), cycle time SLE (analyze_cycle_time), or throughput cadence
  (analyze_throughput). Those are different analyses requiring different charts.
  Always read this skill AND mcs-charts-base before building the chart — do not attempt
  it ad-hoc.
---

# Backtest (Walk-Forward Analysis) Chart

> **Scope:** Use this skill ONLY when `forecast_backtest` data is present in the
> conversation. It visualizes forecast calibration over time — whether historical actual
> outcomes fell within the predicted Monte Carlo cone at each weekly checkpoint — across
> three views: forecast cone, actual-vs-P50 gap, and rolling accuracy.
>
> **This skill extends `mcs-charts-base`.** Read that skill first. Everything defined
> there (stack, color tokens, typography, page/panel wrappers, stat card markup, badge
> system, CartesianGrid, tooltip base style, legend pattern, interactive controls,
> footer style, universal checklist) applies here without repetition. This skill only
> specifies what is unique to the backtest chart.

---

## Prerequisites

```js
mcs-mcp:forecast_backtest({
  board_id, project_key,
  simulation_mode: "duration",  // or "scope"
  items_to_forecast: 10,        // duration mode
  // forecast_horizon_days: 14  // scope mode
})
```

---

## Response Structure

```
data.accuracy
  .accuracy_score              — overall hit rate (0.0–1.0)
  .validation_message          — human-readable summary string
  .checkpoints[]               — one entry per weekly backtest point:
      .date                    — ISO date string "YYYY-MM-DD"
      .actual_value            — actual days (duration) or items (scope) at this point
      .predicted_p50           — P50 forecast at this historical moment
      .predicted_p85           — P85 forecast at this historical moment
      .predicted_p95           — P95 forecast at this historical moment
      .is_within_cone          — boolean: actual fell within P10–P95 cone
      .drift_detected          — boolean: process drift flagged at this point
```

---

## ⚠ ANOMALOUS CHECKPOINT HANDLING — REQUIRED STEP

**Before rendering, always scan checkpoints for anomalous forecast values.**

An anomalous checkpoint occurs when the tool returns `predicted_p50` values of 3650
(or similarly extreme values like 9999, 365+) at a checkpoint. This indicates a
**process void** — a historical moment with near-zero throughput where the simulation
has no meaningful data to draw from. It is NOT a forecast failure; it is a data
artifact.

**The correct handling is to filter these checkpoints out entirely:**

```js
// CORRECT — filter before rendering
const CHECKPOINTS = raw.checkpoints.filter(d => d.predicted_p50 < 1000);

// WRONG — never render them, never show a 3650d bar
```

**Why filtering is correct:**
- A 3650d prediction represents a near-zero-throughput period, not a real forecast
- Including it would collapse the Y-axis and make the chart unreadable
- The accuracy score in the tool response already accounts for these checkpoints
  in its denominator — do not recalculate it from filtered data
- The filtered checkpoint should be acknowledged to the user: add a MUTED badge
  stating how many checkpoints were excluded and why

**Badge text for excluded checkpoints:**
```
"{N} anomalous checkpoint(s) excluded (predicted ≥ 1000d — process void)"
```

---

## Accuracy Score Color

```js
const accuracyColor = (s) =>
  s >= 0.80 ? POSITIVE  // #6bffb8 — reliable
: s >= 0.65 ? CAUTION   // #e2c97e — marginal
:             ALARM;    // #ff6b6b — low reliability
```

---

## Data Preparation

```js
// Step 1: Filter anomalous checkpoints (see above — mandatory)
const CHECKPOINTS = raw.checkpoints.filter(d => d.predicted_p50 < 1000);

// Step 2: Sort chronologically (oldest first — left to right on X-axis)
CHECKPOINTS.sort((a, b) => new Date(a.date) - new Date(b.date));

// Step 3: Gap data (Actual vs P50 view)
const gapData = CHECKPOINTS.map(d => ({
  ...d,
  gap: d.actual_value - d.predicted_p50,
}));

// Step 4: Rolling 5-checkpoint accuracy
const rollingData = CHECKPOINTS.map((d, i) => {
  const window = CHECKPOINTS.slice(Math.max(0, i - 4), i + 1);
  return { ...d, rollingRate: window.filter(w => w.is_within_cone).length / window.length };
});

// Step 5: Counts for stat cards
const HITS  = raw.checkpoints.filter(d => d.is_within_cone).length;   // from raw (unfiltered)
const TOTAL = raw.checkpoints.length;                                  // from raw (unfiltered)
const recentMisses = CHECKPOINTS.slice(-5).filter(d => !d.is_within_cone).length;
```

> **CRITICAL:** `HITS` and `TOTAL` must be computed from the **raw** (unfiltered)
> checkpoint array to match the tool's `accuracy_score`. Do not recompute accuracy
> from the filtered set.

---

## Chart Architecture

Three views, toggled from the badge row. A checkpoint detail table sits below all views.

### View A: Forecast Cone (default)

**`ComposedChart`**, height 340px. X-axis dates rotated -45°, `interval={3}`.

Layers (bottom to top):
1. **Cone fill** — `<Area dataKey="predicted_p95">` with gradient fill (PRIMARY at low opacity),
   `stroke="none"`. This shades the cone region.
2. **P95 line** — `<Line dataKey="predicted_p95">` ALARM, `strokeWidth={1.5}`,
   dashed `"4 3"`, `dot={false}`
3. **P85 line** — `<Line dataKey="predicted_p85">` CAUTION, `strokeWidth={2}`,
   dashed `"6 3"`, `dot={false}`
4. **P50 line** — `<Line dataKey="predicted_p50">` SECONDARY, `strokeWidth={1.5}`,
   dashed `"4 4"`, `dot={false}`
5. **Actual dots** — `<Line dataKey="actual_value">` with `stroke="none"` and a custom
   `dot` renderer: each dot colored POSITIVE (`is_within_cone: true`) or ALARM
   (`is_within_cone: false`), `r={5}`, dark page background stroke for separation.

### View B: Actual vs P50 Gap

**`ComposedChart`** with a single `<Bar dataKey="gap">`, height 320px.

Gap = `actual_value − predicted_p50`.
- Negative gap = actual faster than P50 forecast → POSITIVE fill
- Positive gap = actual slower than P50 forecast → ALARM fill
- Color per bar via `<Cell>`

`<ReferenceLine y={0}>` — MUTED, dashed `"4 4"`, `strokeWidth={1.5}`,
labeled `"P50 forecast line"` at `position="insideTopRight"`.

Y-axis: `tickFormatter={v => (v > 0 ? "+" : "") + v + "d"}` (show sign).

### View C: Rolling Accuracy

**`ComposedChart`** with a single `<Area dataKey="rollingRate">`, height 320px.

- `stroke={PRIMARY}`, `strokeWidth={2}`, fill gradient (PRIMARY, low opacity)
- `type="monotone"`, `dot` renderer coloring each point by `accuracyColor(rollingRate)`

Two reference lines:
- `y={0.80}` — POSITIVE, dashed `"6 3"`, labeled `"80% threshold"` at `insideTopRight`
- `y={accuracy_score}` — `accuracyColor(accuracy_score)`, dashed `"4 4"`,
  labeled `"Overall: {N}%"` at `insideBottomRight`

Y-axis: `domain={[0, 1]}`, `tickFormatter={v => (v*100).toFixed(0) + "%"}`.

### Checkpoint Detail Table (always visible)

Columns: Date | Actual | P50 | P85 | P95 | Result

Display in reverse chronological order (most recent first).

Color rules:
- Actual: POSITIVE if `is_within_cone`, ALARM if not; bold
- P50: SECONDARY, P85: CAUTION, P95: ALARM
- Result: `"✓ Hit"` in POSITIVE or `"✗ Miss"` in ALARM; bold
- Row background: subtle ALARM tint `${ALARM}06` for miss rows

---

## Header (extends base skill header structure)

- **Breadcrumb:** `{PROJECT_KEY} · {board name} · Board {board_id}`
- **Title:** exactly `"Forecast Backtest"`
- **Subtitle:** `"Walk-Forward Analysis · {simulation_mode} mode · {items_to_forecast} items · {CHECKPOINTS.length} valid checkpoints"`

**Stat cards:**

| Label | Value | Sub | Color |
|---|---|---|---|
| `ACCURACY SCORE` | `{accuracy_score * 100}%` | `"{HITS}/{TOTAL} checkpoints"` | `accuracyColor()` |
| `IN CONE` | `{HITS}` | `"actual ≤ P95"` | POSITIVE |
| `MISS` | `{TOTAL - HITS}` | `"outside cone"` | ALARM |
| `RECENT MISSES` | `{recentMisses}/5` | `"last 5 checkpoints"` | ALARM if ≥ 3, else CAUTION |
| `MODE` | `"Duration"` or `"Scope"` | `"{N} items"` | MUTED |

---

## Badge Row (extends base skill badge system)

Always show:
1. `Accuracy: {N}% — {Reliable | Marginal | Low reliability}` — `accuracyColor()`
2. If `recentMisses >= 3`: `⚠ {recentMisses}/5 recent checkpoints missed` — ALARM
3. If anomalous checkpoints were filtered:
   `"{N} anomalous checkpoint(s) excluded (predicted ≥ 1000d — process void)"` — MUTED
4. If any `drift_detected: true` in checkpoints:
   `"⚠ Process drift detected at {date}"` — ALARM

**Interactive controls** (right-aligned, per base skill pattern): three view toggle
buttons using PRIMARY as the active color:
- `FORECAST CONE` (default)
- `ACTUAL vs P50`
- `ROLLING ACCURACY`

---

## Tooltips (extends base skill tooltip base style)

**Forecast Cone and table views:**
```
{formatted date}
──────────────────────────────
Actual:        {actual_value}d   ← POSITIVE if hit, ALARM if miss; bold
──────────────────────────────
P50 forecast:  {p50}d            ← SECONDARY
P85 forecast:  {p85}d            ← CAUTION
P95 forecast:  {p95}d            ← ALARM
──────────────────────────────
✓ Within cone  or  ✗ Outside cone  ← POSITIVE or ALARM; bold
```

**Gap view:**
```
{formatted date}
──────────────────────────────
Gap: {+N or -N}d                  ← POSITIVE if negative, ALARM if positive; bold
{description 11px MUTED}
  negative: "Delivered faster than P50 forecast"
  positive: "Delivered slower than P50 forecast"
```

**Rolling Accuracy view:**
```
{formatted date}
──────────────────────────────
Rolling accuracy: {N}%            ← accuracyColor(rollingRate)
This checkpoint: ✓ Hit / ✗ Miss  ← POSITIVE or ALARM; 11px
```

---

## Legend (extends base skill legend pattern)

**Forecast Cone view:**
```
── SECONDARY dashed    P50 forecast
── CAUTION  dashed     P85 forecast
── ALARM    dashed     P95 forecast
●  POSITIVE            Actual — within cone
●  ALARM               Actual — outside cone
```

**Gap view:**
```
■ POSITIVE   Faster than P50 (negative gap)
■ ALARM      Slower than P50 (positive gap)
```

**Rolling Accuracy view:**
```
■ POSITIVE   ≥ 80% accurate (reliable)
■ CAUTION    65–80% (marginal)
■ ALARM      < 65% (low reliability)
```

---

## Footer Content (follows base skill footer style)

Two sections:

1. **"Reading this chart:"** — Walk-forward analysis reconstructs the system state at
   each past checkpoint, runs the Monte Carlo simulation as it would have been run at
   that moment, and checks whether the actual outcome fell within the predicted cone
   (P10–P95). A hit means the forecast was calibrated; a miss means the system behaved
   outside its historical norm at that point. Consecutive misses — particularly recent
   ones — indicate the forecast model is currently unreliable and future forecasts should
   be treated with extra caution. The rolling accuracy view reveals whether reliability
   is stable, degrading, or recovering over time.

2. **"Data scope:"** — `{TOTAL} checkpoints, weekly cadence. Simulation mode:
   {mode}, {N} items.` If anomalous checkpoints were excluded: append
   `"{N} checkpoint(s) with predicted P50 ≥ 1000d excluded — these represent historical
   process voids (near-zero throughput periods) rather than forecast failures.
   Accuracy score is sourced from the tool and reflects all {TOTAL} checkpoints."`.

---

## Chart-Specific Checklist

> The universal checklist is in `mcs-charts-base`. Only chart-specific items are listed here.

- [ ] Both `mcs-charts-base` and this skill read before building
- [ ] Skill triggered by `forecast_backtest` data
- [ ] Chart title reads exactly **"Forecast Backtest"**
- [ ] Anomalous checkpoints (predicted_p50 ≥ 1000) filtered from chart data before rendering
- [ ] `HITS` and `TOTAL` computed from raw (unfiltered) array — not from filtered set
- [ ] Filtered checkpoint count acknowledged in a MUTED badge and in the footer
- [ ] Checkpoints rendered in chronological order (oldest left, newest right)
- [ ] Forecast Cone view: 4 layers — cone fill Area, P95 line, P85 line, P50 line, actual dots
- [ ] Actual dots colored per `is_within_cone`: POSITIVE (hit) or ALARM (miss)
- [ ] Gap view: Y-axis shows signed values (`+Nd` / `-Nd`); zero reference line present
- [ ] Rolling Accuracy view: computed over 5-checkpoint sliding window; two reference lines (80% threshold + overall)
- [ ] Checkpoint table in reverse chronological order; miss rows have ALARM tint
- [ ] Recent misses stat card: ALARM color if ≥ 3/5, CAUTION otherwise
- [ ] Drift detected badge shown if any checkpoint has `drift_detected: true`
- [ ] Three view toggle buttons in badge row
- [ ] Accuracy label: "Reliable" ≥ 80%, "Marginal" ≥ 65%, "Low reliability" < 65%
