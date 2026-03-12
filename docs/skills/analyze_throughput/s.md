---
name: analyze_throughput-chart
description: >
  Creates a dark-themed React/Recharts Throughput Stability chart for the
  (mcs-mcp:analyze_throughput) tool. Trigger on: "throughput chart", "throughput stability",
  "delivery cadence chart", "throughput XmR", "weekly deliveries chart", "monthly deliveries
  chart", or any "show/chart/plot/visualize that" follow-up after an analyze_throughput result
  is present.
  ONLY for analyze_throughput output. Do NOT use for: cycle time stability
  (analyze_process_stability), WIP count stability (analyze_wip_stability), Total WIP Age
  (analyze_wip_age_stability), or process evolution (analyze_process_evolution).
  Always use this skill — do not build the chart ad-hoc.
---

# Throughput Chart — Template Skill

## Approach

This skill uses a **fixed JSX template**. Do not rewrite or restructure the component.

Your only task is to **inject data** into the `DATA (INJECT)` section at the top of the
template. The `CONFIG` and all component/render code beneath it are never modified.

---

## How to Use This Skill

1. Ensure `mcs-mcp:analyze_throughput` has been called and its result is present in the
   conversation.
2. Locate the `// ── DATA (INJECT) ──` block in the template below.
3. Replace each placeholder value with the corresponding value from the MCP response,
   using the **Injection Reference** table as your map.
4. Output the complete file as a `.jsx` artifact. Do not alter anything outside the
   `DATA (INJECT)` block.

---

## API Options — Effect on Response

| Parameter | Effect on response structure | Template impact |
|---|---|---|
| `bucket` | `week` (default) vs `month` — changes `@metadata` label format and bucket count | None — template reads labels dynamically from `@metadata` |
| `include_abandoned` | Adds abandoned items to counts | None — same structure, different numbers |
| `history_window_weeks` | Changes number of buckets | None — template is data-length agnostic |

---

## Injection Reference

| Placeholder constant | MCP response path | Notes |
|---|---|---|
| `METADATA` | `data["@metadata"][]` | Full array — keep all fields: `label`, `start_date`, `end_date`, `index`, `is_partial` |
| `TOTAL_THROUGHPUT` | `data.total_throughput[]` | Array of integers, one per bucket |
| `STRATIFIED_THROUGHPUT` | `data.stratified_throughput` | Object keyed by issue type name; each value is an array of integers aligned to `METADATA` |
| `STABILITY` | `data.stability` | Object with `average`, `average_moving_range`, `upper_natural_process_limit`, `lower_natural_process_limit`, `values[]`, `moving_ranges[]`, `signals` (null or array) |
| `BOARD_ID` | `context.board_id` | Integer |
| `PROJECT_KEY` | `context.project_key` | String |
| `BOARD_NAME` | Known from `import_boards` result | String |

**Signal object shape** (inside `STABILITY.signals`, or `null` if no signals):
```js
{ index: number, key: string, type: "outlier" | "shift", description: string }
// Note: signals on this tool reference bucket index, not item key
```

**`is_partial` field:** String `"true"` or `"false"`, not boolean. The template handles
the coercion — inject the raw string value as-is.

---

## Template

```jsx
import { useState } from "react";
import {
  ComposedChart, Bar, Cell, ReferenceLine, XAxis, YAxis,
  CartesianGrid, Tooltip, ResponsiveContainer,
} from "recharts";

// ── CONFIG ────────────────────────────────────────────────────────────────────
// Visual settings. Safe to adjust offline; never modified during data injection.

const CFG = {
  // Chart dimensions
  mainChartHeight: 400,
  mrChartHeight:   200,
  mainMargin:      { top: 10, right: 20, bottom: 60, left: 10 },
  mrMargin:        { top: 10, right: 20, bottom: 60, left: 10 },

  // Y-axis: snap-to-grid step for domain ceiling
  ySnapStep: 5,

  // X-axis: target number of visible tick labels
  xTickCount: 10,

  // Partial week opacity
  partialOpacity: 0.4,

  // Issue type colors (semantic, fixed)
  issueTypeColors: {
    Story:    "#6b7de8",
    Bug:      "#ff6b6b",
    Activity: "#7edde2",
    Defect:   "#e2c97e",
  },
  // Fallback palette for unknown issue types (cycles if needed)
  fallbackPalette: ["#6bffb8", "#d946ef", "#f97316"],

  // Color tokens
  COLOR_ALARM:    "#ff6b6b",
  COLOR_CAUTION:  "#e2c97e",
  COLOR_PRIMARY:  "#6b7de8",
  COLOR_POSITIVE: "#6bffb8",
  COLOR_TEXT:     "#dde1ef",
  COLOR_MUTED:    "#505878",
  COLOR_PAGE_BG:  "#080a0f",
  COLOR_PANEL_BG: "#0c0e16",
  COLOR_BORDER:   "#1a1d2e",
  COLOR_MR_BAR:   "#505878",
};

// ── DATA (INJECT) ─────────────────────────────────────────────────────────────
// Replace the values below with data from the mcs-mcp:analyze_throughput
// tool response. See the Injection Reference table in the SKILL.md for field paths.
// Do NOT modify anything outside this block.

const BOARD_ID    = 0;        // context.board_id
const PROJECT_KEY = "PROJ";   // context.project_key
const BOARD_NAME  = "Board";  // from import_boards

const METADATA = [
  // { label: "2025-W37", start_date: "2025-09-08", end_date: "2025-09-14", index: "1", is_partial: "false" },
];

const TOTAL_THROUGHPUT = [
  // integers, one per bucket aligned to METADATA
];

const STRATIFIED_THROUGHPUT = {
  // Keyed by issue type name exactly as returned by the tool.
  // Each value is an array of integers aligned to METADATA.
  // Example:
  // Story:    [0, 3, 4, ...],
  // Bug:      [0, 0, 1, ...],
  // Activity: [1, 1, 3, ...],
};

const STABILITY = {
  average:                     0,
  average_moving_range:        0,
  upper_natural_process_limit: 0,
  lower_natural_process_limit: 0,
  values:        [],  // same as TOTAL_THROUGHPUT
  moving_ranges: [],  // length = n - 1
  signals: null,      // null or array of { index, key, type, description }
};

// ── DERIVED (do not edit) ─────────────────────────────────────────────────────

const MEAN     = STABILITY.average;
const UNPL     = STABILITY.upper_natural_process_limit;
const LNPL     = STABILITY.lower_natural_process_limit;
const MR_MEAN  = STABILITY.average_moving_range;
const MR_UNPL  = 3.267 * MR_MEAN;

const SIGNAL_INDICES = new Set((STABILITY.signals || []).map(s => s.index));

const DETECTED_TYPES  = Object.keys(STRATIFIED_THROUGHPUT);
const ACTIVE_TYPES    = DETECTED_TYPES.filter(t =>
  STRATIFIED_THROUGHPUT[t].some(v => v > 0)
);

const TYPE_COLORS = Object.fromEntries(
  DETECTED_TYPES.map((t, i) => [
    t,
    CFG.issueTypeColors[t] ??
      CFG.fallbackPalette[i % CFG.fallbackPalette.length],
  ])
);

const BUCKETS = METADATA.map((meta, i) => ({
  label:     meta.label,
  startDate: meta.start_date,
  endDate:   meta.end_date,
  isPartial: meta.is_partial === "true",
  total:     TOTAL_THROUGHPUT[i] ?? 0,
  ...Object.fromEntries(
    DETECTED_TYPES.map(t => [t, (STRATIFIED_THROUGHPUT[t] ?? [])[i] ?? 0])
  ),
  mr:       i > 0 ? (STABILITY.moving_ranges[i - 1] ?? null) : null,
  isSignal: SIGNAL_INDICES.has(i),
  isAbove:  (TOTAL_THROUGHPUT[i] ?? 0) > UNPL,
  isBelow:  (TOTAL_THROUGHPUT[i] ?? 0) < LNPL && LNPL > 0,
}));

const DATE_RANGE = METADATA.length > 0
  ? `${METADATA[0].label} – ${METADATA[METADATA.length - 1].label}`
  : "";

const BUCKET_LABEL = METADATA.length > 0 && METADATA[0].label.includes("W")
  ? "week" : "month";

// ── HELPERS ───────────────────────────────────────────────────────────────────

const yDomain = (max) =>
  Math.ceil((max * 1.2) / CFG.ySnapStep) * CFG.ySnapStep;

// ── STAT CARD ─────────────────────────────────────────────────────────────────

const StatCard = ({ label, value, color }) => (
  <div style={{
    background: CFG.COLOR_PANEL_BG, border: `1px solid ${color}33`,
    borderRadius: 8, padding: "10px 16px", minWidth: 110,
  }}>
    <div style={{ fontSize: 10, color: CFG.COLOR_MUTED, marginBottom: 4, letterSpacing: "0.05em" }}>
      {label}
    </div>
    <div style={{ fontSize: 20, fontWeight: 700, color }}>{value}</div>
  </div>
);

// ── BADGE ─────────────────────────────────────────────────────────────────────

const Badge = ({ text, color }) => (
  <span style={{
    fontSize: 11, padding: "4px 10px", borderRadius: 4,
    background: `${color}15`, border: `1px solid ${color}40`, color,
  }}>
    {text}
  </span>
);

// ── TOOLTIP ───────────────────────────────────────────────────────────────────

const CustomTooltip = ({ active, payload }) => {
  if (!active || !payload?.length) return null;
  const d = payload[0].payload;
  return (
    <div style={{
      background: "#0f1117", border: `1px solid ${CFG.COLOR_BORDER}`,
      borderRadius: 8, padding: "10px 14px",
      fontFamily: "'Courier New', monospace", fontSize: 12, color: CFG.COLOR_TEXT,
      minWidth: 200,
    }}>
      <div style={{ fontWeight: 700, marginBottom: 2 }}>{d.label}</div>
      <div style={{ color: CFG.COLOR_MUTED, marginBottom: 6, fontSize: 11 }}>
        {d.startDate} – {d.endDate}
      </div>
      <div style={{ borderTop: `1px solid ${CFG.COLOR_BORDER}`, paddingTop: 6 }}>
        {ACTIVE_TYPES.map(t => d[t] > 0 && (
          <div key={t} style={{ display: "flex", justifyContent: "space-between", gap: 16 }}>
            <span style={{ color: TYPE_COLORS[t] }}>{t}</span>
            <span>{d[t]} items</span>
          </div>
        ))}
        <div style={{ display: "flex", justifyContent: "space-between", gap: 16,
          fontWeight: 700, borderTop: `1px solid ${CFG.COLOR_BORDER}`,
          marginTop: 4, paddingTop: 4 }}>
          <span>Total</span>
          <span>{d.total} items</span>
        </div>
        <div style={{ marginTop: 6, color: CFG.COLOR_MUTED, fontSize: 11 }}>
          <div>X̄: {MEAN.toFixed(1)}</div>
          <div>UNPL: {UNPL.toFixed(1)}</div>
          <div>mR: {d.mr != null ? d.mr.toFixed(1) : "—"}</div>
        </div>
        {d.isPartial && (
          <div style={{ color: CFG.COLOR_CAUTION, marginTop: 4, fontWeight: 700 }}>
            ⚠ Partial {BUCKET_LABEL}
          </div>
        )}
        {d.isSignal && (
          <div style={{ color: CFG.COLOR_ALARM, marginTop: 4, fontWeight: 700 }}>
            ⚠ Signal detected
          </div>
        )}
      </div>
    </div>
  );
};

// ── MAIN EXPORT ───────────────────────────────────────────────────────────────

export default function ThroughputChart() {
  const hasSignals   = STABILITY.signals && STABILITY.signals.length > 0;
  const lastPartial  = BUCKETS.length > 0 && BUCKETS[BUCKETS.length - 1].isPartial;
  const interval     = Math.max(1, Math.floor(BUCKETS.length / CFG.xTickCount));
  const mainYMax     = yDomain(Math.max(...TOTAL_THROUGHPUT, UNPL));
  const mrYMax       = yDomain(Math.max(...(STABILITY.moving_ranges || [0]), MR_UNPL));

  return (
    <div style={{
      background: CFG.COLOR_PAGE_BG, minHeight: "100vh", padding: "32px 24px",
      fontFamily: "'Courier New', monospace", color: CFG.COLOR_TEXT,
    }}>
      <div style={{ maxWidth: 1100, margin: "0 auto" }}>

        {/* breadcrumb */}
        <div style={{ fontSize: 11, color: CFG.COLOR_MUTED, letterSpacing: "0.08em",
          textTransform: "uppercase", marginBottom: 8 }}>
          {PROJECT_KEY} · {BOARD_NAME} · Board {BOARD_ID}
        </div>

        {/* title */}
        <h1 style={{ fontSize: 26, fontWeight: 700, margin: "0 0 4px 0" }}>
          Throughput Stability
        </h1>
        <div style={{ fontSize: 13, color: CFG.COLOR_MUTED, marginBottom: 20 }}>
          {BUCKET_LABEL === "week" ? "Weekly" : "Monthly"} Delivery Volume
          · XmR Process Behavior Chart · {DATE_RANGE}
        </div>

        {/* stat cards */}
        <div style={{ display: "flex", flexWrap: "wrap", gap: 12, marginBottom: 16 }}>
          <StatCard
            label={`X̄ MEAN`}
            value={`${MEAN.toFixed(1)} / ${BUCKET_LABEL}`}
            color={CFG.COLOR_PRIMARY}
          />
          <StatCard
            label="UNPL"
            value={`${UNPL.toFixed(1)} / ${BUCKET_LABEL}`}
            color={CFG.COLOR_ALARM}
          />
          <StatCard
            label="LNPL"
            value={LNPL > 0 ? `${LNPL.toFixed(1)} / ${BUCKET_LABEL}` : "—"}
            color={CFG.COLOR_POSITIVE}
          />
          <StatCard
            label="WINDOW"
            value={`${BUCKETS.length} ${BUCKET_LABEL}s`}
            color={CFG.COLOR_MUTED}
          />
        </div>

        {/* status badges */}
        <div style={{ display: "flex", flexWrap: "wrap", gap: 8, marginBottom: 24, alignItems: "center" }}>
          <Badge
            text={hasSignals ? "⚠ SIGNALS DETECTED" : "✓ STABLE"}
            color={hasSignals ? CFG.COLOR_ALARM : CFG.COLOR_POSITIVE}
          />
          {lastPartial && (
            <Badge
              text={`⚠ ${BUCKETS[BUCKETS.length - 1].label} — partial ${BUCKET_LABEL}`}
              color={CFG.COLOR_CAUTION}
            />
          )}
        </div>

        {/* ── MAIN CHART ── */}
        <div style={{
          background: CFG.COLOR_PANEL_BG, borderRadius: 12,
          border: `1px solid ${CFG.COLOR_BORDER}`, padding: "20px 12px 12px 12px",
          marginBottom: 4,
        }}>
          <ResponsiveContainer width="100%" height={CFG.mainChartHeight}>
            <ComposedChart data={BUCKETS} margin={CFG.mainMargin}>
              <CartesianGrid strokeDasharray="3 3" stroke={CFG.COLOR_BORDER} vertical={false}/>
              <XAxis
                dataKey="label"
                angle={-45} textAnchor="end" height={60}
                interval={interval}
                tick={{ fill: CFG.COLOR_MUTED, fontSize: 11, fontFamily: "'Courier New', monospace" }}
              />
              <YAxis
                domain={[0, mainYMax]}
                tickFormatter={v => v}
                tick={{ fill: CFG.COLOR_MUTED, fontSize: 11, fontFamily: "'Courier New', monospace" }}
                label={{ value: `Items / ${BUCKET_LABEL}`, angle: -90, position: "insideLeft",
                  fill: CFG.COLOR_MUTED, fontSize: 11, fontFamily: "'Courier New', monospace" }}
              />
              <Tooltip content={<CustomTooltip/>}/>

              {/* stacked bars by issue type */}
              {ACTIVE_TYPES.map(t => (
                <Bar key={t} dataKey={t} stackId="tp" fill={TYPE_COLORS[t]} isAnimationActive={false}>
                  {BUCKETS.map((b, i) => (
                    <Cell
                      key={i}
                      fillOpacity={b.isPartial ? CFG.partialOpacity : 1}
                      stroke={b.isSignal && t === ACTIVE_TYPES[ACTIVE_TYPES.length - 1]
                        ? CFG.COLOR_ALARM : "none"}
                      strokeWidth={b.isSignal && t === ACTIVE_TYPES[ACTIVE_TYPES.length - 1] ? 2 : 0}
                    />
                  ))}
                </Bar>
              ))}

              <ReferenceLine y={UNPL} stroke={CFG.COLOR_ALARM} strokeDasharray="6 3" strokeWidth={1.5}
                label={{ value: "UNPL", fill: CFG.COLOR_ALARM, fontSize: 10,
                  fontFamily: "'Courier New', monospace", position: "insideTopLeft" }}/>
              <ReferenceLine y={MEAN} stroke={CFG.COLOR_PRIMARY} strokeDasharray="4 4" strokeWidth={1.5}
                label={{ value: "X̄", fill: CFG.COLOR_PRIMARY, fontSize: 10,
                  fontFamily: "'Courier New', monospace", position: "insideTopLeft" }}/>
              {LNPL > 0 && (
                <ReferenceLine y={LNPL} stroke={CFG.COLOR_POSITIVE} strokeDasharray="6 3" strokeWidth={1}
                  label={{ value: "LNPL", fill: CFG.COLOR_POSITIVE, fontSize: 10,
                    fontFamily: "'Courier New', monospace", position: "insideBottomLeft" }}/>
              )}
            </ComposedChart>
          </ResponsiveContainer>

          {/* legend */}
          <div style={{ display: "flex", flexWrap: "wrap", gap: 16,
            justifyContent: "center", marginTop: 8 }}>
            {ACTIVE_TYPES.map(t => (
              <div key={t} style={{ display: "flex", alignItems: "center", gap: 6 }}>
                <svg width={14} height={12}>
                  <rect x={0} y={0} width={14} height={12} fill={TYPE_COLORS[t]}/>
                </svg>
                <span style={{ fontSize: 11, color: CFG.COLOR_MUTED }}>{t}</span>
              </div>
            ))}
            {[
              { stroke: CFG.COLOR_ALARM,    dash: "6 3", label: "UNPL" },
              { stroke: CFG.COLOR_PRIMARY,  dash: "4 4", label: "X̄ Mean" },
              ...(LNPL > 0
                ? [{ stroke: CFG.COLOR_POSITIVE, dash: "6 3", label: "LNPL" }]
                : []),
            ].map(({ stroke, dash, label }) => (
              <div key={label} style={{ display: "flex", alignItems: "center", gap: 6 }}>
                <svg width={24} height={12}>
                  <line x1={0} y1={6} x2={24} y2={6}
                    stroke={stroke} strokeDasharray={dash} strokeWidth={1.5}/>
                </svg>
                <span style={{ fontSize: 11, color: CFG.COLOR_MUTED }}>{label}</span>
              </div>
            ))}
          </div>
        </div>

        {/* ── MOVING RANGE CHART ── */}
        <div style={{
          background: CFG.COLOR_PANEL_BG, borderRadius: 12,
          border: `1px solid ${CFG.COLOR_BORDER}`, padding: "12px 12px 12px 12px",
          marginBottom: 24,
        }}>
          <div style={{ fontSize: 11, color: CFG.COLOR_MUTED, marginBottom: 8,
            letterSpacing: "0.05em", textTransform: "uppercase" }}>
            Moving Range (week-to-week variation)
          </div>
          <ResponsiveContainer width="100%" height={CFG.mrChartHeight}>
            <ComposedChart data={BUCKETS} margin={CFG.mrMargin}>
              <CartesianGrid strokeDasharray="3 3" stroke={CFG.COLOR_BORDER} vertical={false}/>
              <XAxis
                dataKey="label"
                angle={-45} textAnchor="end" height={60}
                interval={interval}
                tick={{ fill: CFG.COLOR_MUTED, fontSize: 11, fontFamily: "'Courier New', monospace" }}
              />
              <YAxis
                domain={[0, mrYMax]}
                tick={{ fill: CFG.COLOR_MUTED, fontSize: 11, fontFamily: "'Courier New', monospace" }}
                label={{ value: "Moving Range", angle: -90, position: "insideLeft",
                  fill: CFG.COLOR_MUTED, fontSize: 11, fontFamily: "'Courier New', monospace" }}
              />
              <Tooltip content={<CustomTooltip/>}/>
              <Bar dataKey="mr" isAnimationActive={false}>
                {BUCKETS.map((b, i) => (
                  <Cell
                    key={i}
                    fill={b.mr > MR_UNPL ? CFG.COLOR_ALARM : CFG.COLOR_MR_BAR}
                    fillOpacity={b.mr == null ? 0 : 1}
                  />
                ))}
              </Bar>
              <ReferenceLine y={MR_UNPL} stroke={CFG.COLOR_ALARM} strokeDasharray="6 3" strokeWidth={1.5}
                label={{ value: "URL", fill: CFG.COLOR_ALARM, fontSize: 10,
                  fontFamily: "'Courier New', monospace", position: "insideTopLeft" }}/>
              <ReferenceLine y={MR_MEAN} stroke={CFG.COLOR_PRIMARY} strokeDasharray="4 4" strokeWidth={1.5}
                label={{ value: "MR̄", fill: CFG.COLOR_PRIMARY, fontSize: 10,
                  fontFamily: "'Courier New', monospace", position: "insideTopLeft" }}/>
            </ComposedChart>
          </ResponsiveContainer>
        </div>

        {/* footer */}
        <div style={{ fontSize: 11, color: CFG.COLOR_MUTED, lineHeight: 1.7,
          borderTop: `1px solid ${CFG.COLOR_BORDER}`, paddingTop: 16 }}>
          <div style={{ marginBottom: 8 }}>
            <b style={{ color: CFG.COLOR_TEXT }}>Reading this chart:</b> Bars show
            delivery volume per {BUCKET_LABEL}, stacked by issue type. The dashed X̄ line
            is the process mean. UNPL (Upper Natural Process Limit) and LNPL (Lower Natural
            Process Limit) define the expected range of natural variation — bars outside
            these limits are statistical signals indicating a special cause worth
            investigating. The Moving Range panel below shows {BUCKET_LABEL}-to-{BUCKET_LABEL}{" "}
            variation; spikes above the Upper Range Limit (URL) indicate unusually large
            step-changes in delivery rate.
          </div>
          <div>
            <b style={{ color: CFG.COLOR_TEXT }}>Data provenance:</b> Wheeler XmR Process
            Behavior Chart. Limits computed as X̄ ± 2.66 × MR̄ (Natural Process Limits).
            Moving Range Upper Range Limit = 3.267 × MR̄. Partial {BUCKET_LABEL}s (current
            period in progress) are shown at reduced opacity and excluded from limit
            calculations.
          </div>
        </div>

      </div>
    </div>
  );
}
```
