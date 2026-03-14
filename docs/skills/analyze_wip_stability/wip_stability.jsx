import { ComposedChart, Area, Line, ReferenceLine, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer } from "recharts";

// ── INJECTED DATA ─────────────────────────────────────────────────────────────
// These two constants are replaced by Claude via str_replace before delivery.
// Do NOT edit manually.

const MCP_RESPONSE = "__MCP_RESPONSE__";
const CHART_ATTRS  = "__CHART_ATTRS__";

// ── CONFIG ────────────────────────────────────────────────────────────────────

const CFG = {
  chartHeight:  380,
  mainMargin:   { top: 10, right: 20, bottom: 55, left: 10 },
  COLOR_ALARM:     "#ff6b6b",
  COLOR_PRIMARY:   "#6b7de8",
  COLOR_SECONDARY: "#7edde2",
  COLOR_POSITIVE:  "#6bffb8",
  COLOR_TEXT:      "#dde1ef",
  COLOR_MUTED:     "#505878",
  COLOR_PAGE_BG:   "#080a0f",
  COLOR_PANEL_BG:  "#0c0e16",
  COLOR_BORDER:    "#1a1d2e",
};

// ── DERIVED ───────────────────────────────────────────────────────────────────

const ws = MCP_RESPONSE.data.wip_stability;

const BOARD_ID    = CHART_ATTRS.board_id;
const PROJECT_KEY = CHART_ATTRS.project_key;
const BOARD_NAME  = CHART_ATTRS.board_name;

const MEAN   = ws.xmr.average;
const UNPL   = ws.xmr.upper_natural_process_limit;
const LNPL   = ws.xmr.lower_natural_process_limit;
const STATUS = ws.status;

const rawSignals = ws.xmr.signals || [];
const SIGNALS_ABOVE = new Set(
  rawSignals.filter(s => (s.description || "").toLowerCase().includes("above")).map(s => s.key)
);
const SIGNALS_BELOW = new Set(
  rawSignals.filter(s => (s.description || "").toLowerCase().includes("below")).map(s => s.key)
);

const RUN_RAW = ws.run_chart.map(d => [d.date.slice(0, 10), d.count]);

// ── COMPONENTS + RENDER ───────────────────────────────────────────────────────

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

// ── MAIN EXPORT ───────────────────────────────────────────────────────────────

export default function WipStabilityChart() {
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
