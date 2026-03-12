---
name: monte-carlo-forecast-chart
description: >
  Creates a dark-themed React/Recharts chart for the Monte Carlo Forecast
  (mcs-mcp:forecast_monte_carlo). Trigger on: "monte carlo chart", "forecast chart",
  "when will we finish", "how much can we deliver", "forecast visualization",
  "show the forecast", or any "show/chart/plot/visualize that" follow-up after a
  forecast_monte_carlo result is present. Handles BOTH modes: duration (days to
  completion) and scope (items deliverable within a time window). When only one mode
  was run, renders that mode directly with no toggle. When both modes were run in
  the same session, renders a toggle to switch between them.
  Always read this skill before building the chart — do not attempt it ad-hoc.
---

# Monte Carlo Forecast Chart

Scope: Use this skill ONLY when forecast_monte_carlo data is present in the
conversation. It visualises probabilistic delivery forecasts based on historical
throughput, in either duration mode, scope mode, or both.

Do not use this skill for:
- Backtest validation results      → forecast_backtest
- Cycle time SLE percentiles       → analyze_cycle_time
- Throughput volume / cadence      → analyze_throughput

---

## Prerequisites

```js
// Duration mode — forecast when N items will be complete
mcs-mcp:forecast_monte_carlo({
  board_id, project_key,
  mode: "duration",
  include_existing_backlog: true,  // auto-counts unstarted backlog items
  include_wip: true,               // also includes in-progress items
  // additional_items: N           // optional: manually specified extra items
  // targets: { Story: 10, Bug: 5 } // optional: exact type counts
})

// Scope mode — forecast how much will be delivered in N days
mcs-mcp:forecast_monte_carlo({
  board_id, project_key,
  mode: "scope",
  target_days: N,                  // required: forecast window in days
  // target_date: "YYYY-MM-DD"     // alternative to target_days
})
```

Both modes share the same optional parameters for baseline control:

```
history_window_days    lookback in days (overrides default 183d)
history_start_date     explicit start date for baseline
history_end_date       explicit end date for baseline (default: today)
issue_types            filter to specific types (e.g. ["Story", "Bug"])
start_status           override commitment point
end_status             override resolution point
mix_overrides          shift capacity distribution by type
```

---

## Response Structure — Fields Shared by Both Modes

```
data.percentiles              — 8-key object, values differ by mode (see below)
  .aggressive                 — P10
  .unlikely                   — P30
  .coin_toss                  — P50
  .probable                   — P70
  .likely                     — P85  ← professional commitment standard
  .conservative               — P90
  .safe                       — P95
  .almost_certain             — P98

data.percentile_labels        — human-readable description per key (mode-specific wording)

data.spread
  .iqr                        — interquartile range
  .inner_80                   — P10–P90 span

data.fat_tail_ratio           — tail / median ratio
data.tail_to_median_ratio     — additional tail metric
data.predictability           — "Stable" | "Moderate" | "Unstable"

data.throughput_trend
  .direction                  — "Increasing" | "Decreasing" | "Stable"
  .percentage_change          — % change recent vs. overall

data.context
  .days_in_sample             — historical baseline window in days
  .issues_analyzed            — items included in simulation
  .issues_total               — items before filtering
  .dropped_by_outcome         — items excluded (abandoned/filtered)
  .throughput_overall         — items/day over full baseline
  .throughput_recent          — items/day over recent window
  .modeling_insight           — string: stratified vs. pool modeling
  .stratification_decisions[] — per-type eligibility:
    .type                     — issue type name
    .eligible                 — boolean
    .volume                   — items of this type in baseline
    .p85_cycle_time           — P85 cycle time in days
    .reason                   — only present when eligible=false

data.composition              — duration mode only (all zeros in scope mode):
  .existing_backlog           — unstarted items auto-counted from Jira
  .wip                        — in-progress items included
  .additional_items           — manually specified extra items
  .total                      — total items being forecast

guardrails.warnings[]         — string array of threshold alerts
guardrails.insights[]         — string array of context notes
```

---

## Critical: Mode Determines Percentile Semantics

```
DURATION mode: percentiles = days to complete COMPOSITION.total items
  → lower value = faster / better
  → axis: left (P10/aggressive) to right (P98/almost_certain) = increasing days
  → P10 label: "P10 (Aggressive / Best Case)"
  → P98 label: "P98 (Limit / Extreme Outlier Boundaries)"

SCOPE mode: percentiles = items deliverable within TARGET_DAYS
  → higher value = more delivery / better
  → axis: left (P10/aggressive) to right (P98/almost_certain) = decreasing items
  → P10 label: "P10 (10% probability to deliver at least this much)"
  → P98 label: "P98 (98% probability to deliver at least this much)"

The bar chart renders in PERC_ORDER (P10→P98) for both modes.
The AXIS DIRECTION IS INTENTIONALLY OPPOSITE between modes — do not "fix" this.
In scope mode, more conservative estimates correctly show shorter bars.
```

---

## Mode Auto-Detection and Toggle Logic

```js
// DURATION and SCOPE are injection-time constants.
// Set to null if the mode was NOT run in this session.
const DURATION = { ... } || null;
const SCOPE    = { ... } || null;

const hasDuration = DURATION !== null;
const hasScope    = SCOPE    !== null;
const bothModes   = hasDuration && hasScope;

// Default active mode: duration if available, else scope
const [mode, setMode] = useState(hasDuration ? "duration" : "scope");

// Toggle: only render when bothModes === true
// When only one mode is present: no toggle, no mention of the other mode
```

---

## Injection Checklist

```
Placeholder     Source path
BOARD_ID        board_id parameter
PROJECT_KEY     project_key parameter
BOARD_NAME      board name from context / import_boards
DURATION        full structured object from duration mode response, or null
SCOPE           full structured object from scope mode response, or null
```

DURATION object shape:

```js
const DURATION = {
  percentiles:      data.percentiles,           // { aggressive, unlikely, ... }
  labels:           data.percentile_labels,     // { aggressive: "P10 ...", ... }
  spread:           data.spread,                // { iqr, inner_80 }
  predictability:   data.predictability,
  fat_tail_ratio:   data.fat_tail_ratio,
  composition:      data.composition,           // { existing_backlog, wip, additional_items, total }
  throughput_trend: data.throughput_trend,
  context:          data.context,               // { days_in_sample, issues_analyzed, ..., stratification_decisions[] }
  warnings:         guardrails.warnings,
};
```

SCOPE object shape (same minus composition, plus target_days):

```js
const SCOPE = {
  target_days:      N,                          // the target_days parameter value
  percentiles:      data.percentiles,
  labels:           data.percentile_labels,
  spread:           data.spread,
  predictability:   data.predictability,
  fat_tail_ratio:   data.fat_tail_ratio,
  throughput_trend: data.throughput_trend,
  context:          data.context,
  warnings:         guardrails.warnings,
};
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

// Percentile colors — keyed by name (safe: fixed by API contract)
const PERC_COLORS = {
  aggressive:     ALARM,
  unlikely:       CAUTION,
  coin_toss:      CAUTION,
  probable:       PRIMARY,
  likely:         POSITIVE,
  conservative:   POSITIVE,
  safe:           POSITIVE,
  almost_certain: POSITIVE,
};

// Helper colors computed at runtime:
function predictabilityColor(p) {
  const l = (p || "").toLowerCase();
  if (l.includes("stable") || l.includes("high"))     return POSITIVE;
  if (l.includes("moderate") || l.includes("medium")) return CAUTION;
  return ALARM;
}

function fatTailColor(r) {
  if (r <= 1.0) return POSITIVE;
  if (r <= 1.5) return CAUTION;
  return ALARM;
}

function trendColor(direction) {
  if (direction === "Increasing") return POSITIVE;
  if (direction === "Decreasing") return ALARM;
  return MUTED;
}
```

---

## Chart Architecture

Render order: Header → Shared Context Cards → Warnings → [Toggle] → Mode View → Footer.

The mode view contains: Composition/Window Cards → Percentile Chart → Spread Panel → Stratification Table.

### Shared Context Cards (always visible regardless of mode)

Sourced from the active mode's `context` and `throughput_trend`:

```
THROUGHPUT (OVERALL)   ctx.throughput_overall.toFixed(2) + "/d"   TEXT
                       sub: "{issues_analyzed} items · {days_in_sample}d sample"
THROUGHPUT (RECENT)    ctx.throughput_recent.toFixed(2) + "/d"    trendColor(direction)
                       sub: "{direction} · {percentage_change}%"
ITEMS ANALYZED         ctx.issues_analyzed                         TEXT
                       sub: "of {issues_total} total"
DROPPED (OUTCOME)      ctx.dropped_by_outcome                      MUTED
                       sub: "abandoned/excluded"
```

### Warnings (guardrails.warnings[])

Render as CAUTION badges with ⚠ prefix, one per warning. Only render if array is non-empty.

### Mode Toggle (only when bothModes === true)

```jsx
// Pill-style toggle, width="fit-content"
// Duration button label: `Duration — when will ${DURATION.composition.total} items finish?`
// Scope button label:    `Scope — how much in ${SCOPE.target_days} days?`
// Active: PRIMARY + "33" background, TEXT color
// Inactive: transparent background, MUTED color
// Separator: right border on duration button = BORDER
```

### Composition / Window Cards (mode-specific)

Duration mode:

```
TOTAL ITEMS    DURATION.composition.total             TEXT
BACKLOG        DURATION.composition.existing_backlog  CAUTION   sub="unstarted"
WIP            DURATION.composition.wip               PRIMARY   sub="in progress"
ADDITIONAL     DURATION.composition.additional_items  SECONDARY sub="manually added"
               (only render if additional_items > 0)
```

Scope mode:

```
TARGET WINDOW  SCOPE.target_days + "d"                SECONDARY  sub="≈ N weeks"
P85 DELIVERY   SCOPE.percentiles.likely + " items"    POSITIVE   sub="likely outcome"
P50 DELIVERY   SCOPE.percentiles.coin_toss + " items" CAUTION    sub="coin toss"
```

### Percentile Chart — horizontal bar chart

```
BarChart layout="vertical", height=280px
Margin: { top: 4, right: 60, left: 10, bottom: 4 }
XAxis type="number", domain=[0, max * 1.1]
  tickFormatter: duration → v => `${v}d`  |  scope → v => `${v} items`
YAxis type="category", dataKey="label" (short label), width=36
  tick fill=TEXT, fontSize=11
One Bar, dataKey="value", barSize=20, radius=[0,4,4,0]
  Cell per row: fill=PERC_COLORS[key], fillOpacity=0.8
```

Data construction (same for both modes):

```js
const PERC_ORDER = [
  "aggressive","unlikely","coin_toss","probable",
  "likely","conservative","safe","almost_certain"
];

const PERC_SHORT = {
  aggressive:"P10", unlikely:"P30", coin_toss:"P50", probable:"P70",
  likely:"P85", conservative:"P90", safe:"P95", almost_certain:"P98",
};

const data = PERC_ORDER.map(key => ({
  key,
  label: PERC_SHORT[key],
  value: activeMode.percentiles[key],
  unit:  isDuration ? "d" : " items",
}));
```

Optional P85 ReferenceLine (scope mode only):

```jsx
<ReferenceLine x={SCOPE.percentiles.likely} stroke={CAUTION} strokeDasharray="4 4"
  label={{ value: `P85: ${SCOPE.percentiles.likely}`, position: "top",
    fill: CAUTION, fontSize: 10, fontFamily: "'Courier New', monospace" }} />
```

PercTooltip:
- Short label (e.g. "P85") in `PERC_COLORS[key]`, bold
- Full label from `activeMode.labels[key]` in MUTED, smaller
- Value + unit in `PERC_COLORS[key]`

Color legend (centered flex row below chart):

```
ALARM   rect → "P10 — Aggressive / high risk"
CAUTION rect → "P30–P50 — Unlikely to median"
PRIMARY rect → "P70 — Probable"
POSITIVE rect → "P85–P98 — Likely to near-certain"
```

Panel subtitle:
- Duration: `"Days to complete all ${DURATION.composition.total} items · lower = faster"`
- Scope:    `"Items deliverable within ${SCOPE.target_days} days · higher = more delivery"`

### Spread Panel — 4 stat cards (flex row)

```
PREDICTABILITY   predictability string              predictabilityColor(value)
IQR SPREAD       spread.iqr                         CAUTION   sub="interquartile range"
INNER 80         spread.inner_80                    CAUTION   sub="P10–P90 span"
FAT-TAIL RATIO   fat_tail_ratio                     fatTailColor(value)  sub="tail / median"
```

### Stratification Table

Purpose: show which issue types were modeled independently vs. via pool.

```
Columns: Type | Eligible | Volume | P85 Cycle Time | Note
Ineligible row: type name in MUTED, "No" in ALARM, reason in MUTED at fontSize=10
Eligible row:   type name in TEXT, "Yes" in POSITIVE
```

Data: `activeMode.context.stratification_decisions[]`

---

## Header

```
Breadcrumb: {PROJECT_KEY} · {BOARD_NAME} · Board {BOARD_ID}
Title:      exactly "Monte Carlo Forecast"
            + mode subtitle in h1: " Duration mode" or " Scope mode"
            (fontSize=13, fontWeight=400, color=MUTED, marginLeft=12)
Subtitle:   "Throughput-based simulation · probabilistic delivery horizon"
```

---

## Footer — Two Sections (both required, mode-aware)

1. **"Reading this chart:"**
   - Duration: `Duration mode forecasts when ${DURATION.composition.total} items will be
     completed based on historical throughput. Bars show days to completion at each
     confidence level — shorter is faster. P85 (${DURATION.percentiles.likely}d) is the
     professional commitment standard.`
   - Scope: `Scope mode forecasts how many items will be delivered within
     ${SCOPE.target_days} days. Bars show item counts — higher means more delivery.
     P85 (${SCOPE.percentiles.likely} items) is the professional commitment standard.
     Note the axis is intentionally reversed from duration mode: higher confidence
     = fewer items guaranteed.`

2. **"Important:"** (same for both modes)
   This simulation is based solely on historical throughput — it does not account for
   scope changes, team changes, or holidays. Stratified modeling means each eligible
   issue type is simulated independently to capture capacity clashes. Ineligible types
   (volume too low) are modeled via the pool distribution.

Reference actual values (DURATION.composition.total, DURATION.percentiles.likely, etc.)
in the footer — never write generic placeholders.

---

## Checklist Before Delivering

- [ ] Triggered by forecast_monte_carlo data
- [ ] Chart title reads exactly "Monte Carlo Forecast"
- [ ] DURATION set to structured object or null — never omitted
- [ ] SCOPE set to structured object or null — never omitted
- [ ] Mode auto-detected: useState defaults to "duration" if available, else "scope"
- [ ] Toggle rendered ONLY when both DURATION and SCOPE are non-null
- [ ] Header mode subtitle reflects active mode ("Duration mode" / "Scope mode")
- [ ] Shared context cards always visible, sourced from active mode's context
- [ ] Warnings rendered as CAUTION badges — only if warnings[] is non-empty
- [ ] Duration composition cards: ADDITIONAL card only if additional_items > 0
- [ ] Scope window cards: target_days from SCOPE.target_days — never hardcoded
- [ ] PERC_ORDER fixed array of 8 keys — percentile_labels consumed for tooltip only
- [ ] Duration bars: lower = better (no axis reversal needed — natural left-to-right)
- [ ] Scope bars: higher = better — axis direction intentionally shows fewer items
      at higher confidence — do NOT "fix" the apparent reversal
- [ ] P85 ReferenceLine in scope mode only
- [ ] PercTooltip: short label + full label + value + unit
- [ ] Color legend below percentile chart (4 entries)
- [ ] Spread panel: 4 cards, predictability and fat-tail colors computed at runtime
- [ ] Stratification table: ineligible rows in MUTED/ALARM, eligible in TEXT/POSITIVE
- [ ] Footer values reference actual injected data — no generic placeholders
- [ ] Dark theme throughout: PAGE_BG page, PANEL_BG panels, BORDER grid
- [ ] Monospace font throughout
- [ ] Single self-contained .jsx file with default export
