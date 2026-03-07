---
name: residence-time-chart
description: >
  Creates a dark-themed dual-panel React/Recharts chart for the **Residence Time Analysis**
  (mcs-mcp:analyze_residence_time). Trigger on: "residence time chart", "Little's Law chart",
  "coherence gap chart/visualization", "sojourn time vs residence time", "sample path analysis chart",
  or any "show/chart/plot/visualize that" follow-up after an analyze_residence_time result is present.
  Use ONLY when analyze_residence_time data is present in the conversation.
  Always read this skill before building the chart — do not attempt it ad-hoc.
---

# Residence Time Chart

> **Also read:** `mcs-charts-base` SKILL before implementing. It defines the page/panel
> wrappers, color tokens, typography, header structure, badge system, CartesianGrid,
> tooltip base style, area gradient pattern, legend pattern, interactive controls, footer
> structure, and universal checklist items. This skill only specifies what is unique to
> this chart.

> **Scope:** Use this skill ONLY when `analyze_residence_time` data is present in the
> conversation. It is exclusively for charting the Sample Path Analysis (Stidham 1972,
> El-Taha & Stidham 1999) that computes the finite version of Little's Law:
> L(T) = Λ(T) · w(T).

---

## Prerequisites

Call the tool if data is not yet in the conversation:

```js
mcs-mcp:analyze_residence_time({ board_id, project_key })
```

Response structure:
- `data.residence_time.series` — daily data points:
  `date`, `n` (active WIP count), `w` (avg residence time), `w_star` (avg sojourn time),
  `coherence_gap` (w − w_star), `lambda` (cumulative departure rate d/h), `a` (cumulative arrivals),
  `d` (cumulative departures / historical resolved count), `h` (cumulative horizon days),
  `l` (running average population)
- `data.residence_time.summary` — final values:
  `final_w`, `final_w_star`, `final_coherence_gap`, `final_lambda`, `final_l`,
  `active_items`, `in_window_arrivals`, `pre_window_items`, `departures`, `convergence`
- `data.residence_time.validation` — `identity_verified` (boolean)

---

## Critical Semantic Notes

**Do not confuse these fields:**

| Field | Meaning | Scope |
|---|---|---|
| `a` / `in_window_arrivals` | Items entering committed WIP during the observation window | Window-scoped |
| `d` / `departures` | Total historical resolved item count — denominator for W* calculation | Historical (all time) |
| `pre_window_items` | Items already in WIP when the window opened | Pre-window |

`a` and `d` are **NOT** symmetric in/out counts for the same window. `d` is the cumulative
historical denominator for sojourn time W*. Presenting them side-by-side as "Arrivals vs
Departures" is **misleading**. Always label them correctly and separately.

`lambda` (λ = d/h) is the **cumulative departure rate**, not an "arrival rate."

---

## Data Preparation

| Constant | Source |
|---|---|
| `RAW` | `series` array — keep: `date`, `n`, `w`, `w_star`, `coherence_gap` (rename to `gap`), `lambda`, `a` |
| `FINAL_W` | `summary.final_w` |
| `FINAL_W_STAR` | `summary.final_w_star` |
| `FINAL_GAP` | `summary.final_coherence_gap` |
| `FINAL_LAMBDA` | `summary.final_lambda` |
| `FINAL_N` | `summary.active_items` |
| `WINDOW_ARRIVALS` | `summary.in_window_arrivals` |
| `PRE_WINDOW_ITEMS` | `summary.pre_window_items` |
| `RESOLVED_TOTAL_D` | `summary.departures` |
| `CONVERGENCE` | `summary.convergence` |

**Downsampling:** If `series` has more than ~120 points, downsample to every 3rd–4th point.
Always retain the first and last points.

```js
const data = RAW.map(d => ({
  date: d.label || d.date.slice(0, 10),
  n: d.n,
  w:      round(d.w, 2),
  w_star: round(d.w_star, 2),
  gap:    round(d.coherence_gap, 2),
  lambda: round(d.lambda, 4),
  a: d.a,
}));
```

---

## Chart Architecture

Two chart panels stacked vertically inside a single page container.

### Panel 1: Residence Time vs Sojourn Time (Coherence Gap)

`ComposedChart` with two Y-axes:

| Axis | `yAxisId` | Orientation | Data | Tick color |
|---|---|---|---|---|
| Left | `"left"` | left | `w` and `gap` | PRIMARY `#6b7de8` |
| Right | `"right"` | right | `w_star` | CAUTION `#e2c97e` |

**Series:**
1. **Area** (`yAxisId="left"`) — `gap` (coherence gap), ALARM red gradient fill (`#ff6b6b` 20%→2%), red stroke 0.7 opacity, no dots
2. **Line** (`yAxisId="left"`) — `w` (residence time), PRIMARY `#6b7de8`, strokeWidth 2, no dots
3. **Line** (`yAxisId="right"`, dashed `"4 3"`) — `w_star` (sojourn time), CAUTION `#e2c97e`, strokeWidth 2, no dots

**Y-axis domains:**
- Left: `[0, Math.ceil((maxW + 5) / 10) * 10]`
- Right: `[Math.floor((minWStar - 2) / 5) * 5, Math.ceil((maxWStar + 2) / 5) * 5]`

**Axis labels:** Left: `"Days"` (angle -90) · Right: `"W* (days)"` (angle 90)

**Height:** 380px

### Panel 2: Departure Rate λ(T)

`ComposedChart` with single Y-axis.

**Series:**
1. **Area** — `lambda`, SECONDARY cyan gradient (`#7edde2` 20%→2%), solid cyan stroke, strokeWidth 2, no dots
2. **ReferenceLine** — `y={1.0}`, MUTED `#505878`, dashed `"4 4"`, label `"λ = 1"`

**Y-axis domain:** `[0, 1.6]` (adjust if data exceeds)
**Tick formatter:** `v.toFixed(1)`
**Axis label:** `"λ (dep/d)"` (angle -90, SECONDARY cyan)

**Height:** 260px

---

## Header

Per `mcs-charts-base` header structure, with these specifics:

- **Title:** exactly `"Residence Time Analysis"`
- **Subtitle:** `"Sample Path Analysis · Little's Law Identity · {date range}"`
- **Stat cards:**

| Label | Value | Color |
|---|---|---|
| `Residence w̄(T)` | `${FINAL_W.toFixed(1)}d` | PRIMARY `#6b7de8` |
| `Sojourn W*(T)` | `${FINAL_W_STAR.toFixed(1)}d` | CAUTION `#e2c97e` |
| `Gap w̄−W*` | `${FINAL_GAP.toFixed(1)}d` | ALARM `#ff6b6b` |
| `λ(T)` | `${FINAL_LAMBDA.toFixed(2)}/d` | SECONDARY `#7edde2` |

- **Badges:**
  1. Convergence — POSITIVE green if "converging", ALARM red if "diverging"
  2. Active WIP — MUTED styling, e.g. "Active: {FINAL_N} items"
  3. Window arrivals — MUTED, e.g. "Window arrivals (a): {WINDOW_ARRIVALS}"
  4. Resolved total — MUTED, e.g. "Resolved (d): {RESOLVED_TOTAL_D} (W* denominator)"
  5. Identity check — CAUTION amber, e.g. "L(T) = Λ(T) · w(T) ✓"

---

## Tooltip

Per `mcs-charts-base` tooltip base style, with these fields:

| Label (colored) | Value |
|---|---|
| Date (header, bold) | formatted long date |
| Residence w̄ | PRIMARY · `{d.w.toFixed(1)} d` |
| Sojourn W* | CAUTION · `{d.w_star.toFixed(1)} d` |
| Gap w̄−W* | ALARM · `{d.gap.toFixed(1)} d` |
| ──────── separator | |
| λ(T) | SECONDARY · `{d.lambda.toFixed(2)} /d` |
| Active (n) | MUTED · `{d.n}` |
| Arrivals (a) | MUTED · `{d.a}` |

---

## Legend

Per `mcs-charts-base` manual legend pattern, one legend per panel:

**Panel 1:**
- PRIMARY solid line → "Residence w̄(T)"
- CAUTION dashed line → "Sojourn W*(T)"
- ALARM filled rect (25% opacity) → "Coherence Gap"

**Panel 2:**
- SECONDARY filled rect (25% opacity) → "Departure Rate λ(T)"
- MUTED dashed line → "λ = 1 (equilibrium)"

---

## Footer

Two required sections:

1. **"Reading this chart:"** — Top chart shows residence time w̄(T) (blue, left axis) —
   average time items spend in the system including active WIP — against sojourn time W*(T)
   (amber dashed, right axis) — average cycle time of completed items only. Red shaded area
   is the coherence gap: how much active WIP inflates average residence time beyond
   completed-item experience. A diverging gap signals aging WIP accumulation. Bottom chart
   tracks cumulative departure rate λ(T) = d(T)/h(T); values above 1.0 mean the system
   resolves more than one item per elapsed day on average.

2. **"Data provenance:"** — Sample Path Analysis (Stidham 1972, El-Taha & Stidham 1999).
   Little's Law identity L(T) = Λ(T) · w(T) verified exactly. Window arrivals (a): items
   entering committed WIP during the window. Resolved (d): total historical departures used
   as W* denominator.

---

## Chart-Specific Checklist

*(Universal items are in `mcs-charts-base`. Only chart-specific items listed here.)*

- [ ] Skill was triggered by `analyze_residence_time` data
- [ ] Chart title reads exactly **"Residence Time Analysis"**
- [ ] Two chart panels: Panel 1 (Residence vs Sojourn) and Panel 2 (Departure Rate)
- [ ] Panel 1 has dual Y-axes: left for w̄(T) and gap, right for W*(T)
- [ ] Coherence gap rendered as red shaded Area — NOT a line
- [ ] W*(T) rendered as CAUTION amber dashed line on the right axis
- [ ] Panel 2 shows λ(T) as SECONDARY cyan area with reference line at λ = 1
- [ ] λ(T) labeled as **Departure Rate** — NOT "Arrival Rate"
- [ ] Badges correctly distinguish window arrivals (a) from historical resolved count (d)
- [ ] `d` badge labeled as "(W* denominator)", not as "Departures"
- [ ] Convergence badge: POSITIVE green (converging) or ALARM red (diverging)
- [ ] Tooltip shows date, w̄, W*, gap, λ, n, a
- [ ] Data downsampled if >120 points (retain first and last)
