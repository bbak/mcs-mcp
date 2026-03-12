---
name: cumulative-flow-diagram
description: >
  Creates a dark-themed React/Recharts stacked area Cumulative Flow Diagram (CFD) for
  the CFD Data tool (mcs-mcp:generate_cfd_data). Trigger on: "CFD", "cumulative flow
  diagram", "flow diagram", "WIP by status over time", "show the CFD", or any
  "show/chart/plot/visualize that" follow-up after a generate_cfd_data result is present.
  Use ONLY when generate_cfd_data data is present in the conversation AND a confirmed
  workflow mapping (workflow_discover_mapping) exists. Always read this skill before
  building the chart — do not attempt it ad-hoc.
---

# Cumulative Flow Diagram

Scope: Use this skill ONLY when generate_cfd_data data is present in the conversation.
It visualises the daily or weekly per-status item population as a stacked area chart,
stratified by issue type, with interactive issue type and status toggles.

Do not use this skill for:
- Cycle time / Lead time stability       → analyze_process_stability
- Throughput stability                   → analyze_throughput
- WIP Count stability                    → analyze_wip_stability
- Total WIP Age stability                → analyze_wip_age_stability
- Individual item WIP Age outliers       → analyze_work_item_age

---

## Prerequisites

Both tool responses must be present before building the chart:

```js
mcs-mcp:workflow_discover_mapping({ board_id, project_key })
// Confirm status_order and mapping with user before proceeding

mcs-mcp:generate_cfd_data({
  board_id, project_key,
  granularity: "weekly",        // "daily" (default) or "weekly"
  history_window_weeks: 26,     // default 26
})
```

API options note:
- `granularity`: `"weekly"` produces ~26 points for a 26-week window (no downsampling needed).
  `"daily"` produces ~182 points — downsample to every 2nd–3rd point, retain first + last.
- `history_window_weeks`: adjust as needed; longer windows need heavier downsampling when daily.

CRITICAL: The `status_order` from `workflow_discover_mapping` is the SOLE authoritative source
for stack order. Never derive order from the `generate_cfd_data` statuses array (which is
unordered). If the chart order looks wrong, re-run `workflow_discover_mapping` with
`force_refresh: true`.

---

## Response Structure

```
workflow_discover_mapping response:
  data.workflow.status_order[]          — ordered status ID strings (authoritative)
  data.workflow.status_mapping          — { id: { name, tier, role } }

generate_cfd_data response:
  data.cfd_data.buckets[]
    .label                             — date string "YYYY-MM-DD"
    .by_issue_type                     — { issueType: { statusName: count } }
  data.cfd_data.statuses[]             — unordered list of status names seen in data
  data.cfd_data.availableIssueTypes[]  — list of issue type names seen in data
```

---

## Critical: Status Names and Issue Types Are NOT Hardcoded

NEVER hardcode status names, status order, or issue type names. Derive all of them
entirely from the tool responses:

```js
// STATUS_ORDER_NAMES — from workflow_discover_mapping
const STATUS_ORDER_NAMES = status_order.map(id => status_mapping[id].name);
// statusesForLegend = STATUS_ORDER_NAMES
// statusesForChart  = [...STATUS_ORDER_NAMES].reverse()   ← Recharts stacks bottom-to-top

// ALL_ISSUE_TYPES — from generate_cfd_data
const ALL_ISSUE_TYPES = data.cfd_data.availableIssueTypes;

// ISSUE_TYPE_COLORS — built dynamically, never hardcoded per name
const ISSUE_TYPE_PALETTE = [
  "#6b7de8","#ff6b6b","#7edde2","#e2c97e",
  "#6bffb8","#f97316","#8b5cf6","#ec4899",
];
const ISSUE_TYPE_COLORS = Object.fromEntries(
  ALL_ISSUE_TYPES.map((t, i) => [t, ISSUE_TYPE_PALETTE[i % ISSUE_TYPE_PALETTE.length]])
);
```

The reversal of `statusesForChart` is required because Recharts renders the first Area at
the bottom of the stack. To display bands in workflow order (earliest at bottom), the
array must be reversed before rendering.

---

## Data Preparation

### Injection point: RAW_STRATIFIED

Embed the raw buckets preserving per-issue-type counts, before aggregation.
Downsample if needed (weekly data at 26 points needs no downsampling).

```js
// shape: { date: string, byType: { issueType: { statusName: count } } }[]
const RAW_STRATIFIED = buckets.map(b => ({
  date:   b.label,
  byType: b.by_issue_type,
}));
// If daily and > ~60 points: keep every 2nd point, always retain first + last
```

### Aggregation — computed at render time via useMemo

Filter by `selectedIssueTypes`, sum counts per status per date, then apply Done
normalization:

```js
function buildChartData(raw, selectedTypes) {
  const aggregated = raw.map(({ date, byType }) => {
    const entry = { date };
    selectedTypes.forEach(type => {
      const counts = byType[type] || {};
      Object.entries(counts).forEach(([status, count]) => {
        entry[status] = (entry[status] || 0) + count;
      });
    });
    return entry;
  });
  // Normalize Done to Day 1 of window
  const baseline = aggregated[0]?.["Done"] || 0;
  return aggregated.map(d => ({ ...d, Done: (d.Done || 0) - baseline }));
}
```

### DELIVERED stat card — computed from RAW_STRATIFIED, NOT from chartData

```js
const finalDone = useMemo(() => {
  const first = RAW_STRATIFIED[0];
  const last  = RAW_STRATIFIED[RAW_STRATIFIED.length - 1];
  let delta = 0;
  for (const type of selectedTypes) {
    delta += (last.byType[type]?.["Done"] || 0) -
             (first.byType[type]?.["Done"] || 0);
  }
  return delta;
}, [selectedTypes]);
```

---

## Color Tokens

```js
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

Status band colors — assigned by position in `statusesForLegend`:

```js
const CFD_PALETTE = [
  "#ef4444","#f97316","#eab308","#22c55e","#06b6d4",
  "#0ea5e9","#3b82f6","#8b5cf6","#d946ef","#ec4899",
  "#10b981","#14b8a6",
];
const statusColors = Object.fromEntries(
  statusesForLegend.map((s, i) => [s, CFD_PALETTE[i % CFD_PALETTE.length]])
);
```

Issue type toggle colors — assigned dynamically (see above). Do NOT hardcode per type name.

---

## Injection Checklist

```
Placeholder          Source path
BOARD_ID             board_id parameter
PROJECT_KEY          project_key parameter
BOARD_NAME           board name from context / import_boards
STATUS_ORDER_NAMES   workflow_discover_mapping →
                       status_order[].map(id => status_mapping[id].name)
ALL_ISSUE_TYPES      generate_cfd_data → data.cfd_data.availableIssueTypes[]
RAW_STRATIFIED       generate_cfd_data → data.cfd_data.buckets[]
                       → { date: .label, byType: .by_issue_type }
                       Downsample if daily and >~60 points (keep every 2nd, retain first+last)
```

---

## Chart Architecture

Single ComposedChart with stacked Area series, one per status:

```
Height:       520px
Margin:       { top: 10, right: 20, left: 10, bottom: 60 }
X-axis:       dataKey="date", angle=-45, textAnchor="end", height=60
              interval = Math.floor(chartData.length / 10)
              tick formatter: toLocaleDateString "en-GB" { day:"2-digit", month:"short" }
Y-axis:       label "Items" angle=-90
stackId:      "1" on all Areas
Area order:   statusesForChart (reversed from statusesForLegend)
Visibility:   only render Areas for statuses in visibleStatuses Set
Area style:   fillOpacity={0.7}, strokeWidth={1.5}, dot={false}
Animation:    isAnimationActive={true}, animationDuration={600}
```

---

## Interactive Controls

### Issue Type Toggles — right-aligned above chart

Placed in a flex row with `flex:1` spacer pushing them right.
Buttons are built from `ALL_ISSUE_TYPES` dynamically — never hardcoded.

```jsx
<button onClick={() => toggleIssueType(type)} style={{
  fontSize: 10, padding: "5px 14px", borderRadius: 6, cursor: "pointer",
  background: active ? `${color}18` : "#1a1d2e",
  border: `1.5px solid ${active ? color : "#404660"}`,
  color: active ? color : "#505878",
  fontFamily: "'Courier New', monospace", fontWeight: 700,
  transition: "all 0.2s ease",
}}>{type}</button>
```

Guard: at least one issue type must remain active:

```js
if (next.has(type) && next.size === 1) return prev;
```

### Status Legend / Toggles — centered below chart

Toggle buttons built from `statusesForLegend` dynamically — never hardcoded:

```jsx
<button onClick={() => toggleStatus(s)} style={{
  padding: "4px 12px", background: "transparent",
  border: `2px solid ${active ? statusColors[s] : "#505878"}`,
  color: active ? statusColors[s] : "#505878",
  borderRadius: 6, cursor: "pointer", fontSize: 10,
  opacity: active ? 1 : 0.45,
  fontFamily: "'Courier New', monospace",
  transition: "all 0.2s ease",
}}>{s}</button>
```

Guard: at least one status must remain active.

---

## Tooltip

```js
const CustomTooltip = ({ active, payload, label, visibleStatuses }) => {
  // byName: map dataKey → value from payload
  // total: sum of visible statuses
  // rows: statusesForLegend.map — skip if !visibleStatuses.has(s) || !byName[s]
  // each row: color = statusColors[s], value = byName[s]
  // footer row: "Total" / {total}
};
```

Pass `visibleStatuses` as prop:

```jsx
<Tooltip content={props =>
  <CustomTooltip {...props} visibleStatuses={visibleStatuses} />} />
```

---

## Header

```
Breadcrumb: {PROJECT_KEY} · {BOARD_NAME} · Board {BOARD_ID}
Title:      exactly "Cumulative Flow Diagram"
Subtitle:   "{startDate} – {endDate} · {dayCount} days"
            (compute dayCount from first/last bucket dates)
```

Stat cards (4 cards):

```
WINDOW        "{dayCount} days"                                        MUTED
STATUSES      statusesForLegend.length                                 PRIMARY
ISSUE TYPES   "{selectedTypes.length} / {ALL_ISSUE_TYPES.length}"      SECONDARY  (reactive)
DELIVERED     "+{finalDone}"                                           POSITIVE   (reactive)
```

No info badges above the chart — date range is in the subtitle.

---

## Footer — Two Sections (both required)

1. **"Reading this chart:"**
   Each colored band shows items in that workflow status per period. Wider bands signal
   accumulation or bottlenecks. Use the issue type toggles (top right) to filter by type.
   Click the status buttons below the chart to focus on specific workflow stages.

2. **"Note on Done normalization:"**
   Done is shown relative to Day 1 of the window, highlighting delivery velocity rather
   than cumulative historical volume. When issue types are filtered, counts scale
   proportionally to the selected types only.

---

## Checklist Before Delivering

- [ ] Triggered by generate_cfd_data data with confirmed workflow mapping present
- [ ] STATUS_ORDER_NAMES injected from workflow_discover_mapping — none hardcoded
- [ ] ALL_ISSUE_TYPES injected from generate_cfd_data.availableIssueTypes — none hardcoded
- [ ] ISSUE_TYPE_COLORS built dynamically from ALL_ISSUE_TYPES — none hardcoded per name
- [ ] statusesForChart is the exact reverse of statusesForLegend
- [ ] statusColors assigned by position index into CFD_PALETTE — not by name
- [ ] buildChartData aggregates selected issue types and normalizes Done to Day 1
- [ ] finalDone computed from RAW_STRATIFIED directly, not from normalized chartData
- [ ] Issue type toggle buttons built from ALL_ISSUE_TYPES array (dynamic)
- [ ] Status toggle buttons built from statusesForLegend array (dynamic)
- [ ] Both toggle guards prevent reducing selection to zero
- [ ] ISSUE TYPES and DELIVERED stat cards update reactively on toggle
- [ ] Tooltip receives visibleStatuses as prop; only shows visible statuses
- [ ] Subtitle contains only date range and day count — no other text prefix
- [ ] No date-range badge above the chart
- [ ] Data downsampled if daily and >~60 points (retain first + last)
- [ ] CFD_PALETTE used for status band colors — not base semantic tokens
- [ ] Dark theme throughout: PAGE_BG page, PANEL_BG panels, BORDER grid
- [ ] Monospace font throughout
- [ ] Single self-contained .jsx file with default export
