---
name: total-wip-age-stability-chart
description: >
  Creates a dark-themed dual-axis React/Recharts chart for the **Total WIP Age Stability**
  analysis (mcs-mcp:analyze_wip_age_stability). Trigger on: "Total WIP Age chart/trend/stability",
  "XmR chart for WIP Age", "visualize WIP Age stability", or any "show/chart/plot that" follow-up
  after an analyze_wip_age_stability result is present.
  ONLY for analyze_wip_age_stability output (XmR on the daily *sum* of all active items' ages).
  Do NOT use for: individual item WIP Age (analyze_work_item_age), WIP Count stability
  (analyze_wip_stability), cycle time stability (analyze_process_stability), or throughput
  (analyze_throughput). Those are different analyses requiring different charts.
  Always read this skill before building the chart — do not attempt it ad-hoc.
---

# Total WIP Age Stability Chart

> **Scope of this skill:** This skill is exclusively for charting **Total WIP Age Stability**
> — the cumulative age burden of all in-progress items over time, analysed via XmR Process
> Behavior Chart limits. The secondary series (Average WIP Age) is included for convenience
> but is a supporting indicator only; the XmR control limits are always applied to the
> **Total** WIP Age, not the average.
>
> **Do not use this skill for:**
> - Individual item WIP Age outliers → use `analyze_work_item_age`
> - WIP Count stability → use `analyze_wip_stability`
> - Cycle time / Lead time stability → use `analyze_process_stability`
> - Throughput stability → use `analyze_throughput`

This skill produces a dark-themed, dual-axis React/Recharts chart visualising **Total WIP Age**
stability over time, including XmR Natural Process Limits (UNPL/LNPL) on the primary left axis
and Average WIP Age as a secondary reference line on the right axis.

---

## Prerequisites

The chart requires output from `mcs-mcp:analyze_wip_age_stability` — the **Total WIP Age
Stability** tool. Do not confuse this with `analyze_work_item_age` (individual item ages) or
`analyze_wip_stability` (WIP count stability); those produce different data and a different
chart is needed for them.

If the stability data is not yet in the conversation, call the tool first:

```js
mcs-mcp:analyze_wip_age_stability({ board_id, project_key })
```

The response will contain:
- `data.wip_age_stability.run_chart` — daily data points (date, **total_age**, count, average_age)
  → `total_age` is the sum of all active items' WIP ages on that day (primary series)
  → `average_age` is `total_age / count` (secondary reference series, right axis only)
- `data.wip_age_stability.xmr` — XmR limits applied to `total_age`:
  (average, upper_natural_process_limit, lower_natural_process_limit, signals)
- `data.wip_age_stability.status` — "stable" or "unstable"

---

## Data Preparation

Before writing JSX, extract and prepare these constants from the tool response:

| Constant | Source |
|---|---|
| `RAW` | `run_chart` array — keep all fields: `date`, `total_age`, `count`, `average_age` |
| `MEAN` | `xmr.average` |
| `UNPL` | `xmr.upper_natural_process_limit` |
| `LNPL` | `xmr.lower_natural_process_limit` |
| Signal dates | `xmr.signals` — extract `key` values, group into `aboveUnpl` and `belowLnpl` sets |

Annotate each data point:
```js
const data = RAW.map(d => ({
  ...d,
  aboveUnpl: signalsAbove.has(d.date),
  belowLnpl: signalsBelow.has(d.date),
  mean: MEAN,
  unpl: UNPL,
  lnpl: LNPL,
}));
```

If `run_chart` has more than ~180 points, downsample to every 2nd or 3rd point to keep the
chart readable, but **always retain all signal-breach points** (those in `xmr.signals`).

---

## Chart Architecture

Use `ComposedChart` from Recharts inside a `ResponsiveContainer`. The chart has two Y-axes:

| Axis | `yAxisId` | Orientation | Data | Color |
|---|---|---|---|---|
| Left | `"total"` | left | `total_age` (sum of all active items' ages) | amber `#e2c97e` ticks |
| Right | `"avg"` | right | `average_age` (total_age ÷ count, secondary reference only) | cyan `#7edde2` ticks |

### Series

1. **Area** (`yAxisId="total"`) — `total_age` with a subtle gradient fill and a solid stroke
   (`#6b7de8`). Custom dots: render a colored circle only on signal-breach points
   (red `#ff6b6b` for above UNPL, green `#6bffb8` for below LNPL).

2. **Line** (`yAxisId="avg"`, dashed) — `average_age` in cyan `#7edde2`, no regular dots.

### Reference Lines (all on `yAxisId="total"`)

| Line | Value | Color | Dash |
|---|---|---|---|
| UNPL | `UNPL` | `#ff6b6b` | `"6 3"` |
| Mean | `MEAN` | `#6b7de8` | `"4 4"` |
| LNPL | `LNPL` | `#6bffb8` | `"6 3"` |

Each reference line gets a short label (e.g. "UNPL", "Mean", "LNPL") rendered at the left edge.

---

## Visual Design

### Color Palette (dark theme)

```
Background:      #080a0f  (page)    / #0c0e16  (chart panel)
Panel border:    #1a1d2e
Grid lines:      #1a1d2e  (horizontal only, vertical=false)
Tick text:       #404660
Primary line:    #6b7de8  (indigo)
Avg line:        #7edde2  (cyan)
Signal above:    #ff6b6b  (red)
Signal below:    #6bffb8  (green)
Axis label left: #e2c97e  (amber)
Axis label right:#7edde2  (cyan)
Header text:     #dde1ef
Muted text:      #4a5270 / #505878
```

### Typography
Use `'Courier New', monospace` throughout — it matches the analytical, metrics-dashboard
aesthetic.

### Layout
- Full-width page container, max-width 1100px, centred
- **Header section** (above the chart panel):
  - Breadcrumb line: project name, board name, board ID (muted, uppercase, letter-spaced)
  - H1 title: **"Total WIP Age Stability"** (always use this exact title — not "WIP Age Stability")
  - Subtitle: "XmR Process Behavior Chart · {date range}"
  - Stat cards (top-right): Mean / UNPL / LNPL values with colored borders matching the
    reference line colors
- **Signal callout badges** below the header: one badge per notable signal episode plus the
  overall status verdict ("Status: UNSTABLE" / "Status: STABLE")
- **Chart panel**: dark background, rounded corners, 1px border
- **Footer note**: 1–2 lines explaining dot colors and dual-axis

### Gradient fill for the area series
```jsx
<defs>
  <linearGradient id="totalGrad" x1="0" y1="0" x2="0" y2="1">
    <stop offset="5%"  stopColor="#6b7de8" stopOpacity={0.25} />
    <stop offset="95%" stopColor="#6b7de8" stopOpacity={0.02} />
  </linearGradient>
</defs>
```

---

## Custom Components

### CustomTooltip
Shows on hover: formatted date, Total WIP Age, Avg WIP Age, WIP Count, and a colored warning
label if the point is a signal breach. Dark background (`#0f1117`), subtle border, monospace font.

### CustomDot
Renders for every data point on the area series. Only paints a visible circle for signal points:
- `aboveUnpl === true` → red filled circle, r=4
- `belowLnpl === true` → green filled circle, r=4
- otherwise → return `null` (no dot)

```jsx
const CustomDot = ({ cx, cy, payload }) => {
  if (payload.aboveUnpl) return <circle cx={cx} cy={cy} r={4} fill="#ff6b6b" />;
  if (payload.belowLnpl) return <circle cx={cx} cy={cy} r={4} fill="#6bffb8" />;
  return null;
};
```

---

## X-Axis Formatting

Format dates as `DD MMM` (e.g. "15 Oct") using `toLocaleDateString("en-GB", ...)`. Use
`interval` to avoid label crowding — a good rule of thumb: one label per ~5 data points.

---

## Y-Axis Domains

| Axis | Suggested domain |
|---|---|
| Left (`total`) | `[Math.floor((LNPL - 500) / 500) * 500, Math.ceil((max_total + 500) / 500) * 500]` |
| Right (`avg`) | `[Math.floor(min_avg / 10) * 10 - 10, Math.ceil(max_avg / 10) * 10 + 10]` |

Compute `min_avg`, `max_avg`, `max_total` from the data. This keeps the chart from being
clipped and gives a bit of breathing room on both sides.

Format left-axis ticks as `{(v/1000).toFixed(1)}k` (e.g. "5.1k").
Format right-axis ticks as `{v}d` (e.g. "95d").

---

## Header Stat Cards — How to populate

| Card | Value |
|---|---|
| Mean | `MEAN.toLocaleString()` + " d" |
| UNPL | `UNPL.toLocaleString()` + " d" |
| LNPL | `LNPL.toLocaleString()` + " d" |

Borders use the same colors as the corresponding reference lines, with 20% opacity
(hex suffix `33`).

---

## Signal Callout Badges — How to populate

Parse `xmr.signals` to build human-readable episode descriptions:

1. Group consecutive signal dates into date ranges (e.g. "Oct 15 – Nov 3, 2025").
2. Separate `aboveUnpl` and `belowLnpl` groups.
3. Render one badge per episode group plus one "Status: UNSTABLE / STABLE" badge.

Badge styles:
- Above UNPL: `background: #ff6b6b15`, `border: #ff6b6b40`, `color: #ff6b6b`
- Below LNPL: `background: #6bffb815`, `border: #6bffb840`, `color: #6bffb8`
- Status: `background: #e2c97e10`, `border: #e2c97e30`, `color: #e2c97e`

---

## Complete JSX Skeleton

```jsx
import { useMemo } from "react";
import {
  ComposedChart, Area, Line, XAxis, YAxis,
  CartesianGrid, Tooltip, ReferenceLine, Legend, ResponsiveContainer,
} from "recharts";

// --- constants from tool response ---
const RAW = [ /* ... */ ];
const MEAN = /* xmr.average */;
const UNPL = /* xmr.upper_natural_process_limit */;
const LNPL = /* xmr.lower_natural_process_limit */;

export default function WipAgeChart() {
  const data = useMemo(() => RAW.map(d => ({
    ...d,
    aboveUnpl: /* boolean */,
    belowLnpl: /* boolean */,
    mean: MEAN, unpl: UNPL, lnpl: LNPL,
  })), []);

  return (
    <div style={{ background: "#080a0f", minHeight: "100vh", ... }}>
      {/* Header */}
      {/* Stat cards */}
      {/* Signal badges */}

      <div style={{ background: "#0c0e16", borderRadius: 12, border: "1px solid #1a1d2e", ... }}>
        <ResponsiveContainer width="100%" height={420}>
          <ComposedChart data={data} margin={{ top: 10, right: 60, left: 10, bottom: 10 }}>
            <defs>
              <linearGradient id="totalGrad" ...> ... </linearGradient>
            </defs>
            <CartesianGrid strokeDasharray="3 3" stroke="#1a1d2e" vertical={false} />
            <XAxis dataKey="date" tickFormatter={formatDate} ... />
            <YAxis yAxisId="total" orientation="left" ... />
            <YAxis yAxisId="avg"   orientation="right" ... />
            <Tooltip content={<CustomTooltip />} />
            <ReferenceLine yAxisId="total" y={UNPL} stroke="#ff6b6b" ... />
            <ReferenceLine yAxisId="total" y={MEAN} stroke="#6b7de8" ... />
            <ReferenceLine yAxisId="total" y={LNPL} stroke="#6bffb8" ... />
            <Area  yAxisId="total" dataKey="total_age" fill="url(#totalGrad)" stroke="#6b7de8" dot={<CustomDot />} ... />
            <Line  yAxisId="avg"   dataKey="average_age" stroke="#7edde2" strokeDasharray="2 2" dot={false} ... />
            <Legend ... />
          </ComposedChart>
        </ResponsiveContainer>
      </div>

      {/* Footer note */}
    </div>
  );
}
```

---

## Checklist Before Delivering

- [ ] Skill was triggered by `analyze_wip_age_stability` data (not item age or WIP count data)
- [ ] Chart title reads exactly **"Total WIP Age Stability"**
- [ ] Left Y-axis label reads "Total WIP Age (days)" — not just "WIP Age"
- [ ] Right Y-axis label reads "Avg WIP Age (days)" — clearly secondary/supporting
- [ ] All `run_chart` data points are embedded as a JS array literal (no fetch calls)
- [ ] XmR constants (MEAN, UNPL, LNPL) are hardcoded from the tool response (applied to total_age)
- [ ] Signal points are annotated on the data and rendered as colored dots
- [ ] Both Y-axes present with correct orientation, domain, tick formatter, and axis label
- [ ] Three reference lines visible with correct labels
- [ ] Custom tooltip shows date, **Total** WIP Age, Avg WIP Age, WIP Count, and breach indicator
- [ ] Header includes breadcrumb, title, stat cards, and signal badges
- [ ] Dark background applied to both page and chart panel
- [ ] Monospace font used throughout
- [ ] Footer note explains the dot and dual-axis convention
- [ ] Output is a single self-contained `.jsx` file with a default export
