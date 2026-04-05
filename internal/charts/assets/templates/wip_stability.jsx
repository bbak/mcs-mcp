import { ComposedChart, Area, Line, ReferenceLine, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer } from "recharts";
import { ALARM, SECONDARY, POSITIVE, TEXT, MUTED, PAGE_BG, PANEL_BG, BORDER, XMR_UNPL, XMR_MEAN, XMR_LNPL, FONT_STACK } from "mcs-mcp";
import { StatCard, Badge, TOOLTIP_BG } from "./shared.jsx";

// ── INJECTED DATA ─────────────────────────────────────────────────────────────
// Payload is injected by the MCS chart renderer as window.__MCS_PAYLOAD__.

const __MCS_ENVELOPE__ = window.__MCS_PAYLOAD__;
const __MCS_DATA__ = __MCS_ENVELOPE__.data;
const __MCS_GUARDRAILS__ = __MCS_ENVELOPE__.guardrails;
const __MCS_WORKFLOW__ = __MCS_ENVELOPE__.workflow;
// ── CONFIG ────────────────────────────────────────────────────────────────────

const CFG = {
  chartHeight:  380,
  mainMargin:   { top: 10, right: 20, bottom: 55, left: 10 },
};

// ── DERIVED ───────────────────────────────────────────────────────────────────

const ws = __MCS_DATA__.wip_stability;

const BOARD_ID    = __MCS_WORKFLOW__.board_id;
const PROJECT_KEY = __MCS_WORKFLOW__.project_key;
const BOARD_NAME  = __MCS_WORKFLOW__.board_name;

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
      <div style={{ color: SECONDARY }}>WIP Count: <b>{d.count}</b></div>
      <div style={{ borderTop: `1px solid ${BORDER}`, marginTop: 6, paddingTop: 6,
          color: MUTED, fontSize: 11 }}>
        X̄ {MEAN.toFixed(1)} · UNPL {UNPL.toFixed(1)} · LNPL {LNPL.toFixed(1)}
      </div>
      {d.aboveUnpl && <div style={{ color: ALARM,    fontWeight: 700, marginTop: 4 }}>⚠ Signal: above UNPL</div>}
      {d.belowLnpl && <div style={{ color: POSITIVE, fontWeight: 700, marginTop: 4 }}>↓ Signal: below LNPL</div>}
    </div>
  );
};


// ── MAIN EXPORT ───────────────────────────────────────────────────────────────

export default function WipStabilityChart() {
  const fmtX = iso =>
    new Date(iso + "T00:00:00").toLocaleDateString("en-GB", { day: "2-digit", month: "short" });
  const interval = Math.max(1, Math.floor(CHART_DATA.length / 8));
  const statusColor = STATUS === "stable" ? POSITIVE : ALARM;
  const statusLabel = STATUS === "stable" ? "✓ STATUS: STABLE" : "⚠ STATUS: UNSTABLE";

  return (
    <div style={{ background: PAGE_BG, minHeight: "100vh", padding: "24px 20px",
        fontFamily: FONT_STACK, color: TEXT }}>
      <div style={{ maxWidth: 1100, margin: "0 auto" }}>

      {/* Header */}
      <div style={{ fontSize: 11, color: MUTED, letterSpacing: "0.08em",
          textTransform: "uppercase", marginBottom: 6 }}>
        {PROJECT_KEY} · {BOARD_NAME} · Board {BOARD_ID}
      </div>
      <h1 style={{ fontSize: 22, fontWeight: 700, margin: "0 0 4px" }}>WIP Count Stability</h1>
      <div style={{ fontSize: 12, color: MUTED, marginBottom: 16 }}>
        Daily Active WIP · XmR Process Behavior Chart
        · {DATA[0]?.date} – {DATA[DATA.length - 1]?.date}
      </div>

      {/* Stat cards */}
      <div style={{ display: "flex", flexWrap: "wrap", gap: 10, marginBottom: 14 }}>
        <StatCard label="X̄ MEAN" value={MEAN.toFixed(1)}              color={XMR_MEAN}  />
        <StatCard label="UNPL"   value={UNPL.toFixed(1)}              color={ALARM}     />
        <StatCard label="LNPL"   value={LNPL.toFixed(1)}              color={POSITIVE}  />
        <StatCard label="TODAY"  value={DATA[DATA.length - 1]?.count} color={SECONDARY} />
      </div>

      {/* Status + signal badges */}
      <div style={{ display: "flex", flexWrap: "wrap", gap: 6, marginBottom: 20, alignItems: "center" }}>
        <Badge text={statusLabel} color={statusColor} />
        {ABOVE_GROUPS.map((g, i) =>
          <Badge key={"a" + i} text={`↑ Above UNPL · ${fmtRange(g)}`} color={ALARM} />)}
        {BELOW_GROUPS.map((g, i) =>
          <Badge key={"b" + i} text={`↓ Below LNPL · ${fmtRange(g)}`} color={POSITIVE} />)}
      </div>

      {/* Chart panel */}
      <div style={{ background: PANEL_BG, borderRadius: 12,
          border: `1px solid ${BORDER}`, padding: "16px 8px 8px" }}>
        <div style={{ height: CFG.chartHeight }}>
        <ResponsiveContainer width="100%" height="100%">
          <ComposedChart data={CHART_DATA} margin={CFG.mainMargin}>
            <defs>
              <linearGradient id="wipGrad" x1="0" y1="0" x2="0" y2="1">
                <stop offset="5%"  stopColor={SECONDARY} stopOpacity={0.15} />
                <stop offset="95%" stopColor={SECONDARY} stopOpacity={0.01} />
              </linearGradient>
            </defs>
            <CartesianGrid strokeDasharray="3 3" stroke={BORDER} vertical={false} />
            <XAxis dataKey="date" tickFormatter={fmtX} interval={interval} angle={-45}
              textAnchor="end" height={60}
              tick={{ fill: MUTED, fontSize: 10, fontFamily: FONT_STACK }} />
            <YAxis domain={[Y_MIN, Y_MAX]}
              tick={{ fill: MUTED, fontSize: 10, fontFamily: FONT_STACK }}
              label={{ value: "Active WIP (items)", angle: -90, position: "insideLeft",
                fill: MUTED, fontSize: 10, dy: 56 }} />
            <Tooltip content={TT} />
            <Area dataKey="count" fill="url(#wipGrad)" stroke="none"
              dot={false} activeDot={false} isAnimationActive={false} />
            <Line dataKey="count" stroke={SECONDARY} strokeWidth={1.5}
              dot={<Dot />} activeDot={false} isAnimationActive={false} />
            <ReferenceLine y={UNPL} stroke={XMR_UNPL} strokeDasharray="6 3" strokeWidth={1.5}
              label={{ value: "UNPL", fill: XMR_UNPL, fontSize: 10, position: "insideTopRight" }} />
            <ReferenceLine y={MEAN} stroke={XMR_MEAN} strokeDasharray="4 4" strokeWidth={1.5}
              label={{ value: "X̄",   fill: XMR_MEAN, fontSize: 10, position: "insideTopRight" }} />
            <ReferenceLine y={LNPL} stroke={XMR_LNPL} strokeDasharray="6 3" strokeWidth={1}
              label={{ value: "LNPL", fill: XMR_LNPL, fontSize: 10, position: "insideBottomRight" }} />
          </ComposedChart>
        </ResponsiveContainer>
        </div>

        {/* Legend */}
        <div style={{ display: "flex", flexWrap: "wrap", gap: 14, justifyContent: "center", marginTop: 6 }}>
          <div style={{ display: "flex", alignItems: "center", gap: 5 }}>
            <div style={{ width: 24, height: 1.5, background: SECONDARY }} />
            <span style={{ fontSize: 10, color: MUTED }}>Daily WIP</span>
          </div>
          <div style={{ display: "flex", alignItems: "center", gap: 5 }}>
            <div style={{ width: 24, height: 0, borderTop: `1.5px dashed ${XMR_UNPL}` }} />
            <span style={{ fontSize: 10, color: MUTED }}>UNPL</span>
          </div>
          <div style={{ display: "flex", alignItems: "center", gap: 5 }}>
            <div style={{ width: 24, height: 0, borderTop: `1.5px dashed ${XMR_MEAN}` }} />
            <span style={{ fontSize: 10, color: MUTED }}>X̄ Mean</span>
          </div>
          <div style={{ display: "flex", alignItems: "center", gap: 5 }}>
            <div style={{ width: 24, height: 0, borderTop: `1px dashed ${XMR_LNPL}` }} />
            <span style={{ fontSize: 10, color: MUTED }}>LNPL</span>
          </div>
          <div style={{ display: "flex", alignItems: "center", gap: 5 }}>
            <div style={{ width: 8, height: 8, borderRadius: "50%", background: ALARM }} />
            <span style={{ fontSize: 10, color: MUTED }}>Above UNPL</span>
          </div>
          <div style={{ display: "flex", alignItems: "center", gap: 5 }}>
            <div style={{ width: 8, height: 8, borderRadius: "50%", background: POSITIVE }} />
            <span style={{ fontSize: 10, color: MUTED }}>Below LNPL</span>
          </div>
        </div>
      </div>

      {/* Footer */}
      <div style={{ fontSize: 11, color: MUTED, lineHeight: 1.7,
          borderTop: `1px solid ${BORDER}`, paddingTop: 14, marginTop: 16 }}>
        <div style={{ marginBottom: 6 }}>
          <b style={{ color: TEXT }}>Reading this chart: </b>
          The line shows the daily count of active in-progress items (past the commitment
          point, not yet finished). The dashed X̄ is the process mean. UNPL and LNPL define
          the expected range of natural variation — points outside these limits are statistical
          signals. Uncontrolled WIP violates the assumptions of Little's Law, making cycle
          time predictions fundamentally unreliable.
        </div>
        <div>
          <b style={{ color: TEXT }}>Data provenance: </b>
          Wheeler XmR Process Behavior Chart applied to weekly WIP subgroup averages.
          Limits: X̄ ± 2.66 × MR̄. Colored dots mark signal breaches (red = above UNPL,
          green = below LNPL). Chart downsampled to every 2nd point for readability;
          all signal points are always retained.
        </div>
      </div>

      </div>
    </div>
  );
}
