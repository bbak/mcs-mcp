import { useState, useCallback } from "react";
import {
  ScatterChart, Scatter, ReferenceArea, ReferenceLine,
  XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer,
  Customized,
} from "recharts";
import { ALARM, CAUTION, PRIMARY, SECONDARY, TEXT, MUTED, PAGE_BG, PANEL_BG, BORDER, XMR_UNPL, XMR_MEAN, FONT_STACK } from "mcs-mcp";
import { StatCard, Badge, TOOLTIP_BG } from "./shared.jsx";

// ── INJECTED DATA ─────────────────────────────────────────────────────────────
// Payload is injected by the MCS chart renderer as window.__MCS_PAYLOAD__.

const __MCS_ENVELOPE__ = window.__MCS_PAYLOAD__;
const __MCS_DATA__ = __MCS_ENVELOPE__.data;
const __MCS_GUARDRAILS__ = __MCS_ENVELOPE__.guardrails;
const __MCS_WORKFLOW__ = __MCS_ENVELOPE__.workflow;
// ── CONFIG ────────────────────────────────────────────────────────────────────


// ── DERIVED ───────────────────────────────────────────────────────────────────

const evo = __MCS_DATA__.evolution;
const ctx = __MCS_DATA__.context;

const BOARD_ID    = __MCS_WORKFLOW__.board_id;
const PROJECT_KEY = __MCS_WORKFLOW__.project_key;
const BOARD_NAME  = __MCS_WORKFLOW__.board_name;

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

const DotTooltip = ({ active, payload }) => {
  if (!active || !payload?.length) return null;
  const d = payload[0].payload;
  return (
    <div style={{ background: TOOLTIP_BG, border: `1px solid ${BORDER}`, borderRadius: 8,
      padding: "8px 12px", fontFamily: FONT_STACK, fontSize: 12, color: TEXT }}>
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
      fontFamily: FONT_STACK, color: TEXT }}>
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
          <StatCard label="PROCESS AVG (X̄)" value={`${MEAN.toFixed(1)}d`} color={XMR_MEAN} />
          <StatCard label="UNPL"             value={`${UNPL.toFixed(1)}d`} color={XMR_UNPL} />
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
            fontFamily: FONT_STACK, fontWeight: 700,
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
                tick={{ fill: MUTED, fontSize: 10, fontFamily: FONT_STACK }} />
              <YAxis type="number"
                dataKey="y"
                name="y"
                scale={showLog ? "log" : "linear"}
                domain={showLog ? [0.5, 1000] : [0, linearYMax]}
                allowDataOverflow
                tickFormatter={v => `${v}d`}
                tick={{ fill: MUTED, fontSize: 10, fontFamily: FONT_STACK }}
                label={{ value: "Cycle Time (days)", angle: -90, position: "insideLeft",
                  fill: MUTED, fontSize: 10, fontFamily: FONT_STACK }} />
              <Tooltip content={DotTooltip} cursor={false} />

              {HAS_SHIFT && (
                <ReferenceArea
                  x1={SHIFT_START - 0.5} x2={SHIFT_END + 0.5}
                  fill={CAUTION} fillOpacity={0.04}
                  stroke={CAUTION} strokeDasharray="4 4" strokeOpacity={0.3} />
              )}

              <ReferenceLine y={UNPL} stroke={XMR_UNPL} strokeDasharray="6 3" strokeWidth={1.5}
                label={{ value: `UNPL ${UNPL.toFixed(1)}d`, fill: XMR_UNPL,
                  fontSize: 10, position: "right", fontFamily: FONT_STACK }} />
              <ReferenceLine y={MEAN} stroke={XMR_MEAN} strokeDasharray="4 4" strokeWidth={1.5}
                label={{ value: `X̄ ${MEAN.toFixed(1)}d`, fill: XMR_MEAN,
                  fontSize: 10, position: "right", fontFamily: FONT_STACK }} />

              <Scatter data={dots} shape={DotShape} isAnimationActive={false} />

              {/* Average bars as custom SVG layer */}
              <Customized component={AvgBarLayer} />
            </ScatterChart>
          </ResponsiveContainer>

          {/* Legend */}
          <div style={{ display: "flex", flexWrap: "wrap", gap: 12, justifyContent: "center", marginTop: 8 }}>
            <div style={{ display: "flex", alignItems: "center", gap: 5 }}>
              <div style={{ width: 8, height: 8, borderRadius: "50%", background: PRIMARY, opacity: 0.7 }} />
              <span style={{ fontSize: 11, color: MUTED }}>Normal</span>
            </div>
            <div style={{ display: "flex", alignItems: "center", gap: 5 }}>
              <div style={{ width: 8, height: 8, borderRadius: "50%", background: CAUTION, opacity: 0.7 }} />
              <span style={{ fontSize: 11, color: MUTED }}>Shift zone</span>
            </div>
            <div style={{ display: "flex", alignItems: "center", gap: 5 }}>
              <div style={{ width: 8, height: 8, borderRadius: "50%", background: ALARM, opacity: 0.9 }} />
              <span style={{ fontSize: 11, color: MUTED }}>Above UNPL</span>
            </div>
            <div style={{ width: 1, height: 14, background: BORDER }} />
            <div style={{ display: "flex", alignItems: "center", gap: 5 }}>
              <div style={{ width: 24, height: 2.5, background: TEXT, borderRadius: 1 }} />
              <span style={{ fontSize: 11, color: MUTED }}>Avg (normal)</span>
            </div>
            <div style={{ display: "flex", alignItems: "center", gap: 5 }}>
              <div style={{ width: 24, height: 2.5, background: CAUTION, borderRadius: 1 }} />
              <span style={{ fontSize: 11, color: MUTED }}>Avg (shift)</span>
            </div>
            <div style={{ display: "flex", alignItems: "center", gap: 5 }}>
              <div style={{ width: 24, height: 2.5, background: ALARM, borderRadius: 1 }} />
              <span style={{ fontSize: 11, color: MUTED }}>Avg (above UNPL)</span>
            </div>
            <div style={{ width: 1, height: 14, background: BORDER }} />
            <div style={{ display: "flex", alignItems: "center", gap: 5 }}>
              <div style={{ width: 24, height: 0, borderTop: `1.5px dashed ${XMR_MEAN}` }} />
              <span style={{ fontSize: 11, color: MUTED }}>X̄ avg</span>
            </div>
            <div style={{ display: "flex", alignItems: "center", gap: 5 }}>
              <div style={{ width: 24, height: 0, borderTop: `1.5px dashed ${XMR_UNPL}` }} />
              <span style={{ fontSize: 11, color: MUTED }}>UNPL</span>
            </div>
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
