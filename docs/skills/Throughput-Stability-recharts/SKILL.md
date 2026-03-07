---
name: throughput-chart
description: >
  Creates a dark-themed React/Recharts chart for the **Throughput Stability** analysis
  (mcs-mcp:analyze_throughput). Trigger on: "throughput chart", "throughput stability",
  "delivery cadence chart", "throughput XmR", "weekly deliveries chart", or any
  "show/chart/plot/visualize that" follow-up after an analyze_throughput result is present.
  ONLY for analyze_throughput output (weekly delivery volume with XmR Process Behavior
  Chart limits). Do NOT use for: cycle time stability (analyze_process_stability), WIP count
  stability (analyze_wip_stability), Total WIP Age stability (analyze_wip_age_stability),
  or CFD (generate_cfd_data). Those are different analyses requiring different charts.
  Always read this skill before building the chart — do not attempt it ad-hoc.
---

# Throughput Chart

> **Scope:** Use this skill ONLY when `analyze_throughput` data is present in the
> conversation. It visualizes weekly delivery volume as a Process Behavior Chart (Wheeler
> XmR), with stacked issue-type bars and XmR Natural Process Limits overlaid as reference
> lines. A Moving Range panel below the main chart shows delivery variability week-to-week.

---

## Prerequisites

Call the tool if data is not yet in the conversation:

```js
mcs-mcp:analyze_throughput({ board_id, project_key })
// Optional: history_window_weeks (default 26)
```

---

## Response Structure

```
data.stability
  .average                      — process mean (X̄)
  .average_moving_range         — mean moving range (MR̄)
  .upper_natural_process_limit  — UNPL = X̄ + 2.66 × MR̄
  .lower_natural_process_limit  — LNPL (floored at 0)
  .values[]                     — weekly total throughput counts (one per week)
  .moving_ranges[]              — week-to-week absolute differences (length = n-1)
  .signals                      — null or array of signal objects

data.total_throughput[]         — same as stability.values (use either)

data.stratified_throughput
  .Story[]                      — per-week Story delivery counts
  .Bug[]                        — per-week Bug delivery counts
  .Activity[]                   — per-week Activity delivery counts
  .Defect[]                     — per-week Defect delivery counts
  (keys are issue type names — may vary by project)

data.@metadata[]                — one entry per week:
  .label      — ISO week label e.g. "2025-W36"
  .start_date — "YYYY-MM-DD"
  .end_date   — "YYYY-MM-DD"
  .index      — "1", "2", … (string)
  .is_partial — "true" / "false" (string) — last bucket may be incomplete
```

---

## Data Preparation

Zip `@metadata` with `stratified_throughput` and `stability.moving_ranges` into a single
array. Mark partial weeks. Annotate signal breaches.

```js
const MEAN  = data.stability.average;
const UNPL  = data.stability.upper_natural_process_limit;
const LNPL  = data.stability.lower_natural_process_limit;
const MR_MEAN = data.stability.average_moving_range;
// UNPL for Moving Range chart: D4 × MR̄ (D4 = 3.267 for n=2)
const MR_UNPL = 3.267 * MR_MEAN;

// Build signal sets
const signalKeys = new Set((data.stability.signals || []).map(s => s.key || s.date || s));

const weeks = data["@metadata"].map((meta, i) => ({
  label:     meta.label,          // "2025-W36"
  startDate: meta.start_date,
  endDate:   meta.end_date,
  isPartial: meta.is_partial === "true",
  total:     data.total_throughput[i],
  // stratified bars (embed all issue types present)
  ...Object.fromEntries(
    Object.entries(data.stratified_throughput).map(([type, arr]) => [type, arr[i]])
  ),
  // XmR annotations
  mr:        i > 0 ? data.stability.moving_ranges[i - 1] : null,
  mean:      MEAN,
  unpl:      UNPL,
  lnpl:      LNPL,
  mrMean:    MR_MEAN,
  mrUnpl:    MR_UNPL,
  isSignal:  signalKeys.has(meta.label),
  isAbove:   data.total_throughput[i] > UNPL,
  isBelow:   data.total_throughput[i] < LNPL && LNPL > 0,
}));
```

**Partial week:** The last bucket often has `is_partial: "true"` because the current week
is not yet complete. Always render it with reduced opacity (`0.4`) and add a tooltip note.

---

## Chart Architecture

Two panels stacked vertically inside a single page container.

### Panel 1: Throughput Run Chart (main) — height 400px

`ComposedChart` with a single Y-axis.

**Series — rendered in this order:**

1. **Stacked `<Bar>` series** — one per issue type, using `stackId="tp"`.
   Only render types that have at least one non-zero value.
   Issue type colors (semantic, fixed):
   ```js
   const ISSUE_TYPE_COLORS = {
     "Story":    "#6b7de8",
     "Bug":      "#ff6b6b",
     "Activity": "#7edde2",
     "Defect":   "#e2c97e",
   };
   // fallback for unknown types:
   const FALLBACK_COLORS = ["#6bffb8", "#d946ef", "#f97316"];
   ```
   Partial-week bar: apply `fillOpacity={0.4}` via `Cell` on the last bar.

2. **`<ReferenceLine>`** for UNPL — `#ff6b6b`, dashed `"6 3"`, label "UNPL" at left edge.
3. **`<ReferenceLine>`** for MEAN — `#6b7de8`, dashed `"4 4"`, label "X̄" at left edge.
4. **`<ReferenceLine>`** for LNPL — only render if `LNPL > 0` — `#6bffb8`, dashed `"6 3"`,
   label "LNPL" at left edge.

**Y-axis:** domain `[0, Math.ceil((UNPL * 1.2) / 5) * 5]`, label `"Items / week"` (angle -90).

**X-axis:** `dataKey="label"`, angle -45°, `textAnchor="end"`, height 60,
interval `Math.floor(weeks.length / 10)`.

### Panel 2: Moving Range Chart — height 200px

`ComposedChart` with a single Y-axis, sharing the same X-axis labels.

**Series:**

1. **`<Bar>`** — `dataKey="mr"`, color `#505878` (muted), no stack. Skip null (first week).
   Signal bars (where `mr > MR_UNPL`): render in ALARM red `#ff6b6b`.
2. **`<ReferenceLine>`** for MR_UNPL — `#ff6b6b`, dashed `"6 3"`, label "URL" at left.
3. **`<ReferenceLine>`** for MR_MEAN — `#6b7de8`, dashed `"4 4"`, label "MR̄" at left.

**Y-axis:** domain `[0, Math.ceil((MR_UNPL * 1.3) / 5) * 5]`, label `"Moving Range"` (angle -90).

---

## Color & Visual Design

Follow the base skill's dark theme (page `#080a0f`, panel `#0c0e16`, border `#1a1d2e`,
grid `#1a1d2e` horizontal only, ticks `#404660`, font `'Courier New', monospace`).

Signal bars in the main chart: apply a red stroke outline via `Cell` rather than changing
the fill color (the stacked fill colors must remain by issue type). Use `stroke="#ff6b6b"`
and `strokeWidth={2}` on the outermost bar of the stack for that week.

---

## Header

- **Breadcrumb:** `{PROJECT_KEY} · {project name} · Board {board_id}`
- **Title:** exactly `"Throughput Stability"`
- **Subtitle:** `"Weekly Delivery Volume · XmR Process Behavior Chart · {start} – {end}"`
- **Stat cards:**

| Label | Value | Color |
|---|---|---|
| `X̄ (MEAN)` | `{MEAN.toFixed(1)} / wk` | PRIMARY `#6b7de8` |
| `UNPL` | `{UNPL.toFixed(1)} / wk` | ALARM `#ff6b6b` |
| `LNPL` | `{LNPL > 0 ? LNPL.toFixed(1) + " / wk" : "—"}` | POSITIVE `#6bffb8` |
| `WINDOW` | `{n} weeks` | MUTED `#505878` |

- **Signal badges:** one badge per signal week (amber CAUTION style), plus a
  **Status verdict badge** — "STABLE" (POSITIVE green) if `signals` is null or empty,
  "SIGNALS DETECTED" (ALARM red) otherwise.
- **Partial week note** (if last week is partial): small muted badge —
  e.g. "⚠ {label} partial week — excluded from limits"

---

## Tooltip

Show on hover over either panel. Fields:

| Field | Value |
|---|---|
| Week (header) | `{label}  ({startDate} – {endDate})` |
| *(per issue type)* | colored · `{count} items` — only show if count > 0 |
| **Total** | bold · `{total} items` |
| ──── separator | |
| X̄ | `{MEAN.toFixed(1)}` |
| UNPL | `{UNPL.toFixed(1)}` |
| Moving Range | `{mr ?? "—"}` (null for first week) |
| *(if isPartial)* | CAUTION amber · "Partial week" |
| *(if isSignal)* | ALARM red · "⚠ Signal detected" |

---

## Legend

Manual legend (never use Recharts `<Legend>` component). Place below the main chart panel,
centered. Show one swatch per issue type (filled rect + label). Add UNPL and MEAN as dashed
line swatches. Use the same colors as the reference lines and bars.

---

## Footer

Two required sections:

1. **"Reading this chart:"** — Bars show weekly delivery volume stacked by issue type.
   The dashed X̄ line is the process mean. UNPL (Upper Natural Process Limit) and LNPL
   (Lower Natural Process Limit) define the expected range of natural variation. Bars
   outside these limits are statistical signals — evidence of a special cause worth
   investigating. The Moving Range panel below shows week-to-week variation; spikes above
   the Upper Range Limit indicate unusually large step-changes in delivery rate.

2. **"Data provenance:"** — Wheeler XmR Process Behavior Chart. Limits computed as
   X̄ ± 2.66 × MR̄ (Natural Process Limits). Moving Range Upper Limit = 3.267 × MR̄.
   Partial weeks (current week in progress) are shown at reduced opacity and excluded from
   limit calculations.

---

## Chart-Specific Checklist

- [ ] Skill triggered by `analyze_throughput` data
- [ ] Chart title reads exactly **"Throughput Stability"**
- [ ] Two panels: main throughput bars (400px) + moving range bars (200px)
- [ ] Bars are stacked by issue type using `stackId="tp"`
- [ ] Only issue types with at least one non-zero value are rendered
- [ ] UNPL, MEAN, LNPL reference lines present in main panel
- [ ] LNPL reference line only rendered when `LNPL > 0`
- [ ] MR_UNPL and MR_MEAN reference lines present in moving range panel
- [ ] `MR_UNPL` calculated as `3.267 × MR̄` (not hardcoded)
- [ ] Partial week bar rendered at `fillOpacity={0.4}`
- [ ] Signal bars flagged — stroke outline applied to outermost stack bar
- [ ] Status badge: STABLE (green) or SIGNALS DETECTED (red)
- [ ] Tooltip shows per-type counts, total, MR, partial/signal flags
- [ ] Manual legend below main panel (no Recharts Legend component)
- [ ] X̄ stat card shows mean, UNPL and LNPL stat cards shown with correct colors
- [ ] Dark theme applied throughout (page, panel, grid, ticks, fonts)
