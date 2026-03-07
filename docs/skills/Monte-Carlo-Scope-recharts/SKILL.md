---
name: monte-carlo-scope-chart
description: >
  Creates a dark-themed React/Recharts histogram for the **Monte Carlo Scope Forecast**
  (mcs-mcp:forecast_monte_carlo with mode: "scope"). Trigger on: "scope forecast chart",
  "how many items in N days", "delivery volume forecast", "items deliverable", "scope
  histogram", "how much can we deliver", or any "show/chart/plot/visualize that"
  follow-up after a forecast_monte_carlo scope mode result is present.
  ONLY for forecast_monte_carlo mode="scope" output (item counts at 8 percentile levels
  within a fixed time window).
  Do NOT use for: duration mode forecasts (monte-carlo-duration-chart), backtest
  validation (forecast_backtest), cycle time SLE (analyze_cycle_time), or throughput
  cadence (analyze_throughput). Those are different analyses requiring different charts.
  Always read this skill AND mcs-charts-base before building the chart — do not attempt
  it ad-hoc.
---

# Monte Carlo Scope Chart

> **Scope:** Use this skill ONLY when `forecast_monte_carlo` data with `mode: "scope"`
> is present in the conversation. It visualizes 8 percentile bars (P10→P98) representing
> how many items are deliverable within a fixed target window, with P85 as the primary
> SLE commitment reference.
>
> **⚠ AXIS INVERSION WARNING:** In scope mode, the semantics are inverted relative to
> duration mode. Higher percentile = fewer items. Being "more confident" (P85 vs P50)
> means committing to a more conservative (lower) item count. This must be explained
> clearly in the subtitle, tooltip, and footer.
>
> **This skill extends `mcs-charts-base`.** Read that skill first. Everything defined
> there (stack, color tokens, typography, page/panel wrappers, stat card markup, badge
> system, CartesianGrid, tooltip base style, legend pattern, interactive controls,
> footer style, universal checklist) applies here without repetition. This skill only
> specifies what is unique to the Monte Carlo scope histogram.

---

## Prerequisites

```js
mcs-mcp:forecast_monte_carlo({
  board_id, project_key,
  mode: "scope",
  target_days: 90,    // fixed delivery window in days
})
```

---

## Response Structure

```
data.percentiles               — 8 keys, values are ITEM COUNTS in target_days:
  .aggressive                  — P10: optimistic count (most items, lowest confidence)
  .unlikely                    — P30
  .coin_toss                   — P50: median
  .probable                    — P70
  .likely                      — P85: ★ SLE (fewer items, higher confidence)
  .conservative                — P90
  .safe                        — P95
  .almost_certain              — P98: fewest items, near-certain

  ⚠ INVERTED SEMANTICS: P10 has the HIGHEST item count, P98 has the LOWEST.
  Higher percentile key = fewer items = more conservative = more confident.
  The bar heights naturally decrease left to right.

data.spread
  .iqr                         — interquartile range
  .inner_80                    — P10–P90 spread (in items)

data.predictability            — "Stable" | "Marginal" | "Unstable ..."

data.throughput_trend
  .direction                   — "Increasing" | "Decreasing" | "Stable"
  .percentage_change           — % change from overall to recent throughput

data.context
  .days_in_sample              — baseline window
  .issues_analyzed             — items in simulation
  .throughput_overall          — items/day, full baseline
  .throughput_recent           — items/day, recent window

// Note: data.composition is NOT present in scope mode — do not reference it.
// Note: target_days comes from the tool call parameter, not the response body.
//       Store it as a constant when embedding data.
```

---

## Canonical Percentile Order

Always render bars in this fixed order, left to right:

```js
const PERCENTILE_DEFS = [
  { key: "aggressive",     label: "P10", color: POSITIVE  },
  { key: "unlikely",       label: "P30", color: SECONDARY },
  { key: "coin_toss",      label: "P50", color: SECONDARY },
  { key: "probable",       label: "P70", color: CAUTION   },
  { key: "likely",         label: "P85", color: CAUTION,  highlight: true },
  { key: "conservative",   label: "P90", color: ALARM     },
  { key: "safe",           label: "P95", color: ALARM     },
  { key: "almost_certain", label: "P98", color: ALARM     },
];
```

P85 bar is always highlighted: `fillOpacity: 1` (others 0.6), amber `stroke={CAUTION}`
`strokeWidth={2}` via `<Cell>`.

Bar heights will naturally decrease left to right — this is correct and expected.
Do NOT reverse the X-axis or reorder bars.

---

## Predictability Color

```js
const predictColor = (p) =>
  p.includes("Unstable") ? ALARM    // #ff6b6b
: p.includes("Marginal")  ? CAUTION // #e2c97e
:                           POSITIVE; // #6bffb8
```

---

## Chart Architecture

### Histogram Panel

**`BarChart`**, height 320px.
- X-axis: `dataKey="label"` (P10 → P98)
- Y-axis: item count; `domain={[0, ceil(P10_value * 1.15 / 10) * 10]}`
  (P10 has the highest value — use it to set the ceiling)
- One `<Bar dataKey="items">`, colored per bar via `<Cell>`
- P85 bar: full opacity + amber stroke
- `radius={[4, 4, 0, 0]}`
- `<ReferenceLine y={P85_value}>` — CAUTION, dashed `"6 3"`, `strokeWidth={1.5}`,
  labeled `"P85: {N} items"` at `position="insideTopRight"`

No P50 reference line (optional — omit to keep chart clean).

**Panel subtitle** (12px MUTED, above chart):
`"Items deliverable in {target_days} days — higher percentile = more conservative (fewer items certain)"`

### Reference Table (always visible)

Columns: Percentile | Items | Confidence

- P-label: `PERCENTILE_DEFS[i].color`; P85 row CAUTION bold
- Items: P85 CAUTION bold; others TEXT
- Confidence: `"{X}% probability of delivering at least this many"` — MUTED
- P85 row background: `${CAUTION}08`

No composition panel — scope mode has no `data.composition`.

---

## Header (extends base skill header structure)

- **Breadcrumb:** `{PROJECT_KEY} · {board name} · Board {board_id}`
- **Title:** exactly `"Monte Carlo Forecast"`
- **Subtitle:** `"Scope mode · {target_days}-day window · {days_in_sample}-day baseline"`

**Stat cards:**

| Label | Value | Color |
|---|---|---|
| `P50 MEDIAN` | `{coin_toss} items` | SECONDARY |
| `P85 SLE ★` | `{likely} items` | CAUTION |
| `P95 SAFE BET` | `{safe} items` | ALARM |
| `TARGET WINDOW` | `{target_days}d` | MUTED |
| `PREDICTABILITY` | `{predictability}` | `predictColor()` |

No date sub-labels — scope mode has no date output.

---

## Badge Row (extends base skill badge system)

Always show:
1. `⚡ {predictability}` — `predictColor()`
2. `P85: {likely} items in {target_days}d` — CAUTION
3. `P50: {coin_toss} items` — SECONDARY
4. `⚠ Note: higher percentile = fewer items (inverted)` — MUTED
5. If throughput trend notable: `⚠ Throughput +{N}% recently — sustainability uncertain` — CAUTION

No interactive controls — scope chart has one view only.

---

## Tooltip (extends base skill tooltip base style)

```
{P-label} — {items} items
──────────────────────────────
{X}% probability of delivering      ← MUTED
at least {items} items in {target_days} days
```

For P85 specifically, append:
```
★ SLE — recommended commitment level  ← CAUTION, 11px
```

---

## Legend (extends base skill legend pattern)

Centered below chart, filled rect swatches:

```
■ POSITIVE   P10 — optimistic (most items)
■ SECONDARY  P30–P50 — likely range
■ CAUTION    P70–P85 SLE — commitment range   (P85 with amber outline)
■ ALARM      P90–P98 — conservative buffer (fewest items)
```

---

## Footer Content (follows base skill footer style)

Two sections:

1. **"Reading this chart:"** — Each bar shows how many items the team is likely to
   deliver within the target window at that confidence level. The axis is semantically
   inverted: higher percentile labels correspond to fewer items, because being more
   confident (P85 vs P50) means committing to a more conservative (lower) number.
   P10 represents the optimistic scenario — the most items, but only a 10% probability
   of achieving it. P85 means there is an 85% probability of delivering at least that
   many items within the window.

2. **"Data scope:"** — `Monte Carlo simulation from a {days_in_sample}-day throughput
   baseline ({issues_analyzed} items). Target window: {target_days} days.
   Commitment point: "awaiting development".`
   If throughput trend notable: append sustainability warning.

---

## Chart-Specific Checklist

> The universal checklist is in `mcs-charts-base`. Only chart-specific items are listed here.

- [ ] Both `mcs-charts-base` and this skill read before building
- [ ] Skill triggered by `forecast_monte_carlo` data with `mode: "scope"`
- [ ] Chart title reads exactly **"Monte Carlo Forecast"**
- [ ] `target_days` stored as a constant — it is NOT in the response body
- [ ] 8 bars rendered in canonical P10→P98 order (bars naturally decrease left to right — this is correct)
- [ ] P85 bar: `fillOpacity: 1`, amber `stroke`, no stroke on others
- [ ] Y-axis domain ceiling based on P10 value (highest), not P98
- [ ] One reference line only: P85 (CAUTION) — no P50 line needed
- [ ] Axis inversion warning badge shown in badge row
- [ ] Tooltip: states "X% probability of delivering AT LEAST N items"
- [ ] Reference table: Confidence column uses "X% probability of delivering at least this many"
- [ ] No composition panel — `data.composition` does not exist in scope mode
- [ ] No date calculations — scope mode has no date output
- [ ] No mode toggle — this is a single-mode chart
- [ ] Footer and subtitle explain the semantic axis inversion explicitly
- [ ] Throughput trend badge shown when direction is not "Stable"
