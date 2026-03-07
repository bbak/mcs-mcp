---
name: cumulative-flow-diagram
description: >
  Creates a dark-themed React/Recharts stacked area Cumulative Flow Diagram (CFD)
  for the **CFD Data** tool (mcs-mcp:generate_cfd_data). Trigger on: "CFD", "cumulative
  flow diagram", "flow diagram", "WIP by status over time", "show the CFD", or any
  "show/chart/plot/visualize that" follow-up after a generate_cfd_data result is present.
  Use ONLY when generate_cfd_data data is present in the conversation AND a confirmed
  workflow mapping (workflow_discover_mapping) exists.
  Always read this skill before building the chart — do not attempt it ad-hoc.
---

# Cumulative Flow Diagram

> **Also read:** `mcs-charts-base` SKILL before implementing. It defines the page/panel
> wrappers, color tokens, typography, header structure, badge system, CartesianGrid,
> tooltip base style, area gradient pattern, legend pattern, interactive controls, footer
> structure, and universal checklist items. This skill only specifies what is unique to
> this chart.

> **Scope:** Use this skill ONLY when `generate_cfd_data` data is present in the
> conversation. It is exclusively for visualizing the daily per-status item population
> as a stacked area chart, stratified by issue type.

---

## Prerequisites

Call the tools if data is not yet in the conversation:

```js
mcs-mcp:workflow_discover_mapping({ board_id, project_key })
// Confirm status_order and mapping with user before proceeding

mcs-mcp:generate_cfd_data({ board_id, project_key })
```

**Critical:** The `status_order` from `workflow_discover_mapping` is the single authoritative
source for stack order. Never derive order from the `generate_cfd_data` statuses array.
If the chart order looks wrong, re-run `workflow_discover_mapping` with `force_refresh: true`.

Response structure from `generate_cfd_data`:

```
data.cfd_data.buckets[]
  .label          — date string (YYYY-MM-DD)
  .by_issue_type  — { issueType: { statusName: count } }
```

---

## Critical: Status Names Are Project-Specific

**NEVER hardcode status names.** Derive them entirely from `workflow_discover_mapping`:

- `status_order` — array of status IDs in confirmed workflow order (authoritative)
- `status_mapping` — `{ id: { name, tier, role } }` — maps IDs to display names

```js
// Build ordered status name arrays from mapping
const statusesForLegend = status_order.map(id => status_mapping[id].name);
const statusesForChart  = [...statusesForLegend].reverse(); // Recharts stacks bottom-to-top
```

The reversal is required because Recharts renders the first `<Area>` at the bottom of the
stack. To display status bands in workflow order (earliest at bottom), the array must be
reversed before rendering.

---

## Data Preparation

### Stratified Raw Data Structure

Embed the raw data preserving per-issue-type counts, **before** aggregation:

```js
// RAW_STRATIFIED: { date, byType: { issueType: { statusName: count } } }[]
// Downsample: keep every 2nd bucket; always retain first and last.
```

### Aggregation (computed at render time via useMemo)

Filter by `selectedIssueTypes`, sum counts across types per status per day,
then apply **Done normalization**:

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

### DELIVERED stat card — compute from RAW_STRATIFIED, not chartData

The Done normalization means `chartData[last].Done` does not equal the raw delta.
Compute `finalDone` directly from the raw source, **outside** the normalization:

```js
const finalDone = useMemo(() => {
  const first = RAW_STRATIFIED[0];
  const last  = RAW_STRATIFIED[RAW_STRATIFIED.length - 1];
  let delta = 0;
  for (const type of selectedIssueTypes) {
    delta += (last.byType[type]?.["Done"] || 0) - (first.byType[type]?.["Done"] || 0);
  }
  return delta;
}, [selectedIssueTypes]);
```

---

## Color Palette

The CFD uses its own vibrant sequential palette — **not** the base semantic tokens.
Assign colors by position in `statusesForLegend`:

```js
const CFD_PALETTE = [
  "#ef4444", "#f97316", "#eab308", "#22c55e", "#06b6d4",
  "#0ea5e9", "#3b82f6", "#8b5cf6", "#d946ef", "#ec4899",
  "#10b981", "#14b8a6",
];
const statusColors = Object.fromEntries(
  statusesForLegend.map((s, i) => [s, CFD_PALETTE[i % CFD_PALETTE.length]])
);
```

Issue type toggle buttons use semantic colors distinct from the status palette:

```js
const ISSUE_TYPE_COLORS = {
  "Story":    "#6b7de8",   // PRIMARY
  "Bug":      "#ff6b6b",   // ALARM
  "Activity": "#7edde2",   // SECONDARY
  "Defect":   "#e2c97e",   // CAUTION
};
```

---

## Chart Architecture

Single `ComposedChart` with stacked `<Area>` series, one per status.

| Property | Value |
|---|---|
| Height | 520px |
| X-axis | `dataKey="date"`, angled -45°, `textAnchor="end"`, interval = `Math.floor(n / 10)` |
| Y-axis | Label: `"Items"` (angle -90) |
| Stack | All areas share `stackId="1"` |
| Area order | `statusesForChart` (reversed from legend) |
| Visibility | Render only statuses present in `visibleStatuses` Set |
| Area style | `fillOpacity={0.7}`, `strokeWidth={1.5}`, `dot={false}` |
| Animation | `isAnimationActive={true}`, `animationDuration={600}` |

---

## Interactive Controls

### Issue Type Toggles (right-aligned, above chart)

Placed in a flex row with a `flex: 1` spacer pushing them right.
Follow the base skill interactive button style:

```jsx
<button onClick={() => toggleIssueType(type)} style={{
  fontSize: 10,
  padding: "5px 14px",
  borderRadius: 6,
  cursor: "pointer",
  background: active ? `${color}18` : "#1a1d2e",
  border: `1.5px solid ${active ? color : "#404660"}`,
  color: active ? color : "#505878",
  fontFamily: "'Courier New', monospace",
  fontWeight: 700,
  transition: "all 0.2s ease",
}}>
  {type}
</button>
```

**Guard:** At least one issue type must remain active at all times:
```js
if (next.has(type) && next.size === 1) return;
```

### Status Legend / Toggles (inside chart panel, centered below chart)

Each status is a toggle button (not a static legend item):

```jsx
<button onClick={() => toggleStatus(status)} style={{
  padding: "4px 12px",
  background: "transparent",
  border: `2px solid ${active ? statusColors[status] : "#505878"}`,
  color: active ? statusColors[status] : "#505878",
  borderRadius: 6,
  cursor: "pointer",
  fontSize: 10,
  opacity: active ? 1 : 0.45,
  fontFamily: "'Courier New', monospace",
  transition: "all 0.2s ease",
}}>
  {status}
</button>
```

---

## Header

Per `mcs-charts-base` header structure, with these specifics:

- **Breadcrumb:** `{PROJECT_KEY} · {project name} · Board {board_id}`
- **Title:** exactly `"Cumulative Flow Diagram"`
- **Subtitle:** `"{start date} – {end date} · {N} days"` (no other text prefix)
- **Stat cards:**

| Label | Value | Color |
|---|---|---|
| `WINDOW` | e.g. `"183 days"` | MUTED `#505878` |
| `STATUSES` | count of statuses | PRIMARY `#6b7de8` |
| `ISSUE TYPES` | `"{selected} / {total}"` — updates on toggle | SECONDARY `#7edde2` |
| `DELIVERED` | `"+{finalDone}"` — updates on toggle | POSITIVE `#6bffb8` |

No info badges above the chart (the date range is already in the subtitle).

---

## Tooltip

Per `mcs-charts-base` tooltip base style. Only show statuses present in `visibleStatuses`.

```jsx
const CustomTooltip = ({ active, payload, label, visibleStatuses }) => {
  // ...
  return (
    <div style={{ /* base tooltip style */ }}>
      <div style={{ color: SECONDARY }}>{formatDate(label)}</div>
      {statusesForLegend.map(s => {
        if (!visibleStatuses.has(s) || !byName[s]) return null;
        return (
          <div key={s} style={{ display: "flex", justifyContent: "space-between", gap: 16 }}>
            <span style={{ color: statusColors[s] }}>{s}</span>
            <span>{byName[s]}</span>
          </div>
        );
      })}
      <div style={{ borderTop: "1px solid #1a1d2e", /* Total row */ }}>
        <span style={{ color: MUTED }}>Total</span>
        <span style={{ fontWeight: 700 }}>{total}</span>
      </div>
    </div>
  );
};
```

Pass `visibleStatuses` to the tooltip as a prop via the `content` render prop:
```jsx
<Tooltip content={(props) => <CustomTooltip {...props} visibleStatuses={visibleStatuses} />} />
```

---

## Footer

Two required sections:

1. **"Reading this chart:"** — Each colored band shows items in that workflow status per
   day. Wider bands signal accumulation. The *Done* band shows delivery since Day 1
   (normalized). Use the issue type toggles (top right) to filter by issue type. Click
   status legend buttons below the chart to focus on specific workflow stages.

2. **"Note on Done normalization:"** — Done is shown relative to Day 1 of the window,
   highlighting delivery velocity rather than cumulative historical volume. When issue
   types are filtered, counts scale proportionally to the selected types only.

---

## Chart-Specific Checklist

*(Universal items are in `mcs-charts-base`. Only chart-specific items listed here.)*

- [ ] Skill triggered by `generate_cfd_data` data with confirmed workflow mapping
- [ ] Status names derived from `workflow_discover_mapping` — none hardcoded
- [ ] `statusesForChart` is the exact reverse of `statusesForLegend`
- [ ] `buildChartData` aggregates selected issue types and normalizes Done to Day 1
- [ ] `finalDone` computed from `RAW_STRATIFIED` directly, not from normalized `chartData`
- [ ] Issue type toggles right-aligned above chart, using `ISSUE_TYPE_COLORS`
- [ ] At least one issue type always remains active (toggle guard in place)
- [ ] Status legend buttons centered below chart, double as visibility toggles
- [ ] `ISSUE TYPES` stat card reflects selected count and updates on toggle
- [ ] `DELIVERED` stat card reflects selected issue types and updates on toggle
- [ ] Subtitle contains only date range and day count — no other text prefix
- [ ] No date-range badge above the chart (subtitle is sufficient)
- [ ] Tooltip receives `visibleStatuses` as prop; only shows visible statuses
- [ ] Data downsampled to every 2nd bucket (retain first and last)
- [ ] CFD_PALETTE used for status colors — not base semantic tokens
- [ ] Cached `status_order` may be stale; if chart order looks wrong, re-run `workflow_discover_mapping` with `force_refresh: true`
