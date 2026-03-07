---
name: cycle-time-sle-chart
description: >
  Creates a dark-themed React/Recharts chart for the **Cycle Time SLE Distribution**
  analysis (mcs-mcp:analyze_cycle_time). Trigger on: "cycle time SLE chart",
  "cycle time distribution", "SLE chart", "service level expectation chart",
  "cycle time percentiles", "how long do items take", or any
  "show/chart/plot/visualize that" follow-up after an analyze_cycle_time result is present.
  ONLY for analyze_cycle_time output (SLE percentile distributions per issue type).
  Do NOT use for: cycle time stability/XmR run chart (analyze_process_stability),
  throughput stability (analyze_throughput), WIP count stability (analyze_wip_stability),
  Total WIP Age (analyze_wip_age_stability), or process evolution (analyze_process_evolution).
  Those are different analyses requiring different charts.
  Always read this skill AND mcs-charts-base before building the chart — do not attempt
  it ad-hoc.
---

# Cycle Time — SLE Distribution Chart

> **Scope:** Use this skill ONLY when `analyze_cycle_time` data is present in the
> conversation. It visualizes the Service Level Expectation (SLE) percentile spectrum
> per issue type as stacked range segments, with a companion grouped bar view and a
> full SLE reference table.
>
> **This skill extends `mcs-charts-base`.** Read that skill first. Everything defined
> there (stack, color tokens, typography, page/panel wrappers, stat card markup, badge
> system, CartesianGrid, tooltip base style, legend pattern, interactive controls,
> footer style, universal checklist) applies here without repetition. This skill only
> specifies what is unique to the cycle time SLE chart.

---

## Prerequisites

Call the tool if data is not yet in the conversation:

```js
mcs-mcp:analyze_cycle_time({ board_id, project_key })
// Optional: issue_types, start_status, end_status
```

---

## Response Structure

```
data.percentiles                      — overall SLE percentiles (all types pooled)
  .aggressive     — P10  (fast outliers)
  .unlikely       — P30
  .coin_toss      — P50  (median)
  .probable       — P70
  .likely         — P85  ★ PRIMARY SLE — most important threshold
  .conservative   — P90
  .safe           — P95
  .almost_certain — P98  (extreme outliers)

data.spread
  .iqr            — interquartile range (P25–P75)
  .inner_80       — P10–P90 range width

data.fat_tail_ratio         — P98 / P50 ratio (≥ 5.6 = fat tail warning)
data.tail_to_median_ratio   — P85 / P50 ratio (> 3 = high volatility)
data.predictability         — string: "Stable", "Marginal", or "Unstable & Volatile"

data.context
  .days_in_sample           — analysis window in days
  .issues_analyzed          — items included in the percentile model
  .issues_total             — total items in window before filtering
  .dropped_by_outcome       — items excluded (abandoned resolutions)
  .throughput_overall       — avg items/week over the window
  .throughput_recent        — avg items/week in recent period
  .type_counts              — { Story: n, Activity: n, Bug: n, Defect: n }
  .stratification_decisions — array, one per type:
      .type      — issue type name
      .eligible  — boolean (false = volume too low, excluded from type_sles)
      .reason    — human-readable exclusion note if not eligible
      .volume    — item count

data.throughput_trend
  .direction        — "Increasing" | "Decreasing" | "Stable"
  .percentage_change — float (e.g. 58.3 = 58% above average recently)

data.type_sles                        — per-type SLE percentiles (same keys as percentiles)
  .Story    { aggressive, unlikely, coin_toss, probable, likely, conservative, safe, almost_certain }
  .Activity { ... }
  .Bug      { ... }
  .Defect   { ... }   ← only present if eligible (volume ≥ 15 items)
```

> **Important — Overall vs. type_sles:**
> `data.percentiles` contains the overall (pooled) SLE. `data.type_sles` contains
> per-type SLEs. To display "Overall" alongside types, merge them manually:
> `const ALL_SLES = { Overall: data.percentiles, ...data.type_sles }`.
> Only include a type in charts if its `stratification_decisions[].eligible === true`.

---

## Data Preparation

```js
// Merge Overall into the type map
const ALL_SLES = { Overall: data.percentiles, ...data.type_sles };

// Only types with eligible: true in stratification_decisions
const ELIGIBLE_TYPES = data.context.stratification_decisions
  .filter(d => d.eligible)
  .map(d => d.type);
// e.g. ["Story", "Activity", "Bug"]

// Percentile key definitions — ordered P10 → P98
const PERCENTILE_KEYS = [
  { key: "aggressive",     label: "P10",        short: "P10"     },
  { key: "unlikely",       label: "P30",        short: "P30"     },
  { key: "coin_toss",      label: "P50",        short: "P50"     },
  { key: "probable",       label: "P70",        short: "P70"     },
  { key: "likely",         label: "P85 ★ SLE",  short: "P85 SLE" },
  { key: "conservative",   label: "P90",        short: "P90"     },
  { key: "safe",           label: "P95",        short: "P95"     },
  { key: "almost_certain", label: "P98",        short: "P98"     },
];

// Spectrum data — one row per eligible type, 4 stacked segments
const spectrumData = ELIGIBLE_TYPES.map(type => {
  const s = ALL_SLES[type];
  return {
    type,
    p10: s.aggressive,
    p50: s.coin_toss,
    p85: s.likely,
    p95: s.safe,
    seg_0_p10:   s.aggressive,
    seg_p10_p50: s.coin_toss    - s.aggressive,
    seg_p50_p85: s.likely       - s.coin_toss,
    seg_p85_p95: s.safe         - s.likely,
    n: data.context.type_counts[type] || "—",
  };
});

// Grouped data — one entry per percentile, one value per eligible type
const groupedData = PERCENTILE_KEYS.map(({ key, label, short }) => {
  const entry = { label, short };
  ELIGIBLE_TYPES.forEach(t => { entry[t] = Math.round(ALL_SLES[t][key] * 10) / 10; });
  return entry;
});
```

---

## Issue Type Colors

These are fixed semantic colors for issue types, used consistently across all MCS chart skills:

```js
const TYPE_COLORS = {
  Overall:  "#dde1ef",   // TEXT
  Story:    "#6b7de8",   // PRIMARY
  Activity: "#7edde2",   // SECONDARY
  Bug:      "#ff6b6b",   // ALARM
  Defect:   "#e2c97e",   // CAUTION
};
```

---

## Chart Architecture

Two independent views toggled by the user, plus a permanent reference table below both.

### View A: Spectrum View (default)

**Horizontal stacked `BarChart`** (`layout="vertical"`), one row per eligible type.

Stacked segments left → right with fixed semantic color assignment:

| Segment | dataKey | Color | Meaning |
|---|---|---|---|
| 0 → P10 | `seg_0_p10` | POSITIVE `#6bffb8` | Fast outliers |
| P10 → P50 | `seg_p10_p50` | SECONDARY `#7edde2` | Typical fast delivery |
| P50 → P85 | `seg_p50_p85` | CAUTION `#e2c97e` | Normal working range |
| P85 → P95 | `seg_p85_p95` | ALARM `#ff6b6b` | Tail risk |

Add a `<LabelList>` on the last segment (`seg_p85_p95`), `position="right"`,
formatter `v => v + "d"`, showing the P95 value at the right edge of each bar.

X-axis: `type="number"`, `tickFormatter={v => v + "d"}`.
Y-axis: `type="category"`, `dataKey="type"`, `width={65}`.
Height: `eligibleTypes.length * 80 + 40` px.

### View B: Grouped View

**Vertical `BarChart`**, X-axis = percentile `short` labels, one `<Bar>` per eligible type.
Use `TYPE_COLORS` for bar fills. `radius={[3,3,0,0]}`. No stacking.
Height: 360px.

### SLE Reference Table (always visible, below both views)

A rendered `<table>` (not a chart). Columns: Percentile | Overall | then one column per
eligible type. Rows: one per PERCENTILE_KEYS entry.

Highlight the P85 row: amber background tint `${CAUTION}08`, text color CAUTION `#e2c97e`,
bold, label reads `"P85 ★ SLE"`. All other rows use TEXT color `#dde1ef`.

---

## Header (extends base skill header structure)

- **Breadcrumb:** `{PROJECT_KEY} · {board name} · Board {board_id}`
- **Title:** exactly `"Cycle Time — SLE Distribution"`
- **Subtitle:** `"Service Level Expectation Percentiles · {days_in_sample}-day window · {issues_analyzed} items delivered"`

**Stat cards** (all values from `data.percentiles` — Overall):

| Label | Value | Color |
|---|---|---|
| `P50 MEDIAN` | `{coin_toss.toFixed(0)}d` | SECONDARY `#7edde2` |
| `P85 SLE` | `{likely.toFixed(0)}d` | CAUTION `#e2c97e` |
| `P95 SAFE BET` | `{safe.toFixed(0)}d` | ALARM `#ff6b6b` |
| `FAT-TAIL RATIO` | `{fat_tail_ratio.toFixed(2)}` | ALARM `#ff6b6b` |
| `ITEMS ANALYZED` | `{issues_analyzed}` | MUTED `#505878` |

---

## Badge Row (extends base skill badge system)

```js
const predictColor =
  data.predictability.includes("Unstable") ? "#ff6b6b" :
  data.predictability.includes("Marginal") ? "#e2c97e" : "#6bffb8";

const trendColor =
  data.throughput_trend.direction === "Increasing" ? "#6bffb8" : "#ff6b6b";
```

Always show:
1. `⚡ {data.predictability}` — color: `predictColor`
2. `Fat-Tail: {fat_tail_ratio}x (threshold ≥ 5.6)` — ALARM if ratio ≥ 5.6, else MUTED
3. `Tail/Median: {tail_to_median_ratio}x (threshold > 3)` — ALARM if ratio > 3, else MUTED
4. `Throughput Trend: ↑{percentage_change}% recently` — color: `trendColor`
   (omit entirely if `direction === "Stable"` and `percentage_change < 10`)

**Interactive controls** (right-aligned in badge row, per base skill interactive controls
pattern): `SPECTRUM VIEW` and `GROUPED VIEW` toggle buttons.

---

## Type Toggle Buttons

Below the badge row, one button per eligible type. Follows the base skill's interactive
controls button style, using `TYPE_COLORS[type]` as the active color.

Show item count as small sub-label: `n={data.context.type_counts[type]}`.

Guard: at least 1 type must always remain selected — disable deselection of the last active type.

---

## Tooltips (extends base skill tooltip base style)

**Spectrum View:**
```
{type}  · n={n}
──────────────────────
P10:      {p10.toFixed(0)}d
P50:      {p50.toFixed(0)}d          ← SECONDARY color
P85 SLE:  {p85.toFixed(0)}d          ← CAUTION color, bold
P95:      {p95.toFixed(0)}d          ← ALARM color
```

**Grouped View:**
```
{percentile label}   (e.g. "P85 SLE")
──────────────────────────────────────
{TypeName}: {value}d    ← one row per active type, color = TYPE_COLORS[type]
```

---

## Legend (extends base skill legend pattern)

**Spectrum view** — centered below chart panel, filled rect swatches (base skill fill pattern):
```
■ POSITIVE   0 → P10   (fast outliers)
■ SECONDARY  P10 → P50 (typical fast)
■ CAUTION    P50 → P85 SLE (normal range)
■ ALARM      P85 → P95 (tail risk)
```

**Grouped view** — centered below chart panel, filled rect swatches:
```
■ TYPE_COLOR[type]   {TypeName}   — one entry per active type
```

---

## Footer Content (follows base skill footer style)

Two sections:

1. **"Reading this chart:"** — Each bar segment represents the probability range for
   cycle time from commitment point to delivery. P50 is the median — half of items
   finish faster. P85 is the Service Level Expectation (SLE): the threshold to commit
   to stakeholders with confidence. The stacked spectrum view reveals how predictable
   each issue type is: a narrow bar means tight distribution, a wide bar signals high
   variance. The fat-tail ratio (P98 ÷ P50, threshold ≥ 5.6) and tail-to-median ratio
   (P85 ÷ P50, threshold > 3) quantify how much extreme outliers dominate the process.

2. **"Data scope:"** — `{issues_analyzed}` delivered items over `{days_in_sample}` days.
   `{dropped_by_outcome}` items excluded (abandoned resolutions). Commitment point:
   "awaiting development". Types with fewer than 15 items are excluded from per-type
   modeling. Overall percentiles use stratified modeling across eligible types.

---

## Chart-Specific Checklist

> The universal checklist is in `mcs-charts-base`. Only chart-specific items are listed here.

- [ ] Both `mcs-charts-base` and this skill read before building
- [ ] Skill triggered by `analyze_cycle_time` data (not `analyze_process_stability`)
- [ ] Chart title reads exactly **"Cycle Time — SLE Distribution"**
- [ ] `data.percentiles` used for Overall (there is no `data.type_sles.Overall` key)
- [ ] Only eligible types rendered — `stratification_decisions[].eligible === true`
- [ ] Spectrum: 4 stacked segments with POSITIVE / SECONDARY / CAUTION / ALARM colors in order
- [ ] P95 LabelList shown at the right end of each spectrum bar
- [ ] Grouped view: one `<Bar>` per eligible type, one group per percentile key
- [ ] SLE reference table always visible below both views; P85 row highlighted amber
- [ ] Stat cards use Overall `data.percentiles` values (P50, P85, P95, fat-tail, items)
- [ ] View toggle buttons (SPECTRUM / GROUPED) right-aligned in badge row
- [ ] Type toggle buttons with `n=` sub-labels; minimum 1 type always remains active
- [ ] Predictability badge colored ALARM / CAUTION / POSITIVE by threshold logic
- [ ] Fat-tail badge ALARM when ≥ 5.6, tail-to-median badge ALARM when > 3
- [ ] Throughput trend badge omitted when Stable and change < 10%
- [ ] Spectrum tooltip: P10 / P50 (SECONDARY) / P85 (CAUTION, bold) / P95 (ALARM)
- [ ] Grouped tooltip: per-type values colored by TYPE_COLORS
