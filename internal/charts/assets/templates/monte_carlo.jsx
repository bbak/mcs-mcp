import {
  BarChart, Bar, Cell, ReferenceLine, XAxis, YAxis,
  CartesianGrid, Tooltip, ResponsiveContainer,
} from "recharts";
import { ALARM, CAUTION, PRIMARY, SECONDARY, POSITIVE, TEXT, MUTED, PAGE_BG, PANEL_BG, BORDER, percColor, FONT_STACK } from "mcs-mcp";
import { StatCard, Badge, TOOLTIP_BG } from "./shared.jsx";

// ── INJECTED DATA ─────────────────────────────────────────────────────────────
// Payload is injected by the MCS chart renderer as window.__MCS_PAYLOAD__.

const __MCS_ENVELOPE__ = window.__MCS_PAYLOAD__;
const __MCS_DATA__ = __MCS_ENVELOPE__.data;
const __MCS_GUARDRAILS__ = __MCS_ENVELOPE__.guardrails;
const __MCS_WORKFLOW__ = __MCS_ENVELOPE__.workflow;
// ── CONFIG ────────────────────────────────────────────────────────────────────


const PERC_ORDER = [
  "aggressive","unlikely","coin_toss","probable",
  "likely","conservative","safe","almost_certain",
];
const PERC_SHORT = {
  aggressive:"P10", unlikely:"P30", coin_toss:"P50", probable:"P70",
  likely:"P85", conservative:"P90", safe:"P95", almost_certain:"P98",
};

function predictabilityColor(p) {
  const l = (p || "").toLowerCase();
  if (l.includes("stable") || l.includes("high")) return POSITIVE;
  if (l.includes("moderate") || l.includes("medium")) return CAUTION;
  return ALARM;
}
function fatTailColor(r) {
  if (r <= 1.0) return POSITIVE;
  if (r <= 1.5) return CAUTION;
  return ALARM;
}
function trendColor(direction) {
  if (direction === "Increasing") return POSITIVE;
  if (direction === "Decreasing") return ALARM;
  return MUTED;
}

// ── DERIVED ───────────────────────────────────────────────────────────────────

const BOARD_ID    = __MCS_WORKFLOW__.board_id;
const PROJECT_KEY = __MCS_WORKFLOW__.project_key;
const BOARD_NAME  = __MCS_WORKFLOW__.board_name;

// The server renders one mode per tool call. The simulation result is
// directly in __MCS_DATA__, with the mode in context.simulation_mode.
const SIM = __MCS_DATA__;
const MODE = SIM.context?.simulation_mode || (SIM.composition?.total > 0 ? "duration" : "scope");
const isDuration = MODE === "duration";
const TARGET_DAYS = SIM.context?.target_days || null;

// ── TABLE ROW COMPONENTS ──────────────────────────────────────────────────────

function StratRow({ s, i }) {
  return (
    <tr style={{ background: i % 2 === 0 ? "transparent" : `${PRIMARY}05` }}>
      <td style={{ padding: "8px 8px", color: s.eligible ? TEXT : MUTED, whiteSpace: "nowrap" }}>{s.type}</td>
      <td style={{ padding: "8px 8px", color: s.eligible ? POSITIVE : ALARM, whiteSpace: "nowrap" }}>{s.eligible ? "Yes" : "No"}</td>
      <td style={{ padding: "8px 8px", whiteSpace: "nowrap" }}>{s.volume}</td>
      <td style={{ padding: "8px 8px", whiteSpace: "nowrap" }}>{s.p85_cycle_time?.toFixed(1) || "—"}d</td>
      <td style={{ padding: "8px 8px", color: MUTED, fontSize: 10 }}>{s.reason || (s.eligible ? "independently modeled" : "")}</td>
    </tr>
  );
}

// ── SUB-COMPONENTS ────────────────────────────────────────────────────────────

const PercTooltip = ({ active, payload, isDuration, labels }) => {
  if (!active || !payload?.length) return null;
  const d = payload[0].payload;
  return (
    <div style={{ background: TOOLTIP_BG, border: `1px solid ${BORDER}`, borderRadius: 8,
      padding: "10px 14px", fontFamily: FONT_STACK, fontSize: 12, color: TEXT }}>
      <div style={{ fontWeight: 700, color: percColor(d.key), marginBottom: 2 }}>{d.label}</div>
      <div style={{ fontSize: 10, color: MUTED, marginBottom: 6 }}>{labels?.[d.key] || ""}</div>
      <div style={{ color: percColor(d.key) }}>{d.value}{d.unit}</div>
    </div>
  );
};

// ── MAIN EXPORT ───────────────────────────────────────────────────────────────

export default function MonteCarloChart() {
  if (!SIM || !SIM.percentiles) return <div style={{ color: ALARM, padding: 40 }}>No forecast data available.</div>;

  const ctx = SIM.context;
  const tc  = trendColor(SIM.throughput_trend.direction);

  // Percentile chart data
  const percData = PERC_ORDER.map(key => ({
    key,
    label: PERC_SHORT[key],
    value: SIM.percentiles[key],
    unit:  isDuration ? "d" : " items",
  }));
  const maxPerc = Math.ceil(Math.max(...percData.map(d => d.value)) * 1.1);

  // Stratification table
  const strat = ctx.stratification_decisions || [];

  const modeLabel = isDuration ? "Duration mode" : "Scope mode";

  function PercTooltipWithCtx(props) {
    return <PercTooltip {...props} isDuration={isDuration} labels={SIM.labels} />;
  }

  return (
    <div style={{ background: PAGE_BG, minHeight: "100vh", padding: "24px 20px",
      fontFamily: FONT_STACK, color: TEXT }}>
      <div style={{ maxWidth: 1100, margin: "0 auto" }}>

        {/* Header */}
        <div style={{ fontSize: 11, color: MUTED, letterSpacing: "0.08em",
          textTransform: "uppercase", marginBottom: 6 }}>
          {PROJECT_KEY} · {BOARD_NAME} · Board {BOARD_ID}
        </div>
        <h1 style={{ fontSize: 22, fontWeight: 700, margin: "0 0 4px" }}>
          Monte Carlo Forecast
          <span style={{ fontSize: 13, fontWeight: 400, color: MUTED, marginLeft: 12 }}>{modeLabel}</span>
        </h1>
        <div style={{ fontSize: 12, color: MUTED, marginBottom: 16 }}>
          Throughput-based simulation · probabilistic delivery horizon
        </div>

        {/* Shared context cards */}
        <div style={{ display: "flex", flexWrap: "wrap", gap: 10, marginBottom: 14 }}>
          <StatCard label="THROUGHPUT (OVERALL)" value={`${ctx.throughput_overall.toFixed(2)}/d`}
            sub={`${ctx.issues_analyzed} items · ${ctx.days_in_sample}d sample`} color={TEXT} />
          <StatCard label="THROUGHPUT (RECENT)" value={`${ctx.throughput_recent.toFixed(2)}/d`}
            sub={`${SIM.throughput_trend.direction} · ${SIM.throughput_trend.percentage_change >= 0 ? "+" : ""}${SIM.throughput_trend.percentage_change.toFixed(0)}%`}
            color={tc} />
          <StatCard label="ITEMS ANALYZED" value={ctx.issues_analyzed}
            sub={`of ${ctx.issues_total} total`} color={TEXT} />
          <StatCard label="DROPPED (OUTCOME)" value={ctx.dropped_by_outcome}
            sub="abandoned/excluded" color={MUTED} />
        </div>

        {/* Warnings */}
        {(SIM.warnings || []).length > 0 && (
          <div style={{ display: "flex", flexWrap: "wrap", gap: 6, marginBottom: 14 }}>
            {SIM.warnings.map((w, i) => <Badge key={i} text={`⚠ ${w}`} color={CAUTION} />)}
          </div>
        )}

        {/* Mode-specific cards */}
        <div style={{ display: "flex", flexWrap: "wrap", gap: 10, marginBottom: 16 }}>
          {isDuration && SIM.composition && (
            <>
              <StatCard label="TOTAL ITEMS" value={SIM.composition.total} color={TEXT} />
              <StatCard label="BACKLOG" value={SIM.composition.existing_backlog}
                sub="unstarted" color={CAUTION} />
              <StatCard label="WIP" value={SIM.composition.wip}
                sub="in progress" color={PRIMARY} />
              {SIM.composition.additional_items > 0 && (
                <StatCard label="ADDITIONAL" value={SIM.composition.additional_items}
                  sub="manually added" color={SECONDARY} />
              )}
            </>
          )}
          {!isDuration && TARGET_DAYS && (
            <>
              <StatCard label="TARGET WINDOW" value={`${TARGET_DAYS}d`}
                sub={`≈ ${Math.round(TARGET_DAYS / 7)} weeks`} color={SECONDARY} />
              <StatCard label="P85 DELIVERY" value={`${SIM.percentiles.likely} items`}
                sub="likely outcome" color={POSITIVE} />
              <StatCard label="P50 DELIVERY" value={`${SIM.percentiles.coin_toss} items`}
                sub="coin toss" color={CAUTION} />
            </>
          )}
        </div>

        {/* Percentile chart */}
        <div style={{ background: PANEL_BG, borderRadius: 12,
          border: `1px solid ${BORDER}`, padding: "14px 8px 12px", marginBottom: 16 }}>
          <div style={{ fontSize: 11, color: MUTED, marginBottom: 8 }}>
            {isDuration
              ? `Days to complete all ${SIM.composition?.total || "?"} items · lower = faster`
              : `Items deliverable within ${TARGET_DAYS || "?"} days · higher = more delivery`}
          </div>
          <ResponsiveContainer width="100%" height={280}>
            <BarChart data={percData} layout="vertical"
              margin={{ top: 4, right: 60, left: 10, bottom: 4 }}>
              <CartesianGrid strokeDasharray="3 3" stroke={BORDER} horizontal={false} />
              <XAxis type="number" domain={[0, maxPerc]}
                tickFormatter={isDuration ? (v => `${v}d`) : (v => `${v}`)}
                tick={{ fill: MUTED, fontSize: 10, fontFamily: FONT_STACK }} />
              <YAxis type="category" dataKey="label" width={36}
                tick={{ fill: TEXT, fontSize: 11, fontFamily: FONT_STACK }} />
              <Tooltip content={PercTooltipWithCtx}
                cursor={{ fill: `${PRIMARY}0c` }} />
              <Bar dataKey="value" barSize={20} radius={[0, 4, 4, 0]} isAnimationActive={false}>
                {percData.map((d, i) => (
                  <Cell key={`cell-${i}`} fill={percColor(d.key)} fillOpacity={0.8} />
                ))}
              </Bar>
              <ReferenceLine x={SIM.percentiles.likely} stroke={percColor("likely")} strokeDasharray="4 4"
                label={{ value: `P85: ${SIM.percentiles.likely}${isDuration ? "d" : ""}`, position: "top",
                  fill: percColor("likely"), fontSize: 10, fontFamily: FONT_STACK }} />
            </BarChart>
          </ResponsiveContainer>
          <div style={{ display: "flex", flexWrap: "wrap", gap: 12, justifyContent: "center", marginTop: 8 }}>
            <div style={{ display: "flex", alignItems: "center", gap: 5 }}>
              <div style={{ width: 14, height: 10, background: percColor("aggressive"), borderRadius: 2, opacity: 0.8 }} />
              <span style={{ fontSize: 10, color: MUTED }}>P10 — Aggressive / low risk</span>
            </div>
            <div style={{ display: "flex", alignItems: "center", gap: 5 }}>
              <div style={{ width: 14, height: 10, background: percColor("unlikely"), borderRadius: 2, opacity: 0.8 }} />
              <span style={{ fontSize: 10, color: MUTED }}>P30 — Unlikely</span>
            </div>
            <div style={{ display: "flex", alignItems: "center", gap: 5 }}>
              <div style={{ width: 14, height: 10, background: percColor("coin_toss"), borderRadius: 2, opacity: 0.8 }} />
              <span style={{ fontSize: 10, color: MUTED }}>P50 — Median</span>
            </div>
            <div style={{ display: "flex", alignItems: "center", gap: 5 }}>
              <div style={{ width: 14, height: 10, background: percColor("probable"), borderRadius: 2, opacity: 0.8 }} />
              <span style={{ fontSize: 10, color: MUTED }}>P70 — Probable</span>
            </div>
            <div style={{ display: "flex", alignItems: "center", gap: 5 }}>
              <div style={{ width: 14, height: 10, background: percColor("likely"), borderRadius: 2, opacity: 0.8 }} />
              <span style={{ fontSize: 10, color: MUTED }}>P85 — SLE / likely</span>
            </div>
            <div style={{ display: "flex", alignItems: "center", gap: 5 }}>
              <div style={{ width: 14, height: 10, background: percColor("safe"), borderRadius: 2, opacity: 0.8 }} />
              <span style={{ fontSize: 10, color: MUTED }}>P95–P98 — Safe to near-certain</span>
            </div>
          </div>
        </div>

        {/* Spread panel */}
        <div style={{ display: "flex", flexWrap: "wrap", gap: 10, marginBottom: 16 }}>
          <StatCard label="PREDICTABILITY" value={SIM.predictability}
            color={predictabilityColor(SIM.predictability)} />
          <StatCard label="IQR SPREAD" value={SIM.spread.iqr}
            sub="interquartile range" color={CAUTION} />
          <StatCard label="INNER 80" value={SIM.spread.inner_80}
            sub="P10–P90 span" color={CAUTION} />
          <StatCard label="FAT-TAIL RATIO" value={`${SIM.fat_tail_ratio}×`}
            sub="tail / median" color={fatTailColor(SIM.fat_tail_ratio)} />
        </div>

        {/* Stratification table */}
        {strat.length > 0 && (
          <div style={{ background: PANEL_BG, borderRadius: 12,
            border: `1px solid ${BORDER}`, padding: "14px 12px", marginBottom: 16,
            overflowX: "auto" }}>
            <div style={{ fontSize: 11, color: MUTED, marginBottom: 10, letterSpacing: "0.05em",
              textTransform: "uppercase" }}>Stratification Decisions</div>
            <table style={{ width: "100%", minWidth: 500, borderCollapse: "collapse", fontSize: 11 }}>
              <thead>
                <tr style={{ borderBottom: `1px solid ${BORDER}` }}>
                  <th style={{ padding: "6px 8px", textAlign: "left", color: MUTED, fontSize: 10, fontWeight: 700, whiteSpace: "nowrap" }}>TYPE</th>
                  <th style={{ padding: "6px 8px", textAlign: "left", color: MUTED, fontSize: 10, fontWeight: 700, whiteSpace: "nowrap" }}>ELIGIBLE</th>
                  <th style={{ padding: "6px 8px", textAlign: "left", color: MUTED, fontSize: 10, fontWeight: 700, whiteSpace: "nowrap" }}>VOLUME</th>
                  <th style={{ padding: "6px 8px", textAlign: "left", color: MUTED, fontSize: 10, fontWeight: 700, whiteSpace: "nowrap" }}>P85 CT</th>
                  <th style={{ padding: "6px 8px", textAlign: "left", color: MUTED, fontSize: 10, fontWeight: 700, whiteSpace: "nowrap" }}>NOTE</th>
                </tr>
              </thead>
              <tbody>
                {strat.map((s, i) => <StratRow key={s.type} s={s} i={i} />)}
              </tbody>
            </table>
          </div>
        )}

        {/* Footer */}
        <div style={{ fontSize: 11, color: MUTED, lineHeight: 1.7,
          borderTop: `1px solid ${BORDER}`, paddingTop: 14 }}>
          <div style={{ marginBottom: 6 }}>
            <b style={{ color: TEXT }}>Reading this chart: </b>
            {isDuration
              ? `Duration mode forecasts when ${SIM.composition?.total || "?"} items will be completed based on historical throughput. Bars show days to completion at each confidence level — shorter is faster. P85 (${SIM.percentiles.likely}d) is the professional commitment standard.`
              : `Scope mode forecasts how many items will be delivered within ${TARGET_DAYS || "?"} days. Bars show item counts — higher means more delivery. P85 (${SIM.percentiles.likely} items) is the professional commitment standard. Note the axis is intentionally reversed from duration mode: higher confidence = fewer items guaranteed.`}
          </div>
          <div>
            <b style={{ color: TEXT }}>Important: </b>
            This simulation is based solely on historical throughput — it does not account for
            scope changes, team changes, or holidays. Stratified modeling means each eligible
            issue type is simulated independently to capture capacity clashes. Ineligible types
            (volume too low) are modeled via the pool distribution.
          </div>
        </div>

      </div>
    </div>
  );
}
