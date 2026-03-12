---
name: residence-time-chart
description: >
  Creates a dark-themed dual-panel React/Recharts chart for the Residence Time Analysis
  (mcs-mcp:analyze_residence_time). Trigger on: "residence time chart", "Little's Law
  chart", "coherence gap chart/visualization", "sojourn time vs residence time",
  "sample path analysis chart", or any "show/chart/plot/visualize that" follow-up after
  an analyze_residence_time result is present. ONLY for analyze_residence_time output
  (Sample Path Analysis with finite Little's Law identity). Do NOT use for: cycle time
  stability (analyze_process_stability), throughput (analyze_throughput), WIP count
  stability (analyze_wip_stability), Total WIP Age stability (analyze_wip_age_stability),
  or individual item WIP Age (analyze_work_item_age). Always read this skill before
  building the chart — do not attempt it ad-hoc.
---

# Residence Time Chart

Scope: Use this skill ONLY when analyze_residence_time data is present in the
conversation. It visualises residence time w̄(T), sojourn time W*(T), the coherence
gap between them, and the cumulative departure rate λ(T) — all derived from the
finite Little's Law sample path analysis.

Do not use this skill for:
- Cycle time / Lead time stability        → analyze_process_stability
- Throughput stability                    → analyze_throughput
- WIP Count stability                     → analyze_wip_stability
- Total WIP Age stability                 → analyze_wip_age_stability
- Individual item WIP Age outliers        → analyze_work_item_age

---

## Prerequisites

```js
mcs-mcp:analyze_residence_time({
  board_id, project_key,
  granularity: "weekly",           // "daily" (default) or "weekly"
  history_window_weeks: 52,        // default 52
  issue_types: null,               // optional filter
})
```

API options note:
- `granularity`: `"weekly"` produces ~52 points for a 52-week window (manageable).
  `"daily"` produces ~365 points and will require downsampling to every 3rd–4th point.
- `issue_types`: if provided, filters the analysis to specific item types. Affects all
  series values but not the response structure.

IMPORTANT: This tool ALWAYS applies backflow reset (uses the LAST commitment date).
This diverges from the configurable `commitmentBackflowReset` used by other tools like
`analyze_work_item_age`. Note this in the footer.

---

## Response Structure

```
data.residence_time.series[]         — time series, one entry per period:
  .date                              — ISO datetime with timezone offset
  .label                             — period label e.g. "2025-W11", "2025-03-10"
  .n                                 — active WIP count at this point
  .h                                 — cumulative horizon (days or weeks elapsed)
  .l                                 — running average population L(T)
  .a                                 — cumulative arrivals into committed WIP
  .lambda                            — cumulative departure rate d/h
  .w                                 — average residence time w̄(T) — PRIMARY metric
  .d                                 — cumulative historical resolved count (W* denominator)
  .w_star                            — average sojourn time W*(T) (completed items only)
  .coherence_gap                     — w - w_star (rename to gap in injection)

data.residence_time.summary          — final-state snapshot:
  .final_w                           — final w̄(T)
  .final_w_star                      — final W*(T)
  .final_coherence_gap               — final coherence gap
  .final_lambda                      — final departure rate
  .final_l                           — final L(T)
  .active_items                      — active WIP count at window end
  .in_window_arrivals                — items entering committed WIP during window
  .pre_window_items                  — items already in WIP when window opened
  .departures                        — total historical resolved count (W* denominator)
  .convergence                       — "converging" / "diverging" / "metastable"

data.residence_time.validation
  .identity_verified                 — boolean (Little's Law identity holds)
```

---

## Critical Semantic Rules

NEVER confuse these fields:

```
a / in_window_arrivals  — items entering committed WIP during the window (window-scoped)
d / departures          — TOTAL HISTORICAL resolved count — W* denominator (all time)
pre_window_items        — items already in WIP when the window opened
```

`a` and `d` are NOT symmetric in/out counts for the same window. Presenting them
side-by-side as "Arrivals vs Departures" is MISLEADING — it falsely implies massive
WIP reduction. Always label `d` as `"(W* denominator)"` or `"total historical resolved"`.

`lambda` is the CUMULATIVE DEPARTURE RATE (d/h), not an "arrival rate".

---

## Data Preparation

Flatten the series into a compact array. Use `.label` as the date field (already
human-readable). Rename `.coherence_gap` to `.gap`.

```js
const RAW = series.map(d => ({
  date:   d.label,
  n:      d.n,
  w:      Math.round(d.w * 100) / 100,
  w_star: Math.round(d.w_star * 100) / 100,
  gap:    Math.round(d.coherence_gap * 100) / 100,
  lambda: Math.round(d.lambda * 10000) / 10000,
  a:      d.a,
}));
```

Downsampling: If granularity is `"daily"` and series has >120 points, keep every
3rd–4th point. Always retain the first and last points.
Weekly granularity (52 points) does not need downsampling.

Summary constants:

```js
const FINAL_W          = summary.final_w;
const FINAL_W_STAR     = summary.final_w_star;
const FINAL_GAP        = summary.final_coherence_gap;
const FINAL_LAMBDA     = summary.final_lambda;
const FINAL_N          = summary.active_items;
const WINDOW_ARRIVALS  = summary.in_window_arrivals;
const PRE_WINDOW_ITEMS = summary.pre_window_items;
const RESOLVED_TOTAL_D = summary.departures;
const CONVERGENCE      = summary.convergence;
```

---

## Color Tokens

```js
const ALARM     = "#ff6b6b";
const CAUTION   = "#e2c97e";
const PRIMARY   = "#6b7de8";
const SECONDARY = "#7edde2";
const POSITIVE  = "#6bffb8";
const TEXT      = "#dde1ef";
const MUTED     = "#505878";
const MUTED_DK  = "#4a5270";
const PAGE_BG   = "#080a0f";
const PANEL_BG  = "#0c0e16";
const BORDER    = "#1a1d2e";
```

Series color assignment:

```
w (residence):     PRIMARY   solid line, left Y-axis
w_star (sojourn):  CAUTION   dashed line "4 3", right Y-axis
gap (coherence):   ALARM     Area fill with gradient
lambda:            SECONDARY Area fill with gradient
λ = 1 reference:   MUTED     dashed "4 4"
```

Convergence badge color:

```
"converging"  → POSITIVE
"diverging"   → ALARM
"metastable"  → CAUTION
```

---

## Chart Architecture

Two ComposedChart panels stacked vertically, single page container.

### Panel 1: Residence Time vs Sojourn Time (Coherence Gap)

```
ComposedChart, height=380px, dual Y-axes
Margin: { top: 10, right: 70, left: 10, bottom: 10 }

Left Y-axis  (yAxisId="left"):  w̄ and gap — PRIMARY ticks
Right Y-axis (yAxisId="right"): W*          — CAUTION ticks

Y-axis domains:
  Left:  [0, ceil((maxW + 5) / 10) * 10]
  Right: [floor((minWStar - 2) / 5) * 5,  ceil((maxWStar + 2) / 5) * 5]

Y-axis tick format: `${v}d` on both axes
Left Y-axis label:  "Days"      angle=-90
Right Y-axis label: "W* (days)" angle=90
```

Gradient definitions (inside `<defs>`):

```jsx
<linearGradient id="gapGrad" x1="0" y1="0" x2="0" y2="1">
  <stop offset="5%"  stopColor={ALARM}   stopOpacity={0.20} />
  <stop offset="95%" stopColor={ALARM}   stopOpacity={0.02} />
</linearGradient>
<linearGradient id="wGrad" x1="0" y1="0" x2="0" y2="1">
  <stop offset="5%"  stopColor={PRIMARY} stopOpacity={0.25} />
  <stop offset="95%" stopColor={PRIMARY} stopOpacity={0.02} />
</linearGradient>
```

Series (render order: Area first, then Lines — prevents Area overdrawing lines):

```jsx
<Area  yAxisId="left"  dataKey="gap"    fill="url(#gapGrad)"
       stroke={ALARM}   strokeWidth={1.5} strokeOpacity={0.7} dot={false} />
<Line  yAxisId="left"  dataKey="w"      stroke={PRIMARY} strokeWidth={2} dot={false} />
<Line  yAxisId="right" dataKey="w_star" stroke={CAUTION} strokeWidth={2}
       strokeDasharray="4 3" dot={false} />
```

Panel subtitle: `"Residence Time w̄(T) vs Sojourn Time W*(T) — Coherence Gap"`

### Panel 2: Departure Rate λ(T)

```
ComposedChart, height=260px, single Y-axis
Margin: { top: 10, right: 70, left: 10, bottom: 10 }

Y-axis domain: [0, ceil((maxLambda + 1) / 2) * 2]
Y-axis tick format: v.toFixed(1)
Y-axis label: "λ (dep/wk)" or "λ (dep/d)" depending on granularity — SECONDARY
```

Gradient:

```jsx
<linearGradient id="lambdaGrad" x1="0" y1="0" x2="0" y2="1">
  <stop offset="5%"  stopColor={SECONDARY} stopOpacity={0.25} />
  <stop offset="95%" stopColor={SECONDARY} stopOpacity={0.02} />
</linearGradient>
```

Series:

```jsx
<ReferenceLine y={1.0} stroke={MUTED} strokeDasharray="4 4"
  label={{ value: "λ = 1", fill: MUTED, fontSize: 10, position: "right" }} />
<Area dataKey="lambda" fill="url(#lambdaGrad)"
      stroke={SECONDARY} strokeWidth={2} dot={false} />
```

Panel subtitle: `"Departure Rate λ(T) — Cumulative departures per [week/day] (d / horizon)"`

---

## X-Axis Formatting

For weekly labels (`"2025-W11"`):

```js
function formatDate(label) {
  if (!label) return "";
  const [yr, wk] = label.split("-");
  return `${wk} '${yr.slice(2)}`;   // → "W11 '25"
}
```

For daily labels (`"2025-03-10"`):

```js
function formatDate(label) {
  if (!label) return "";
  const d = new Date(label);
  return d.toLocaleDateString("en-GB", { day: "2-digit", month: "short" });  // "10 Mar"
}
```

```
Use interval={3} for weekly data; interval={6} for daily data.
```

---

## Custom Tooltip

Dark background, two-column grid layout. Applied to both panels via shared component.

```jsx
// { active, payload } =>
//   d = payload[0].payload

// Header: d.date (bold)
// Grid rows:
"Residence w̄(T)"  PRIMARY   {d.w?.toFixed(1)}d
"Sojourn W*(T)"    CAUTION   {d.w_star?.toFixed(1)}d
"Gap w̄−W*"        ALARM     {d.gap?.toFixed(1)}d
──── divider ────
"λ(T)"             SECONDARY {d.lambda?.toFixed(2)}/[wk|d]
"Active (n)"       MUTED     {d.n}
"Arrivals (a)"     MUTED     {d.a}
```

---

## Stat Cards (5 cards)

```
RESIDENCE w̄(T)   FINAL_W.toFixed(1) + "d"        PRIMARY
SOJOURN W*(T)    FINAL_W_STAR.toFixed(1) + "d"    CAUTION
COHERENCE GAP    FINAL_GAP.toFixed(1) + "d"       ALARM
λ(T)             FINAL_LAMBDA.toFixed(2) + "/wk"  SECONDARY
ACTIVE WIP       FINAL_N                           MUTED
```

---

## Badge Row (5 badges, horizontal flex)

```
1. "Window arrivals (a): {WINDOW_ARRIVALS}"                   PRIMARY
2. "Pre-window items: {PRE_WINDOW_ITEMS}"                     MUTED
3. "Resolved (d): {RESOLVED_TOTAL_D} (W* denominator)"       CAUTION
4. "L(T) = Λ(T) · w(T) ✓"                                   CAUTION
5. "Convergence: {CONVERGENCE}"                               convergenceColor
```

Badge 3 MUST label `d` as `"(W* denominator)"` — never `"Departures"` alone, which
implies a window-scoped outflow count.

---

## Legends

Manual legends below each panel — no Recharts Legend component.

```
Panel 1:
  SVG solid line PRIMARY 2px    → "Residence w̄(T)"
  SVG dashed line CAUTION "4 3" → "Sojourn W*(T)"
  Filled rect ALARM opacity 0.4 → "Coherence Gap"

Panel 2:
  Filled rect SECONDARY opacity 0.4 → "Departure Rate λ(T)"
  SVG dashed line MUTED "4 4"        → "λ = 1 (equilibrium)"
```

---

## Header

```
Breadcrumb: {PROJECT_KEY} · {BOARD_NAME} · Board {BOARD_ID}
Title:      exactly "Residence Time Analysis"
Subtitle:   "Sample Path Analysis · Little's Law L(T) = Λ(T) · w(T) · {first label} – {last label}"
```

---

## Footer — Two Sections (both required)

1. **"Reading this chart:"**
   Top panel: residence time w̄(T) (blue, left axis) — average time items spend in
   system including active WIP — vs sojourn time W*(T) (amber dashed, right axis) —
   average cycle time of completed items only. Red shaded area = coherence gap: how
   much active WIP inflates average residence time beyond completed-item experience.
   A diverging/growing gap signals aging WIP accumulation. Bottom panel: cumulative
   departure rate λ(T) = d(T)/h(T); above 1.0 means system resolves more than one
   item per elapsed period on average.

2. **"Data:"**
   Sample Path Analysis (Stidham 1972, El-Taha & Stidham 1999). Little's Law identity
   L(T) = Λ(T) · w(T) verified exactly. Window arrivals (a): items entering committed
   WIP during the window. Resolved (d): {RESOLVED_TOTAL_D} total historical departures
   used as W* denominator. Note: this tool always applies backflow reset (last commitment
   date), which may diverge from other tools using configurable backflow settings.

---

## Injection Checklist

```
Placeholder         Source path
BOARD_ID            board_id parameter
PROJECT_KEY         project_key parameter
BOARD_NAME          board name from context / import_boards
FINAL_W             data.residence_time.summary.final_w
FINAL_W_STAR        data.residence_time.summary.final_w_star
FINAL_GAP           data.residence_time.summary.final_coherence_gap
FINAL_LAMBDA        data.residence_time.summary.final_lambda
FINAL_N             data.residence_time.summary.active_items
WINDOW_ARRIVALS     data.residence_time.summary.in_window_arrivals
PRE_WINDOW_ITEMS    data.residence_time.summary.pre_window_items
RESOLVED_TOTAL_D    data.residence_time.summary.departures
CONVERGENCE         data.residence_time.summary.convergence
RAW array           data.residence_time.series[] →
                      { date: .label, n, w, w_star, gap: .coherence_gap, lambda, a }
                    Downsample if daily and >120 points (keep every 3rd–4th, retain first+last)
```

---

## Checklist Before Delivering

- [ ] Triggered by analyze_residence_time data
- [ ] Chart title reads exactly "Residence Time Analysis"
- [ ] Two panels: Panel 1 (Residence vs Sojourn, dual Y-axis) + Panel 2 (Departure Rate)
- [ ] Panel 1 has dual Y-axes: left for w̄(T) and gap, right for W*(T)
- [ ] Coherence gap rendered as red shaded Area (not a line)
- [ ] W*(T) rendered as amber dashed line on the right Y-axis
- [ ] Panel 2 shows λ(T) as cyan area with reference line at λ = 1
- [ ] λ(T) labeled as "Departure Rate" — NOT "Arrival Rate"
- [ ] Series render order: Area first, then Lines (prevents Area overdrawing lines)
- [ ] Both panels share the same CustomTooltip component
- [ ] All data embedded as JS literals — no fetch calls
- [ ] Badge 3 labels d as "(W* denominator)" — never just "Departures"
- [ ] a and d NOT presented as symmetric in/out window counts
- [ ] Convergence badge color: POSITIVE / ALARM / CAUTION
- [ ] Right margin 70px on both charts to prevent reference line label clipping
- [ ] X-axis formatted per granularity: weekly → "W11 '25", daily → "10 Mar"
- [ ] useMemo wrapping data array
- [ ] isAnimationActive={false} on all series
- [ ] Legends rendered manually — no Recharts Legend component
- [ ] Dark theme throughout: PAGE_BG page, PANEL_BG panels, BORDER grid
- [ ] Monospace font throughout
- [ ] Single self-contained .jsx file with default export
