import { useState } from "react";
import {
  ComposedChart, Line, ScatterChart, Scatter, ReferenceLine, XAxis, YAxis,
  CartesianGrid, Tooltip, ResponsiveContainer,
} from "recharts";
import { ALARM, CAUTION, SECONDARY, POSITIVE, TEXT, MUTED, PAGE_BG, PANEL_BG, BORDER, typeColor, XMR_UNPL, XMR_LNPL, FONT_STACK } from "mcs-mcp";
import { StatCard, Badge, TOOLTIP_BG } from "./shared.jsx";

// ── INJECTED DATA ─────────────────────────────────────────────────────────────
// Payload is injected by the MCS chart renderer as window.__MCS_PAYLOAD__.

const __MCS_ENVELOPE__ = window.__MCS_PAYLOAD__;
const __MCS_DATA__ = __MCS_ENVELOPE__.data;
const __MCS_GUARDRAILS__ = __MCS_ENVELOPE__.guardrails;
const __MCS_WORKFLOW__ = __MCS_ENVELOPE__.workflow;
// ── CONFIG ────────────────────────────────────────────────────────────────────

const CFG = {
  mainChartHeight:   440,
  miniChartHeight:   140,
  mainMargin:        { top: 4, right: 20, bottom: 50, left: 10 },

  siClogged:         1.3,
  siMarginal:        0.9,

  dotOutlierRadius:  5,
  dotShiftRadius:    4,
  dotNormalRadius:   2.5,

  xTickCount:        9,
  ySnapStep:         50,
  defaultLogScale:   false,
};

// ── DERIVED ───────────────────────────────────────────────────────────────────

const d = __MCS_DATA__;

const RAW_SCATTERPLOT = d.scatterplot;
const OVERALL_XMR     = d.stability.xmr;
const OVERALL_SI      = d.stability.stability_index;
const OVERALL_ELT     = d.stability.expected_lead_time;
const STRATIFIED      = d.stratified;

const BOARD_ID    = __MCS_WORKFLOW__.board_id;
const PROJECT_KEY = __MCS_WORKFLOW__.project_key;
const BOARD_NAME  = __MCS_WORKFLOW__.board_name;

const DETECTED_TYPES = [...new Set(RAW_SCATTERPLOT.map(p => p.issue_type))].sort();
const TYPE_COLORS = Object.fromEntries([
  ["Overall", TEXT],
  ...DETECTED_TYPES.map(t => [t, typeColor(t, DETECTED_TYPES)]),
]);
const TABS = ["Overall", ...DETECTED_TYPES];

// ── HELPERS ───────────────────────────────────────────────────────────────────

const siColor = (si) =>
  si > CFG.siClogged  ? ALARM   :
  si > CFG.siMarginal ? CAUTION :
                        POSITIVE;

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
      fill={ALARM} stroke={PAGE_BG} strokeWidth={1.5}/>;
  if (payload.isShift)
    return <circle cx={cx} cy={cy} r={CFG.dotShiftRadius}
      fill={CAUTION} stroke={PAGE_BG} strokeWidth={1.5}/>;
  return <circle cx={cx} cy={cy} r={CFG.dotNormalRadius}
    fill={typeColor} fillOpacity={0.5} stroke="none"/>;
};

// ── TOOLTIP ───────────────────────────────────────────────────────────────────

const CustomTooltip = ({ active, payload, mean, unpl }) => {
  if (!active || !payload?.length) return null;
  const d = payload[0].payload;
  return (
    <div style={{ background: TOOLTIP_BG, border: `1px solid ${BORDER}`,
      borderRadius: 8, padding: "10px 14px",
      fontFamily: FONT_STACK, fontSize: 12, color: TEXT }}>
      <div style={{ fontWeight: 700, marginBottom: 4 }}>{d.issueKey}</div>
      <div style={{ color: MUTED, marginBottom: 6 }}>{formatDate(d.date)}</div>
      <div style={{ borderTop: `1px solid ${BORDER}`, paddingTop: 6 }}>
        <div>Cycle Time: <b>{d.value.toFixed(1)} d</b></div>
        <div>mR: {d.mr != null ? d.mr.toFixed(1) + " d" : "–"}</div>
        <div style={{ color: MUTED }}>X̄: {mean.toFixed(1)} d</div>
        <div style={{ color: MUTED }}>UNPL: {unpl.toFixed(1)} d</div>
        {d.isOutlier && <div style={{ color: ALARM,   fontWeight: 700, marginTop: 4 }}>⚠ Outlier: above UNPL</div>}
        {d.isShift   && <div style={{ color: CAUTION, fontWeight: 700, marginTop: 4 }}>⇶ Process Shift detected</div>}
      </div>
    </div>
  );
};

// ── MINI CHART ────────────────────────────────────────────────────────────────

function MiniDot({ cx, cy, payload, color }) {
  if (cx == null || cy == null) return null;
  const dotColor   = payload.isOutlier ? ALARM : payload.isShift ? CAUTION : color;
  const dotOpacity = payload.isOutlier ? 0.85 : 0.5;
  const r          = payload.isOutlier ? CFG.dotOutlierRadius : CFG.dotNormalRadius;
  return <circle cx={cx} cy={cy} r={r} fill={dotColor} fillOpacity={dotOpacity} stroke="none"/>;
}

function MiniChart({ type, data, mean, unpl }) {
  const color  = TYPE_COLORS[type];
  const yMax   = Math.ceil((Math.max(...data.map(d => d.value), unpl) * 1.1) / CFG.ySnapStep) * CFG.ySnapStep;
  const xMax   = Math.max(data.length - 1, 1);
  const scData = data.map((d, i) => ({ x: i, y: d.value, isOutlier: d.isOutlier, isShift: d.isShift }));
  return (
    <div style={{ height: CFG.miniChartHeight }}>
      <ResponsiveContainer width="100%" height="100%">
        <ScatterChart margin={{ top: 8, right: 8, bottom: 8, left: 8 }}>
          <CartesianGrid strokeDasharray="3 3" stroke={BORDER} vertical={false}/>
          <XAxis type="number" dataKey="x" domain={[0, xMax]} hide/>
          <YAxis type="number" dataKey="y" domain={[0, yMax]} hide/>
          <ReferenceLine y={unpl} stroke={XMR_UNPL} strokeDasharray="4 2" strokeWidth={1}/>
          <ReferenceLine y={mean} stroke={color} strokeDasharray="4 4" strokeWidth={1}/>
          <Scatter data={scData} shape={(props) => <MiniDot {...props} color={color} />} isAnimationActive={false}/>
        </ScatterChart>
      </ResponsiveContainer>
    </div>
  );
}


// ── TYPE SUMMARY CARD ─────────────────────────────────────────────────────────

function TypeCard({ item }) {
  return (
    <div style={{ background: PANEL_BG, borderRadius: 10,
      border: `1px solid ${BORDER}`, padding: "14px 12px" }}>
      <div style={{ display: "flex", justifyContent: "space-between",
        alignItems: "center", marginBottom: 8 }}>
        <span style={{ fontWeight: 700, color: TYPE_COLORS[item.type], fontSize: 13 }}>{item.type}</span>
        <span style={{ fontSize: 10, padding: "2px 8px", borderRadius: 4,
          background: `${siColor(item.si)}15`, border: `1px solid ${siColor(item.si)}40`,
          color: siColor(item.si) }}>{siLabel(item.si)}</span>
      </div>
      <div style={{ fontSize: 11, color: MUTED, marginBottom: 8,
        display: "grid", gridTemplateColumns: "1fr 1fr", rowGap: 3 }}>
        <span>X̄ <b style={{ color: TEXT }}>{item.mean.toFixed(1)}d</b></span>
        <span>UNPL <b style={{ color: ALARM }}>{item.unpl.toFixed(1)}d</b></span>
        <span>P50 <b style={{ color: SECONDARY }}>{item.elt}d</b></span>
        <span>SI <b style={{ color: siColor(item.si) }}>{item.si.toFixed(2)}</b></span>
        {item.outliers > 0 && <span style={{ color: ALARM }}>⚠ {item.outliers} outlier{item.outliers > 1 ? "s" : ""}</span>}
        {item.shifts   > 0 && <span style={{ color: CAUTION }}>⇶ {item.shifts} shift{item.shifts > 1 ? "s" : ""}</span>}
      </div>
      <MiniChart type={item.type} data={item.data} mean={item.mean} unpl={item.unpl}/>
    </div>
  );
}

// ── CHART PANEL ───────────────────────────────────────────────────────────────

function ChartPanel({ data, mean, unpl, lnpl, typeColor }) {
  const [logScale, setLogScale] = useState(CFG.defaultLogScale);
  const yMax     = Math.ceil((Math.max(...data.map(d => d.value), unpl) * 1.1) / CFG.ySnapStep) * CFG.ySnapStep;
  const interval = Math.max(1, Math.floor(data.length / CFG.xTickCount));
  const outlierCount = data.filter(d => d.isOutlier).length;
  const shiftCount   = data.filter(d => d.isShift).length;

  function TooltipWithCtx(props) {
    return <CustomTooltip {...props} mean={mean} unpl={unpl} />;
  }

  return (
    <div style={{ background: PANEL_BG, borderRadius: 12,
      border: `1px solid ${BORDER}`, padding: "20px 12px 12px 12px", marginBottom: 20 }}>

      <div style={{ display: "flex", flexWrap: "wrap", gap: 8, marginBottom: 16, alignItems: "center" }}>
        {outlierCount > 0 && <Badge text={`⚠ ${outlierCount} outlier${outlierCount > 1 ? "s" : ""} above UNPL`} color={ALARM}/>}
        {shiftCount   > 0 && <Badge text={`⇶ ${shiftCount} process shift${shiftCount > 1 ? "s" : ""} detected`}  color={CAUTION}/>}
        <div style={{ flex: 1 }}/>
        <button onClick={() => setLogScale(s => !s)} style={{
          fontSize: 10, padding: "5px 14px", borderRadius: 6, cursor: "pointer",
          background: logScale ? `${SECONDARY}18` : BORDER,
          border: `1.5px solid ${logScale ? SECONDARY : MUTED}`,
          color: logScale ? SECONDARY : TEXT,
          fontFamily: FONT_STACK, fontWeight: 700,
        }}>
          {logScale ? "LOG" : "LINEAR"}
        </button>
      </div>

      <div style={{ height: CFG.mainChartHeight }}>
        <ResponsiveContainer width="100%" height="100%">
          <ComposedChart data={data} margin={CFG.mainMargin}>
            <CartesianGrid strokeDasharray="3 3" stroke={BORDER} vertical={false}/>
            <XAxis dataKey="date" tickFormatter={formatDate} interval={interval}
              angle={-45} textAnchor="end" height={60}
              tick={{ fill: MUTED, fontSize: 11, fontFamily: FONT_STACK }}/>
            <YAxis scale={logScale ? "log" : "auto"}
              domain={logScale ? [0.01, yMax] : [0, yMax]}
              allowDataOverflow
              tickFormatter={v => `${v}d`}
              tick={{ fill: MUTED, fontSize: 11, fontFamily: FONT_STACK }}
              label={{ value: "Cycle Time (days)", angle: -90, position: "insideLeft",
                fill: MUTED, fontSize: 11, fontFamily: FONT_STACK }}/>
            <Tooltip content={TooltipWithCtx}/>
            <ReferenceLine y={unpl} stroke={XMR_UNPL} strokeDasharray="6 3" strokeWidth={1.5}
              label={{ value: "UNPL", fill: XMR_UNPL, fontSize: 10,
                fontFamily: FONT_STACK, position: "insideTopRight" }}/>
            <ReferenceLine y={mean} stroke={typeColor} strokeDasharray="4 4" strokeWidth={1.5}
              label={{ value: "X̄", fill: typeColor, fontSize: 10,
                fontFamily: FONT_STACK, position: "insideTopRight" }}/>
            {lnpl > 0 && (
              <ReferenceLine y={lnpl} stroke={XMR_LNPL} strokeDasharray="4 4" strokeWidth={1}
                label={{ value: "LNPL", fill: XMR_LNPL, fontSize: 10,
                  fontFamily: FONT_STACK, position: "insideBottomRight" }}/>
            )}
            <Line dataKey="value" dot={CustomDot(typeColor)} activeDot={false}
              stroke="none" strokeWidth={0} isAnimationActive={false}/>
          </ComposedChart>
        </ResponsiveContainer>
      </div>

      <div style={{ display: "flex", flexWrap: "wrap", gap: 16, justifyContent: "center", marginTop: 8 }}>
        <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
          <div style={{ width: 8, height: 8, borderRadius: "50%", background: typeColor, opacity: 0.5 }} />
          <span style={{ fontSize: 11, color: MUTED }}>Cycle Time (individual items)</span>
        </div>
        <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
          <div style={{ width: 24, height: 0, borderTop: `1.5px dashed ${XMR_UNPL}` }} />
          <span style={{ fontSize: 11, color: MUTED }}>UNPL</span>
        </div>
        <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
          <div style={{ width: 24, height: 0, borderTop: `1.5px dashed ${typeColor}` }} />
          <span style={{ fontSize: 11, color: MUTED }}>X̄ Mean</span>
        </div>
        <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
          <div style={{ width: 10, height: 10, borderRadius: "50%", background: ALARM }} />
          <span style={{ fontSize: 11, color: MUTED }}>Outlier (above UNPL)</span>
        </div>
        <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
          <div style={{ width: 8, height: 8, borderRadius: "50%", background: CAUTION }} />
          <span style={{ fontSize: 11, color: MUTED }}>Process Shift anchor</span>
        </div>
      </div>
    </div>
  );
}

// ── MAIN EXPORT ───────────────────────────────────────────────────────────────

export default function ProcessStabilityChart() {
  const [activeTab, setActiveTab] = useState("Overall");

  const overallData = buildChartData(RAW_SCATTERPLOT, OVERALL_XMR.signals);
  const isOverall   = activeTab === "Overall";

  const xmr       = isOverall ? OVERALL_XMR : STRATIFIED[activeTab]?.xmr;
  const si        = isOverall ? OVERALL_SI  : (STRATIFIED[activeTab]?.stability_index ?? 0);
  const elt       = isOverall ? OVERALL_ELT : (STRATIFIED[activeTab]?.expected_lead_time ?? 0);
  const tabColor = TYPE_COLORS[activeTab] ?? TEXT;

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
    <div style={{ background: PAGE_BG, minHeight: "100vh", padding: "32px 24px",
      fontFamily: FONT_STACK, color: TEXT }}>
      <div style={{ maxWidth: 1100, margin: "0 auto" }}>

        <div style={{ fontSize: 11, color: MUTED, letterSpacing: "0.08em",
          textTransform: "uppercase", marginBottom: 8 }}>
          {PROJECT_KEY} · {BOARD_NAME} · Board {BOARD_ID}
        </div>

        <h1 style={{ fontSize: 26, fontWeight: 700, margin: "0 0 4px 0" }}>Process Stability</h1>
        <div style={{ fontSize: 13, color: MUTED, marginBottom: 20 }}>
          Cycle Time Scatterplot · Individual items by completion date · {dateRange}
        </div>

        <div style={{ display: "flex", flexWrap: "wrap", gap: 12, marginBottom: 16 }}>
          <StatCard label="X̄ MEAN"      value={`${mean.toFixed(1)}d`} color={tabColor} valueSize={20}/>
          <StatCard label="UNPL"         value={`${unpl.toFixed(1)}d`} color={XMR_UNPL} valueSize={20}/>
          <StatCard label="EXPECTED P50" value={`${elt}d`}             color={SECONDARY} valueSize={20}/>
          <StatCard label="STAB. INDEX"  value={si.toFixed(2)}         color={siColor(si)} valueSize={20}/>
        </div>

        <div style={{ display: "flex", flexWrap: "wrap", gap: 8, marginBottom: 24, alignItems: "center" }}>
          <Badge text={siLabel(si)} color={siColor(si)}/>
          {outlierCount > 0 && <Badge text={`⚠ ${outlierCount} outlier${outlierCount > 1 ? "s" : ""} above UNPL`} color={ALARM}/>}
          {shiftCount   > 0 && <Badge text={`⇶ ${shiftCount} process shift${shiftCount > 1 ? "s" : ""} detected`}  color={CAUTION}/>}
        </div>

        <div style={{ display: "flex", flexWrap: "wrap", gap: 8, marginBottom: 20 }}>
          {TABS.filter(t => t === "Overall" || STRATIFIED[t]).map(t => {
            const tSi    = t === "Overall" ? OVERALL_SI : (STRATIFIED[t]?.stability_index ?? 0);
            const tColor = TYPE_COLORS[t] ?? TEXT;
            const active = activeTab === t;
            return (
              <button key={t} onClick={() => setActiveTab(t)} style={{
                padding: "6px 14px", borderRadius: 6, cursor: "pointer",
                background: active ? `${tColor}18` : PANEL_BG,
                border: `1.5px solid ${active ? tColor : BORDER}`,
                color: active ? tColor : MUTED,
                fontFamily: FONT_STACK, fontSize: 12,
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
          typeColor={tabColor}
        />

        {isOverall && typeSummary.length > 0 && (
          <div style={{
            display: "grid",
            gridTemplateColumns: `repeat(${Math.min(typeSummary.length, 3)}, 1fr)`,
            gap: 16, marginBottom: 24,
          }}>
            {typeSummary.map(item => <TypeCard key={item.type} item={item} />)}
          </div>
        )}

        <div style={{ fontSize: 11, color: MUTED, lineHeight: 1.7,
          borderTop: `1px solid ${BORDER}`, paddingTop: 16 }}>
          <div style={{ marginBottom: 8 }}>
            <b style={{ color: TEXT }}>Reading this chart:</b> Each point is the cycle
            time of one delivered item, plotted by its completion date. Multiple items may share
            a date — this is a scatterplot, not a line chart. The dashed X̄ is the process mean.
            The UNPL defines the outer boundary of natural variation — points above it are outliers
            caused by special circumstances, not normal process noise. Yellow dots mark the anchor
            of an 8-point run on one side of the mean (Process Shift signal). The Stability Index
            (WIP ÷ Throughput ÷ Mean Cycle Time) summarises system health: below {CFG.siMarginal} =
            stable, {CFG.siMarginal}–{CFG.siClogged} = marginal, above {CFG.siClogged} = clogged.
          </div>
          <div>
            <b style={{ color: TEXT }}>Data provenance:</b> Wheeler XmR applied to
            individual item cycle times. Signals: outlier = above UNPL; shift = 8+ consecutive
            points on one side of X̄. Values near 0 indicate items committed and resolved the
            same day.
          </div>
        </div>

      </div>
    </div>
  );
}
