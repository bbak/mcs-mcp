import { useMemo } from "react";
import {
  ComposedChart, Bar, Line, Area, ReferenceLine,
  XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer,
} from "recharts";
import { ALARM, CAUTION, PRIMARY, SECONDARY, POSITIVE, TEXT, MUTED, PAGE_BG, PANEL_BG, BORDER, FONT_STACK } from "mcs-mcp";
import { StatCard, Badge, TOOLTIP_BG } from "./shared.jsx";

// ── INJECTED DATA ─────────────────────────────────────────────────────────────
// Payload is injected by the MCS chart renderer as window.__MCS_PAYLOAD__.

const __MCS_ENVELOPE__ = window.__MCS_PAYLOAD__;
const __MCS_DATA__ = __MCS_ENVELOPE__.data;
const __MCS_GUARDRAILS__ = __MCS_ENVELOPE__.guardrails;
const __MCS_WORKFLOW__ = __MCS_ENVELOPE__.workflow;
// ── CONFIG ────────────────────────────────────────────────────────────────────

// ── DERIVED ───────────────────────────────────────────────────────────────────

const fd = __MCS_DATA__.flow_debt;
const guardrails = __MCS_GUARDRAILS__;

const BOARD_ID    = __MCS_WORKFLOW__.board_id;
const PROJECT_KEY = __MCS_WORKFLOW__.project_key;
const BOARD_NAME  = __MCS_WORKFLOW__.board_name;

const TOTAL_DEBT = fd.totalDebt;
const BUCKETS    = fd.buckets;

const MONTH_NAMES = ["Jan","Feb","Mar","Apr","May","Jun",
                     "Jul","Aug","Sep","Oct","Nov","Dec"];

function shortLabel(l) {
  if (/W\d+/.test(l)) return l.replace(/^\d{4}-/, "");
  const parts = l.split("-");
  if (parts.length === 2)
    return MONTH_NAMES[parseInt(parts[1], 10) - 1] || l;
  return l;
}

const round1 = v => Math.round((v || 0) * 10) / 10;

// ── SUB-COMPONENTS ────────────────────────────────────────────────────────────

const debtDotColor = d => d > 0 ? ALARM : d < 0 ? POSITIVE : MUTED;

const MainTooltip = ({ active, payload }) => {
  if (!active || !payload?.length) return null;
  const d = payload[0].payload;
  const dc = debtDotColor(d.debt);
  return (
    <div style={{ background: TOOLTIP_BG, border: `1px solid ${BORDER}`, borderRadius: 8,
      padding: "10px 14px", fontFamily: FONT_STACK, fontSize: 12, color: TEXT }}>
      <div style={{ fontWeight: 700, marginBottom: 6 }}>{d.label}</div>
      <div style={{ display: "grid", gridTemplateColumns: "1fr auto", rowGap: 3, columnGap: 16 }}>
        <span style={{ color: PRIMARY }}>Arrivals</span><span>{d.arrivals}</span>
        <span style={{ color: SECONDARY }}>Departures</span><span>{d.departures}</span>
        <span style={{ color: dc }}>Debt</span>
        <span style={{ color: dc }}>{d.debt > 0 ? "+" : ""}{d.debt}</span>
      </div>
    </div>
  );
};

const CumTooltip = ({ active, payload }) => {
  if (!active || !payload?.length) return null;
  const d = payload[0].payload;
  const wc = debtDotColor(d.debt);
  const cc = debtDotColor(d.cumDebt);
  return (
    <div style={{ background: TOOLTIP_BG, border: `1px solid ${BORDER}`, borderRadius: 8,
      padding: "10px 14px", fontFamily: FONT_STACK, fontSize: 12, color: TEXT }}>
      <div style={{ fontWeight: 700, marginBottom: 6 }}>{d.label}</div>
      <div style={{ display: "grid", gridTemplateColumns: "1fr auto", rowGap: 3, columnGap: 16 }}>
        <span style={{ color: wc }}>Week debt</span>
        <span style={{ color: wc }}>{d.debt > 0 ? "+" : ""}{d.debt}</span>
        <span style={{ color: cc, fontWeight: 700 }}>Cumulative</span>
        <span style={{ color: cc, fontWeight: 700 }}>{d.cumDebt > 0 ? "+" : ""}{d.cumDebt}</span>
      </div>
    </div>
  );
};

// ── MAIN EXPORT ───────────────────────────────────────────────────────────────

export default function FlowDebtChart() {
  const enriched = useMemo(() => {
    let running = 0;
    return BUCKETS.map(b => {
      running = round1(running + b.debt);
      return { ...b, debt: round1(b.debt), short: shortLabel(b.label), cumDebt: running };
    });
  }, []);

  const avgArrivals   = round1(BUCKETS.reduce((s, b) => s + b.arrivals, 0) / BUCKETS.length);
  const avgDepartures = round1(BUCKETS.reduce((s, b) => s + b.departures, 0) / BUCKETS.length);
  const weeksPositive = BUCKETS.filter(b => b.debt > 0).length;
  const weeksNegative = BUCKETS.filter(b => b.debt < 0).length;
  const debtColor = TOTAL_DEBT > 0 ? ALARM : TOTAL_DEBT < 0 ? POSITIVE : MUTED;

  const maxVol     = Math.ceil(Math.max(...BUCKETS.map(b => Math.max(b.arrivals, b.departures))) * 1.15);
  const debtExtent = Math.ceil(Math.max(...BUCKETS.map(b => Math.abs(b.debt)), 1));
  const cumExtent  = Math.ceil(Math.max(...enriched.map(b => Math.abs(b.cumDebt)), 1));

  const step  = Math.max(1, Math.ceil(enriched.length / 8));
  const xTick = (value, index) => index % step === 0 ? value : "";

  return (
    <div style={{ background: PAGE_BG, minHeight: "100vh", padding: "24px 20px",
      fontFamily: FONT_STACK, color: TEXT }}>
      <div style={{ maxWidth: 1100, margin: "0 auto" }}>

        {/* Header */}
        <div style={{ fontSize: 11, color: MUTED, letterSpacing: "0.08em",
          textTransform: "uppercase", marginBottom: 6 }}>
          {PROJECT_KEY} · {BOARD_NAME} · Board {BOARD_ID}
        </div>
        <h1 style={{ fontSize: 22, fontWeight: 700, margin: "0 0 4px" }}>Flow Debt</h1>
        <div style={{ fontSize: 12, color: MUTED, marginBottom: 16 }}>
          Arrivals vs. departures · leading indicator of cycle time inflation
        </div>

        {/* Stat cards */}
        <div style={{ display: "flex", flexWrap: "wrap", gap: 10, marginBottom: 14 }}>
          <StatCard label="TOTAL DEBT" value={`${TOTAL_DEBT > 0 ? "+" : ""}${TOTAL_DEBT}`}
            sub={`over ${BUCKETS.length} buckets`} color={debtColor} />
          <StatCard label="AVG ARRIVALS"   value={avgArrivals}   sub="per bucket" color={PRIMARY} />
          <StatCard label="AVG DEPARTURES" value={avgDepartures} sub="per bucket" color={SECONDARY} />
          <StatCard label="WEEKS POSITIVE" value={weeksPositive}
            sub="arrivals > departures" color={ALARM} />
          <StatCard label="WEEKS NEGATIVE" value={weeksNegative}
            sub="departures > arrivals" color={POSITIVE} />
        </div>

        {/* Guardrail badges */}
        <div style={{ display: "flex", flexWrap: "wrap", gap: 6, marginBottom: 20, alignItems: "center" }}>
          <Badge text="Positive debt = WIP growing → future cycle time inflation" color={ALARM} />
          <Badge text="Zero / negative debt = stable or improving throughput ratio" color={POSITIVE} />
          <Badge text="Leading indicator — signals problems before delays appear" color={CAUTION} />
        </div>

        {/* Panel 1: Arrivals vs Departures + Debt Line */}
        <div style={{ background: PANEL_BG, borderRadius: 12,
          border: `1px solid ${BORDER}`, padding: "14px 8px 12px", marginBottom: 16 }}>
          <div style={{ fontSize: 11, color: MUTED, marginBottom: 8 }}>
            Arrivals vs. departures · weekly debt (line, right axis)
          </div>
          <ResponsiveContainer width="100%" height={300}>
            <ComposedChart data={enriched} margin={{ top: 8, right: 24, left: 8, bottom: 4 }}>
              <CartesianGrid strokeDasharray="3 3" stroke={BORDER} vertical={false} />
              <XAxis dataKey="short" tickFormatter={xTick}
                tick={{ fill: MUTED, fontSize: 10, fontFamily: FONT_STACK }} />
              <YAxis yAxisId="vol" domain={[0, maxVol]}
                tick={{ fill: MUTED, fontSize: 10, fontFamily: FONT_STACK }}
                label={{ value: "items", angle: -90, position: "insideLeft",
                  fill: MUTED, fontSize: 10 }} />
              <YAxis yAxisId="debt" orientation="right"
                domain={[-debtExtent, debtExtent]}
                tick={{ fill: CAUTION, fontSize: 10, fontFamily: FONT_STACK }}
                label={{ value: "debt", angle: 90, position: "insideRight",
                  fill: CAUTION, fontSize: 10, dy: -20 }} />
              <ReferenceLine yAxisId="debt" y={0} stroke={MUTED} strokeDasharray="4 4" />
              <Tooltip content={MainTooltip} />
              <Bar yAxisId="vol" dataKey="arrivals"   barSize={10} fill={PRIMARY}
                fillOpacity={0.75} radius={[3, 3, 0, 0]} isAnimationActive={false} />
              <Bar yAxisId="vol" dataKey="departures" barSize={10} fill={SECONDARY}
                fillOpacity={0.75} radius={[3, 3, 0, 0]} isAnimationActive={false} />
              <Line yAxisId="debt" dataKey="debt" type="monotone"
                stroke={CAUTION} strokeWidth={2} isAnimationActive={false}
                dot={(props) => {
                  const { cx, cy, payload } = props;
                  if (!cx || !cy) return null;
                  const color = debtDotColor(payload.debt);
                  return <circle key={cx} cx={cx} cy={cy} r={3} fill={color} stroke="none" />;
                }} />
            </ComposedChart>
          </ResponsiveContainer>

          <div style={{ display: "flex", flexWrap: "wrap", gap: 12, justifyContent: "center", marginTop: 8 }}>
            <div style={{ display: "flex", alignItems: "center", gap: 5 }}>
              <div style={{ width: 14, height: 10, background: PRIMARY, borderRadius: 2, opacity: 0.75 }} />
              <span style={{ fontSize: 10, color: MUTED }}>Arrivals (commitments)</span>
            </div>
            <div style={{ display: "flex", alignItems: "center", gap: 5 }}>
              <div style={{ width: 14, height: 10, background: SECONDARY, borderRadius: 2, opacity: 0.75 }} />
              <span style={{ fontSize: 10, color: MUTED }}>Departures (deliveries)</span>
            </div>
            <div style={{ display: "flex", alignItems: "center", gap: 5 }}>
              <div style={{ width: 16, height: 2, background: CAUTION }} />
              <div style={{ width: 8, height: 8, borderRadius: "50%", background: ALARM }} />
              <span style={{ fontSize: 10, color: MUTED }}>Weekly debt (dot: red=positive, green=negative)</span>
            </div>
          </div>
        </div>

        {/* Panel 2: Cumulative Flow Debt */}
        <div style={{ background: PANEL_BG, borderRadius: 12,
          border: `1px solid ${BORDER}`, padding: "14px 8px 12px", marginBottom: 16 }}>
          <div style={{ fontSize: 11, color: MUTED, marginBottom: 8 }}>
            Cumulative flow debt · systemic drift over time
          </div>
          <ResponsiveContainer width="100%" height={220}>
            <ComposedChart data={enriched} margin={{ top: 8, right: 24, left: 8, bottom: 4 }}>
              <defs>
                <linearGradient id="cumGrad" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="5%"  stopColor={ALARM} stopOpacity={0.3} />
                  <stop offset="95%" stopColor={ALARM} stopOpacity={0.02} />
                </linearGradient>
              </defs>
              <CartesianGrid strokeDasharray="3 3" stroke={BORDER} vertical={false} />
              <XAxis dataKey="short" tickFormatter={xTick}
                tick={{ fill: MUTED, fontSize: 10, fontFamily: FONT_STACK }} />
              <YAxis domain={[-cumExtent, cumExtent]}
                tick={{ fill: ALARM, fontSize: 10, fontFamily: FONT_STACK }}
                label={{ value: "cumulative debt", angle: -90, position: "insideLeft",
                  fill: ALARM, fontSize: 10, dy: 50 }} />
              <ReferenceLine y={0} stroke={MUTED} strokeDasharray="4 4" />
              <Tooltip content={CumTooltip} />
              <Area dataKey="cumDebt" type="monotone"
                stroke={ALARM} strokeWidth={2} fill="url(#cumGrad)"
                dot={false} activeDot={false} isAnimationActive={false} />
            </ComposedChart>
          </ResponsiveContainer>

          <div style={{ display: "flex", gap: 12, justifyContent: "center", marginTop: 8 }}>
            <div style={{ display: "flex", alignItems: "center", gap: 5 }}>
              <div style={{ width: 24, height: 2, background: ALARM }} />
              <span style={{ fontSize: 10, color: MUTED }}>Cumulative debt (above zero = net over-commitment)</span>
            </div>
          </div>
        </div>

        {/* Footer */}
        <div style={{ fontSize: 11, color: MUTED, lineHeight: 1.7,
          borderTop: `1px solid ${BORDER}`, paddingTop: 14 }}>
          <div style={{ marginBottom: 6 }}>
            <b style={{ color: TEXT }}>Reading this chart: </b>
            Each bucket shows arrivals (commitments made) vs. departures (items delivered).
            The debt line shows the weekly gap — positive when more work enters than leaves.
            The cumulative panel shows systemic drift: a rising line means the system is
            consistently taking on more than it delivers, which mathematically guarantees
            future cycle time inflation via Little's Law.
          </div>
          <div>
            <b style={{ color: TEXT }}>Important: </b>
            Flow debt is a leading indicator — it signals accumulating pressure before delivery
            delays become visible. A total debt of {TOTAL_DEBT > 0 ? "+" : ""}{TOTAL_DEBT} items
            means the system has committed to {Math.abs(TOTAL_DEBT)} {TOTAL_DEBT >= 0 ? "more" : "fewer"} items
            than it has delivered over this window. Negative weekly debt is healthy (catch-up),
            but sustained positive debt requires intervention.
          </div>
        </div>

      </div>
    </div>
  );
}
