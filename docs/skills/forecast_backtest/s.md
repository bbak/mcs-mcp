---
name: forecast-backtest-chart
description: >
  Creates a dark-themed React/Recharts chart for the Walk-Forward Backtest
  (mcs-mcp:forecast_backtest). Trigger on: "backtest chart", "walk-forward chart",
  "forecast accuracy", "validate the forecast", "how reliable is the simulation",
  "show the backtest", or any "show/chart/plot/visualize that" follow-up after a
  forecast_backtest result is present. Handles BOTH modes: duration (days to
  completion) and scope (items deliverable within a time window). When only one
  mode was run, renders that mode directly with no toggle. When both modes were
  run in the same session, renders a toggle to switch between them.
  Always read this skill before building the chart — do not attempt it ad-hoc.
---

# Walk-Forward Backtest Chart

Scope: Use this skill ONLY when forecast_backtest data is present in the
conversation. It visualises the empirical accuracy of Monte Carlo forecasts by
replaying past checkpoints and comparing predicted cones to actual outcomes.

Do not use this skill for:
- Monte Carlo forecast output     → forecast_monte_carlo
- Cycle time SLE percentiles      → analyze_cycle_time
- Throughput volume / cadence     → analyze_throughput

---

## Prerequisites

```js
// Scope mode — validates "how many items delivered in N days?"
mcs-mcp:forecast_backtest({
  board_id, project_key,
  simulation_mode: "scope",
  forecast_horizon_days: 14,   // default
})

// Duration mode — validates "how many days to deliver N items?"
mcs-mcp:forecast_backtest({
  board_id, project_key,
  simulation_mode: "duration",
  items_to_forecast: 5,        // default
})
```

Both modes share optional baseline parameters:

```
history_window_days    lookback in days
history_start_date     explicit start date
history_end_date       explicit end date
issue_types            filter to specific types
```

---

## Response Structure

```
data.accuracy
  .accuracy_score          — float 0–1 (e.g. 0.84)
  .checkpoints[]           — one entry per past checkpoint:
    .date                  — ISO date string "YYYY-MM-DD"
    .actual_value          — what actually happened (items or days, per mode)
    .predicted_p50         — P50 from simulation at that checkpoint
    .predicted_p85         — P85 from simulation at that checkpoint
    .predicted_p95         — P95 from simulation at that checkpoint
    .is_within_cone        — boolean: actual fell inside P10–P98 cone
    .drift_detected        — boolean: process shift detected at this point
  .validation_message      — human-readable summary string
```

---

## Mode Auto-Detection and Toggle Logic

```js
// DURATION and SCOPE are injection-time constants.
// Set to null if the mode was NOT run in this session.
const SCOPE    = { ... } || null;
const DURATION = { ... } || null;

const hasScope    = SCOPE    !== null;
const hasDuration = DURATION !== null;
const bothModes   = hasScope && hasDuration;

// Default active mode: duration if available, else scope
const [mode, setMode] = useState(hasDuration ? "duration" : "scope");

// Toggle: only render when bothModes === true
// When only one mode is present: no toggle, no mention of the other mode
```

---

## Data Flattening

Flatten checkpoints at injection time into a compact shape:

```js
// Per-checkpoint shape (rename for brevity):
//   actual  <- actual_value
//   p50     <- predicted_p50
//   p85     <- predicted_p85
//   p95     <- predicted_p95
//   hit     <- is_within_cone
//   drift   <- drift_detected

// Derive summary values:
const hits    = data.accuracy.checkpoints.filter(c => c.is_within_cone).length;
const total   = data.accuracy.checkpoints.length;
const misses  = checkpoints.filter(c => !c.hit);
const driftPts = checkpoints.filter(c => c.drift);
```

Chart data is rendered **chronologically** (oldest → newest, left → right):

```js
const chartData = [...checkpoints].reverse();  // API returns newest-first
```

---

## Injection Checklist

```
Placeholder        Source path
BOARD_ID           board_id parameter
PROJECT_KEY        project_key parameter
BOARD_NAME         board name from context / import_boards
SCOPE              full structured object from scope mode response, or null
DURATION           full structured object from duration mode response, or null
```

SCOPE / DURATION object shape:

```js
{
  simulation_mode: "scope" | "duration",
  accuracy_score:  data.accuracy.accuracy_score,         // float
  hits:            checkpoints.filter(c=>c.hit).length,
  total:           checkpoints.length,
  validation_msg:  data.accuracy.validation_message,
  checkpoints: [
    { date, actual, p50, p85, p95, hit, drift },         // flattened
    ...
  ],
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
```

Accuracy color function:

```js
function accuracyColor(score) {
  if (score >= 0.80) return POSITIVE;   // reliable
  if (score >= 0.65) return CAUTION;    // moderate
  return ALARM;                          // unreliable
}
```

---

## Chart Architecture

Render order (top to bottom):

```
Header
[Mode Toggle — only if bothModes]
Stat Cards
  └─ BacktestPanel (per active mode):
       Reliability Panel (accuracy gauge strip)
       Badges
       Main Chart Panel (Actual vs. Predicted)
       Miss Detail Table (only if misses > 0)
Footer
```

---

## Header

```
Breadcrumb: {PROJECT_KEY} · {BOARD_NAME} · Board {BOARD_ID}
Title:      exactly "Walk-Forward Backtest"
            + mode subtitle: "Duration mode" or "Scope mode"
            (fontSize=13, fontWeight=400, color=MUTED, marginLeft=12, inline)
Subtitle:   "Empirical validation of Monte Carlo forecast accuracy · time-travel reconstruction"
```

---

## Mode Toggle (only when bothModes === true)

```jsx
// Pill-style toggle, inline-flex, border BORDER, borderRadius=6, overflow hidden
// Duration button: "Duration — days to completion"
//   borderRight: `1px solid ${BORDER}`
// Scope button:    "Scope — items per window"
// Active:   PRIMARY+"33" background, TEXT color
// Inactive: transparent background, MUTED color
// border: none on both buttons (outer container provides the border)
```

---

## Stat Cards (5 cards)

```
ACCURACY       Math.round(score*100) + "%"           accuracyColor(score)
WITHIN CONE    hits + " / " + total                  PRIMARY    sub="checkpoints"
MISSES         total − hits                          ALARM      sub="outside cone"
DRIFT SIGNALS  driftPts.length                       ALARM if >0, POSITIVE if 0
                                                                sub="process shifts"
MODE           "Duration" | "Scope"                  SECONDARY  sub=mode description
```

---

## Reliability Panel

### Accuracy Gauge Strip

```
Label row (flex space-between):
  left:  "0%"                        MUTED
  center: "{pct}% accuracy — {hits}/{total} checkpoints within cone"  accuracyColor
  right: "100%"                      MUTED

Strip bar (height=14, background=#12141e, borderRadius=6, overflow hidden):
  Fill: left=0, width="{pct}%", background=accuracyColor, opacity=0.75, borderRadius=6
  Threshold marker at 65%: width=1, background=CAUTION, opacity=0.6
  Threshold marker at 80%: width=1, background=POSITIVE, opacity=0.6

Legend row below strip (fontSize=10, MUTED):
  "■ <65% unreliable"    ALARM
  "■ 65–79% moderate"    CAUTION
  "■ ≥80% reliable"      POSITIVE
```

Below strip: `validation_msg` in MUTED, fontSize=11, fontStyle=italic.

---

## Badges (flex row, wrapping)

```
"Accuracy: {pct}%"                      accuracyColor
"Misses: {misses} / {total}"            ALARM if misses>0, POSITIVE if 0
"Drift: {N} checkpoint(s)"              ALARM — only render if driftPts.length > 0
"Duration — days to completion"         MUTED — or "Scope — items per window"
```

---

## Main Chart Panel — ComposedChart

Title: `"Actual vs. Predicted — Chronological Checkpoints"`

Subtitle (MUTED, fontSize=11):
- Duration: `"Actual days to delivery vs. simulated P50 / P85 / P95 at each past checkpoint"`
- Scope:    `"Actual items delivered in Nd vs. simulated P50 / P85 / P95 at each past checkpoint"`

### Legend (flex row, wrapping, above chart)

```
Swatch (solid rect)   POSITIVE  "Actual (within cone)"
Swatch (solid rect)   ALARM     "Actual (outside cone)"
Swatch (SVG dashed)   SECONDARY "P50 predicted"   strokeDasharray="4 2"
Swatch (SVG dashed)   CAUTION   "P85 predicted"   strokeDasharray="6 3"
Swatch (SVG dashed)   PRIMARY   "P95 predicted"   strokeDasharray="3 3"
```

Swatch component:

```jsx
function Swatch({ color, label, dashed }) {
  return (
    <div style={{ display:"flex", alignItems:"center", gap:5 }}>
      {dashed
        ? <svg width="20" height="10">
            <line x1="0" y1="5" x2="20" y2="5"
              stroke={color} strokeDasharray={dashed} strokeWidth="1.5"/>
          </svg>
        : <div style={{ width:14, height:10, background:color,
            borderRadius:2, opacity:0.85 }} />
      }
      <span style={{ fontSize:11, color:MUTED,
        fontFamily:"'Courier New', monospace" }}>{label}</span>
    </div>
  );
}
```

### ResponsiveContainer + ComposedChart

```jsx
<ResponsiveContainer width="100%" height={440}>
  <ComposedChart data={chartData}
    margin={{ top:8, right:20, left:10, bottom:50 }}>
    <CartesianGrid strokeDasharray="3 3" stroke={BORDER} />
    <XAxis dataKey="date" tickFormatter={shortDate}
      tick={{ fill:MUTED, fontSize:10, fontFamily:"'Courier New', monospace" }}
      angle={-45} textAnchor="end" height={50} interval={1} />
    <YAxis domain={[0, yMax]} tickFormatter={v => `${v}${unitShort}`}
      tick={{ fill:MUTED, fontSize:10, fontFamily:"'Courier New', monospace" }} />
    <Tooltip content={<CheckpointTooltip isDuration={isDuration} />}
      cursor={{ fill:`${PRIMARY}0c` }} />
    <Bar dataKey="actual" barSize={16} radius={[3,3,0,0]}>
      {chartData.map((d, i) => (
        <Cell key={`cell-${i}`}
          fill={d.hit ? POSITIVE : ALARM}
          fillOpacity={d.hit ? 0.6 : 0.85} />
      ))}
    </Bar>
    <Line type="monotone" dataKey="p50" stroke={SECONDARY}
      strokeDasharray="4 2" strokeWidth={1.5} dot={false} />
    <Line type="monotone" dataKey="p85" stroke={CAUTION}
      strokeDasharray="6 3" strokeWidth={1.5} dot={false} />
    <Line type="monotone" dataKey="p95" stroke={PRIMARY}
      strokeDasharray="3 3" strokeWidth={1} dot={false} />
  </ComposedChart>
</ResponsiveContainer>
```

Notes:
- `yMax = Math.max(...checkpoints.flatMap(c => [c.actual, c.p50, c.p95])) * 1.15`
- `unitShort`: duration → `"d"`, scope → `""` (unit appears in tooltip only)
- `shortDate(iso)`: formats "YYYY-MM-DD" → "MM/DD" for axis ticks
- No axis labels — ticks only. No legend component — use Swatch.
- Panel `padding: "16px 20px 0 20px"` — zero bottom padding so chart fills flush.

### CheckpointTooltip

```
Background: #0f1117, border: BORDER, borderRadius: 8, monospace
Header: d.date  TEXT, bold
Body:
  Actual:  d.actual.toFixed(1) + unit   POSITIVE if hit, ALARM if miss, bold
  P50:     d.p50 + unit                 SECONDARY
  P85:     d.p85 + unit                 CAUTION
  P95:     d.p95 + unit                 PRIMARY
Footer (always): "✓ Within cone" POSITIVE bold  |  "✗ Outside cone" ALARM bold
```

---

## Miss Detail Table (only if misses.length > 0)

Columns: DATE | ACTUAL | P50 | P85 | P95 | DIRECTION

```
DATE:      d.date                           CAUTION
ACTUAL:    d.actual.toFixed(1) + unit       ALARM, bold
P50:       d.p50 + unit                     SECONDARY
P85:       d.p85 + unit                     CAUTION
P95:       d.p95 + unit                     PRIMARY
DIRECTION: computed label                   POSITIVE if favorable, ALARM if unfavorable
```

Direction logic:

```js
// Duration mode: actual < p50 = faster than forecast = favorable
// Scope mode:    actual > p50 = more than forecast   = favorable
const over = isDuration ? d.actual < d.p50 : d.actual > d.p50;
const dirLabel = isDuration
  ? (over ? "Under (faster than forecast)" : "Over (slower than forecast)")
  : (over ? "Over (more than forecast)"    : "Under (less than forecast)");
```

Alternating row tint: `i%2===0 ? "transparent" : PRIMARY+"05"`

---

## Footer — Three Sections (all required, mode-aware)

1. **"Reading this chart:"**
   Each bar is a past checkpoint where the system reconstructed the Jira state at that
   date, ran a Monte Carlo simulation, and checked whether the actual outcome fell inside
   the predicted cone (P10–P98). Green bars are hits; red bars are misses. The P50, P85,
   and P95 lines show what the simulation predicted at that moment in time.
   Append mode-specific sentence:
   - Duration: `"In duration mode, the actual value is the number of days the forecasted items actually took to deliver."`
   - Scope:    `"In scope mode, the actual value is the number of items actually delivered within the forecast window."`

2. **"Reliability thresholds:"** (CAUTION label)
   ≥80% is reliable (green). 65–79% is moderate (caution). Below 65% means historical
   throughput is not a stable predictor — Monte Carlo results should be treated with low
   confidence.
   Append cross-mode sentence ONLY when bothModes === true:
   `"A low duration score ({N}%) combined with a higher scope score ({N}%) typically
   indicates that throughput volume is more predictable than individual item cycle times."`

3. **"Important:"**
   The walk-forward analysis stops automatically if a process drift (system shift via
   3-way control chart) is detected — drift-contaminated checkpoints are excluded from
   accuracy scoring.

---

## Checklist Before Delivering

- [ ] Triggered by forecast_backtest data
- [ ] Chart title reads exactly "Walk-Forward Backtest"
- [ ] SCOPE set to structured object or null — never omitted
- [ ] DURATION set to structured object or null — never omitted
- [ ] Checkpoints flattened: actual / p50 / p85 / p95 / hit / drift
- [ ] chartData = [...checkpoints].reverse() — chronological left→right
- [ ] Mode auto-detected: useState defaults to "duration" if available, else "scope"
- [ ] Toggle rendered ONLY when bothModes === true
- [ ] Header mode subtitle reflects active mode
- [ ] Stat cards: DRIFT SIGNALS card color = ALARM if >0, POSITIVE if 0
- [ ] AccuracyStrip: threshold markers at 65% and 80%
- [ ] validation_msg rendered below strip in MUTED italic
- [ ] Drift badge rendered ONLY if driftPts.length > 0
- [ ] ResponsiveContainer width="100%" height={440}
- [ ] ComposedChart margin bottom=50, XAxis height=50 — no axis labels
- [ ] Panel padding "16px 20px 0 20px" — zero bottom padding
- [ ] Bar uses Cell per item: POSITIVE if hit, ALARM if miss (Rule: single Bar, no stackId)
- [ ] Cell keys use template literal `cell-${i}`
- [ ] Lines: p50 SECONDARY / p85 CAUTION / p95 PRIMARY — all dot={false}
- [ ] yMax = max of all actual + p95 values × 1.15
- [ ] Miss detail table only rendered if misses.length > 0
- [ ] Direction column: favorable = POSITIVE, unfavorable = ALARM
- [ ] Footer cross-mode sentence only when bothModes === true
- [ ] All data embedded as JS literals — no fetch calls
- [ ] Dark theme throughout: PAGE_BG page, PANEL_BG panels, BORDER grid
- [ ] Monospace font throughout
- [ ] Single self-contained .jsx file with default export
