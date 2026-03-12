---
name: cycle-time-sle-chart
description: >
  Creates a dark-themed three-panel React/Recharts chart for the Cycle Time SLE analysis
  (mcs-mcp:analyze_cycle_time). Trigger on: "cycle time SLE", "service level expectation",
  "SLE chart", "percentile distribution", "how long does an item take", "P85 chart",
  or any "show/chart/plot/visualize that" follow-up after an analyze_cycle_time result
  is present. ONLY for analyze_cycle_time output (SLE percentile tables, stratification,
  fat-tail metrics). Do NOT use for: cycle time stability over time (analyze_process_stability),
  process evolution (analyze_process_evolution), or throughput (analyze_throughput).
  Always read this skill before building the chart — do not attempt it ad-hoc.
---

# Cycle Time SLE Chart

Scope: Use this skill ONLY when analyze_cycle_time data is present in the conversation.
It visualises Service Level Expectation (SLE) percentile distributions — pool-level and
per-type — plus predictability / fat-tail diagnostics. This is a STATIC SNAPSHOT, not a
time series. There are no individual item dots and no time axis.

Do not use this skill for:
- Cycle time stability over time    → analyze_process_stability
- Long-term process evolution       → analyze_process_evolution
- Throughput / delivery volume      → analyze_throughput
- WIP age or aging outliers         → analyze_work_item_age

---

## Prerequisites

```js
mcs-mcp:analyze_cycle_time({
  board_id, project_key,
  issue_types: null,        // optional: filter to specific types
  start_status: null,       // optional: override commitment point
  end_status: null,         // optional: override finish status
})
```

API options note:
- `issue_types`: if provided, restricts the pool analysis to those types. Affects all
  percentiles and stratification. If omitted, all types are included.
- `start_status` / `end_status`: explicit overrides for cycle time measurement boundaries.
  When omitted, the commitment point and Finished tier from workflow_discover_mapping are used.
- No time-series or granularity parameter — this tool returns a single static snapshot.

---

## Response Structure

```
data.percentiles              — pool-level SLE (all eligible types combined):
  .aggressive                 — P10
  .unlikely                   — P30
  .coin_toss                  — P50 (median)
  .probable                   — P70
  .likely                     — P85 (canonical SLE)
  .conservative               — P90
  .safe                       — P95
  .almost_certain             — P98

data.percentile_labels        — { key: "P10 (Aggressive / Fast Outliers)", ... }
                                Human-readable label per percentile key

data.spread
  .iqr                        — interquartile range (P25–P75)
  .inner_80                   — P10–P90 span

data.fat_tail_ratio           — P98 / P50: ≥5.6 = extreme outliers in control
data.tail_to_median_ratio     — P95 / P50: >3 = highly volatile
data.predictability           — string label e.g. "Unstable & Volatile"

data.throughput_trend
  .direction                  — "Increasing" | "Decreasing" | "Stable"
  .percentage_change          — % delta recent vs. overall

data.type_sles                — per-type percentile tables, same keys as data.percentiles
  .{IssueType}                — e.g. Activity, Bug, Story, Defect
    (same 8 keys as pool)

data.context
  .issues_analyzed            — items included after filtering
  .issues_total               — total items before filtering
  .days_in_sample             — window length
  .dropped_by_outcome         — excluded as non-deliveries
  .modeling_insight           — string describing stratification strategy
  .stratification_decisions[]
    .type                     — issue type name
    .eligible                 — boolean
    .reason                   — why eligible or not
    .volume                   — item count
    .p85_cycle_time           — this type's P85
  .stratification_eligible    — { IssueType: boolean } shorthand map
```

---

## Critical: Issue Types Are NOT Hardcoded

NEVER hardcode issue type names. Derive them from `context.stratification_decisions[]`:

```js
// ALL_ISSUE_TYPES — all types seen in the response
const ALL_ISSUE_TYPES = context.stratification_decisions.map(d => d.type);

// TYPE_SLES — keyed by type name, augmented with eligibility and volume
const TYPE_SLES = Object.fromEntries(
  context.stratification_decisions.map(d => [d.type, {
    eligible: d.eligible,
    volume:   d.volume,
    ...data.type_sles[d.type],   // all 8 percentile keys
  }])
);

// ISSUE_TYPE_COLORS — built dynamically, never hardcoded per name
const ISSUE_TYPE_PALETTE = [
  "#6b7de8","#ff6b6b","#7edde2","#e2c97e",
  "#6bffb8","#f97316","#8b5cf6","#ec4899",
];
const ISSUE_TYPE_COLORS = Object.fromEntries(
  ALL_ISSUE_TYPES.map((t, i) => [t, ISSUE_TYPE_PALETTE[i % ISSUE_TYPE_PALETTE.length]])
);
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
```

Percentile color ramp (fast → slow, assigned by key — these ARE fixed):

```js
// The 8 percentile keys are fixed by the API contract — safe to hardcode by key.
// Issue type names are NOT safe to hardcode — use ISSUE_TYPE_PALETTE by index.
const PERC_COLORS = {
  aggressive:     POSITIVE,    // P10
  unlikely:       "#4ade80",   // P30
  coin_toss:      SECONDARY,   // P50
  probable:       PRIMARY,     // P70
  likely:         CAUTION,     // P85 ← canonical SLE
  conservative:   "#f97316",   // P90
  safe:           ALARM,       // P95
  almost_certain: "#c026d3",   // P98
};
```

---

## Injection Checklist

```
Placeholder             Source path
BOARD_ID                board_id parameter
PROJECT_KEY             project_key parameter
BOARD_NAME              board name from context / import_boards
POOL_PERCENTILES        data.percentiles (all 8 keys, rounded to 2dp)
IQR                     data.spread.iqr
INNER_80                data.spread.inner_80
FAT_TAIL_RATIO          data.fat_tail_ratio
TAIL_TO_MEDIAN          data.tail_to_median_ratio
PREDICTABILITY          data.predictability
ISSUES_ANALYZED         data.context.issues_analyzed
ISSUES_TOTAL            data.context.issues_total
DAYS_IN_SAMPLE          data.context.days_in_sample
DROPPED_OUTCOME         data.context.dropped_by_outcome
THROUGHPUT_DIR          data.throughput_trend.direction
THROUGHPUT_PCT          data.throughput_trend.percentage_change
MODELING_INSIGHT        data.context.modeling_insight
ALL_ISSUE_TYPES         data.context.stratification_decisions[].type  (dynamic)
TYPE_SLES               data.type_sles + stratification_decisions     (dynamic)
```

---

## Chart Architecture — Three Panels

Render order: Predictability → Pool SLE → Per-Type SLE (top to bottom).
No interactive toggles or selectors — all panels are static.

### Panel 1: Predictability / Fat-Tail / Spread

Purpose: immediate risk signal before the practitioner reads any numbers.

Warning badges (flex row, wrapping):

```
"Predictability: {PREDICTABILITY}"         ALARM if contains "Unstable" or "Volatile"
"Fat-tail ratio: {FAT_TAIL_RATIO}× ..."    ALARM if ≥5.6, CAUTION if ≥3, POSITIVE otherwise
"Tail-to-median: {TAIL_TO_MEDIAN}× ..."    ALARM if ≥3,   CAUTION if ≥2, POSITIVE otherwise
"Throughput: {THROUGHPUT_DIR} +{PCT}%"     CAUTION always (directional signal)
"{MODELING_INSIGHT}"                        MUTED
```

Spread strip (proportional horizontal range bar):

```
Full-width bar, height=28px, background=#12141e, border-radius=6
Scale fn:     v => `${Math.min((v / P98) * 100, 100).toFixed(1)}%`
IQR band:     left=scale(P30), width=scale(P70)−scale(P30), fill=PRIMARY at 30% opacity
Inner80 band: left=scale(P10), width=scale(P90)−scale(P10), fill=PRIMARY at 18% opacity,
              inset 8px top/bottom
P50 tick:     2px wide, fill=SECONDARY
P85 tick:     2px wide, fill=CAUTION
P98 tick:     2px wide at right edge, fill=ALARM
```

Legend row below strip:

```
P10 (POSITIVE) · P50 (SECONDARY) · P85 (CAUTION) · P98 (ALARM)
```

Panel height: auto — pure div layout, no chart component.

### Panel 2: Pool SLE — horizontal bar chart

Purpose: full percentile distribution at a glance.

```
BarChart layout="vertical", height=280px
Margin: { top: 4, right: 80, left: 140, bottom: 4 }
XAxis type="number", domain=[0, P98 * 1.05], tickFormatter v => `${v}d`
YAxis type="category", dataKey="label", width=130
  tick fill=TEXT
Bar dataKey="days", radius=[0,4,4,0], barSize=22
  Cell per bar: fill=PERC_COLORS[key], fillOpacity= isSLE ? 1.0 : 0.55
ReferenceLine x={P85} stroke=CAUTION strokeDasharray="4 3" strokeWidth=1.5
  label: `SLE ${round1(P85)}d`, fill=CAUTION, position="right"
```

Data array (built from PERC_KEYS in order):

```js
const PERC_KEYS = [
  "aggressive","unlikely","coin_toss","probable",
  "likely","conservative","safe","almost_certain"
];

const data = PERC_KEYS.map(k => ({
  key:   k,
  label: shortLabel(k),     // e.g. "P85 · SLE" derived from PERC_LABELS
  days:  round1(POOL_PERCENTILES[k]),
  color: PERC_COLORS[k],
  isSLE: k === "likely",
}));
```

Spread footer below chart:

```
"IQR (P25–P75): {IQR}d"     "Inner 80 (P10–P90): {INNER_80}d"
```

Panel subtitle:

```
"Pool SLE — All issue types combined · {ISSUES_ANALYZED} items · {DAYS_IN_SAMPLE} days"
```

### Panel 3: Per-Type SLE — horizontal grouped bar chart

Purpose: compare eligible type streams at P85 (fixed highlight). P85 is always
highlighted — no user toggle.

```
BarChart layout="vertical", height=280px
Margin: { top: 4, right: 80, left: 140, bottom: 4 }
XAxis type="number", domain=[0, max(all eligible type almost_certain) * 1.05]
YAxis type="category", dataKey="label", width=130
  Custom tick: bold + CAUTION color for P85 row, TEXT color for all others
One Bar per eligible type, barSize=7, radius=[0,3,3,0]
  fill=ISSUE_TYPE_COLORS[type], fillOpacity=0.75
```

Data construction:

```js
const eligibleTypes = ALL_ISSUE_TYPES.filter(t => TYPE_SLES[t].eligible);

const data = PERC_KEYS.map(k => {
  const row = { key: k, label: shortLabel(k), isSLE: k === "likely" };
  eligibleTypes.forEach(t => { row[t] = round1(TYPE_SLES[t][k]); });
  return row;
});
```

Ineligible types notice (below chart, only if any ineligible types exist):

```
"Ineligible (volume too low — collapsed to pool): {type} ({volume})"
Each type name colored ISSUE_TYPE_COLORS[type]
```

Type legend (centered below ineligible notice):

```
colored dot + "{type} (n={volume})" for each eligible type (dynamic — never hardcoded)
```

Panel subtitle:

```
"Per-type SLE comparison · eligible streams · P85 highlighted"
```

---

## Tooltip (shared across panels 2 and 3)

```
Background: #0f1117, border: BORDER, border-radius: 8, monospace font

Panel 2 tooltip:
  Header: d.label  color=d.color, bold
  Body:   "{d.days}d"
  Footer (isSLE only): "← canonical SLE"  color=CAUTION

Panel 3 tooltip:
  Header: label (percentile name)  color=TEXT, bold
  Rows:   one per eligible type — color=ISSUE_TYPE_COLORS[t], value="{v}d"
```

---

## Stat Cards (5 cards)

```
P50 MEDIAN    round1(coin_toss) + "d"             SECONDARY
P85 SLE       round1(likely) + "d"                CAUTION
P95 SAFE BET  round1(safe) + "d"                  ALARM
FAT-TAIL ×    FAT_TAIL_RATIO + "×"                ALARM      sub="≥5.6 = extreme"
ANALYZED      ANALYZED + " / " + TOTAL            MUTED      sub="−{DROPPED} by outcome"
```

---

## Header

```
Breadcrumb: {PROJECT_KEY} · {BOARD_NAME} · Board {BOARD_ID}
Title:      exactly "Cycle Time SLE"
Subtitle:   "Service Level Expectations · {ISSUES_ANALYZED} of {ISSUES_TOTAL} items
             analyzed · {DAYS_IN_SAMPLE} days"
```

---

## Footer — Two Sections (both required)

1. **"Reading this chart:"**
   The pool SLE panel shows the full percentile distribution for all item types
   combined — how long it takes 10%, 30%, 50%... 98% of items to complete. P85 is
   the canonical SLE: 85% of items finish within that time. The per-type panel
   compares the same percentiles across independent delivery streams, always
   highlighting P85.

2. **"Warning:"**
   State the fat-tail ratio and its implication for SLE reliability. Note how many
   items were dropped by outcome. Mention that stratification decisions are automatic
   based on volume and variance thresholds.

---

## Checklist Before Delivering

- [ ] Triggered by analyze_cycle_time data
- [ ] Chart title reads exactly "Cycle Time SLE"
- [ ] ALL_ISSUE_TYPES derived from stratification_decisions — none hardcoded
- [ ] TYPE_SLES built from type_sles + stratification_decisions — none hardcoded
- [ ] ISSUE_TYPE_COLORS assigned by index from ISSUE_TYPE_PALETTE — not by name
- [ ] PERC_COLORS hardcoded by key (safe — keys are fixed by API contract)
- [ ] Three panels rendered top-to-bottom: Predictability → Pool SLE → Per-Type SLE
- [ ] No interactive toggles or percentile selector — fully static
- [ ] Pool SLE uses Cell per bar for individual fill colors
- [ ] P85 bar has fillOpacity=1.0, all others 0.55
- [ ] P85 ReferenceLine rendered on Pool SLE panel
- [ ] Per-type panel always highlights P85 row (bold CAUTION YAxis tick)
- [ ] Ineligible types listed below Panel 3 with volume (only if any exist)
- [ ] Spread strip in Panel 1 uses proportional scale anchored to P98
- [ ] Fat-tail badge color: ALARM ≥5.6, CAUTION ≥3, POSITIVE otherwise
- [ ] Tail-to-median badge color: ALARM ≥3, CAUTION ≥2, POSITIVE otherwise
- [ ] No time axis anywhere — this is a static percentile snapshot
- [ ] No individual item dots — those belong to analyze_process_stability
- [ ] All data embedded as JS literals — no fetch calls
- [ ] Dark theme throughout: PAGE_BG page, PANEL_BG panels, BORDER grid
- [ ] Monospace font throughout
- [ ] Single self-contained .jsx file with default export
