import {
  BarChart, Bar, Cell, ComposedChart, Scatter,
  ReferenceLine, XAxis, YAxis,
  CartesianGrid, Tooltip, ResponsiveContainer,
} from "recharts";

// ── INJECTED DATA ─────────────────────────────────────────────────────────────
// These two constants are replaced by Claude via str_replace before delivery.
// Do NOT edit manually.

const MCP_RESPONSE = "__MCP_RESPONSE__";
const CHART_ATTRS  = "__CHART_ATTRS__";

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

const PERC_COLORS = {
  aggressive:     POSITIVE,
  unlikely:       "#4ade80",
  coin_toss:      SECONDARY,
  probable:       PRIMARY,
  likely:         CAUTION,
  conservative:   "#f97316",
  safe:           ALARM,
  almost_certain: "#c026d3",
};

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

const ISSUE_TYPE_PALETTE = [
  "#6b7de8","#ff6b6b","#7edde2","#e2c97e",
  "#6bffb8","#f97316","#8b5cf6","#ec4899",
];

// ── DERIVED ───────────────────────────────────────────────────────────────────

const d   = MCP_RESPONSE.data;
const ctx = d.context;

const BOARD_ID    = CHART_ATTRS.board_id;
const PROJECT_KEY = CHART_ATTRS.project_key;
const BOARD_NAME  = CHART_ATTRS.board_name;

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
  ALL_ISSUE_TYPES.map((t, i) => [t, ISSUE_TYPE_PALETTE[i % ISSUE_TYPE_PALETTE.length]])
);

const eligibleTypes   = ALL_ISSUE_TYPES.filter(t => TYPE_SLES[t].eligible);
const ineligibleTypes = ALL_ISSUE_TYPES.filter(t => !TYPE_SLES[t].eligible);

const round1 = v => Math.round((v || 0) * 10) / 10;

// Pool bar data
const poolData = PERC_KEYS.map(k => ({
  key:   k,
  label: PERC_SHORT[k],
  days:  round1(POOL[k]),
  color: PERC_COLORS[k],
  isSLE: k === "likely",
}));

// Per-type bar data
const typeData = PERC_KEYS.map(k => {
  const row = { key: k, label: PERC_SHORT[k], isSLE: k === "likely" };
  eligibleTypes.forEach(t => { row[t] = round1(TYPE_SLES[t][k]); });
  return row;
});

const P85 = round1(POOL.likely);
const P98 = round1(POOL.almost_certain);
const maxTypeP98 = eligibleTypes.length > 0
  ? Math.max(...eligibleTypes.map(t => round1(TYPE_SLES[t].almost_certain || 0)))
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

const ScatterDot = ({ cx, cy, payload }) => {
  if (!cx || !cy) return null;
  const typeIdx = ALL_ISSUE_TYPES.indexOf(payload.issueType);
  const color = ISSUE_TYPE_PALETTE[typeIdx >= 0 ? typeIdx % ISSUE_TYPE_PALETTE.length : 0];
  if (payload.aboveSLE)
    return <circle cx={cx} cy={cy} r={4} fill={color} stroke={CAUTION} strokeWidth={1.5}/>;
  return <circle cx={cx} cy={cy} r={2.5} fill={color} fillOpacity={0.5} stroke="none"/>;
};

const ScatterTooltip = ({ active, payload }) => {
  if (!active || !payload?.length) return null;
  const pt = payload[0].payload;
  const typeIdx = ALL_ISSUE_TYPES.indexOf(pt.issueType);
  const color = ISSUE_TYPE_PALETTE[typeIdx >= 0 ? typeIdx % ISSUE_TYPE_PALETTE.length : 0];
  return (
    <div style={{ background: "#0f1117", border: `1px solid ${BORDER}`, borderRadius: 8,
      padding: "10px 14px", fontFamily: "'Courier New', monospace", fontSize: 12, color: TEXT }}>
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
};

// ── HELPERS ───────────────────────────────────────────────────────────────────

const fatTailColor = FAT_TAIL_RATIO >= 5.6 ? ALARM : FAT_TAIL_RATIO >= 3 ? CAUTION : POSITIVE;
const tailMedianColor = TAIL_TO_MEDIAN >= 3 ? ALARM : TAIL_TO_MEDIAN >= 2 ? CAUTION : POSITIVE;
const predColor = (PREDICTABILITY || "").toLowerCase().includes("unstable") ||
                  (PREDICTABILITY || "").toLowerCase().includes("volatile") ? ALARM : CAUTION;

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

const PoolTooltip = ({ active, payload }) => {
  if (!active || !payload?.length) return null;
  const d = payload[0].payload;
  return (
    <div style={{ background: "#0f1117", border: `1px solid ${BORDER}`, borderRadius: 8,
      padding: "10px 14px", fontFamily: "'Courier New', monospace", fontSize: 12, color: TEXT }}>
      <div style={{ fontWeight: 700, color: d.color, marginBottom: 4 }}>{d.label}</div>
      <div>{d.days}d</div>
      {d.isSLE && <div style={{ color: CAUTION, marginTop: 4 }}>← canonical SLE</div>}
    </div>
  );
};

const TypeTooltip = ({ active, payload, label }) => {
  if (!active || !payload?.length) return null;
  return (
    <div style={{ background: "#0f1117", border: `1px solid ${BORDER}`, borderRadius: 8,
      padding: "10px 14px", fontFamily: "'Courier New', monospace", fontSize: 12, color: TEXT }}>
      <div style={{ fontWeight: 700, marginBottom: 6 }}>{label}</div>
      {eligibleTypes.map(t => {
        const v = payload.find(p => p.dataKey === t)?.value;
        return v != null ? (
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

export default function CycleTimeSleChart() {
  const scale = v => `${Math.min((v / P98) * 100, 100).toFixed(1)}%`;

  return (
    <div style={{ background: PAGE_BG, minHeight: "100vh", padding: "24px 20px",
      fontFamily: "'Courier New', monospace", color: TEXT }}>
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
          <StatCard label="P50 MEDIAN"   value={`${round1(POOL.coin_toss)}d`}  color={SECONDARY} />
          <StatCard label="P85 SLE"      value={`${P85}d`}                      color={CAUTION} />
          <StatCard label="P95 SAFE BET" value={`${round1(POOL.safe)}d`}        color={ALARM} />
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
              left: scale(round1(POOL.coin_toss)), background: SECONDARY }} />
            {/* P85 tick */}
            <div style={{ position: "absolute", top: 0, bottom: 0, width: 2,
              left: scale(P85), background: CAUTION }} />
            {/* P98 tick */}
            <div style={{ position: "absolute", top: 0, bottom: 0, width: 2,
              right: 0, background: ALARM }} />
          </div>

          <div style={{ display: "flex", gap: 16, justifyContent: "center", fontSize: 10, color: MUTED }}>
            <span><span style={{ color: POSITIVE }}>●</span> P10</span>
            <span><span style={{ color: SECONDARY }}>●</span> P50</span>
            <span><span style={{ color: CAUTION }}>●</span> P85</span>
            <span><span style={{ color: ALARM }}>●</span> P98</span>
          </div>
        </div>

        {/* Panel 2: Pool SLE */}
        <div style={{ background: PANEL_BG, borderRadius: 12,
          border: `1px solid ${BORDER}`, padding: "14px 8px 12px", marginBottom: 16 }}>
          <div style={{ fontSize: 11, color: MUTED, marginBottom: 8 }}>
            Pool SLE — All issue types combined · {ISSUES_ANALYZED} items · {DAYS_IN_SAMPLE} days
          </div>
          <ResponsiveContainer width="100%" height={280}>
            <BarChart data={poolData} layout="vertical"
              margin={{ top: 4, right: 80, left: 140, bottom: 4 }}>
              <CartesianGrid strokeDasharray="3 3" stroke={BORDER} horizontal={false} />
              <XAxis type="number" domain={[0, P98 * 1.05]} tickFormatter={v => `${v}d`}
                tick={{ fill: MUTED, fontSize: 10, fontFamily: "'Courier New', monospace" }} />
              <YAxis type="category" dataKey="label" width={130}
                tick={{ fill: TEXT, fontSize: 10, fontFamily: "'Courier New', monospace" }} />
              <Tooltip content={<PoolTooltip />} cursor={{ fill: `${PRIMARY}0c` }} />
              <Bar dataKey="days" radius={[0, 4, 4, 0]} barSize={22} isAnimationActive={false}>
                {poolData.map((d, i) => (
                  <Cell key={`cell-${i}`} fill={d.color} fillOpacity={d.isSLE ? 1.0 : 0.55} />
                ))}
              </Bar>
              <ReferenceLine x={P85} stroke={CAUTION} strokeDasharray="4 3" strokeWidth={1.5}
                label={{ value: `SLE ${P85}d`, fill: CAUTION, fontSize: 10, position: "right" }} />
            </BarChart>
          </ResponsiveContainer>
          <div style={{ display: "flex", gap: 16, justifyContent: "center", fontSize: 10, color: MUTED, marginTop: 6 }}>
            <span>IQR (P25–P75): {IQR}d</span>
            <span>Inner 80 (P10–P90): {INNER_80}d</span>
          </div>
        </div>

        {/* Panel 3: Per-Type SLE */}
        {eligibleTypes.length > 0 && (
          <div style={{ background: PANEL_BG, borderRadius: 12,
            border: `1px solid ${BORDER}`, padding: "14px 8px 12px", marginBottom: 16 }}>
            <div style={{ fontSize: 11, color: MUTED, marginBottom: 8 }}>
              Per-type SLE comparison · eligible streams · P85 highlighted
            </div>
            <ResponsiveContainer width="100%" height={280}>
              <BarChart data={typeData} layout="vertical"
                margin={{ top: 4, right: 80, left: 140, bottom: 4 }}>
                <CartesianGrid strokeDasharray="3 3" stroke={BORDER} horizontal={false} />
                <XAxis type="number" domain={[0, maxTypeP98 * 1.05]} tickFormatter={v => `${v}d`}
                  tick={{ fill: MUTED, fontSize: 10, fontFamily: "'Courier New', monospace" }} />
                <YAxis type="category" dataKey="label" width={130}
                  tick={({ x, y, payload }) => (
                    <text x={x} y={y} dy={4} textAnchor="end"
                      fill={payload.value === PERC_SHORT.likely ? CAUTION : TEXT}
                      fontWeight={payload.value === PERC_SHORT.likely ? 700 : 400}
                      fontSize={10} fontFamily="'Courier New', monospace">
                      {payload.value}
                    </text>
                  )} />
                <Tooltip content={<TypeTooltip />} cursor={{ fill: `${PRIMARY}0c` }} />
                {eligibleTypes.map(t => (
                  <Bar key={t} dataKey={t} barSize={7} radius={[0, 3, 3, 0]}
                    fill={ISSUE_TYPE_COLORS[t]} fillOpacity={0.75} isAnimationActive={false} />
                ))}
              </BarChart>
            </ResponsiveContainer>

            {ineligibleTypes.length > 0 && (
              <div style={{ fontSize: 10, color: MUTED, marginTop: 8, padding: "0 12px" }}>
                Ineligible (volume too low — collapsed to pool):{" "}
                {ineligibleTypes.map((t, i) => (
                  <span key={t}>
                    {i > 0 && ", "}
                    <span style={{ color: ISSUE_TYPE_COLORS[t] }}>{t}</span>
                    {" "}({TYPE_SLES[t].volume})
                  </span>
                ))}
              </div>
            )}

            <div style={{ display: "flex", flexWrap: "wrap", gap: 12, justifyContent: "center",
              marginTop: 8 }}>
              {eligibleTypes.map(t => (
                <div key={t} style={{ display: "flex", alignItems: "center", gap: 5 }}>
                  <div style={{ width: 10, height: 10, borderRadius: "50%",
                    background: ISSUE_TYPE_COLORS[t] }} />
                  <span style={{ fontSize: 10, color: MUTED }}>{t} (n={TYPE_SLES[t].volume})</span>
                </div>
              ))}
            </div>
          </div>
        )}

        {/* Panel 4: Cycle Time Scatterplot */}
        {scatterData.length > 0 && (
          <div style={{ background: PANEL_BG, borderRadius: 12,
            border: `1px solid ${BORDER}`, padding: "14px 12px 12px", marginBottom: 16 }}>
            <div style={{ fontSize: 11, color: MUTED, marginBottom: 8, letterSpacing: "0.05em",
              textTransform: "uppercase" }}>
              Cycle Time Scatterplot — Individual items by completion date · {scatterData.length} items
            </div>

            <ResponsiveContainer width="100%" height={380}>
              <ComposedChart data={scatterData} margin={{ top: 10, right: 20, bottom: 60, left: 10 }}>
                <CartesianGrid strokeDasharray="3 3" stroke={BORDER} vertical={false} />
                <XAxis dataKey="date" tickFormatter={formatDate} interval={scatterInterval}
                  angle={-45} textAnchor="end" height={60}
                  tick={{ fill: MUTED, fontSize: 11, fontFamily: "'Courier New', monospace" }} />
                <YAxis domain={[0, scatterYMax]} tickFormatter={v => `${v}d`}
                  tick={{ fill: MUTED, fontSize: 11, fontFamily: "'Courier New', monospace" }}
                  label={{ value: "Cycle Time (days)", angle: -90, position: "insideLeft",
                    fill: MUTED, fontSize: 11, fontFamily: "'Courier New', monospace" }} />
                <Tooltip content={<ScatterTooltip />} />
                <ReferenceLine y={P50} stroke={SECONDARY} strokeDasharray="4 4" strokeWidth={1.5}
                  label={{ value: `P50 ${P50}d`, fill: SECONDARY, fontSize: 10,
                    fontFamily: "'Courier New', monospace", position: "insideTopRight" }} />
                <ReferenceLine y={P70} stroke={PRIMARY} strokeDasharray="4 4" strokeWidth={1.5}
                  label={{ value: `P70 ${P70}d`, fill: PRIMARY, fontSize: 10,
                    fontFamily: "'Courier New', monospace", position: "insideTopRight" }} />
                <ReferenceLine y={P85} stroke={CAUTION} strokeDasharray="6 3" strokeWidth={1.5}
                  label={{ value: `P85 SLE ${P85}d`, fill: CAUTION, fontSize: 10,
                    fontFamily: "'Courier New', monospace", position: "insideTopRight" }} />
                <ReferenceLine y={P95} stroke={ALARM} strokeDasharray="4 4" strokeWidth={1.5}
                  label={{ value: `P95 ${P95}d`, fill: ALARM, fontSize: 10,
                    fontFamily: "'Courier New', monospace", position: "insideTopRight" }} />
                <Scatter dataKey="value" shape={<ScatterDot />} isAnimationActive={false} />
              </ComposedChart>
            </ResponsiveContainer>

            <div style={{ display: "flex", flexWrap: "wrap", gap: 16, justifyContent: "center", marginTop: 8 }}>
              {ALL_ISSUE_TYPES.map((t, i) => (
                <div key={t} style={{ display: "flex", alignItems: "center", gap: 5 }}>
                  <div style={{ width: 10, height: 10, borderRadius: "50%",
                    background: ISSUE_TYPE_PALETTE[i % ISSUE_TYPE_PALETTE.length] }} />
                  <span style={{ fontSize: 10, color: MUTED }}>{t}</span>
                </div>
              ))}
              {[
                { svg: <line x1={0} y1={6} x2={24} y2={6} stroke={SECONDARY} strokeDasharray="4 4" strokeWidth={1.5}/>, label: "P50 Median" },
                { svg: <line x1={0} y1={6} x2={24} y2={6} stroke={PRIMARY} strokeDasharray="4 4" strokeWidth={1.5}/>, label: "P70 Probable" },
                { svg: <line x1={0} y1={6} x2={24} y2={6} stroke={CAUTION} strokeDasharray="6 3" strokeWidth={1.5}/>, label: "P85 SLE" },
                { svg: <line x1={0} y1={6} x2={24} y2={6} stroke={ALARM} strokeDasharray="4 4" strokeWidth={1.5}/>, label: "P95 Safe Bet" },
              ].map(({ svg, label }) => (
                <div key={label} style={{ display: "flex", alignItems: "center", gap: 6 }}>
                  <svg width={24} height={12}>{svg}</svg>
                  <span style={{ fontSize: 10, color: MUTED }}>{label}</span>
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
