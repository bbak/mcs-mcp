---
name: yield-chart
description: >
  Creates a dark-themed React/Recharts chart for the **Delivery Yield** analysis
  (mcs-mcp:analyze_yield). Trigger on: "yield chart", "delivery efficiency",
  "abandonment chart", "loss points", "where is work being abandoned", "yield rate",
  "delivery funnel", or any "show/chart/plot/visualize that" follow-up after an
  analyze_yield result is present.
  ONLY for analyze_yield output (yield rates, abandoned item counts, and loss points
  per tier, stratified by issue type).
  Do NOT use for: flow debt (analyze_flow_debt), throughput stability (analyze_throughput),
  cycle time SLE (analyze_cycle_time), status persistence (analyze_status_persistence),
  or WIP age (analyze_work_item_age). Those are different analyses requiring different charts.
  Always read this skill AND mcs-charts-base before building the chart — do not attempt
  it ad-hoc.
---

# Delivery Yield Chart

> **Scope:** Use this skill ONLY when `analyze_yield` data is present in the conversation.
> It visualizes delivery efficiency — what fraction of ingested work reaches a delivered
> resolution, where losses occur across workflow tiers, and how much investment is wasted
> per abandoned item (average age at abandonment).
>
> **This skill extends `mcs-charts-base`.** Read that skill first. Everything defined
> there (stack, color tokens, typography, page/panel wrappers, stat card markup, badge
> system, CartesianGrid, tooltip base style, legend pattern, interactive controls,
> footer style, universal checklist) applies here without repetition. This skill only
> specifies what is unique to the yield chart.

---

## Prerequisites

Call the tool if data is not yet in the conversation:

```js
mcs-mcp:analyze_yield({ board_id, project_key })
```

Workflow mapping (tiers) must be confirmed before calling — tier assignments directly
determine which losses are classified as Demand / Upstream / Downstream.

---

## Response Structure

```
data.yield                          — overall (all types pooled):
  .totalIngested                    — total items ever ingested
  .deliveredCount                   — items that reached delivered resolution
  .abandonedCount                   — items that reached abandoned resolution
  .overallYieldRate                 — deliveredCount / (deliveredCount + abandonedCount)
  .lossPoints[]                     — one entry per tier where abandonment occurred:
      .tier                         — "Demand" | "Upstream" | "Downstream"
      .count                        — abandoned item count at this tier
      .percentage                   — count / totalIngested
      .avgAge                       — average age in days at abandonment
      .severity                     — "Low" | "Medium" | "High"

data.stratified                     — same structure, keyed by issue type:
  .Story   { totalIngested, deliveredCount, abandonedCount, overallYieldRate, lossPoints[] }
  .Activity{ ... }
  .Bug     { ... }
  .Defect  { ... }
```

> **Severity semantics:** Downstream is always "High" (committed + partially executed
> work abandoned = maximum waste). Upstream is "Medium" (invested analysis work lost).
> Demand is "Low" (pre-commitment discard = cheapest form of abandonment).
>
> **Yield rate formula:** `deliveredCount / (deliveredCount + abandonedCount)`.
> Active WIP items are excluded — they haven't resolved yet.
>
> **totalIngested** includes delivered + abandoned + still-active items. Do NOT use it
> as the yield denominator — use `deliveredCount + abandonedCount`.

---

## Data Preparation

```js
// Merge Overall + stratified into one lookup
const ALL_DATA = { Overall: data.yield, ...data.stratified };

// Yield color thresholds
const yieldColor = (rate) =>
  rate >= 0.80 ? POSITIVE   // #6bffb8 — healthy
: rate >= 0.65 ? CAUTION    // #e2c97e — marginal
:                ALARM;     // #ff6b6b — critical

// Funnel comparison data — one entry per type
const funnelData = Object.entries(ALL_DATA).map(([type, d]) => ({
  type,
  rate:      d.overallYieldRate,
  delivered: d.deliveredCount,
  abandoned: d.abandonedCount,
  ingested:  d.totalIngested,
  yieldPct:  Math.round(d.overallYieldRate * 1000) / 10,
  color:     TYPE_COLORS[type],
}));

// Loss data for the selected type
const lossData = (type) =>
  ALL_DATA[type].lossPoints.map(lp => ({
    ...lp,
    type,
    lossPct: Math.round(lp.percentage * 1000) / 10,
  }));

// Age-at-abandon data: one entry per tier, values per type
const ageData = ["Demand", "Upstream", "Downstream"].map(tier => {
  const entry = { tier };
  ["Overall", "Story", "Activity", "Bug"].forEach(t => {
    const lp = ALL_DATA[t]?.lossPoints.find(l => l.tier === tier);
    entry[t] = lp ? lp.avgAge : 0;
  });
  return entry;
});
```

---

## Yield Color Thresholds

```js
// Applied to yield rate bars, dial bars, stat card border, and badge color
const yieldColor = (rate) =>
  rate >= 0.80 ? "#6bffb8"  // POSITIVE — healthy
: rate >= 0.65 ? "#e2c97e"  // CAUTION  — marginal
:                "#ff6b6b"; // ALARM    — critical
```

---

## Issue Type and Tier Color Maps

```js
const TYPE_COLORS = {
  Overall:  "#dde1ef",   // TEXT
  Story:    "#6b7de8",   // PRIMARY
  Activity: "#7edde2",   // SECONDARY
  Bug:      "#ff6b6b",   // ALARM
  Defect:   "#e2c97e",   // CAUTION
};

const TIER_COLORS = {
  Demand:     "#505878",  // MUTED     — low cost abandonment
  Upstream:   "#7edde2",  // SECONDARY — medium cost
  Downstream: "#ff6b6b",  // ALARM     — highest cost
};

const SEVERITY_COLORS = {
  High:   "#ff6b6b",   // ALARM
  Medium: "#e2c97e",   // CAUTION
  Low:    "#505878",   // MUTED
};
```

---

## Chart Architecture

Three independent views, toggled from the badge row. A permanent summary table sits
below all views.

### View A: Yield Comparison (default)

Two parts rendered together:

**1. Yield Dial Cards** — a horizontal flex row of cards, one per type in `ALL_DATA`.
Each card contains:
- Type label (MUTED, small)
- Horizontal progress bar: full-width track in BORDER color, filled portion in
  `yieldColor(rate)`, height 10px, `borderRadius 5px`
- Large yield percentage in `yieldColor(rate)`, bold
- Small line: `{delivered} delivered · {abandoned} abandoned · {ingested} total`

**2. Yield Comparison Bar Chart** — `BarChart`, height 280px.
X-axis: `dataKey="type"`. Y-axis: `domain={[0,1]}`,
`tickFormatter={v => (v*100).toFixed(0) + "%"}`.
One `<Bar dataKey="rate">`, each bar filled by `yieldColor(d.rate)` via `<Cell>`.
`radius={[4,4,0,0]}`, `fillOpacity={0.8}`.
`<LabelList dataKey="yieldPct" position="top" formatter={v => v + "%"}>`
in MUTED color.

### View B: Loss Breakdown

**Bar chart** — height 280px. X-axis: `dataKey="tier"` (3 bars: Demand, Upstream,
Downstream). Y-axis: raw count. One `<Bar dataKey="count">`, each bar filled by
`SEVERITY_COLORS[d.severity]` via `<Cell>`. `radius={[4,4,0,0]}`.
`<LabelList dataKey="count" position="top">` in MUTED.

**Loss detail cards** — horizontal flex row below the chart, one card per loss point
in the current type's `lossPoints`. Each card:
- Background tint: `${SEVERITY_COLORS[severity]}08`
- Border: `${SEVERITY_COLORS[severity]}33`
- Header: `{tier} — {severity}` in `SEVERITY_COLORS[severity]`, bold
- Large abandoned count in ALARM
- Small text: `{lossPct}% loss rate` and `Avg {avgAge}d at abandonment`

Driven by `activeType` state toggle (default: "Overall").

### View C: Age at Abandon

**Grouped `BarChart`** — height 280px. X-axis: `dataKey="tier"` (Demand, Upstream,
Downstream). One `<Bar>` per type (`Overall`, `Story`, `Activity`, `Bug`), using
`TYPE_COLORS`. `radius={[3,3,0,0]}`, `fillOpacity={0.75}`. Y-axis:
`tickFormatter={v => v + "d"}`. No stacking.

This view always shows all types simultaneously — no type toggle needed.
The type toggle buttons should be hidden when this view is active.

### Summary Table (always visible)

A rendered `<table>` with columns:
Type | Ingested | Delivered | Abandoned | Yield Rate | Downstream Loss | Avg Age @ DS Abandon

Color rules:
- Type name: `TYPE_COLORS[type]`, bold
- Delivered: POSITIVE
- Abandoned: ALARM
- Yield Rate: `yieldColor(rate)`, bold
- Downstream Loss count: ALARM
- Avg Age @ DS Abandon: CAUTION
- "—" for types with no Downstream loss point

---

## Header (extends base skill header structure)

- **Breadcrumb:** `{PROJECT_KEY} · {board name} · Board {board_id}`
- **Title:** exactly `"Delivery Yield"`
- **Subtitle:** `"Delivery efficiency across tiers · {data.yield.totalIngested} items ingested · full history"`

**Stat cards** (all from `data.yield` — Overall):

| Label | Value | Color |
|---|---|---|
| `OVERALL YIELD` | `{pct(overallYieldRate)}` | `yieldColor(overallYieldRate)` |
| `DELIVERED` | `{deliveredCount}` | POSITIVE `#6bffb8` |
| `ABANDONED` | `{abandonedCount}` | ALARM `#ff6b6b` |
| `DOWNSTREAM LOSS` | `{downstream lossPoint count} items` | ALARM `#ff6b6b` |
| `AVG AGE @ ABANDON` | `{downstream lossPoint avgAge}d` | CAUTION `#e2c97e` |

Downstream values sourced from `data.yield.lossPoints.find(l => l.tier === "Downstream")`.

---

## Badge Row (extends base skill badge system)

Always show:
1. `Overall Yield: {pct(overallYieldRate)}` — color: `yieldColor(overallYieldRate)`
2. `⚠ Downstream loss: {count} items ({pct(percentage)})` — ALARM
3. `Demand loss: {count} items` — MUTED
4. `Story yield: {pct(stratified.Story.overallYieldRate)}` — `yieldColor(Story rate)`

**Interactive controls** (right-aligned in badge row, per base skill interactive controls
pattern): three view toggle buttons using PRIMARY as the active color:
- `YIELD COMPARISON` (default)
- `LOSS BREAKDOWN`
- `AGE AT ABANDON`

---

## Type Toggle Buttons

Shown only for **Loss Breakdown** view (hidden for Yield Comparison and Age at Abandon).

One button per type: `Overall` (default), `Story`, `Activity`, `Bug`.
Skip Defect unless it has meaningful loss data (> 1 item with non-zero lossPoints).
Follows the base skill interactive controls button style, using `TYPE_COLORS[type]`
as the active color.

---

## Tooltips (extends base skill tooltip base style)

**Yield Comparison view:**
```
{type}
──────────────────────────
{yieldColor} {pct(rate)} yield   ← large, bold
──────────────────────────
Delivered:  {delivered}          ← POSITIVE
Abandoned:  {abandoned}          ← ALARM
Ingested:   {ingested}           ← MUTED
```

**Loss Breakdown view:**
```
{tier} tier — {type}
──────────────────────────
Severity: {severity}             ← SEVERITY_COLORS[severity]
──────────────────────────
Abandoned:         {count} items ← ALARM
Loss rate:         {pct}         ← CAUTION
Avg age @ abandon: {avgAge}d     ← MUTED
```

**Age at Abandon view:**
```
{tier} — avg age at abandon
──────────────────────────
Overall:  {val}d                 ← TEXT
Story:    {val}d                 ← PRIMARY
Activity: {val}d                 ← SECONDARY
Bug:      {val}d                 ← ALARM
```

---

## Legend (extends base skill legend pattern)

**Yield Comparison view** — centered below bar chart, filled rect swatches:
```
■ POSITIVE   ≥ 80% yield (healthy)
■ CAUTION    65–80% yield (marginal)
■ ALARM      < 65% yield (critical)
```

**Loss Breakdown view** — centered below bar chart, filled rect swatches:
```
■ ALARM    High severity
■ CAUTION  Medium severity
■ MUTED    Low severity
```

**Age at Abandon view** — centered below chart, filled rect swatches:
```
■ TYPE_COLORS[type]   {type}   — one per type shown
```

---

## Footer Content (follows base skill footer style)

Two sections:

1. **"Reading this chart:"** — Yield rate is the fraction of all resolved items that
   reached a delivered resolution. Loss points identify where in the workflow items
   were abandoned. Demand losses are the cheapest — work is discarded before commitment.
   Upstream losses indicate definition or prioritisation problems. Downstream losses are
   the most expensive: work was committed and partially executed before being abandoned,
   meaning invested capacity produced no value. Average age at abandonment quantifies
   how much of that investment was wasted — higher numbers indicate items were held for
   long periods before being discarded. Severity reflects the relative cost of each
   loss tier, not just its volume.

2. **"Data scope:"** — Full project history. Yield rate = delivered ÷
   (delivered + abandoned). Active WIP items are excluded from yield calculations —
   they have not yet resolved. `totalIngested` includes active items and should not
   be used as the yield denominator. Downstream severity is always "High" because
   abandoned Downstream items represent committed capacity that produced no delivered value.

---

## Chart-Specific Checklist

> The universal checklist is in `mcs-charts-base`. Only chart-specific items are listed here.

- [ ] Both `mcs-charts-base` and this skill read before building
- [ ] Skill triggered by `analyze_yield` data
- [ ] Chart title reads exactly **"Delivery Yield"**
- [ ] `data.yield` used for Overall (not a key inside `data.stratified`)
- [ ] Yield rate denominator is `deliveredCount + abandonedCount`, NOT `totalIngested`
- [ ] `yieldColor()` thresholds applied: ≥ 0.80 POSITIVE / ≥ 0.65 CAUTION / < 0.65 ALARM
- [ ] Three view toggle buttons right-aligned in badge row: YIELD COMPARISON / LOSS BREAKDOWN / AGE AT ABANDON
- [ ] Yield Comparison: dial cards + bar chart both shown; bars colored by `yieldColor(rate)` per bar via `<Cell>`
- [ ] Loss Breakdown: bar chart colored by `SEVERITY_COLORS[severity]` + detail cards below
- [ ] Age at Abandon: grouped bars, all types shown simultaneously, no type toggle
- [ ] Type toggle buttons shown ONLY for Loss Breakdown view
- [ ] Summary table always visible; Yield Rate cell colored by `yieldColor`, bold
- [ ] Downstream loss values pulled from `lossPoints.find(l => l.tier === "Downstream")`
- [ ] Stat card "DOWNSTREAM LOSS" and "AVG AGE @ ABANDON" sourced from Downstream loss point
- [ ] Tooltip for Loss view shows severity (SEVERITY_COLORS), count, loss rate, avg age
- [ ] Legend reflects current view (yield thresholds / severity / type colors)
