---
name: monte-carlo-duration-chart
description: >
  Creates a dark-themed React/Recharts histogram for the **Monte Carlo Duration Forecast**
  (mcs-mcp:forecast_monte_carlo with mode: "duration"). Trigger on: "duration forecast
  chart", "when will we finish", "completion date forecast", "how long to deliver",
  "P85 date", "SLE forecast chart", "duration histogram", or any "show/chart/plot/
  visualize that" follow-up after a forecast_monte_carlo duration mode result is present.
  ONLY for forecast_monte_carlo mode="duration" output (days-to-completion at 8
  percentile levels).
  Do NOT use for: scope mode forecasts (monte-carlo-scope-chart), backtest validation
  (forecast_backtest), cycle time SLE (analyze_cycle_time), or throughput cadence
  (analyze_throughput). Those are different analyses requiring different charts.
  Always read this skill AND mcs-charts-base before building the chart — do not attempt
  it ad-hoc.
---

# Monte Carlo Duration Chart

> **Scope:** Use this skill ONLY when `forecast_monte_carlo` data with `mode: "duration"`
> is present in the conversation. It visualizes 8 percentile bars (P10→P98) representing
> days to completion, with P85 as the primary SLE commitment reference and estimated
> calendar dates derived from today.
>
> **This skill extends `mcs-charts-base`.** Read that skill first. Everything defined
> there (stack, color tokens, typography, page/panel wrappers, stat card markup, badge
> system, CartesianGrid, tooltip base style, legend pattern, interactive controls,
> footer style, universal checklist) applies here without repetition. This skill only
> specifies what is unique to the Monte Carlo duration histogram.

---

## Prerequisites

```js
mcs-mcp:forecast_monte_carlo({
  board_id, project_key,
  mode: "duration",
  include_wip: true,           // include items already in progress
  issue_types: ["Story"],      // filter to relevant types
  additional_items: N,         // new backlog items not yet in Jira
})
```

---

## Response Structure

```
data.percentiles               — 8 keys, values are DAYS to completion:
  .aggressive                  — P10: best case
  .unlikely                    — P30
  .coin_toss                   — P50: median
  .probable                    — P70
  .likely                      — P85: ★ SLE (primary commitment threshold)
  .conservative                — P90
  .safe                        — P95
  .almost_certain              — P98: near-certain upper bound

data.spread
  .iqr                         — interquartile range (P25–P75)
  .inner_80                    — P10–P90 spread

data.fat_tail_ratio            — P95/P50; > 2 = fat tail risk
data.predictability            — "Stable" | "Marginal" | "Unstable ..."

data.composition               — items included in this simulation:
  .wip                         — WIP items
  .additional_items            — extra scope
  .existing_backlog            — auto-included backlog
  .total                       — total items simulated

data.throughput_trend
  .direction                   — "Increasing" | "Decreasing" | "Stable"
  .percentage_change           — % change from overall to recent throughput

data.context
  .days_in_sample              — baseline window
  .issues_analyzed             — items in simulation
  .throughput_overall          — items/day, full baseline
  .throughput_recent           — items/day, recent window
  .issue_types                 — types filtered
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

---

## Date Calculation

```js
const TODAY = new Date(); // runtime — NEVER hardcode

const addDays = (n) => {
  const d = new Date(TODAY);
  d.setDate(d.getDate() + n);
  return d.toLocaleDateString("en-GB", { day: "2-digit", month: "short", year: "2-digit" });
};
// addDays(191) → "14 Oct 26"
```

Show derived dates as stat card sub-labels and in the reference table.

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
- Y-axis: `tickFormatter={v => v + "d"}`, domain `[0, ceil(P98 * 1.12 / 50) * 50]`
- One `<Bar dataKey="days">`, colored per bar via `<Cell>`
- P85 bar: full opacity + amber stroke
- `radius={[4, 4, 0, 0]}`
- `<ReferenceLine y={P85}>` — CAUTION, dashed `"6 3"`, `strokeWidth={1.5}`,
  labeled `"P85 SLE: {N}d"` at `position="insideTopRight"`
- `<ReferenceLine y={P50}>` — SECONDARY, dashed `"4 4"`, `strokeWidth={1}`,
  labeled `"P50: {N}d"` at `position="insideTopLeft"`

**Panel subtitle** (12px MUTED, above chart):
`"Days to completion at each probability level — P85 is the professional SLE commitment threshold"`

### Reference Table (always visible)

Columns: Percentile | Days | Est. Completion

- P-label: `PERCENTILE_DEFS[i].color`; P85 row CAUTION bold
- Days: P85 CAUTION bold; others TEXT
- Est. Completion: `addDays(val)` — MUTED
- P85 row background: `${CAUTION}08`

### Composition Panel

Below the reference table. Small cards for simulation inputs:

| Label | Value | Color |
|---|---|---|
| `WIP ITEMS` | `{composition.wip}` | SECONDARY |
| `ADDITIONAL ITEMS` | `{composition.additional_items}` | PRIMARY |
| `ISSUE TYPES` | comma-joined | MUTED |
| `BASELINE` | `{days_in_sample}d` | MUTED |
| `THROUGHPUT` | `{throughput_overall.toFixed(2)}/wk` | MUTED |

---

## Header (extends base skill header structure)

- **Breadcrumb:** `{PROJECT_KEY} · {board name} · Board {board_id}`
- **Title:** exactly `"Monte Carlo Forecast"`
- **Subtitle:** `"Duration mode · {issue_types.join(", ")} · {days_in_sample}-day baseline"`

**Stat cards:**

| Label | Value | Sub | Color |
|---|---|---|---|
| `P50 MEDIAN` | `{coin_toss}d` | `addDays(coin_toss)` | SECONDARY |
| `P85 SLE ★` | `{likely}d` | `addDays(likely)` | CAUTION |
| `P95 SAFE BET` | `{safe}d` | `addDays(safe)` | ALARM |
| `IQR SPREAD` | `{spread.iqr}d` | — | MUTED |
| `PREDICTABILITY` | `{predictability}` | — | `predictColor()` |

---

## Badge Row (extends base skill badge system)

Always show:
1. `⚡ {predictability}` — `predictColor()`
2. `P85 SLE: {likely}d → {addDays(likely)}` — CAUTION
3. `Inner80: {spread.inner_80}d spread` — MUTED
4. `Issue types: {issue_types.join(", ")}` — MUTED
5. If throughput trend notable: `⚠ Throughput +{N}% recently — sustainability uncertain` — CAUTION

No interactive controls — duration chart has one view only.

---

## Tooltip (extends base skill tooltip base style)

```
{P-label} — {days}d
──────────────────────────────
Est. date: {addDays(days)}           ← SECONDARY
{desc for P10/P50/P85/P95/P98}       ← MUTED 11px
```

Descriptions:
- P10: "Best case — only 10% of simulations finished faster"
- P50: "Coin toss — 50% probability of finishing by this date"
- P85: "★ SLE — professional commitment threshold"
- P95: "Safe bet — high confidence buffer"
- P98: "Near-certain upper bound"

---

## Legend (extends base skill legend pattern)

Centered below chart, filled rect swatches:

```
■ POSITIVE   P10 — best case
■ SECONDARY  P30–P50 — likely fast
■ CAUTION    P70–P85 SLE — target range   (P85 with amber outline)
■ ALARM      P90–P98 — tail / buffer
```

---

## Footer Content (follows base skill footer style)

Two sections:

1. **"Reading this chart:"** — Each bar represents a percentile from the Monte Carlo
   simulation. The height is days to complete the forecast scope at that confidence level.
   P10 is the optimistic best case — only 10% of simulations finished faster. P85 is the
   professional SLE commitment threshold: 85% of simulations completed within this duration.
   P98 is the near-certain upper bound. Dates are calculated from today.

2. **"Data scope:"** — `Monte Carlo simulation from a {days_in_sample}-day throughput
   baseline ({issues_analyzed} items). Commitment point: "awaiting development".`
   If throughput trend notable: append sustainability warning.

---

## Chart-Specific Checklist

> The universal checklist is in `mcs-charts-base`. Only chart-specific items are listed here.

- [ ] Both `mcs-charts-base` and this skill read before building
- [ ] Skill triggered by `forecast_monte_carlo` data with `mode: "duration"`
- [ ] Chart title reads exactly **"Monte Carlo Forecast"**
- [ ] Dates derived from `new Date()` at runtime — never hardcoded
- [ ] 8 bars rendered in canonical P10→P98 order
- [ ] P85 bar: `fillOpacity: 1`, amber `stroke`, no stroke on others
- [ ] Two reference lines: P85 (CAUTION, insideTopRight) and P50 (SECONDARY, insideTopLeft)
- [ ] Y-axis domain leaves headroom above P98 (`ceil(P98 * 1.12 / 50) * 50`)
- [ ] Stat cards: P50, P85, P95 all show estimated dates as sub-labels
- [ ] Reference table: P85 row amber tint + bold; all dates via `addDays()`
- [ ] Composition panel present and sourced from `data.composition`
- [ ] Throughput trend badge shown when direction is not "Stable"
- [ ] Tooltip shows estimated date (SECONDARY) + description for key percentiles
- [ ] No mode toggle — this is a single-mode chart
