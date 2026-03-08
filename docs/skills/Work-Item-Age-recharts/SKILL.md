---
name: work-item-age-chart
description: >
  Creates a dark-themed React/Recharts chart for the **WIP Item Age** analysis
  (mcs-mcp:analyze_work_item_age). Trigger on: "WIP age chart", "aging items",
  "which items are old", "aging outliers", "how old is my WIP", "stale items",
  "item age ranking", or any "show/chart/plot/visualize that" follow-up after an
  analyze_work_item_age result is present.
  ONLY for analyze_work_item_age output (per-item WIP age, percentile ranking, and
  outlier flags).
  Do NOT use for: Total WIP Age stability over time (analyze_wip_age_stability),
  WIP count stability (analyze_wip_stability), cycle time SLE (analyze_cycle_time),
  or status persistence (analyze_status_persistence). Those are different analyses
  requiring different charts.
  Always read this skill AND mcs-charts-base before building the chart — do not attempt
  it ad-hoc.
---

# WIP Item Age Chart

> **Scope:** Use this skill ONLY when `analyze_work_item_age` data is present in the
> conversation. It visualizes each active WIP item's age since the commitment point,
> flags outliers against the P85 cycle time threshold, and shows the distribution of
> aging risk across workflow statuses and issue types.
>
> **This skill extends `mcs-charts-base`.** Read that skill first. Everything defined
> there (stack, color tokens, typography, page/panel wrappers, stat card markup, badge
> system, CartesianGrid, tooltip base style, legend pattern, interactive controls,
> footer style, universal checklist) applies here without repetition. This skill only
> specifies what is unique to the work item age chart.

---

## Prerequisites

```js
mcs-mcp:analyze_work_item_age({
  board_id, project_key,
  age_type: "wip",       // "wip" (since commitment) or "total" (since creation)
  tier_filter: "WIP",    // default — excludes Demand and Finished
})
```

Workflow mapping (commitment point) must be confirmed before calling — the commitment
point defines when `age_since_commitment_days` starts.

---

## Response Structure

```
data.aging[]                         — one entry per active WIP item:
  .key                               — Jira issue key (e.g. "PROJ-1234")
  .type                              — issue type (Story, Activity, Bug, Defect...)
  .status                            — current workflow status
  .tier                              — workflow tier (Downstream, Upstream...)
  .age_since_commitment_days         — ★ PRIMARY age metric (WIP mode)
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
  .p85_threshold_days                — historical cycle time P85 ★
  .p95_threshold_days                — historical cycle time P95
  .distribution                      — bucketed item counts:
    ."Inconspicuous (within P50)"    — items aging < P50
    ."Aging (P50-P85)"               — items between P50 and P85
    ."Warning (P85-P95)"             — items between P85 and P95
    ."Extreme (>P95)"                — items exceeding P95
  .stability_index                   — Little's Law index (WIP / throughput × avgCT)
```

> **Primary age metric:** Always use `age_since_commitment_days` as the main displayed
> value in WIP age mode. `total_age_since_creation_days` can be informative for items
> with long pre-commitment history but is NOT the outlier basis.
>
> **`is_aging_outlier` flag:** Set by the tool when `age_since_commitment_days` exceeds
> the historical P85 cycle time. The P85 threshold is available in
> `data.summary.p85_threshold_days`.
>
> **`percentile` field:** Relative to historical completed items. P85+ = outlier.
> P98 = extreme outlier. This is the tool's internal P85 flagging basis — do not
> recalculate it.
>
> **`stability_index`:** Little's Law ratio (WIP ÷ throughput × avg cycle time).
> Values > 1.3 indicate a clogged system; < 0.7 indicate a starving system.

---

## Data Preparation

Flatten the tool response fields into a compact local array at the top of the component.
Use short property names (e.g. `wip`, `statusAge`, `pct`, `outlier`) rather than the
full API field names. This makes JSX easier to read and avoids deep property access
inside render loops.

```js
// Recommended shape for each item in the hardcoded array:
// { key, type, status, wip, statusAge, pct, outlier }
//   wip       ← age_since_commitment_days
//   statusAge ← age_in_current_status_days
//   pct       ← percentile
//   outlier   ← is_aging_outlier

const sorted = [...AGING].sort((a, b) => b.wip - a.wip);
const oldest = sorted[0];
const median = sorted[Math.floor(sorted.length / 2)].wip;

// Outliers (for table)
const OUTLIERS = AGING.filter(d => d.outlier);

// By-status breakdown
const byStatus = STATUS_ORDER.map(s => {
  const items = AGING.filter(d => d.status === s);
  if (!items.length) return null;
  return {
    status:     s,            // abbreviated label for X-axis
    fullStatus: s,            // full name for tooltip
    count:      items.length,
    outliers:   items.filter(d => d.outlier).length,
    normal:     items.filter(d => !d.outlier).length,
    maxAge:     Math.max(...items.map(d => d.wip)),
  };
}).filter(Boolean);

// By-type breakdown
const byType = ALL_TYPES.map(t => {
  const items = AGING.filter(d => d.type === t);
  return {
    type:     t,
    total:    items.length,
    outliers: items.filter(d => d.outlier).length,
    normal:   items.filter(d => !d.outlier).length,
    maxAge:   Math.max(...items.map(d => d.wip)),
    avgAge:   items.reduce((s, d) => s + d.wip, 0) / items.length,
  };
});
```

---

## Issue Type Color Map

```js
const TYPE_COLORS = {
  Story:    "#6b7de8",   // PRIMARY
  Activity: "#7edde2",   // SECONDARY
  Bug:      "#ff6b6b",   // ALARM
  Defect:   "#e2c97e",   // CAUTION
};
```

---

## Outlier Severity Color Scale

Four levels applied to bar fills, table text, and stat cards:

```js
function outlierColor(age) {
  if (age >= 300) return ALARM;       // #ff6b6b — critical
  if (age >= 120) return "#ff9b6b";   // orange-red — severe
  if (age >= 85)  return CAUTION;     // #e2c97e — outlier (at/near P85)
  return PRIMARY;                     // #6b7de8 — normal
}
// fillOpacity: outlier = 0.9, normal = 0.45
```

---

## Recharts Rendering Rules  ⚠ CRITICAL

These rules reflect confirmed rendering behaviour. Violating them causes silent
chart failures (blank panels, missing bars, console errors).

### Rule 1 — `<Cell>` is only safe inside a single-series `<Bar>`

`<Cell>` works reliably when a `<Bar>` is the **only** bar in the chart (no `stackId`).
**Never** combine `<Cell>` per-row coloring with `stackId` — Recharts drops the bar
silently or renders nothing.

```jsx
// ✅ CORRECT — single Bar with Cell (Age Ranking view)
<Bar dataKey="wip" radius={[0, 3, 3, 0]}>
  {filtered.map((d, i) => (
    <Cell key={`cell-${i}`}
      fill={outlierColor(d.wip)}
      fillOpacity={d.outlier ? 0.9 : 0.45} />
  ))}
</Bar>

// ❌ WRONG — Cell inside a stacked Bar
<Bar dataKey="normal" stackId="s" fill={PRIMARY}>
  {items.map((d, i) => <Cell key={i} fill={TYPE_COLORS[d.type]} />)}  // DO NOT DO THIS
</Bar>
```

### Rule 2 — Stacked bars use uniform fill only

For By Status and By Type views, both bars share the same `stackId`. Apply color via
the `fill` prop on `<Bar>` — not via `<Cell>`. Use `fillOpacity` to visually separate
the two series.

```jsx
// ✅ CORRECT — stacked bars, uniform fill
<Bar dataKey="outliers" stackId="s" fill={ALARM}   fillOpacity={0.85} />
<Bar dataKey="normal"   stackId="s" fill={PRIMARY} fillOpacity={0.5} radius={[3,3,0,0]} />
```

### Rule 3 — Do not mix `layout="vertical"` with `stackId`

The Age Ranking view uses `layout="vertical"` (horizontal bars). The By Status and
By Type views use the default vertical layout (upright bars). Never apply `stackId`
inside a `layout="vertical"` chart.

### Rule 4 — `ResponsiveContainer` height must be a stable number

Always pass a numeric `height` directly. Never derive it inside JSX in a way that
could produce `NaN` or `0`. Minimum safe height for the Age Ranking view:

```js
const rankH = Math.max(420, filtered.length * 14);
// Pass as: <ResponsiveContainer width="100%" height={rankH}>
```

### Rule 5 — Unique `key` props on `<Cell>`

Always use `key={\`cell-${i}\`}` (template literal), not `key={i}` alone. Recharts
uses the key to track cell identity across re-renders; plain integer keys can collide.

### Rule 6 — Keep tooltip `cursor` subtle

```jsx
cursor={{ fill: PRIMARY + "0c" }}   // ~5% opacity — avoids obscuring bars
```

---

## Chart Architecture

Three views, toggled from the badge row. An outlier table sits below all views.
Type filter toggles (All / Story / Activity / Bug / …) apply to Age Ranking and
By Status views. By Type view always shows all types.

### View A: Age Ranking (default)

**Horizontal `BarChart`** (`layout="vertical"`).

- Height: `Math.max(420, filtered.length * 14)` — scales with item count
- X-axis: `type="number"`, `tickFormatter={v => v + "d"}`
- Y-axis: `type="category"`, `dataKey="key"`, `width={138}`, 9px font
- Single `<Bar dataKey="wip">` with per-item `<Cell>` coloring (see Rule 1)
- `radius={[0, 3, 3, 0]}` — rounded right edge only
- `<ReferenceLine x={P85}>` — CAUTION, `strokeDasharray="6 3"`, `strokeWidth={1.5}`,
  `label={{ position: "insideTopRight" }}`

**Panel subtitle:** `"WIP age per item, sorted descending — P85 outlier threshold marked"`

### View B: By Status

**Stacked `BarChart`** (default vertical layout), height 310px.

- X-axis: abbreviated status labels (e.g. "await dev", "depl QA"), rotated -35°,
  `height={70}` to accommodate rotation
- Y-axis: item count
- Two stacked bars (see Rule 2):
  1. `<Bar dataKey="outliers" stackId="s">` — ALARM, `fillOpacity={0.85}`
  2. `<Bar dataKey="normal"   stackId="s">` — PRIMARY, `fillOpacity={0.5}`,
     `radius={[3,3,0,0]}`
- Store full status name in `fullStatus` field; display abbreviation on axis,
  full name in tooltip
- Only statuses with at least one item are included

### View C: By Type

**Stacked `BarChart`** (default vertical layout), height 280px.

- X-axis: issue types
- Two stacked bars with uniform fill (see Rule 2):
  1. `<Bar dataKey="outliers" stackId="t">` — ALARM, `fillOpacity={0.85}`
  2. `<Bar dataKey="normal"   stackId="t">` — PRIMARY, `fillOpacity={0.5}`,
     `radius={[3,3,0,0]}`
- No type filter in this view (all types always shown)
- **Do not** use per-type colors via `<Cell>` here — use uniform fill only

### Outlier Table (always visible below all views)

Columns: ITEM | TYPE | STATUS | WIP AGE | IN STATUS | PCT

- Show only `outlier: true` items, filtered by active type toggle
- Sorted by `wip` descending (most urgent first)
- Color rules:
  - Item key: `outlierColor(wip)`, bold
  - Type: `TYPE_COLORS[type]`
  - Status: MUTED, 10px
  - WIP Age: `outlierColor(wip)`, bold
  - In Status: SECONDARY
  - Pct: ALARM, format as `"P{pct}"`

---

## Legend

Replace the Recharts `<Legend>` with a manual swatch row using a reusable `Swatch`
component. Inline SVG is not required — a styled `<div>` with `borderRadius: 2`
is simpler and equally correct:

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

**Age Ranking legend:**
```
Swatch ALARM     "≥ 300d — critical"
Swatch #ff9b6b   "120–299d — severe"
Swatch CAUTION   "85–119d — outlier"
Swatch PRIMARY   "< P85 — normal (dimmed)"
```

**By Status and By Type legend:**
```
Swatch ALARM    "Aging outliers (≥ P85)"
Swatch PRIMARY  "Normal items (< P85)"
```

---

## Header (extends base skill header structure)

- **Breadcrumb:** `{PROJECT_KEY} · {board name} · Board {board_id}`
- **Title:** exactly `"WIP Item Age"`
- **Subtitle:** `"Age since commitment · {TOTAL} active items · P85 outlier threshold: {P85}d"`

**Stat cards:**

| Label | Value | Sub | Color |
|---|---|---|---|
| `TOTAL WIP` | `{TOTAL}` | — | MUTED |
| `OUTLIERS (P85+)` | `{OUTLIER_COUNT}` | `"{N}% of WIP"` | ALARM |
| `OLDEST ITEM` | `{oldest.wip.toFixed(0)}d` | `{oldest.key}` | ALARM |
| `P85 THRESHOLD` | `{P85.toFixed(0)}d` | `"historical cycle time"` | CAUTION |
| `MEDIAN AGE` | `{median.toFixed(1)}d` | — | SECONDARY |
| `STABILITY INDEX` | `{STABILITY.toFixed(2)}` | `"> 1.3 clogged / < 0.7 starving"` | `STABILITY > 1.3 ? ALARM : STABILITY < 0.7 ? CAUTION : POSITIVE` |

---

## Badge Row (extends base skill badge system)

Always show:
1. `⚠ {N} aging outliers — {N}% of WIP exceeds P85` — ALARM
2. `Oldest: {key} — {age}d in {status}` — ALARM
3. `P85: {P85}d` — CAUTION
4. `Little's Law: {STABILITY} — {verdict}` — ALARM / CAUTION / POSITIVE

**Interactive controls** (right-aligned): three view toggle buttons:
- `AGE RANKING` (default active)
- `BY STATUS`
- `BY TYPE`

---

## Type Filter Buttons

Shown for Age Ranking and By Status views. One button per type present in the data,
plus "All" (default). Uses `TYPE_COLORS[type]` as the active border/text color.
Hidden for By Type view.

---

## Tooltips

**Age Ranking view:**
```
{key}
{type} · {status}           ← TYPE_COLORS[type], 11px
────────────────────────────
WIP age:    {wip}d          ← outlierColor(wip), bold
In status:  {statusAge}d    ← SECONDARY
Percentile: P{pct}          ← ALARM if outlier, MUTED otherwise
────────────────────────────
⚠ Aging outlier (P85+)     ← ALARM, bold — only if outlier
```

**By Status view:**
```
{fullStatus}
────────────────────────────
Outliers: {N}               ← ALARM
Normal:   {N}               ← PRIMARY
Total:    {N}               ← MUTED
Max age:  {N}d              ← CAUTION
```

**By Type view:**
```
{type}                      ← TYPE_COLORS[type]
────────────────────────────
Total WIP: {N}              ← TEXT
Outliers:  {N}              ← ALARM
Max age:   {N}d             ← CAUTION
Avg age:   {N}d             ← SECONDARY
```

---

## Footer Content (follows base skill footer style)

**"Reading this chart:"** — WIP age is measured from the commitment point to today.
Items flagged as aging outliers exceed the historical P85 cycle time — they have been
in progress longer than 85% of all previously completed items. This does not
necessarily mean they are blocked; it means they are statistically unusual and warrant
attention. "Age in current status" shows how long an item has been sitting in its
current workflow step — a high ratio relative to total WIP age suggests the item is
stalled at that particular step.

**Stability index sentence:** inline after the above — state the index value, the
verdict (Clogged / Starving / Balanced), and the 1.3 threshold.

---

## Chart-Specific Checklist

> The universal checklist is in `mcs-charts-base`. Only chart-specific items here.

- [ ] Both `mcs-charts-base` and this skill read before building
- [ ] Skill triggered by `analyze_work_item_age` data
- [ ] Chart title reads exactly **"WIP Item Age"**
- [ ] Data flattened into compact local array (short field names: wip, statusAge, pct, outlier)
- [ ] Primary age metric is `age_since_commitment_days` → stored as `wip`
- [ ] `is_aging_outlier` flag taken directly from tool → stored as `outlier`
- [ ] P85 threshold taken from `data.summary.p85_threshold_days`
- [ ] Outlier severity color scale applied: ≥300d ALARM / 120–299d #ff9b6b / 85–119d CAUTION / < P85 PRIMARY dimmed
- [ ] Age Ranking: `layout="vertical"` BarChart, single Bar with Cell per item, no stackId ← **Rule 1 + Rule 3**
- [ ] Age Ranking height: `Math.max(420, filtered.length * 14)` ← **Rule 4**
- [ ] Cell keys use template literal `\`cell-${i}\`` ← **Rule 5**
- [ ] By Status: stacked bars, uniform fill per Bar (no Cell), canonical workflow order ← **Rule 2**
- [ ] By Type: stacked bars, uniform fill per Bar (no Cell), all types shown ← **Rule 2**
- [ ] Type filter shown for Age Ranking and By Status; hidden for By Type
- [ ] Outlier table always visible; sorted descending by wip; filtered by type toggle
- [ ] Stat card "OLDEST ITEM" shows item key as sub-label
- [ ] Stat card "STABILITY INDEX" uses clogged/starving/balanced coloring
- [ ] Badge: Little's Law stability index with verdict
- [ ] Tooltip cursor subtle: `fill: PRIMARY + "0c"`
- [ ] Legend rendered as Swatch components — no Recharts `<Legend>`
- [ ] Footer distinguishes WIP age (since last commitment) from total age (since creation)
