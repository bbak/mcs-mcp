---
name: residence-time-chart
description: >
  Creates a dark-themed dual-panel React/Recharts chart for the **Residence Time Analysis**
  (mcs-mcp:analyze_residence_time). Trigger on: "residence time chart", "Little's Law chart",
  "coherence gap chart/visualization", "sojourn time vs residence time", "sample path analysis chart",
  or any "show/chart/plot/visualize that" follow-up after an analyze_residence_time result is present.
  ONLY for analyze_residence_time output (Sample Path Analysis with finite Little's Law identity).
  Do NOT use for: cycle time stability (analyze_process_stability), throughput (analyze_throughput),
  WIP count stability (analyze_wip_stability), Total WIP Age stability (analyze_wip_age_stability),
  or individual item WIP Age (analyze_work_item_age). Those are different analyses requiring
  different charts. Always read this skill before building the chart — do not attempt it ad-hoc.
---

# Residence Time Chart

> **Scope of this skill:** This skill is exclusively for charting **Residence Time Analysis**
> output — the Sample Path Analysis (Stidham 1972, El-Taha & Stidham 1999) that computes the
> finite version of Little's Law: L(T) = Λ(T) · w(T). The chart visualises residence time w̄(T),
> sojourn time W*(T), the coherence gap between them, and the cumulative departure rate λ(T).
>
> **Do not use this skill for:**
> - Cycle time / Lead time stability → use `analyze_process_stability`
> - Throughput stability → use `analyze_throughput`
> - WIP Count stability → use `analyze_wip_stability`
> - Total WIP Age stability → use `analyze_wip_age_stability`
> - Individual item WIP Age outliers → use `analyze_work_item_age`

This skill produces a dark-themed, two-panel React/Recharts dashboard visualising the
**Residence Time** sample path analysis over time.

---

## Prerequisites

The chart requires output from `mcs-mcp:analyze_residence_time`. If the data is not yet in
the conversation, call the tool first:

```js
mcs-mcp:analyze_residence_time({ board_id, project_key })
```

The response will contain:
- `data.residence_time.series` — daily data points with fields:
  `date`, `n` (active WIP count), `w` (average residence time), `w_star` (average sojourn time),
  `coherence_gap` (w − w_star), `lambda` (cumulative departure rate d/h), `a` (cumulative arrivals),
  `d` (cumulative departures / historical resolved count), `h` (cumulative horizon days),
  `l` (running average population)
- `data.residence_time.summary` — final values:
  `final_w`, `final_w_star`, `final_coherence_gap`, `final_lambda`, `final_l`,
  `active_items`, `in_window_arrivals`, `pre_window_items`, `departures`,
  `convergence` ("converging" or "diverging")
- `data.residence_time.validation` — `identity_verified` (boolean)

---

## Critical Semantic Notes

**IMPORTANT — Do not confuse these fields:**

| Field | Meaning | Scope |
|---|---|---|
| `a` / `in_window_arrivals` | Items entering committed WIP during the observation window | Window-scoped |
| `d` / `departures` | Total historical resolved item count — denominator for W* calculation | Historical (all time) |
| `pre_window_items` | Items already in WIP when the window opened | Pre-window |

`a` and `d` are **NOT** symmetric in/out counts for the same window. `d` is the cumulative
historical denominator for sojourn time W*. Presenting them side-by-side as "Arrivals vs
Departures" is **misleading** — it would falsely suggest massive WIP reduction. Always label
them correctly and separately.

Similarly, `lambda` (λ = d/h) is the **cumulative departure rate**, not an "arrival rate."

---

## Data Preparation

Before writing JSX, extract and prepare these from the tool response:

| Constant | Source |
|---|---|
| `RAW` | `series` array — keep: `date`, `n`, `w`, `w_star`, `coherence_gap` (rename to `gap`), `lambda`, `a` |
| `FINAL_W` | `summary.final_w` |
| `FINAL_W_STAR` | `summary.final_w_star` |
| `FINAL_GAP` | `summary.final_coherence_gap` |
| `FINAL_LAMBDA` | `summary.final_lambda` |
| `FINAL_N` | `summary.active_items` |
| `WINDOW_ARRIVALS` | `summary.in_window_arrivals` |
| `PRE_WINDOW_ITEMS` | `summary.pre_window_items` |
| `RESOLVED_TOTAL_D` | `summary.departures` |
| `CONVERGENCE` | `summary.convergence` |

**Downsampling:** If `series` has more than ~120 points, downsample to every 3rd–4th point to
keep the artifact manageable. Always retain the first and last points.

Prepare each data point:
```js
const data = RAW.map(d => ({
  date: d.label || d.date.slice(0, 10),
  n: d.n,
  w: round(d.w, 2),         // or use the raw field name from the response
  w_star: round(d.w_star, 2),
  gap: round(d.coherence_gap, 2),
  lambda: round(d.lambda, 4),
  a: d.a,
}));
```

---

## Chart Architecture

The dashboard has **two chart panels** stacked vertically inside a single dark page container.

### Panel 1: Residence Time vs Sojourn Time (Coherence Gap)

Uses `ComposedChart` with two Y-axes:

| Axis | `yAxisId` | Orientation | Data | Tick color |
|---|---|---|---|---|
| Left | `"left"` | left | `w` (residence) and `gap` (coherence gap) | indigo `#6b7de8` |
| Right | `"right"` | right | `w_star` (sojourn time) | amber `#e2c97e` |

**Series:**

1. **Area** (`yAxisId="left"`) — `gap` (coherence gap) with a red gradient fill (`#ff6b6b`
   at 20% → 2% opacity), red stroke at 0.7 opacity, no dots.

2. **Line** (`yAxisId="left"`) — `w` (residence time) in indigo `#6b7de8`, strokeWidth 2, no dots.

3. **Line** (`yAxisId="right"`, dashed `"4 3"`) — `w_star` (sojourn time) in amber `#e2c97e`,
   strokeWidth 2, no dots.

**Gradient definitions:**
```jsx
<defs>
  <linearGradient id="wGrad" x1="0" y1="0" x2="0" y2="1">
    <stop offset="5%"  stopColor="#6b7de8" stopOpacity={0.25} />
    <stop offset="95%" stopColor="#6b7de8" stopOpacity={0.02} />
  </linearGradient>
  <linearGradient id="gapGrad" x1="0" y1="0" x2="0" y2="1">
    <stop offset="5%"  stopColor="#ff6b6b" stopOpacity={0.20} />
    <stop offset="95%" stopColor="#ff6b6b" stopOpacity={0.02} />
  </linearGradient>
</defs>
```

**Y-axis domains:**
- Left: `[0, Math.ceil((maxW + 5) / 10) * 10]` — start at 0, round up
- Right: `[Math.floor((minWStar - 2) / 5) * 5, Math.ceil((maxWStar + 2) / 5) * 5]`

**Y-axis tick formatting:**
- Left: `${v}d`
- Right: `${v}d`

**Left axis label:** `"Days"` (angle -90)
**Right axis label:** `"W* (days)"` (angle 90)

### Panel 2: Departure Rate λ(T)

Uses `ComposedChart` with a single Y-axis.

**Series:**

1. **Area** — `lambda` with a cyan gradient fill (`#7edde2` at 20% → 2% opacity), solid
   cyan stroke, strokeWidth 2, no dots.

2. **ReferenceLine** — at `y={1.0}`, stroke `#505878`, dashed `"4 4"`, with label `"λ = 1"`.

**Y-axis domain:** `[0, 1.6]` (or adjust based on data max)
**Y-axis tick formatting:** `v.toFixed(1)`
**Axis label:** `"λ (dep/d)"` (angle -90, cyan)

**Height:** 260px (shorter than Panel 1 at 380px)

---

## Visual Design

### Color Palette (dark theme)

```
Background:       #080a0f  (page)    / #0c0e16  (chart panels)
Panel border:     #1a1d2e
Grid lines:       #1a1d2e  (horizontal only, vertical=false)
Tick text:        #404660
Residence line:   #6b7de8  (indigo)
Sojourn line:     #e2c97e  (amber, dashed)
Coherence gap:    #ff6b6b  (red area fill)
Lambda line:      #7edde2  (cyan)
Header text:      #dde1ef
Muted text:       #4a5270 / #505878
```

### Typography
Use `'Courier New', monospace` throughout.

### Layout
- Full-width page container, max-width 1100px, centred
- **Header section** (above chart panels):
  - Breadcrumb line: project key, board name, board ID (muted, uppercase, letter-spaced)
  - H1 title: **"Residence Time Analysis"**
  - Subtitle: "Sample Path Analysis (Little's Law) · {date range}"
  - Stat cards (flex row): w̄(T) / W*(T) / Gap / λ(T)
- **Status badges** below header: convergence trend, active WIP, window arrivals + pre-window,
  resolved (d) with "(W* denominator)" label, Little's Law identity check
- **Panel 1**: Residence vs Sojourn with coherence gap
- **Panel 2**: Departure rate λ(T)
- **Footer note**: Explanation of chart reading + data provenance

---

## Header Stat Cards

| Card | Label | Value | Color |
|---|---|---|---|
| 1 | `Residence w̄(T)` | `${FINAL_W.toFixed(1)}d` | `#6b7de8` |
| 2 | `Sojourn W*(T)` | `${FINAL_W_STAR.toFixed(1)}d` | `#e2c97e` |
| 3 | `Gap w̄−W*` | `${FINAL_GAP.toFixed(1)}d` | `#ff6b6b` |
| 4 | `λ(T)` | `${FINAL_LAMBDA.toFixed(2)}/d` | `#7edde2` |

Card style: dark background `#0c0e16`, border `1px solid ${color}33`, rounded 8px,
padding `8px 14px`, min-width 100.

---

## Status Badges

Render as a horizontal flex row of pill-shaped badges:

1. **Convergence trend**: red if "diverging", green if "converging"
   - Diverging: `background: #ff6b6b15`, `border: #ff6b6b40`, `color: #ff6b6b`
   - Converging: `background: #6bffb815`, `border: #6bffb840`, `color: #6bffb8`
   - Text: `Trend: {CONVERGENCE}`

2. **Active WIP**: cyan `#7edde2` styling
   - Text: `Active WIP: {FINAL_N}`

3. **Window context**: indigo `#6b7de8` styling
   - Text: `Window Arrivals: {WINDOW_ARRIVALS} · Pre-Window: {PRE_WINDOW_ITEMS}`

4. **Resolved denominator**: muted `#505878` styling
   - Text: `Resolved (d): {RESOLVED_TOTAL_D} (W* denominator)`

5. **Identity check**: amber `#e2c97e` styling
   - Text: `L(T) = Λ(T) · w(T) ✓`

---

## Custom Components

### CustomTooltip

Shows on hover with dark background `#0f1117`, subtle border `#1a1d2e`, monospace font.
Grid layout with two columns:

| Row | Label (colored) | Value |
|---|---|---|
| Date | (header row, bold) | formatted long date |
| Residence (w̄) | `#6b7de8` | `{d.w.toFixed(1)} d` |
| Sojourn (W*) | `#e2c97e` | `{d.w_star.toFixed(1)} d` |
| Gap (w̄ − W*) | `#ff6b6b` | `{d.gap.toFixed(1)} d` |
| ──────── | separator | |
| λ(T) | `#7edde2` | `{d.lambda.toFixed(2)} /d` |
| Active (n) | `#505878` | `{d.n}` |
| Arrivals (a) | `#505878` | `{d.a}` |

### Manual Legends (below each chart panel)

Use inline SVG icons (24×12) with corresponding labels. Do NOT use the built-in Recharts
`<Legend>` component — render manually for design consistency.

**Panel 1 legend:**
- Blue solid line → "Residence w̄(T)"
- Amber dashed line → "Sojourn W*(T)"
- Red filled rect (opacity 0.25) → "Coherence Gap"

**Panel 2 legend:**
- Cyan filled rect (opacity 0.25) → "Departure Rate λ(T)"
- Grey dashed line → "λ = 1 (equilibrium)"

---

## X-Axis Formatting

Format dates as `DD MMM` (e.g. "15 Oct") using `toLocaleDateString("en-GB", ...)`.
Use `interval={7}` (or adjust based on data density) to avoid crowding.

---

## Footer Note

1–3 lines explaining chart interpretation:

> **Reading this chart:** The top chart shows residence time w̄(T) (blue, left axis) — the
> average time items spend in the system including active WIP — against sojourn time W*(T)
> (amber dashed, right axis) — the average cycle time of completed items only. The red shaded
> area is the coherence gap: how much active WIP inflates average residence time beyond
> completed-item experience. A diverging gap signals aging WIP accumulation. The bottom chart
> tracks the cumulative departure rate λ(T) = d(T)/h(T); values above 1.0 mean the system
> resolves more than one item per elapsed day on average.
>
> Data: Sample Path Analysis (Stidham 1972, El-Taha & Stidham 1999) · Little's Law identity
> L(T) = Λ(T) · w(T) verified exactly. Window arrivals (a): items entering committed WIP
> during the window. Resolved (d): total historical departures used as W* denominator.

---

## Complete JSX Skeleton

```jsx
import { useMemo } from "react";
import {
  ComposedChart, Area, Line, XAxis, YAxis,
  CartesianGrid, Tooltip, ReferenceLine, ResponsiveContainer,
} from "recharts";

// --- constants from tool response ---
const RAW = [ /* downsampled series data */ ];
const FINAL_W = /* summary.final_w */;
const FINAL_W_STAR = /* summary.final_w_star */;
const FINAL_GAP = /* summary.final_coherence_gap */;
const FINAL_LAMBDA = /* summary.final_lambda */;
const FINAL_N = /* summary.active_items */;
const WINDOW_ARRIVALS = /* summary.in_window_arrivals */;
const PRE_WINDOW_ITEMS = /* summary.pre_window_items */;
const RESOLVED_TOTAL_D = /* summary.departures */;
const CONVERGENCE = /* summary.convergence */;
const WINDOW_START = /* first date */;
const WINDOW_END = /* last date */;

const formatDate = (d) => { /* DD MMM format */ };
const formatDateLong = (d) => { /* DD MMM YYYY format */ };

const CustomTooltip = ({ active, payload }) => {
  /* see CustomTooltip section above */
};

export default function ResidenceTimeChart() {
  const data = useMemo(() => RAW, []);

  /* compute y-axis domains from data */

  const statCards = [
    { label: "Residence w̄(T)", value: `${FINAL_W.toFixed(1)}d`, color: "#6b7de8" },
    { label: "Sojourn W*(T)", value: `${FINAL_W_STAR.toFixed(1)}d`, color: "#e2c97e" },
    { label: "Gap w̄−W*", value: `${FINAL_GAP.toFixed(1)}d`, color: "#ff6b6b" },
    { label: "λ(T)", value: `${FINAL_LAMBDA.toFixed(2)}/d`, color: "#7edde2" },
  ];

  return (
    <div style={{ background: "#080a0f", minHeight: "100vh", padding: "32px 24px",
      fontFamily: "'Courier New', monospace", color: "#dde1ef" }}>
      <div style={{ maxWidth: 1100, margin: "0 auto" }}>
        {/* Breadcrumb */}
        {/* Header with title + stat cards */}
        {/* Status badges */}

        {/* Panel 1: Residence vs Sojourn */}
        <div style={{ background: "#0c0e16", borderRadius: 12, border: "1px solid #1a1d2e",
          padding: "20px 12px 12px 12px", marginBottom: 20 }}>
          <div style={{ fontSize: 11, color: "#505878", marginBottom: 12, paddingLeft: 8 }}>
            Residence Time w̄(T) vs Sojourn Time W*(T) — Coherence Gap
          </div>
          <ResponsiveContainer width="100%" height={380}>
            <ComposedChart data={data} margin={{ top: 10, right: 60, left: 10, bottom: 10 }}>
              <defs>
                <linearGradient id="wGrad" ...> ... </linearGradient>
                <linearGradient id="gapGrad" ...> ... </linearGradient>
              </defs>
              <CartesianGrid strokeDasharray="3 3" stroke="#1a1d2e" vertical={false} />
              <XAxis dataKey="date" tickFormatter={formatDate} ... />
              <YAxis yAxisId="left" orientation="left" ... />
              <YAxis yAxisId="right" orientation="right" ... />
              <Tooltip content={<CustomTooltip />} />
              <Area yAxisId="left" dataKey="gap" fill="url(#gapGrad)" stroke="#ff6b6b"
                strokeWidth={1.5} strokeOpacity={0.7} dot={false} />
              <Line yAxisId="left" dataKey="w" stroke="#6b7de8" strokeWidth={2} dot={false} />
              <Line yAxisId="right" dataKey="w_star" stroke="#e2c97e" strokeWidth={2}
                strokeDasharray="4 3" dot={false} />
            </ComposedChart>
          </ResponsiveContainer>
          {/* Manual legend */}
        </div>

        {/* Panel 2: Departure Rate λ(T) */}
        <div style={{ background: "#0c0e16", borderRadius: 12, border: "1px solid #1a1d2e",
          padding: "20px 12px 12px 12px", marginBottom: 20 }}>
          <div style={{ fontSize: 11, color: "#505878", marginBottom: 12, paddingLeft: 8 }}>
            Departure Rate λ(T) — Cumulative departures per day (d / horizon)
          </div>
          <ResponsiveContainer width="100%" height={260}>
            <ComposedChart data={data} margin={{ top: 10, right: 60, left: 10, bottom: 10 }}>
              <defs>
                <linearGradient id="lambdaGrad" ...> ... </linearGradient>
              </defs>
              <CartesianGrid ... />
              <XAxis ... />
              <YAxis domain={[0, 1.6]} tickFormatter={(v) => v.toFixed(1)} ... />
              <Tooltip content={<CustomTooltip />} />
              <ReferenceLine y={1.0} stroke="#505878" strokeDasharray="4 4"
                label={{ value: "λ = 1", ... }} />
              <Area dataKey="lambda" fill="url(#lambdaGrad)" stroke="#7edde2"
                strokeWidth={2} dot={false} />
            </ComposedChart>
          </ResponsiveContainer>
          {/* Manual legend */}
        </div>

        {/* Footer note */}
      </div>
    </div>
  );
}
```

---

## Checklist Before Delivering

- [ ] Skill was triggered by `analyze_residence_time` data (not cycle time, WIP count, or throughput data)
- [ ] Chart title reads exactly **"Residence Time Analysis"**
- [ ] Two chart panels: Panel 1 (Residence vs Sojourn) and Panel 2 (Departure Rate)
- [ ] Panel 1 has dual Y-axes: left for w̄(T) and gap, right for W*(T)
- [ ] Coherence gap rendered as red shaded Area, NOT a line
- [ ] W*(T) rendered as amber dashed line on the right axis
- [ ] Panel 2 shows λ(T) as cyan area with a reference line at λ = 1
- [ ] λ(T) is labeled as **Departure Rate**, NOT "Arrival Rate"
- [ ] All `series` data points are embedded as a JS array literal (no fetch calls)
- [ ] Summary constants hardcoded from tool response
- [ ] Badges correctly distinguish window arrivals (a) from historical resolved count (d)
- [ ] Badges do NOT present `a` and `d` as symmetric in/out counts
- [ ] `d` badge is labeled as "(W* denominator)", not as "Departures"
- [ ] Convergence badge shows "diverging" (red) or "converging" (green)
- [ ] Custom tooltip shows date, w̄, W*, gap, λ, n, a
- [ ] Header includes breadcrumb, title, stat cards, and status badges
- [ ] Dark background applied to page and both chart panels
- [ ] Monospace font used throughout
- [ ] Footer note explains chart reading and data provenance
- [ ] Data downsampled if >120 points (retain first/last)
- [ ] Output is a single self-contained `.jsx` file with a default export
