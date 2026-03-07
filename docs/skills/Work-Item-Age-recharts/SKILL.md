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
```

> **Primary age metric:** Always use `age_since_commitment_days` as the main displayed
> value in WIP age mode. `total_age_since_creation_days` can be informative for items
> with long pre-commitment history but is NOT the outlier basis.
>
> **`is_aging_outlier` flag:** Set by the tool when `age_since_commitment_days` exceeds
> the historical P85 cycle time. The P85 threshold value is NOT returned by this tool —
> it must be cross-referenced from `analyze_cycle_time` results if available. If not
> available, acknowledge this in the footer.
>
> **`percentile` field:** Relative to historical completed items. P85+ = outlier.
> P98 = extreme outlier. This is the tool's internal P85 flagging basis — do not
> recalculate it.

---

## P85 Threshold

The tool flags outliers internally using P85 but does NOT return the threshold value.

**If `analyze_cycle_time` results are available in the session:**
```js
const P85_THRESHOLD = data_from_analyze_cycle_time.percentiles.p85; // days
// Show as reference line in the Age Ranking view
// Show in stat card: "P85 THRESHOLD — {N}d — from cycle time SLE"
```

**If not available:**
```js
// Omit the reference line
// Stat card: "P85 THRESHOLD — N/A — run analyze_cycle_time"
// Footer note: "P85 threshold not available — run analyze_cycle_time for the exact value"
```

---

## Outlier Severity Color Scale

Four levels, applied to bars, table text, and outlier cards:

```js
const outlierColor = (wipAge) =>
  wipAge >= 300 ? ALARM              // #ff6b6b — critical (extreme outlier)
: wipAge >= 120 ? "#ff9b6b"          // orange-red — severe
: wipAge >=  85 ? CAUTION            // #e2c97e — outlier (at/near P85)
:                 PRIMARY;           // #6b7de8 — normal (dimmed)

// fillOpacity: outlier = 0.9, normal = 0.5
```

This scale communicates urgency beyond the binary outlier flag.

---

## Data Preparation

```js
// Sort all items by WIP age descending for Age Ranking view
const sorted = [...data.aging].sort((a, b) =>
  b.age_since_commitment_days - a.age_since_commitment_days
);

// Outliers only (for table + stat cards)
const OUTLIERS = data.aging.filter(d => d.is_aging_outlier);
const TOTAL    = data.aging.length;

// Status breakdown (for By Status view)
// Ordered by canonical workflow position — use workflow_set_order if known
const STATUS_ORDER = [
  "awaiting development", "developing",
  "awaiting deploy to QA", "deploying to QA",
  "awaiting UAT", "UAT (+Fix)",
  "awaiting deploy to Prod", "deploying to Prod",
];

const byStatus = STATUS_ORDER.map(s => {
  const inStatus = data.aging.filter(d => d.status === s);
  return {
    status:     s,
    count:      inStatus.length,
    outliers:   inStatus.filter(d => d.is_aging_outlier).length,
    normal:     inStatus.filter(d => !d.is_aging_outlier).length,
    maxAge:     inStatus.length ? Math.max(...inStatus.map(d => d.age_since_commitment_days)) : 0,
  };
}).filter(d => d.count > 0);

// Type breakdown (for By Type view)
const byType = [...new Set(data.aging.map(d => d.type))].map(t => {
  const items = data.aging.filter(d => d.type === t);
  return {
    type:     t,
    total:    items.length,
    outliers: items.filter(d => d.is_aging_outlier).length,
    normal:   items.filter(d => !d.is_aging_outlier).length,
    maxAge:   Math.max(...items.map(d => d.age_since_commitment_days)),
    avgAge:   items.reduce((s, d) => s + d.age_since_commitment_days, 0) / items.length,
    color:    TYPE_COLORS[t] || TEXT,
  };
});
```

---

## Issue Type and Status Color Maps

```js
const TYPE_COLORS = {
  Story:    "#6b7de8",   // PRIMARY
  Activity: "#7edde2",   // SECONDARY
  Bug:      "#ff6b6b",   // ALARM
  Defect:   "#e2c97e",   // CAUTION
};
```

---

## Chart Architecture

Three views, toggled from the badge row. An outlier table sits below all views.
Type filter toggles (All / Story / Activity / Bug) apply to Age Ranking and By Status
views. By Type view always shows all types.

### View A: Age Ranking (default)

**Horizontal `BarChart`** (`layout="vertical"`), height scales with item count
(`Math.max(340, items.length * 13)`).

- X-axis: `type="number"`, `tickFormatter={v => v + "d"}`
- Y-axis: `type="category"`, `dataKey="key"` (Jira issue keys), `width={120}`, 9px font
- `<Bar dataKey="age_since_commitment_days">` — one bar per item, sorted descending
- Bar fill: `outlierColor(wipAge)` via `<Cell>`, `fillOpacity` 0.9 (outlier) / 0.5 (normal)
- `barSize={9}`, `radius={[0, 3, 3, 0]}`
- `<ReferenceLine x={P85_THRESHOLD}>` — CAUTION, dashed `"6 3"`, `strokeWidth={1.5}`,
  labeled `"P85: {N}d"` at `position="top"`. Omit if threshold unavailable.

**Panel subtitle:** `"WIP age per item, sorted descending — P85 outlier threshold marked"`

### View B: By Status

**Stacked `BarChart`**, height 300px. X-axis: status names (truncated for display),
rotated -35°. Y-axis: item count.

Two stacked bars:
1. `<Bar dataKey="outliers" stackId="a">` — ALARM fill, `fillOpacity={0.85}`,
   `radius={[0,0,0,0]}`
2. `<Bar dataKey="normal"  stackId="a">` — PRIMARY fill, `fillOpacity={0.5}`,
   `radius={[3,3,0,0]}`

Status names on X-axis can be abbreviated (e.g. "await dev", "depl QA") to prevent
label crowding. Store the full name separately for tooltips.

Statuses ordered by canonical workflow position (use `STATUS_ORDER` from data
preparation). Only statuses with at least one item are rendered.

### View C: By Type

**Stacked `BarChart`**, height 280px. X-axis: issue types. Two stacked bars per type
(outliers ALARM / normal at `TYPE_COLORS[type]`).

Type toggle not needed in this view — always show all types side by side.

### Outlier Table (always visible)

Columns: Item | Type | Status | WIP Age | In Status | Pct

- Show only `is_aging_outlier: true` items, filtered by active type toggle
- Sorted by `age_since_commitment_days` descending (most urgent first)
- Color rules:
  - Item key: `outlierColor(wipAge)`, bold
  - Type: `TYPE_COLORS[type]`
  - Status: MUTED, 10px
  - WIP Age: `outlierColor(wipAge)`, bold
  - In Status: SECONDARY
  - Pct: ALARM, format as `"P{percentile}"`
- Row background: no tint (outlierColor on text is sufficient signal)

---

## Header (extends base skill header structure)

- **Breadcrumb:** `{PROJECT_KEY} · {board name} · Board {board_id}`
- **Title:** exactly `"WIP Item Age"`
- **Subtitle:** `"Age since commitment · {TOTAL} active items · P85 outlier threshold: {P85_THRESHOLD}d"`
  (or `"P85 threshold: N/A"` if not available)

**Stat cards:**

| Label | Value | Sub | Color |
|---|---|---|---|
| `TOTAL WIP` | `{TOTAL}` | — | MUTED |
| `OUTLIERS (P85+)` | `{OUTLIERS.length}` | `"{N}% of WIP"` | ALARM |
| `OLDEST ITEM` | `{max wip age}d` | `{item key}` | ALARM |
| `P85 THRESHOLD` | `{P85_THRESHOLD}d` or `"N/A"` | `"from cycle time SLE"` | CAUTION |
| `MEDIAN AGE` | `{median age}d` | — | SECONDARY |

Median = `sorted[Math.floor(sorted.length / 2)].age_since_commitment_days`.

---

## Badge Row (extends base skill badge system)

Always show:
1. `⚠ {N} aging outliers — {N}% of WIP exceeds P85` — ALARM
2. `Oldest: {key} — {age}d in {status}` — ALARM
3. `P85 threshold cross-referenced from analyze_cycle_time` — MUTED
   (or `⚠ P85 threshold unavailable — run analyze_cycle_time` — CAUTION if missing)

**Interactive controls** (right-aligned, per base skill pattern): three view toggle
buttons using PRIMARY as the active color:
- `AGE RANKING` (default)
- `BY STATUS`
- `BY TYPE`

---

## Type Filter Buttons

Shown for Age Ranking and By Status views. One button per type present in the data,
plus "All" (default). Uses `TYPE_COLORS[type]` as the active color.
Hidden for By Type view (always shows all types).

---

## Tooltips (extends base skill tooltip base style)

**Age Ranking view:**
```
{item key}
{type} · {status}   ← TYPE_COLORS[type], 11px
──────────────────────────────
WIP age:          {age}d     ← outlierColor(age), bold
In current status:{statusAge}d ← SECONDARY
Percentile:       P{pct}     ← ALARM if outlier, MUTED otherwise
──────────────────────────────
⚠ Aging outlier (P85+)       ← ALARM, bold — only if is_aging_outlier
```

**By Status view:**
```
{full status name}
──────────────────────────────
Outliers:  {N}               ← ALARM
Normal:    {N}               ← PRIMARY
Total:     {N}               ← MUTED
──────────────────────────────
Max age:   {N}d              ← CAUTION, 11px
```

**By Type view:**
```
{type}
──────────────────────────────
Total WIP: {N}               ← TEXT
Outliers:  {N}               ← ALARM
Max age:   {N}d              ← CAUTION
Avg age:   {N}d              ← SECONDARY
```

---

## Legend (extends base skill legend pattern)

**Age Ranking view:**
```
■ ALARM     #ff6b6b   ≥ 300d — critical
■           #ff9b6b   120–299d — severe
■ CAUTION   #e2c97e   85–119d — outlier
■ PRIMARY   #6b7de8   < P85 — normal   (dimmed)
```

**By Status and By Type views:**
```
■ ALARM    Aging outliers (≥ P85)
■ TYPE/PRIMARY  Normal items (< P85)
```

---

## Footer Content (follows base skill footer style)

Two sections:

1. **"Reading this chart:"** — WIP age is measured from the commitment point to today.
   Items flagged as aging outliers exceed the historical P85 cycle time — they have been
   in progress longer than 85% of all previously completed items. This does not
   necessarily mean they are blocked; it means they are statistically unusual and warrant
   attention. "Age in current status" shows how long an item has been sitting in its
   current workflow step specifically — a high status age relative to total WIP age
   suggests the item is stalled at that particular step.

2. **"Data scope:"** — `{TOTAL} active WIP items. Age type: WIP (resets on backflow
   to Demand/Upstream — reflects the LAST commitment date).`
   If P85 threshold available: `P85 threshold ({N}d) cross-referenced from analyze_cycle_time.`
   If not: `P85 threshold not available — run analyze_cycle_time to surface the exact value.`

---

## Chart-Specific Checklist

> The universal checklist is in `mcs-charts-base`. Only chart-specific items are listed here.

- [ ] Both `mcs-charts-base` and this skill read before building
- [ ] Skill triggered by `analyze_work_item_age` data
- [ ] Chart title reads exactly **"WIP Item Age"**
- [ ] Primary age metric is `age_since_commitment_days` — NOT `total_age_since_creation_days`
- [ ] `is_aging_outlier` flag taken directly from tool — never recalculated
- [ ] P85 threshold cross-referenced from `analyze_cycle_time` if available; acknowledged if not
- [ ] Outlier severity color scale applied: ≥300d ALARM / 120–299d orange-red / 85–119d CAUTION / normal PRIMARY dimmed
- [ ] Age Ranking: horizontal bar chart, sorted descending; P85 reference line if threshold known
- [ ] By Status: stacked bars in canonical workflow order; only statuses with items shown
- [ ] By Type: all types shown simultaneously; no type toggle needed
- [ ] Type filter buttons shown for Age Ranking and By Status; hidden for By Type
- [ ] Outlier table always visible; sorted descending by WIP age; filtered by type toggle
- [ ] Stat card "OLDEST ITEM" shows item key as sub-label
- [ ] Stat card "P85 THRESHOLD" shows "N/A" and CAUTION if threshold unavailable
- [ ] Badge: `⚠ P85 threshold unavailable` shown when analyze_cycle_time not run
- [ ] Tooltip: shows WIP age + status age + percentile + outlier flag if applicable
- [ ] Footer distinguishes WIP age (since last commitment) from total age (since creation)
