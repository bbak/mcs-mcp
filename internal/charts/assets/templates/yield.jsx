import {
  BarChart, Bar, Cell, XAxis, YAxis, CartesianGrid,
  Tooltip, ResponsiveContainer, PieChart, Pie,
} from "recharts";
import { ALARM, CAUTION, PRIMARY, POSITIVE, TEXT, MUTED, PAGE_BG, PANEL_BG, BORDER, typeColor, tierColor, severityColor, FONT_STACK } from "mcs-mcp";
import { StatCard, Badge, TOOLTIP_BG } from "./shared.jsx";

// ── INJECTED DATA ─────────────────────────────────────────────────────────────
// Payload is injected by the MCS chart renderer as window.__MCS_PAYLOAD__.

const __MCS_ENVELOPE__ = window.__MCS_PAYLOAD__;
const __MCS_DATA__ = __MCS_ENVELOPE__.data;
const __MCS_GUARDRAILS__ = __MCS_ENVELOPE__.guardrails;
const __MCS_WORKFLOW__ = __MCS_ENVELOPE__.workflow;
// ── CONFIG ────────────────────────────────────────────────────────────────────

// ── DERIVED ───────────────────────────────────────────────────────────────────

const dd = __MCS_DATA__;

const BOARD_ID    = __MCS_WORKFLOW__.board_id;
const PROJECT_KEY = __MCS_WORKFLOW__.project_key;
const BOARD_NAME  = __MCS_WORKFLOW__.board_name;

const POOL       = dd.yield;
const STRATIFIED = dd.stratified;
const ALL_ISSUE_TYPES = Object.keys(STRATIFIED);

const ISSUE_TYPE_COLORS = Object.fromEntries(
  ALL_ISSUE_TYPES.map(t => [t, typeColor(t, ALL_ISSUE_TYPES)])
);

const inFlight = POOL.totalIngested - POOL.deliveredCount - POOL.abandonedCount;

function yieldColor(rate) {
  if (rate >= 0.80) return POSITIVE;
  if (rate >= 0.65) return CAUTION;
  return ALARM;
}

const pct = v => `${Math.round((v || 0) * 100)}%`;

// Panel 1 data
const yieldData = [
  { type: "Pool", ...POOL },
  ...ALL_ISSUE_TYPES.map(t => ({ type: t, ...STRATIFIED[t] })),
];

// Panel 2 data
const tiers = ["Demand", "Upstream", "Downstream"];
const lossRows = yieldData.map(d => {
  const row = { type: d.type, total: d.totalIngested };
  tiers.forEach(tier => {
    const lp = (d.lossPoints || []).find(l => l.tier === tier);
    row[tier]             = lp ? lp.count      : 0;
    row[`${tier}_pct`]    = lp ? lp.percentage : 0;
    row[`${tier}_avgAge`] = lp ? lp.avgAge     : null;
    row[`${tier}_sev`]    = lp ? lp.severity   : null;
  });
  return row;
});
const maxLoss = Math.ceil(Math.max(...lossRows.map(r => tiers.reduce((s, t) => s + r[t], 0)), 1) * 1.2);

// ── SUB-COMPONENTS ────────────────────────────────────────────────────────────


function YieldDonut({ data, size = 64 }) {
  const other = data.totalIngested - data.deliveredCount - data.abandonedCount;
  const slices = [
    { name: "Delivered", value: data.deliveredCount, color: POSITIVE },
    { name: "Abandoned", value: data.abandonedCount, color: ALARM },
    ...(other > 0 ? [{ name: "Other", value: other, color: MUTED }] : []),
  ];
  const yc = yieldColor(data.overallYieldRate);
  return (
    <div style={{ position: "relative", width: size, height: size }}>
      <PieChart width={size} height={size}>
        <Pie data={slices} dataKey="value" cx="50%" cy="50%"
          innerRadius={size * 0.33} outerRadius={size * 0.47}
          startAngle={90} endAngle={-270} strokeWidth={0}
          isAnimationActive={false}>
          {slices.map((s, i) => <Cell key={i} fill={s.color} />)}
        </Pie>
      </PieChart>
      <div style={{ position: "absolute", top: 0, left: 0, width: "100%", height: "100%",
        display: "flex", alignItems: "center", justifyContent: "center",
        fontSize: 11, fontWeight: 700, color: yc }}>
        {pct(data.overallYieldRate)}
      </div>
    </div>
  );
}

function LossTooltip({ active, payload }) {
  if (!active || !payload?.length) return null;
  const d = payload[0].payload;
  return (
    <div style={{ background: TOOLTIP_BG, border: `1px solid ${BORDER}`, borderRadius: 8,
      padding: "10px 14px", fontFamily: FONT_STACK, fontSize: 12, color: TEXT }}>
      <div style={{ fontWeight: 700, marginBottom: 6 }}>{d.type}</div>
      <div style={{ display: "grid", gridTemplateColumns: "1fr auto", rowGap: 3, columnGap: 16 }}>
        <span style={{ color: MUTED }}>Ingested</span><span>{d.totalIngested}</span>
        <span style={{ color: POSITIVE }}>Delivered</span><span>{d.deliveredCount}</span>
        <span style={{ color: ALARM }}>Abandoned</span><span>{d.abandonedCount}</span>
        <span style={{ color: yieldColor(d.overallYieldRate), fontWeight: 700 }}>Yield</span>
        <span style={{ fontWeight: 700 }}>{pct(d.overallYieldRate)}</span>
      </div>
    </div>
  );
}

function BreakdownTierRow({ tier, d }) {
  if (d[tier] == null || d[tier] === 0) return null;
  const sev = d[`${tier}_sev`];
  return (
    <div style={{ marginBottom: 4 }}>
      <div style={{ color: tierColor(tier) }}>
        {tier}: {d[tier]} items ({pct(d[`${tier}_pct`])})
      </div>
      {sev && (
        <div style={{ fontSize: 10, color: severityColor(sev) }}>
          severity: {sev} · avg age {d[`${tier}_avgAge`]?.toFixed(0)}d
        </div>
      )}
    </div>
  );
}

function BreakdownTooltip({ active, payload }) {
  if (!active || !payload?.length) return null;
  const d = payload[0].payload;
  return (
    <div style={{ background: TOOLTIP_BG, border: `1px solid ${BORDER}`, borderRadius: 8,
      padding: "10px 14px", fontFamily: FONT_STACK, fontSize: 12, color: TEXT }}>
      <div style={{ fontWeight: 700, marginBottom: 6 }}>{d.type}</div>
      {tiers.map(tier => <BreakdownTierRow key={tier} tier={tier} d={d} />)}
    </div>
  );
}

// ── TYPE CARD ─────────────────────────────────────────────────────────────────

function LossPointRow({ lp }) {
  return (
    <div style={{ marginBottom: 6 }}>
      <div style={{ fontSize: 10, display: "flex", gap: 8, flexWrap: "wrap" }}>
        <span style={{ color: tierColor(lp.tier) }}>{lp.tier}</span>
        <span style={{ color: MUTED }}>{lp.count} · {pct(lp.percentage)}</span>
        {lp.avgAge != null && <span style={{ color: MUTED }}>avg {lp.avgAge.toFixed(0)}d</span>}
        <span style={{ color: severityColor(lp.severity) }}>{lp.severity}</span>
      </div>
      <div style={{ height: 4, borderRadius: 2, marginTop: 3,
        background: BORDER, overflow: "hidden" }}>
        <div style={{ height: "100%", borderRadius: 2,
          width: `${Math.min(lp.percentage / 0.15 * 100, 100)}%`,
          background: severityColor(lp.severity), opacity: 0.7 }} />
      </div>
    </div>
  );
}

function TypeCard({ type }) {
  const td = STRATIFIED[type];
  const tc = ISSUE_TYPE_COLORS[type];
  return (
    <div style={{ flex: "1 1 300px", minWidth: 280, maxWidth: "calc(33.33% - 8px)",
      background: PAGE_BG, border: `1px solid ${tc}4d`, borderRadius: 10, padding: "14px 16px" }}>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center",
        marginBottom: 10 }}>
        <span style={{ fontSize: 14, fontWeight: 700, color: tc }}>{type}</span>
        <YieldDonut data={td} size={64} />
      </div>
      <div style={{ fontSize: 11, color: MUTED, display: "grid",
        gridTemplateColumns: "1fr auto", rowGap: 3, marginBottom: 8 }}>
        <span>Ingested</span><span style={{ color: TEXT }}>{td.totalIngested}</span>
        <span>Delivered</span><span style={{ color: POSITIVE }}>{td.deliveredCount}</span>
        <span>Abandoned</span><span style={{ color: ALARM }}>{td.abandonedCount}</span>
      </div>
      {(td.lossPoints || []).map(lp => <LossPointRow key={lp.tier} lp={lp} />)}
    </div>
  );
}

// ── MAIN EXPORT ───────────────────────────────────────────────────────────────

export default function YieldChart() {
  const poolYieldColor = yieldColor(POOL.overallYieldRate);

  return (
    <div style={{ background: PAGE_BG, minHeight: "100vh", padding: "24px 20px",
      fontFamily: FONT_STACK, color: TEXT }}>
      <div style={{ maxWidth: 1100, margin: "0 auto" }}>

        {/* Header */}
        <div style={{ fontSize: 11, color: MUTED, letterSpacing: "0.08em",
          textTransform: "uppercase", marginBottom: 6 }}>
          {PROJECT_KEY} · {BOARD_NAME} · Board {BOARD_ID}
        </div>
        <h1 style={{ fontSize: 22, fontWeight: 700, margin: "0 0 4px" }}>Yield</h1>
        <div style={{ fontSize: 12, color: MUTED, marginBottom: 16 }}>
          Delivery efficiency · what fraction of committed work actually reaches done?
        </div>

        {/* Stat cards */}
        <div style={{ display: "flex", flexWrap: "wrap", gap: 10, marginBottom: 14 }}>
          <StatCard label="OVERALL YIELD" value={pct(POOL.overallYieldRate)} color={poolYieldColor} />
          <StatCard label="TOTAL INGESTED" value={POOL.totalIngested} color={TEXT} />
          <StatCard label="DELIVERED" value={POOL.deliveredCount}
            sub={pct(POOL.deliveredCount / POOL.totalIngested)} color={POSITIVE} />
          <StatCard label="ABANDONED" value={POOL.abandonedCount}
            sub={pct(POOL.abandonedCount / POOL.totalIngested)} color={ALARM} />
          {inFlight > 0 && (
            <StatCard label="IN FLIGHT" value={inFlight}
              sub="neither delivered nor abandoned" color={MUTED} />
          )}
        </div>

        {/* Guardrail badges */}
        <div style={{ display: "flex", flexWrap: "wrap", gap: 6, marginBottom: 20, alignItems: "center" }}>
          <Badge text="Downstream abandonment = High severity (late-stage waste)" color={ALARM} />
          <Badge text="Upstream abandonment = Medium (discovery / refinement gap)" color={CAUTION} />
          <Badge text="Demand abandonment = Low (normal backlog pruning)" color={POSITIVE} />
        </div>

        {/* Panel 1: Yield Rate Comparison */}
        <div style={{ background: PANEL_BG, borderRadius: 12,
          border: `1px solid ${BORDER}`, padding: "14px 8px 12px", marginBottom: 16 }}>
          <div style={{ fontSize: 11, color: MUTED, marginBottom: 8 }}>
            Overall yield rate by issue type · delivered ÷ ingested
          </div>
          <ResponsiveContainer width="100%" height={220}>
            <BarChart data={yieldData} layout="vertical"
              margin={{ top: 4, right: 60, left: 10, bottom: 4 }}>
              <CartesianGrid strokeDasharray="3 3" stroke={BORDER} horizontal={false} />
              <XAxis type="number" domain={[0, 1]} tickFormatter={v => `${Math.round(v * 100)}%`}
                tick={{ fill: MUTED, fontSize: 10, fontFamily: FONT_STACK }} />
              <YAxis type="category" dataKey="type" width={70}
                tick={{ fill: TEXT, fontSize: 11, fontFamily: FONT_STACK }} />
              <Tooltip content={LossTooltip} cursor={{ fill: `${PRIMARY}0c` }} />
              <Bar dataKey="overallYieldRate" barSize={18} radius={[0, 4, 4, 0]} isAnimationActive={false}>
                {yieldData.map((d, i) => (
                  <Cell key={`cell-${i}`}
                    fill={i === 0 ? TEXT : ISSUE_TYPE_COLORS[d.type] || TEXT}
                    fillOpacity={i === 0 ? 0.35 : 0.75} />
                ))}
              </Bar>
            </BarChart>
          </ResponsiveContainer>
          <div style={{ fontSize: 10, color: MUTED, textAlign: "center", marginTop: 4 }}>
            80% yield threshold — above = healthy · below = systemic loss
          </div>
        </div>

        {/* Panel 2: Loss Breakdown by Tier */}
        <div style={{ background: PANEL_BG, borderRadius: 12,
          border: `1px solid ${BORDER}`, padding: "14px 8px 12px", marginBottom: 16 }}>
          <div style={{ fontSize: 11, color: MUTED, marginBottom: 8 }}>
            Abandoned items by tier · where in the workflow is work lost?
          </div>
          <ResponsiveContainer width="100%" height={220}>
            <BarChart data={lossRows} layout="vertical"
              margin={{ top: 4, right: 60, left: 10, bottom: 4 }}>
              <CartesianGrid strokeDasharray="3 3" stroke={BORDER} horizontal={false} />
              <XAxis type="number" domain={[0, maxLoss]}
                tick={{ fill: MUTED, fontSize: 10, fontFamily: FONT_STACK }}
                label={{ value: "abandoned items", position: "insideBottom", offset: -2,
                  fill: MUTED, fontSize: 10 }} />
              <YAxis type="category" dataKey="type" width={70}
                tick={{ fill: TEXT, fontSize: 11, fontFamily: FONT_STACK }} />
              <Tooltip content={BreakdownTooltip} cursor={{ fill: `${PRIMARY}0c` }} />
              <Bar dataKey="Demand"     stackId="a" fill={tierColor("Demand")}     fillOpacity={0.75} barSize={18} radius={[0, 0, 0, 0]} isAnimationActive={false} />
              <Bar dataKey="Upstream"   stackId="a" fill={tierColor("Upstream")}   fillOpacity={0.75} barSize={18} radius={[0, 0, 0, 0]} isAnimationActive={false} />
              <Bar dataKey="Downstream" stackId="a" fill={tierColor("Downstream")} fillOpacity={0.75} barSize={18} radius={[0, 4, 4, 0]} isAnimationActive={false} />
            </BarChart>
          </ResponsiveContainer>
          <div style={{ display: "flex", gap: 12, justifyContent: "center", marginTop: 8 }}>
            <div style={{ display: "flex", alignItems: "center", gap: 5 }}>
              <div style={{ width: 14, height: 10, background: tierColor("Demand"), borderRadius: 2, opacity: 0.75 }} />
              <span style={{ fontSize: 10, color: MUTED }}>Demand</span>
            </div>
            <div style={{ display: "flex", alignItems: "center", gap: 5 }}>
              <div style={{ width: 14, height: 10, background: tierColor("Upstream"), borderRadius: 2, opacity: 0.75 }} />
              <span style={{ fontSize: 10, color: MUTED }}>Upstream</span>
            </div>
            <div style={{ display: "flex", alignItems: "center", gap: 5 }}>
              <div style={{ width: 14, height: 10, background: tierColor("Downstream"), borderRadius: 2, opacity: 0.75 }} />
              <span style={{ fontSize: 10, color: MUTED }}>Downstream</span>
            </div>
          </div>
        </div>

        {/* Panel 3: Per-Type Detail Cards */}
        <div style={{ display: "flex", flexWrap: "wrap", gap: 12, marginBottom: 16 }}>
          {ALL_ISSUE_TYPES.map(type => <TypeCard key={type} type={type} />)}
        </div>

        {/* Footer */}
        <div style={{ fontSize: 11, color: MUTED, lineHeight: 1.7,
          borderTop: `1px solid ${BORDER}`, paddingTop: 14 }}>
          <div style={{ marginBottom: 6 }}>
            <b style={{ color: TEXT }}>Reading this chart: </b>
            Yield = delivered ÷ ingested. The rate panel shows how efficiently each issue type
            moves from commitment to done. The loss breakdown shows where abandoned items drop
            off — Downstream abandonment is the most costly (work was nearly complete).
            The per-type cards show the full picture: volume, yield donut, and loss per tier
            with severity and average age at abandonment.
          </div>
          <div>
            <b style={{ color: TEXT }}>Important: </b>
            Abandoned Downstream items represent the highest form of waste — investment was made
            across the full delivery pipeline before the item was discarded. High avgAge at
            Downstream loss points signals items that lingered before being abandoned, compounding
            the waste. Abandoned Demand items are generally healthy backlog hygiene.
          </div>
        </div>

      </div>
    </div>
  );
}
