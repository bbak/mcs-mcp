import { useState } from "react";
import {
  BarChart, Bar, Cell, ComposedChart, Line, Scatter,
  ReferenceLine, XAxis, YAxis,
  CartesianGrid, Tooltip, ResponsiveContainer,
} from "recharts";
import { ALARM, CAUTION, PRIMARY, SECONDARY, POSITIVE, TEXT, MUTED, PAGE_BG, PANEL_BG, BORDER, typeColor, percColor, FONT_STACK } from "mcs-mcp";
import { StatCard, Badge, TOOLTIP_BG } from "./shared.jsx";

// ── INJECTED DATA ─────────────────────────────────────────────────────────────
// Payload is injected by the MCS chart renderer as window.__MCS_PAYLOAD__.

const __MCS_ENVELOPE__ = window.__MCS_PAYLOAD__;
const __MCS_DATA__ = __MCS_ENVELOPE__.data;
const __MCS_GUARDRAILS__ = __MCS_ENVELOPE__.guardrails;
const __MCS_WORKFLOW__ = __MCS_ENVELOPE__.workflow;
// ── CONFIG ────────────────────────────────────────────────────────────────────


const PERC_KEYS = [
  "aggressive","unlikely","coin_toss","probable",
  "likely","conservative","safe","almost_certain",
];

const PERC_SHORT = {
  aggressive:     "P10 · Aggressive",
  unlikely:       "P30 · Unlikely",
  coin_toss:      "P50 · Coin Toss",
  probable:       "P70 · Probable",
  likely:         "P85 · SLE",
  conservative:   "P90 · Conservative",
  safe:           "P95 · Safe Bet",
  almost_certain: "P98 · Almost Certain",
};


// ── DERIVED ───────────────────────────────────────────────────────────────────

const d   = __MCS_DATA__;
const ctx = d.context;

const BOARD_ID    = __MCS_WORKFLOW__.board_id;
const PROJECT_KEY = __MCS_WORKFLOW__.project_key;
const BOARD_NAME  = __MCS_WORKFLOW__.board_name;

const POOL = d.percentiles;
const IQR            = d.spread.iqr;
const INNER_80       = d.spread.inner_80;
const FAT_TAIL_RATIO = d.fat_tail_ratio;
const TAIL_TO_MEDIAN = d.tail_to_median_ratio;
const PREDICTABILITY = d.predictability;
const THROUGHPUT_DIR = d.throughput_trend.direction;
const THROUGHPUT_PCT = d.throughput_trend.percentage_change;
const MODELING_INSIGHT = ctx.modeling_insight;
const ISSUES_ANALYZED  = ctx.issues_analyzed;
const ISSUES_TOTAL     = ctx.issues_total;
const DAYS_IN_SAMPLE   = ctx.days_in_sample;
const DROPPED_OUTCOME  = ctx.dropped_by_outcome;

const ALL_ISSUE_TYPES = ctx.stratification_decisions.map(dd => dd.type);
const TYPE_SLES = Object.fromEntries(
  ctx.stratification_decisions.map(dd => [dd.type, {
    eligible: dd.eligible,
    volume:   dd.volume,
    reason:   dd.reason,
    ...(d.type_sles[dd.type] || {}),
  }])
);
const ISSUE_TYPE_COLORS = Object.fromEntries(
  ALL_ISSUE_TYPES.map(t => [t, typeColor(t, ALL_ISSUE_TYPES)])
);

const eligibleTypes   = ALL_ISSUE_TYPES.filter(t => TYPE_SLES[t].eligible);
const ineligibleTypes = ALL_ISSUE_TYPES.filter(t => !TYPE_SLES[t].eligible);

const round1 = v => Math.round((v || 0) * 10) / 10;

// Pool bar data
const poolData = PERC_KEYS.map(k => ({
  key:   k,
  label: PERC_SHORT[k],
  days:  round1(POOL[k]),
  color: percColor(k),
  isSLE: k === "likely",
}));

// Per-type bar data — include ALL types (eligible + ineligible).
// Stratification eligibility still gates the simulation math, but the chart
// shows every type so users can see how close each stream sits to the pool.
const typeData = PERC_KEYS.map(k => {
  const row = { key: k, label: PERC_SHORT[k], isSLE: k === "likely" };
  ALL_ISSUE_TYPES.forEach(t => { row[t] = round1(TYPE_SLES[t][k]); });
  return row;
});

const P85 = round1(POOL.likely);
const P98 = round1(POOL.almost_certain);
const maxTypeP98Display = ALL_ISSUE_TYPES.length > 0
  ? Math.max(...ALL_ISSUE_TYPES.map(t => round1(TYPE_SLES[t].almost_certain || 0)), P98)
  : P98;

// ── SCATTERPLOT DATA ─────────────────────────────────────────────────────────

const RAW_SCATTERPLOT = d.scatterplot || [];
const P50 = round1(POOL.coin_toss);
const P70 = round1(POOL.probable);
const P95 = round1(POOL.safe);

const scatterData = RAW_SCATTERPLOT.map(pt => ({
  date:      pt.date,
  value:     pt.value,
  key:       pt.key,
  issueType: pt.issue_type,
  aboveSLE:  pt.value > POOL.likely,
}));

const formatDate = (d) =>
  new Date(d + "T00:00:00").toLocaleDateString("en-GB", { day: "2-digit", month: "short" });

const scatterYMax = scatterData.length > 0
  ? Math.ceil(Math.max(...scatterData.map(d => d.value), P95) * 1.1 / 50) * 50
  : P98;

const scatterInterval = Math.max(1, Math.floor(scatterData.length / 9));

// ── SLE ADHERENCE DATA ───────────────────────────────────────────────────────

const ADHERENCE = d.sle_adherence || null;
const adherenceData = ADHERENCE
  ? ADHERENCE.buckets.map(b => ({
      label:        b.bucket_label,
      delivered:    b.delivered_count,
      breaches:     b.breach_count,
      attainmentPct: Math.round((b.attainment_rate || 0) * 100),
      maxCT:        round1(b.max_cycle_time_days),
      p95Excess:    round1(b.p95_breach_magnitude_days),
      isPartial:    !!b.is_partial,
    }))
  : [];

const expectedRatePct = ADHERENCE ? Math.round(ADHERENCE.expected_attainment_rate * 100) : 0;
const overallRatePct  = ADHERENCE ? Math.round(ADHERENCE.overall_attainment_rate * 100) : 0;
const sleSourceLabel = ADHERENCE
  ? (ADHERENCE.sle_source === "user"
      ? `user-supplied (${ADHERENCE.sle_duration_days}d)`
      : `auto-derived P${ADHERENCE.sle_percentile} (${ADHERENCE.sle_duration_days}d)`)
  : "";

const adherenceBarColor = (rate) => {
  if (rate >= expectedRatePct) return POSITIVE;
  if (rate >= expectedRatePct - 10) return CAUTION;
  return ALARM;
};

const adherenceInterval = Math.max(0, Math.floor(adherenceData.length / 12));
const severityYMax = adherenceData.length > 0
  ? Math.ceil(Math.max(...adherenceData.map(a => Math.max(a.maxCT, ADHERENCE.sle_duration_days))) * 1.1)
  : 0;

function ScatterDot({ cx, cy, payload }) {
  if (!cx || !cy) return null;
  const color = ISSUE_TYPE_COLORS[payload.issueType] ?? PRIMARY;
  if (payload.aboveSLE)
    return <circle cx={cx} cy={cy} r={4} fill={color} stroke={CAUTION} strokeWidth={1.5}/>;
  return <circle cx={cx} cy={cy} r={2.5} fill={color} fillOpacity={0.5} stroke="none"/>;
}

function ScatterTooltip({ active, payload }) {
  if (!active || !payload?.length) return null;
  const pt = payload[0].payload;
  const color = ISSUE_TYPE_COLORS[pt.issueType] ?? PRIMARY;
  return (
    <div style={{ background: TOOLTIP_BG, border: `1px solid ${BORDER}`, borderRadius: 8,
      padding: "10px 14px", fontFamily: FONT_STACK, fontSize: 12, color: TEXT }}>
      <div style={{ fontWeight: 700, marginBottom: 4 }}>{pt.key}</div>
      <div style={{ color: MUTED, marginBottom: 6 }}>{formatDate(pt.date)}</div>
      <div style={{ borderTop: `1px solid ${BORDER}`, paddingTop: 6 }}>
        <div>Cycle Time: <b>{pt.value.toFixed(1)} d</b></div>
        <div>Type: <span style={{ color }}>{pt.issueType}</span></div>
        <div style={{ color: MUTED }}>P50: {P50}d · P70: {P70}d · P85: {P85}d · P95: {P95}d</div>
        {pt.aboveSLE && <div style={{ color: CAUTION, fontWeight: 700, marginTop: 4 }}>Above P85 SLE</div>}
      </div>
    </div>
  );
}

// ── HELPERS ───────────────────────────────────────────────────────────────────

const fatTailColor = FAT_TAIL_RATIO >= 5.6 ? ALARM : FAT_TAIL_RATIO >= 3 ? CAUTION : POSITIVE;
const tailMedianColor = TAIL_TO_MEDIAN >= 3 ? ALARM : TAIL_TO_MEDIAN >= 2 ? CAUTION : POSITIVE;
const predColor = (PREDICTABILITY || "").toLowerCase().includes("unstable") ||
                  (PREDICTABILITY || "").toLowerCase().includes("volatile") ? ALARM : CAUTION;

// ── SUB-COMPONENTS ────────────────────────────────────────────────────────────


function PoolTooltip({ active, payload }) {
  if (!active || !payload?.length) return null;
  const d = payload[0].payload;
  return (
    <div style={{ background: TOOLTIP_BG, border: `1px solid ${BORDER}`, borderRadius: 8,
      padding: "10px 14px", fontFamily: FONT_STACK, fontSize: 12, color: TEXT }}>
      <div style={{ fontWeight: 700, color: d.color, marginBottom: 4 }}>{d.label}</div>
      <div>{d.days}d</div>
      {d.isSLE && <div style={{ color: CAUTION, marginTop: 4 }}>← canonical SLE</div>}
    </div>
  );
}

function TypeTooltipRow({ t, v }) {
  return (
    <div style={{ display: "flex", justifyContent: "space-between", gap: 16 }}>
      <span style={{ color: ISSUE_TYPE_COLORS[t] }}>{t}</span>
      <span>{v}d</span>
    </div>
  );
}

function TypeLegendItem({ t, label, opacity }) {
  return (
    <span style={{ fontSize: 10, color: MUTED, opacity: opacity }}>
      <span style={{ color: ISSUE_TYPE_COLORS[t] }}>●</span>{" "}{label}
    </span>
  );
}

function AdherenceTooltip({ active, payload, label }) {
  if (!active || !payload?.length) return null;
  const p = payload[0].payload;
  const delivered = p.delivered;
  const kept = delivered - p.breaches;
  return (
    <div style={{ background: TOOLTIP_BG, border: `1px solid ${BORDER}`, borderRadius: 8,
      padding: "10px 14px", fontFamily: FONT_STACK, fontSize: 12, color: TEXT }}>
      <div style={{ fontWeight: 700, marginBottom: 4 }}>{label}{p.isPartial ? " (partial)" : ""}</div>
      <div>Attainment: <b>{p.attainmentPct}%</b> ({kept}/{delivered})</div>
      <div style={{ color: MUTED, marginTop: 4 }}>Breaches: {p.breaches} · Max CT: {p.maxCT}d</div>
    </div>
  );
}

function SeverityTooltip({ active, payload, label }) {
  if (!active || !payload?.length) return null;
  const p = payload[0].payload;
  return (
    <div style={{ background: TOOLTIP_BG, border: `1px solid ${BORDER}`, borderRadius: 8,
      padding: "10px 14px", fontFamily: FONT_STACK, fontSize: 12, color: TEXT }}>
      <div style={{ fontWeight: 700, marginBottom: 4 }}>{label}{p.isPartial ? " (partial)" : ""}</div>
      <div>Max CT: <b>{p.maxCT}d</b></div>
      <div>P95 breach excess: <b>{p.p95Excess}d</b></div>
      <div style={{ color: MUTED, marginTop: 4 }}>Delivered: {p.delivered} · Breaches: {p.breaches}</div>
    </div>
  );
}

function TypeTooltip({ active, payload, label }) {
  if (!active || !payload?.length) return null;
  const rows = ALL_ISSUE_TYPES.map(t => ({ t, v: payload.find(p => p.dataKey === t)?.value })).filter(r => r.v != null);
  return (
    <div style={{ background: TOOLTIP_BG, border: `1px solid ${BORDER}`, borderRadius: 8,
      padding: "10px 14px", fontFamily: FONT_STACK, fontSize: 12, color: TEXT }}>
      <div style={{ fontWeight: 700, marginBottom: 6 }}>{label}</div>
      {rows.map(({ t, v }) => <TypeTooltipRow key={t} t={t} v={v} />)}
    </div>
  );
}

// ── MAIN EXPORT ───────────────────────────────────────────────────────────────

export default function CycleTimeSleChart() {
  const scale = v => `${Math.min((v / P98) * 100, 100).toFixed(1)}%`;
  const [scatterLogScale, setScatterLogScale] = useState(false);

  return (
    <div style={{ background: PAGE_BG, minHeight: "100vh", padding: "24px 20px",
      fontFamily: FONT_STACK, color: TEXT }}>
      <div style={{ maxWidth: 1100, margin: "0 auto" }}>

        {/* Header */}
        <div style={{ fontSize: 11, color: MUTED, letterSpacing: "0.08em",
          textTransform: "uppercase", marginBottom: 6 }}>
          {PROJECT_KEY} · {BOARD_NAME} · Board {BOARD_ID}
        </div>
        <h1 style={{ fontSize: 22, fontWeight: 700, margin: "0 0 4px" }}>Cycle Time SLE</h1>
        <div style={{ fontSize: 12, color: MUTED, marginBottom: 16 }}>
          Service Level Expectations · {ISSUES_ANALYZED} of {ISSUES_TOTAL} items analyzed · {DAYS_IN_SAMPLE} days
        </div>

        {/* Stat cards */}
        <div style={{ display: "flex", flexWrap: "wrap", gap: 10, marginBottom: 14 }}>
          <StatCard label="P50 MEDIAN"   value={`${round1(POOL.coin_toss)}d`}  color={percColor("coin_toss")} />
          <StatCard label="P85 SLE"      value={`${P85}d`}                      color={percColor("likely")} />
          <StatCard label="P95 SAFE BET" value={`${round1(POOL.safe)}d`}        color={percColor("safe")} />
          <StatCard label="FAT-TAIL ×"   value={`${FAT_TAIL_RATIO}×`}
            sub="≥5.6 = extreme"                                                 color={ALARM} />
          <StatCard label="ANALYZED"     value={`${ISSUES_ANALYZED} / ${ISSUES_TOTAL}`}
            sub={`−${DROPPED_OUTCOME} by outcome`}                               color={MUTED} />
        </div>

        {/* Panel 1: Predictability / Fat-Tail / Spread */}
        <div style={{ background: PANEL_BG, borderRadius: 12,
          border: `1px solid ${BORDER}`, padding: "14px 12px", marginBottom: 16 }}>
          <div style={{ fontSize: 11, color: MUTED, marginBottom: 10, letterSpacing: "0.05em",
            textTransform: "uppercase" }}>Predictability & Spread</div>

          <div style={{ display: "flex", flexWrap: "wrap", gap: 6, marginBottom: 14 }}>
            <Badge text={`Predictability: ${PREDICTABILITY}`} color={predColor} />
            <Badge text={`Fat-tail ratio: ${FAT_TAIL_RATIO}× (P98/P50)`} color={fatTailColor} />
            <Badge text={`Tail-to-median: ${TAIL_TO_MEDIAN}× (P95/P50)`} color={tailMedianColor} />
            <Badge text={`Throughput: ${THROUGHPUT_DIR} ${THROUGHPUT_PCT >= 0 ? "+" : ""}${THROUGHPUT_PCT.toFixed(0)}%`} color={CAUTION} />
            <Badge text={MODELING_INSIGHT} color={MUTED} />
          </div>

          {/* Spread strip */}
          <div style={{ position: "relative", height: 28, background: "#12141e",
            borderRadius: 6, overflow: "hidden", marginBottom: 8 }}>
            {/* Inner 80 band */}
            <div style={{ position: "absolute", top: 8, bottom: 8,
              left: scale(round1(POOL.aggressive)), width: `calc(${scale(round1(POOL.conservative))} - ${scale(round1(POOL.aggressive))})`,
              background: `${PRIMARY}2e`, borderRadius: 3 }} />
            {/* IQR band */}
            <div style={{ position: "absolute", top: 0, bottom: 0,
              left: scale(round1(POOL.unlikely)), width: `calc(${scale(round1(POOL.probable))} - ${scale(round1(POOL.unlikely))})`,
              background: `${PRIMARY}4d`, borderRadius: 3 }} />
            {/* P50 tick */}
            <div style={{ position: "absolute", top: 0, bottom: 0, width: 2,
              left: scale(round1(POOL.coin_toss)), background: percColor("coin_toss") }} />
            {/* P85 tick */}
            <div style={{ position: "absolute", top: 0, bottom: 0, width: 2,
              left: scale(P85), background: percColor("likely") }} />
            {/* P98 tick */}
            <div style={{ position: "absolute", top: 0, bottom: 0, width: 2,
              right: 0, background: percColor("almost_certain") }} />
          </div>

          <div style={{ display: "flex", gap: 16, justifyContent: "center", fontSize: 10, color: MUTED }}>
            <span><span style={{ color: percColor("aggressive") }}>●</span> P10</span>
            <span><span style={{ color: percColor("coin_toss") }}>●</span> P50</span>
            <span><span style={{ color: percColor("likely") }}>●</span> P85</span>
            <span><span style={{ color: percColor("almost_certain") }}>●</span> P98</span>
          </div>
        </div>

        {/* Panel 2: Cycle Time Scatterplot */}
        {scatterData.length > 0 && (
          <div style={{ background: PANEL_BG, borderRadius: 12,
            border: `1px solid ${BORDER}`, padding: "14px 12px 12px", marginBottom: 16 }}>
            <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center",
              marginBottom: 8 }}>
              <div style={{ fontSize: 11, color: MUTED, letterSpacing: "0.05em",
                textTransform: "uppercase" }}>
                Cycle Time Scatterplot — Individual items by completion date · {scatterData.length} items
              </div>
              <button
                onClick={() => setScatterLogScale(s => !s)}
                style={{
                  fontSize: 10, fontFamily: FONT_STACK, padding: "4px 10px", borderRadius: 6,
                  cursor: "pointer", letterSpacing: "0.05em",
                  background: scatterLogScale ? `${SECONDARY}18` : BORDER,
                  border: `1.5px solid ${scatterLogScale ? SECONDARY : MUTED}`,
                  color: scatterLogScale ? SECONDARY : TEXT,
                }}>
                {scatterLogScale ? "LOG" : "LINEAR"}
              </button>
            </div>

            <ResponsiveContainer width="100%" height={570}>
              <ComposedChart data={scatterData} margin={{ top: 10, right: 20, bottom: 5, left: 10 }}>
                <CartesianGrid strokeDasharray="3 3" stroke={BORDER} vertical={false} />
                <XAxis dataKey="date" tickFormatter={formatDate} interval={scatterInterval}
                  angle={-45} textAnchor="end" height={60}
                  tick={{ fill: MUTED, fontSize: 11, fontFamily: FONT_STACK }} />
                <YAxis
                  scale={scatterLogScale ? "log" : "auto"}
                  domain={scatterLogScale ? [0.1, scatterYMax] : [0, scatterYMax]}
                  allowDataOverflow
                  tickFormatter={v => `${v}d`}
                  tick={{ fill: MUTED, fontSize: 11, fontFamily: FONT_STACK }}
                  label={{ value: "Cycle Time (days)", angle: -90, position: "insideLeft",
                    fill: MUTED, fontSize: 11, fontFamily: FONT_STACK }} />
                <Tooltip content={ScatterTooltip} />
                <ReferenceLine y={P50} stroke={percColor("coin_toss")} strokeDasharray="4 4" strokeWidth={1.5}
                  label={{ value: `P50 ${P50}d`, fill: percColor("coin_toss"), fontSize: 10,
                    fontFamily: FONT_STACK, position: "insideTopRight" }} />
                <ReferenceLine y={P70} stroke={percColor("probable")} strokeDasharray="4 4" strokeWidth={1.5}
                  label={{ value: `P70 ${P70}d`, fill: percColor("probable"), fontSize: 10,
                    fontFamily: FONT_STACK, position: "insideTopRight" }} />
                <ReferenceLine y={P85} stroke={percColor("likely")} strokeDasharray="6 3" strokeWidth={1.5}
                  label={{ value: `P85 SLE ${P85}d`, fill: percColor("likely"), fontSize: 10,
                    fontFamily: FONT_STACK, position: "insideTopRight" }} />
                <ReferenceLine y={P95} stroke={percColor("safe")} strokeDasharray="4 4" strokeWidth={1.5}
                  label={{ value: `P95 ${P95}d`, fill: percColor("safe"), fontSize: 10,
                    fontFamily: FONT_STACK, position: "insideTopRight" }} />
                <Scatter dataKey="value" shape={<ScatterDot />} isAnimationActive={false} />
              </ComposedChart>
            </ResponsiveContainer>

            <div style={{ display: "flex", flexWrap: "wrap", gap: 16, justifyContent: "center", marginTop: 8 }}>
              {ALL_ISSUE_TYPES.map(t => (
                <TypeLegendItem key={t} t={t} label={t} opacity={1} />
              ))}
              <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
                <div style={{ width: 24, height: 0, borderTop: `1.5px dashed ${percColor("coin_toss")}` }} />
                <span style={{ fontSize: 10, color: MUTED }}>P50 Median</span>
              </div>
              <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
                <div style={{ width: 24, height: 0, borderTop: `1.5px dashed ${percColor("probable")}` }} />
                <span style={{ fontSize: 10, color: MUTED }}>P70 Probable</span>
              </div>
              <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
                <div style={{ width: 24, height: 0, borderTop: `1.5px dashed ${percColor("likely")}` }} />
                <span style={{ fontSize: 10, color: MUTED }}>P85 SLE</span>
              </div>
              <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
                <div style={{ width: 24, height: 0, borderTop: `1.5px dashed ${percColor("safe")}` }} />
                <span style={{ fontSize: 10, color: MUTED }}>P95 Safe Bet</span>
              </div>
            </div>
          </div>
        )}

        {/* Panel 3: Pool SLE */}
        <div style={{ background: PANEL_BG, borderRadius: 12,
          border: `1px solid ${BORDER}`, padding: "14px 8px 12px", marginBottom: 16 }}>
          <div style={{ fontSize: 11, color: MUTED, marginBottom: 8 }}>
            Pool SLE — All issue types combined · {ISSUES_ANALYZED} items · {DAYS_IN_SAMPLE} days
          </div>
          <ResponsiveContainer width="100%" height={280}>
            <BarChart data={poolData} layout="vertical"
              margin={{ top: 4, right: 80, left: 140, bottom: 4 }}>
              <CartesianGrid strokeDasharray="3 3" stroke={BORDER} horizontal={false} />
              <XAxis type="number" domain={[0, Math.ceil(P98 * 1.05)]} tickFormatter={v => `${v}d`}
                tick={{ fill: MUTED, fontSize: 10, fontFamily: FONT_STACK }} />
              <YAxis type="category" dataKey="label" width={130}
                tick={{ fill: TEXT, fontSize: 10, fontFamily: FONT_STACK }} />
              <Tooltip content={PoolTooltip} cursor={{ fill: `${PRIMARY}0c` }} />
              <Bar dataKey="days" radius={[0, 4, 4, 0]} barSize={22} isAnimationActive={false}>
                {poolData.map((d, i) => (
                  <Cell key={`cell-${i}`} fill={d.color} fillOpacity={d.isSLE ? 1.0 : 0.55} />
                ))}
              </Bar>
              <ReferenceLine x={P85} stroke={percColor("likely")} strokeDasharray="4 3" strokeWidth={1.5}
                label={{ value: `SLE ${P85}d`, fill: percColor("likely"), fontSize: 10, position: "right" }} />
            </BarChart>
          </ResponsiveContainer>
          <div style={{ display: "flex", gap: 16, justifyContent: "center", fontSize: 10, color: MUTED, marginTop: 6 }}>
            <span>IQR (P25–P75): {IQR}d</span>
            <span>Inner 80 (P10–P90): {INNER_80}d</span>
          </div>
        </div>

        {/* Panel 4: Per-Type SLE */}
        {ALL_ISSUE_TYPES.length > 0 && (
          <div style={{ background: PANEL_BG, borderRadius: 12,
            border: `1px solid ${BORDER}`, padding: "14px 8px 12px", marginBottom: 16 }}>
            <div style={{ fontSize: 11, color: MUTED, marginBottom: 8 }}>
              Per-type SLE comparison · all streams · P85 highlighted · faded = pooled (ineligible for stratified simulation)
            </div>
            <ResponsiveContainer width="100%" height={280}>
              <BarChart data={typeData} layout="vertical"
                margin={{ top: 4, right: 80, left: 140, bottom: 4 }}>
                <CartesianGrid strokeDasharray="3 3" stroke={BORDER} horizontal={false} />
                <XAxis type="number" domain={[0, Math.ceil(maxTypeP98Display * 1.05)]} tickFormatter={v => `${v}d`}
                  tick={{ fill: MUTED, fontSize: 10, fontFamily: FONT_STACK }} />
                <YAxis type="category" dataKey="label" width={130}
                  tick={({ x, y, payload }) => (
                    <text x={x} y={y} dy={4} textAnchor="end"
                      fill={payload.value === PERC_SHORT.likely ? percColor("likely") : TEXT}
                      fontWeight={payload.value === PERC_SHORT.likely ? 700 : 400}
                      fontSize={10} fontFamily={FONT_STACK}>
                      {payload.value}
                    </text>
                  )} />
                <Tooltip content={TypeTooltip} cursor={{ fill: `${PRIMARY}0c` }} />
                {ALL_ISSUE_TYPES.map(t => (
                  <Bar key={t} dataKey={t} barSize={7} radius={[0, 3, 3, 0]}
                    fill={ISSUE_TYPE_COLORS[t]}
                    fillOpacity={TYPE_SLES[t].eligible ? 0.75 : 0.3}
                    isAnimationActive={false} />
                ))}
              </BarChart>
            </ResponsiveContainer>

            <div style={{ display: "flex", flexWrap: "wrap", gap: 12, justifyContent: "center",
              marginTop: 8 }}>
              {ALL_ISSUE_TYPES.map(t => {
                const sles = TYPE_SLES[t];
                const label = sles.eligible
                  ? `${t} (n=${sles.volume})`
                  : `${t} (n=${sles.volume}) — ${sles.reason}`;
                return <TypeLegendItem key={t} t={t} label={label} opacity={sles.eligible ? 1 : 0.6} />;
              })}
            </div>
          </div>
        )}

        {/* Panel 5: SLE Attainment Trend */}
        {ADHERENCE && adherenceData.length > 0 && (
          <div style={{ background: PANEL_BG, borderRadius: 12,
            border: `1px solid ${BORDER}`, padding: "14px 12px 12px", marginBottom: 16 }}>
            <div style={{ fontSize: 11, color: MUTED, marginBottom: 4, letterSpacing: "0.05em",
              textTransform: "uppercase" }}>
              SLE Attainment Trend — weekly · {adherenceData.length} buckets
            </div>
            <div style={{ fontSize: 11, color: MUTED, marginBottom: 8 }}>
              SLE: <b style={{ color: TEXT }}>{ADHERENCE.sle_duration_days}d</b>
              {" · source: "}<span style={{ color: TEXT }}>{sleSourceLabel}</span>
              {" · expected: "}<span style={{ color: percColor("likely") }}>{expectedRatePct}%</span>
              {" · overall: "}<span style={{ color: TEXT }}>{overallRatePct}%</span>
              {" · fat-tail (P95/P85): "}<span style={{ color: TEXT }}>{ADHERENCE.fat_tail_ratio_p95_p85}×</span>
            </div>
            <ResponsiveContainer width="100%" height={260}>
              <ComposedChart data={adherenceData} margin={{ top: 10, right: 20, bottom: 5, left: 10 }}>
                <CartesianGrid strokeDasharray="3 3" stroke={BORDER} vertical={false} />
                <XAxis dataKey="label" interval={adherenceInterval} angle={-45} textAnchor="end" height={60}
                  tick={{ fill: MUTED, fontSize: 10, fontFamily: FONT_STACK }} />
                <YAxis domain={[0, 100]} tickFormatter={v => `${v}%`}
                  tick={{ fill: MUTED, fontSize: 11, fontFamily: FONT_STACK }}
                  label={{ value: "Attainment", angle: -90, position: "insideLeft",
                    fill: MUTED, fontSize: 11, fontFamily: FONT_STACK }} />
                <Tooltip content={AdherenceTooltip} cursor={{ fill: `${PRIMARY}0c` }} />
                <Bar dataKey="attainmentPct" radius={[3, 3, 0, 0]} barSize={18} isAnimationActive={false}>
                  {adherenceData.map((b, i) => (
                    <Cell key={`a-${i}`} fill={adherenceBarColor(b.attainmentPct)}
                      fillOpacity={b.isPartial ? 0.45 : 0.9} />
                  ))}
                </Bar>
                <ReferenceLine y={expectedRatePct} stroke={percColor("likely")} strokeDasharray="6 3" strokeWidth={1.5}
                  label={{ value: `Expected ${expectedRatePct}%`, fill: percColor("likely"), fontSize: 10,
                    fontFamily: FONT_STACK, position: "insideTopRight" }} />
                <ReferenceLine y={overallRatePct} stroke={MUTED} strokeDasharray="3 3" strokeWidth={1}
                  label={{ value: `Overall ${overallRatePct}%`, fill: MUTED, fontSize: 10,
                    fontFamily: FONT_STACK, position: "insideBottomRight" }} />
              </ComposedChart>
            </ResponsiveContainer>
            <div style={{ display: "flex", flexWrap: "wrap", gap: 14, justifyContent: "center", fontSize: 10,
              color: MUTED, marginTop: 6 }}>
              <span><span style={{ color: POSITIVE }}>●</span> at or above expected</span>
              <span><span style={{ color: CAUTION }}>●</span> within 10pp below</span>
              <span><span style={{ color: ALARM }}>●</span> further below</span>
              <span style={{ opacity: 0.5 }}>(faded = partial bucket)</span>
            </div>
          </div>
        )}

        {/* Panel 6: Breach Severity */}
        {ADHERENCE && adherenceData.length > 0 && (
          <div style={{ background: PANEL_BG, borderRadius: 12,
            border: `1px solid ${BORDER}`, padding: "14px 12px 12px", marginBottom: 16 }}>
            <div style={{ fontSize: 11, color: MUTED, marginBottom: 8, letterSpacing: "0.05em",
              textTransform: "uppercase" }}>
              Breach Severity — max cycle time + P95 of breach excess per week
            </div>
            <ResponsiveContainer width="100%" height={260}>
              <ComposedChart data={adherenceData} margin={{ top: 10, right: 20, bottom: 5, left: 10 }}>
                <CartesianGrid strokeDasharray="3 3" stroke={BORDER} vertical={false} />
                <XAxis dataKey="label" interval={adherenceInterval} angle={-45} textAnchor="end" height={60}
                  tick={{ fill: MUTED, fontSize: 10, fontFamily: FONT_STACK }} />
                <YAxis domain={[0, severityYMax || "auto"]} tickFormatter={v => `${v}d`}
                  tick={{ fill: MUTED, fontSize: 11, fontFamily: FONT_STACK }}
                  label={{ value: "Days", angle: -90, position: "insideLeft",
                    fill: MUTED, fontSize: 11, fontFamily: FONT_STACK }} />
                <Tooltip content={SeverityTooltip} cursor={{ fill: `${PRIMARY}0c` }} />
                <Bar dataKey="maxCT" name="Max CT (d)" fill={ALARM} fillOpacity={0.55}
                  radius={[3, 3, 0, 0]} barSize={18} isAnimationActive={false} />
                <Line dataKey="p95Excess" name="P95 breach excess (d)" stroke={CAUTION}
                  strokeWidth={2} dot={{ r: 3, fill: CAUTION }} isAnimationActive={false} />
                <ReferenceLine y={ADHERENCE.sle_duration_days} stroke={percColor("likely")}
                  strokeDasharray="6 3" strokeWidth={1.5}
                  label={{ value: `SLE ${ADHERENCE.sle_duration_days}d`, fill: percColor("likely"),
                    fontSize: 10, fontFamily: FONT_STACK, position: "insideTopRight" }} />
              </ComposedChart>
            </ResponsiveContainer>
            <div style={{ display: "flex", flexWrap: "wrap", gap: 14, justifyContent: "center", fontSize: 10,
              color: MUTED, marginTop: 6 }}>
              <span><span style={{ color: ALARM }}>●</span> Max cycle time per week</span>
              <span><span style={{ color: CAUTION }}>●</span> P95 of breach excess</span>
              <span><span style={{ color: percColor("likely") }}>—</span> SLE threshold</span>
            </div>
          </div>
        )}

        {/* Footer */}
        <div style={{ fontSize: 11, color: MUTED, lineHeight: 1.7,
          borderTop: `1px solid ${BORDER}`, paddingTop: 14 }}>
          <div style={{ marginBottom: 6 }}>
            <b style={{ color: TEXT }}>Reading this chart: </b>
            The pool SLE panel shows the full percentile distribution for all item types
            combined — how long it takes 10%, 30%, 50%... 98% of items to complete. P85 is
            the canonical SLE: 85% of items finish within that time. The per-type panel
            compares the same percentiles across independent delivery streams, always
            highlighting P85. The scatterplot shows each individual item by its completion
            date — points above the P85 line exceeded the Service Level Expectation.
          </div>
          <div>
            <b style={{ color: TEXT }}>Warning: </b>
            Fat-tail ratio is {FAT_TAIL_RATIO}× — {FAT_TAIL_RATIO >= 5.6
              ? "extreme outliers dominate the tail, making SLE commitments unreliable"
              : FAT_TAIL_RATIO >= 3
              ? "significant tail risk; SLE commitments should include caveats"
              : "tail risk is moderate; SLE commitments are reasonably reliable"}.
            {DROPPED_OUTCOME > 0 && ` ${DROPPED_OUTCOME} items dropped by outcome (non-deliveries).`}
            {" "}Stratification decisions are automatic based on volume and variance thresholds.
          </div>
        </div>

      </div>
    </div>
  );
}
