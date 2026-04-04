import {
  ComposedChart, Area, Line, ReferenceLine,
  XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer,
} from "recharts";
import { ALARM, PRIMARY, SECONDARY, POSITIVE, TEXT, MUTED, PAGE_BG, PANEL_BG, BORDER, XMR_UNPL, XMR_MEAN, XMR_LNPL, FONT_STACK } from "mcs-mcp";
import { StatCard, Badge, TOOLTIP_BG } from "./shared.jsx";

// ── INJECTED DATA ─────────────────────────────────────────────────────────────
// Payload is injected by the MCS chart renderer as window.__MCS_PAYLOAD__.

const __MCS_ENVELOPE__ = window.__MCS_PAYLOAD__;
const __MCS_DATA__ = __MCS_ENVELOPE__.data;
const __MCS_GUARDRAILS__ = __MCS_ENVELOPE__.guardrails;
const __MCS_WORKFLOW__ = __MCS_ENVELOPE__.workflow;
// ── CONFIG ────────────────────────────────────────────────────────────────────

const CFG = {
  chartHeight:  420,
  mainMargin:   { top: 10, right: 60, bottom: 60, left: 10 },
};

// ── DERIVED ───────────────────────────────────────────────────────────────────

const wa = __MCS_DATA__.wip_age_stability;

const BOARD_ID    = __MCS_WORKFLOW__.board_id;
const PROJECT_KEY = __MCS_WORKFLOW__.project_key;
const BOARD_NAME  = __MCS_WORKFLOW__.board_name;

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
    return <circle cx={cx} cy={cy} r={5} fill={ALARM}    stroke={PAGE_BG} strokeWidth={1.5} />;
  if (payload.belowLnpl)
    return <circle cx={cx} cy={cy} r={5} fill={POSITIVE} stroke={PAGE_BG} strokeWidth={1.5} />;
  return null;
};

const TT = ({ active, payload }) => {
  if (!active || !payload?.length) return null;
  const d = payload[0].payload;
  const fmtFull = iso =>
    new Date(iso + "T00:00:00").toLocaleDateString("en-GB", { day: "2-digit", month: "short", year: "numeric" });
  return (
    <div style={{ background: TOOLTIP_BG, border: `1px solid ${BORDER}`, borderRadius: 8,
        padding: "10px 14px", fontFamily: FONT_STACK, fontSize: 12, color: TEXT }}>
      <div style={{ fontWeight: 700, marginBottom: 6 }}>{fmtFull(d.date)}</div>
      <div style={{ color: PRIMARY }}>Total WIP Age: <b>{d.total_age.toFixed(1)} days</b></div>
      <div style={{ color: SECONDARY, marginTop: 2 }}>WIP Count: <b>{d.count}</b></div>
      <div style={{ borderTop: `1px solid ${BORDER}`, marginTop: 6, paddingTop: 6,
          color: MUTED, fontSize: 11 }}>
        X̄: {MEAN.toFixed(0)} · UNPL: {UNPL.toFixed(0)} · LNPL: {LNPL.toFixed(0)}
      </div>
      {d.aboveUnpl && <div style={{ color: ALARM,    fontWeight: 700, marginTop: 4 }}>⚠ Signal: above UNPL</div>}
      {d.belowLnpl && <div style={{ color: POSITIVE, fontWeight: 700, marginTop: 4 }}>↓ Signal: below LNPL</div>}
    </div>
  );
};


// ── MAIN EXPORT ───────────────────────────────────────────────────────────────

export default function WipAgeStabilityChart() {
  const fmtX = iso =>
    new Date(iso + "T00:00:00").toLocaleDateString("en-GB", { day: "2-digit", month: "short" });
  const interval    = Math.max(1, Math.floor(CHART_DATA.length / 9));
  const latest      = RAW[RAW.length - 1];
  const statusColor = STATUS === "stable" ? POSITIVE : ALARM;

  return (
    <div style={{ background: PAGE_BG, minHeight: "100vh", padding: "24px 20px",
        fontFamily: FONT_STACK, color: TEXT }}>
      <div style={{ maxWidth: 1100, margin: "0 auto" }}>

        <div style={{ fontSize: 11, color: MUTED, letterSpacing: "0.08em",
            textTransform: "uppercase", marginBottom: 6 }}>
          {PROJECT_KEY} · {BOARD_NAME} · Board {BOARD_ID}
        </div>
        <h1 style={{ fontSize: 22, fontWeight: 700, margin: "0 0 4px" }}>Total WIP Age Stability</h1>
        <div style={{ fontSize: 12, color: MUTED, marginBottom: 16 }}>
          Cumulative Age Burden · XmR Process Behavior Chart
          · {RAW[0]?.date} – {RAW[RAW.length - 1]?.date}
        </div>

        <div style={{ display: "flex", flexWrap: "wrap", gap: 10, marginBottom: 14 }}>
          <StatCard label="X̄ MEAN (days)"  value={MEAN.toFixed(0)}              color={XMR_MEAN}  />
          <StatCard label="UNPL (days)"     value={UNPL.toFixed(0)}              color={XMR_UNPL}  />
          <StatCard label="LNPL (days)"     value={LNPL.toFixed(0)}              color={XMR_LNPL}  />
          <StatCard label="TODAY (days)"    value={latest?.total_age.toFixed(0)} color={PRIMARY}   />
          <StatCard label="TODAY WIP COUNT" value={latest?.count}                color={SECONDARY} />
        </div>

        <div style={{ display: "flex", flexWrap: "wrap", gap: 6, marginBottom: 20, alignItems: "center" }}>
          <Badge text={STATUS === "stable" ? "✓ STATUS: STABLE" : "⚠ STATUS: UNSTABLE"} color={statusColor} />
          {ABOVE_GROUPS.map((g, i) =>
            <Badge key={"a" + i} text={`↑ Above UNPL · ${fmtR(g)}`} color={ALARM} />)}
          {BELOW_GROUPS.map((g, i) =>
            <Badge key={"b" + i} text={`↓ Below LNPL · ${fmtR(g)}`} color={POSITIVE} />)}
        </div>

        <div style={{ background: PANEL_BG, borderRadius: 12,
            border: `1px solid ${BORDER}`, padding: "16px 8px 8px" }}>
          <ResponsiveContainer width="100%" height={CFG.chartHeight}>
            <ComposedChart data={CHART_DATA} margin={CFG.mainMargin}>
              <defs>
                <linearGradient id="ageGrad" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="5%"  stopColor={PRIMARY} stopOpacity={0.15} />
                  <stop offset="95%" stopColor={PRIMARY} stopOpacity={0.01} />
                </linearGradient>
              </defs>
              <CartesianGrid strokeDasharray="3 3" stroke={BORDER} vertical={false} />
              <XAxis dataKey="date" tickFormatter={fmtX} interval={interval} angle={-45}
                textAnchor="end" height={60}
                tick={{ fill: MUTED, fontSize: 10, fontFamily: FONT_STACK }} />
              <YAxis yAxisId="age" orientation="left" domain={[Y_L_MIN, Y_L_MAX]}
                tick={{ fill: PRIMARY, fontSize: 10, fontFamily: FONT_STACK }}
                label={{ value: "Total WIP Age (days)", angle: -90, position: "insideLeft",
                  fill: PRIMARY, fontSize: 10, dy: 70 }} />
              <YAxis yAxisId="count" orientation="right" domain={[Y_R_MIN, Y_R_MAX]}
                tick={{ fill: SECONDARY, fontSize: 10, fontFamily: FONT_STACK }}
                label={{ value: "WIP Count", angle: 90, position: "insideRight",
                  fill: SECONDARY, fontSize: 10, dy: -40 }} />
              <Tooltip content={<TT />} />
              <Area yAxisId="age" dataKey="total_age" fill="url(#ageGrad)"
                stroke="none" dot={false} activeDot={false} isAnimationActive={false} />
              <Line yAxisId="age"   dataKey="total_age" stroke={PRIMARY}   strokeWidth={1.5}
                dot={<AgeDot />} activeDot={false} isAnimationActive={false} />
              <Line yAxisId="count" dataKey="count"     stroke={SECONDARY} strokeWidth={1}
                strokeDasharray="3 3" dot={false} activeDot={false} isAnimationActive={false} />
              <ReferenceLine yAxisId="age" y={UNPL} stroke={XMR_UNPL} strokeDasharray="6 3"
                strokeWidth={1.5} label={{ value: "UNPL", fill: XMR_UNPL, fontSize: 10, position: "insideTopRight" }} />
              <ReferenceLine yAxisId="age" y={MEAN} stroke={XMR_MEAN} strokeDasharray="4 4"
                strokeWidth={1.5} label={{ value: "X̄",   fill: XMR_MEAN, fontSize: 10, position: "insideTopRight" }} />
              <ReferenceLine yAxisId="age" y={LNPL} stroke={XMR_LNPL} strokeDasharray="6 3"
                strokeWidth={1}   label={{ value: "LNPL", fill: XMR_LNPL, fontSize: 10, position: "insideBottomRight" }} />
            </ComposedChart>
          </ResponsiveContainer>

          <div style={{ display: "flex", flexWrap: "wrap", gap: 14, justifyContent: "center", marginTop: 8 }}>
            {[
              [<line x1={0} y1={6} x2={24} y2={6} stroke={PRIMARY}   strokeWidth={1.5} />,                      "Total WIP Age"],
              [<line x1={0} y1={6} x2={24} y2={6} stroke={SECONDARY} strokeDasharray="3 3" strokeWidth={1} />,  "WIP Count (right axis)"],
              [<line x1={0} y1={6} x2={24} y2={6} stroke={XMR_UNPL} strokeDasharray="6 3" strokeWidth={1.5} />, "UNPL"],
              [<line x1={0} y1={6} x2={24} y2={6} stroke={XMR_MEAN} strokeDasharray="4 4" strokeWidth={1.5} />, "X̄ Mean"],
              [<line x1={0} y1={6} x2={24} y2={6} stroke={XMR_LNPL} strokeDasharray="6 3" strokeWidth={1} />,   "LNPL"],
              [<circle cx={6} cy={6} r={4} fill={ALARM} />,                                                      "Above UNPL"],
              [<circle cx={6} cy={6} r={4} fill={POSITIVE} />,                                                   "Below LNPL"],
            ].map(([el, label]) => (
              <div key={label} style={{ display: "flex", alignItems: "center", gap: 5 }}>
                <svg width={24} height={12}>{el}</svg>
                <span style={{ fontSize: 10, color: MUTED }}>{label}</span>
              </div>
            ))}
          </div>
        </div>

        <div style={{ fontSize: 11, color: MUTED, lineHeight: 1.7,
            borderTop: `1px solid ${BORDER}`, paddingTop: 14, marginTop: 16 }}>
          <div style={{ marginBottom: 6 }}>
            <b style={{ color: TEXT }}>Reading this chart: </b>
            Total WIP Age is the sum of all active items ages — the cumulative age burden
            on the system. Unlike WIP Count, it detects stagnation: even a stable item
            count can hide growing backlogs if items are not moving. XmR limits are applied
            directly to Total WIP Age, making signals robust and assumption-free. The dashed
            cyan overlay shows WIP Count on the right axis for context.
          </div>
          <div>
            <b style={{ color: TEXT }}>Data provenance: </b>
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
