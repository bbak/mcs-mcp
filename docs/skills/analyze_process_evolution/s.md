---
name: process-evolution-chart
description: >
  Creates a dark-themed React/Recharts scatterplot for the Strategic Process Evolution
  analysis (mcs-mcp:analyze_process_evolution). Trigger on: "process evolution chart",
  "strategic evolution chart/visualization", "Three-Way Control Chart", "monthly cycle
  time scatter", "evolution scatterplot", or any "show/chart/plot/visualize that"
  follow-up after an analyze_process_evolution result is present.
  ONLY for analyze_process_evolution output (Three-Way Control Chart with monthly
  subgroup averages and individual item cycle times). Do NOT use for: cycle time
  stability (analyze_process_stability), throughput (analyze_throughput), WIP count
  stability (analyze_wip_stability), Total WIP Age stability (analyze_wip_age_stability),
  individual item WIP Age (analyze_work_item_age), or residence time
  (analyze_residence_time). Always read this skill before building the chart — do not
  attempt it ad-hoc.
---

# Process Evolution Chart

Scope: Use this skill ONLY when analyze_process_evolution data is present in the
conversation. It renders individual item cycle times as a scatterplot with monthly
subgroup averages as disconnected horizontal bars and XmR control limits.

Do not use this skill for:
- Cycle time / Lead time stability        → analyze_process_stability
- Throughput stability                    → analyze_throughput
- WIP Count stability                     → analyze_wip_stability
- Total WIP Age stability                 → analyze_wip_age_stability
- Individual item WIP Age outliers        → analyze_work_item_age
- Residence time / Little's Law           → analyze_residence_time

---

## Prerequisites

```js
mcs-mcp:analyze_process_evolution({
  board_id, project_key,
  history_window_months: 60,   // default 12, up to 60 for deep history
})
```

IMPORTANT: The tool does NOT provide issue type metadata for individual items.
The values arrays contain raw cycle times without type information. Do not claim
the chart shows type-level breakdown.

---

## Response Structure

```
data.evolution.subgroups[]           — monthly subgroups:
  .label                             — "Mar 2023" etc.
  .average                           — subgroup mean cycle time (days)
  .values[]                          — individual item cycle times (days)

data.evolution.average_chart         — XmR on subgroup averages:
  .average                           — process average X̄
  .upper_natural_process_limit       — UNPL
  .lower_natural_process_limit       — LNPL (usually 0)
  .signals[]                         — detected signals:
    .type                            — "shift" (8-point run)
    .index                           — subgroup index where run ends
    .description                     — human-readable description

data.evolution.status                — "stable" / "unstable" / "migrating"
                                       INTERNAL ONLY — never surface as a badge

data.context
  .subgroup_count
  .total_issues
  .window_months
```

---

## Data Preparation

Constants from tool response:

```js
const MEAN = average_chart.average;
const UNPL = average_chart.upper_natural_process_limit;

// Subgroups: keep label + values only (average is recomputed in chart)
const SUBGROUPS = [
  { label: "Mar 2023", values: [...] },
  // ...
];
```

Label shortening for X-axis:

```js
function shortLabel(l) {
  const [mon, yr] = l.split(" ");
  return `${mon} ${yr.slice(2)}`;   // "Mar 2023" → "Mar 23"
}
```

Shift range from signals:

```js
// Signal index = last subgroup in the 8-point run.
const SHIFT_START = signal.index - 7;
const SHIFT_END   = signal.index;
```

Building scatter data:

```js
// Dots — one per item per subgroup
vals.forEach((v, i) => {
  const jitter = vals.length === 1 ? 0
    : -0.32 + (i / (vals.length - 1)) * 0.64;
  dots.push({
    x: idx + jitter,
    y: Math.max(v, 0.01),     // log-safe floor
    label: sg.label,
    isShift: idx >= SHIFT_START && idx <= SHIFT_END,
    isAboveUnpl: v > UNPL,
  });
});

// Average bars — left + right edge per subgroup
const pairId = `avg-${idx}`;
avgBars.push({ x: idx - 0.38, y: avg, edge: "left",  pairId, isShift, isAboveUnpl: avg > UNPL });
avgBars.push({ x: idx + 0.38, y: avg, edge: "right", pairId, isShift, isAboveUnpl: avg > UNPL });
```

---

## Color Tokens

```js
const ALARM     = "#ff6b6b";
const CAUTION   = "#e2c97e";
const PRIMARY   = "#6b7de8";
const SECONDARY = "#7edde2";
const TEXT      = "#dde1ef";
const MUTED     = "#505878";
const MUTED_DK  = "#4a5270";
const PAGE_BG   = "#080a0f";
const PANEL_BG  = "#0c0e16";
const BORDER    = "#1a1d2e";
```

Series color assignment:

```
Normal dots:          PRIMARY   opacity 0.35   r=2.5
Shift zone dots:      CAUTION   opacity 0.35   r=2.5
Above-UNPL dots:      ALARM     opacity 0.75   r=3.5
Avg bar (normal):     TEXT      strokeWidth 2.5
Avg bar (shift):      CAUTION   strokeWidth 2.5
Avg bar (above UNPL): ALARM     strokeWidth 2.5
X̄ reference line:    PRIMARY   strokeDasharray "4 4"
UNPL reference line:  ALARM     strokeDasharray "6 3"
Shift zone fill:      CAUTION   fillOpacity 0.04
```

---

## Chart Architecture

ScatterChart from Recharts with two Scatter series and no Recharts Legend.

### Custom Shapes

DotShape — renders per-item scatter dots:

```jsx
const DotShape = useCallback(({ cx, cy, payload }) => {
  if (!payload || payload.edge) return null;
  const color   = payload.isAboveUnpl ? ALARM : payload.isShift ? CAUTION : PRIMARY;
  const opacity = payload.isAboveUnpl ? 0.75 : 0.35;
  const r       = payload.isAboveUnpl ? 3.5 : 2.5;
  return <circle cx={cx} cy={cy} r={r} fill={color} fillOpacity={opacity} />;
}, []);
```

AvgBarShape — renders disconnected horizontal average bars:

```jsx
// Wrap pairPositions in useMemo(() => ({}), []) for stability
const pairPositions = useMemo(() => ({}), []);

const AvgBarShape = useCallback(({ cx, cy, payload }) => {
  if (!payload) return null;
  if (payload.edge === "left") {
    pairPositions[payload.pairId] = { x: cx, y: cy };
    return null;
  }
  const left = pairPositions[payload.pairId];
  if (!left) return null;
  const color = payload.isAboveUnpl ? ALARM : payload.isShift ? CAUTION : TEXT;
  return (
    <line x1={left.x} y1={left.y} x2={cx} y2={cy}
      stroke={color} strokeWidth={2.5} strokeLinecap="round" opacity={0.9} />
  );
}, [pairPositions]);
```

### Axes

```jsx
// XAxis
<XAxis
  type="number"
  domain={[-0.6, SUBGROUPS.length - 0.4]}
  ticks={SUBGROUPS.map((_, i) => i)}
  tickFormatter={i => shortLabel(SUBGROUPS[i]?.label || "")}
  angle={-45}
  textAnchor="end"
  height={60}
/>

// YAxis
<YAxis
  type="number"
  scale={showLog ? "log" : "linear"}
  domain={
    showLog
      ? [0.5, 1000]
      : [0, Math.ceil(Math.max(UNPL * 1.15, 300) / 50) * 50]
  }
  allowDataOverflow
  tickFormatter={v => `${v}d`}
/>
```

### Reference Elements

```jsx
<ReferenceArea
  x1={SHIFT_START - 0.5} x2={SHIFT_END + 0.5}
  fill={CAUTION} fillOpacity={0.04}
  stroke={CAUTION} strokeDasharray="4 4" strokeOpacity={0.3}
/>

<ReferenceLine y={UNPL} stroke={ALARM} strokeDasharray="6 3" strokeWidth={1.5}
  label={{ value: `UNPL ${UNPL.toFixed(1)}d`, fill: ALARM,
    fontSize: 10, position: "right", fontFamily: "'Courier New', monospace" }} />

<ReferenceLine y={MEAN} stroke={PRIMARY} strokeDasharray="4 4" strokeWidth={1.5}
  label={{ value: `X̄ ${MEAN.toFixed(1)}d`, fill: PRIMARY,
    fontSize: 10, position: "right", fontFamily: "'Courier New', monospace" }} />
```

### Scale Toggle

```jsx
// State
const [showLog, setShowLog] = useState(false);

// Styled as a clearly interactive button — NOT a pill badge:
// border-radius: 6px (rectangular), border: 1.5px solid, font-weight: 700
// Right-aligned via flex spacer

<div style={{ flex: 1 }} />
<button onClick={() => setShowLog(!showLog)} style={{
  fontSize: 10, padding: "5px 14px", borderRadius: 6, cursor: "pointer",
  background: showLog ? `${SECONDARY}18` : BORDER,
  border: `1.5px solid ${showLog ? SECONDARY : "#404660"}`,
  color: showLog ? SECONDARY : TEXT,
  fontFamily: "'Courier New', monospace", fontWeight: 700,
}}>
  {showLog ? "▾ Log Scale" : "▾ Linear Scale"}
</button>
```

### Chart Margins

```js
margin={{ top: 10, right: 20, bottom: 60, left: 10 }}
```

Note: `right: 20` — reference line labels use `position: "right"` and sit outside
the plot area; Recharts clips them unless the chart is given room. If labels are
clipped, increase the right margin.

### Tooltip

```
DotTooltip — minimal: month label + cycle time value.
Set cursor={false} on ScatterChart to suppress the default crosshair.
```

---

## Stat Cards (4 cards)

```
PROCESS AVG (X̄)   MEAN.toFixed(1) + "d"   PRIMARY
UNPL               UNPL.toFixed(1) + "d"   ALARM
ITEMS              totalItems               SECONDARY
MONTHS             SUBGROUPS.length         MUTED
```

---

## Badge Row

Two informational badges + scale toggle right-aligned:

```
1. "⚠ Process Shift: 8-point run ({shiftStart} – {shiftEnd})"   CAUTION
   (only if signals contains a shift entry)

2. "{N}-month window · {M} subgroups · {I} items"               PRIMARY
```

Scale toggle is right-aligned via `flex: 1` spacer — see Scale Toggle section above.

Do NOT include a `"Status: migrating/stable/unstable"` badge. That is a
tool-internal classification, not a user-facing label.

---

## Legend

Manual legend below the chart — no Recharts Legend component.
Three groups separated by dividers:

```
Group 1 — Dot types (SVG circle swatches):
  PRIMARY  opacity 0.7   "Normal"
  CAUTION  opacity 0.7   "Shift zone"
  ALARM    opacity 0.9   "Above UNPL"

Group 2 — Avg bar types (SVG line swatches, strokeWidth 2.5):
  TEXT     "Avg (normal)"
  CAUTION  "Avg (shift)"
  ALARM    "Avg (above UNPL)"

Group 3 — Reference lines (SVG dashed line swatches):
  PRIMARY  strokeDasharray "4 4"  "X̄ avg"
  ALARM    strokeDasharray "6 3"  "UNPL"
```

---

## Header

```
Breadcrumb: {PROJECT_KEY} · {BOARD_NAME} · Board {BOARD_ID}
Title:      exactly "Strategic Process Evolution"
Subtitle:   "Three-Way Control Chart · Cycle Time Distribution · {first label} – {last label}"
```

---

## Footer — Three Sections (all required)

1. **"What is a Three-Way Control Chart?"**
   Longitudinal audit tool for process behaviour over longer time horizons. Groups
   cycle times into monthly subgroups. XmR applied to subgroup averages. X̄ and UNPL
   define routine variation. Signals: UNPL breach or 8-point run on one side of X̄.

2. **"Reading this chart:"**
   Dots = individual items. Horizontal bars = monthly subgroup averages (disconnected
   by design). Amber shading = shift zone. Red dots = above UNPL. Scale toggle for
   linear vs log view.

3. **"Note on early subgroups:"**
   Months with only 1–2 items produce a technically correct average bar but one that
   is statistically meaningless. Focus interpretation on months with 10+ items.

---

## Injection Checklist

```
Placeholder       Source path
BOARD_ID          board_id parameter
PROJECT_KEY       project_key parameter
BOARD_NAME        board name from context / import_boards
MEAN              data.evolution.average_chart.average
UNPL              data.evolution.average_chart.upper_natural_process_limit
SUBGROUPS         data.evolution.subgroups[] → { label, values }
                  (use full precision values, not rounded — log scale needs them)
SHIFT_START       signal.index - 7  (from average_chart.signals where type="shift")
SHIFT_END         signal.index
totalItems        sum of values.length across all subgroups
                  (or data.context.total_issues)
```

If no shift signal is present: omit the shift badge, ReferenceArea, and shift coloring.

---

## Checklist Before Delivering

- [ ] Triggered by analyze_process_evolution data
- [ ] Chart title reads exactly "Strategic Process Evolution"
- [ ] Individual items rendered as scatter dots (not bars, not lines)
- [ ] Monthly averages rendered as disconnected horizontal bars (NOT a connected line)
- [ ] Average bars color-coded: TEXT (normal) / CAUTION (shift) / ALARM (above UNPL)
- [ ] Dots color-coded: PRIMARY (normal) / CAUTION (shift zone) / ALARM (above UNPL)
- [ ] XmR reference lines: X̄ PRIMARY dashed, UNPL ALARM dashed, both with position "right" labels
- [ ] Shift zone highlighted via ReferenceArea with CAUTION shading
- [ ] pairPositions wrapped in useMemo(() => ({}), [])
- [ ] AvgBarShape and DotShape wrapped in useCallback
- [ ] Scale toggle present, styled as interactive button (rectangular, bold border)
- [ ] Scale toggle right-aligned via flex spacer, visually distinct from badge pills
- [ ] No "Status: migrating/stable/unstable" badge — never surface tool-internal labels
- [ ] No "regime" or "migrating" language in explanatory text
- [ ] cursor={false} on ScatterChart (suppress default crosshair)
- [ ] allowDataOverflow on YAxis
- [ ] Footer has all three sections: What is it / How to read it / Note on early subgroups
- [ ] Tool does not provide issue type per item — do not claim type-level breakdown
- [ ] All data embedded as JS literals — no fetch calls
- [ ] Dark theme throughout: PAGE_BG page, PANEL_BG panel, BORDER grid
- [ ] Monospace font throughout
- [ ] Single self-contained .jsx file with default export
