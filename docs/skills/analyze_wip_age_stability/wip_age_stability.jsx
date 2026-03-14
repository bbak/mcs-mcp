import {
  ComposedChart, Area, Line, ReferenceLine,
  XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer,
} from "recharts";

// ── INJECTED DATA ─────────────────────────────────────────────────────────────
// These two constants are replaced by Claude via str_replace before delivery.
// Do NOT edit manually.

const MCP_RESPONSE = "__MCP_RESPONSE__";
const CHART_ATTRS  = "__CHART_ATTRS__";

// ── CONFIG ────────────────────────────────────────────────────────────────────

const CFG = {
  chartHeight:  420,
  mainMargin:   { top: 10, right: 60, bottom: 60, left: 10 },
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

const wa = MCP_RESPONSE.data.wip_age_stability;

const BOARD_ID    = CHART_ATTRS.board_id;
const PROJECT_KEY = CHART_ATTRS.project_key;
const BOARD_NAME  = CHART_ATTRS.board_name;

const MEAN   = wa.xmr.average;
const UNPL   = wa.xmr.upper_natural_process_limit;
const LNPL   = wa.xmr.lower_natural_process_limit;
const STATUS = wa.status;

const rawSignals = wa.xmr.signals || [];
const SIGNALS_ABOVE = new Set(
  rawSignals.filter(s => (s.description || "").toLowerCase().includes("above")).map(s => s.key)
);
const SIGNALS_BELOW = new Set(
  rawSignals.filter(s => (s.description || "").toLowerCase().includes("below")).map(s => s.key)
);

const RUN_RAW = wa.run_chart.map(d => [d.date.slice(0, 10), d.total_age, d.count]);

// ── COMPONENTS + RENDER ───────────────────────────────────────────────────────

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

// ── MAIN EXPORT ───────────────────────────────────────────────────────────────

export default function WipAgeStabilityChart() {
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
