---
name: process-stability-chart
description: >
  Creates a dark-themed React/Recharts chart for the **Process Stability** analysis
  (mcs-mcp:analyze_process_stability). Trigger on: "process stability chart",
  "cycle time XmR", "lead time run chart", "cycle time stability", "stability index chart",
  "cycle time scatterplot", or any "show/chart/plot/visualize that" follow-up after an
  analyze_process_stability result is present.
  ONLY for analyze_process_stability output (individual item cycle times with XmR limits
  and Stability Index). Do NOT use for: throughput stability (analyze_throughput),
  WIP count stability (analyze_wip_stability), Total WIP Age (analyze_wip_age_stability),
  or process evolution (analyze_process_evolution). Those are different analyses requiring
  different charts. Always read this skill before building the chart — do not attempt it ad-hoc.
---

# Process Stability Chart

> **Scope:** Use this skill ONLY when `analyze_process_stability` data is present in the
> conversation. It visualizes individual item cycle times as a **Cycle Time Scatterplot**
> with XmR reference lines (UNPL, Mean), signal dot annotations, a Stability Index summary,
> and per-issue-type breakdowns.

---

## Prerequisites

Call the tool if data is not yet in the conversation:

```js
mcs-mcp:analyze_process_stability({ board_id, project_key })
// Optional: history_window_weeks (default 26)
```

---

## Response Structure

```
data.stability
  .xmr
    .average                      — process mean (X̄) in days
    .average_moving_range         — MR̄
    .upper_natural_process_limit  — UNPL = X̄ + 2.66 × MR̄
    .lower_natural_process_limit  — LNPL (usually 0 for cycle times)
    .values[]                     — individual item cycle times (only if includeRawSeries)
    .moving_ranges[]              — absolute day-to-day differences (only if includeRawSeries)
    .signals[]                    — null or array:
        .index       — position in scatterplot[] array (0-based)
        .key         — Jira issue key e.g. "IESFSCPL-1452" (NOT a date)
        .type        — "outlier" | "shift"
        .description — human-readable description

  .stability_index    — (WIP / Throughput) / Mean Cycle Time
                        <0.9 = stable, 0.9–1.3 = marginal, >1.3 = clogged
  .expected_lead_time — estimated median lead time in days
  .signals[]          — duplicate of xmr.signals (same data, use either)

data.stratified       — per-issue-type breakdown:
  .Story / .Bug / .Activity / .Defect
    .xmr { average, average_moving_range,
            upper_natural_process_limit, lower_natural_process_limit,
            signals[] }
    .stability_index
    .expected_lead_time
    .signals[]
  NOTE: Stratified results never include values[] or moving_ranges[].
        Use data.scatterplot filtered by issue_type instead.

data.scatterplot[]    — chart-ready array, one entry per delivered work item:
    .date             — completion date (outcome date), "YYYY-MM-DD"
    .value            — cycle time in days (rounded to 2 decimal places)
    .moving_range     — |cycleTimes[i] - cycleTimes[i-1]|, pooled (null for first item)
    .key              — Jira issue key e.g. "MOCK-100"
    .issue_type       — e.g. "Story", "Bug", "Activity"
```

**Critical difference from other charts:** `scatterplot[]` entries are **individual item
cycle times** plotted by completion date. The X-axis is calendar date (multiple items may
share a date — this is a scatterplot, not a line chart). Signal keys are **Jira issue
keys**, not dates.

---

## Data Preparation

```js
// Extract overall XmR constants
const MEAN  = data.stability.xmr.average;
const UNPL  = data.stability.xmr.upper_natural_process_limit;
const LNPL  = data.stability.xmr.lower_natural_process_limit; // usually 0
const SI    = data.stability.stability_index;
const ELT   = data.stability.expected_lead_time; // estimated P50 in days

// Build signal lookup sets (indexed by position in scatterplot[])
const signalOutliers = new Set(
  (data.stability.xmr.signals || [])
    .filter(s => s.type === "outlier")
    .map(s => s.index)
);
const signalShifts = new Set(
  (data.stability.xmr.signals || [])
    .filter(s => s.type === "shift")
    .map(s => s.index)
);
// Map index → issue key for tooltip display
const signalKeys = Object.fromEntries(
  (data.stability.xmr.signals || []).map(s => [s.index, s.key])
);

// Annotate data array from scatterplot
const chartData = data.scatterplot.map((pt, i) => ({
  date:       pt.date,             // completion date (X-axis)
  value:      pt.value,            // cycle time in days (Y-axis)
  mr:         pt.moving_range,     // pooled moving range (tooltip)
  key:        pt.key,              // Jira issue key (tooltip)
  issueType:  pt.issue_type,       // for type filtering
  mean:       MEAN,
  unpl:       UNPL,
  isOutlier:  signalOutliers.has(i),
  isShift:    signalShifts.has(i),
  issueKey:   signalKeys[i] || pt.key,
}));
```

For type-filtered views (e.g., "Story"), filter `chartData` by `issueType` and use the
matching stratified XmR limits:

```js
const typeData = chartData.filter(d => d.issueType === selectedType);
const typeMean = data.stratified[selectedType].xmr.average;
const typeUNPL = data.stratified[selectedType].xmr.upper_natural_process_limit;
// Re-map mean/unpl on each filtered point
```

Values near 0.00 are valid — items committed and resolved on the same day.
Do not filter them out.

---

## Chart Architecture

**Single panel** `ComposedChart` with one Y-axis. No dual axis, no stacked bars.

### X-axis
- `dataKey="date"` (completion date)
- Tick formatter: format as short date (e.g., `"Jun 12"`, `"Mar 3"`)
- Interval: `Math.floor(data.length / 8)` to show ~8–10 labels
- Multiple items may share a date — this is a scatterplot, not a line chart

### Y-axis
- Domain: `[0, Math.ceil((Math.max(...values, UNPL) * 1.1) / 50) * 50]`
- Tick formatter: `v => \`${v}d\`` (days)
- Label: `"Cycle Time (days)"`

### Series

1. **`<Scatter>`** (preferred) or **`<Line>`** with `type="monotone"` — `dataKey="value"`.
   Use `<Scatter>` for a true scatterplot (no connecting line). If using `<Line>`, set
   `strokeOpacity={0.3}`, `strokeWidth={1}`, `connectNulls={false}`.
   Custom dots for signals only.

2. **`<ReferenceLine>`** for UNPL — `#ff6b6b`, dashed `"6 3"`, label "UNPL".
3. **`<ReferenceLine>`** for MEAN — type color, dashed `"4 4"`, label "X̄".
4. **Only render LNPL if `LNPL > 0`** — for cycle times LNPL is almost always 0.

### CustomDot

```jsx
const CustomDot = ({ cx, cy, payload }) => {
  if (payload.isOutlier) return <circle cx={cx} cy={cy} r={5} fill="#ff6b6b" stroke="#080a0f" strokeWidth={1.5} />;
  if (payload.isShift)   return <circle cx={cx} cy={cy} r={4} fill="#e2c97e" stroke="#080a0f" strokeWidth={1.5} />;
  return <circle cx={cx} cy={cy} r={2.5} fill="#dde1ef" fillOpacity={0.6} stroke="none" />;
};
```

Red dot = outlier above UNPL. Amber dot = shift anchor (8-point run). Small dot = normal item.

---

## Issue Type Toggle

Add toggle buttons for `Overall`, `Story`, `Activity`, `Bug` (and `Defect` if present).

- Show SI value as small sub-label on each per-type button
- Switching type re-renders the chart with the selected type's filtered scatterplot data,
  `mean`, `unpl`, `signals`, `stability_index`, and `expected_lead_time`
- Defect often has only 1 item — still renderable but note "insufficient data" in the panel header

Type colors:
```js
{ Story:"#6b7de8", Activity:"#7edde2", Bug:"#ff6b6b", Overall:"#dde1ef" }
```

---

## Stability Index (SI) Color Logic

```js
const siColor = (si) =>
  si > 1.3 ? "#ff6b6b"  // clogged
: si > 0.9 ? "#e2c97e"  // marginal
:            "#6bffb8"; // stable
```

Status label:
- `> 1.3` → `"⚠ CLOGGED SYSTEM"`
- `0.9–1.3` → `"~ MARGINAL"`
- `< 0.9` → `"✓ STABLE"`

---

## Header

- **Breadcrumb:** `{PROJECT_KEY} · {board name} · Board {board_id}` — muted, uppercase
- **Title:** exactly `"Process Stability"`
- **Subtitle:** `"Cycle Time Scatterplot · Individual items by completion date"`
- **Stat cards:**

| Label | Value | Color |
|---|---|---|
| `X̄ Mean` | `{mean.toFixed(1)}d` | type color |
| `UNPL` | `{unpl.toFixed(1)}d` | ALARM `#ff6b6b` |
| `Expected P50` | `{expectedLeadTime}d` | SECONDARY `#7edde2` |
| `Stab. Index` | `{stabilityIndex.toFixed(2)}` | SI color |

---

## Signal Badges

Below the header, always show:
1. Status verdict (SI-based: CLOGGED / MARGINAL / STABLE)
2. Outlier count badge (red) — e.g. `"⚠ 7 outliers above UNPL"`
3. Shift count badge (amber) — e.g. `"⇶ 6 process shifts detected"`

Omit outlier/shift badges if count is 0.

---

## Per-Type Summary Cards (shown when Overall is selected)

When the "Overall" view is active, render a 3-column summary grid below the main chart,
one card per issue type (Story, Activity, Bug — skip Defect if only 1 item).

Each card shows:
- Type name + SI badge (colored by SI)
- X̄, UNPL, P50 estimate, outlier count, shift count
- A mini scatterplot (height ~140px) using the type's filtered data

---

## Tooltip

```
{issueKey}                       ← bold, Jira key
{date}                           ← completion date, formatted
─────────────────────
Cycle Time: {value.toFixed(1)} days
mR:         {mr?.toFixed(1) || "–"} d
X̄:    {mean.toFixed(1)} d
UNPL: {unpl.toFixed(1)} d
[if isOutlier] ⚠ Outlier: above UNPL     ← red, bold
[if isShift]   ⇶ Process Shift detected  ← amber, bold
```

---

## Legend

Manual legend (never use Recharts `<Legend>` component). Center below chart.

```
● (type color, semi-transparent)   Cycle Time (individual items)
-- (red dashed)                     UNPL
-- (type color dashed)              X̄ Mean
● (red)                             Outlier (above UNPL)
● (amber)                           Process Shift anchor
```

---

## Footer

Two sections:

1. **"Reading this chart:"** — Each point is the cycle time of one delivered item,
   plotted by its completion date (outcome date). Multiple items may share a date.
   The dashed X̄ is the process mean. The UNPL defines the outer boundary of natural
   variation — points above it are outliers caused by special circumstances, not normal
   process noise. Yellow dots mark the anchor of an 8-point run on one side of the mean
   (Process Shift signal). The Stability Index (WIP ÷ Throughput ÷ Mean Cycle Time)
   summarises system health: below 0.9 = stable, 0.9–1.3 = marginal, above 1.3 = clogged.

2. **"Data provenance:"** — Wheeler XmR applied to individual item cycle times. Signals:
   outlier = above UNPL; shift = 8+ consecutive points on one side of X̄. Values near 0
   indicate items committed and resolved the same day.

---

## Chart-Specific Checklist

- [ ] Skill triggered by `analyze_process_stability` data
- [ ] Chart title reads exactly **"Process Stability"**
- [ ] X-axis shows completion dates (not sequence numbers)
- [ ] Signal keys are Jira issue keys (e.g. "IESFSCPL-1452"), NOT dates — used only in tooltip
- [ ] LNPL reference line only rendered if `LNPL > 0`
- [ ] CustomDot: red r=5 for outliers, amber r=4 for shift anchors, small dot otherwise
- [ ] Type toggle buttons: Overall / Story / Activity / Bug
- [ ] Type-filtered views use stratified XmR limits, not pooled
- [ ] SI shown on toggle buttons as small sub-label
- [ ] Status badge: CLOGGED (red) / MARGINAL (amber) / STABLE (green)
- [ ] Outlier and shift count badges shown when count > 0
- [ ] Per-type summary grid visible when Overall is selected
- [ ] Tooltip shows issue key + completion date + cycle time + mR + signal type
- [ ] Stat cards: X̄, UNPL, Expected P50, Stability Index
- [ ] SI card color driven by SI thresholds (0.9 / 1.3)
- [ ] Manual legend below chart panel
- [ ] Dark theme throughout
- [ ] Monospace font throughout
- [ ] Single self-contained `.jsx` file with default export
