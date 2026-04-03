import {
  ComposedChart, Bar, Cell, Line, XAxis, YAxis,
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

function accuracyColor(score) {
  if (score >= 0.80) return POSITIVE;
  if (score >= 0.65) return CAUTION;
  return ALARM;
}

// ── DERIVED ───────────────────────────────────────────────────────────────────

const BOARD_ID    = __MCS_WORKFLOW__.board_id;
const PROJECT_KEY = __MCS_WORKFLOW__.project_key;
const BOARD_NAME  = __MCS_WORKFLOW__.board_name;

// The server renders one backtest per tool call. The accuracy result is
// directly in __MCS_DATA__.accuracy with raw API field names.
const RAW = __MCS_DATA__.accuracy;
// Normalize checkpoint field names for the template.
const BACKTEST = RAW ? {
  accuracy_score:  RAW.accuracy_score,
  hits:            (RAW.checkpoints || []).filter(c => c.is_within_cone).length,
  total:           (RAW.checkpoints || []).length,
  validation_msg:  RAW.validation_message,
  checkpoints:     (RAW.checkpoints || []).map(c => ({
    date:   c.date,
    actual: c.actual_value,
    p50:    c.predicted_p50,
    p85:    c.predicted_p85,
    p95:    c.predicted_p95,
    hit:    c.is_within_cone,
    drift:  c.drift_detected,
  })),
} : null;
// Backtest is always scope mode (hardcoded in the handler).
const isDuration = false;

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

function Swatch({ color, label, dashed }) {
  return (
    <div style={{ display: "flex", alignItems: "center", gap: 5 }}>
      {dashed
        ? <svg width={20} height={10}>
            <line x1={0} y1={5} x2={20} y2={5}
              stroke={color} strokeDasharray={dashed} strokeWidth={1.5} />
          </svg>
        : <div style={{ width: 14, height: 10, background: color,
            borderRadius: 2, opacity: 0.85 }} />
      }
      <span style={{ fontSize: 11, color: MUTED,
        fontFamily: "'Courier New', monospace" }}>{label}</span>
    </div>
  );
}

const shortDate = iso => {
  if (!iso) return "";
  const parts = iso.split("-");
  return `${parts[1]}/${parts[2]}`;
};

// ── BACKTEST PANEL ────────────────────────────────────────────────────────────

function BacktestPanel({ data, isDuration }) {
  const { accuracy_score, hits, total, validation_msg, checkpoints } = data;
  const misses   = total - hits;
  const driftPts = checkpoints.filter(c => c.drift);
  const pct      = Math.round(accuracy_score * 100);
  const ac       = accuracyColor(accuracy_score);
  const unitShort = isDuration ? "d" : "";
  const unitLong  = isDuration ? " days" : " items";

  // Chronological order (API returns newest-first)
  const chartData = [...checkpoints].reverse();

  const yMax = Math.ceil(
    Math.max(...checkpoints.flatMap(c => [c.actual, c.p50, c.p95])) * 1.15
  );

  const missRows = checkpoints.filter(c => !c.hit);

  const CheckpointTooltip = ({ active, payload }) => {
    if (!active || !payload?.length) return null;
    const d = payload[0].payload;
    return (
      <div style={{ background: "#0f1117", border: `1px solid ${BORDER}`, borderRadius: 8,
        padding: "10px 14px", fontFamily: "'Courier New', monospace", fontSize: 12, color: TEXT }}>
        <div style={{ fontWeight: 700, marginBottom: 6 }}>{d.date}</div>
        <div style={{ color: d.hit ? POSITIVE : ALARM, fontWeight: 700 }}>
          Actual: {d.actual.toFixed(1)}{unitLong}
        </div>
        <div style={{ color: SECONDARY }}>P50: {d.p50}{unitLong}</div>
        <div style={{ color: CAUTION }}>P85: {d.p85}{unitLong}</div>
        <div style={{ color: PRIMARY }}>P95: {d.p95}{unitLong}</div>
        <div style={{ marginTop: 6, fontWeight: 700,
          color: d.hit ? POSITIVE : ALARM }}>
          {d.hit ? "✓ Within cone" : "✗ Outside cone"}
        </div>
      </div>
    );
  };

  return (
    <>
      {/* Reliability Panel */}
      <div style={{ background: PANEL_BG, borderRadius: 12,
        border: `1px solid ${BORDER}`, padding: "14px 16px", marginBottom: 16 }}>
        <div style={{ display: "flex", justifyContent: "space-between", fontSize: 10,
          color: MUTED, marginBottom: 4 }}>
          <span>0%</span>
          <span style={{ color: ac }}>{pct}% accuracy — {hits}/{total} checkpoints within cone</span>
          <span>100%</span>
        </div>
        <div style={{ position: "relative", height: 14, background: "#12141e",
          borderRadius: 6, overflow: "hidden", marginBottom: 6 }}>
          <div style={{ position: "absolute", top: 0, bottom: 0, left: 0,
            width: `${pct}%`, background: ac, opacity: 0.75, borderRadius: 6 }} />
          <div style={{ position: "absolute", top: 0, bottom: 0, left: "65%",
            width: 1, background: CAUTION, opacity: 0.6 }} />
          <div style={{ position: "absolute", top: 0, bottom: 0, left: "80%",
            width: 1, background: POSITIVE, opacity: 0.6 }} />
        </div>
        <div style={{ display: "flex", gap: 16, fontSize: 10, color: MUTED, marginBottom: 8 }}>
          <span><span style={{ color: ALARM }}>■</span> &lt;65% unreliable</span>
          <span><span style={{ color: CAUTION }}>■</span> 65–79% moderate</span>
          <span><span style={{ color: POSITIVE }}>■</span> ≥80% reliable</span>
        </div>
        {validation_msg && (
          <div style={{ fontSize: 11, color: MUTED, fontStyle: "italic" }}>{validation_msg}</div>
        )}
      </div>

      {/* Badges */}
      <div style={{ display: "flex", flexWrap: "wrap", gap: 6, marginBottom: 16, alignItems: "center" }}>
        <Badge text={`Accuracy: ${pct}%`} color={ac} />
        <Badge text={`Misses: ${misses} / ${total}`} color={misses > 0 ? ALARM : POSITIVE} />
        {driftPts.length > 0 && (
          <Badge text={`Drift: ${driftPts.length} checkpoint(s)`} color={ALARM} />
        )}
        <Badge text={isDuration ? "Duration — days to completion" : "Scope — items per window"} color={MUTED} />
      </div>

      {/* Main Chart */}
      <div style={{ background: PANEL_BG, borderRadius: 12,
        border: `1px solid ${BORDER}`, padding: "16px 20px 0 20px", marginBottom: 16 }}>
        <div style={{ fontSize: 12, fontWeight: 700, marginBottom: 4 }}>
          Actual vs. Predicted — Chronological Checkpoints
        </div>
        <div style={{ fontSize: 11, color: MUTED, marginBottom: 10 }}>
          {isDuration
            ? "Actual days to delivery vs. simulated P50 / P85 / P95 at each past checkpoint"
            : "Actual items delivered vs. simulated P50 / P85 / P95 at each past checkpoint"}
        </div>

        <div style={{ display: "flex", flexWrap: "wrap", gap: 12, marginBottom: 10 }}>
          <Swatch color={POSITIVE}  label="Actual (within cone)" />
          <Swatch color={ALARM}     label="Actual (outside cone)" />
          <Swatch color={SECONDARY} label="P50 predicted" dashed="4 2" />
          <Swatch color={CAUTION}   label="P85 predicted" dashed="6 3" />
          <Swatch color={PRIMARY}   label="P95 predicted" dashed="3 3" />
        </div>

        <ResponsiveContainer width="100%" height={440}>
          <ComposedChart data={chartData}
            margin={{ top: 8, right: 20, left: 10, bottom: 50 }}>
            <CartesianGrid strokeDasharray="3 3" stroke={BORDER} />
            <XAxis dataKey="date" tickFormatter={shortDate}
              tick={{ fill: MUTED, fontSize: 10, fontFamily: "'Courier New', monospace" }}
              angle={-45} textAnchor="end" height={50} interval={1} />
            <YAxis domain={[0, yMax]} tickFormatter={v => `${v}${unitShort}`}
              tick={{ fill: MUTED, fontSize: 10, fontFamily: "'Courier New', monospace" }} />
            <Tooltip content={<CheckpointTooltip />} cursor={{ fill: `${PRIMARY}0c` }} />
            <Bar dataKey="actual" barSize={16} radius={[3, 3, 0, 0]} isAnimationActive={false}>
              {chartData.map((d, i) => (
                <Cell key={`cell-${i}`}
                  fill={d.hit ? POSITIVE : ALARM}
                  fillOpacity={d.hit ? 0.6 : 0.85} />
              ))}
            </Bar>
            <Line type="monotone" dataKey="p50" stroke={SECONDARY}
              strokeDasharray="4 2" strokeWidth={1.5} dot={false} isAnimationActive={false} />
            <Line type="monotone" dataKey="p85" stroke={CAUTION}
              strokeDasharray="6 3" strokeWidth={1.5} dot={false} isAnimationActive={false} />
            <Line type="monotone" dataKey="p95" stroke={PRIMARY}
              strokeDasharray="3 3" strokeWidth={1} dot={false} isAnimationActive={false} />
          </ComposedChart>
        </ResponsiveContainer>
      </div>

      {/* Miss Detail Table */}
      {missRows.length > 0 && (
        <div style={{ background: PANEL_BG, borderRadius: 12,
          border: `1px solid ${BORDER}`, padding: "14px 12px", marginBottom: 16,
          overflowX: "auto" }}>
          <div style={{ fontSize: 11, color: MUTED, marginBottom: 10, letterSpacing: "0.05em",
            textTransform: "uppercase" }}>Miss Details ({missRows.length})</div>
          <table style={{ width: "100%", borderCollapse: "collapse", fontSize: 11 }}>
            <thead>
              <tr style={{ borderBottom: `1px solid ${BORDER}` }}>
                {["DATE", "ACTUAL", "P50", "P85", "P95", "DIRECTION"].map(h => (
                  <th key={h} style={{ padding: "6px 8px", textAlign: "left", color: MUTED,
                    fontSize: 10, fontWeight: 700 }}>{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {missRows.map((d, i) => {
                const over = isDuration ? d.actual < d.p50 : d.actual > d.p50;
                const dirLabel = isDuration
                  ? (over ? "Under (faster)" : "Over (slower)")
                  : (over ? "Over (more)" : "Under (less)");
                return (
                  <tr key={d.date} style={{ background: i % 2 === 0 ? "transparent" : `${PRIMARY}05` }}>
                    <td style={{ padding: "5px 8px", color: CAUTION }}>{d.date}</td>
                    <td style={{ padding: "5px 8px", color: ALARM, fontWeight: 700 }}>
                      {d.actual.toFixed(1)}{unitLong}</td>
                    <td style={{ padding: "5px 8px", color: SECONDARY }}>{d.p50}{unitLong}</td>
                    <td style={{ padding: "5px 8px", color: CAUTION }}>{d.p85}{unitLong}</td>
                    <td style={{ padding: "5px 8px", color: PRIMARY }}>{d.p95}{unitLong}</td>
                    <td style={{ padding: "5px 8px", color: over ? POSITIVE : ALARM }}>{dirLabel}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </>
  );
}

// ── MAIN EXPORT ───────────────────────────────────────────────────────────────

export default function BacktestChart() {
  if (!BACKTEST) return <div style={{ color: ALARM, padding: 40 }}>No backtest data available.</div>;

  const modeLabel = isDuration ? "Duration mode" : "Scope mode";

  return (
    <div style={{ background: PAGE_BG, minHeight: "100vh", padding: "24px 20px",
      fontFamily: "'Courier New', monospace", color: TEXT }}>
      <div style={{ maxWidth: 1100, margin: "0 auto" }}>

        {/* Header */}
        <div style={{ fontSize: 11, color: MUTED, letterSpacing: "0.08em",
          textTransform: "uppercase", marginBottom: 6 }}>
          {PROJECT_KEY} · {BOARD_NAME} · Board {BOARD_ID}
        </div>
        <h1 style={{ fontSize: 22, fontWeight: 700, margin: "0 0 4px" }}>
          Walk-Forward Backtest
          <span style={{ fontSize: 13, fontWeight: 400, color: MUTED, marginLeft: 12 }}>{modeLabel}</span>
        </h1>
        <div style={{ fontSize: 12, color: MUTED, marginBottom: 16 }}>
          Empirical validation of Monte Carlo forecast accuracy · time-travel reconstruction
        </div>

        {/* Stat cards */}
        <div style={{ display: "flex", flexWrap: "wrap", gap: 10, marginBottom: 16 }}>
          <StatCard label="ACCURACY" value={`${Math.round(BACKTEST.accuracy_score * 100)}%`}
            color={accuracyColor(BACKTEST.accuracy_score)} />
          <StatCard label="WITHIN CONE" value={`${BACKTEST.hits} / ${BACKTEST.total}`}
            sub="checkpoints" color={PRIMARY} />
          <StatCard label="MISSES" value={BACKTEST.total - BACKTEST.hits}
            sub="outside cone" color={ALARM} />
          <StatCard label="DRIFT SIGNALS"
            value={BACKTEST.checkpoints.filter(c => c.drift).length}
            sub="process shifts"
            color={BACKTEST.checkpoints.filter(c => c.drift).length > 0 ? ALARM : POSITIVE} />
          <StatCard label="MODE" value={isDuration ? "Duration" : "Scope"}
            sub={isDuration ? "days to completion" : "items per window"} color={SECONDARY} />
        </div>

        <BacktestPanel data={BACKTEST} isDuration={isDuration} />

        {/* Footer */}
        <div style={{ fontSize: 11, color: MUTED, lineHeight: 1.7,
          borderTop: `1px solid ${BORDER}`, paddingTop: 14 }}>
          <div style={{ marginBottom: 6 }}>
            <b style={{ color: TEXT }}>Reading this chart: </b>
            Each bar is a past checkpoint where the system reconstructed the Jira state at that
            date, ran a Monte Carlo simulation, and checked whether the actual outcome fell inside
            the predicted cone (P10–P98). Green bars are hits; red bars are misses. The P50, P85,
            and P95 lines show what the simulation predicted at that moment in time.
            {isDuration
              ? " In duration mode, the actual value is the number of days the forecasted items actually took to deliver."
              : " In scope mode, the actual value is the number of items actually delivered within the forecast window."}
          </div>
          <div style={{ marginBottom: 6 }}>
            <b style={{ color: CAUTION }}>Reliability thresholds: </b>
            ≥80% is reliable (green). 65–79% is moderate (caution). Below 65% means historical
            throughput is not a stable predictor — Monte Carlo results should be treated with low
            confidence.
          </div>
          <div>
            <b style={{ color: TEXT }}>Important: </b>
            The walk-forward analysis stops automatically if a process drift (system shift via
            3-way control chart) is detected — drift-contaminated checkpoints are excluded from
            accuracy scoring.
          </div>
        </div>

      </div>
    </div>
  );
}
