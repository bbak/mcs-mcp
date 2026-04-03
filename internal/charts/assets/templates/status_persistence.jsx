import {
  BarChart, Bar, Cell, XAxis, YAxis,
  CartesianGrid, Tooltip, ResponsiveContainer,
} from "recharts";

// ── INJECTED DATA ─────────────────────────────────────────────────────────────
// Payload is injected by the MCS chart renderer as window.__MCS_PAYLOAD__.

const __MCS_ENVELOPE__ = window.__MCS_PAYLOAD__;
const __MCS_DATA__ = __MCS_ENVELOPE__.data;
const __MCS_GUARDRAILS__ = __MCS_ENVELOPE__.guardrails;
const __MCS_WORKFLOW__ = __MCS_ENVELOPE__.workflow;
// ── CONFIG ────────────────────────────────────────────────────────────────────

const ALARM     = "#ff6b6b";
const CAUTION   = "#e2c97e";
const PRIMARY   = "#6b7de8";
const SECONDARY = "#7edde2";
const POSITIVE  = "#6bffb8";
const TEXT      = "#dde1ef";
const MUTED     = "#505878";
const PAGE_BG   = "#080a0f";
const PANEL_BG  = "#0c0e16";
const BORDER    = "#1a1d2e";

const TIER_COLORS = { Demand: CAUTION, Upstream: PRIMARY, Downstream: SECONDARY };
const ROLE_COLORS = { active: SECONDARY, queue: ALARM };

const ISSUE_TYPE_PALETTE = [PRIMARY, ALARM, SECONDARY, CAUTION, POSITIVE, "#f97316"];

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
  ALL_ISSUE_TYPES.map((t, i) => [t, ISSUE_TYPE_PALETTE[i % ISSUE_TYPE_PALETTE.length]])
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

const maxPoolP95 = Math.max(...poolRows.map(dd => dd.p85 + dd.p95ext));
const maxTypeP85 = Math.max(
  ...typeChartData.flatMap(row => ALL_ISSUE_TYPES.map(t => row[t] || 0))
);

// ── SUB-COMPONENTS ────────────────────────────────────────────────────────────

const StatCard = ({ label, value, sub, color }) => (
  <div style={{ background: PANEL_BG, border: `1px solid ${color}33`,
    borderRadius: 8, padding: "8px 14px", minWidth: 110 }}>
    <div style={{ fontSize: 10, color: MUTED, marginBottom: 3, letterSpacing: "0.05em" }}>{label}</div>
    <div style={{ fontSize: 18, fontWeight: 700, color }}>{value}</div>
    {sub && <div style={{ fontSize: 9, color: MUTED, marginTop: 2 }}>{sub}</div>}
  </div>
);

const Badge = ({ text, color }) => (
  <span style={{ fontSize: 11, padding: "3px 8px", borderRadius: 4,
    background: `${color}15`, border: `1px solid ${color}40`, color,
    fontFamily: "'Courier New', monospace" }}>{text}</span>
);

function TierTick({ x, y, payload }) {
  const row = POOL_DATA.find(dd => dd.statusName === payload.value);
  const tierColor = row ? TIER_COLORS[row.tier] || MUTED : MUTED;
  const roleColor = row ? ROLE_COLORS[row.role] || MUTED : MUTED;
  return (
    <g transform={`translate(${x},${y})`}>
      <circle cx={-8} cy={0} r={3} fill={tierColor} opacity={0.8} />
      <text x={-16} y={0} dy={4} textAnchor="end"
        fill={roleColor} fontSize={10}
        fontFamily="'Courier New', monospace">{abbrev(payload.value)}</text>
    </g>
  );
}

const PoolTooltip = ({ active, payload }) => {
  if (!active || !payload?.length) return null;
  const dd = payload[0].payload;
  return (
    <div style={{ background: "#0f1117", border: `1px solid ${BORDER}`, borderRadius: 8,
      padding: "10px 14px", fontFamily: "'Courier New', monospace", fontSize: 12, color: TEXT }}>
      <div style={{ fontWeight: 700, marginBottom: 6 }}>{dd.statusName}</div>
      <div style={{ display: "grid", gridTemplateColumns: "1fr auto", rowGap: 3, columnGap: 16 }}>
        <span style={{ color: TIER_COLORS[dd.tier] || MUTED }}>Tier</span><span>{dd.tier}</span>
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
};

const StratTooltip = ({ active, payload, label }) => {
  if (!active || !payload?.length) return null;
  const row = typeChartData.find(dd => dd.label === label);
  return (
    <div style={{ background: "#0f1117", border: `1px solid ${BORDER}`, borderRadius: 8,
      padding: "10px 14px", fontFamily: "'Courier New', monospace", fontSize: 12, color: TEXT }}>
      <div style={{ fontWeight: 700, marginBottom: 6 }}>{row?.statusName || label}</div>
      {ALL_ISSUE_TYPES.map(t => {
        const v = row?.[t];
        return v != null && v > 0 ? (
          <div key={t} style={{ display: "flex", justifyContent: "space-between", gap: 16 }}>
            <span style={{ color: ISSUE_TYPE_COLORS[t] }}>{t}</span>
            <span>{v}d</span>
          </div>
        ) : null;
      })}
    </div>
  );
};

// ── MAIN EXPORT ───────────────────────────────────────────────────────────────

export default function StatusPersistenceChart() {
  return (
    <div style={{ background: PAGE_BG, minHeight: "100vh", padding: "24px 20px",
      fontFamily: "'Courier New', monospace", color: TEXT }}>
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
          {["Demand", "Upstream", "Downstream"].map(tier => {
            const ts = TIER_SUMMARY[tier];
            if (!ts) return null;
            const tc = TIER_COLORS[tier];
            return (
              <div key={tier} style={{ flex: "1 1 200px", background: PAGE_BG,
                border: `1px solid ${tc}4d`, borderRadius: 10, padding: "14px 16px" }}>
                <div style={{ fontSize: 13, fontWeight: 700, color: tc, marginBottom: 8 }}>{tier}</div>
                <div style={{ fontSize: 11, color: MUTED, display: "grid",
                  gridTemplateColumns: "1fr 1fr", rowGap: 4 }}>
                  <span>P50 <b style={{ color: SECONDARY }}>{round1(ts.combined_median)}d</b></span>
                  <span>P85 <b style={{ color: CAUTION }}>{round1(ts.combined_p85)}d</b></span>
                  <span>Items <b style={{ color: TEXT }}>{ts.count}</b></span>
                </div>
              </div>
            );
          })}
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
              <XAxis type="number" domain={[0, maxPoolP95 * 1.05]} tickFormatter={v => `${v}d`}
                tick={{ fill: MUTED, fontSize: 10, fontFamily: "'Courier New', monospace" }} />
              <YAxis type="category" dataKey="statusName" width={200}
                tick={<TierTick />} />
              <Tooltip content={<PoolTooltip />} cursor={{ fill: `${PRIMARY}0c` }} />
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
            {[
              { color: SECONDARY, opacity: 0.75, label: "Active (value-adding)" },
              { color: ALARM,     opacity: 0.75, label: "Queue (waiting / flow debt)" },
              { color: MUTED,     opacity: 0.25, label: "P95 extension" },
            ].map(({ color, opacity, label }) => (
              <div key={label} style={{ display: "flex", alignItems: "center", gap: 5 }}>
                <div style={{ width: 14, height: 10, background: color, borderRadius: 2, opacity }} />
                <span style={{ fontSize: 10, color: MUTED }}>{label}</span>
              </div>
            ))}
            <div style={{ width: 1, height: 14, background: BORDER }} />
            {["Demand", "Upstream", "Downstream"].map(tier => (
              <div key={tier} style={{ display: "flex", alignItems: "center", gap: 5 }}>
                <svg width={10} height={10}><circle cx={5} cy={5} r={3} fill={TIER_COLORS[tier]} opacity={0.8} /></svg>
                <span style={{ fontSize: 10, color: MUTED }}>{tier}</span>
              </div>
            ))}
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
                <XAxis type="number" domain={[0, maxTypeP85 * 1.05]} tickFormatter={v => `${v}d`}
                  tick={{ fill: MUTED, fontSize: 10, fontFamily: "'Courier New', monospace" }} />
                <YAxis type="category" dataKey="label" width={200}
                  tick={{ fill: TEXT, fontSize: 10, fontFamily: "'Courier New', monospace" }} />
                <Tooltip content={<StratTooltip />} cursor={{ fill: `${PRIMARY}0c` }} />
                {ALL_ISSUE_TYPES.map(t => (
                  <Bar key={t} dataKey={t} barSize={7} radius={[0, 3, 3, 0]}
                    fill={ISSUE_TYPE_COLORS[t]} fillOpacity={0.75} isAnimationActive={false} />
                ))}
              </BarChart>
            </ResponsiveContainer>

            <div style={{ display: "flex", flexWrap: "wrap", gap: 12, justifyContent: "center", marginTop: 8 }}>
              {ALL_ISSUE_TYPES.map(t => (
                <div key={t} style={{ display: "flex", alignItems: "center", gap: 5 }}>
                  <div style={{ width: 10, height: 10, borderRadius: "50%",
                    background: ISSUE_TYPE_COLORS[t] }} />
                  <span style={{ fontSize: 10, color: MUTED }}>{t}</span>
                </div>
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
