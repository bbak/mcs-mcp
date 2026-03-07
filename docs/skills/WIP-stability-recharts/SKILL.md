---
name: wip-stability-chart
description: >
  Creates a dark-themed React/Recharts chart for the **WIP Count Stability** analysis
  (mcs-mcp:analyze_wip_stability). Trigger on: "WIP stability chart", "WIP count chart",
  "daily WIP chart", "WIP run chart", "WIP XmR", or any "show/chart/plot/visualize that"
  follow-up after an analyze_wip_stability result is present.
  ONLY for analyze_wip_stability output (daily active WIP count with XmR Process Behavior
  Chart limits). Do NOT use for: Total WIP Age stability (analyze_wip_age_stability),
  cycle time stability (analyze_process_stability), or throughput (analyze_throughput).
  Those are different analyses requiring different charts.
  Always read this skill before building the chart — do not attempt it ad-hoc.
---

# WIP Count Stability Chart

> **Scope:** Use this skill ONLY when `analyze_wip_stability` data is present in the
> conversation. It visualizes the daily count of active in-progress items as a Process
> Behavior Chart (Wheeler XmR), with UNPL/LNPL reference lines and colored signal dots.
>
> **Do not use this skill for:**
> - Total WIP Age (cumulative age burden) → use `analyze_wip_age_stability`
> - Cycle time / lead time stability → use `analyze_process_stability`
> - Throughput volume → use `analyze_throughput`

---

## Prerequisites

Call the tool if data is not yet in the conversation:

```js
mcs-mcp:analyze_wip_stability({ board_id, project_key })
// Optional: history_window_weeks (default 26)
```

---

## Response Structure

```
data.wip_stability
  .run_chart[]          — daily data points
    .date               — ISO 8601 with timezone offset e.g. "2025-09-06T00:00:00+02:00"
                          Strip to "YYYY-MM-DD" for display and signal key matching
    .count              — active WIP item count for that day

  .xmr
    .average                      — process mean (X̄)
    .average_moving_range         — MR̄
    .upper_natural_process_limit  — UNPL = X̄ + 2.66 × MR̄
    .lower_natural_process_limit  — LNPL = X̄ − 2.66 × MR̄ (may be > 0)
    .values[]                     — weekly subgroup averages used for XmR calculation
    .moving_ranges[]              — week-to-week absolute differences (length = n-1)
    .signals[]                    — null or array of signal objects:
        .index       — position in values[] array
        .key         — "YYYY-MM-DD" — use this to match signal points
        .type        — "outlier"
        .description — human-readable e.g. "WIP count above Upper Natural Process Limit"

  .status               — "stable" or "unstable"
```

**Important:** `run_chart` dates include timezone offsets. Strip to `YYYY-MM-DD` before
use (e.g. `d.date.slice(0, 10)`). Signal keys in `xmr.signals` are already `YYYY-MM-DD`.

---

## Data Preparation

```js
const MEAN = data.wip_stability.xmr.average;
const UNPL = data.wip_stability.xmr.upper_natural_process_limit;
const LNPL = data.wip_stability.xmr.lower_natural_process_limit;
const STATUS = data.wip_stability.status; // "stable" | "unstable"

// Build signal sets from xmr.signals (already YYYY-MM-DD keys)
const SIGNALS_ABOVE = new Set(
  (data.wip_stability.xmr.signals || [])
    .filter(s => s.description.toLowerCase().includes("above"))
    .map(s => s.key)
);
const SIGNALS_BELOW = new Set(
  (data.wip_stability.xmr.signals || [])
    .filter(s => s.description.toLowerCase().includes("below"))
    .map(s => s.key)
);

// Annotate data — strip timezone from date
const RAW = data.wip_stability.run_chart.map(d => ({
  date:       d.date.slice(0, 10),
  count:      d.count,
  mean:       MEAN,
  unpl:       UNPL,
  lnpl:       LNPL,
  aboveUnpl:  SIGNALS_ABOVE.has(d.date.slice(0, 10)),
  belowLnpl:  SIGNALS_BELOW.has(d.date.slice(0, 10)),
}));

// Downsample if > 100 points — retain every 2nd point, always keep signals
const CHART_DATA = RAW.filter((d, i) =>
  RAW.length <= 100 || i % 2 === 0 || d.aboveUnpl || d.belowLnpl
);
```

---

## Chart Architecture

Single `ComposedChart` with one Y-axis. No dual axis (unlike Total WIP Age chart).

**Series — rendered in this order:**

1. **`<Area>`** — `dataKey="count"`, gradient fill, no stroke, no dots, no active dot.
   Used purely for the shaded fill under the line. Set `stroke="none"`.

2. **`<Line>`** — `dataKey="count"`, solid cyan `#7edde2`, `strokeWidth={1.5}`,
   custom dots (signal points only), `activeDot={false}`.

3. **`<ReferenceLine>`** for UNPL — `#ff6b6b`, dashed `"6 3"`, label "UNPL".
4. **`<ReferenceLine>`** for MEAN — `#6b7de8`, dashed `"4 4"`, label "X̄".
5. **`<ReferenceLine>`** for LNPL — `#6bffb8`, dashed `"6 3"`, label "LNPL".
   Always render LNPL even if value is close to 0 — it is always present in this tool's output.

**Gradient fill:**
```jsx
<defs>
  <linearGradient id="wipGrad" x1="0" y1="0" x2="0" y2="1">
    <stop offset="5%"  stopColor="#7edde2" stopOpacity={0.18} />
    <stop offset="95%" stopColor="#7edde2" stopOpacity={0.02} />
  </linearGradient>
</defs>
// ...
<Area dataKey="count" fill="url(#wipGrad)" stroke="none" dot={false} activeDot={false} />
```

---

## CustomDot

Paint colored circles only on signal-breach points:

```jsx
const CustomDot = ({ cx, cy, payload }) => {
  if (payload.aboveUnpl)
    return <circle cx={cx} cy={cy} r={5} fill="#ff6b6b" stroke="#080a0f" strokeWidth={1.5} />;
  if (payload.belowLnpl)
    return <circle cx={cx} cy={cy} r={5} fill="#6bffb8" stroke="#080a0f" strokeWidth={1.5} />;
  return null;
};
```

The `stroke="#080a0f" strokeWidth={1.5}` halo makes dots pop against the line.

---

## Y-Axis Domain

```js
const Y_MIN = Math.floor((LNPL - 5) / 5) * 5;
const Y_MAX = Math.ceil((Math.max(...RAW.map(d => d.count)) + 5) / 5) * 5;
// domain={[Y_MIN, Y_MAX]}
```

Single axis, left orientation. Label: `"Active WIP (items)"` (angle −90).
Tick formatter: plain integer `v => \`${v}\``.

---

## X-Axis

```js
// Format: "06 Sep" style
const fmtDate = (iso) => {
  const d = new Date(iso);
  return d.toLocaleDateString("en-GB", { day: "2-digit", month: "short" });
};

// Interval — aim for ~8–10 visible labels
const interval = Math.floor(CHART_DATA.length / 8);
```

Angle −45°, `textAnchor="end"`, height 60.

---

## Tooltip

```
{date formatted as "DD MMM YYYY"}
WIP Count: {count}  ← colored cyan
─────────────────
X̄:    {MEAN.toFixed(1)}
UNPL: {UNPL.toFixed(1)}
LNPL: {LNPL.toFixed(1)}
[if aboveUnpl] ⚠ Signal: above UNPL  ← red, bold
[if belowLnpl] ⬇ Signal: below LNPL  ← green, bold
```

---

## Header

- **Breadcrumb:** `{PROJECT_KEY} · {board name} · Board {board_id}` — muted, uppercase
- **Title:** exactly `"WIP Count Stability"`
- **Subtitle:** `"Daily Active WIP · XmR Process Behavior Chart · {start} – {end}"`
- **Stat cards:**

| Label | Value | Border color |
|---|---|---|
| `X̄ Mean` | `{MEAN.toFixed(1)}` | PRIMARY `#6b7de8` |
| `UNPL`   | `{UNPL.toFixed(1)}` | ALARM `#ff6b6b` |
| `LNPL`   | `{LNPL.toFixed(1)}` | POSITIVE `#6bffb8` |
| `Today`  | current WIP count (`RAW.at(-1).count`) | SECONDARY `#7edde2` |

---

## Signal Badges

Always show a **Status verdict badge** first:

- `STATUS === "stable"` → green `#6bffb8` — `"STATUS: STABLE"`
- `STATUS === "unstable"` → red `#ff6b6b` — `"STATUS: UNSTABLE"`

Then one badge per signal, grouped by type:

```
↓ Below LNPL · {date range}   — green style
↑ Above UNPL · {date range}   — red style
```

Badge styles (same as base skill):
- UNSTABLE / above: `background: #ff6b6b15`, `border: 1px solid #ff6b6b40`, `color: #ff6b6b`
- STABLE / below: `background: #6bffb815`, `border: 1px solid #6bffb840`, `color: #6bffb8`

To produce signal date labels: parse signal keys and format as `"DD MMM YYYY"`.
If two signals are consecutive calendar days, render as a range `"DD – DD MMM YYYY"`.

---

## Legend

Manual legend (never use Recharts `<Legend>` component). Center below the chart panel.

```
── (cyan solid)        Daily WIP Count
-- (red dashed)        UNPL
-- (indigo dashed)     X̄ Mean
-- (green dashed)      LNPL
● (red dot)            Signal above UNPL
● (green dot)          Signal below LNPL
```

Use inline `<svg>` for line swatches. For dot swatches, use a plain `<div>` or `<span>`
with matching background + border-radius: 50%.

---

## Footer

Two sections:

1. **"Reading this chart:"** — The line shows the daily count of active in-progress items
   (past the commitment point, not yet finished). The dashed X̄ is the process mean. UNPL
   and LNPL define the expected range of natural variation — points outside these limits are
   statistical signals. Uncontrolled WIP violates the assumptions of Little's Law, making
   cycle time predictions fundamentally unreliable.

2. **"Data provenance:"** — Wheeler XmR Process Behavior Chart applied to weekly WIP
   subgroup averages. Limits: X̄ ± 2.66 × MR̄. Colored dots mark signal breaches
   (red = above UNPL, green = below LNPL). Chart downsampled to every 2nd point for
   readability; all signal points are always retained.

---

## Checklist Before Delivering

- [ ] Skill triggered by `analyze_wip_stability` data (not WIP Age or cycle time data)
- [ ] Chart title reads exactly **"WIP Count Stability"**
- [ ] Single Y-axis only (no dual axis)
- [ ] Area series used for fill only (`stroke="none"`, no dots)
- [ ] Line series for the actual line + signal dots
- [ ] Signal dots: red circle for above UNPL, green for below LNPL, null otherwise
- [ ] Date strings stripped to `YYYY-MM-DD` before signal key matching
- [ ] UNPL, MEAN, LNPL all present as reference lines
- [ ] Status badge: STABLE (green) or UNSTABLE (red)
- [ ] One signal badge per breach event with date
- [ ] Stat cards: X̄, UNPL, LNPL, Today's WIP count
- [ ] Manual legend below chart panel (no Recharts Legend component)
- [ ] Downsampled to every 2nd point; all signal points always retained
- [ ] Dark theme throughout (page `#080a0f`, panel `#0c0e16`, grid `#1a1d2e`)
- [ ] Monospace font throughout
- [ ] Single self-contained `.jsx` file with default export
