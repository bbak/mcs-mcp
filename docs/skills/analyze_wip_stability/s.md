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
// Optional: history_window_weeks (default 26) — only affects data volume, not structure
```

**API options note:** `history_window_weeks` is the only parameter that affects output.
It changes the number of data points returned but not the response shape.

---

## Response Structure

```
data.wip_stability
  .run_chart[]          — daily data points
    .date               — ISO 8601 with timezone offset e.g. "2025-09-06T00:00:00+02:00"
                          Strip to "YYYY-MM-DD" via .slice(0,10) for display and signal matching
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
        .key         — "YYYY-MM-DD" — use this to match signal points on the run chart
        .type        — "outlier"
        .description — human-readable e.g. "WIP count above Upper Natural Process Limit"

  .status               — "stable" or "unstable"
```

**Important:** `run_chart` dates include timezone offsets. Always strip to `YYYY-MM-DD`
via `.slice(0, 10)` before use. Signal `.key` values are already `YYYY-MM-DD`.

---

## JSX Template

Output a single self-contained `.jsx` artifact with a default export.
The file has three clearly delimited sections:

```
// ── CONFIG ──         safe to edit: colors, sizes, margins
// ── DATA (INJECT) ──  replace placeholder values with real data from the MCP response
// ── COMPONENTS + RENDER ──  never modify
```

### Full template

```jsx
import { ComposedChart, Area, Line, ReferenceLine, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer } from "recharts";

// ── CONFIG ──────────────────────────────────────────────────────────────────────────────
const CFG = {
  chartHeight:  380,
  mainMargin:   { top: 10, right: 20, bottom: 55, left: 10 },
  COLOR_ALARM:     "#ff6b6b",   // UNPL line + above-signal dots + UNSTABLE badge
  COLOR_PRIMARY:   "#6b7de8",   // X̄ mean reference line + MEAN stat card
  COLOR_SECONDARY: "#7edde2",   // WIP count line + TODAY stat card
  COLOR_POSITIVE:  "#6bffb8",   // LNPL line + below-signal dots + STABLE badge
  COLOR_TEXT:      "#dde1ef",
  COLOR_MUTED:     "#505878",
  COLOR_PAGE_BG:   "#080a0f",
  COLOR_PANEL_BG:  "#0c0e16",
  COLOR_BORDER:    "#1a1d2e",
};
// ── END CONFIG ──────────────────────────────────────────────────────────────────────────


// ── DATA (INJECT) ────────────────────────────────────────────────────────────────────────
// Source: mcs-mcp:analyze_wip_stability response
// Replace every placeholder below with real values extracted from the MCP response.

const BOARD_ID    = 0;                        // data source: board_id parameter
const PROJECT_KEY = "PROJ";                   // data source: project_key parameter
const BOARD_NAME  = "Board Name";             // data source: board name from context

// data.wip_stability.xmr.*
const MEAN   = 0;                             // .average
const UNPL   = 0;                             // .upper_natural_process_limit
const LNPL   = 0;                             // .lower_natural_process_limit
const STATUS = "unstable";                    // .status — "stable" | "unstable"

// data.wip_stability.xmr.signals[] filtered by description
// Use: (signals || []).filter(s => s.description.toLowerCase().includes("above")).map(s => s.key)
const SIGNALS_ABOVE = new Set([
  // "YYYY-MM-DD", ...
]);

// Use: (signals || []).filter(s => s.description.toLowerCase().includes("below")).map(s => s.key)
const SIGNALS_BELOW = new Set([
  // "YYYY-MM-DD", ...
]);

// data.wip_stability.run_chart[] — strip timezone: d.date.slice(0,10)
const RUN_RAW = [
  // ["YYYY-MM-DD", count], ...
];
// ── END DATA (INJECT) ────────────────────────────────────────────────────────────────────


// ── COMPONENTS + RENDER (do not edit) ───────────────────────────────────────────────────
const DATA = RUN_RAW.map(([date, count]) => ({
  date, count,
  aboveUnpl: SIGNALS_ABOVE.has(date),
  belowLnpl: SIGNALS_BELOW.has(date),
}));

// Downsample: keep every 2nd point; always retain signal points
const CHART_DATA = DATA.filter((d, i) =>
  DATA.length <= 100 || i % 2 === 0 || d.aboveUnpl || d.belowLnpl
);

const Y_MIN = Math.floor((LNPL - 5) / 5) * 5;
const Y_MAX = Math.ceil((Math.max(...DATA.map(d => d.count), UNPL) + 5) / 5) * 5;

// Group consecutive signal dates into ranges for badge display
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

const fmtShort = iso =>
  new Date(iso + "T00:00:00").toLocaleDateString("en-GB", { day: "2-digit", month: "short", year: "2-digit" });
const fmtRange = ([s, e]) => s === e ? fmtShort(s) : `${fmtShort(s)} – ${fmtShort(e)}`;
const ABOVE_GROUPS = groupConsecutive([...SIGNALS_ABOVE]);
const BELOW_GROUPS = groupConsecutive([...SIGNALS_BELOW]);

const Dot = ({ cx, cy, payload }) => {
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
      <div style={{ color: CFG.COLOR_SECONDARY }}>WIP Count: <b>{d.count}</b></div>
      <div style={{ borderTop: `1px solid ${CFG.COLOR_BORDER}`, marginTop: 6, paddingTop: 6,
          color: CFG.COLOR_MUTED, fontSize: 11 }}>
        X̄ {MEAN.toFixed(1)} · UNPL {UNPL.toFixed(1)} · LNPL {LNPL.toFixed(1)}
      </div>
      {d.aboveUnpl && <div style={{ color: CFG.COLOR_ALARM,    fontWeight: 700, marginTop: 4 }}>⚠ Signal: above UNPL</div>}
      {d.belowLnpl && <div style={{ color: CFG.COLOR_POSITIVE, fontWeight: 700, marginTop: 4 }}>↓ Signal: below LNPL</div>}
    </div>
  );
};

const Card = ({ label, value, color }) => (
  <div style={{ background: CFG.COLOR_PANEL_BG, border: `1px solid ${color}33`,
      borderRadius: 8, padding: "8px 14px", minWidth: 90 }}>
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
  const interval = Math.max(1, Math.floor(CHART_DATA.length / 8));
  const statusColor = STATUS === "stable" ? CFG.COLOR_POSITIVE : CFG.COLOR_ALARM;
  const statusLabel = STATUS === "stable" ? "✓ STATUS: STABLE" : "⚠ STATUS: UNSTABLE";

  return (
    <div style={{ background: CFG.COLOR_PAGE_BG, minHeight: "100vh", padding: "24px 20px",
        fontFamily: "'Courier New', monospace", color: CFG.COLOR_TEXT }}>

      {/* Header */}
      <div style={{ fontSize: 11, color: CFG.COLOR_MUTED, letterSpacing: "0.08em",
          textTransform: "uppercase", marginBottom: 6 }}>
        {PROJECT_KEY} · {BOARD_NAME} · Board {BOARD_ID}
      </div>
      <h1 style={{ fontSize: 22, fontWeight: 700, margin: "0 0 4px" }}>WIP Count Stability</h1>
      <div style={{ fontSize: 12, color: CFG.COLOR_MUTED, marginBottom: 16 }}>
        Daily Active WIP · XmR Process Behavior Chart
        · {DATA[0]?.date} – {DATA[DATA.length - 1]?.date}
      </div>

      {/* Stat cards */}
      <div style={{ display: "flex", flexWrap: "wrap", gap: 10, marginBottom: 14 }}>
        <Card label="X̄ MEAN" value={MEAN.toFixed(1)}              color={CFG.COLOR_PRIMARY}   />
        <Card label="UNPL"   value={UNPL.toFixed(1)}              color={CFG.COLOR_ALARM}     />
        <Card label="LNPL"   value={LNPL.toFixed(1)}              color={CFG.COLOR_POSITIVE}  />
        <Card label="TODAY"  value={DATA[DATA.length - 1]?.count} color={CFG.COLOR_SECONDARY} />
      </div>

      {/* Status + signal badges */}
      <div style={{ display: "flex", flexWrap: "wrap", gap: 6, marginBottom: 20, alignItems: "center" }}>
        <Badge text={statusLabel} color={statusColor} />
        {ABOVE_GROUPS.map((g, i) =>
          <Badge key={"a" + i} text={`↑ Above UNPL · ${fmtRange(g)}`} color={CFG.COLOR_ALARM} />)}
        {BELOW_GROUPS.map((g, i) =>
          <Badge key={"b" + i} text={`↓ Below LNPL · ${fmtRange(g)}`} color={CFG.COLOR_POSITIVE} />)}
      </div>

      {/* Chart panel */}
      <div style={{ background: CFG.COLOR_PANEL_BG, borderRadius: 12,
          border: `1px solid ${CFG.COLOR_BORDER}`, padding: "16px 8px 8px" }}>
        <ResponsiveContainer width="100%" height={CFG.chartHeight}>
          <ComposedChart data={CHART_DATA} margin={CFG.mainMargin}>
            <defs>
              <linearGradient id="wipGrad" x1="0" y1="0" x2="0" y2="1">
                <stop offset="5%"  stopColor={CFG.COLOR_SECONDARY} stopOpacity={0.15} />
                <stop offset="95%" stopColor={CFG.COLOR_SECONDARY} stopOpacity={0.01} />
              </linearGradient>
            </defs>
            <CartesianGrid strokeDasharray="3 3" stroke={CFG.COLOR_BORDER} vertical={false} />
            <XAxis dataKey="date" tickFormatter={fmtX} interval={interval} angle={-45}
              textAnchor="end" height={60}
              tick={{ fill: CFG.COLOR_MUTED, fontSize: 10, fontFamily: "'Courier New', monospace" }} />
            <YAxis domain={[Y_MIN, Y_MAX]}
              tick={{ fill: CFG.COLOR_MUTED, fontSize: 10, fontFamily: "'Courier New', monospace" }}
              label={{ value: "Active WIP (items)", angle: -90, position: "insideLeft",
                fill: CFG.COLOR_MUTED, fontSize: 10, dy: 56 }} />
            <Tooltip content={<TT />} />
            <Area dataKey="count" fill="url(#wipGrad)" stroke="none"
              dot={false} activeDot={false} isAnimationActive={false} />
            <Line dataKey="count" stroke={CFG.COLOR_SECONDARY} strokeWidth={1.5}
              dot={<Dot />} activeDot={false} isAnimationActive={false} />
            <ReferenceLine y={UNPL} stroke={CFG.COLOR_ALARM}    strokeDasharray="6 3" strokeWidth={1.5}
              label={{ value: "UNPL", fill: CFG.COLOR_ALARM,    fontSize: 10, position: "insideTopRight" }} />
            <ReferenceLine y={MEAN} stroke={CFG.COLOR_PRIMARY}  strokeDasharray="4 4" strokeWidth={1.5}
              label={{ value: "X̄",   fill: CFG.COLOR_PRIMARY,  fontSize: 10, position: "insideTopRight" }} />
            <ReferenceLine y={LNPL} stroke={CFG.COLOR_POSITIVE} strokeDasharray="6 3" strokeWidth={1}
              label={{ value: "LNPL", fill: CFG.COLOR_POSITIVE, fontSize: 10, position: "insideBottomRight" }} />
          </ComposedChart>
        </ResponsiveContainer>

        {/* Legend */}
        <div style={{ display: "flex", flexWrap: "wrap", gap: 14, justifyContent: "center", marginTop: 6 }}>
          {[
            [<line x1={0} y1={6} x2={24} y2={6} stroke={CFG.COLOR_SECONDARY} strokeWidth={1.5} />,                           "Daily WIP"],
            [<line x1={0} y1={6} x2={24} y2={6} stroke={CFG.COLOR_ALARM}    strokeDasharray="6 3" strokeWidth={1.5} />,       "UNPL"],
            [<line x1={0} y1={6} x2={24} y2={6} stroke={CFG.COLOR_PRIMARY}  strokeDasharray="4 4" strokeWidth={1.5} />,       "X̄ Mean"],
            [<line x1={0} y1={6} x2={24} y2={6} stroke={CFG.COLOR_POSITIVE} strokeDasharray="6 3" strokeWidth={1} />,         "LNPL"],
            [<circle cx={6} cy={6} r={4} fill={CFG.COLOR_ALARM} />,                                                            "Above UNPL"],
            [<circle cx={6} cy={6} r={4} fill={CFG.COLOR_POSITIVE} />,                                                         "Below LNPL"],
          ].map(([el, label]) => (
            <div key={label} style={{ display: "flex", alignItems: "center", gap: 5 }}>
              <svg width={24} height={12}>{el}</svg>
              <span style={{ fontSize: 10, color: CFG.COLOR_MUTED }}>{label}</span>
            </div>
          ))}
        </div>
      </div>

      {/* Footer */}
      <div style={{ fontSize: 11, color: CFG.COLOR_MUTED, lineHeight: 1.7,
          borderTop: `1px solid ${CFG.COLOR_BORDER}`, paddingTop: 14, marginTop: 16 }}>
        <div style={{ marginBottom: 6 }}>
          <b style={{ color: CFG.COLOR_TEXT }}>Reading this chart: </b>
          The line shows the daily count of active in-progress items (past the commitment
          point, not yet finished). The dashed X̄ is the process mean. UNPL and LNPL define
          the expected range of natural variation — points outside these limits are statistical
          signals. Uncontrolled WIP violates the assumptions of Little's Law, making cycle
          time predictions fundamentally unreliable.
        </div>
        <div>
          <b style={{ color: CFG.COLOR_TEXT }}>Data provenance: </b>
          Wheeler XmR Process Behavior Chart applied to weekly WIP subgroup averages.
          Limits: X̄ ± 2.66 × MR̄. Colored dots mark signal breaches (red = above UNPL,
          green = below LNPL). Chart downsampled to every 2nd point for readability;
          all signal points are always retained.
        </div>
      </div>

    </div>
  );
}
// ── END COMPONENTS + RENDER ─────────────────────────────────────────────────────────────
```

---

## Injection Checklist

When populating DATA (INJECT) from a real `analyze_wip_stability` response:

| Placeholder | Source path |
|---|---|
| `BOARD_ID` | board_id parameter used in the call |
| `PROJECT_KEY` | project_key parameter used in the call |
| `BOARD_NAME` | board name from context / import_boards |
| `MEAN` | `data.wip_stability.xmr.average` |
| `UNPL` | `data.wip_stability.xmr.upper_natural_process_limit` |
| `LNPL` | `data.wip_stability.xmr.lower_natural_process_limit` |
| `STATUS` | `data.wip_stability.status` |
| `SIGNALS_ABOVE` | signals where description includes "above" → `.key` values |
| `SIGNALS_BELOW` | signals where description includes "below" → `.key` values |
| `RUN_RAW` | `data.wip_stability.run_chart[]` → `[date.slice(0,10), count]` tuples |

---

## Checklist Before Delivering

- [ ] Triggered by `analyze_wip_stability` data (not WIP Age, cycle time, or throughput)
- [ ] Chart title reads exactly **"WIP Count Stability"**
- [ ] Single Y-axis only (no dual axis)
- [ ] `<Area>` for gradient fill only — `stroke="none"`, no dots
- [ ] `<Line>` for actual line + signal dots via `<Dot>`
- [ ] Signal dots: red for above UNPL, green for below LNPL, null otherwise
- [ ] Date strings stripped to `YYYY-MM-DD` before signal key matching
- [ ] UNPL, MEAN, LNPL all present as reference lines
- [ ] Status badge: STABLE (green) or UNSTABLE (red)
- [ ] Signal badges with date ranges
- [ ] Stat cards: X̄, UNPL, LNPL, Today's WIP count
- [ ] Manual legend below chart panel (no Recharts `<Legend>` component)
- [ ] Downsampled: every 2nd point; all signal points always retained
- [ ] Dark theme throughout (`#080a0f` page, `#0c0e16` panel, `#1a1d2e` grid)
- [ ] Monospace font throughout
- [ ] Single self-contained `.jsx` file with default export
