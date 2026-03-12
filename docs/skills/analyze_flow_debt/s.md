---
name: flow-debt-chart
description: >
  Creates a dark-themed two-panel React/Recharts chart for the Flow Debt analysis
  (mcs-mcp:analyze_flow_debt). Trigger on: "flow debt chart", "arrivals vs departures",
  "commitment vs delivery", "WIP growth chart", "flow clog", "cycle time inflation risk",
  or any "show/chart/plot/visualize that" follow-up after an analyze_flow_debt result is
  present. ONLY for analyze_flow_debt output. Do NOT use for: WIP count stability
  (analyze_wip_stability), throughput volume (analyze_throughput), Total WIP Age
  (analyze_wip_age_stability), or residence time (analyze_residence_time).
  Always read this skill before building the chart — do not attempt it ad-hoc.
---

# Flow Debt Chart

Scope: Use this skill ONLY when analyze_flow_debt data is present in the conversation.
It visualises the weekly balance between arrivals (commitments) and departures (deliveries),
plus the running cumulative debt — a leading indicator of future cycle time inflation.

Do not use this skill for:
- WIP count over time          → analyze_wip_stability
- Delivery volume / cadence    → analyze_throughput
- Total WIP Age burden         → analyze_wip_age_stability
- Residence time / Little's Law → analyze_residence_time

---

## Prerequisites

```js
mcs-mcp:analyze_flow_debt({ board_id, project_key })
```

API options that alter response shape:

```
bucket_size           "week" (default) | "month"
                      Determines label format: "YYYY-Www" vs "YYYY-MM"
                      Detect automatically from label format — do not hardcode
history_window_weeks  default 26 — affects number of buckets returned
```

No workflow_discover_mapping prerequisite — this tool is self-contained.

---

## Response Structure

```
data.flow_debt
  .totalDebt          — integer: net cumulative debt over the full window
                        (sum of all bucket .debt values)
  .buckets[]          — one entry per time bucket:
    .label            — bucket identifier, e.g. "2025-W37" or "2025-10"
    .arrivals         — items committed (entered Downstream) this bucket
    .departures       — items delivered (reached Finished) this bucket
    .debt             — arrivals − departures for this bucket (can be negative)

guardrails.insights[] — string array; surface in footer
```

---

## Injection Checklist

```
Placeholder     Source path
BOARD_ID        board_id parameter
PROJECT_KEY     project_key parameter
BOARD_NAME      board name from context / import_boards
TOTAL_DEBT      data.flow_debt.totalDebt
BUCKETS         data.flow_debt.buckets[]  (full array, all fields)
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

// Semantic debt coloring (applied per data point — never hardcoded):
// debt > 0  → ALARM    (over-committed, WIP growing)
// debt < 0  → POSITIVE (catch-up, WIP shrinking)
// debt === 0 → MUTED
```

---

## X-Axis Label Shortening

Long bucket labels must be shortened for display — detect format from label content:

```js
const MONTH_NAMES = ["Jan","Feb","Mar","Apr","May","Jun",
                     "Jul","Aug","Sep","Oct","Nov","Dec"];

function shortLabel(l) {
  if (/W\d+/.test(l)) return l.replace(/^\d{4}-/, "");   // "2025-W37" → "W37"
  const parts = l.split("-");
  if (parts.length === 2)
    return MONTH_NAMES[parseInt(parts[1], 10) - 1] || l; // "2025-10" → "Oct"
  return l;
}
```

Apply `shortLabel` in `useMemo` when enriching BUCKETS with `short` and `cumDebt`.

---

## Computed Fields (add in useMemo)

```js
const enriched = useMemo(() => {
  let running = 0;
  return BUCKETS.map(b => {
    running += b.debt;
    return { ...b, short: shortLabel(b.label), cumDebt: running };
  });
}, []);

const avgArrivals   = round1(BUCKETS.reduce((s,b) => s + b.arrivals,   0) / BUCKETS.length);
const avgDepartures = round1(BUCKETS.reduce((s,b) => s + b.departures, 0) / BUCKETS.length);
const weeksPositive = BUCKETS.filter(b => b.debt > 0).length;
const weeksNegative = BUCKETS.filter(b => b.debt < 0).length;
```

---

## Chart Architecture — Two Panels

Render order: Stat Cards → Guardrail Badges → Panel 1 → Panel 2 → Footer.

### Panel 1: Arrivals vs. Departures + Debt Line

Purpose: show the weekly balance between commitments and deliveries.

```
ComposedChart, height=300px
Margin: { top: 8, right: 24, left: 8, bottom: 4 }
CartesianGrid strokeDasharray="3 3"
```

Axes:

```
XAxis  dataKey="short", tickFormatter=xTick (show every ~4th label)
YAxis  yAxisId="vol"   left,  domain [0, max(arrivals,departures) * 1.15]
                        label: "items"
YAxis  yAxisId="debt"  right, domain [-debtExtent*1.2, debtExtent*1.2]
                        label: "debt"
                        debtExtent = max(...BUCKETS.map(b => Math.abs(b.debt)))
ReferenceLine yAxisId="debt" y=0, stroke=MUTED, strokeDasharray="4 4"
```

Series:

```jsx
// Grouped bars — side by side (no stackId)
<Bar yAxisId="vol" dataKey="arrivals"   barSize={10} fill={PRIMARY}   fillOpacity={0.75} radius={[3,3,0,0]} />
<Bar yAxisId="vol" dataKey="departures" barSize={10} fill={SECONDARY} fillOpacity={0.75} radius={[3,3,0,0]} />

// Debt line with per-dot coloring
<Line yAxisId="debt" dataKey="debt" type="monotone"
  stroke={CAUTION} strokeWidth={2}
  dot={(props) => {
    const { cx, cy, payload } = props;
    const color = payload.debt > 0 ? ALARM : payload.debt < 0 ? POSITIVE : MUTED;
    return <circle key={cx} cx={cx} cy={cy} r={3} fill={color} stroke="none" />;
  }} />
```

MainTooltip — grid layout:

```
Arrivals     PRIMARY
Departures   SECONDARY
Debt         ALARM / POSITIVE / MUTED  (signed: "+5" or "-3")
```

Legend (centered flex row below chart):

```
Blue filled rect    → "Arrivals (commitments)"
Cyan filled rect    → "Departures (deliveries)"
Caution line + dot  → "Weekly debt (dot: red=positive, green=negative)"
```

Panel subtitle: `"Arrivals vs. departures · weekly debt (line, right axis)"`

### Panel 2: Cumulative Flow Debt — area chart

Purpose: reveal systemic drift — is the system accumulating debt over time?

```
ComposedChart (or AreaChart), height=220px
Margin: { top: 8, right: 24, left: 8, bottom: 4 }
XAxis  dataKey="short", tickFormatter=xTick
YAxis  domain [-cumExtent*1.2, cumExtent*1.2]
        label: "cumulative debt"
        cumExtent = max(...enriched.map(b => Math.abs(b.cumDebt)))
ReferenceLine y=0, stroke=MUTED_DK, strokeDasharray="4 4"
```

Series:

```jsx
<defs>
  <linearGradient id="cumGrad" x1="0" y1="0" x2="0" y2="1">
    <stop offset="5%"  stopColor={ALARM} stopOpacity={0.3} />
    <stop offset="95%" stopColor={ALARM} stopOpacity={0.02} />
  </linearGradient>
</defs>
<Area dataKey="cumDebt" type="monotone"
  stroke={ALARM} strokeWidth={2} fill="url(#cumGrad)" />
```

CumTooltip — grid layout:

```
Week debt    ALARM / POSITIVE / MUTED  (signed)
Cumulative   ALARM / POSITIVE / MUTED  (signed, bold)
```

Legend:

```
ALARM line  → "Cumulative debt (above zero = net over-commitment)"
```

Panel subtitle: `"Cumulative flow debt · systemic drift over time"`

---

## X-Tick Density

With 27+ weekly buckets the X-axis becomes unreadable if every label is shown.
Thin the labels to approximately 8 visible ticks:

```js
const step = Math.ceil(enriched.length / 8);
const xTick = (value, index) => index % step === 0 ? value : "";
// Pass as tickFormatter to both XAxis elements
```

---

## Stat Cards (5 cards)

```js
// TOTAL_DEBT card color: ALARM if positive, POSITIVE if negative, MUTED if zero
const debtColor = TOTAL_DEBT > 0 ? ALARM : TOTAL_DEBT < 0 ? POSITIVE : MUTED;
```

```
TOTAL DEBT       TOTAL_DEBT (signed "+27")   debtColor   sub="over N buckets"
AVG ARRIVALS     avgArrivals                 PRIMARY     sub="per bucket"
AVG DEPARTURES   avgDepartures               SECONDARY   sub="per bucket"
WEEKS POSITIVE   weeksPositive               ALARM       sub="arrivals > departures"
WEEKS NEGATIVE   weeksNegative               POSITIVE    sub="departures > arrivals"
```

---

## Guardrail Badges (3 badges, flex row)

```
"Positive debt = WIP growing → future cycle time inflation"   ALARM
"Zero / negative debt = stable or improving throughput ratio"  POSITIVE
"Leading indicator — signals problems before delays appear"    CAUTION
```

---

## Header

```
Breadcrumb: {PROJECT_KEY} · {BOARD_NAME} · Board {BOARD_ID}
Title:      exactly "Flow Debt"
Subtitle:   "Arrivals vs. departures · leading indicator of cycle time inflation"
```

---

## Footer — Two Sections (both required)

1. **"Reading this chart:"**
   Each bucket shows arrivals (commitments made) vs. departures (items delivered).
   The debt line shows the weekly gap — positive when more work enters than leaves.
   The cumulative panel shows systemic drift: a rising line means the system is
   consistently taking on more than it delivers, which mathematically guarantees
   future cycle time inflation via Little's Law.

2. **"Important:"**
   Flow debt is a leading indicator — it signals accumulating pressure before delivery
   delays become visible. A total debt of {TOTAL_DEBT} items means the system has
   committed to {TOTAL_DEBT} more items than it has delivered over this window. Negative
   weekly debt is healthy (catch-up), but sustained positive debt requires intervention.
   Reference the actual TOTAL_DEBT value — never write a generic placeholder.

---

## Checklist Before Delivering

- [ ] Triggered by analyze_flow_debt data
- [ ] Chart title reads exactly "Flow Debt"
- [ ] BUCKETS injected in full from data.flow_debt.buckets[]
- [ ] TOTAL_DEBT from data.flow_debt.totalDebt
- [ ] shortLabel() detects week vs. month format dynamically — not hardcoded
- [ ] enriched computed in useMemo: adds short + running cumDebt
- [ ] X-tick thinning applied (≈8 visible labels regardless of bucket count)
- [ ] Panel 1: grouped bars (PRIMARY=arrivals, SECONDARY=departures) + debt Line
- [ ] Panel 1: debt line uses per-dot coloring (ALARM / POSITIVE / MUTED)
- [ ] Panel 1: dual Y-axis (vol left, debt right), zero ReferenceLine on debt axis
- [ ] Panel 2: cumulative area with ALARM gradient fill
- [ ] Panel 2: zero ReferenceLine, symmetric Y domain
- [ ] Debt coloring is always computed — never hardcoded
- [ ] TOTAL DEBT stat card color computed from sign of TOTAL_DEBT
- [ ] Footer references actual TOTAL_DEBT value
- [ ] Guardrail badges present and correctly worded
- [ ] Dark theme throughout: PAGE_BG page, PANEL_BG panels, BORDER grid
- [ ] Monospace font throughout
- [ ] Single self-contained .jsx file with default export
