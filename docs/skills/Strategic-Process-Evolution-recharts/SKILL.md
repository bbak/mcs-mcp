---
name: process-evolution-chart
description: >
  Creates a dark-themed React/Recharts scatterplot chart for the **Strategic Process Evolution**
  analysis (mcs-mcp:analyze_process_evolution). Trigger on: "process evolution chart",
  "strategic evolution chart/visualization", "Three-Way Control Chart", "monthly cycle time
  scatter", "evolution scatterplot", or any "show/chart/plot/visualize that" follow-up after
  an analyze_process_evolution result is present.
  Use ONLY when analyze_process_evolution data is present in the conversation.
  Always read this skill before building the chart — do not attempt it ad-hoc.
---

# Process Evolution Chart

> **Also read:** `mcs-charts-base` SKILL before implementing. It defines the page/panel
> wrappers, color tokens, typography, header structure, badge system, CartesianGrid,
> tooltip base style, area gradient pattern, legend pattern, interactive controls, footer
> structure, and universal checklist items. This skill only specifies what is unique to
> this chart.

> **Scope:** Use this skill ONLY when `analyze_process_evolution` data is present in the
> conversation. It is exclusively for charting the Three-Way Control Chart that groups
> historical cycle times into monthly subgroups and applies XmR analysis to subgroup averages.

---

## Prerequisites

Call the tool if data is not yet in the conversation:

```js
mcs-mcp:analyze_process_evolution({ board_id, project_key, history_window_months: 60 })
```

Response structure:
- `data.evolution.subgroups` — array of monthly subgroups, each with:
  - `label` (e.g. "Mar 2023"), `average` (subgroup mean), `values` (individual cycle times in days)
- `data.evolution.average_chart` — XmR on subgroup averages:
  - `average` (X̄), `upper_natural_process_limit` (UNPL), `lower_natural_process_limit` (LNPL)
  - `signals` — detected shift signals (type, description, index)
- `data.evolution.status` — "stable", "unstable", or "migrating" *(internal — do not surface)*
- `data.context` — `subgroup_count`, `total_issues`, `window_months`

**IMPORTANT:** The tool does NOT provide issue type metadata for individual items.
`values` arrays are raw cycle times only. Do not claim type-level breakdown.

---

## Data Preparation

| Constant | Source |
|---|---|
| `SUBGROUPS` | `evolution.subgroups` — keep `label` and `values` |
| `MEAN` | `average_chart.average` |
| `UNPL` | `average_chart.upper_natural_process_limit` |
| Shift indices | Parse `average_chart.signals` for `type: "shift"` entries |

**Label shortening:** `"Mar 2023"` → `"Mar 23"` for X-axis tick labels.

**Scatter points (individual items):** Each item becomes a point at `(monthIndex + jitter, cycleTime)`.
Jitter distributes items within ±0.32 of the month index:
```js
const jitter = values.length === 1 ? 0 :
  -0.32 + (i / (values.length - 1)) * 0.64;
```

**Average bar endpoints:** Two points per subgroup at column edges:
```js
{ x1: monthIdx - 0.38, x2: monthIdx + 0.38, y: avg }
```
Rendered as disconnected horizontal segments — NOT connected across months.

**Shift annotation:** Parse `average_chart.signals` to find shift runs. Mark start/end
subgroup indices. Set `isShift: true` on all scatter points and average bars within range.

---

## Chart Architecture

Uses `ScatterChart` (not ComposedChart) with a single Y-axis.

### Series

1. **Scatter — individual items**, custom shape function:
   - Normal: PRIMARY `#6b7de8`, opacity 0.3, r=2.5
   - In shift zone: CAUTION `#e2c97e`, opacity 0.3, r=2.5
   - Above UNPL: ALARM `#ff6b6b`, opacity 0.75, r=3.5

2. **Scatter — average bars**, invisible scatter with custom `AvgBarShape` rendering
   horizontal `<line>` segments:
   - Normal: TEXT `#dde1ef`, strokeWidth 2.5
   - Shift zone: CAUTION `#e2c97e`, strokeWidth 2.5
   - Above UNPL: ALARM `#ff6b6b`, strokeWidth 2.5

   **`AvgBarShape` implementation:** Use a mutable `pairPositions` object (wrapped in
   `useMemo(() => ({}), [])`) to track left-edge coordinates. When the right edge renders,
   draw the line between stored left and current right. Use `useCallback` for the shape fn:
   ```js
   const AvgBarShape = ({ cx, cy, payload }) => {
     if (payload.edge === "left") {
       pairPositions[payload.pairId] = { x1: cx, y: cy };
       return null;
     }
     const left = pairPositions[payload.pairId];
     if (!left) return null;
     const color = payload.isAboveUnpl ? "#ff6b6b" :
                   payload.isShift     ? "#e2c97e" : "#dde1ef";
     return <line x1={left.x1} y1={left.y} x2={cx} y2={cy}
       stroke={color} strokeWidth={2.5} strokeLinecap="round" opacity={0.9} />;
   };
   ```

### Reference Elements

| Element | Value | Color | Dash |
|---|---|---|---|
| UNPL line | `UNPL` | ALARM `#ff6b6b` | `"6 3"` |
| X̄ line | `MEAN` | PRIMARY `#6b7de8` | `"4 4"` |
| Shift zone | `ReferenceArea` shift start→end | CAUTION `#e2c97e` at 4% fill | `"4 4"` stroke |

### X-Axis Configuration

- `type="number"`, `domain=[-0.6, SUBGROUPS.length - 0.4]`
- `ticks` = integer index array `[0, 1, 2, ...]`
- `tickFormatter` maps index to shortened label
- `angle={-45}`, `textAnchor="end"`, `height={60}`

### Y-Axis Domain & Scale Toggle

Include a `useState` toggle between linear and log scale — essential because extreme
outliers (400–800 day items) compress the lower distribution in linear view.

- **Linear:** `[0, ceil(max(P97, UNPL * 1.15) / 50) * 50]` — clips outliers, keeps UNPL visible
- **Log:** `[0.5, 1000]`

Use `allowDataOverflow` so dots above the linear cap still render at the top edge.

Style the toggle per the **Interactive Controls** pattern in `mcs-charts-base`
(rectangular button, right-aligned in the badge row via flex spacer).

---

## Header

Per `mcs-charts-base` header structure, with these specifics:

- **Title:** exactly `"Strategic Process Evolution"`
- **Subtitle:** `"Three-Way Control Chart · Cycle Time Distribution · {date range}"`
- **Stat cards:**

| Label | Value | Color |
|---|---|---|
| `Process Avg (X̄)` | `${MEAN.toFixed(1)}d` | PRIMARY `#6b7de8` |
| `UNPL` | `${UNPL.toFixed(1)}d` | ALARM `#ff6b6b` |
| `Items` | total item count | SECONDARY `#7edde2` |
| `Months` | subgroup count | MUTED `#505878` |

- **Badges:**
  1. Process shift (if detected) — CAUTION styling, e.g. "Process Shift: 8-point run (Mar 24 – Oct 24)"
  2. Summary — PRIMARY styling, e.g. "60-month window · 32 subgroups · 861 items"
  3. Scale toggle — right-aligned (see above)

  **Do NOT include a "Status: stable/unstable/migrating" badge** — that is a tool-internal
  classification, not a user-facing label. Do not use "regime" or "migrating" language anywhere.

---

## Legend

Three groups (per `mcs-charts-base` manual legend pattern):
1. **Dot types:** normal (PRIMARY indigo circle), shift zone (CAUTION amber circle), above UNPL (ALARM red circle)
2. **Avg bar types:** normal (TEXT white line), shift zone (CAUTION amber line), above UNPL (ALARM red line)
3. **Reference lines:** X̄ (PRIMARY indigo dashed), UNPL (ALARM red dashed)

---

## Footer

Three required sections:

1. **"What is a Three-Way Control Chart?"** — Longitudinal audit tool for process behavior
   over longer time horizons. Groups cycle times into monthly subgroups, applies XmR to
   subgroup averages. X̄ and UNPL define routine variation. Points outside UNPL or 8+
   consecutive points on one side of X̄ signal a process shift.

2. **"Reading this chart:"** — Dots = individual items; horizontal bars = subgroup averages;
   amber zone = shift run; red dots/bars = above UNPL; scale toggle for linear/log.

3. **"Note on early subgroups:"** — Low-count months (single-item subgroups) produce a
   technically correct but statistically meaningless average bar. Interpret those months
   with caution.

---

## Chart-Specific Checklist

*(Universal items are in `mcs-charts-base`. Only chart-specific items listed here.)*

- [ ] Skill was triggered by `analyze_process_evolution` data
- [ ] Chart title reads exactly **"Strategic Process Evolution"**
- [ ] Individual items rendered as scatter dots — not bars, not lines
- [ ] Monthly averages rendered as **disconnected horizontal bars** (NOT a connected line)
- [ ] Average bars color-coded: TEXT white (normal), CAUTION amber (shift), ALARM red (above UNPL)
- [ ] Dots color-coded: PRIMARY indigo (normal), CAUTION amber (shift zone), ALARM red (above UNPL)
- [ ] XmR reference lines: X̄ (PRIMARY dashed) and UNPL (ALARM dashed) with labels
- [ ] Shift zone highlighted via `ReferenceArea` with CAUTION amber shading at 4% fill
- [ ] Scale toggle present, styled as interactive button (NOT a pill badge), right-aligned
- [ ] **No "Status: migrating/stable/unstable" badge** — do not surface tool-internal labels
- [ ] No "regime" or "migrating" language anywhere in the output
- [ ] Footer has all three required sections
- [ ] Tool does not provide issue type per item — do not claim type-level breakdown
