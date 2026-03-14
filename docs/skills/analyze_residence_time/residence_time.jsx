import { useMemo } from "react";
import {
  ComposedChart, Area, Line, ReferenceLine,
  XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer,
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

// ── DERIVED ───────────────────────────────────────────────────────────────────

const rt  = MCP_RESPONSE.data.residence_time;
const sum = rt.summary;

const BOARD_ID    = CHART_ATTRS.board_id;
const PROJECT_KEY = CHART_ATTRS.project_key;
const BOARD_NAME  = CHART_ATTRS.board_name;

const FINAL_W          = sum.final_w;
const FINAL_W_STAR     = sum.final_w_star;
const FINAL_GAP        = sum.final_coherence_gap;
const FINAL_LAMBDA     = sum.final_lambda;
const FINAL_N          = sum.active_items;
const WINDOW_ARRIVALS  = sum.in_window_arrivals;
const PRE_WINDOW_ITEMS = sum.pre_window_items;
const RESOLVED_TOTAL_D = sum.departures;
const CONVERGENCE      = sum.convergence;

// Detect granularity from first label
const isWeekly = (rt.series[0]?.label || "").includes("W");

// Build data array with downsampling for daily granularity
const rawSeries = rt.series.map(d => ({
  date:   d.label,
  n:      d.n,
  w:      Math.round(d.w * 100) / 100,
  w_star: Math.round(d.w_star * 100) / 100,
  gap:    Math.round(d.coherence_gap * 100) / 100,
  lambda: Math.round(d.lambda * 10000) / 10000,
  a:      d.a,
}));

// Downsample daily data if >120 points; always keep first and last
const RAW = rawSeries.length > 120 && !isWeekly
  ? rawSeries.filter((d, i) => i === 0 || i === rawSeries.length - 1 || i % 3 === 0)
  : rawSeries;

const convergenceColor =
  CONVERGENCE === "converging" ? POSITIVE :
  CONVERGENCE === "diverging"  ? ALARM    : CAUTION;

const periodLabel = isWeekly ? "wk" : "d";

// ── AXIS HELPERS ──────────────────────────────────────────────────────────────

function formatDate(label) {
  if (!label) return "";
  if (label.includes("W")) {
    const [yr, wk] = label.split("-");
    return `${wk} '${yr.slice(2)}`;
  }
  const d = new Date(label);
  return d.toLocaleDateString("en-GB", { day: "2-digit", month: "short" });
}

// Y-axis domains
const maxW     = Math.max(...RAW.map(d => Math.max(d.w, d.gap)));
const Y_L_MAX  = Math.ceil((maxW + 5) / 10) * 10;
const minWStar = Math.min(...RAW.map(d => d.w_star));
const maxWStar = Math.max(...RAW.map(d => d.w_star));
const Y_R_MIN  = Math.floor((minWStar - 2) / 5) * 5;
const Y_R_MAX  = Math.ceil((maxWStar + 2) / 5) * 5;
const maxLambda = Math.max(...RAW.map(d => d.lambda));
const Y_L_MAX2  = Math.ceil((maxLambda + 1) / 2) * 2;

const xInterval = isWeekly ? 3 : 6;

// ── SUB-COMPONENTS ────────────────────────────────────────────────────────────

const StatCard = ({ label, value, color }) => (
  <div style={{ background: PANEL_BG, border: `1px solid ${color}33`,
    borderRadius: 8, padding: "8px 14px", minWidth: 110 }}>
    <div style={{ fontSize: 10, color: MUTED, marginBottom: 3, letterSpacing: "0.05em" }}>{label}</div>
    <div style={{ fontSize: 18, fontWeight: 700, color }}>{value}</div>
  </div>
);

const Badge = ({ text, color }) => (
  <span style={{ fontSize: 11, padding: "3px 8px", borderRadius: 4,
    background: `${color}15`, border: `1px solid ${color}40`, color,
    fontFamily: "'Courier New', monospace" }}>{text}</span>
);

const CustomTooltip = ({ active, payload }) => {
  if (!active || !payload?.length) return null;
  const d = payload[0].payload;
  return (
    <div style={{ background: "#0f1117", border: `1px solid ${BORDER}`, borderRadius: 8,
      padding: "10px 14px", fontFamily: "'Courier New', monospace", fontSize: 12, color: TEXT }}>
      <div style={{ fontWeight: 700, marginBottom: 6 }}>{d.date}</div>
      <div style={{ display: "grid", gridTemplateColumns: "1fr auto", rowGap: 3, columnGap: 16 }}>
        <span style={{ color: PRIMARY }}>Residence w̄(T)</span>
        <span>{d.w?.toFixed(1)}d</span>
        <span style={{ color: CAUTION }}>Sojourn W*(T)</span>
        <span>{d.w_star?.toFixed(1)}d</span>
        <span style={{ color: ALARM }}>Gap w̄−W*</span>
        <span>{d.gap?.toFixed(1)}d</span>
      </div>
      <div style={{ borderTop: `1px solid ${BORDER}`, marginTop: 6, paddingTop: 6,
        display: "grid", gridTemplateColumns: "1fr auto", rowGap: 3, columnGap: 16 }}>
        <span style={{ color: SECONDARY }}>λ(T)</span>
        <span>{d.lambda?.toFixed(2)}/{periodLabel}</span>
        <span style={{ color: MUTED }}>Active (n)</span>
        <span>{d.n}</span>
        <span style={{ color: MUTED }}>Arrivals (a)</span>
        <span>{d.a}</span>
      </div>
    </div>
  );
};

// ── MAIN EXPORT ───────────────────────────────────────────────────────────────

export default function ResidenceTimeChart() {
  const data = useMemo(() => RAW, []);

  const dateRange = RAW.length > 0
    ? `${RAW[0].date} – ${RAW[RAW.length - 1].date}` : "";

  return (
    <div style={{ background: PAGE_BG, minHeight: "100vh", padding: "24px 20px",
      fontFamily: "'Courier New', monospace", color: TEXT }}>
      <div style={{ maxWidth: 1100, margin: "0 auto" }}>

        {/* Header */}
        <div style={{ fontSize: 11, color: MUTED, letterSpacing: "0.08em",
          textTransform: "uppercase", marginBottom: 6 }}>
          {PROJECT_KEY} · {BOARD_NAME} · Board {BOARD_ID}
        </div>
        <h1 style={{ fontSize: 22, fontWeight: 700, margin: "0 0 4px" }}>Residence Time Analysis</h1>
        <div style={{ fontSize: 12, color: MUTED, marginBottom: 16 }}>
          Sample Path Analysis · Little's Law L(T) = Λ(T) · w(T) · {dateRange}
        </div>

        {/* Stat cards */}
        <div style={{ display: "flex", flexWrap: "wrap", gap: 10, marginBottom: 14 }}>
          <StatCard label="RESIDENCE w̄(T)" value={`${FINAL_W.toFixed(1)}d`}                 color={PRIMARY} />
          <StatCard label="SOJOURN W*(T)"   value={`${FINAL_W_STAR.toFixed(1)}d`}            color={CAUTION} />
          <StatCard label="COHERENCE GAP"   value={`${FINAL_GAP.toFixed(1)}d`}               color={ALARM} />
          <StatCard label="λ(T)"            value={`${FINAL_LAMBDA.toFixed(2)}/${periodLabel}`} color={SECONDARY} />
          <StatCard label="ACTIVE WIP"      value={FINAL_N}                                   color={MUTED} />
        </div>

        {/* Badges */}
        <div style={{ display: "flex", flexWrap: "wrap", gap: 6, marginBottom: 20, alignItems: "center" }}>
          <Badge text={`Window arrivals (a): ${WINDOW_ARRIVALS}`} color={PRIMARY} />
          <Badge text={`Pre-window items: ${PRE_WINDOW_ITEMS}`} color={MUTED} />
          <Badge text={`Resolved (d): ${RESOLVED_TOTAL_D} (W* denominator)`} color={CAUTION} />
          <Badge text="L(T) = Λ(T) · w(T) ✓" color={CAUTION} />
          <Badge text={`Convergence: ${CONVERGENCE}`} color={convergenceColor} />
        </div>

        {/* Panel 1: Residence Time vs Sojourn Time */}
        <div style={{ background: PANEL_BG, borderRadius: 12,
          border: `1px solid ${BORDER}`, padding: "16px 8px 12px", marginBottom: 16 }}>
          <div style={{ fontSize: 11, color: MUTED, marginBottom: 8 }}>
            Residence Time w̄(T) vs Sojourn Time W*(T) — Coherence Gap
          </div>
          <ResponsiveContainer width="100%" height={380}>
            <ComposedChart data={data} margin={{ top: 10, right: 70, left: 10, bottom: 10 }}>
              <defs>
                <linearGradient id="gapGrad" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="5%"  stopColor={ALARM}   stopOpacity={0.20} />
                  <stop offset="95%" stopColor={ALARM}   stopOpacity={0.02} />
                </linearGradient>
                <linearGradient id="wGrad" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="5%"  stopColor={PRIMARY} stopOpacity={0.25} />
                  <stop offset="95%" stopColor={PRIMARY} stopOpacity={0.02} />
                </linearGradient>
              </defs>
              <CartesianGrid strokeDasharray="3 3" stroke={BORDER} vertical={false} />
              <XAxis dataKey="date" tickFormatter={formatDate} interval={xInterval}
                angle={-45} textAnchor="end" height={60}
                tick={{ fill: MUTED, fontSize: 10, fontFamily: "'Courier New', monospace" }} />
              <YAxis yAxisId="left" domain={[0, Y_L_MAX]} tickFormatter={v => `${v}d`}
                tick={{ fill: PRIMARY, fontSize: 10, fontFamily: "'Courier New', monospace" }}
                label={{ value: "Days", angle: -90, position: "insideLeft",
                  fill: PRIMARY, fontSize: 10, dy: 20 }} />
              <YAxis yAxisId="right" orientation="right" domain={[Y_R_MIN, Y_R_MAX]}
                tickFormatter={v => `${v}d`}
                tick={{ fill: CAUTION, fontSize: 10, fontFamily: "'Courier New', monospace" }}
                label={{ value: "W* (days)", angle: 90, position: "insideRight",
                  fill: CAUTION, fontSize: 10, dy: -30 }} />
              <Tooltip content={<CustomTooltip />} />
              <Area yAxisId="left" dataKey="gap" fill="url(#gapGrad)"
                stroke={ALARM} strokeWidth={1.5} strokeOpacity={0.7}
                dot={false} activeDot={false} isAnimationActive={false} />
              <Line yAxisId="left" dataKey="w" stroke={PRIMARY} strokeWidth={2}
                dot={false} activeDot={false} isAnimationActive={false} />
              <Line yAxisId="right" dataKey="w_star" stroke={CAUTION} strokeWidth={2}
                strokeDasharray="4 3" dot={false} activeDot={false} isAnimationActive={false} />
            </ComposedChart>
          </ResponsiveContainer>

          {/* Legend Panel 1 */}
          <div style={{ display: "flex", flexWrap: "wrap", gap: 14, justifyContent: "center", marginTop: 8 }}>
            <div style={{ display: "flex", alignItems: "center", gap: 5 }}>
              <svg width={24} height={12}><line x1={0} y1={6} x2={24} y2={6} stroke={PRIMARY} strokeWidth={2} /></svg>
              <span style={{ fontSize: 11, color: MUTED }}>Residence w̄(T)</span>
            </div>
            <div style={{ display: "flex", alignItems: "center", gap: 5 }}>
              <svg width={24} height={12}><line x1={0} y1={6} x2={24} y2={6} stroke={CAUTION} strokeWidth={2} strokeDasharray="4 3" /></svg>
              <span style={{ fontSize: 11, color: MUTED }}>Sojourn W*(T)</span>
            </div>
            <div style={{ display: "flex", alignItems: "center", gap: 5 }}>
              <svg width={14} height={10}><rect width={14} height={10} fill={ALARM} opacity={0.4} rx={2} /></svg>
              <span style={{ fontSize: 11, color: MUTED }}>Coherence Gap</span>
            </div>
          </div>
        </div>

        {/* Panel 2: Departure Rate */}
        <div style={{ background: PANEL_BG, borderRadius: 12,
          border: `1px solid ${BORDER}`, padding: "16px 8px 12px", marginBottom: 16 }}>
          <div style={{ fontSize: 11, color: MUTED, marginBottom: 8 }}>
            Departure Rate λ(T) — Cumulative departures per {isWeekly ? "week" : "day"} (d / horizon)
          </div>
          <ResponsiveContainer width="100%" height={260}>
            <ComposedChart data={data} margin={{ top: 10, right: 70, left: 10, bottom: 10 }}>
              <defs>
                <linearGradient id="lambdaGrad" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="5%"  stopColor={SECONDARY} stopOpacity={0.25} />
                  <stop offset="95%" stopColor={SECONDARY} stopOpacity={0.02} />
                </linearGradient>
              </defs>
              <CartesianGrid strokeDasharray="3 3" stroke={BORDER} vertical={false} />
              <XAxis dataKey="date" tickFormatter={formatDate} interval={xInterval}
                angle={-45} textAnchor="end" height={60}
                tick={{ fill: MUTED, fontSize: 10, fontFamily: "'Courier New', monospace" }} />
              <YAxis domain={[0, Y_L_MAX2]} tickFormatter={v => v.toFixed(1)}
                tick={{ fill: SECONDARY, fontSize: 10, fontFamily: "'Courier New', monospace" }}
                label={{ value: `λ (dep/${periodLabel})`, angle: -90, position: "insideLeft",
                  fill: SECONDARY, fontSize: 10, dy: 30 }} />
              <Tooltip content={<CustomTooltip />} />
              <ReferenceLine y={1.0} stroke={MUTED} strokeDasharray="4 4"
                label={{ value: "λ = 1", fill: MUTED, fontSize: 10, position: "right" }} />
              <Area dataKey="lambda" fill="url(#lambdaGrad)"
                stroke={SECONDARY} strokeWidth={2}
                dot={false} activeDot={false} isAnimationActive={false} />
            </ComposedChart>
          </ResponsiveContainer>

          {/* Legend Panel 2 */}
          <div style={{ display: "flex", flexWrap: "wrap", gap: 14, justifyContent: "center", marginTop: 8 }}>
            <div style={{ display: "flex", alignItems: "center", gap: 5 }}>
              <svg width={14} height={10}><rect width={14} height={10} fill={SECONDARY} opacity={0.4} rx={2} /></svg>
              <span style={{ fontSize: 11, color: MUTED }}>Departure Rate λ(T)</span>
            </div>
            <div style={{ display: "flex", alignItems: "center", gap: 5 }}>
              <svg width={24} height={12}><line x1={0} y1={6} x2={24} y2={6} stroke={MUTED} strokeDasharray="4 4" strokeWidth={1.5} /></svg>
              <span style={{ fontSize: 11, color: MUTED }}>λ = 1 (equilibrium)</span>
            </div>
          </div>
        </div>

        {/* Footer */}
        <div style={{ fontSize: 11, color: MUTED, lineHeight: 1.7,
          borderTop: `1px solid ${BORDER}`, paddingTop: 14 }}>
          <div style={{ marginBottom: 6 }}>
            <b style={{ color: TEXT }}>Reading this chart: </b>
            Top panel: residence time w̄(T) (blue, left axis) — average time items spend in
            system including active WIP — vs sojourn time W*(T) (amber dashed, right axis) —
            average cycle time of completed items only. Red shaded area = coherence gap: how
            much active WIP inflates average residence time beyond completed-item experience.
            A diverging/growing gap signals aging WIP accumulation. Bottom panel: cumulative
            departure rate λ(T) = d(T)/h(T); above 1.0 means the system resolves more than
            one item per elapsed {isWeekly ? "week" : "day"} on average.
          </div>
          <div>
            <b style={{ color: TEXT }}>Data: </b>
            Sample Path Analysis (Stidham 1972, El-Taha & Stidham 1999). Little's Law identity
            L(T) = Λ(T) · w(T) verified exactly. Window arrivals (a): items entering committed
            WIP during the window. Resolved (d): {RESOLVED_TOTAL_D} total historical departures
            used as W* denominator. Note: this tool always applies backflow reset (last commitment
            date), which may diverge from other tools using configurable backflow settings.
          </div>
        </div>

      </div>
    </div>
  );
}
