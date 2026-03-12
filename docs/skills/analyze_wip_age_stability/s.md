---
name: total-wip-age-stability-chart
description: >
  Creates a dark-themed dual-axis React/Recharts chart for the Total WIP Age Stability
  analysis (mcs-mcp:analyze_wip_age_stability). Trigger on: "Total WIP Age chart/trend/stability",
  "XmR chart for WIP Age", "visualize WIP Age stability", or any "show/chart/plot that"
  follow-up after an analyze_wip_age_stability result is present. ONLY for
  analyze_wip_age_stability output (XmR on the daily sum of all active items' ages).
  Do NOT use for: individual item WIP Age (analyze_work_item_age), WIP Count stability
  (analyze_wip_stability), cycle time stability (analyze_process_stability), or throughput
  (analyze_throughput). Always read this skill before building the chart — do not attempt
  it ad-hoc.
---

# Total WIP Age Stability Chart

Scope: Use this skill ONLY when analyze_wip_age_stability data is present in the
conversation. It visualizes the daily sum of all active items' ages as a Process
Behavior Chart (Wheeler XmR), with WIP Count overlaid on a second axis for context.

Do not use this skill for:
- Individual item WIP Age ranking → use analyze_work_item_age
- WIP Count stability → use analyze_wip_stability
- Cycle time / lead time stability → use analyze_process_stability
- Throughput volume → use analyze_throughput

---

## Prerequisites

Call the tool if data is not yet in the conversation:

  mcs-mcp:analyze_wip_age_stability({ board_id, project_key })
  // Optional: history_window_weeks (default 26) — only affects data volume, not structure

API options note: history_window_weeks is the only parameter. It changes the number
of data points returned but not the response shape. Use 52 for the richest view.

---

## Response Structure

  data.wip_age_stability
    .run_chart[]          — daily data points
      .date               — ISO 8601 with timezone offset e.g. "2025-09-06T00:00:00+02:00"
                            Strip to "YYYY-MM-DD" via .slice(0,10)
      .total_age          — sum of all active items' ages in days (the primary metric)
      .count              — active WIP item count that day (context only)
      .average_age        — mean age per item (provided for convenience, not used in XmR)

    .xmr
      .average                      — process mean X̄ (of total_age)
      .average_moving_range         — MR̄
      .upper_natural_process_limit  — UNPL = X̄ + 2.66 × MR̄
      .lower_natural_process_limit  — LNPL = X̄ − 2.66 × MR̄
      .values[]                     — weekly subgroup values used for XmR
      .moving_ranges[]              — week-to-week absolute differences (length = n-1)
      .signals[]                    — null or array of signal objects:
          .index       — position in values[] array
          .key         — "YYYY-MM-DD" — use this to match signal points on run_chart
          .type        — "outlier"
          .description — e.g. "Total WIP Age above Upper Natural Process Limit (UNPL)"
                          or "Total WIP Age below Lower Natural Process Limit (LNPL)"

    .status               — "stable" or "unstable"

Important: run_chart dates include timezone offsets. Always strip to YYYY-MM-DD
via .slice(0, 10). Signal .key values are already YYYY-MM-DD.

Signal classification:
- description.toLowerCase().includes("above") → SIGNALS_ABOVE (red dots)
- description.toLowerCase().includes("below") → SIGNALS_BELOW (green dots)

---

## JSX Template

Output a single self-contained .jsx artifact with a default export.
Three clearly delimited sections inside the file:

  // ── CONFIG ──         safe to edit: colors, sizes, margins
  // ── DATA (INJECT) ──  replace placeholder values with real MCP response data
  // ── COMPONENTS + RENDER ──  never modify

### Full template

~~~jsx
import {
  ComposedChart, Area, Line, ReferenceLine,
  XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer,
} from "recharts";

// ── CONFIG ──────────────────────────────────────────────────────────────────────────────
const CFG = {
  chartHeight:  420,
  mainMargin:   { top: 10, right: 60, bottom: 60, left: 10 },
  COLOR_ALARM:     "#ff6b6b",   // UNPL line + above-signal dots + UNSTABLE badge
  COLOR_PRIMARY:   "#6b7de8",   // Total WIP Age line + left axis + MEAN stat card
  COLOR_SECONDARY: "#7edde2",   // WIP Count overlay + right axis + COUNT stat card
  COLOR_POSITIVE:  "#6bffb8",   // LNPL line + below-signal dots + STABLE badge
  COLOR_TEXT:      "#dde1ef",
  COLOR_MUTED:     "#505878",   // X̄ reference line + grid
  COLOR_PAGE_BG:   "#080a0f",
  COLOR_PANEL_BG:  "#0c0e16",
  COLOR_BORDER:    "#1a1d2e",
};
// ── END CONFIG ──────────────────────────────────────────────────────────────────────────


// ── DATA (INJECT) ────────────────────────────────────────────────────────────────────────
// Source: mcs-mcp:analyze_wip_age_stability response
// Replace every placeholder below with real values from the MCP response.

const BOARD_ID    = 0;            // board_id parameter used in the call
const PROJECT_KEY = "PROJ";       // project_key parameter used in the call
const BOARD_NAME  = "Board Name"; // board name from context / import_boards

// data.wip_age_stability.xmr.*
const MEAN   = 0;                 // .average
const UNPL   = 0;                 // .upper_natural_process_limit
const LNPL   = 0;                 // .lower_natural_process_limit
const STATUS = "unstable";        // .status — "stable" | "unstable"

// data.wip_age_stability.xmr.signals[] filtered by description
// Above: signals where description.toLowerCase().includes("above") → .key values
const SIGNALS_ABOVE = new Set([
  // "YYYY-MM-DD", ...
]);

// Below: signals where description.toLowerCase().includes("below") → .key values
const SIGNALS_BELOW = new Set([
  // "YYYY-MM-DD", ...
]);

// data.wip_age_stability.run_chart[]
// Each entry: [date.slice(0,10), total_age, count]
const RUN_RAW = [
  // ["YYYY-MM-DD", total_age, count], ...
];
// ── END DATA (INJECT) ────────────────────────────────────────────────────────────────────


// ── COMPONENTS + RENDER (do not edit) ───────────────────────────────────────────────────
const RAW = RUN_RAW.map(([date, total_age, count]) => ({
  date, total_age, count,
  aboveUnpl: SIGNALS_ABOVE.has(date),
  belowLnpl: SIGNALS_BELOW.has(date),
}));

// Downsample: keep every 2nd point; always retain signal points
const CHART_DATA = RAW.filter((d, i) =>
  RAW.length <= 100 || i % 2 === 0 || d.aboveUnpl || d.belowLnpl
);

// Y-axis domains
const maxAge  = Math.max(...RAW.map(d => d.total_age), UNPL);
const Y_L_MAX = Math.ceil((maxAge + 400) / 500) * 500;
const Y_L_MIN = Math.floor((LNPL - 600) / 500) * 500;
const maxCount = Math.max(...RAW.map(d => d.count));
const minCount = Math.min(...RAW.map(d => d.count));
const Y_R_MAX = Math.ceil((maxCount + 5) / 10) * 10;
const Y_R_MIN = Math.floor((minCount - 5) / 10) * 10;

function groupConsecutive(dates) {
  const sorted = [...dates].sort();
  if (!sorted.length) return [];
  const groups = [];
  let start = sorted[0], prev = sorted[0];
  for (let i = 1; i < sorted.length; i++) {
    const diff = (new Date(sorted[i]) - new Date(prev)) / 86400000;
    if (diff <= 2) { prev = sorted[i]; }
    else { groups.push([start, prev]); start = sorted[i]; prev = sorted[i]; }
  }
  groups.push([start, prev]);
  return groups;
}

const fmtS = iso =>
  new Date(iso + "T00:00:00").toLocaleDateString("en-GB", { day: "2-digit", month: "short", year: "2-digit" });
const fmtR = ([s, e]) => s === e ? fmtS(s) : `${fmtS(s)} – ${fmtS(e)}`;
const ABOVE_GROUPS = groupConsecutive([...SIGNALS_ABOVE]);
const BELOW_GROUPS = groupConsecutive([...SIGNALS_BELOW]);

const AgeDot = ({ cx, cy, payload }) => {
  if (!cx || !cy) return null;
  if (payload.aboveUnpl)
    return <circle cx={cx} cy={cy} r={5} fill={CFG.COLOR_ALARM}    stroke={CFG.COLOR_PAGE_BG} strokeWidth={1.5} />;
  if (payload.belowLnpl)
    return <circle cx={cx} cy={cy} r={5} fill={CFG.COLOR_POSITIVE} stroke={CFG.COLOR_PAGE_BG} strokeWidth={1.5} />;
  return null;
};

const TT = ({ active, payload }) => {
  if (!active || !payload?.length) return null;
  const d = payload[0].payload;
  const fmtFull = iso =>
    new Date(iso + "T00:00:00").toLocaleDateString("en-GB", { day: "2-digit", month: "short", year: "numeric" });
  return (
    <div style={{ background: "#0f1117", border: `1px solid ${CFG.COLOR_BORDER}`, borderRadius: 8,
        padding: "10px 14px", fontFamily: "'Courier New', monospace", fontSize: 12, color: CFG.COLOR_TEXT }}>
      <div style={{ fontWeight: 700, marginBottom: 6 }}>{fmtFull(d.date)}</div>
      <div style={{ color: CFG.COLOR_PRIMARY }}>Total WIP Age: <b>{d.total_age.toFixed(1)} days</b></div>
      <div style={{ color: CFG.COLOR_SECONDARY, marginTop: 2 }}>WIP Count: <b>{d.count}</b></div>
      <div style={{ borderTop: `1px solid ${CFG.COLOR_BORDER}`, marginTop: 6, paddingTop: 6,
          color: CFG.COLOR_MUTED, fontSize: 11 }}>
        X̄: {MEAN.toFixed(0)} · UNPL: {UNPL.toFixed(0)} · LNPL: {LNPL.toFixed(0)}
      </div>
      {d.aboveUnpl && <div style={{ color: CFG.COLOR_ALARM,    fontWeight: 700, marginTop: 4 }}>⚠ Signal: above UNPL</div>}
      {d.belowLnpl && <div style={{ color: CFG.COLOR_POSITIVE, fontWeight: 700, marginTop: 4 }}>↓ Signal: below LNPL</div>}
    </div>
  );
};

const Card = ({ label, value, color }) => (
  <div style={{ background: CFG.COLOR_PANEL_BG, border: `1px solid ${color}33`,
      borderRadius: 8, padding: "8px 14px", minWidth: 110 }}>
    <div style={{ fontSize: 10, color: CFG.COLOR_MUTED, marginBottom: 3, letterSpacing: "0.05em" }}>{label}</div>
    <div style={{ fontSize: 18, fontWeight: 700, color }}>{value}</div>
  </div>
);

const Badge = ({ text, color }) => (
  <span style={{ fontSize: 11, padding: "3px 8px", borderRadius: 4,
      background: `${color}15`, border: `1px solid ${color}40`, color,
      fontFamily: "'Courier New', monospace" }}>{text}</span>
);

export default function App() {
  const fmtX = iso =>
    new Date(iso + "T00:00:00").toLocaleDateString("en-GB", { day: "2-digit", month: "short" });
  const interval    = Math.max(1, Math.floor(CHART_DATA.length / 9));
  const latest      = RAW[RAW.length - 1];
  const statusColor = STATUS === "stable" ? CFG.COLOR_POSITIVE : CFG.COLOR_ALARM;

  return (
    <div style={{ background: CFG.COLOR_PAGE_BG, minHeight: "100vh", padding: "24px 20px",
        fontFamily: "'Courier New', monospace", color: CFG.COLOR_TEXT }}>
      <div style={{ maxWidth: 1100, margin: "0 auto" }}>

        <div style={{ fontSize: 11, color: CFG.COLOR_MUTED, letterSpacing: "0.08em",
            textTransform: "uppercase", marginBottom: 6 }}>
          {PROJECT_KEY} · {BOARD_NAME} · Board {BOARD_ID}
        </div>
        <h1 style={{ fontSize: 22, fontWeight: 700, margin: "0 0 4px" }}>Total WIP Age Stability</h1>
        <div style={{ fontSize: 12, color: CFG.COLOR_MUTED, marginBottom: 16 }}>
          Cumulative Age Burden · XmR Process Behavior Chart
          · {RAW[0]?.date} – {RAW[RAW.length - 1]?.date}
        </div>

        <div style={{ display: "flex", flexWrap: "wrap", gap: 10, marginBottom: 14 }}>
          <Card label="X̄ MEAN (days)"  value={MEAN.toFixed(0)}              color={CFG.COLOR_PRIMARY}   />
          <Card label="UNPL (days)"     value={UNPL.toFixed(0)}              color={CFG.COLOR_ALARM}     />
          <Card label="LNPL (days)"     value={LNPL.toFixed(0)}              color={CFG.COLOR_POSITIVE}  />
          <Card label="TODAY (days)"    value={latest?.total_age.toFixed(0)} color={CFG.COLOR_PRIMARY}   />
          <Card label="TODAY WIP COUNT" value={latest?.count}                color={CFG.COLOR_SECONDARY} />
        </div>

        <div style={{ display: "flex", flexWrap: "wrap", gap: 6, marginBottom: 20, alignItems: "center" }}>
          <Badge text={STATUS === "stable" ? "✓ STATUS: STABLE" : "⚠ STATUS: UNSTABLE"} color={statusColor} />
          {ABOVE_GROUPS.map((g, i) =>
            <Badge key={"a" + i} text={`↑ Above UNPL · ${fmtR(g)}`} color={CFG.COLOR_ALARM} />)}
          {BELOW_GROUPS.map((g, i) =>
            <Badge key={"b" + i} text={`↓ Below LNPL · ${fmtR(g)}`} color={CFG.COLOR_POSITIVE} />)}
        </div>

        <div style={{ background: CFG.COLOR_PANEL_BG, borderRadius: 12,
            border: `1px solid ${CFG.COLOR_BORDER}`, padding: "16px 8px 8px" }}>
          <ResponsiveContainer width="100%" height={CFG.chartHeight}>
            <ComposedChart data={CHART_DATA} margin={CFG.mainMargin}>
              <defs>
                <linearGradient id="ageGrad" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="5%"  stopColor={CFG.COLOR_PRIMARY} stopOpacity={0.15} />
                  <stop offset="95%" stopColor={CFG.COLOR_PRIMARY} stopOpacity={0.01} />
                </linearGradient>
              </defs>
              <CartesianGrid strokeDasharray="3 3" stroke={CFG.COLOR_BORDER} vertical={false} />
              <XAxis dataKey="date" tickFormatter={fmtX} interval={interval} angle={-45}
                textAnchor="end" height={60}
                tick={{ fill: CFG.COLOR_MUTED, fontSize: 10, fontFamily: "'Courier New', monospace" }} />
              <YAxis yAxisId="age" orientation="left" domain={[Y_L_MIN, Y_L_MAX]}
                tick={{ fill: CFG.COLOR_PRIMARY, fontSize: 10, fontFamily: "'Courier New', monospace" }}
                label={{ value: "Total WIP Age (days)", angle: -90, position: "insideLeft",
                  fill: CFG.COLOR_PRIMARY, fontSize: 10, dy: 70 }} />
              <YAxis yAxisId="count" orientation="right" domain={[Y_R_MIN, Y_R_MAX]}
                tick={{ fill: CFG.COLOR_SECONDARY, fontSize: 10, fontFamily: "'Courier New', monospace" }}
                label={{ value: "WIP Count", angle: 90, position: "insideRight",
                  fill: CFG.COLOR_SECONDARY, fontSize: 10, dy: -40 }} />
              <Tooltip content={<TT />} />
              <Area yAxisId="age" dataKey="total_age" fill="url(#ageGrad)"
                stroke="none" dot={false} activeDot={false} isAnimationActive={false} />
              <Line yAxisId="age"   dataKey="total_age" stroke={CFG.COLOR_PRIMARY}   strokeWidth={1.5}
                dot={<AgeDot />} activeDot={false} isAnimationActive={false} />
              <Line yAxisId="count" dataKey="count"     stroke={CFG.COLOR_SECONDARY} strokeWidth={1}
                strokeDasharray="3 3" dot={false} activeDot={false} isAnimationActive={false} />
              <ReferenceLine yAxisId="age" y={UNPL} stroke={CFG.COLOR_ALARM}    strokeDasharray="6 3"
                strokeWidth={1.5} label={{ value: "UNPL", fill: CFG.COLOR_ALARM,    fontSize: 10, position: "insideTopRight" }} />
              <ReferenceLine yAxisId="age" y={MEAN} stroke={CFG.COLOR_MUTED}    strokeDasharray="4 4"
                strokeWidth={1.5} label={{ value: "X̄",   fill: CFG.COLOR_MUTED,    fontSize: 10, position: "insideTopRight" }} />
              <ReferenceLine yAxisId="age" y={LNPL} stroke={CFG.COLOR_POSITIVE} strokeDasharray="6 3"
                strokeWidth={1}   label={{ value: "LNPL", fill: CFG.COLOR_POSITIVE, fontSize: 10, position: "insideBottomRight" }} />
            </ComposedChart>
          </ResponsiveContainer>

          <div style={{ display: "flex", flexWrap: "wrap", gap: 14, justifyContent: "center", marginTop: 8 }}>
            {[
              [<line x1={0} y1={6} x2={24} y2={6} stroke={CFG.COLOR_PRIMARY}   strokeWidth={1.5} />,                      "Total WIP Age"],
              [<line x1={0} y1={6} x2={24} y2={6} stroke={CFG.COLOR_SECONDARY} strokeDasharray="3 3" strokeWidth={1} />,  "WIP Count (right axis)"],
              [<line x1={0} y1={6} x2={24} y2={6} stroke={CFG.COLOR_ALARM}    strokeDasharray="6 3" strokeWidth={1.5} />, "UNPL"],
              [<line x1={0} y1={6} x2={24} y2={6} stroke={CFG.COLOR_MUTED}    strokeDasharray="4 4" strokeWidth={1.5} />, "X̄ Mean"],
              [<line x1={0} y1={6} x2={24} y2={6} stroke={CFG.COLOR_POSITIVE} strokeDasharray="6 3" strokeWidth={1} />,   "LNPL"],
              [<circle cx={6} cy={6} r={4} fill={CFG.COLOR_ALARM} />,                                                      "Above UNPL"],
              [<circle cx={6} cy={6} r={4} fill={CFG.COLOR_POSITIVE} />,                                                   "Below LNPL"],
            ].map(([el, label]) => (
              <div key={label} style={{ display: "flex", alignItems: "center", gap: 5 }}>
                <svg width={24} height={12}>{el}</svg>
                <span style={{ fontSize: 10, color: CFG.COLOR_MUTED }}>{label}</span>
              </div>
            ))}
          </div>
        </div>

        <div style={{ fontSize: 11, color: CFG.COLOR_MUTED, lineHeight: 1.7,
            borderTop: `1px solid ${CFG.COLOR_BORDER}`, paddingTop: 14, marginTop: 16 }}>
          <div style={{ marginBottom: 6 }}>
            <b style={{ color: CFG.COLOR_TEXT }}>Reading this chart: </b>
            Total WIP Age is the sum of all active items ages — the cumulative age burden
            on the system. Unlike WIP Count, it detects stagnation: even a stable item
            count can hide growing backlogs if items are not moving. XmR limits are applied
            directly to Total WIP Age, making signals robust and assumption-free. The dashed
            cyan overlay shows WIP Count on the right axis for context.
          </div>
          <div>
            <b style={{ color: CFG.COLOR_TEXT }}>Data provenance: </b>
            Wheeler XmR Process Behavior Chart applied to weekly Total WIP Age subgroup
            values. Limits: X̄ ± 2.66 × MR̄. Signal dots mark UNPL/LNPL breaches
            (red = above UNPL, green = below LNPL). Chart downsampled to every 2nd point;
            all signal points are always retained.
          </div>
        </div>

      </div>
    </div>
  );
}
// ── END COMPONENTS + RENDER ─────────────────────────────────────────────────────────────
~~~

---

## Injection Checklist

When populating DATA (INJECT) from a real analyze_wip_age_stability response:

| Placeholder    | Source path |
|----------------|-------------|
| BOARD_ID       | board_id parameter used in the call |
| PROJECT_KEY    | project_key parameter used in the call |
| BOARD_NAME     | board name from context / import_boards |
| MEAN           | data.wip_age_stability.xmr.average |
| UNPL           | data.wip_age_stability.xmr.upper_natural_process_limit |
| LNPL           | data.wip_age_stability.xmr.lower_natural_process_limit |
| STATUS         | data.wip_age_stability.status |
| SIGNALS_ABOVE  | signals where description includes "above" → .key values |
| SIGNALS_BELOW  | signals where description includes "below" → .key values |
| RUN_RAW        | run_chart[] → [date.slice(0,10), total_age, count] tuples |

---

## Key Differences from WIP Count Stability Chart

| Aspect           | analyze_wip_stability        | analyze_wip_age_stability         |
|------------------|------------------------------|-----------------------------------|
| Primary metric   | count (integer)              | total_age (float, days)           |
| Y-axis           | Single axis                  | Dual axis (age left, count right) |
| Count series     | Is the primary metric        | Dashed overlay on right axis      |
| RUN_RAW tuple    | [date, count]                | [date, total_age, count]          |
| Y-axis label     | "Active WIP (items)"         | "Total WIP Age (days)"            |
| Gradient color   | COLOR_SECONDARY (cyan)       | COLOR_PRIMARY (indigo)            |

---

## Checklist Before Delivering

- Triggered by analyze_wip_age_stability data (not WIP Count, cycle time, or throughput)
- Chart title reads exactly "Total WIP Age Stability"
- Dual Y-axis: Total WIP Age on left (indigo), WIP Count on right (cyan)
- Area for gradient fill under age line — stroke="none", no dots
- Line yAxisId="age" for Total WIP Age + signal dots
- Line yAxisId="count" for WIP Count — dashed, no dots, right axis
- Signal dots: red for above UNPL, green for below LNPL, null otherwise
- UNPL, MEAN, LNPL reference lines all on yAxisId="age"
- Date strings stripped to YYYY-MM-DD before signal key matching
- Status badge: STABLE (green) or UNSTABLE (red)
- Signal badges with date ranges for both above and below groups
- Stat cards: X̄, UNPL, LNPL, Today's total age, Today's WIP Count
- Manual legend below chart panel (no Recharts Legend component)
- Downsampled: every 2nd point; all signal points always retained
- Dark theme throughout (#080a0f page, #0c0e16 panel, #1a1d2e grid)
- Monospace font throughout
- Single self-contained .jsx file with default export
