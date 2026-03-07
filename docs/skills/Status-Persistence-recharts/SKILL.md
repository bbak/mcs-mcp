---
name: status-persistence-chart
description: >
  Creates a dark-themed React/Recharts chart for the **Status Persistence** analysis
  (mcs-mcp:analyze_status_persistence). Trigger on: "status persistence chart",
  "time per status", "where do items get stuck", "status bottleneck chart",
  "flow debt by status", "residency chart", or any "show/chart/plot/visualize that"
  follow-up after an analyze_status_persistence result is present.
  ONLY for analyze_status_persistence output (per-status residency times with IQR/Inner80
  dispersion, tier summary, and optional stratification by issue type).
  Do NOT use for: cycle time SLE distribution (analyze_cycle_time), throughput stability
  (analyze_throughput), WIP count stability (analyze_wip_stability), Total WIP Age
  (analyze_wip_age_stability), process stability XmR (analyze_process_stability), or
  process evolution (analyze_process_evolution). Those are different analyses requiring
  different charts.
  Always read this skill AND mcs-charts-base before building the chart — do not attempt
  it ad-hoc.
---

# Status Persistence Chart

> **Scope:** Use this skill ONLY when `analyze_status_persistence` data is present in
> the conversation. It visualizes how long delivered items spent in each workflow status,
> with dispersion metrics to identify bottlenecks and flow debt hotspots.
>
> **This skill extends `mcs-charts-base`.** Read that skill first. Everything defined
> there (stack, color tokens, typography, page/panel wrappers, stat card markup, badge
> system, CartesianGrid, tooltip base style, legend pattern, interactive controls,
> footer style, universal checklist) applies here without repetition. This skill only
> specifies what is unique to the status persistence chart.

---

## ⚠ Whitelist: Statuses to Include vs. Exclude

This is the most important rule for this chart. **Do not render all statuses.**
Apply this explicit whitelist logic before building any chart data array.

### Include — Operational Downstream + Upstream statuses only:
- All statuses with `tier: "Downstream"` (both `role: "active"` and `role: "queue"`)
- All statuses with `tier: "Upstream"`

### Exclude — always, unconditionally:
- `tier: "Demand"` statuses (e.g. "Open") — these are pre-commitment backlog states;
  residency here is not delivery time and misleads the chart scale dramatically
- `tier: "Finished"` statuses (e.g. "Done", "Closed") — terminal states; "Done" is
  always 0d (instantaneous), "Closed" reflects abandonment semantics, not flow time
- Any status with `role: "terminal"`
- Any status with `role: "ignore"`

```js
// CORRECT: whitelist filter
const OPERATIONAL = data.persistence
  .filter(s => (s.tier === "Downstream" || s.tier === "Upstream") && s.role !== "terminal")
  .sort((a, b) => WORKFLOW_ORDER.indexOf(a.statusName) - WORKFLOW_ORDER.indexOf(b.statusName));

// WRONG: do not show all statuses
// const allStatuses = data.persistence; // ← never do this
```

Sort the result by the confirmed workflow order (from `workflow_set_order` / `workflow_discover_mapping`).
If the workflow order is not available in the conversation, sort by tier (Upstream first, then Downstream)
and within tier by `likely` descending.

---

## Prerequisites

Call the tool if data is not yet in the conversation:

```js
mcs-mcp:analyze_status_persistence({ board_id, project_key })
```

Workflow mapping must be confirmed before calling — tiers must be set correctly or
results will be misleading (guardrail from the tool itself).

---

## Response Structure

```
data.persistence[]               — one entry per status:
  .statusID                      — Jira status ID
  .statusName                    — display name
  .share                         — fraction of delivered items that visited this status (0–1)
  .role                          — "active" | "queue" | "terminal"
  .tier                          — "Demand" | "Upstream" | "Downstream" | "Finished"
  .coin_toss                     — P50 median residency in days
  .probable                      — P70
  .likely                        — P85 (primary SLE threshold)
  .safe_bet                      — P95
  .iqr                           — interquartile range (P25–P75 spread)
  .inner_80                      — P10–P90 spread
  .interpretation                — optional human-readable string from server

data.stratified_persistence      — same structure, keyed by issue type:
  .Story[]   / .Activity[]  / .Bug[]  / .Defect[]

data.tier_summary                — aggregated stats by tier:
  .Demand     { combined_median, combined_p85, count, statuses[], interpretation }
  .Upstream   { combined_median, combined_p85, count, statuses[], interpretation }
  .Downstream { combined_median, combined_p85, count, statuses[], interpretation }
```

---

## Data Preparation

```js
// Whitelist filter — see ⚠ Whitelist section above
const OPERATIONAL = data.persistence
  .filter(s => (s.tier === "Downstream" || s.tier === "Upstream") && s.role !== "terminal")
  .sort((a, b) => WORKFLOW_ORDER.indexOf(a.statusName) - WORKFLOW_ORDER.indexOf(b.statusName));

// Metric selector — driven by state toggle
const metricKey = metric === "p50" ? "coin_toss"
                : metric === "p85" ? "likely"
                : "safe_bet"; // p95

// Color-by selector — driven by state toggle
const colorOf = (s) => colorBy === "role" ? ROLE_COLORS[s.role] : TIER_COLORS[s.tier];

// Per-type data: for stratified view, look up each operational status in the type array
const buildData = (type) => {
  const source = type === "Overall"
    ? OPERATIONAL
    : OPERATIONAL.map(s => {
        const match = (data.stratified_persistence[type] || [])
          .find(t => t.statusName === s.statusName);
        return match ? { ...s, ...match } : { ...s, coin_toss: 0, likely: 0, safe_bet: 0, iqr: 0, inner_80: 0 };
      });
  return source.map(s => ({
    ...s,
    value: s[metricKey],
    color: colorOf(s),
    shortName: s.statusName.length > 18 ? s.statusName.slice(0, 16) + "…" : s.statusName,
  }));
};
```

---

## Issue Type Colors and Role/Tier Color Maps

These supplement the base skill color tokens:

```js
const TYPE_COLORS = {
  Overall:  "#dde1ef",   // TEXT
  Story:    "#6b7de8",   // PRIMARY
  Activity: "#7edde2",   // SECONDARY
  Bug:      "#ff6b6b",   // ALARM
  Defect:   "#e2c97e",   // CAUTION
};

const ROLE_COLORS = {
  active:   "#6b7de8",   // PRIMARY — value-adding work
  queue:    "#e2c97e",   // CAUTION — waiting / flow debt
  terminal: "#505878",   // MUTED   (excluded from chart, included for legend completeness)
};

const TIER_COLORS = {
  Demand:     "#505878",  // MUTED
  Upstream:   "#7edde2",  // SECONDARY
  Downstream: "#6b7de8",  // PRIMARY
  Finished:   "#505878",  // MUTED (excluded from chart)
};
```

---

## Chart Architecture

Two panels stacked vertically, plus a permanent tier summary table below.

### Panel 1: Residency Bar Chart (main) — height 360px

Vertical `BarChart`. One bar per operational status, sorted by workflow order.
X-axis: `dataKey="shortName"`, rotated labels (`angle={-40}`, `textAnchor="end"`, `height={70}`),
`interval={0}` to show all labels.
Y-axis: `tickFormatter={v => v + "d"}`, domain `[0, Math.ceil(maxVal * 1.15 / 10) * 10]`.
Bar fill: driven by `colorBy` state (role or tier colors). `radius={[4,4,0,0]}`.
`<LabelList>` on top, formatter `v => v > 0 ? v + "d" : ""`, muted color.

### Panel 2: Variance Panel — height 260px

Vertical `BarChart`, same X-axis as Panel 1. Two `<Bar>` series side by side (no stack):
1. `dataKey="iqr"` — PRIMARY `#6b7de8`, `fillOpacity={0.75}` — middle 50%
2. `dataKey="inner_80"` — CAUTION `#e2c97e`, `fillOpacity={0.4}` — middle 80%

A wide `inner_80` relative to `iqr` indicates fat-tail behavior at that status.

Panel subtitle: "Variance — IQR (middle 50%) vs Inner80 (middle 80%) · wider = more unpredictable"
Add a second line: "Wide Inner80 relative to IQR indicates fat tails — the status occasionally
holds items for extreme durations."

### Tier Summary Table (always visible)

A rendered `<table>` below both panels. Columns: Tier | P50 Median | P85 (Likely) | Interpretation.
Rows: Demand, Upstream, Downstream (exclude Finished). Color each tier name with TIER_COLORS.

---

## Header (extends base skill header structure)

- **Breadcrumb:** `{PROJECT_KEY} · {board name} · Board {board_id}`
- **Title:** exactly `"Status Persistence"`
- **Subtitle:** `"Time spent per workflow status · Delivered items only · 6-month window"`

**Stat cards:**

| Label | Value | Color |
|---|---|---|
| `DOWNSTREAM P50` | `{tier_summary.Downstream.combined_median}d` | PRIMARY `#6b7de8` |
| `DOWNSTREAM P85` | `{tier_summary.Downstream.combined_p85}d` | CAUTION `#e2c97e` |
| `UPSTREAM P85` | `{tier_summary.Upstream.combined_p85}d` | SECONDARY `#7edde2` |
| `TOP BOTTLENECK` | name of status with highest `likely` value (among OPERATIONAL) | ALARM `#ff6b6b` |
| `BOTTLENECK P85` | `{topBottleneck.likely}d` | ALARM `#ff6b6b` |

Top bottleneck is the OPERATIONAL status (after whitelist filter) with the highest `likely` value.
Truncate the name if needed: `name.replace("await", "awt")` or `name.slice(0, 14) + "…"`.

---

## Badge Row (extends base skill badge system)

Always show:
1. `⚠ Primary bottleneck: {topBottleneck.statusName} (P85 = {topBottleneck.likely}d)` — ALARM
2. `Downstream P85: {tier_summary.Downstream.combined_p85}d` — CAUTION
3. `Upstream P85: {tier_summary.Upstream.combined_p85}d` — SECONDARY

**Interactive controls** (right-aligned in badge row, per base skill interactive controls pattern):

Metric toggle — controls which percentile is shown in Panel 1:
- `P50` / `P85` (default) / `P95` — use CAUTION as active color

Color-by toggle — controls bar fill coloring in Panel 1:
- `BY ROLE` / `BY TIER` — use PRIMARY as active color

---

## Type Toggle Buttons

Below the badge row, one button per available type: `Overall` (default), then any type
present in `data.stratified_persistence` (typically Story, Activity, Bug; skip Defect
unless it has > 1 item with non-zero values).

Follows the base skill's interactive controls button style, using `TYPE_COLORS[type]`
as the active color. Switching type re-renders both panels with that type's data.

---

## Tooltips (extends base skill tooltip base style)

Show on hover over either panel:

```
{statusName}
{tier} · {role}           ← tier color · role color respectively
──────────────────────────
P50 Median:    {coin_toss}d    ← SECONDARY
P85 (Likely):  {likely}d       ← CAUTION, bold
P95 (Safe Bet):{safe_bet}d     ← ALARM
──────────────────────────
IQR:           {iqr}d
Inner80:       {inner_80}d
Visit share:   {(share*100).toFixed(0)}%
```

---

## Legend (extends base skill legend pattern)

**Panel 1 legend** — centered below panel, filled rect swatches, content depends on `colorBy`:

When `colorBy === "role"`:
```
■ PRIMARY   active  (value-adding work)
■ CAUTION   queue   (waiting / flow debt)
```

When `colorBy === "tier"`:
```
■ SECONDARY  Upstream
■ PRIMARY    Downstream
```

**Panel 2 legend** — centered below variance panel:
```
■ PRIMARY (0.75 opacity)  IQR — middle 50% (P25–P75)
■ CAUTION (0.40 opacity)  Inner80 — middle 80% (P10–P90)
```

---

## Footer Content (follows base skill footer style)

Two sections:

1. **"Reading this chart:"** — Each bar shows how long items typically resided in a
   single workflow status before moving on. These are internal durations, not
   end-to-end times. P85 is the primary threshold — 85% of visits are shorter.
   Active statuses with high P85 are local bottlenecks where work accumulates.
   Queue statuses with high P85 are pure flow debt — waiting time with no value being
   added. The variance panel reveals predictability: a wide Inner80 relative to IQR
   signals that the status occasionally holds items for extreme durations (fat-tail behavior).

2. **"Data scope:"** — Only items that reached a "delivered" resolution are included;
   active WIP is excluded to prevent inflating historical norms. Demand and Finished
   statuses are excluded from the chart — they reflect backlog age and terminal states,
   not delivery flow. Visit share shows the fraction of delivered items that passed
   through each status.

---

## Chart-Specific Checklist

> The universal checklist is in `mcs-charts-base`. Only chart-specific items are listed here.

- [ ] Both `mcs-charts-base` and this skill read before building
- [ ] Skill triggered by `analyze_status_persistence` data
- [ ] Chart title reads exactly **"Status Persistence"**
- [ ] **Whitelist filter applied** — only `tier: "Downstream"` and `tier: "Upstream"` statuses rendered
- [ ] **Demand statuses (e.g. "Open") explicitly excluded** — never appear in any panel
- [ ] **Finished/terminal statuses (e.g. "Done", "Closed") explicitly excluded** — never appear in any panel
- [ ] Statuses sorted by confirmed workflow order
- [ ] Panel 1: metric toggle (P50 / P85 / P95) drives bar heights
- [ ] Panel 1: color-by toggle (BY ROLE / BY TIER) drives bar colors
- [ ] Panel 2: IQR and Inner80 shown as side-by-side bars (not stacked)
- [ ] Tier summary table always visible; rows for Demand, Upstream, Downstream only
- [ ] Type toggles: Overall + types from `stratified_persistence` (skip Defect if sparse)
- [ ] Top bottleneck identified from OPERATIONAL whitelist (highest `likely`), not all statuses
- [ ] Stat cards: Downstream P50, Downstream P85, Upstream P85, Top Bottleneck name + P85
- [ ] Tooltip shows tier + role, P50/P85/P95, IQR, Inner80, visit share
- [ ] Legend reflects current `colorBy` state (role vs. tier)
