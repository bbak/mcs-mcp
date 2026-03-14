import { useState } from "react";
import {
  ComposedChart, Scatter, ReferenceLine, XAxis, YAxis,
  CartesianGrid, Tooltip, ResponsiveContainer,
} from "recharts";

// ── INJECTED DATA ─────────────────────────────────────────────────────────────
// These two constants are replaced by Claude via str_replace before delivery.
// Do NOT edit manually.

const MCP_RESPONSE = "__MCP_RESPONSE__";
const CHART_ATTRS  = "__CHART_ATTRS__";

// ── CONFIG ────────────────────────────────────────────────────────────────────

const CFG = {
  mainChartHeight:   380,
  miniChartHeight:   140,
  mainMargin:        { top: 10, right: 20, bottom: 60, left: 10 },

  siClogged:         1.3,
  siMarginal:        0.9,

  dotOutlierRadius:  5,
  dotShiftRadius:    4,
  dotNormalRadius:   2.5,

  COLOR_ALARM:       "#ff6b6b",
  COLOR_CAUTION:     "#e2c97e",
  COLOR_SECONDARY:   "#7edde2",
  COLOR_POSITIVE:    "#6bffb8",
  COLOR_TEXT:        "#dde1ef",
  COLOR_MUTED:       "#505878",
  COLOR_PAGE_BG:     "#080a0f",
  COLOR_PANEL_BG:    "#0c0e16",
  COLOR_BORDER:      "#1a1d2e",
  COLOR_OVERALL:     "#dde1ef",

  typePalette: ["#6b7de8", "#7edde2", "#ff6b6b", "#e2c97e", "#6bffb8", "#c97eb2", "#e2a97e"],

  xTickCount:        9,
  ySnapStep:         50,
  defaultLogScale:   false,
};

// ── DERIVED ───────────────────────────────────────────────────────────────────

const d = MCP_RESPONSE.data;

const RAW_SCATTERPLOT = d.scatterplot;
const OVERALL_XMR     = d.stability.xmr;
const OVERALL_SI      = d.stability.stability_index;
const OVERALL_ELT     = d.stability.expected_lead_time;
const STRATIFIED      = d.stratified;

const BOARD_ID    = CHART_ATTRS.board_id;
const PROJECT_KEY = CHART_ATTRS.project_key;
const BOARD_NAME  = CHART_ATTRS.board_name;

const DETECTED_TYPES = [...new Set(RAW_SCATTERPLOT.map(p => p.issue_type))].sort();
const TYPE_COLORS = Object.fromEntries([
  ["Overall", CFG.COLOR_OVERALL],
  ...DETECTED_TYPES.map((t, i) => [t, CFG.typePalette[i % CFG.typePalette.length]]),
]);
const TABS = ["Overall", ...DETECTED_TYPES];

// ── HELPERS ───────────────────────────────────────────────────────────────────

const siColor = (si) =>
  si > CFG.siClogged  ? CFG.COLOR_ALARM   :
  si > CFG.siMarginal ? CFG.COLOR_CAUTION :
                        CFG.COLOR_POSITIVE;

const siLabel = (si) =>
  si > CFG.siClogged  ? "⚠ CLOGGED"  :
  si > CFG.siMarginal ? "~ MARGINAL" :
                        "✓ STABLE";

const formatDate = (d) =>
  new Date(d + "T00:00:00").toLocaleDateString("en-GB", { day: "2-digit", month: "short" });

function buildChartData(scatterplot, signals) {
  const outliers = new Set((signals || []).filter(s => s.type === "outlier").map(s => s.index));
  const shifts   = new Set((signals || []).filter(s => s.type === "shift").map(s => s.index));
  const keyMap   = Object.fromEntries((signals || []).map(s => [s.index, s.key]));
  return scatterplot.map((pt, i) => ({
    date:      pt.date,
    value:     pt.value,
    mr:        pt.moving_range,
    key:       pt.key,
    issueType: pt.issue_type,
    isOutlier: outliers.has(i),
    isShift:   shifts.has(i),
    issueKey:  keyMap[i] || pt.key,
  }));
}

// ── CUSTOM DOT ────────────────────────────────────────────────────────────────

const CustomDot = (typeColor) => ({ cx, cy, payload }) => {
  if (!cx || !cy) return null;
  if (payload.isOutlier)
    return <circle cx={cx} cy={cy} r={CFG.dotOutlierRadius}
      fill={CFG.COLOR_ALARM} stroke={CFG.COLOR_PAGE_BG} strokeWidth={1.5}/>;
  if (payload.isShift)
    return <circle cx={cx} cy={cy} r={CFG.dotShiftRadius}
      fill={CFG.COLOR_CAUTION} stroke={CFG.COLOR_PAGE_BG} strokeWidth={1.5}/>;
  return <circle cx={cx} cy={cy} r={CFG.dotNormalRadius}
    fill={typeColor} fillOpacity={0.5} stroke="none"/>;
};

// ── TOOLTIP ───────────────────────────────────────────────────────────────────

const CustomTooltip = ({ active, payload, mean, unpl }) => {
  if (!active || !payload?.length) return null;
  const d = payload[0].payload;
  return (
    <div style={{ background: "#0f1117", border: `1px solid ${CFG.COLOR_BORDER}`,
      borderRadius: 8, padding: "10px 14px",
      fontFamily: "'Courier New', monospace", fontSize: 12, color: CFG.COLOR_TEXT }}>
      <div style={{ fontWeight: 700, marginBottom: 4 }}>{d.issueKey}</div>
      <div style={{ color: CFG.COLOR_MUTED, marginBottom: 6 }}>{formatDate(d.date)}</div>
      <div style={{ borderTop: `1px solid ${CFG.COLOR_BORDER}`, paddingTop: 6 }}>
        <div>Cycle Time: <b>{d.value.toFixed(1)} d</b></div>
        <div>mR: {d.mr != null ? d.mr.toFixed(1) + " d" : "–"}</div>
        <div style={{ color: CFG.COLOR_MUTED }}>X̄: {mean.toFixed(1)} d</div>
        <div style={{ color: CFG.COLOR_MUTED }}>UNPL: {unpl.toFixed(1)} d</div>
        {d.isOutlier && <div style={{ color: CFG.COLOR_ALARM,   fontWeight: 700, marginTop: 4 }}>⚠ Outlier: above UNPL</div>}
        {d.isShift   && <div style={{ color: CFG.COLOR_CAUTION, fontWeight: 700, marginTop: 4 }}>⇶ Process Shift detected</div>}
      </div>
    </div>
  );
};

// ── MINI CHART ────────────────────────────────────────────────────────────────

const MiniChart = ({ type, data, mean, unpl }) => {
  const color = TYPE_COLORS[type];
  const yMax  = Math.ceil((Math.max(...data.map(d => d.value), unpl) * 1.1) / CFG.ySnapStep) * CFG.ySnapStep;
  return (
    <ResponsiveContainer width="100%" height={CFG.miniChartHeight}>
      <ComposedChart data={data} margin={{ top: 8, right: 8, bottom: 8, left: 8 }}>
        <CartesianGrid strokeDasharray="3 3" stroke={CFG.COLOR_BORDER} vertical={false}/>
        <XAxis dataKey="date" hide/>
        <YAxis domain={[0, yMax]} hide/>
        <Scatter dataKey="value" shape={CustomDot(color)} isAnimationActive={false}/>
        <ReferenceLine y={unpl} stroke={CFG.COLOR_ALARM} strokeDasharray="4 2" strokeWidth={1}/>
        <ReferenceLine y={mean} stroke={color}            strokeDasharray="4 4" strokeWidth={1}/>
      </ComposedChart>
    </ResponsiveContainer>
  );
};

// ── STAT CARD ─────────────────────────────────────────────────────────────────

const StatCard = ({ label, value, color }) => (
  <div style={{ background: CFG.COLOR_PANEL_BG, border: `1px solid ${color}33`,
    borderRadius: 8, padding: "10px 16px", minWidth: 110 }}>
    <div style={{ fontSize: 10, color: CFG.COLOR_MUTED, marginBottom: 4, letterSpacing: "0.05em" }}>
      {label}
    </div>
    <div style={{ fontSize: 20, fontWeight: 700, color }}>{value}</div>
  </div>
);

// ── BADGE ─────────────────────────────────────────────────────────────────────

const Badge = ({ text, color }) => (
  <span style={{ fontSize: 11, padding: "4px 10px", borderRadius: 4,
    background: `${color}15`, border: `1px solid ${color}40`, color }}>
    {text}
  </span>
);

// ── CHART PANEL ───────────────────────────────────────────────────────────────

const ChartPanel = ({ data, mean, unpl, lnpl, typeColor }) => {
  const [logScale, setLogScale] = useState(CFG.defaultLogScale);
  const yMax     = Math.ceil((Math.max(...data.map(d => d.value), unpl) * 1.1) / CFG.ySnapStep) * CFG.ySnapStep;
  const interval = Math.max(1, Math.floor(data.length / CFG.xTickCount));
  const outlierCount = data.filter(d => d.isOutlier).length;
  const shiftCount   = data.filter(d => d.isShift).length;

  return (
    <div style={{ background: CFG.COLOR_PANEL_BG, borderRadius: 12,
      border: `1px solid ${CFG.COLOR_BORDER}`, padding: "20px 12px 12px 12px", marginBottom: 20 }}>

      <div style={{ display: "flex", flexWrap: "wrap", gap: 8, marginBottom: 16, alignItems: "center" }}>
        {outlierCount > 0 && <Badge text={`⚠ ${outlierCount} outlier${outlierCount > 1 ? "s" : ""} above UNPL`} color={CFG.COLOR_ALARM}/>}
        {shiftCount   > 0 && <Badge text={`⇶ ${shiftCount} process shift${shiftCount > 1 ? "s" : ""} detected`}  color={CFG.COLOR_CAUTION}/>}
        <div style={{ flex: 1 }}/>
        <button onClick={() => setLogScale(s => !s)} style={{
          fontSize: 10, padding: "5px 14px", borderRadius: 6, cursor: "pointer",
          background: logScale ? `${CFG.COLOR_SECONDARY}18` : CFG.COLOR_BORDER,
          border: `1.5px solid ${logScale ? CFG.COLOR_SECONDARY : CFG.COLOR_MUTED}`,
          color: logScale ? CFG.COLOR_SECONDARY : CFG.COLOR_TEXT,
          fontFamily: "'Courier New', monospace", fontWeight: 700,
        }}>
          {logScale ? "LOG" : "LINEAR"}
        </button>
      </div>

      <ResponsiveContainer width="100%" height={CFG.mainChartHeight}>
        <ComposedChart data={data} margin={CFG.mainMargin}>
          <CartesianGrid strokeDasharray="3 3" stroke={CFG.COLOR_BORDER} vertical={false}/>
          <XAxis dataKey="date" tickFormatter={formatDate} interval={interval}
            angle={-45} textAnchor="end" height={60}
            tick={{ fill: CFG.COLOR_MUTED, fontSize: 11, fontFamily: "'Courier New', monospace" }}/>
          <YAxis scale={logScale ? "log" : "auto"}
            domain={logScale ? [0.01, yMax] : [0, yMax]}
            allowDataOverflow
            tickFormatter={v => `${v}d`}
            tick={{ fill: CFG.COLOR_MUTED, fontSize: 11, fontFamily: "'Courier New', monospace" }}
            label={{ value: "Cycle Time (days)", angle: -90, position: "insideLeft",
              fill: CFG.COLOR_MUTED, fontSize: 11, fontFamily: "'Courier New', monospace" }}/>
          <Tooltip content={<CustomTooltip mean={mean} unpl={unpl}/>}/>
          <ReferenceLine y={unpl} stroke={CFG.COLOR_ALARM} strokeDasharray="6 3" strokeWidth={1.5}
            label={{ value: "UNPL", fill: CFG.COLOR_ALARM, fontSize: 10,
              fontFamily: "'Courier New', monospace", position: "insideTopRight" }}/>
          <ReferenceLine y={mean} stroke={typeColor} strokeDasharray="4 4" strokeWidth={1.5}
            label={{ value: "X̄", fill: typeColor, fontSize: 10,
              fontFamily: "'Courier New', monospace", position: "insideTopRight" }}/>
          {lnpl > 0 && (
            <ReferenceLine y={lnpl} stroke={CFG.COLOR_POSITIVE} strokeDasharray="4 4" strokeWidth={1}
              label={{ value: "LNPL", fill: CFG.COLOR_POSITIVE, fontSize: 10,
                fontFamily: "'Courier New', monospace", position: "insideBottomRight" }}/>
          )}
          <Scatter dataKey="value" shape={CustomDot(typeColor)} isAnimationActive={false}/>
        </ComposedChart>
      </ResponsiveContainer>

      <div style={{ display: "flex", flexWrap: "wrap", gap: 16, justifyContent: "center", marginTop: 8 }}>
        {[
          { svg: <circle cx={6} cy={6} r={2.5} fill={typeColor} fillOpacity={0.5}/>, label: "Cycle Time (individual items)" },
          { svg: <line x1={0} y1={6} x2={24} y2={6} stroke={CFG.COLOR_ALARM} strokeDasharray="6 3" strokeWidth={1.5}/>, label: "UNPL" },
          { svg: <line x1={0} y1={6} x2={24} y2={6} stroke={typeColor} strokeDasharray="4 4" strokeWidth={1.5}/>, label: "X̄ Mean" },
          { svg: <circle cx={6} cy={6} r={5} fill={CFG.COLOR_ALARM}/>, label: "Outlier (above UNPL)" },
          { svg: <circle cx={6} cy={6} r={4} fill={CFG.COLOR_CAUTION}/>, label: "Process Shift anchor" },
        ].map(({ svg, label }) => (
          <div key={label} style={{ display: "flex", alignItems: "center", gap: 6 }}>
            <svg width={24} height={12}>{svg}</svg>
            <span style={{ fontSize: 11, color: CFG.COLOR_MUTED }}>{label}</span>
          </div>
        ))}
      </div>
    </div>
  );
};

// ── MAIN EXPORT ───────────────────────────────────────────────────────────────

export default function ProcessStabilityChart() {
  const [activeTab, setActiveTab] = useState("Overall");

  const overallData = buildChartData(RAW_SCATTERPLOT, OVERALL_XMR.signals);
  const isOverall   = activeTab === "Overall";

  const xmr       = isOverall ? OVERALL_XMR : STRATIFIED[activeTab]?.xmr;
  const si        = isOverall ? OVERALL_SI  : (STRATIFIED[activeTab]?.stability_index ?? 0);
  const elt       = isOverall ? OVERALL_ELT : (STRATIFIED[activeTab]?.expected_lead_time ?? 0);
  const typeColor = TYPE_COLORS[activeTab] ?? CFG.COLOR_OVERALL;

  const viewData = isOverall
    ? overallData
    : buildChartData(
        RAW_SCATTERPLOT.filter(p => p.issue_type === activeTab),
        xmr?.signals
      );

  const mean = xmr?.average ?? 0;
  const unpl = xmr?.upper_natural_process_limit ?? 0;
  const lnpl = xmr?.lower_natural_process_limit ?? 0;

  const outlierCount = (xmr?.signals || []).filter(s => s.type === "outlier").length;
  const shiftCount   = (xmr?.signals || []).filter(s => s.type === "shift").length;

  const dateRange = RAW_SCATTERPLOT.length > 0
    ? `${formatDate(RAW_SCATTERPLOT[0].date)} – ${formatDate(RAW_SCATTERPLOT[RAW_SCATTERPLOT.length - 1].date)}`
    : "";

  const typeSummary = DETECTED_TYPES.filter(t => STRATIFIED[t]).map(t => {
    const pts = RAW_SCATTERPLOT.filter(p => p.issue_type === t);
    const str = STRATIFIED[t];
    return {
      type:     t,
      data:     buildChartData(pts, str?.xmr?.signals),
      mean:     str?.xmr?.average ?? 0,
      unpl:     str?.xmr?.upper_natural_process_limit ?? 0,
      si:       str?.stability_index ?? 0,
      elt:      str?.expected_lead_time ?? 0,
      outliers: (str?.xmr?.signals || []).filter(s => s.type === "outlier").length,
      shifts:   (str?.xmr?.signals || []).filter(s => s.type === "shift").length,
    };
  });

  return (
    <div style={{ background: CFG.COLOR_PAGE_BG, minHeight: "100vh", padding: "32px 24px",
      fontFamily: "'Courier New', monospace", color: CFG.COLOR_TEXT }}>
      <div style={{ maxWidth: 1100, margin: "0 auto" }}>

        <div style={{ fontSize: 11, color: CFG.COLOR_MUTED, letterSpacing: "0.08em",
          textTransform: "uppercase", marginBottom: 8 }}>
          {PROJECT_KEY} · {BOARD_NAME} · Board {BOARD_ID}
        </div>

        <h1 style={{ fontSize: 26, fontWeight: 700, margin: "0 0 4px 0" }}>Process Stability</h1>
        <div style={{ fontSize: 13, color: CFG.COLOR_MUTED, marginBottom: 20 }}>
          Cycle Time Scatterplot · Individual items by completion date · {dateRange}
        </div>

        <div style={{ display: "flex", flexWrap: "wrap", gap: 12, marginBottom: 16 }}>
          <StatCard label="X̄ MEAN"      value={`${mean.toFixed(1)}d`} color={typeColor}/>
          <StatCard label="UNPL"         value={`${unpl.toFixed(1)}d`} color={CFG.COLOR_ALARM}/>
          <StatCard label="EXPECTED P50" value={`${elt}d`}             color={CFG.COLOR_SECONDARY}/>
          <StatCard label="STAB. INDEX"  value={si.toFixed(2)}         color={siColor(si)}/>
        </div>

        <div style={{ display: "flex", flexWrap: "wrap", gap: 8, marginBottom: 24, alignItems: "center" }}>
          <Badge text={siLabel(si)} color={siColor(si)}/>
          {outlierCount > 0 && <Badge text={`⚠ ${outlierCount} outlier${outlierCount > 1 ? "s" : ""} above UNPL`} color={CFG.COLOR_ALARM}/>}
          {shiftCount   > 0 && <Badge text={`⇶ ${shiftCount} process shift${shiftCount > 1 ? "s" : ""} detected`}  color={CFG.COLOR_CAUTION}/>}
        </div>

        <div style={{ display: "flex", flexWrap: "wrap", gap: 8, marginBottom: 20 }}>
          {TABS.filter(t => t === "Overall" || STRATIFIED[t]).map(t => {
            const tSi    = t === "Overall" ? OVERALL_SI : (STRATIFIED[t]?.stability_index ?? 0);
            const tColor = TYPE_COLORS[t] ?? CFG.COLOR_OVERALL;
            const active = activeTab === t;
            return (
              <button key={t} onClick={() => setActiveTab(t)} style={{
                padding: "6px 14px", borderRadius: 6, cursor: "pointer",
                background: active ? `${tColor}18` : CFG.COLOR_PANEL_BG,
                border: `1.5px solid ${active ? tColor : CFG.COLOR_BORDER}`,
                color: active ? tColor : CFG.COLOR_MUTED,
                fontFamily: "'Courier New', monospace", fontSize: 12,
              }}>
                {t}
                <span style={{ display: "block", fontSize: 9, color: siColor(tSi), marginTop: 1 }}>
                  SI {tSi.toFixed(2)}
                </span>
              </button>
            );
          })}
        </div>

        <ChartPanel
          data={viewData} mean={mean} unpl={unpl} lnpl={lnpl}
          typeColor={typeColor}
        />

        {isOverall && typeSummary.length > 0 && (
          <div style={{
            display: "grid",
            gridTemplateColumns: `repeat(${Math.min(typeSummary.length, 3)}, 1fr)`,
            gap: 16, marginBottom: 24,
          }}>
            {typeSummary.map(({ type, data, mean, unpl, si, elt, outliers, shifts }) => (
              <div key={type} style={{ background: CFG.COLOR_PANEL_BG, borderRadius: 10,
                border: `1px solid ${CFG.COLOR_BORDER}`, padding: "14px 12px" }}>
                <div style={{ display: "flex", justifyContent: "space-between",
                  alignItems: "center", marginBottom: 8 }}>
                  <span style={{ fontWeight: 700, color: TYPE_COLORS[type], fontSize: 13 }}>{type}</span>
                  <span style={{ fontSize: 10, padding: "2px 8px", borderRadius: 4,
                    background: `${siColor(si)}15`, border: `1px solid ${siColor(si)}40`,
                    color: siColor(si) }}>{siLabel(si)}</span>
                </div>
                <div style={{ fontSize: 11, color: CFG.COLOR_MUTED, marginBottom: 8,
                  display: "grid", gridTemplateColumns: "1fr 1fr", rowGap: 3 }}>
                  <span>X̄ <b style={{ color: CFG.COLOR_TEXT }}>{mean.toFixed(1)}d</b></span>
                  <span>UNPL <b style={{ color: CFG.COLOR_ALARM }}>{unpl.toFixed(1)}d</b></span>
                  <span>P50 <b style={{ color: CFG.COLOR_SECONDARY }}>{elt}d</b></span>
                  <span>SI <b style={{ color: siColor(si) }}>{si.toFixed(2)}</b></span>
                  {outliers > 0 && <span style={{ color: CFG.COLOR_ALARM   }}>⚠ {outliers} outlier{outliers > 1 ? "s" : ""}</span>}
                  {shifts   > 0 && <span style={{ color: CFG.COLOR_CAUTION }}>⇶ {shifts} shift{shifts > 1 ? "s" : ""}</span>}
                </div>
                <MiniChart type={type} data={data} mean={mean} unpl={unpl}/>
              </div>
            ))}
          </div>
        )}

        <div style={{ fontSize: 11, color: CFG.COLOR_MUTED, lineHeight: 1.7,
          borderTop: `1px solid ${CFG.COLOR_BORDER}`, paddingTop: 16 }}>
          <div style={{ marginBottom: 8 }}>
            <b style={{ color: CFG.COLOR_TEXT }}>Reading this chart:</b> Each point is the cycle
            time of one delivered item, plotted by its completion date. Multiple items may share
            a date — this is a scatterplot, not a line chart. The dashed X̄ is the process mean.
            The UNPL defines the outer boundary of natural variation — points above it are outliers
            caused by special circumstances, not normal process noise. Yellow dots mark the anchor
            of an 8-point run on one side of the mean (Process Shift signal). The Stability Index
            (WIP ÷ Throughput ÷ Mean Cycle Time) summarises system health: below {CFG.siMarginal} =
            stable, {CFG.siMarginal}–{CFG.siClogged} = marginal, above {CFG.siClogged} = clogged.
          </div>
          <div>
            <b style={{ color: CFG.COLOR_TEXT }}>Data provenance:</b> Wheeler XmR applied to
            individual item cycle times. Signals: outlier = above UNPL; shift = 8+ consecutive
            points on one side of X̄. Values near 0 indicate items committed and resolved the
            same day.
          </div>
        </div>

      </div>
    </div>
  );
}
