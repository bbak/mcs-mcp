---
name: yield-chart
description: >
  Creates a dark-themed three-panel React/Recharts chart for the Yield analysis
  (mcs-mcp:analyze_yield). Trigger on: "yield chart", "delivery efficiency",
  "abandonment rate", "how much work gets done", "loss points", "what fraction
  of work gets delivered", or any "show/chart/plot/visualize that" follow-up after
  an analyze_yield result is present. ONLY for analyze_yield output. Do NOT use
  for: throughput volume (analyze_throughput), flow debt (analyze_flow_debt),
  or WIP age outliers (analyze_work_item_age).
  Always read this skill before building the chart — do not attempt it ad-hoc.
---

# Yield Chart

Scope: Use this skill ONLY when analyze_yield data is present in the conversation.
It visualises delivery efficiency — what fraction of committed work actually reaches
done — broken into a yield rate comparison, a loss breakdown by tier, and a per-type
detail panel with donuts and mini loss bars.

Do not use this skill for:
- Throughput volume / delivery cadence  → analyze_throughput
- Arrivals vs. departures balance       → analyze_flow_debt
- Individual item WIP age outliers      → analyze_work_item_age

---

## Prerequisites

```js
mcs-mcp:analyze_yield({ board_id, project_key })
```

No optional parameters — the tool always returns pool + full per-type stratification.

PREREQUISITE: workflow_discover_mapping must have been confirmed before calling this
tool, with tiers (Demand / Upstream / Downstream) correctly set. Results are subpar
without correct tier mapping.

---

## Response Structure

```
data.yield                    — pool-level summary (all types combined):
  .totalIngested              — total items that entered the commitment point
  .deliveredCount             — items that reached Finished with "delivered" outcome
  .abandonedCount             — items that reached Finished with "abandoned" outcome
  .overallYieldRate           — deliveredCount / totalIngested (0–1 float)
  .lossPoints[]               — one entry per tier where abandonment occurred:
    .tier                     — "Demand" | "Upstream" | "Downstream"
    .count                    — number of items abandoned in this tier
    .percentage               — count / totalIngested (0–1 float)
    .avgAge                   — average age at abandonment in days
    .severity                 — "Low" | "Medium" | "High"

data.stratified               — per-type breakdown, keyed by issue type name:
  .{IssueType}                — same shape as data.yield above
    .totalIngested
    .deliveredCount
    .abandonedCount
    .overallYieldRate
    .lossPoints[]

guardrails.insights[]         — string array; surface in footer or badges
```

Note: not every tier appears in lossPoints for every type — some types may have no
Upstream abandonment. Always use `.find()` to look up by tier name — never by index.

---

## Critical: Issue Types Are NOT Hardcoded

NEVER hardcode issue type names. Derive everything from the tool response:

```js
// ALL_ISSUE_TYPES — keys of data.stratified
const ALL_ISSUE_TYPES = Object.keys(data.stratified);

// ISSUE_TYPE_COLORS — assigned by index, never by name
const ISSUE_TYPE_PALETTE = [PRIMARY, ALARM, SECONDARY, CAUTION, POSITIVE, "#f97316"];
const ISSUE_TYPE_COLORS = Object.fromEntries(
  ALL_ISSUE_TYPES.map((t, i) => [t, ISSUE_TYPE_PALETTE[i % ISSUE_TYPE_PALETTE.length]])
);
```

Tier names and severity levels ARE fixed by API contract — safe to hardcode by key:

```js
const TIER_COLORS     = { Demand: CAUTION, Upstream: PRIMARY, Downstream: ALARM };
const SEVERITY_COLORS = { Low: POSITIVE, Medium: CAUTION, High: ALARM };
```

---

## Injection Checklist

```
Placeholder         Source path
BOARD_ID            board_id parameter
PROJECT_KEY         project_key parameter
BOARD_NAME          board name from context / import_boards
POOL                data.yield  (full object)
STRATIFIED          data.stratified  (full object, all types)
ALL_ISSUE_TYPES     Object.keys(data.stratified)  (dynamic)
```

---

## Computed Fields

```js
// In-flight: items neither delivered nor abandoned yet
const inFlight = POOL.totalIngested - POOL.deliveredCount - POOL.abandonedCount;

// Yield color threshold — applied per type and for pool
function yieldColor(rate) {
  if (rate >= 0.80) return POSITIVE;
  if (rate >= 0.65) return CAUTION;
  return ALARM;
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
const MUTED_DK  = "#4a5270";
const PAGE_BG   = "#080a0f";
const PANEL_BG  = "#0c0e16";
const BORDER    = "#1a1d2e";

// Fixed by API contract — safe to hardcode by key:
const TIER_COLORS     = { Demand: CAUTION, Upstream: PRIMARY, Downstream: ALARM };
const SEVERITY_COLORS = { Low: POSITIVE, Medium: CAUTION, High: ALARM };

// Dynamic — assigned by index, never by name:
const ISSUE_TYPE_PALETTE = [PRIMARY, ALARM, SECONDARY, CAUTION, POSITIVE, "#f97316"];
```

---

## Chart Architecture — Three Panels

Render order: Stat Cards → Guardrail Badges → Panel 1 → Panel 2 → Panel 3 → Footer.

### Panel 1: Yield Rate Comparison — horizontal bar chart

Purpose: compare overall yield rate across Pool + all issue types at a glance.

```
BarChart layout="vertical", height=220px
Margin: { top: 4, right: 60, left: 10, bottom: 4 }
XAxis type="number", domain=[0, 1], tickFormatter v => `${Math.round(v * 100)}%`
YAxis type="category", dataKey="type", width=70
  tick fill=TEXT, fontSize=11
One Bar, dataKey="overallYieldRate", barSize=18, radius=[0,4,4,0]
  Cell per row:
    Pool row (index 0): fill=TEXT, fillOpacity=0.35  (neutral / reference)
    Type rows:          fill=ISSUE_TYPE_COLORS[type], fillOpacity=0.75
```

Data construction:

```js
const data = [
  { type: "Pool", ...POOL },
  ...ALL_ISSUE_TYPES.map(t => ({ type: t, ...STRATIFIED[t] })),
];
```

LossTooltip — grid layout:

```
Ingested     MUTED
Delivered    POSITIVE
Abandoned    ALARM
Yield        yieldColor(rate), bold
```

Panel subtitle: `"Overall yield rate by issue type · delivered ÷ ingested"`
Note below chart: `"80% yield threshold — above = healthy · below = systemic loss"`

### Panel 2: Loss Breakdown by Tier — horizontal stacked bar chart

Purpose: show where in the workflow abandonment occurs, per issue type.

```
BarChart layout="vertical", height=220px
Margin: { top: 4, right: 60, left: 10, bottom: 4 }
XAxis type="number", domain=[0, maxLoss * 1.2]
  label: "abandoned items"
YAxis type="category", dataKey="type", width=70
  tick fill=TEXT, fontSize=11
Three stacked Bars (stackId="a"), one per tier in ["Demand","Upstream","Downstream"]:
  fill=TIER_COLORS[tier], fillOpacity=0.75, barSize=18
  Downstream bar: radius=[0,4,4,0]  (last in stack — rounded right edge)
  Others:         radius=[0,0,0,0]
```

Data construction:

```js
const tiers = ["Demand", "Upstream", "Downstream"];
const rows = [
  { type: "Pool", ...POOL },
  ...ALL_ISSUE_TYPES.map(t => ({ type: t, ...STRATIFIED[t] })),
].map(d => {
  const row = { type: d.type, total: d.totalIngested };
  tiers.forEach(tier => {
    const lp = d.lossPoints.find(l => l.tier === tier); // always .find() — never by index
    row[tier]                = lp ? lp.count      : 0;
    row[`${tier}_pct`]       = lp ? lp.percentage : 0;
    row[`${tier}_avgAge`]    = lp ? lp.avgAge      : null;
    row[`${tier}_sev`]       = lp ? lp.severity    : null;
  });
  return row;
});

const maxLoss = Math.max(...rows.map(r => tiers.reduce((s, t) => s + r[t], 0)));
```

BreakdownTooltip — per tier, only render if count > 0:

```
{tier}   TIER_COLORS[tier]   count items (pct%)
         severity: {sev} · avg age {avgAge}d     SEVERITY_COLORS[sev]
```

Tier legend (centered flex row below chart):
Colored rect + tier name for each tier in `["Demand","Upstream","Downstream"]`.

Panel subtitle: `"Abandoned items by tier · where in the workflow is work lost?"`

### Panel 3: Per-Type Detail Cards

Purpose: full breakdown per issue type — volume, yield donut, and per-tier loss mini-bars.

```
Flex wrap layout, gap=12px
Card sizing: flex="1 1 300px", minWidth=280, maxWidth="calc(33.33% - 8px)"
→ 3 cards per row, 4th wraps to next row filling full width
```

Each card contains:

1. **Header row**: type name (colored `ISSUE_TYPE_COLORS[type]`) + YieldDonut (size=64) right-aligned
2. **Volume grid** (2-column, fontSize=11):
   ```
   Ingested   TEXT
   Delivered  POSITIVE
   Abandoned  ALARM
   ```
3. **Per-tier loss rows** (only for tiers present in lossPoints):
   ```
   Label row: tier name (TIER_COLORS[tier]) + "count · pct% · avg Xd · Severity"
              severity colored SEVERITY_COLORS[severity]
   Mini-bar:  height=4px, fill=SEVERITY_COLORS[severity], opacity=0.7
              width proportional to percentage (scaled to max ~15% = 100%)
   ```

YieldDonut component:

```jsx
// Donut — 3 slices: Delivered (POSITIVE), Abandoned (ALARM), Other/InFlight (MUTED)
function YieldDonut({ data, size = 64 }) {
  const other = data.totalIngested - data.deliveredCount - data.abandonedCount;
  const slices = [
    { name: "Delivered", value: data.deliveredCount, color: POSITIVE },
    { name: "Abandoned", value: data.abandonedCount, color: ALARM    },
    ...(other > 0 ? [{ name: "Other", value: other, color: MUTED }] : []),
  ];
  // Center label: pct(overallYieldRate), colored yieldColor(rate)
  // PieChart innerRadius=size*0.33, outerRadius=size*0.47
  // startAngle=90, endAngle=-270 (clockwise from top)
  // strokeWidth=0 on cells
}
```

Card background: `PAGE_BG`, border `1px solid ISSUE_TYPE_COLORS[type] at 30% opacity`.

---

## Stat Cards (4–5 cards)

```js
const poolYieldColor = yieldColor(POOL.overallYieldRate);
const inFlight = POOL.totalIngested - POOL.deliveredCount - POOL.abandonedCount;
```

```
OVERALL YIELD     pct(POOL.overallYieldRate)                   poolYieldColor
TOTAL INGESTED    POOL.totalIngested                           TEXT
DELIVERED         POOL.deliveredCount   sub=pct(delivered/ingested)   POSITIVE
ABANDONED         POOL.abandonedCount   sub=pct(abandoned/ingested)   ALARM
IN FLIGHT         inFlight   sub="neither delivered nor abandoned"     MUTED
  (only render IN FLIGHT card if inFlight > 0)
```

---

## Guardrail Badges (3 badges, flex row)

```
"Downstream abandonment = High severity (late-stage waste)"   ALARM
"Upstream abandonment = Medium (discovery / refinement gap)"  CAUTION
"Demand abandonment = Low (normal backlog pruning)"           POSITIVE
```

---

## Header

```
Breadcrumb: {PROJECT_KEY} · {BOARD_NAME} · Board {BOARD_ID}
Title:      exactly "Yield"
Subtitle:   "Delivery efficiency · what fraction of committed work actually reaches done?"
```

---

## Footer — Two Sections (both required)

1. **"Reading this chart:"**
   Yield = delivered ÷ ingested. The rate panel shows how efficiently each issue type
   moves from commitment to done. The loss breakdown shows where abandoned items drop
   off — Downstream abandonment is the most costly (work was nearly complete).
   The per-type cards show the full picture: volume, yield donut, and loss per tier
   with severity and average age at abandonment.

2. **"Important:"**
   Abandoned Downstream items represent the highest form of waste — investment was made
   across the full delivery pipeline before the item was discarded. High avgAge at
   Downstream loss points signals items that lingered before being abandoned, compounding
   the waste. Abandoned Demand items are generally healthy backlog hygiene.

---

## Checklist Before Delivering

- [ ] Triggered by analyze_yield data
- [ ] Chart title reads exactly "Yield"
- [ ] POOL injected from data.yield
- [ ] STRATIFIED injected from data.stratified (full object)
- [ ] ALL_ISSUE_TYPES derived from Object.keys(data.stratified) — none hardcoded
- [ ] ISSUE_TYPE_COLORS assigned by index from palette — not by name
- [ ] TIER_COLORS and SEVERITY_COLORS hardcoded by key (safe — fixed by API contract)
- [ ] lossPoints always looked up with .find(l => l.tier === tier) — never by index
- [ ] Panel 1 Pool row rendered neutral (TEXT, 35% opacity) as reference bar
- [ ] Panel 2 stacked bars built from tiers array — never hardcoded type structure
- [ ] Panel 2 Downstream bar has rounded right edge; others do not
- [ ] Panel 3 cards: flex="1 1 300px", maxWidth="calc(33.33% - 8px)" → 3-per-row wrap
- [ ] YieldDonut: clockwise from top, center label colored by yieldColor threshold
- [ ] YieldDonut: in-flight slice included only if inFlight > 0
- [ ] Per-tier loss rows in Panel 3 only rendered if tier present in lossPoints
- [ ] IN FLIGHT stat card only rendered if inFlight > 0
- [ ] OVERALL YIELD stat card color computed via yieldColor() — never hardcoded
- [ ] Guardrail badges present and correctly worded
- [ ] Dark theme throughout: PAGE_BG page, PANEL_BG panels, BORDER grid
- [ ] Monospace font throughout
- [ ] Single self-contained .jsx file with default export
