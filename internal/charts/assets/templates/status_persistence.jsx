import {
  BarChart, Bar, Cell, XAxis, YAxis,
  CartesianGrid, Tooltip, ResponsiveContainer,
} from "recharts";
import { ALARM, CAUTION, PRIMARY, SECONDARY, TEXT, MUTED, PAGE_BG, PANEL_BG, BORDER, typeColor, tierColor, FONT_STACK } from "mcs-mcp";
import { StatCard, Badge, TOOLTIP_BG } from "./shared.jsx";

// ── INJECTED DATA ─────────────────────────────────────────────────────────────
// Payload is injected by the MCS chart renderer as window.__MCS_PAYLOAD__.

const __MCS_ENVELOPE__ = window.__MCS_PAYLOAD__;
const __MCS_DATA__ = __MCS_ENVELOPE__.data;
const __MCS_GUARDRAILS__ = __MCS_ENVELOPE__.guardrails;
const __MCS_WORKFLOW__ = __MCS_ENVELOPE__.workflow;
// ── CONFIG ────────────────────────────────────────────────────────────────────

const ROLE_COLORS = { active: SECONDARY, queue: ALARM };

// ── DERIVED ───────────────────────────────────────────────────────────────────

const d = __MCS_DATA__;

const BOARD_ID    = __MCS_WORKFLOW__.board_id;
const PROJECT_KEY = __MCS_WORKFLOW__.project_key;
const BOARD_NAME  = __MCS_WORKFLOW__.board_name;
const STATUS_ORDER = __MCS_WORKFLOW__.status_order_names;

// Pool data — filter to STATUS_ORDER (exclude Finished tier like Done/Closed)
const statusSet = new Set(STATUS_ORDER);
const POOL_DATA = d.persistence.filter(dd => statusSet.has(dd.statusName));

const TIER_SUMMARY = d.tier_summary;

const ALL_ISSUE_TYPES = Object.keys(d.stratified_persistence);
const STRATIFIED = Object.fromEntries(
  ALL_ISSUE_TYPES.map(type => [
    type,
    d.stratified_persistence[type].filter(dd => statusSet.has(dd.statusName)),
  ])
);

const ISSUE_TYPE_COLORS = Object.fromEntries(
  ALL_ISSUE_TYPES.map(t => [t, typeColor(t, ALL_ISSUE_TYPES)])
);

// ── HELPERS ───────────────────────────────────────────────────────────────────

function abbrev(name) {
  return name.length > 28 ? name.slice(0, 26) + "…" : name;
}

function buildRows(dataArr) {
  const byName = Object.fromEntries(dataArr.map(dd => [dd.statusName, dd]));
  return STATUS_ORDER.map(name => byName[name]).filter(Boolean);
}

const round1 = v => Math.round((v || 0) * 10) / 10;

// Pool chart data
const poolRows = buildRows(POOL_DATA).map(dd => ({
  statusName: dd.statusName,
  label:      abbrev(dd.statusName),
  tier:       dd.tier,
  role:       dd.role,
  share:      dd.share,
  coin_toss:  round1(dd.coin_toss),
  probable:   round1(dd.probable),
  p85:        round1(dd.likely),
  safe_bet:   round1(dd.safe_bet),
  p95ext:     Math.max(0, round1(dd.safe_bet) - round1(dd.likely)),
  iqr:        round1(dd.iqr),
  inner_80:   round1(dd.inner_80),
  interpretation: dd.interpretation,
}));

// Per-type data
const typeChartData = STATUS_ORDER.map(name => {
  const row = { statusName: name, label: abbrev(name) };
  ALL_ISSUE_TYPES.forEach(type => {
    const entry = (STRATIFIED[type] || []).find(dd => dd.statusName === name);
    row[type] = entry ? round1(entry.likely) : 0;
  });
  return row;
});

// Top bottleneck
const topBottleneck = [...POOL_DATA]
  .filter(dd => dd.tier === "Downstream")
  .sort((a, b) => b.likely - a.likely)[0] || POOL_DATA[0] || { statusName: "—", likely: 0 };

const maxPoolP95 = Math.ceil(Math.max(...poolRows.map(dd => dd.p85 + dd.p95ext)) * 1.05);
const maxTypeP85 = Math.ceil(Math.max(
  ...typeChartData.flatMap(row => ALL_ISSUE_TYPES.map(t => row[t] || 0))
) * 1.05);

// ── SUB-COMPONENTS ────────────────────────────────────────────────────────────


function TierTick({ x, y, payload }) {
  const row = POOL_DATA.find(dd => dd.statusName === payload.value);
  const dotColor = row ? tierColor(row.tier) || MUTED : MUTED;
  const roleColor = row ? ROLE_COLORS[row.role] || MUTED : MUTED;
  return (
    <g transform={`translate(${x},${y})`}>
      <circle cx={-8} cy={0} r={3} fill={dotColor} opacity={0.8} />
      <text x={-16} y={0} dy={4} textAnchor="end"
        fill={roleColor} fontSize={10}
        fontFamily={FONT_STACK}>{abbrev(payload.value)}</text>
    </g>
  );
}

function PoolTooltip({ active, payload }) {
  if (!active || !payload?.length) return null;
  const dd = payload[0].payload;
  return (
    <div style={{ background: TOOLTIP_BG, border: `1px solid ${BORDER}`, borderRadius: 8,
      padding: "10px 14px", fontFamily: FONT_STACK, fontSize: 12, color: TEXT }}>
      <div style={{ fontWeight: 700, marginBottom: 6 }}>{dd.statusName}</div>
      <div style={{ display: "grid", gridTemplateColumns: "1fr auto", rowGap: 3, columnGap: 16 }}>
        <span style={{ color: tierColor(dd.tier) }}>Tier</span><span>{dd.tier}</span>
        <span style={{ color: ROLE_COLORS[dd.role] || MUTED }}>Role</span><span>{dd.role}</span>
        <span style={{ color: MUTED }}>Share</span><span>{Math.round(dd.share * 100)}%</span>
        <span style={{ color: SECONDARY }}>P50</span><span>{dd.coin_toss}d</span>
        <span style={{ color: PRIMARY }}>P70</span><span>{dd.probable}d</span>
        <span style={{ color: CAUTION }}>P85</span><span>{dd.p85}d</span>
        <span style={{ color: ALARM }}>P95</span><span>{dd.safe_bet}d</span>
        <span style={{ color: MUTED }}>IQR</span><span>{dd.iqr}d</span>
        <span style={{ color: MUTED }}>Inner 80</span><span>{dd.inner_80}d</span>
      </div>
    </div>
  );
}

function StratTooltipRow({ t, v }) {
  return (
    <div style={{ display: "flex", justifyContent: "space-between", gap: 16 }}>
      <span style={{ color: ISSUE_TYPE_COLORS[t] }}>{t}</span>
      <span>{v}d</span>
    </div>
  );
}

function StratTooltip({ active, payload, label }) {
  if (!active || !payload?.length) return null;
  const row = payload[0].payload;
  const rows = ALL_ISSUE_TYPES.map(t => ({ t, v: row[t] || 0 })).filter(x => x.v > 0);
  return (
    <div style={{ background: TOOLTIP_BG, border: `1px solid ${BORDER}`, borderRadius: 8,
      padding: "10px 14px", fontFamily: FONT_STACK, fontSize: 12, color: TEXT }}>
      <div style={{ fontWeight: 700, marginBottom: 6 }}>{row.statusName || label}</div>
      {rows.length === 0
        ? <div style={{ color: MUTED }}>No data for this status</div>
        : rows.map(({ t, v }) => <StratTooltipRow key={t} t={t} v={v} />)
      }
    </div>
  );
}

// ── MAIN EXPORT ───────────────────────────────────────────────────────────────

export default function StatusPersistenceChart() {
  return (
    <div style={{ background: PAGE_BG, minHeight: "100vh", padding: "24px 20px",
      fontFamily: FONT_STACK, color: TEXT }}>
      <div style={{ maxWidth: 1100, margin: "0 auto" }}>

        {/* Header */}
        <div style={{ fontSize: 11, color: MUTED, letterSpacing: "0.08em",
          textTransform: "uppercase", marginBottom: 6 }}>
          {PROJECT_KEY} · {BOARD_NAME} · Board {BOARD_ID}
        </div>
        <h1 style={{ fontSize: 22, fontWeight: 700, margin: "0 0 4px" }}>Status Persistence</h1>
        <div style={{ fontSize: 12, color: MUTED, marginBottom: 16 }}>
          Dwell time per status · delivered items only · internal dwell, not end-to-end
        </div>

        {/* Stat cards */}
        <div style={{ display: "flex", flexWrap: "wrap", gap: 10, marginBottom: 14 }}>
          <StatCard label="DOWNSTREAM P85"
            value={`${round1(TIER_SUMMARY.Downstream?.combined_p85)}d`} color={SECONDARY} />
          <StatCard label="UPSTREAM P85"
            value={`${round1(TIER_SUMMARY.Upstream?.combined_p85)}d`} color={PRIMARY} />
          <StatCard label="DEMAND P85"
            value={`${round1(TIER_SUMMARY.Demand?.combined_p85)}d`}
            sub="non-blocking" color={CAUTION} />
          <StatCard label="TOP BOTTLENECK"
            value={abbrev(topBottleneck.statusName)}
            sub={`P85 = ${round1(topBottleneck.likely)}d`} color={ALARM} />
        </div>

        {/* Guardrail badges */}
        <div style={{ display: "flex", flexWrap: "wrap", gap: 6, marginBottom: 20, alignItems: "center" }}>
          <Badge text="Delivered items only — active WIP excluded" color={MUTED} />
          <Badge text="Persistence = time within one status (not end-to-end)" color={MUTED} />
          <Badge text="Queue persistence = Flow Debt" color={ALARM} />
          <Badge text="Active persistence = local bottleneck signal" color={SECONDARY} />
        </div>

        {/* Panel 1: Tier Summary */}
        <div style={{ display: "flex", flexWrap: "wrap", gap: 12, marginBottom: 16 }}>
          {TIER_SUMMARY["Demand"] && (
            <div style={{ flex: "1 1 200px", background: PAGE_BG,
              border: `1px solid ${tierColor("Demand")}4d`, borderRadius: 10, padding: "14px 16px" }}>
              <div style={{ fontSize: 13, fontWeight: 700, color: tierColor("Demand"), marginBottom: 8 }}>Demand</div>
              <div style={{ fontSize: 11, color: MUTED, display: "grid",
                gridTemplateColumns: "1fr 1fr", rowGap: 4 }}>
                <span>P50 <b style={{ color: SECONDARY }}>{round1(TIER_SUMMARY["Demand"].combined_median)}d</b></span>
                <span>P85 <b style={{ color: CAUTION }}>{round1(TIER_SUMMARY["Demand"].combined_p85)}d</b></span>
                <span>Items <b style={{ color: TEXT }}>{TIER_SUMMARY["Demand"].count}</b></span>
              </div>
            </div>
          )}
          {TIER_SUMMARY["Upstream"] && (
            <div style={{ flex: "1 1 200px", background: PAGE_BG,
              border: `1px solid ${tierColor("Upstream")}4d`, borderRadius: 10, padding: "14px 16px" }}>
              <div style={{ fontSize: 13, fontWeight: 700, color: tierColor("Upstream"), marginBottom: 8 }}>Upstream</div>
              <div style={{ fontSize: 11, color: MUTED, display: "grid",
                gridTemplateColumns: "1fr 1fr", rowGap: 4 }}>
                <span>P50 <b style={{ color: SECONDARY }}>{round1(TIER_SUMMARY["Upstream"].combined_median)}d</b></span>
                <span>P85 <b style={{ color: CAUTION }}>{round1(TIER_SUMMARY["Upstream"].combined_p85)}d</b></span>
                <span>Items <b style={{ color: TEXT }}>{TIER_SUMMARY["Upstream"].count}</b></span>
              </div>
            </div>
          )}
          {TIER_SUMMARY["Downstream"] && (
            <div style={{ flex: "1 1 200px", background: PAGE_BG,
              border: `1px solid ${tierColor("Downstream")}4d`, borderRadius: 10, padding: "14px 16px" }}>
              <div style={{ fontSize: 13, fontWeight: 700, color: tierColor("Downstream"), marginBottom: 8 }}>Downstream</div>
              <div style={{ fontSize: 11, color: MUTED, display: "grid",
                gridTemplateColumns: "1fr 1fr", rowGap: 4 }}>
                <span>P50 <b style={{ color: SECONDARY }}>{round1(TIER_SUMMARY["Downstream"].combined_median)}d</b></span>
                <span>P85 <b style={{ color: CAUTION }}>{round1(TIER_SUMMARY["Downstream"].combined_p85)}d</b></span>
                <span>Items <b style={{ color: TEXT }}>{TIER_SUMMARY["Downstream"].count}</b></span>
              </div>
            </div>
          )}
        </div>

        {/* Panel 2: Pool Persistence */}
        <div style={{ background: PANEL_BG, borderRadius: 12,
          border: `1px solid ${BORDER}`, padding: "14px 8px 12px", marginBottom: 16 }}>
          <div style={{ fontSize: 11, color: MUTED, marginBottom: 8 }}>
            Pool — all types · P85 solid + P95 extension
          </div>
          <ResponsiveContainer width="100%" height={360}>
            <BarChart data={poolRows} layout="vertical"
              margin={{ top: 4, right: 80, left: 10, bottom: 4 }}>
              <CartesianGrid strokeDasharray="3 3" stroke={BORDER} horizontal={false} />
              <XAxis type="number" domain={[0, maxPoolP95]} tickFormatter={v => `${v}d`}
                tick={{ fill: MUTED, fontSize: 10, fontFamily: FONT_STACK }} />
              <YAxis type="category" dataKey="statusName" width={200}
                tick={<TierTick />} />
              <Tooltip content={PoolTooltip} cursor={{ fill: `${PRIMARY}0c` }} />
              <Bar dataKey="p85" stackId="a" barSize={18} radius={[0, 0, 0, 0]} isAnimationActive={false}>
                {poolRows.map((dd, i) => (
                  <Cell key={`p85-${i}`} fill={ROLE_COLORS[dd.role]} fillOpacity={0.75} />
                ))}
              </Bar>
              <Bar dataKey="p95ext" stackId="a" barSize={18} radius={[0, 4, 4, 0]} isAnimationActive={false}>
                {poolRows.map((dd, i) => (
                  <Cell key={`p95-${i}`} fill={ROLE_COLORS[dd.role]} fillOpacity={0.25} />
                ))}
              </Bar>
            </BarChart>
          </ResponsiveContainer>

          <div style={{ display: "flex", flexWrap: "wrap", gap: 12, justifyContent: "center", marginTop: 8 }}>
            <div style={{ display: "flex", alignItems: "center", gap: 5 }}>
              <div style={{ width: 14, height: 10, background: SECONDARY, borderRadius: 2, opacity: 0.75 }} />
              <span style={{ fontSize: 10, color: MUTED }}>Active (value-adding)</span>
            </div>
            <div style={{ display: "flex", alignItems: "center", gap: 5 }}>
              <div style={{ width: 14, height: 10, background: ALARM, borderRadius: 2, opacity: 0.75 }} />
              <span style={{ fontSize: 10, color: MUTED }}>Queue (waiting / flow debt)</span>
            </div>
            <div style={{ display: "flex", alignItems: "center", gap: 5 }}>
              <div style={{ width: 14, height: 10, background: MUTED, borderRadius: 2, opacity: 0.25 }} />
              <span style={{ fontSize: 10, color: MUTED }}>P95 extension</span>
            </div>
            <div style={{ width: 1, height: 14, background: BORDER }} />
            <div style={{ display: "flex", alignItems: "center", gap: 5 }}>
              <div style={{ width: 8, height: 8, borderRadius: "50%", background: tierColor("Demand"), opacity: 0.8 }} />
              <span style={{ fontSize: 10, color: MUTED }}>Demand</span>
            </div>
            <div style={{ display: "flex", alignItems: "center", gap: 5 }}>
              <div style={{ width: 8, height: 8, borderRadius: "50%", background: tierColor("Upstream"), opacity: 0.8 }} />
              <span style={{ fontSize: 10, color: MUTED }}>Upstream</span>
            </div>
            <div style={{ display: "flex", alignItems: "center", gap: 5 }}>
              <div style={{ width: 8, height: 8, borderRadius: "50%", background: tierColor("Downstream"), opacity: 0.8 }} />
              <span style={{ fontSize: 10, color: MUTED }}>Downstream</span>
            </div>
            <span style={{ fontSize: 10, color: MUTED }}>(dot = tier)</span>
          </div>
        </div>

        {/* Panel 3: Per-Type P85 */}
        {ALL_ISSUE_TYPES.length > 0 && (
          <div style={{ background: PANEL_BG, borderRadius: 12,
            border: `1px solid ${BORDER}`, padding: "14px 8px 12px", marginBottom: 16 }}>
            <div style={{ fontSize: 11, color: MUTED, marginBottom: 8 }}>
              P85 persistence by status and issue type
            </div>
            <ResponsiveContainer width="100%" height={360}>
              <BarChart data={typeChartData} layout="vertical"
                margin={{ top: 4, right: 80, left: 10, bottom: 4 }}>
                <CartesianGrid strokeDasharray="3 3" stroke={BORDER} horizontal={false} />
                <XAxis type="number" domain={[0, maxTypeP85]} tickFormatter={v => `${v}d`}
                  tick={{ fill: MUTED, fontSize: 10, fontFamily: FONT_STACK }} />
                <YAxis type="category" dataKey="label" width={200}
                  tick={{ fill: TEXT, fontSize: 10, fontFamily: FONT_STACK }} />
                <Tooltip content={StratTooltip} cursor={{ fill: `${PRIMARY}0c` }} />
                {ALL_ISSUE_TYPES.map(t => (
                  <Bar key={t} dataKey={t} barSize={7} radius={[0, 3, 3, 0]}
                    fill={ISSUE_TYPE_COLORS[t]} fillOpacity={0.75} isAnimationActive={false} />
                ))}
              </BarChart>
            </ResponsiveContainer>

            <div style={{ display: "flex", flexWrap: "wrap", gap: 12, justifyContent: "center", marginTop: 8 }}>
              {ALL_ISSUE_TYPES.map(t => (
                <span key={t} style={{ fontSize: 10, color: MUTED }}>
                  <span style={{ color: ISSUE_TYPE_COLORS[t] }}>●</span>{" "}{t}
                </span>
              ))}
            </div>
          </div>
        )}

        {/* Footer */}
        <div style={{ fontSize: 11, color: MUTED, lineHeight: 1.7,
          borderTop: `1px solid ${BORDER}`, paddingTop: 14 }}>
          <div style={{ marginBottom: 6 }}>
            <b style={{ color: TEXT }}>Reading this chart: </b>
            Each bar shows how long items typically dwell in that status. Bar length = P85
            (85% of visits resolved within that time); the lighter extension = P95. Cyan bars
            are active (value-adding) stages; red bars are queues (waiting / flow debt).
            The tier dot on the Y-axis indicates workflow phase. The per-type panel compares
            P85 persistence across issue types for the same statuses.
          </div>
          <div>
            <b style={{ color: TEXT }}>Important: </b>
            These numbers measure INTERNAL persistence within one status visit — not
            end-to-end cycle time. An item may visit the same status multiple times. Queue
            persistence ("awaiting X") represents pure flow debt — no value is added while
            items wait. This analysis uses only successfully delivered items; active WIP is
            excluded to prevent skewing historical norms.
          </div>
        </div>

      </div>
    </div>
  );
}
