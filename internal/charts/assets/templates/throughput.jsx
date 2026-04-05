import {
  ComposedChart, Bar, Cell, ReferenceLine, XAxis, YAxis,
  CartesianGrid, Tooltip, ResponsiveContainer,
} from "recharts";
import { ALARM, CAUTION, POSITIVE, TEXT, MUTED, PAGE_BG, PANEL_BG, BORDER, typeColor, XMR_UNPL, XMR_MEAN, XMR_LNPL, FONT_STACK } from "mcs-mcp";
import { StatCard, Badge, TOOLTIP_BG } from "./shared.jsx";

// ── INJECTED DATA ─────────────────────────────────────────────────────────────
// Payload is injected by the MCS chart renderer as window.__MCS_PAYLOAD__.

const __MCS_ENVELOPE__ = window.__MCS_PAYLOAD__;
const __MCS_DATA__ = __MCS_ENVELOPE__.data;
const __MCS_GUARDRAILS__ = __MCS_ENVELOPE__.guardrails;
const __MCS_WORKFLOW__ = __MCS_ENVELOPE__.workflow;
// ── CONFIG ────────────────────────────────────────────────────────────────────

const CFG = {
  mainChartHeight: 360,
  mrChartHeight:   160,
  mainMargin:      { top: 10, right: 20, bottom: 5, left: 10 },
  mrMargin:        { top: 10, right: 20, bottom: 5, left: 10 },
  ySnapStep:       5,
  xTickCount:      10,
  partialOpacity:  0.4,
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
  DETECTED_TYPES.map(t => [t, typeColor(t, DETECTED_TYPES)])
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


function TooltipTypeRow({ t, count }) {
  return (
    <div style={{ display: "flex", justifyContent: "space-between", gap: 16 }}>
      <span style={{ color: TYPE_COLORS[t] }}>{t}</span>
      <span>{count} items</span>
    </div>
  );
}

function CustomTooltip({ active, payload }) {
  if (!active || !payload?.length) return null;
  const b = payload[0].payload;
  const typeRows = ACTIVE_TYPES.filter(t => b[t] > 0);
  return (
    <div style={{ background: TOOLTIP_BG, border: `1px solid ${BORDER}`,
      borderRadius: 8, padding: "10px 14px", fontFamily: FONT_STACK,
      fontSize: 12, color: TEXT, minWidth: 200 }}>
      <div style={{ fontWeight: 700, marginBottom: 2 }}>{b.label}</div>
      <div style={{ color: MUTED, marginBottom: 6, fontSize: 11 }}>{b.startDate} – {b.endDate}</div>
      <div style={{ borderTop: `1px solid ${BORDER}`, paddingTop: 6 }}>
        {typeRows.map(t => <TooltipTypeRow key={t} t={t} count={b[t]} />)}
        <div style={{ display: "flex", justifyContent: "space-between", gap: 16,
          fontWeight: 700, borderTop: `1px solid ${BORDER}`, marginTop: 4, paddingTop: 4 }}>
          <span>Total</span><span>{b.total} items</span>
        </div>
        <div style={{ marginTop: 6, color: MUTED, fontSize: 11 }}>
          <div>X̄: {MEAN.toFixed(1)}</div>
          <div>UNPL: {UNPL.toFixed(1)}</div>
          <div>mR: {b.mr != null ? b.mr.toFixed(1) : "—"}</div>
        </div>
        {b.isPartial && <div style={{ color: CAUTION, marginTop: 4, fontWeight: 700 }}>⚠ Partial {BUCKET_LABEL}</div>}
        {b.isSignal  && <div style={{ color: ALARM,   marginTop: 4, fontWeight: 700 }}>⚠ Signal detected</div>}
      </div>
    </div>
  );
}

// ── MAIN EXPORT ───────────────────────────────────────────────────────────────

export default function ThroughputChart() {
  const hasSignals  = STAB.signals && STAB.signals.length > 0;
  const lastPartial = BUCKETS.length > 0 && BUCKETS[BUCKETS.length - 1].isPartial;
  const interval    = Math.max(1, Math.floor(BUCKETS.length / CFG.xTickCount));
  const mainYMax    = yDomain(Math.max(...TOTAL, UNPL));
  const mrYMax      = yDomain(Math.max(...(STAB.moving_ranges || [0]), MR_UNPL));

  return (
    <div style={{ background: PAGE_BG, minHeight: "100vh", padding: "32px 24px",
      fontFamily: FONT_STACK, color: TEXT }}>
      <div style={{ maxWidth: 1100, margin: "0 auto" }}>

        {/* Header */}
        <div style={{ fontSize: 11, color: MUTED, letterSpacing: "0.08em",
          textTransform: "uppercase", marginBottom: 8 }}>
          {PROJECT_KEY} · {BOARD_NAME} · Board {BOARD_ID}
        </div>
        <h1 style={{ fontSize: 26, fontWeight: 700, margin: "0 0 4px 0" }}>Throughput Stability</h1>
        <div style={{ fontSize: 13, color: MUTED, marginBottom: 20 }}>
          {BUCKET_LABEL === "week" ? "Weekly" : "Monthly"} Delivery Volume · XmR Process Behavior Chart · {DATE_RANGE}
        </div>

        {/* Stat cards */}
        <div style={{ display: "flex", flexWrap: "wrap", gap: 12, marginBottom: 16 }}>
          <StatCard label="X̄ MEAN"  value={`${MEAN.toFixed(1)} / ${BUCKET_LABEL}`} color={XMR_MEAN} valueSize={20} />
          <StatCard label="UNPL"    value={`${UNPL.toFixed(1)} / ${BUCKET_LABEL}`} color={XMR_UNPL} valueSize={20} />
          <StatCard label="LNPL"    value={LNPL > 0 ? `${LNPL.toFixed(1)} / ${BUCKET_LABEL}` : "—"} color={XMR_LNPL} valueSize={20} />
          <StatCard label="WINDOW"  value={`${BUCKETS.length} ${BUCKET_LABEL}s`} color={MUTED} valueSize={20} />
        </div>

        {/* Badges */}
        <div style={{ display: "flex", flexWrap: "wrap", gap: 8, marginBottom: 24, alignItems: "center" }}>
          <Badge text={hasSignals ? "⚠ SIGNALS DETECTED" : "✓ STABLE"}
            color={hasSignals ? ALARM : POSITIVE} />
          {lastPartial && (
            <Badge text={`⚠ ${BUCKETS[BUCKETS.length - 1].label} — partial ${BUCKET_LABEL}`}
              color={CAUTION} />
          )}
        </div>

        {/* Main Chart */}
        <div style={{ background: PANEL_BG, borderRadius: 12,
          border: `1px solid ${BORDER}`, padding: "20px 12px 12px 12px", marginBottom: 4 }}>
          <div style={{ height: CFG.mainChartHeight }}>
          <ResponsiveContainer width="100%" height="100%">
            <ComposedChart data={BUCKETS} margin={CFG.mainMargin}>
              <CartesianGrid strokeDasharray="3 3" stroke={BORDER} vertical={false} />
              <XAxis dataKey="label" angle={-45} textAnchor="end" height={60} interval={interval}
                tick={{ fill: MUTED, fontSize: 11, fontFamily: FONT_STACK }} />
              <YAxis domain={[0, mainYMax]}
                tick={{ fill: MUTED, fontSize: 11, fontFamily: FONT_STACK }}
                label={{ value: `Items / ${BUCKET_LABEL}`, angle: -90, position: "insideLeft",
                  fill: MUTED, fontSize: 11, fontFamily: FONT_STACK }} />
              <Tooltip content={CustomTooltip} />
              {ACTIVE_TYPES.map(t => (
                <Bar key={t} dataKey={t} stackId="tp" fill={TYPE_COLORS[t]} isAnimationActive={false}>
                  {BUCKETS.map((b, i) => (
                    <Cell key={i}
                      fillOpacity={b.isPartial ? CFG.partialOpacity : 1}
                      stroke={b.isSignal && t === ACTIVE_TYPES[ACTIVE_TYPES.length - 1] ? ALARM : "none"}
                      strokeWidth={b.isSignal && t === ACTIVE_TYPES[ACTIVE_TYPES.length - 1] ? 2 : 0}
                    />
                  ))}
                </Bar>
              ))}
              <ReferenceLine y={UNPL} stroke={XMR_UNPL} strokeDasharray="6 3" strokeWidth={1.5}
                label={{ value: "UNPL", fill: XMR_UNPL, fontSize: 10, position: "insideTopLeft",
                  fontFamily: FONT_STACK }} />
              <ReferenceLine y={MEAN} stroke={XMR_MEAN} strokeDasharray="4 4" strokeWidth={1.5}
                label={{ value: "X̄", fill: XMR_MEAN, fontSize: 10, position: "insideTopLeft",
                  fontFamily: FONT_STACK }} />
              {LNPL > 0 && (
                <ReferenceLine y={LNPL} stroke={XMR_LNPL} strokeDasharray="6 3" strokeWidth={1}
                  label={{ value: "LNPL", fill: XMR_LNPL, fontSize: 10, position: "insideBottomLeft",
                    fontFamily: FONT_STACK }} />
              )}
            </ComposedChart>
          </ResponsiveContainer>
          </div>

          {/* Legend */}
          <div style={{ display: "flex", flexWrap: "wrap", gap: 16, justifyContent: "center", marginTop: 8 }}>
            {ACTIVE_TYPES.map(t => (
              <span key={t} style={{ fontSize: 11, color: MUTED }}>
                <span style={{ color: TYPE_COLORS[t] }}>●</span>{" "}{t}
              </span>
            ))}
            <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
              <div style={{ width: 24, height: 0, borderTop: `1.5px dashed ${XMR_UNPL}` }} />
              <span style={{ fontSize: 11, color: MUTED }}>UNPL</span>
            </div>
            <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
              <div style={{ width: 24, height: 0, borderTop: `1.5px dashed ${XMR_MEAN}` }} />
              <span style={{ fontSize: 11, color: MUTED }}>X̄ Mean</span>
            </div>
            {LNPL > 0 && (
              <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
                <div style={{ width: 24, height: 0, borderTop: `1.5px dashed ${XMR_LNPL}` }} />
                <span style={{ fontSize: 11, color: MUTED }}>LNPL</span>
              </div>
            )}
          </div>
        </div>

        {/* Moving Range Chart */}
        <div style={{ background: PANEL_BG, borderRadius: 12,
          border: `1px solid ${BORDER}`, padding: "12px", marginBottom: 24 }}>
          <div style={{ fontSize: 11, color: MUTED, marginBottom: 8,
            letterSpacing: "0.05em", textTransform: "uppercase" }}>
            Moving Range ({BUCKET_LABEL}-to-{BUCKET_LABEL} variation)
          </div>
          <div style={{ height: CFG.mrChartHeight }}>
          <ResponsiveContainer width="100%" height="100%">
            <ComposedChart data={BUCKETS} margin={CFG.mrMargin}>
              <CartesianGrid strokeDasharray="3 3" stroke={BORDER} vertical={false} />
              <XAxis dataKey="label" angle={-45} textAnchor="end" height={60} interval={interval}
                tick={{ fill: MUTED, fontSize: 11, fontFamily: FONT_STACK }} />
              <YAxis domain={[0, mrYMax]}
                tick={{ fill: MUTED, fontSize: 11, fontFamily: FONT_STACK }}
                label={{ value: "Moving Range", angle: -90, position: "insideLeft",
                  fill: MUTED, fontSize: 11, fontFamily: FONT_STACK }} />
              <Tooltip content={CustomTooltip} />
              <Bar dataKey="mr" isAnimationActive={false}>
                {BUCKETS.map((b, i) => (
                  <Cell key={i}
                    fill={b.mr > MR_UNPL ? ALARM : MUTED}
                    fillOpacity={b.mr == null ? 0 : 1}
                  />
                ))}
              </Bar>
              <ReferenceLine y={MR_UNPL} stroke={XMR_UNPL} strokeDasharray="6 3" strokeWidth={1.5}
                label={{ value: "URL", fill: XMR_UNPL, fontSize: 10, position: "insideTopLeft",
                  fontFamily: FONT_STACK }} />
              <ReferenceLine y={MR_MEAN} stroke={XMR_MEAN} strokeDasharray="4 4" strokeWidth={1.5}
                label={{ value: "MR̄", fill: XMR_MEAN, fontSize: 10, position: "insideTopLeft",
                  fontFamily: FONT_STACK }} />
            </ComposedChart>
          </ResponsiveContainer>
          </div>
        </div>

        {/* Footer */}
        <div style={{ fontSize: 11, color: MUTED, lineHeight: 1.7,
          borderTop: `1px solid ${BORDER}`, paddingTop: 16 }}>
          <div style={{ marginBottom: 8 }}>
            <b style={{ color: TEXT }}>Reading this chart:</b> Bars show delivery volume
            per {BUCKET_LABEL}, stacked by issue type. The dashed X̄ is the process mean. UNPL and
            LNPL define the expected range of natural variation — bars outside these limits signal a
            special cause worth investigating. The Moving Range panel shows {BUCKET_LABEL}-to-{BUCKET_LABEL}{" "}
            variation; spikes above the Upper Range Limit (URL) indicate unusually large step-changes
            in delivery rate.
          </div>
          <div>
            <b style={{ color: TEXT }}>Data provenance:</b> Wheeler XmR Process Behavior
            Chart. Limits: X̄ ± 2.66 × MR̄. URL = 3.267 × MR̄. Partial {BUCKET_LABEL}s shown at
            reduced opacity, excluded from limit calculations.
          </div>
        </div>

      </div>
    </div>
  );
}
