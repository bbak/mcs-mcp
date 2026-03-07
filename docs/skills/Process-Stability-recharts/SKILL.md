---
name: process-stability-chart
description: >
  Creates a dark-themed React/Recharts chart for the **Process Stability** analysis
  (mcs-mcp:analyze_process_stability). Trigger on: "process stability chart",
  "cycle time XmR", "lead time run chart", "cycle time stability", "stability index chart",
  or any "show/chart/plot/visualize that" follow-up after an analyze_process_stability
  result is present.
  ONLY for analyze_process_stability output (individual item cycle times with XmR limits
  and Stability Index). Do NOT use for: throughput stability (analyze_throughput),
  WIP count stability (analyze_wip_stability), Total WIP Age (analyze_wip_age_stability),
  or process evolution (analyze_process_evolution). Those are different analyses requiring
  different charts. Always read this skill before building the chart — do not attempt it ad-hoc.
---

# Process Stability Chart

> **Scope:** Use this skill ONLY when `analyze_process_stability` data is present in the
> conversation. It visualizes individual item cycle times as an XmR run chart with UNPL
> and mean reference lines, signal dot annotations, a Stability Index summary, and
> per-issue-type breakdowns.

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
    .values[]                     — individual item cycle times in days, ordered by
                                    completion date (NOT calendar time buckets)
    .moving_ranges[]              — absolute day-to-day differences (length = n-1)
    .signals[]                    — null or array:
        .index       — position in values[] array (0-based)
        .key         — Jira issue key e.g. "IESFSCPL-1452" (NOT a date)
        .type        — "outlier" | "shift"
        .description — human-readable description

  .stability_index    — (WIP / Throughput) / Mean Cycle Time
                        <0.9 = stable, 0.9–1.3 = marginal, >1.3 = clogged
  .expected_lead_time — estimated median lead time in days
  .signals[]          — duplicate of xmr.signals (same data, use either)

data.stratified       — per-issue-type breakdown, same structure as stability:
  .Story / .Bug / .Activity / .Defect
    .xmr { average, upper_natural_process_limit, lower_natural_process_limit,
            values[], signals[] }
    .stability_index
    .expected_lead_time
    .signals[]
```

**Critical difference from other charts:** `values[]` are **individual item cycle times**,
not time-bucketed aggregates. The X-axis is item sequence number, not calendar date.
Signal keys are **Jira issue keys**, not dates.

> **Known limitation — completion dates not exposed (MCP server improvement pending):**
> `analyze_process_stability` does not include per-item completion dates in its response.
> The server uses completion dates internally for ordering `values[]`, but does not surface
> them. As a result, the X-axis can only show item sequence numbers, not calendar dates.
> Do NOT attempt to work around this by calling `analyze_item_journey` per item — it would
> require N extra tool calls and only covers signal items, leaving the rest undated.
> When this is fixed server-side (each `values[]` entry enriched with `{ value, date, key }`),
> update the X-axis to use `dataKey="date"` with `tickFormatter={fmtDate}` and enable
> the tooltip to show the formatted completion date.

---

## Data Preparation

```js
// Extract overall XmR constants
const MEAN  = data.stability.xmr.average;
const UNPL  = data.stability.xmr.upper_natural_process_limit;
const LNPL  = data.stability.xmr.lower_natural_process_limit; // usually 0
const SI    = data.stability.stability_index;
const ELT   = data.stability.expected_lead_time; // estimated P50 in days

// Build signal lookup sets (indexed by position in values[])
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

// Annotate data array
const chartData = data.stability.xmr.values.map((v, i) => ({
  i,                        // sequence number (X-axis)
  value: v,
  mean:  MEAN,
  unpl:  UNPL,
  isOutlier: signalOutliers.has(i),
  isShift:   signalShifts.has(i),
  issueKey:  signalKeys[i] || null,
}));
```

Apply same preparation to each stratified type. Values near 0.00 are valid — items
committed and resolved on the same day. Do not filter them out.

---

## Chart Architecture

**Single panel** `ComposedChart` with one Y-axis. No dual axis, no stacked bars.

### X-axis
- `dataKey="i"` (sequence index)
- Tick formatter: `v => \`#${v+1}\`` — item number starting at 1
- Interval: `Math.floor(data.length / 8)` to show ~8–10 labels
- **No dates on the X-axis** — items are not evenly spaced in time

### Y-axis
- Domain: `[0, Math.ceil((Math.max(...values, UNPL) * 1.1) / 50) * 50]`
- Tick formatter: `v => \`${v}d\`` (days)
- Label: `"Cycle Time (days)"`

### Series

1. **`<Line>`** — `dataKey="value"`, thin line connecting items, `strokeOpacity={0.5}`,
   `strokeWidth={1}`. Purpose: show trend/shape. Custom dots for signals only.

2. **`<ReferenceLine>`** for UNPL — `#ff6b6b`, dashed `"6 3"`, label "UNPL".
3. **`<ReferenceLine>`** for MEAN — type color, dashed `"4 4"`, label "X̄".
4. **Only render LNPL if `LNPL > 0`** — for cycle times LNPL is almost always 0.

### CustomDot

```jsx
const CustomDot = ({ cx, cy, payload }) => {
  if (payload.isOutlier) return <circle cx={cx} cy={cy} r={5} fill="#ff6b6b" stroke="#080a0f" strokeWidth={1.5} />;
  if (payload.isShift)   return <circle cx={cx} cy={cy} r={4} fill="#e2c97e" stroke="#080a0f" strokeWidth={1.5} />;
  return null;
};
```

Red dot = outlier above UNPL. Amber dot = shift anchor (8-point run).

---

## Issue Type Toggle

Add toggle buttons for `Overall`, `Story`, `Activity`, `Bug` (and `Defect` if present).

- Show SI value as small sub-label on each per-type button
- Switching type re-renders the chart with the selected type's `values`, `mean`, `unpl`,
  `signals`, `stability_index`, and `expected_lead_time`
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
- **Subtitle:** `"Cycle Time XmR Run Chart · Individual items ordered by completion date"`
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
- A mini run chart (height ~140px) using `RunChart` with the type's data

---

## Tooltip

```
Item #{i+1}
{issueKey if present}          ← bold, Jira key
─────────────────────
Cycle Time: {value.toFixed(1)} days
X̄:    {mean.toFixed(1)} d
UNPL: {unpl.toFixed(1)} d
[if isOutlier] ⚠ Outlier: above UNPL     ← red, bold
[if isShift]   ⇶ Process Shift detected  ← amber, bold
```

---

## Legend

Manual legend (never use Recharts `<Legend>` component). Center below chart.

```
── (type color, semi-transparent)   Cycle Time (individual items)
-- (red dashed)                     UNPL
-- (type color dashed)              X̄ Mean
● (red)                             Outlier (above UNPL)
● (amber)                           Process Shift anchor
```

---

## Footer

Two sections:

1. **"Reading this chart:"** — Each point is the cycle time of one delivered item,
   ordered chronologically. The dashed X̄ is the process mean. The UNPL defines the
   outer boundary of natural variation — points above it are outliers caused by special
   circumstances, not normal process noise. Yellow dots mark the anchor of an 8-point
   run on one side of the mean (Process Shift signal). The Stability Index
   (WIP ÷ Throughput ÷ Mean Cycle Time) summarises system health: below 0.9 = stable,
   0.9–1.3 = marginal, above 1.3 = clogged.

2. **"Data provenance:"** — Wheeler XmR applied to individual item cycle times. Signals:
   outlier = above UNPL; shift = 8+ consecutive points on one side of X̄. Values near 0
   indicate items committed and resolved the same day.

---

## Chart-Specific Checklist

- [ ] Skill triggered by `analyze_process_stability` data
- [ ] Chart title reads exactly **"Process Stability"**
- [ ] X-axis shows item sequence numbers, NOT dates
- [ ] Signal keys are Jira issue keys (e.g. "IESFSCPL-1452"), NOT dates — used only in tooltip
- [ ] LNPL reference line only rendered if `LNPL > 0`
- [ ] CustomDot: red r=5 for outliers, amber r=4 for shift anchors, null otherwise
- [ ] Type toggle buttons: Overall / Story / Activity / Bug
- [ ] SI shown on toggle buttons as small sub-label
- [ ] Status badge: CLOGGED (red) / MARGINAL (amber) / STABLE (green)
- [ ] Outlier and shift count badges shown when count > 0
- [ ] Per-type summary grid visible when Overall is selected
- [ ] Tooltip shows issue key + cycle time + signal type
- [ ] Stat cards: X̄, UNPL, Expected P50, Stability Index
- [ ] SI card color driven by SI thresholds (0.9 / 1.3)
- [ ] Manual legend below chart panel
- [ ] Dark theme throughout
- [ ] Monospace font throughout
- [ ] Single self-contained `.jsx` file with default export
