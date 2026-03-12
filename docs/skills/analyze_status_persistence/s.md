---
name: status-persistence-chart
description: >
  Creates a dark-themed three-panel React/Recharts chart for the Status Persistence
  analysis (mcs-mcp:analyze_status_persistence). Trigger on: "status persistence chart",
  "persistence chart", "dwell time by status", "bottleneck by status", "how long do items
  stay in each status", "flow debt by status", or any "show/chart/plot/visualize that"
  follow-up after an analyze_status_persistence result is present. ONLY for
  analyze_status_persistence output. Do NOT use for: residence time / Little's Law
  (analyze_residence_time), cycle time SLE (analyze_cycle_time), throughput
  (analyze_throughput), or WIP age outliers (analyze_work_item_age).
  Always read this skill before building the chart — do not attempt it ad-hoc.
---

# Status Persistence Chart

Scope: Use this skill ONLY when analyze_status_persistence data is present in the
conversation. It visualises how long delivered items typically dwell in each workflow
status — broken into pool-level and per-type views — plus a tier summary panel.

IMPORTANT TERMINOLOGY: This chart uses "persistence" and "dwell" throughout.
Never use "residence" or "residency" — those terms belong to analyze_residence_time
(Sample Path / Little's Law analysis), which is a completely different tool.

Do not use this skill for:
- Residence time / Little's Law        → analyze_residence_time
- Cycle time SLE percentiles           → analyze_cycle_time
- Throughput / delivery volume         → analyze_throughput
- Individual item WIP age outliers     → analyze_work_item_age

---

## Prerequisites

```js
mcs-mcp:analyze_status_persistence({ board_id, project_key })
```

No optional parameters — the tool always returns all statuses and all issue types
with stratified persistence data.

PREREQUISITE: workflow_discover_mapping must have been confirmed before calling this
tool. Results are subpar if tier/role mappings are missing or incorrect.

---

## Response Structure

```
data.persistence[]                   — pool-level (all types combined), one entry per status:
  .statusID                          — Jira status ID string
  .statusName                        — display name
  .tier                              — "Demand" | "Upstream" | "Downstream" (Finished excluded)
  .role                              — "active" | "queue"
  .share                             — fraction of items that visited this status
  .coin_toss                         — P50 dwell time in days
  .probable                          — P70
  .likely                            — P85 (primary SLE target)
  .safe_bet                          — P95
  .iqr                               — interquartile range
  .inner_80                          — P10–P90 span
  .interpretation                    — guardrail string (queue = flow debt, active = bottleneck)

data.stratified_persistence          — per-type breakdown, same shape as persistence[]:
  .{IssueType}[]                     — e.g. Story, Bug, Activity, Defect

data.tier_summary                    — aggregated by tier:
  .{Tier}
    .count                           — items in this tier
    .combined_median                 — P50 across all tier statuses
    .combined_p85                    — P85 across all tier statuses
    .statuses[]                      — status IDs in this tier
    .interpretation                  — guardrail string
```

---

## Critical: Status Names, Issue Types, and Tier Structure Are NOT Hardcoded

NEVER hardcode status names, tier assignments, roles, or issue type names.
Derive everything from the tool response:

```js
// STATUS_ORDER — from workflow_discover_mapping, Finished tier excluded
// Use the same workflow order as CFD / other tools for consistency
const STATUS_ORDER = status_order
  .map(id => status_mapping[id])
  .filter(s => s.tier !== "Finished")
  .map(s => s.name);

// POOL_DATA — from data.persistence[], filtered to STATUS_ORDER statuses
// (Finished-tier entries like Done/Closed may appear in persistence — exclude them)
const POOL_DATA = data.persistence.filter(d =>
  STATUS_ORDER.includes(d.statusName)
);

// ALL_ISSUE_TYPES — keys of data.stratified_persistence
const ALL_ISSUE_TYPES = Object.keys(data.stratified_persistence);

// STRATIFIED — from data.stratified_persistence, same Finished filter applied
const STRATIFIED = Object.fromEntries(
  ALL_ISSUE_TYPES.map(type => [
    type,
    data.stratified_persistence[type].filter(d => STATUS_ORDER.includes(d.statusName))
  ])
);

// TIER_COLORS — keyed by tier name (safe: tier names are fixed by API contract)
const TIER_COLORS = { Demand: CAUTION, Upstream: PRIMARY, Downstream: SECONDARY };

// ROLE_COLORS — keyed by role name (safe: role names are fixed by API contract)
const ROLE_COLORS = { active: SECONDARY, queue: ALARM };

// ISSUE_TYPE_COLORS — built dynamically by index, never by name
const ISSUE_TYPE_PALETTE = [PRIMARY, ALARM, SECONDARY, CAUTION, POSITIVE, "#f97316"];
const ISSUE_TYPE_COLORS = Object.fromEntries(
  ALL_ISSUE_TYPES.map((t, i) => [t, ISSUE_TYPE_PALETTE[i % ISSUE_TYPE_PALETTE.length]])
);
```

---

## Injection Checklist

```
Placeholder         Source path
BOARD_ID            board_id parameter
PROJECT_KEY         project_key parameter
BOARD_NAME          board name from context / import_boards
STATUS_ORDER        workflow_discover_mapping → status_order mapped to names,
                      Finished tier excluded
POOL_DATA           data.persistence[] filtered to STATUS_ORDER statuses
TIER_SUMMARY        data.tier_summary (all three tiers: Demand, Upstream, Downstream)
ALL_ISSUE_TYPES     Object.keys(data.stratified_persistence)  (dynamic)
STRATIFIED          data.stratified_persistence, Finished tier filtered  (dynamic)
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
const TIER_COLORS  = { Demand: CAUTION, Upstream: PRIMARY, Downstream: SECONDARY };
const ROLE_COLORS  = { active: SECONDARY, queue: ALARM };

// Dynamic — assigned by index, never by name:
const ISSUE_TYPE_PALETTE = [PRIMARY, ALARM, SECONDARY, CAUTION, POSITIVE, "#f97316"];
```

---

## Chart Architecture — Three Panels

Render order: Tier Summary → Pool Persistence → Per-Type P85 (top to bottom).
No interactive toggles — all panels are static.

### Panel 1: Tier Summary

Purpose: immediate high-level signal before per-status detail.
Pure div layout — no chart component.

Three cards (Demand / Upstream / Downstream) in a flex row, each showing:
- Tier name as header colored `TIER_COLORS[tier]`
- P50 `combined_median` in SECONDARY
- P85 `combined_p85` in CAUTION
- Item count

Card style: `background: PAGE_BG`, `border: 1px solid TIER_COLORS[tier] at 30% opacity`.

### Panel 2: Pool Persistence — horizontal stacked bar chart

Purpose: per-status dwell distribution across all issue types combined.

```
BarChart layout="vertical", height=360px
Margin: { top: 4, right: 80, left: 10, bottom: 4 }
XAxis type="number", domain=[0, max(safe_bet) * 1.05], tickFormatter v => `${v}d`
YAxis type="category", dataKey="statusName", width=200
  Custom tick component TierTick (see below)
Data rows ordered by STATUS_ORDER via buildRows() helper
```

Two stacked `<Bar>` series (`stackId="a"`):

```jsx
// Bar 1: P85 solid — role color, 75% opacity, no border radius
<Bar dataKey="p85" stackId="a" barSize={18} radius={[0,0,0,0]}>
  {data.map((d, i) => <Cell key={i} fill={ROLE_COLORS[d.role]} fillOpacity={0.75} />)}
</Bar>

// Bar 2: P95 extension — same color, 25% opacity, rounded right edge
// p95ext = Math.max(0, safe_bet - likely)
<Bar dataKey="p95ext" stackId="a" barSize={18} radius={[0,4,4,0]}>
  {data.map((d, i) => <Cell key={i} fill={ROLE_COLORS[d.role]} fillOpacity={0.25} />)}
</Bar>
```

TierTick custom Y-axis tick component:

```jsx
function TierTick({ x, y, payload }) {
  const row = POOL_DATA.find(d => d.statusName === payload.value);
  const tierColor = row ? TIER_COLORS[row.tier] || MUTED : MUTED;
  const roleColor = row ? ROLE_COLORS[row.role] || MUTED : MUTED;
  return (
    <g transform={`translate(${x},${y})`}>
      <circle cx={-8} cy={0} r={3} fill={tierColor} opacity={0.8} />
      <text x={-16} y={0} dy={4} textAnchor="end"
        fill={roleColor} fontSize={10}
        fontFamily="'Courier New', monospace">{abbrev(payload.value)}</text>
    </g>
  );
}
```

```js
// abbrev — clip label at 28 chars for Y-axis
function abbrev(name) {
  return name.length > 28 ? name.slice(0, 26) + "…" : name;
}

// buildRows — preserve STATUS_ORDER sequence
function buildRows(dataArr) {
  const byName = Object.fromEntries(dataArr.map(d => [d.statusName, d]));
  return STATUS_ORDER.map(name => byName[name]).filter(Boolean);
}
```

PoolTooltip — grid layout:

```
Tier        TIER_COLORS[tier]
Role        ROLE_COLORS[role]
Share       Math.round(share * 100)%
P50         coin_toss + "d"      SECONDARY
P70         probable + "d"       PRIMARY
P85         likely + "d"         CAUTION
P95         safe_bet + "d"       ALARM
IQR         iqr + "d"            MUTED
Inner 80    inner_80 + "d"       MUTED
```

Legend (centered flex row below chart):

```
Cyan filled rect    → "Active (value-adding)"
Red filled rect     → "Queue (waiting / flow debt)"
Muted dim rect      → "P95 extension"
Tier dots:           ● Demand  ● Upstream  ● Downstream  "(dot = tier)"
```

Panel subtitle: `"Pool — all types · P85 solid + P95 extension"`

### Panel 3: Per-Type P85 — horizontal grouped bar chart

Purpose: compare P85 dwell across issue types for each status.

```
BarChart layout="vertical", height=360px
Margin: { top: 4, right: 80, left: 10, bottom: 4 }
XAxis type="number", domain=[0, max(all type likely values) * 1.05]
YAxis type="category", dataKey="label" (abbrev of statusName), width=200
  tick fill=TEXT
One Bar per type in ALL_ISSUE_TYPES:
  dataKey=type, barSize=7, fill=ISSUE_TYPE_COLORS[type], fillOpacity=0.75
  radius=[0,3,3,0]
```

Data construction:

```js
const data = STATUS_ORDER.map(name => {
  const row = { statusName: name, label: abbrev(name) };
  ALL_ISSUE_TYPES.forEach(type => {
    const entry = (STRATIFIED[type] || []).find(d => d.statusName === name);
    row[type] = entry ? entry.likely : 0;
  });
  return row;
});
```

StratTooltip:

```
Header: statusName (TEXT, bold)
Rows:   one per type — color=ISSUE_TYPE_COLORS[type], value="{v}d"
```

Legend (centered below chart):
Colored dot + `"{type}"` for each type in `ALL_ISSUE_TYPES` (dynamic — never hardcoded).

Panel subtitle: `"P85 persistence by status and issue type"`

---

## Stat Cards (4 cards)

```js
// Compute dynamically — never hardcode
const topBottleneck = POOL_DATA
  .filter(d => d.tier === "Downstream")
  .sort((a, b) => b.likely - a.likely)[0];
```

```
DOWNSTREAM P85   TIER_SUMMARY.Downstream.combined_p85 + "d"   SECONDARY
UPSTREAM P85     TIER_SUMMARY.Upstream.combined_p85 + "d"     PRIMARY
DEMAND P85       TIER_SUMMARY.Demand.combined_p85 + "d"       CAUTION   sub="non-blocking"
TOP BOTTLENECK   topBottleneck.statusName                      ALARM     sub="P85 = {likely}d"
```

---

## Guardrail Badges (4 badges, flex row)

```
"Delivered items only — active WIP excluded"              MUTED
"Persistence = time within one status (not end-to-end)"  MUTED
"Queue persistence = Flow Debt"                           ALARM
"Active persistence = local bottleneck signal"            SECONDARY
```

---

## Header

```
Breadcrumb: {PROJECT_KEY} · {BOARD_NAME} · Board {BOARD_ID}
Title:      exactly "Status Persistence"
Subtitle:   "Dwell time per status · delivered items only · internal dwell, not end-to-end"
```

---

## Footer — Two Sections (both required)

1. **"Reading this chart:"**
   Each bar shows how long items typically dwell in that status. Bar length = P85
   (85% of visits resolved within that time); the lighter extension = P95. Cyan bars
   are active (value-adding) stages; red bars are queues (waiting / flow debt).
   The tier dot on the Y-axis indicates workflow phase. The per-type panel compares
   P85 persistence across issue types for the same statuses.

2. **"Important:"**
   These numbers measure INTERNAL persistence within one status visit — not
   end-to-end cycle time. An item may visit the same status multiple times. Queue
   persistence ("awaiting X") represents pure flow debt — no value is added while
   items wait. This analysis uses only successfully delivered items; active WIP is
   excluded to prevent skewing historical norms.

---

## Checklist Before Delivering

- [ ] Triggered by analyze_status_persistence data
- [ ] Chart title reads exactly "Status Persistence"
- [ ] "Persistence" and "dwell" used throughout — never "residence" or "residency"
- [ ] STATUS_ORDER derived from workflow_discover_mapping — Finished tier excluded
- [ ] POOL_DATA filtered to STATUS_ORDER statuses (Done/Closed excluded)
- [ ] ALL_ISSUE_TYPES derived from Object.keys(stratified_persistence) — none hardcoded
- [ ] STRATIFIED filtered to STATUS_ORDER statuses — Finished tier excluded
- [ ] ISSUE_TYPE_COLORS assigned by index from palette — not by name
- [ ] TIER_COLORS and ROLE_COLORS hardcoded by key (safe — fixed by API contract)
- [ ] buildRows() helper preserves STATUS_ORDER sequence in both panels
- [ ] Panel 2 uses two stacked Bars: p85 (solid) + p95ext (dim extension)
- [ ] Each Bar in Panel 2 uses Cell per row for role-based color
- [ ] TierTick custom component: colored dot (tier) + colored name text (role)
- [ ] abbrev() clips at 28 chars for Y-axis labels
- [ ] Panel 3 bars built dynamically from ALL_ISSUE_TYPES — never hardcoded type list
- [ ] topBottleneck computed dynamically from POOL_DATA — never hardcoded
- [ ] Tier summary cards colored with TIER_COLORS per tier
- [ ] Guardrail badges present and correctly worded
- [ ] No interactive toggles — fully static
- [ ] Dark theme throughout: PAGE_BG page, PANEL_BG panels, BORDER grid
- [ ] Monospace font throughout
- [ ] Single self-contained .jsx file with default export
