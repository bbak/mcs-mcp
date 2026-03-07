---
name: flow-debt-chart
description: >
  Creates a dark-themed React/Recharts chart for the **Flow Debt** analysis
  (mcs-mcp:analyze_flow_debt). Trigger on: "flow debt chart", "arrivals vs departures",
  "commitment vs delivery", "WIP pressure chart", "flow clog chart", "debt accumulation",
  or any "show/chart/plot/visualize that" follow-up after an analyze_flow_debt result
  is present.
  ONLY for analyze_flow_debt output (weekly/monthly arrivals, departures, per-bucket debt,
  and cumulative total debt).
  Do NOT use for: throughput stability (analyze_throughput), WIP count stability
  (analyze_wip_stability), Total WIP Age (analyze_wip_age_stability), cycle time SLE
  (analyze_cycle_time), CFD (generate_cfd_data), or residence time (analyze_residence_time).
  Those are different analyses requiring different charts.
  Always read this skill AND mcs-charts-base before building the chart — do not attempt
  it ad-hoc.
---

# Flow Debt Chart

> **Scope:** Use this skill ONLY when `analyze_flow_debt` data is present in the
> conversation. It visualizes the weekly (or monthly) gap between arrivals and departures,
> the per-bucket debt, and the running cumulative debt — making WIP pressure visible
> before it manifests as cycle time inflation.
>
> **This skill extends `mcs-charts-base`.** Read that skill first. Everything defined
> there (stack, color tokens, typography, page/panel wrappers, stat card markup, badge
> system, CartesianGrid, tooltip base style, legend pattern, interactive controls,
> footer style, universal checklist) applies here without repetition. This skill only
> specifies what is unique to the flow debt chart.

---

## Prerequisites

Call the tool if data is not yet in the conversation:

```js
mcs-mcp:analyze_flow_debt({ board_id, project_key })
// Optional: bucket_size ("week" default | "month"), history_window_weeks (default 26)
```

---

## Response Structure

```
data.flow_debt
  .buckets[]               — one entry per time bucket:
      .label               — ISO week or month label (e.g. "2025-W36", "2025-09")
      .arrivals            — items that crossed the commitment point in this bucket
      .departures          — items that reached a delivered resolution in this bucket
      .debt                — arrivals − departures (positive = more in than out)
  .totalDebt               — sum of all bucket debts over the window
```

> **Semantic note:** Arrivals = items that crossed the commitment point (e.g. entered
> "awaiting development"). Departures = items that reached a delivered resolution.
> `debt` is a signed integer: positive = WIP growing, negative = WIP shrinking,
> zero = balanced. `totalDebt` is the net accumulated imbalance over the window — a
> positive total is a leading indicator of future cycle time inflation via Little's Law.

---

## Data Preparation

```js
// Build cumulative debt and derive color per bucket
let cumulative = 0;
const chartData = data.flow_debt.buckets.map(b => {
  cumulative += b.debt;
  return {
    ...b,
    cumulative,
    // Shorten label for X-axis: "2025-W36" → "25W36", "2025-09" → "25-09"
    shortLabel: b.label.replace("20", "").replace("-W", "W"),
    debtColor: b.debt > 0 ? ALARM : b.debt < 0 ? POSITIVE : MUTED,
  };
});

// Summary stats
const totalArrivals   = buckets.reduce((s, b) => s + b.arrivals, 0);
const totalDepartures = buckets.reduce((s, b) => s + b.departures, 0);
const avgArrival      = (totalArrivals   / buckets.length).toFixed(1);
const avgDeparture    = (totalDepartures / buckets.length).toFixed(1);
const weeksPositive   = buckets.filter(b => b.debt > 0).length;
const weeksNegative   = buckets.filter(b => b.debt < 0).length;
const worstDebt       = Math.max(...buckets.map(b => b.debt));
const worstBucket     = buckets.find(b => b.debt === worstDebt)?.label;
```

---

## System State Classification

Classify `data.flow_debt.totalDebt` to drive the headline badge and stat card color:

```js
const systemState = totalDebt > 10 ? "DEBT ACCUMULATING"
                  : totalDebt >  0 ? "MILD DEBT"
                  :                  "BALANCED";
const systemColor = totalDebt > 10 ? ALARM
                  : totalDebt >  0 ? CAUTION
                  :                  POSITIVE;
```

---

## Chart Architecture

Three independent views, toggled from the badge row. All share the same X-axis bucketing.

### View A: Weekly Debt (default)

**Vertical `BarChart`**, one bar per bucket.
`dataKey="debt"`. Bar fill driven by `debtColor` (ALARM / POSITIVE / MUTED per bucket)
via `<Cell>`. `radius={[3,3,0,0]}`.

Add a `<ReferenceLine y={0}>` in MUTED, dashed `"4 4"`, `strokeWidth={1.5}` — the
zero line is the key visual anchor in this view.

Y-axis: `tickFormatter={v => v > 0 ? "+" + v : String(v)}` to show sign explicitly.

Height: 320px.

### View B: Arrival vs. Departure

**`ComposedChart`** with two `<Area>` series:
1. `dataKey="arrivals"`   — PRIMARY `#6b7de8`, `strokeWidth={2}`, gradient fill `arrivalGrad`
2. `dataKey="departures"` — SECONDARY `#7edde2`, `strokeWidth={2}`, gradient fill `departureGrad`

Both use `type="monotone"`, `dot={false}`.

Add two `<ReferenceLine>` lines for average arrival and average departure rates:
- Average arrivals: PRIMARY color, dashed, labeled `"Avg arr {avgArrival}"`
- Average departures: SECONDARY color, dashed, labeled `"Avg dep {avgDeparture}"`

Use the base skill's area gradient pattern for both gradients (distinct `id` values).

Height: 320px.

### View C: Cumulative Debt

**`ComposedChart`** with a single `<Area>`:
- `dataKey="cumulative"` — ALARM `#ff6b6b`, `strokeWidth={2.5}`, gradient fill with
  ALARM color at low opacity (use base skill area gradient pattern, id `"cumulGrad"`).
- `dot={false}`, `activeDot={{ r: 4, fill: ALARM }}`

Add a `<ReferenceLine y={0}>` in MUTED, dashed, labeled `"Balanced"` at
`position="insideTopRight"`.

Y-axis: `tickFormatter={v => v > 0 ? "+" + v : String(v)}`.

Height: 320px.

### Weekly Breakdown Table (always visible, below all views)

A rendered `<table>` with columns: Week | Arrivals | Departures | Weekly Debt | Cumulative.

Color rules:
- Arrivals column: PRIMARY
- Departures column: SECONDARY
- Weekly Debt: ALARM if positive, POSITIVE if negative, MUTED if zero; bold if non-zero
- Cumulative: ALARM if positive, POSITIVE if negative, MUTED if zero
- Row background: subtle ALARM tint `${ALARM}08` for rows where `debt > 8` (extreme weeks)

Debt and cumulative values: prefix with `"+"` when positive.

---

## Header (extends base skill header structure)

- **Breadcrumb:** `{PROJECT_KEY} · {board name} · Board {board_id}`
- **Title:** exactly `"Flow Debt"`
- **Subtitle:** `"Arrivals vs. Departures · Weekly Debt & Cumulative Pressure · {n}-week window"`

**Stat cards:**

| Label | Value | Color |
|---|---|---|
| `TOTAL DEBT` | `+{totalDebt}` (or `{totalDebt}` if ≤ 0) | `systemColor` |
| `AVG ARRIVALS` | `{avgArrival}/wk` | PRIMARY `#6b7de8` |
| `AVG DEPARTURES` | `{avgDeparture}/wk` | SECONDARY `#7edde2` |
| `WEEKS IN DEBT` | `{weeksPositive}/{n}` | ALARM `#ff6b6b` |
| `WORST BUCKET` | `+{worstDebt} ({worstBucket last 3 chars})` | ALARM `#ff6b6b` |

---

## Badge Row (extends base skill badge system)

Always show:
1. `⚡ {systemState} — Cumulative Debt: +{totalDebt} items` — color: `systemColor`
2. `{weeksPositive} weeks debt · {weeksNegative} weeks surplus` — CAUTION
3. `Avg surplus/deficit: {(avgDeparture - avgArrival).toFixed(1)}/wk` —
   POSITIVE if `avgDeparture >= avgArrival`, ALARM otherwise

**Interactive controls** (right-aligned in badge row, per base skill interactive controls
pattern): three view toggle buttons using PRIMARY as the active color:
- `WEEKLY DEBT` (default)
- `ARRIVAL vs DEPARTURE`
- `CUMULATIVE`

---

## Tooltips (extends base skill tooltip base style)

**Weekly Debt and Arrival vs. Departure views:**
```
{bucket label}   (e.g. "2025-W36")
────────────────────────────────
Arrivals:    {arrivals}          ← PRIMARY color
Departures:  {departures}        ← SECONDARY color
────────────────────────────────
Weekly Debt: {+debt or debt}     ← debtColor, bold
Cumulative:  {+cumulative or cumulative}  ← MUTED
```

**Cumulative view:**
```
{bucket label}
────────────────────────────────
Cumulative Debt: {+cumulative}   ← ALARM/POSITIVE/MUTED
{contextual note}                ← 11px MUTED:
  positive → "More items entered than left — WIP pressure building"
  negative → "More items left than entered — system releasing pressure"
  zero     → "Balanced arrivals and departures"
```

---

## Legend (extends base skill legend pattern)

**Weekly Debt view** — centered below panel, filled rect swatches:
```
■ ALARM    Positive debt (arrivals > departures)
■ POSITIVE Negative debt (departures > arrivals)
■ MUTED    Balanced (debt = 0)
```

**Arrival vs. Departure view** — centered below panel, line swatches:
```
─ PRIMARY    Arrivals (avg {avgArrival}/wk)
─ SECONDARY  Departures (avg {avgDeparture}/wk)
```

**Cumulative view** — centered below panel:
```
─ ALARM      Cumulative flow debt
-- MUTED     Zero (balanced)
```

---

## Footer Content (follows base skill footer style)

Two sections:

1. **"Reading this chart:"** — Weekly debt is the difference between items committed
   (arrivals) and items delivered (departures) in each period. A positive debt means
   the system absorbed more work than it released — WIP grows. A persistent positive
   cumulative debt is a leading indicator of cycle time inflation: by Little's Law,
   more WIP mathematically produces longer wait times. The arrival vs. departure view
   shows raw rates — when the arrival line consistently sits above the departure line,
   debt accumulates. The cumulative view shows the running total — a rising curve signals
   growing systemic pressure even if individual weeks appear balanced.

2. **"Data scope:"** — Arrivals = items that crossed the commitment point in that bucket.
   Departures = items that reached a delivered resolution in that bucket. Window: {n} weeks.
   Total accumulated debt over the window: `+{totalDebt} items` (in systemColor).

---

## Chart-Specific Checklist

> The universal checklist is in `mcs-charts-base`. Only chart-specific items are listed here.

- [ ] Both `mcs-charts-base` and this skill read before building
- [ ] Skill triggered by `analyze_flow_debt` data
- [ ] Chart title reads exactly **"Flow Debt"**
- [ ] Cumulative debt computed in data preparation (running sum), not taken from tool response
- [ ] `debtColor` applied per bucket: ALARM (positive) / POSITIVE (negative) / MUTED (zero)
- [ ] System state classification: DEBT ACCUMULATING / MILD DEBT / BALANCED drives badge + stat card color
- [ ] Three view toggle buttons right-aligned in badge row: WEEKLY DEBT / ARRIVAL vs DEPARTURE / CUMULATIVE
- [ ] Weekly Debt view: zero `<ReferenceLine>` present; Y-axis shows sign (`+N` / `-N`)
- [ ] Arrival vs. Departure view: two `<Area>` series with gradients; avg reference lines for both
- [ ] Cumulative view: single ALARM `<Area>` with gradient; zero `<ReferenceLine>` labeled "Balanced"
- [ ] Weekly breakdown table always visible; debt and cumulative prefixed `+` when positive
- [ ] Extreme rows (debt > 8) highlighted with subtle ALARM row tint
- [ ] Stat cards: Total Debt (systemColor), Avg Arrivals, Avg Departures, Weeks in Debt, Worst Bucket
- [ ] Badges: system state, weeks in debt/surplus, avg surplus/deficit with correct directional color
- [ ] Tooltip: Weekly/Rates views show arrivals + departures + signed debt + cumulative
- [ ] Tooltip: Cumulative view shows signed cumulative + contextual note
- [ ] Legend reflects current view (debt color swatches / line swatches / cumulative line)
