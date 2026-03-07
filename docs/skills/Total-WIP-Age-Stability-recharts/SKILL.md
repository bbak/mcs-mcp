---
name: total-wip-age-stability-chart
description: >
  Creates a dark-themed dual-axis React/Recharts chart for the **Total WIP Age Stability**
  analysis (mcs-mcp:analyze_wip_age_stability). Trigger on: "Total WIP Age chart/trend/stability",
  "XmR chart for WIP Age", "visualize WIP Age stability", or any "show/chart/plot that" follow-up
  after an analyze_wip_age_stability result is present.
  Use ONLY when analyze_wip_age_stability data is present in the conversation.
  Always read this skill before building the chart — do not attempt it ad-hoc.
---

# Total WIP Age Stability Chart

> **Also read:** `mcs-charts-base` SKILL before implementing. It defines the page/panel
> wrappers, color tokens, typography, header structure, badge system, CartesianGrid,
> tooltip base style, area gradient pattern, legend pattern, interactive controls, footer
> structure, and universal checklist items. This skill only specifies what is unique to
> this chart.

> **Scope:** Use this skill ONLY when `analyze_wip_age_stability` data is present in the
> conversation. It is exclusively for charting the cumulative age burden of all in-progress
> items over time, analysed via XmR Process Behavior Chart limits.
>
> The secondary series (Average WIP Age) is a supporting indicator only. XmR control limits
> are always applied to **Total** WIP Age, not the average.

---

## Prerequisites

Call the tool if data is not yet in the conversation:

```js
mcs-mcp:analyze_wip_age_stability({ board_id, project_key })
```

Response structure:
- `data.wip_age_stability.run_chart` — daily data points: `date`, `total_age` (sum of all
  active items' WIP ages), `count` (active item count), `average_age` (`total_age / count`)
- `data.wip_age_stability.xmr` — XmR limits applied to `total_age`:
  `average`, `upper_natural_process_limit`, `lower_natural_process_limit`, `signals`
- `data.wip_age_stability.status` — "stable" or "unstable"

---

## Data Preparation

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
  mean: MEAN, unpl: UNPL, lnpl: LNPL,
}));
```

**Downsampling:** If `run_chart` has more than ~180 points, downsample to every 2nd–3rd
point. Always retain all signal-breach points (those in `xmr.signals`).

---

## Chart Architecture

Single `ComposedChart` with two Y-axes:

| Axis | `yAxisId` | Orientation | Data | Tick color |
|---|---|---|---|---|
| Left | `"total"` | left | `total_age` (primary) | CAUTION `#e2c97e` |
| Right | `"avg"` | right | `average_age` (secondary reference) | SECONDARY `#7edde2` |

### Series

1. **Area** (`yAxisId="total"`) — `total_age`, PRIMARY indigo gradient fill, PRIMARY
   `#6b7de8` stroke. Custom dots: visible only on signal-breach points:
   - `aboveUnpl` → ALARM red `#ff6b6b`, r=4
   - `belowLnpl` → POSITIVE green `#6bffb8`, r=4
   - otherwise → `null`

2. **Line** (`yAxisId="avg"`, dashed) — `average_age`, SECONDARY `#7edde2`, no dots

### Reference Lines (all on `yAxisId="total"`)

| Line | Value | Color | Dash |
|---|---|---|---|
| UNPL | `UNPL` | ALARM `#ff6b6b` | `"6 3"` |
| Mean | `MEAN` | PRIMARY `#6b7de8` | `"4 4"` |
| LNPL | `LNPL` | POSITIVE `#6bffb8` | `"6 3"` |

Each reference line gets a short label rendered at the left edge.

### CustomDot

```jsx
const CustomDot = ({ cx, cy, payload }) => {
  if (payload.aboveUnpl) return <circle cx={cx} cy={cy} r={4} fill="#ff6b6b" />;
  if (payload.belowLnpl) return <circle cx={cx} cy={cy} r={4} fill="#6bffb8" />;
  return null;
};
```

### Y-Axis Domains

| Axis | Domain |
|---|---|
| Left (`total`) | `[Math.floor((LNPL - 500) / 500) * 500, Math.ceil((max_total + 500) / 500) * 500]` |
| Right (`avg`) | `[Math.floor(min_avg / 10) * 10 - 10, Math.ceil(max_avg / 10) * 10 + 10]` |

Compute `min_avg`, `max_avg`, `max_total` from the data.

**Tick formatters:**
- Left: `${(v/1000).toFixed(1)}k` (e.g. "5.1k")
- Right: `${v}d` (e.g. "95d")

**Chart height:** 420px

---

## Header

Per `mcs-charts-base` header structure, with these specifics:

- **Title:** exactly `"Total WIP Age Stability"` — never just "WIP Age Stability"
- **Subtitle:** `"XmR Process Behavior Chart · {date range}"`
- **Stat cards:**

| Label | Value | Color |
|---|---|---|
| `Mean` | `MEAN.toLocaleString() + " d"` | PRIMARY `#6b7de8` |
| `UNPL` | `UNPL.toLocaleString() + " d"` | ALARM `#ff6b6b` |
| `LNPL` | `LNPL.toLocaleString() + " d"` | POSITIVE `#6bffb8` |

Card borders use the corresponding color at 20% opacity (hex suffix `33`).

- **Badges:** Parse `xmr.signals` to build human-readable episode descriptions:
  1. Group consecutive signal dates into date ranges (e.g. "Oct 15 – Nov 3, 2025")
  2. One badge per episode — ALARM red for above UNPL, POSITIVE green for below LNPL
  3. Status verdict badge — CAUTION amber styling, "Status: UNSTABLE" or "Status: STABLE"

---

## Tooltip

Per `mcs-charts-base` tooltip base style, with these fields:

| Label | Value |
|---|---|
| Date (header, bold) | formatted long date |
| Total WIP Age | `{d.total_age.toLocaleString()} d` |
| Avg WIP Age | `{d.average_age.toFixed(1)} d` |
| WIP Count | `{d.count}` |
| Signal breach (if any) | ALARM red or POSITIVE green warning label |

---

## Legend

Per `mcs-charts-base` manual legend pattern:
- PRIMARY indigo area (25% opacity) → "Total WIP Age"
- SECONDARY cyan dashed line → "Avg WIP Age (secondary)"
- ALARM red dashed line → "UNPL"
- PRIMARY indigo dashed line → "Mean"
- POSITIVE green dashed line → "LNPL"
- ALARM red dot → "Above UNPL"
- POSITIVE green dot → "Below LNPL"

---

## Footer

1–2 lines explaining:
- Red dots = above UNPL signal breaches; green dots = below LNPL
- Right axis (cyan dashed) is average WIP age — secondary reference only; XmR limits apply to the total

---

## Chart-Specific Checklist

*(Universal items are in `mcs-charts-base`. Only chart-specific items listed here.)*

- [ ] Skill was triggered by `analyze_wip_age_stability` data
- [ ] Chart title reads exactly **"Total WIP Age Stability"**
- [ ] Left Y-axis label reads "Total WIP Age (days)" — not just "WIP Age"
- [ ] Right Y-axis label reads "Avg WIP Age (days)" — clearly secondary/supporting
- [ ] XmR limits (MEAN, UNPL, LNPL) applied to `total_age` — not `average_age`
- [ ] Three reference lines visible with correct labels and colors
- [ ] Signal points rendered as colored dots (ALARM red above UNPL, POSITIVE green below LNPL)
- [ ] Signal badge episodes correctly grouped into date ranges
- [ ] Status verdict badge present (STABLE or UNSTABLE)
- [ ] Tooltip shows Total WIP Age, Avg WIP Age, WIP Count, and breach indicator
- [ ] Data downsampled if >180 points — all signal-breach points retained
