import { useState, useCallback } from "react";
import {
  ScatterChart, Scatter, ReferenceArea, ReferenceLine,
  XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer,
  Customized,
} from "recharts";

// ── INJECTED DATA ─────────────────────────────────────────────────────────────
// These two constants are replaced via find-and-replace before delivery.
// Do NOT edit manually.

const MCP_RESPONSE = "__MCP_RESPONSE__";
const CHART_ATTRS  = "__CHART_ATTRS__";

// ── CONFIG ────────────────────────────────────────────────────────────────────

const ALARM     = "#ff6b6b";
const CAUTION   = "#e2c97e";
const PRIMARY   = "#6b7de8";
const SECONDARY = "#7edde2";
const TEXT      = "#dde1ef";
const MUTED     = "#505878";
const PAGE_BG   = "#080a0f";
const PANEL_BG  = "#0c0e16";
const BORDER    = "#1a1d2e";

// ── DERIVED ───────────────────────────────────────────────────────────────────

const evo = MCP_RESPONSE.data.evolution;
const ctx = MCP_RESPONSE.data.context;

const BOARD_ID    = CHART_ATTRS.board_id;
const PROJECT_KEY = CHART_ATTRS.project_key;
const BOARD_NAME  = CHART_ATTRS.board_name;

const MEAN = evo.average_chart.average;
const UNPL = evo.average_chart.upper_natural_process_limit;

const SUBGROUPS = evo.subgroups.map(sg => ({
  label: sg.label,
  average: sg.average,
  values: sg.values,
}));

const shiftSignal = (evo.average_chart.signals || []).find(s => s.type === "shift");
const HAS_SHIFT   = !!shiftSignal;
const SHIFT_START  = HAS_SHIFT ? shiftSignal.index - 7 : -1;
const SHIFT_END    = HAS_SHIFT ? shiftSignal.index : -1;

const totalItems = ctx?.total_issues ?? SUBGROUPS.reduce((s, sg) => s + sg.values.length, 0);

function shortLabel(l) {
  if (!l) return "";
  const parts = l.split(" ");
  if (parts.length === 2) return `${parts[0]} ${parts[1].slice(2)}`;
  return l;
}

// Build scatter dot data — one entry per individual item
const dots = [];
SUBGROUPS.forEach((sg, idx) => {
  const vals = sg.values;
  const isShift = HAS_SHIFT && idx >= SHIFT_START && idx <= SHIFT_END;
  vals.forEach((v, i) => {
    const jitter = vals.length === 1 ? 0
      : -0.32 + (i / (vals.length - 1)) * 0.64;
    dots.push({
      x: idx + jitter,
      y: Math.max(v, 0.01),
      label: sg.label,
      isShift,
      isAboveUnpl: v > UNPL,
    });
  });
});

// Avg bar metadata for the custom layer
const avgBarData = SUBGROUPS.map((sg, idx) => ({
  idx,
  avg: sg.average,
  isShift: HAS_SHIFT && idx >= SHIFT_START && idx <= SHIFT_END,
  isAboveUnpl: sg.average > UNPL,
  hasValues: sg.values.length > 0,
})).filter(d => d.hasValues);

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

const DotTooltip = ({ active, payload }) => {
  if (!active || !payload?.length) return null;
  const d = payload[0].payload;
  return (
    <div style={{ background: "#0f1117", border: `1px solid ${BORDER}`, borderRadius: 8,
      padding: "8px 12px", fontFamily: "'Courier New', monospace", fontSize: 12, color: TEXT }}>
      <div style={{ fontWeight: 700, marginBottom: 4 }}>{d.label}</div>
      <div>Cycle Time: <b>{d.y.toFixed(1)}d</b></div>
    </div>
  );
};

// Custom layer that draws average bars using axis scales from Recharts internals
const AvgBarLayer = ({ xAxisMap, yAxisMap }) => {
  const xAxis = xAxisMap && Object.values(xAxisMap)[0];
  const yAxis = yAxisMap && Object.values(yAxisMap)[0];
  if (!xAxis?.scale || !yAxis?.scale) return null;

  return (
    <g>
      {avgBarData.map(d => {
        const xLeft  = xAxis.scale(d.idx - 0.38);
        const xRight = xAxis.scale(d.idx + 0.38);
        const cy     = yAxis.scale(d.avg);
        if (isNaN(xLeft) || isNaN(xRight) || isNaN(cy)) return null;
        const color = d.isAboveUnpl ? ALARM : d.isShift ? CAUTION : TEXT;
        return (
          <line key={`avg-${d.idx}`}
            x1={xLeft} y1={cy} x2={xRight} y2={cy}
            stroke={color} strokeWidth={2.5} strokeLinecap="round" opacity={0.9} />
        );
      })}
    </g>
  );
};

// ── MAIN EXPORT ───────────────────────────────────────────────────────────────

export default function ProcessEvolutionChart() {
  const [showLog, setShowLog] = useState(false);

  const DotShape = useCallback(({ cx, cy, payload }) => {
    if (!cx || !cy || !payload) return null;
    const color   = payload.isAboveUnpl ? ALARM : payload.isShift ? CAUTION : PRIMARY;
    const opacity = payload.isAboveUnpl ? 0.75 : 0.35;
    const r       = payload.isAboveUnpl ? 3.5 : 2.5;
    return <circle cx={cx} cy={cy} r={r} fill={color} fillOpacity={opacity} />;
  }, []);

  const linearYMax = Math.ceil(Math.max(UNPL * 1.15, ...dots.map(d => d.y), 300) / 50) * 50;
  const dateRange = SUBGROUPS.length > 0
    ? `${SUBGROUPS[0].label} – ${SUBGROUPS[SUBGROUPS.length - 1].label}` : "";

  const shiftLabel = HAS_SHIFT
    ? `${shortLabel(SUBGROUPS[SHIFT_START]?.label || "")} – ${shortLabel(SUBGROUPS[SHIFT_END]?.label || "")}`
    : "";

  return (
    <div style={{ background: PAGE_BG, minHeight: "100vh", padding: "24px 20px",
      fontFamily: "'Courier New', monospace", color: TEXT }}>
      <div style={{ maxWidth: 1100, margin: "0 auto" }}>

        {/* Header */}
        <div style={{ fontSize: 11, color: MUTED, letterSpacing: "0.08em",
          textTransform: "uppercase", marginBottom: 6 }}>
          {PROJECT_KEY} · {BOARD_NAME} · Board {BOARD_ID}
        </div>
        <h1 style={{ fontSize: 22, fontWeight: 700, margin: "0 0 4px" }}>Strategic Process Evolution</h1>
        <div style={{ fontSize: 12, color: MUTED, marginBottom: 16 }}>
          Cycle Time Distribution by Monthly Subgroup · {dateRange}
        </div>

        {/* Stat cards */}
        <div style={{ display: "flex", flexWrap: "wrap", gap: 10, marginBottom: 14 }}>
          <StatCard label="PROCESS AVG (X̄)" value={`${MEAN.toFixed(1)}d`} color={PRIMARY} />
          <StatCard label="UNPL"             value={`${UNPL.toFixed(1)}d`} color={ALARM} />
          <StatCard label="ITEMS"            value={totalItems}            color={SECONDARY} />
          <StatCard label="MONTHS"           value={SUBGROUPS.length}      color={MUTED} />
        </div>

        {/* Badges + scale toggle */}
        <div style={{ display: "flex", flexWrap: "wrap", gap: 6, marginBottom: 20, alignItems: "center" }}>
          {HAS_SHIFT && (
            <Badge text={`⚠ Process Shift: 8-point run (${shiftLabel})`} color={CAUTION} />
          )}
          <Badge text={`${ctx?.window_months ?? SUBGROUPS.length}-month window · ${SUBGROUPS.length} subgroups · ${totalItems} items`} color={PRIMARY} />
          <div style={{ flex: 1 }} />
          <button onClick={() => setShowLog(!showLog)} style={{
            fontSize: 10, padding: "5px 14px", borderRadius: 6, cursor: "pointer",
            background: showLog ? `${SECONDARY}18` : BORDER,
            border: `1.5px solid ${showLog ? SECONDARY : "#404660"}`,
            color: showLog ? SECONDARY : TEXT,
            fontFamily: "'Courier New', monospace", fontWeight: 700,
          }}>
            {showLog ? "▾ Log Scale" : "▾ Linear Scale"}
          </button>
        </div>

        {/* Chart panel */}
        <div style={{ background: PANEL_BG, borderRadius: 12,
          border: `1px solid ${BORDER}`, padding: "16px 8px 12px" }}>
          <ResponsiveContainer width="100%" height={420}>
            <ScatterChart margin={{ top: 10, right: 20, bottom: 60, left: 10 }}>
              <CartesianGrid strokeDasharray="3 3" stroke={BORDER} vertical={false} />
              <XAxis type="number"
                dataKey="x"
                name="x"
                domain={[-0.6, SUBGROUPS.length - 0.4]}
                ticks={SUBGROUPS.map((_, i) => i)}
                tickFormatter={i => shortLabel(SUBGROUPS[i]?.label || "")}
                angle={-45} textAnchor="end" height={60}
                tick={{ fill: MUTED, fontSize: 10, fontFamily: "'Courier New', monospace" }} />
              <YAxis type="number"
                dataKey="y"
                name="y"
                scale={showLog ? "log" : "linear"}
                domain={showLog ? [0.5, 1000] : [0, linearYMax]}
                allowDataOverflow
                tickFormatter={v => `${v}d`}
                tick={{ fill: MUTED, fontSize: 10, fontFamily: "'Courier New', monospace" }}
                label={{ value: "Cycle Time (days)", angle: -90, position: "insideLeft",
                  fill: MUTED, fontSize: 10, fontFamily: "'Courier New', monospace" }} />
              <Tooltip content={<DotTooltip />} cursor={false} />

              {HAS_SHIFT && (
                <ReferenceArea
                  x1={SHIFT_START - 0.5} x2={SHIFT_END + 0.5}
                  fill={CAUTION} fillOpacity={0.04}
                  stroke={CAUTION} strokeDasharray="4 4" strokeOpacity={0.3} />
              )}

              <ReferenceLine y={UNPL} stroke={ALARM} strokeDasharray="6 3" strokeWidth={1.5}
                label={{ value: `UNPL ${UNPL.toFixed(1)}d`, fill: ALARM,
                  fontSize: 10, position: "right", fontFamily: "'Courier New', monospace" }} />
              <ReferenceLine y={MEAN} stroke={PRIMARY} strokeDasharray="4 4" strokeWidth={1.5}
                label={{ value: `X̄ ${MEAN.toFixed(1)}d`, fill: PRIMARY,
                  fontSize: 10, position: "right", fontFamily: "'Courier New', monospace" }} />

              <Scatter data={dots} shape={DotShape} isAnimationActive={false} />

              {/* Average bars as custom SVG layer */}
              <Customized component={AvgBarLayer} />
            </ScatterChart>
          </ResponsiveContainer>

          {/* Legend */}
          <div style={{ display: "flex", flexWrap: "wrap", gap: 12, justifyContent: "center", marginTop: 8 }}>
            {[
              { color: PRIMARY, opacity: 0.7, label: "Normal" },
              { color: CAUTION, opacity: 0.7, label: "Shift zone" },
              { color: ALARM,   opacity: 0.9, label: "Above UNPL" },
            ].map(({ color, opacity, label }) => (
              <div key={label} style={{ display: "flex", alignItems: "center", gap: 5 }}>
                <svg width={14} height={12}><circle cx={6} cy={6} r={3} fill={color} fillOpacity={opacity} /></svg>
                <span style={{ fontSize: 11, color: MUTED }}>{label}</span>
              </div>
            ))}
            <div style={{ width: 1, height: 14, background: BORDER }} />
            {[
              { color: TEXT,    label: "Avg (normal)" },
              { color: CAUTION, label: "Avg (shift)" },
              { color: ALARM,   label: "Avg (above UNPL)" },
            ].map(({ color, label }) => (
              <div key={label} style={{ display: "flex", alignItems: "center", gap: 5 }}>
                <svg width={24} height={12}>
                  <line x1={0} y1={6} x2={24} y2={6} stroke={color} strokeWidth={2.5} strokeLinecap="round" />
                </svg>
                <span style={{ fontSize: 11, color: MUTED }}>{label}</span>
              </div>
            ))}
            <div style={{ width: 1, height: 14, background: BORDER }} />
            {[
              { stroke: PRIMARY, dash: "4 4", label: "X̄ avg" },
              { stroke: ALARM,   dash: "6 3", label: "UNPL" },
            ].map(({ stroke, dash, label }) => (
              <div key={label} style={{ display: "flex", alignItems: "center", gap: 5 }}>
                <svg width={24} height={12}>
                  <line x1={0} y1={6} x2={24} y2={6} stroke={stroke} strokeDasharray={dash} strokeWidth={1.5} />
                </svg>
                <span style={{ fontSize: 11, color: MUTED }}>{label}</span>
              </div>
            ))}
          </div>
        </div>

        {/* Footer */}
        <div style={{ fontSize: 11, color: MUTED, lineHeight: 1.7,
          borderTop: `1px solid ${BORDER}`, paddingTop: 14, marginTop: 16 }}>
          <div style={{ marginBottom: 6 }}>
            <b style={{ color: TEXT }}>Reading this chart: </b>
            Each dot represents one delivered item's cycle time, grouped by the month it was
            completed. Horizontal bars show the monthly subgroup average (disconnected by design —
            each bar represents one month independently). XmR control limits (X̄ and UNPL) are
            applied to the subgroup averages. Amber shading marks a shift zone (8 consecutive
            months on one side of X̄). Red dots = items above UNPL. Use the scale toggle for
            linear vs log view.
          </div>
          <div>
            <b style={{ color: TEXT }}>Note on early subgroups: </b>
            Months with only 1–2 items produce a technically correct average bar but one
            that is statistically meaningless. Focus interpretation on months with 10+ items.
          </div>
        </div>

      </div>
    </div>
  );
}
