import {
  ComposedChart, Bar, Cell, ReferenceLine, XAxis, YAxis,
  CartesianGrid, Tooltip, ResponsiveContainer,
} from "recharts";

// ── INJECTED DATA ─────────────────────────────────────────────────────────────
// Payload is injected by the MCS chart renderer as window.__MCS_PAYLOAD__.

const __MCS_ENVELOPE__ = window.__MCS_PAYLOAD__;
const __MCS_DATA__ = __MCS_ENVELOPE__.data;
const __MCS_GUARDRAILS__ = __MCS_ENVELOPE__.guardrails;
const __MCS_WORKFLOW__ = __MCS_ENVELOPE__.workflow;
// ── CONFIG ────────────────────────────────────────────────────────────────────

const CFG = {
  mainChartHeight: 400,
  mrChartHeight:   180,
  mainMargin:      { top: 10, right: 20, bottom: 60, left: 10 },
  mrMargin:        { top: 10, right: 20, bottom: 60, left: 10 },
  ySnapStep:       5,
  xTickCount:      10,
  partialOpacity:  0.4,
  issueTypeColors: {
    Story:    "#6b7de8",
    Bug:      "#ff6b6b",
    Activity: "#7edde2",
    Defect:   "#e2c97e",
  },
  fallbackPalette: ["#6bffb8", "#d946ef", "#f97316"],
  COLOR_ALARM:    "#ff6b6b",
  COLOR_CAUTION:  "#e2c97e",
  COLOR_PRIMARY:  "#6b7de8",
  COLOR_POSITIVE: "#6bffb8",
  COLOR_TEXT:     "#dde1ef",
  COLOR_MUTED:    "#505878",
  COLOR_PAGE_BG:  "#080a0f",
  COLOR_PANEL_BG: "#0c0e16",
  COLOR_BORDER:   "#1a1d2e",
  COLOR_MR_BAR:   "#505878",
};

// ── DERIVED ───────────────────────────────────────────────────────────────────

const d          = __MCS_DATA__;
const METADATA   = d["@metadata"];
const TOTAL      = d.total_throughput;
const STRAT      = d.stratified_throughput;
const STAB       = d.stability;

const BOARD_ID    = __MCS_WORKFLOW__.board_id;
const PROJECT_KEY = __MCS_WORKFLOW__.project_key;
const BOARD_NAME  = __MCS_WORKFLOW__.board_name;

const MEAN    = STAB.average;
const UNPL    = STAB.upper_natural_process_limit;
const LNPL    = STAB.lower_natural_process_limit;
const MR_MEAN = STAB.average_moving_range;
const MR_UNPL = 3.267 * MR_MEAN;

const SIGNAL_INDICES = new Set((STAB.signals || []).map(s => s.index));

const DETECTED_TYPES = Object.keys(STRAT);
const ACTIVE_TYPES   = DETECTED_TYPES.filter(t => STRAT[t].some(v => v > 0));

const TYPE_COLORS = Object.fromEntries(
  DETECTED_TYPES.map((t, i) => [
    t,
    CFG.issueTypeColors[t] ?? CFG.fallbackPalette[i % CFG.fallbackPalette.length],
  ])
);

const BUCKETS = METADATA.map((meta, i) => ({
  label:     meta.label,
  startDate: meta.start_date,
  endDate:   meta.end_date,
  isPartial: meta.is_partial === "true",
  total:     TOTAL[i] ?? 0,
  ...Object.fromEntries(DETECTED_TYPES.map(t => [t, (STRAT[t] ?? [])[i] ?? 0])),
  mr:        i > 0 ? (STAB.moving_ranges[i - 1] ?? null) : null,
  isSignal:  SIGNAL_INDICES.has(i),
  isAbove:   (TOTAL[i] ?? 0) > UNPL,
  isBelow:   (TOTAL[i] ?? 0) < LNPL && LNPL > 0,
}));

const DATE_RANGE   = METADATA.length > 0
  ? `${METADATA[0].label} – ${METADATA[METADATA.length - 1].label}` : "";
const BUCKET_LABEL = METADATA.length > 0 && METADATA[0].label.includes("W") ? "week" : "month";

const yDomain = (max) => Math.ceil((max * 1.2) / CFG.ySnapStep) * CFG.ySnapStep;

// ── SUB-COMPONENTS ────────────────────────────────────────────────────────────

const StatCard = ({ label, value, color }) => (
  <div style={{ background: CFG.COLOR_PANEL_BG, border: `1px solid ${color}33`,
    borderRadius: 8, padding: "10px 16px", minWidth: 120 }}>
    <div style={{ fontSize: 10, color: CFG.COLOR_MUTED, marginBottom: 4, letterSpacing: "0.05em" }}>{label}</div>
    <div style={{ fontSize: 20, fontWeight: 700, color }}>{value}</div>
  </div>
);

const Badge = ({ text, color }) => (
  <span style={{ fontSize: 11, padding: "4px 10px", borderRadius: 4,
    background: `${color}15`, border: `1px solid ${color}40`, color }}>
    {text}
  </span>
);

const CustomTooltip = ({ active, payload }) => {
  if (!active || !payload?.length) return null;
  const b = payload[0].payload;
  return (
    <div style={{ background: "#0f1117", border: `1px solid ${CFG.COLOR_BORDER}`,
      borderRadius: 8, padding: "10px 14px", fontFamily: "'Courier New', monospace",
      fontSize: 12, color: CFG.COLOR_TEXT, minWidth: 200 }}>
      <div style={{ fontWeight: 700, marginBottom: 2 }}>{b.label}</div>
      <div style={{ color: CFG.COLOR_MUTED, marginBottom: 6, fontSize: 11 }}>{b.startDate} – {b.endDate}</div>
      <div style={{ borderTop: `1px solid ${CFG.COLOR_BORDER}`, paddingTop: 6 }}>
        {ACTIVE_TYPES.map(t => b[t] > 0 && (
          <div key={t} style={{ display: "flex", justifyContent: "space-between", gap: 16 }}>
            <span style={{ color: TYPE_COLORS[t] }}>{t}</span>
            <span>{b[t]} items</span>
          </div>
        ))}
        <div style={{ display: "flex", justifyContent: "space-between", gap: 16,
          fontWeight: 700, borderTop: `1px solid ${CFG.COLOR_BORDER}`, marginTop: 4, paddingTop: 4 }}>
          <span>Total</span><span>{b.total} items</span>
        </div>
        <div style={{ marginTop: 6, color: CFG.COLOR_MUTED, fontSize: 11 }}>
          <div>X̄: {MEAN.toFixed(1)}</div>
          <div>UNPL: {UNPL.toFixed(1)}</div>
          <div>mR: {b.mr != null ? b.mr.toFixed(1) : "—"}</div>
        </div>
        {b.isPartial && <div style={{ color: CFG.COLOR_CAUTION, marginTop: 4, fontWeight: 700 }}>⚠ Partial {BUCKET_LABEL}</div>}
        {b.isSignal  && <div style={{ color: CFG.COLOR_ALARM,   marginTop: 4, fontWeight: 700 }}>⚠ Signal detected</div>}
      </div>
    </div>
  );
};

// ── MAIN EXPORT ───────────────────────────────────────────────────────────────

export default function ThroughputChart() {
  const hasSignals  = STAB.signals && STAB.signals.length > 0;
  const lastPartial = BUCKETS.length > 0 && BUCKETS[BUCKETS.length - 1].isPartial;
  const interval    = Math.max(1, Math.floor(BUCKETS.length / CFG.xTickCount));
  const mainYMax    = yDomain(Math.max(...TOTAL, UNPL));
  const mrYMax      = yDomain(Math.max(...(STAB.moving_ranges || [0]), MR_UNPL));

  return (
    <div style={{ background: CFG.COLOR_PAGE_BG, minHeight: "100vh", padding: "32px 24px",
      fontFamily: "'Courier New', monospace", color: CFG.COLOR_TEXT }}>
      <div style={{ maxWidth: 1100, margin: "0 auto" }}>

        {/* Header */}
        <div style={{ fontSize: 11, color: CFG.COLOR_MUTED, letterSpacing: "0.08em",
          textTransform: "uppercase", marginBottom: 8 }}>
          {PROJECT_KEY} · {BOARD_NAME} · Board {BOARD_ID}
        </div>
        <h1 style={{ fontSize: 26, fontWeight: 700, margin: "0 0 4px 0" }}>Throughput Stability</h1>
        <div style={{ fontSize: 13, color: CFG.COLOR_MUTED, marginBottom: 20 }}>
          {BUCKET_LABEL === "week" ? "Weekly" : "Monthly"} Delivery Volume · XmR Process Behavior Chart · {DATE_RANGE}
        </div>

        {/* Stat cards */}
        <div style={{ display: "flex", flexWrap: "wrap", gap: 12, marginBottom: 16 }}>
          <StatCard label="X̄ MEAN"  value={`${MEAN.toFixed(1)} / ${BUCKET_LABEL}`} color={CFG.COLOR_PRIMARY} />
          <StatCard label="UNPL"    value={`${UNPL.toFixed(1)} / ${BUCKET_LABEL}`} color={CFG.COLOR_ALARM} />
          <StatCard label="LNPL"    value={LNPL > 0 ? `${LNPL.toFixed(1)} / ${BUCKET_LABEL}` : "—"} color={CFG.COLOR_POSITIVE} />
          <StatCard label="WINDOW"  value={`${BUCKETS.length} ${BUCKET_LABEL}s`} color={CFG.COLOR_MUTED} />
        </div>

        {/* Badges */}
        <div style={{ display: "flex", flexWrap: "wrap", gap: 8, marginBottom: 24, alignItems: "center" }}>
          <Badge text={hasSignals ? "⚠ SIGNALS DETECTED" : "✓ STABLE"}
            color={hasSignals ? CFG.COLOR_ALARM : CFG.COLOR_POSITIVE} />
          {lastPartial && (
            <Badge text={`⚠ ${BUCKETS[BUCKETS.length - 1].label} — partial ${BUCKET_LABEL}`}
              color={CFG.COLOR_CAUTION} />
          )}
        </div>

        {/* Main Chart */}
        <div style={{ background: CFG.COLOR_PANEL_BG, borderRadius: 12,
          border: `1px solid ${CFG.COLOR_BORDER}`, padding: "20px 12px 12px 12px", marginBottom: 4 }}>
          <ResponsiveContainer width="100%" height={CFG.mainChartHeight}>
            <ComposedChart data={BUCKETS} margin={CFG.mainMargin}>
              <CartesianGrid strokeDasharray="3 3" stroke={CFG.COLOR_BORDER} vertical={false} />
              <XAxis dataKey="label" angle={-45} textAnchor="end" height={60} interval={interval}
                tick={{ fill: CFG.COLOR_MUTED, fontSize: 11, fontFamily: "'Courier New', monospace" }} />
              <YAxis domain={[0, mainYMax]}
                tick={{ fill: CFG.COLOR_MUTED, fontSize: 11, fontFamily: "'Courier New', monospace" }}
                label={{ value: `Items / ${BUCKET_LABEL}`, angle: -90, position: "insideLeft",
                  fill: CFG.COLOR_MUTED, fontSize: 11, fontFamily: "'Courier New', monospace" }} />
              <Tooltip content={<CustomTooltip />} />
              {ACTIVE_TYPES.map(t => (
                <Bar key={t} dataKey={t} stackId="tp" fill={TYPE_COLORS[t]} isAnimationActive={false}>
                  {BUCKETS.map((b, i) => (
                    <Cell key={i}
                      fillOpacity={b.isPartial ? CFG.partialOpacity : 1}
                      stroke={b.isSignal && t === ACTIVE_TYPES[ACTIVE_TYPES.length - 1] ? CFG.COLOR_ALARM : "none"}
                      strokeWidth={b.isSignal && t === ACTIVE_TYPES[ACTIVE_TYPES.length - 1] ? 2 : 0}
                    />
                  ))}
                </Bar>
              ))}
              <ReferenceLine y={UNPL} stroke={CFG.COLOR_ALARM} strokeDasharray="6 3" strokeWidth={1.5}
                label={{ value: "UNPL", fill: CFG.COLOR_ALARM, fontSize: 10, position: "insideTopLeft",
                  fontFamily: "'Courier New', monospace" }} />
              <ReferenceLine y={MEAN} stroke={CFG.COLOR_PRIMARY} strokeDasharray="4 4" strokeWidth={1.5}
                label={{ value: "X̄", fill: CFG.COLOR_PRIMARY, fontSize: 10, position: "insideTopLeft",
                  fontFamily: "'Courier New', monospace" }} />
              {LNPL > 0 && (
                <ReferenceLine y={LNPL} stroke={CFG.COLOR_POSITIVE} strokeDasharray="6 3" strokeWidth={1}
                  label={{ value: "LNPL", fill: CFG.COLOR_POSITIVE, fontSize: 10, position: "insideBottomLeft",
                    fontFamily: "'Courier New', monospace" }} />
              )}
            </ComposedChart>
          </ResponsiveContainer>

          {/* Legend */}
          <div style={{ display: "flex", flexWrap: "wrap", gap: 16, justifyContent: "center", marginTop: 8 }}>
            {ACTIVE_TYPES.map(t => (
              <div key={t} style={{ display: "flex", alignItems: "center", gap: 6 }}>
                <svg width={14} height={12}><rect x={0} y={0} width={14} height={12} fill={TYPE_COLORS[t]} /></svg>
                <span style={{ fontSize: 11, color: CFG.COLOR_MUTED }}>{t}</span>
              </div>
            ))}
            {[
              { stroke: CFG.COLOR_ALARM,   dash: "6 3", label: "UNPL" },
              { stroke: CFG.COLOR_PRIMARY, dash: "4 4", label: "X̄ Mean" },
              ...(LNPL > 0 ? [{ stroke: CFG.COLOR_POSITIVE, dash: "6 3", label: "LNPL" }] : []),
            ].map(({ stroke, dash, label }) => (
              <div key={label} style={{ display: "flex", alignItems: "center", gap: 6 }}>
                <svg width={24} height={12}>
                  <line x1={0} y1={6} x2={24} y2={6} stroke={stroke} strokeDasharray={dash} strokeWidth={1.5} />
                </svg>
                <span style={{ fontSize: 11, color: CFG.COLOR_MUTED }}>{label}</span>
              </div>
            ))}
          </div>
        </div>

        {/* Moving Range Chart */}
        <div style={{ background: CFG.COLOR_PANEL_BG, borderRadius: 12,
          border: `1px solid ${CFG.COLOR_BORDER}`, padding: "12px", marginBottom: 24 }}>
          <div style={{ fontSize: 11, color: CFG.COLOR_MUTED, marginBottom: 8,
            letterSpacing: "0.05em", textTransform: "uppercase" }}>
            Moving Range ({BUCKET_LABEL}-to-{BUCKET_LABEL} variation)
          </div>
          <ResponsiveContainer width="100%" height={CFG.mrChartHeight}>
            <ComposedChart data={BUCKETS} margin={CFG.mrMargin}>
              <CartesianGrid strokeDasharray="3 3" stroke={CFG.COLOR_BORDER} vertical={false} />
              <XAxis dataKey="label" angle={-45} textAnchor="end" height={60} interval={interval}
                tick={{ fill: CFG.COLOR_MUTED, fontSize: 11, fontFamily: "'Courier New', monospace" }} />
              <YAxis domain={[0, mrYMax]}
                tick={{ fill: CFG.COLOR_MUTED, fontSize: 11, fontFamily: "'Courier New', monospace" }}
                label={{ value: "Moving Range", angle: -90, position: "insideLeft",
                  fill: CFG.COLOR_MUTED, fontSize: 11, fontFamily: "'Courier New', monospace" }} />
              <Tooltip content={<CustomTooltip />} />
              <Bar dataKey="mr" isAnimationActive={false}>
                {BUCKETS.map((b, i) => (
                  <Cell key={i}
                    fill={b.mr > MR_UNPL ? CFG.COLOR_ALARM : CFG.COLOR_MR_BAR}
                    fillOpacity={b.mr == null ? 0 : 1}
                  />
                ))}
              </Bar>
              <ReferenceLine y={MR_UNPL} stroke={CFG.COLOR_ALARM} strokeDasharray="6 3" strokeWidth={1.5}
                label={{ value: "URL", fill: CFG.COLOR_ALARM, fontSize: 10, position: "insideTopLeft",
                  fontFamily: "'Courier New', monospace" }} />
              <ReferenceLine y={MR_MEAN} stroke={CFG.COLOR_PRIMARY} strokeDasharray="4 4" strokeWidth={1.5}
                label={{ value: "MR̄", fill: CFG.COLOR_PRIMARY, fontSize: 10, position: "insideTopLeft",
                  fontFamily: "'Courier New', monospace" }} />
            </ComposedChart>
          </ResponsiveContainer>
        </div>

        {/* Footer */}
        <div style={{ fontSize: 11, color: CFG.COLOR_MUTED, lineHeight: 1.7,
          borderTop: `1px solid ${CFG.COLOR_BORDER}`, paddingTop: 16 }}>
          <div style={{ marginBottom: 8 }}>
            <b style={{ color: CFG.COLOR_TEXT }}>Reading this chart:</b> Bars show delivery volume
            per {BUCKET_LABEL}, stacked by issue type. The dashed X̄ is the process mean. UNPL and
            LNPL define the expected range of natural variation — bars outside these limits signal a
            special cause worth investigating. The Moving Range panel shows {BUCKET_LABEL}-to-{BUCKET_LABEL}{" "}
            variation; spikes above the Upper Range Limit (URL) indicate unusually large step-changes
            in delivery rate.
          </div>
          <div>
            <b style={{ color: CFG.COLOR_TEXT }}>Data provenance:</b> Wheeler XmR Process Behavior
            Chart. Limits: X̄ ± 2.66 × MR̄. URL = 3.267 × MR̄. Partial {BUCKET_LABEL}s shown at
            reduced opacity, excluded from limit calculations.
          </div>
        </div>

      </div>
    </div>
  );
}
