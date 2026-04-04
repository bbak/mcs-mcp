import { useState, useMemo } from "react";
import {
  ComposedChart, Area, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer,
} from "recharts";
import { PRIMARY, SECONDARY, POSITIVE, TEXT, MUTED, PAGE_BG, PANEL_BG, BORDER, typeColor, FONT_STACK } from "mcs-mcp";
import { StatCard, TOOLTIP_BG } from "./shared.jsx";

// ── INJECTED DATA ─────────────────────────────────────────────────────────────
// Payload is injected by the MCS chart renderer as window.__MCS_PAYLOAD__.

const __MCS_ENVELOPE__ = window.__MCS_PAYLOAD__;
const __MCS_DATA__ = __MCS_ENVELOPE__.data;
const __MCS_GUARDRAILS__ = __MCS_ENVELOPE__.guardrails;
const __MCS_WORKFLOW__ = __MCS_ENVELOPE__.workflow;
// ── CONFIG ────────────────────────────────────────────────────────────────────

const CFD_PALETTE = [
  "#ef4444","#f97316","#eab308","#22c55e","#06b6d4",
  "#0ea5e9","#3b82f6","#8b5cf6","#d946ef","#ec4899",
  "#10b981","#14b8a6",
];


// ── DERIVED ───────────────────────────────────────────────────────────────────

const cfd = __MCS_DATA__.cfd_data;

const BOARD_ID    = __MCS_WORKFLOW__.board_id;
const PROJECT_KEY = __MCS_WORKFLOW__.project_key;
const BOARD_NAME  = __MCS_WORKFLOW__.board_name;

// Status order from workflow mapping (authoritative) — passed via workflow payload
const STATUS_ORDER_NAMES = __MCS_WORKFLOW__.status_order_names;

// Legend order = workflow order; chart stack order = reversed (Recharts stacks bottom-to-top)
const statusesForLegend = STATUS_ORDER_NAMES;
const statusesForChart  = [...STATUS_ORDER_NAMES].reverse();

const statusColors = Object.fromEntries(
  statusesForLegend.map((s, i) => [s, CFD_PALETTE[i % CFD_PALETTE.length]])
);

// Issue types — from CFD data
const ALL_ISSUE_TYPES = cfd.availableIssueTypes;
const ISSUE_TYPE_COLORS = Object.fromEntries(
  ALL_ISSUE_TYPES.map(t => [t, typeColor(t, ALL_ISSUE_TYPES)])
);

// Raw stratified data — downsample if needed
const rawBuckets = cfd.buckets.map(b => ({
  date:   b.label,
  byType: b.by_issue_type,
}));

const RAW_STRATIFIED = rawBuckets.length > 60
  ? rawBuckets.filter((d, i) => i === 0 || i === rawBuckets.length - 1 || i % 2 === 0)
  : rawBuckets;

// Date range and day count
const startDate = RAW_STRATIFIED[0]?.date || "";
const endDate   = RAW_STRATIFIED[RAW_STRATIFIED.length - 1]?.date || "";
const dayCount  = startDate && endDate
  ? Math.round((new Date(endDate) - new Date(startDate)) / 86400000) : 0;

// ── AGGREGATION ───────────────────────────────────────────────────────────────

function buildChartData(raw, selectedTypes) {
  const aggregated = raw.map(({ date, byType }) => {
    const entry = { date };
    selectedTypes.forEach(type => {
      const counts = byType[type] || {};
      Object.entries(counts).forEach(([status, count]) => {
        entry[status] = (entry[status] || 0) + count;
      });
    });
    return entry;
  });
  const baseline = aggregated[0]?.["Done"] || 0;
  return aggregated.map(d => ({ ...d, Done: (d.Done || 0) - baseline }));
}

// ── SUB-COMPONENTS ────────────────────────────────────────────────────────────

const CustomTooltip = ({ active, payload, label, visibleStatuses }) => {
  if (!active || !payload?.length) return null;
  const byName = Object.fromEntries(payload.map(p => [p.dataKey, p.value]));
  const total = statusesForLegend.reduce((s, st) =>
    s + (visibleStatuses.has(st) && byName[st] ? byName[st] : 0), 0);

  const fmtDate = d => {
    try { return new Date(d).toLocaleDateString("en-GB", { day: "2-digit", month: "short", year: "numeric" }); }
    catch { return d; }
  };

  return (
    <div style={{ background: TOOLTIP_BG, border: `1px solid ${BORDER}`, borderRadius: 8,
      padding: "10px 14px", fontFamily: FONT_STACK, fontSize: 12, color: TEXT,
      minWidth: 180 }}>
      <div style={{ fontWeight: 700, marginBottom: 6 }}>{fmtDate(label)}</div>
      {statusesForLegend.map(s => {
        if (!visibleStatuses.has(s) || !byName[s]) return null;
        return (
          <div key={s} style={{ display: "flex", justifyContent: "space-between", gap: 16 }}>
            <span style={{ color: statusColors[s] }}>{s}</span>
            <span>{byName[s]}</span>
          </div>
        );
      })}
      <div style={{ display: "flex", justifyContent: "space-between", gap: 16,
        fontWeight: 700, borderTop: `1px solid ${BORDER}`, marginTop: 4, paddingTop: 4 }}>
        <span>Total</span><span>{total}</span>
      </div>
    </div>
  );
};

// ── MAIN EXPORT ───────────────────────────────────────────────────────────────

export default function CfdChart() {
  const [selectedTypes, setSelectedTypes] = useState(new Set(ALL_ISSUE_TYPES));
  const [visibleStatuses, setVisibleStatuses] = useState(new Set(statusesForLegend));

  const toggleIssueType = (type) => {
    setSelectedTypes(prev => {
      const next = new Set(prev);
      if (next.has(type)) {
        if (next.size === 1) return prev;
        next.delete(type);
      } else {
        next.add(type);
      }
      return next;
    });
  };

  const toggleStatus = (s) => {
    setVisibleStatuses(prev => {
      const next = new Set(prev);
      if (next.has(s)) {
        if (next.size === 1) return prev;
        next.delete(s);
      } else {
        next.add(s);
      }
      return next;
    });
  };

  const selectedArr = useMemo(() => [...selectedTypes], [selectedTypes]);
  const chartData = useMemo(() => buildChartData(RAW_STRATIFIED, selectedArr), [selectedArr]);

  const finalDone = useMemo(() => {
    const first = RAW_STRATIFIED[0];
    const last  = RAW_STRATIFIED[RAW_STRATIFIED.length - 1];
    let delta = 0;
    for (const type of selectedArr) {
      delta += (last.byType[type]?.["Done"] || 0) -
               (first.byType[type]?.["Done"] || 0);
    }
    return delta;
  }, [selectedArr]);

  const fmtX = d => {
    try { return new Date(d).toLocaleDateString("en-GB", { day: "2-digit", month: "short" }); }
    catch { return d; }
  };
  const interval = Math.max(1, Math.floor(chartData.length / 10));

  return (
    <div style={{ background: PAGE_BG, minHeight: "100vh", padding: "24px 20px",
      fontFamily: FONT_STACK, color: TEXT }}>
      <div style={{ maxWidth: 1100, margin: "0 auto" }}>

        {/* Header */}
        <div style={{ fontSize: 11, color: MUTED, letterSpacing: "0.08em",
          textTransform: "uppercase", marginBottom: 6 }}>
          {PROJECT_KEY} · {BOARD_NAME} · Board {BOARD_ID}
        </div>
        <h1 style={{ fontSize: 22, fontWeight: 700, margin: "0 0 4px" }}>Cumulative Flow Diagram</h1>
        <div style={{ fontSize: 12, color: MUTED, marginBottom: 16 }}>
          {startDate} – {endDate} · {dayCount} days
        </div>

        {/* Stat cards */}
        <div style={{ display: "flex", flexWrap: "wrap", gap: 10, marginBottom: 16 }}>
          <StatCard label="WINDOW"      value={`${dayCount} days`}                                      color={MUTED} />
          <StatCard label="STATUSES"    value={statusesForLegend.length}                                color={PRIMARY} />
          <StatCard label="ISSUE TYPES" value={`${selectedTypes.size} / ${ALL_ISSUE_TYPES.length}`}     color={SECONDARY} />
          <StatCard label="DELIVERED"   value={`+${finalDone}`}                                         color={POSITIVE} />
        </div>

        {/* Issue type toggles */}
        <div style={{ display: "flex", flexWrap: "wrap", gap: 6, marginBottom: 16, alignItems: "center" }}>
          <div style={{ flex: 1 }} />
          {ALL_ISSUE_TYPES.map(type => {
            const active = selectedTypes.has(type);
            const color = ISSUE_TYPE_COLORS[type];
            return (
              <button key={type} onClick={() => toggleIssueType(type)} style={{
                fontSize: 10, padding: "5px 14px", borderRadius: 6, cursor: "pointer",
                background: active ? `${color}18` : "#1a1d2e",
                border: `1.5px solid ${active ? color : "#404660"}`,
                color: active ? color : "#505878",
                fontFamily: FONT_STACK, fontWeight: 700,
                transition: "all 0.2s ease",
              }}>{type}</button>
            );
          })}
        </div>

        {/* Chart panel */}
        <div style={{ background: PANEL_BG, borderRadius: 12,
          border: `1px solid ${BORDER}`, padding: "16px 8px 12px" }}>
          <ResponsiveContainer width="100%" height={520}>
            <ComposedChart data={chartData} margin={{ top: 10, right: 20, left: 10, bottom: 60 }}>
              <CartesianGrid strokeDasharray="3 3" stroke={BORDER} vertical={false} />
              <XAxis dataKey="date" tickFormatter={fmtX} interval={interval}
                angle={-45} textAnchor="end" height={60}
                tick={{ fill: MUTED, fontSize: 10, fontFamily: FONT_STACK }} />
              <YAxis tick={{ fill: MUTED, fontSize: 10, fontFamily: FONT_STACK }}
                label={{ value: "Items", angle: -90, position: "insideLeft",
                  fill: MUTED, fontSize: 10 }} />
              <Tooltip content={props =>
                <CustomTooltip {...props} visibleStatuses={visibleStatuses} />} />
              {statusesForChart.map(s => (
                visibleStatuses.has(s) ? (
                  <Area key={s} type="monotone" dataKey={s} stackId="1"
                    fill={statusColors[s]} stroke={statusColors[s]}
                    fillOpacity={0.7} strokeWidth={1.5}
                    dot={false} activeDot={false}
                    isAnimationActive={true} animationDuration={600} />
                ) : null
              ))}
            </ComposedChart>
          </ResponsiveContainer>

          {/* Status legend / toggles */}
          <div style={{ display: "flex", flexWrap: "wrap", gap: 6, justifyContent: "center", marginTop: 10 }}>
            {statusesForLegend.map(s => {
              const active = visibleStatuses.has(s);
              return (
                <button key={s} onClick={() => toggleStatus(s)} style={{
                  padding: "4px 12px", background: "transparent",
                  border: `2px solid ${active ? statusColors[s] : "#505878"}`,
                  color: active ? statusColors[s] : "#505878",
                  borderRadius: 6, cursor: "pointer", fontSize: 10,
                  opacity: active ? 1 : 0.45,
                  fontFamily: FONT_STACK,
                  transition: "all 0.2s ease",
                }}>{s}</button>
              );
            })}
          </div>
        </div>

        {/* Footer */}
        <div style={{ fontSize: 11, color: MUTED, lineHeight: 1.7,
          borderTop: `1px solid ${BORDER}`, paddingTop: 14, marginTop: 16 }}>
          <div style={{ marginBottom: 6 }}>
            <b style={{ color: TEXT }}>Reading this chart: </b>
            Each colored band shows items in that workflow status per period. Wider bands
            signal accumulation or bottlenecks. Use the issue type toggles (top right) to
            filter by type. Click the status buttons below the chart to focus on specific
            workflow stages.
          </div>
          <div>
            <b style={{ color: TEXT }}>Note on Done normalization: </b>
            Done is shown relative to Day 1 of the window, highlighting delivery velocity
            rather than cumulative historical volume. When issue types are filtered, counts
            scale proportionally to the selected types only.
          </div>
        </div>

      </div>
    </div>
  );
}
