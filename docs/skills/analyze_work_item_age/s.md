---
name: work-item-age-chart
description: >
  Creates a dark-themed React/Recharts chart for the WIP Item Age analysis
  (mcs-mcp:analyze_work_item_age). Trigger on: "WIP age chart", "aging items",
  "which items are old", "aging outliers", "how old is my WIP", "stale items",
  "item age ranking", or any "show/chart/plot/visualize that" follow-up after an
  analyze_work_item_age result is present. ONLY for analyze_work_item_age output
  (per-item WIP age, percentile ranking, and outlier flags). Do NOT use for: Total
  WIP Age stability over time (analyze_wip_age_stability), WIP count stability
  (analyze_wip_stability), cycle time SLE (analyze_cycle_time), or status persistence
  (analyze_status_persistence). Always read this skill before building the chart —
  do not attempt it ad-hoc.
---

# WIP Item Age Chart

Scope: Use this skill ONLY when analyze_work_item_age data is present in the
conversation. It visualizes each active WIP item's age since the commitment point,
flags outliers against the P85 cycle time threshold, and shows the distribution of
aging risk across workflow statuses and issue types.

Do not use this skill for:
- Total WIP Age stability over time → use analyze_wip_age_stability
- WIP Count stability → use analyze_wip_stability
- Cycle time SLE → use analyze_cycle_time
- Status persistence (completed items only) → use analyze_status_persistence

---

## Prerequisites

```js
mcs-mcp:analyze_work_item_age({
  board_id, project_key,
  age_type:    "wip",    // "wip" (since commitment) or "total" (since creation)
  tier_filter: "WIP",    // default — excludes Demand and Finished
})
```

Workflow mapping (commitment point) must be confirmed before calling — the commitment
point defines when `age_since_commitment_days` starts.

API options note:
- `age_type`: `"wip"` uses `age_since_commitment_days` as the primary metric (default and preferred).
  `"total"` uses `total_age_since_creation_days`. The outlier flag is always based on wip age.
- `tier_filter`: `"WIP"` (default), `"All"`, `"Demand"`, `"Upstream"`, `"Downstream"`, `"Finished"`.
  Use `"WIP"` unless the user explicitly asks to include Demand/Finished items.

---

## Response Structure

```
data.aging[]                         — one entry per active WIP item:
  .key                               — Jira issue key (e.g. "PROJ-1234")
  .type                              — issue type (Story, Activity, Bug, Defect...)
  .status                            — current workflow status
  .tier                              — workflow tier (Downstream, Upstream...)
  .age_since_commitment_days         — PRIMARY age metric (WIP mode)
  .total_age_since_creation_days     — age since Jira creation
  .age_in_current_status_days        — time spent in current status only
  .cumulative_wip_days               — cumulative active WIP time
  .cumulative_upstream_days          — time spent in Upstream tier
  .cumulative_downstream_days        — time spent in Downstream tier
  .percentile                        — relative to historical cycle time distribution
  .is_aging_outlier                  — true if age exceeds historical P85

data.summary                         — aggregate WIP health snapshot:
  .total_items                       — total active WIP count
  .outlier_count                     — items exceeding P85 cycle time
  .p50_threshold_days                — historical cycle time P50
  .p85_threshold_days                — historical cycle time P85 (outlier threshold)
  .p95_threshold_days                — historical cycle time P95 (extreme outlier)
  .distribution                      — bucketed item counts:
    ."Inconspicuous (within P50)"    — items aging < P50
    ."Aging (P50-P85)"               — items between P50 and P85
    ."Warning (P85-P95)"             — items between P85 and P95
    ."Extreme (>P95)"                — items exceeding P95
  .stability_index                   — Little's Law index (WIP / throughput × avgCT)
```

Primary age metric: Always use `age_since_commitment_days` as the main displayed
value in WIP age mode. `total_age_since_creation_days` is informative context only.

`is_aging_outlier` flag: Set by the tool when `age_since_commitment_days` exceeds
the historical P85 cycle time. Do not recalculate — use the flag directly.

`stability_index`: Little's Law ratio. Values >1.3 indicate a clogged system;
<0.7 indicate a starving system.

---

## Data Preparation

Flatten the tool response into a compact local array:

```js
// { key, type, status, wip, statusAge, pct, outlier }
//   wip       <- age_since_commitment_days
//   statusAge <- age_in_current_status_days
//   pct       <- percentile
//   outlier   <- is_aging_outlier

const AGING = [
  // { key, type, status, wip, statusAge, pct, outlier }, ...
];

const sorted     = [...AGING].sort((a, b) => b.wip - a.wip);
const oldest     = sorted[0];
const medianItem = sorted[Math.floor(sorted.length / 2)];
```

Derived sets for chart views:

```js
// By Status — use canonical STATUS_ORDER (from workflow mapping)
const byStatus = STATUS_ORDER.map(s => {
  const items = AGING.filter(d => d.status === s && (typeFilter === "All" || d.type === typeFilter));
  if (!items.length) return null;
  return {
    status:     abbrevStatus(s),   // abbreviated for X-axis labels
    fullStatus: s,                 // full name for tooltip
    count:      items.length,
    outliers:   items.filter(d => d.outlier).length,
    normal:     items.filter(d => !d.outlier).length,
    maxAge:     Math.max(...items.map(d => d.wip)),
  };
}).filter(Boolean);

// By Type — always all types, no filter
const byType = presentTypes.map(t => {
  const items = AGING.filter(d => d.type === t);
  return {
    type:     t,
    total:    items.length,
    outliers: items.filter(d => d.outlier).length,
    normal:   items.filter(d => !d.outlier).length,
    maxAge:   items.length ? Math.max(...items.map(d => d.wip)) : 0,
    avgAge:   items.length ? items.reduce((s, d) => s + d.wip, 0) / items.length : 0,
  };
});
```

Status abbreviation helper — shorten long status names for X-axis readability:

```js
function abbrevStatus(s) {
  return s
    .replace("awaiting", "await")
    .replace("development", "dev")
    .replace("deploying", "depl")
    .replace("deploy to", "->")
    .replace("UAT (+Fix)", "UAT+Fix")
    .replace("awaiting UAT", "await UAT");
}
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
const PAGE_BG   = "#080a0f";
const PANEL_BG  = "#0c0e16";
const BORDER    = "#1a1d2e";
const SEVERE    = "#ff9b6b";   // unique to this chart — 120-299d severity tier
```

Issue type color map:

```js
const TYPE_COLORS = {
  Story:    PRIMARY,    // #6b7de8
  Activity: SECONDARY,  // #7edde2
  Bug:      ALARM,      // #ff6b6b
  Defect:   CAUTION,    // #e2c97e
};
```

Outlier severity color scale — applied to bar fills, table text, stat cards:

```js
function outlierColor(age) {
  if (age >= 300) return ALARM;    // critical
  if (age >= 120) return SEVERE;   // "#ff9b6b" — severe
  if (age >= 85)  return CAUTION;  // outlier (at/near P85)
  return PRIMARY;                  // normal
}
// fillOpacity: outlier = 0.9, normal = 0.45
```

---

## Recharts Rendering Rules — CRITICAL

These rules reflect confirmed rendering behaviour. Violations cause silent failures.

### Rule 1 — Cell is only safe inside a single-series Bar

Cell works reliably when a Bar is the ONLY bar in the chart (no `stackId`).
NEVER combine Cell per-row coloring with `stackId` — Recharts drops bars silently.

```jsx
// CORRECT — single Bar with Cell (Age Ranking view)
<Bar dataKey="wip" radius={[0, 3, 3, 0]}>
  {filtered.map((d, i) => (
    <Cell key={`cell-${i}`}
      fill={outlierColor(d.wip)}
      fillOpacity={d.outlier ? 0.9 : 0.45} />
  ))}
</Bar>

// WRONG — Cell inside a stacked Bar — DO NOT DO THIS
<Bar dataKey="normal" stackId="s" fill={PRIMARY}>
  {items.map((d, i) => <Cell key={i} fill={TYPE_COLORS[d.type]} />)}
</Bar>
```

### Rule 2 — Stacked bars use uniform fill only

For By Status and By Type views, bars share the same `stackId`. Apply color via
the `fill` prop on Bar — not via Cell. Use `fillOpacity` to separate the two series.

```jsx
// CORRECT — stacked bars, uniform fill
<Bar dataKey="outliers" stackId="s" fill={ALARM}   fillOpacity={0.85} />
<Bar dataKey="normal"   stackId="s" fill={PRIMARY} fillOpacity={0.5} radius={[3,3,0,0]} />
```

### Rule 3 — Height scales with item count in Age Ranking

```js
height={Math.max(420, filtered.length * 14)}
```

### Rule 4 — Cell keys use template literals

```jsx
<Cell key={`cell-${i}`} ... />
```

---

## Three Chart Views

```js
// State
const [view, setView] = useState("ranking");
// Interactive toggle buttons: AGE RANKING (default) | BY STATUS | BY TYPE
```

### View A: Age Ranking (default)

Horizontal BarChart (`layout="vertical"`):

```
Margin: { top: 4, right: 80, bottom: 4, left: 10 }  — right:80 to avoid label clip
Height: Math.max(420, filtered.length * 14)
XAxis: type="number", tickFormatter={v => v + "d"}
YAxis: type="category", dataKey="key", width={138}, fontSize=9px
Single Bar dataKey="wip" with per-item Cell coloring (Rule 1)
radius={[0, 3, 3, 0]} — rounded right edge only
```

Three ReferenceLine thresholds, all with `position: "right"` labels:

```jsx
<ReferenceLine x={P95} stroke={ALARM}    strokeDasharray="4 2" strokeWidth={1}
  label={{ value: `P95 ${P95}d`, fill: ALARM,    fontSize: 10, position: "right" }} />
<ReferenceLine x={P85} stroke={CAUTION}  strokeDasharray="6 3" strokeWidth={1.5}
  label={{ value: `P85 ${P85}d`, fill: CAUTION,  fontSize: 10, position: "right" }} />
<ReferenceLine x={P50} stroke={POSITIVE} strokeDasharray="3 3" strokeWidth={1}
  label={{ value: `P50 ${P50}d`, fill: POSITIVE, fontSize: 10, position: "right" }} />
```

```
Panel subtitle: "WIP age per item, sorted descending — P50 / P85 / P95 thresholds marked"
Type filter visible
```

### View B: By Status

Stacked BarChart (default vertical layout), height=310px:

```
XAxis: abbreviated status labels, rotated -35°, height={70}
YAxis: item count
Two stacked bars (Rule 2), stackId="s":
  outliers  ALARM   fillOpacity={0.85}
  normal    PRIMARY fillOpacity={0.5}  radius={[3,3,0,0]}
fullStatus in data for tooltip; abbrevStatus for axis label
Only statuses with at least one item included
Type filter visible
```

### View C: By Type

Stacked BarChart (default vertical layout), height=280px:

```
XAxis: issue types
Two stacked bars (Rule 2), stackId="t":
  outliers  ALARM   fillOpacity={0.85}
  normal    PRIMARY fillOpacity={0.5}  radius={[3,3,0,0]}
No type filter (all types always shown)
Do NOT use Cell here
```

---

## Outlier Table (always visible, below all views)

```
Columns: ITEM | TYPE | STATUS | WIP AGE | IN STATUS | PCT

- Show only outlier: true items, filtered by active type toggle
- Sorted by wip descending (most urgent first)
- Color rules:
    Item key:  outlierColor(wip), bold
    Type:      TYPE_COLORS[type]
    Status:    MUTED, 10px
    WIP Age:   outlierColor(wip), bold
    In Status: SECONDARY
    Pct:       ALARM, format as "P{pct}"
- Alternating row tint: i % 2 === 0 ? transparent : PRIMARY + "05"
```

---

## Stat Cards (6 cards)

```
TOTAL WIP        {TOTAL}                         —                                MUTED
OUTLIERS (P85+)  {OUTLIER_COUNT}                 "{N}% of WIP"                    ALARM
OLDEST ITEM      {oldest.wip.toFixed(0)}d        {oldest.key} (sub-label)         ALARM
P85 THRESHOLD    {P85.toFixed(0)}d               "historical cycle time"          CAUTION
MEDIAN AGE       {medianItem.wip.toFixed(1)}d    —                                SECONDARY
STABILITY INDEX  {STABILITY.toFixed(2)}          "> 1.3 clogged / < 0.7 starving"
                                                 ALARM if >1.3 / CAUTION if <0.7 / POSITIVE otherwise
```

---

## Badge Row

```
Always show these four badges:
1. "⚠ {N} aging outliers — {N}% of WIP exceeds P85"  ALARM
2. "Oldest: {key} — {age}d in {status}"               ALARM
3. "P85: {P85}d"                                      CAUTION
4. "Little's Law: {STABILITY} — {verdict}"            ALARM / CAUTION / POSITIVE

View toggle buttons right-aligned in the same row.
```

---

## Type Filter Buttons

```
Shown for Age Ranking and By Status views; hidden for By Type view.
One button per type present in AGING, plus "All" (default).
Active border/text color = TYPE_COLORS[type].
```

---

## Legend

Use Swatch components — no Recharts Legend component:

```jsx
function Swatch({ color, label }) {
  return (
    <div style={{ display: "flex", alignItems: "center", gap: 5 }}>
      <div style={{ width: 14, height: 10, background: color,
        borderRadius: 2, opacity: 0.85 }} />
      <span style={{ fontSize: 11, color: MUTED }}>{label}</span>
    </div>
  );
}
```

Age Ranking legend (bar color swatches + line swatches for thresholds):

```
Swatch ALARM    ">= 300d — critical"
Swatch SEVERE   "120-299d — severe"
Swatch CAUTION  "85-119d — outlier"
Swatch PRIMARY  "< P85 — normal (dimmed)"
--- divider ---
SVG line ALARM    strokeDasharray="4 2"  "P95 {P95}d"
SVG line CAUTION  strokeDasharray="6 3"  "P85 {P85}d"
SVG line POSITIVE strokeDasharray="3 3"  "P50 {P50}d"
```

By Status and By Type legend:

```
Swatch ALARM    "Aging outliers (>= P85)"
Swatch PRIMARY  "Normal items (< P85)"
```

---

## Tooltips

Use custom Tooltip components — no default Recharts tooltip.
Cursor: `fill: PRIMARY + "0c"` (very subtle highlight).

Age Ranking tooltip:

```
{key}                              bold
{type} · {status}                  TYPE_COLORS[type], 11px
─────────────────────────────────
WIP age:    {wip}d                 outlierColor(wip), bold
In status:  {statusAge}d           SECONDARY
Percentile: P{pct}                 ALARM if outlier, MUTED otherwise
─────────────────────────────────
⚠ Aging outlier (P85+)            ALARM, bold — only if outlier === true
```

By Status tooltip:

```
{fullStatus}                       bold
─────────────────────────────────
Outliers: {N}                      ALARM
Normal:   {N}                      PRIMARY
Total:    {N}                      MUTED
Max age:  {N}d                     CAUTION
```

By Type tooltip:

```
{type}                             TYPE_COLORS[type], bold
─────────────────────────────────
Total WIP: {N}                     TEXT
Outliers:  {N}                     ALARM
Max age:   {N}d                    CAUTION
Avg age:   {N}d                    SECONDARY
```

---

## Header

```
Breadcrumb: {PROJECT_KEY} · {BOARD_NAME} · Board {BOARD_ID}
Title:      exactly "WIP Item Age"
Subtitle:   "Age since commitment · {TOTAL} active items · P85 outlier threshold: {P85}d"
```

---

## Footer

Reading this chart: WIP age is measured from the commitment point to today. Items
flagged as aging outliers exceed the historical P85 cycle time — they have been in
progress longer than 85% of all previously completed items. This does not necessarily
mean they are blocked; it means they are statistically unusual and warrant attention.
"Age in current status" shows how long an item has been sitting in its current
workflow step — a high ratio relative to total WIP age suggests stalling at that step.

Stability index sentence: State the index value, the verdict (Clogged / Starving /
Balanced), and the 1.3 / 0.7 thresholds.

---

## Injection Checklist

```
Placeholder      Source path
BOARD_ID         board_id parameter used in the call
PROJECT_KEY      project_key parameter used in the call
BOARD_NAME       board name from context / import_boards
P50              data.summary.p50_threshold_days
P85              data.summary.p85_threshold_days
P95              data.summary.p95_threshold_days
STABILITY        data.summary.stability_index
DIST             data.summary.distribution (object — four bucket keys)
AGING array      data.aging[] flattened to { key, type, status, wip, statusAge, pct, outlier }
                   wip       <- .age_since_commitment_days
                   statusAge <- .age_in_current_status_days
                   pct       <- .percentile
                   outlier   <- .is_aging_outlier
STATUS_ORDER     canonical order from confirmed workflow mapping (workflow_discover_mapping)
```

---

## Checklist Before Delivering

- [ ] Triggered by analyze_work_item_age data
- [ ] Chart title reads exactly "WIP Item Age"
- [ ] Data flattened into compact array (short field names: wip, statusAge, pct, outlier)
- [ ] Primary age metric is age_since_commitment_days stored as wip
- [ ] is_aging_outlier taken directly from tool — stored as outlier, not recalculated
- [ ] P50, P85, P95 taken from data.summary.*_threshold_days
- [ ] STABILITY taken from data.summary.stability_index
- [ ] Outlier severity color scale: >=300d ALARM / 120-299d SEVERE / 85-119d CAUTION / <P85 PRIMARY dimmed
- [ ] Age Ranking: layout="vertical" BarChart, single Bar with Cell, no stackId (Rule 1)
- [ ] Age Ranking height: Math.max(420, filtered.length * 14) (Rule 3)
- [ ] Age Ranking margin right: 80 to prevent label clipping
- [ ] Cell keys use template literal `cell-${i}` (Rule 4)
- [ ] Three ReferenceLine thresholds: P95 (ALARM), P85 (CAUTION), P50 (POSITIVE)
- [ ] All three ReferenceLine labels use position: "right"
- [ ] By Status: stacked bars, uniform fill per Bar, no Cell (Rule 2)
- [ ] By Type: stacked bars, uniform fill per Bar, no Cell (Rule 2)
- [ ] Type filter shown for Age Ranking and By Status; hidden for By Type
- [ ] Outlier table always visible; sorted descending by wip; filtered by type toggle
- [ ] Stat card OLDEST ITEM shows item key as sub-label
- [ ] Stat card STABILITY INDEX uses clogged/starving/balanced coloring
- [ ] Badge: Little's Law stability index with verdict
- [ ] Tooltip cursor subtle: fill PRIMARY + "0c"
- [ ] Legend rendered as Swatch components — no Recharts Legend
- [ ] Footer distinguishes WIP age (since last commitment) from total age (since creation)
- [ ] Dark theme throughout: PAGE_BG page, PANEL_BG panel, BORDER grid
- [ ] Monospace font throughout
- [ ] Single self-contained .jsx file with default export
