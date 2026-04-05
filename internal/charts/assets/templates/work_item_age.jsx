import { useState } from "react";
import {
  BarChart, Bar, Cell, ReferenceLine, XAxis, YAxis,
  CartesianGrid, Tooltip, ResponsiveContainer,
} from "recharts";
import { ALARM, CAUTION, PRIMARY, SECONDARY, POSITIVE, TEXT, MUTED, PAGE_BG, PANEL_BG, BORDER, typeColor, FONT_STACK } from "mcs-mcp";
import { StatCard, Badge, TOOLTIP_BG } from "./shared.jsx";

// ── INJECTED DATA ─────────────────────────────────────────────────────────────
// Payload is injected by the MCS chart renderer as window.__MCS_PAYLOAD__.

const __MCS_ENVELOPE__ = window.__MCS_PAYLOAD__;
const __MCS_DATA__ = __MCS_ENVELOPE__.data;
const __MCS_GUARDRAILS__ = __MCS_ENVELOPE__.guardrails;
const __MCS_WORKFLOW__ = __MCS_ENVELOPE__.workflow;
// ── CONFIG ────────────────────────────────────────────────────────────────────

const SEVERE = "#ff9b6b";

// ── DERIVED ───────────────────────────────────────────────────────────────────

const d = __MCS_DATA__;

const BOARD_ID      = __MCS_WORKFLOW__.board_id;
const PROJECT_KEY   = __MCS_WORKFLOW__.project_key;
const BOARD_NAME    = __MCS_WORKFLOW__.board_name;
const STATUS_ORDER  = __MCS_WORKFLOW__.status_order_names;

const P50       = d.summary.p50_threshold_days;
const P85       = d.summary.p85_threshold_days;
const P95       = d.summary.p95_threshold_days;
const TOTAL     = d.summary.total_items;
const OUTLIER_COUNT = d.summary.outlier_count;
const STABILITY = d.summary.stability_index;

const AGING = d.aging.map(item => ({
  key:       item.key,
  type:      item.type,
  status:    item.status,
  wip:       item.age_since_commitment_days,
  statusAge: item.age_in_current_status_days,
  pct:       item.percentile,
  outlier:   item.is_aging_outlier,
}));

const sorted     = [...AGING].sort((a, b) => b.wip - a.wip);
const medianItem = sorted[Math.floor(sorted.length / 2)] || { wip: 0 };

const presentTypes = [...new Set(AGING.map(d => d.type))].sort();

const typeColorMap = Object.fromEntries(
  presentTypes.map(t => [t, typeColor(t, presentTypes)])
);

// ── HELPERS ───────────────────────────────────────────────────────────────────

function outlierColor(age) {
  if (age >= 300) return ALARM;
  if (age >= 120) return SEVERE;
  if (age >= 85)  return CAUTION;
  return PRIMARY;
}

function abbrevStatus(s) {
  return s
    .replace("awaiting", "await")
    .replace("development", "dev")
    .replace("deploying", "depl")
    .replace("deploy to", "->")
    .replace("UAT (+Fix)", "UAT+Fix")
    .replace("awaiting UAT", "await UAT");
}

function siVerdict(si) {
  if (si > 1.3) return { label: "Clogged", color: ALARM };
  if (si < 0.7) return { label: "Starving", color: CAUTION };
  return { label: "Balanced", color: POSITIVE };
}

// ── SUB-COMPONENTS ────────────────────────────────────────────────────────────

function OutlierRow({ d, i }) {
  return (
    <tr style={{ background: i % 2 === 0 ? "transparent" : `${PRIMARY}05` }}>
      <td style={{ padding: "8px 8px", color: outlierColor(d.wip), fontWeight: 700, whiteSpace: "nowrap" }}>{d.key}</td>
      <td style={{ padding: "8px 8px", color: typeColorMap[d.type] || MUTED, whiteSpace: "nowrap" }}>{d.type}</td>
      <td style={{ padding: "8px 8px", color: MUTED, fontSize: 10, whiteSpace: "nowrap" }}>{d.status}</td>
      <td style={{ padding: "8px 8px", color: outlierColor(d.wip), fontWeight: 700, whiteSpace: "nowrap" }}>{d.wip.toFixed(1)}d</td>
      <td style={{ padding: "8px 8px", color: SECONDARY, whiteSpace: "nowrap" }}>{d.statusAge}d</td>
      <td style={{ padding: "8px 8px", color: ALARM, whiteSpace: "nowrap" }}>P{d.pct}</td>
    </tr>
  );
}


const Swatch = ({ color, label }) => (
  <div style={{ display: "flex", alignItems: "center", gap: 5 }}>
    <div style={{ width: 14, height: 10, background: color, borderRadius: 2, opacity: 0.85 }} />
    <span style={{ fontSize: 11, color: MUTED }}>{label}</span>
  </div>
);

// ── TOOLTIPS ──────────────────────────────────────────────────────────────────

const RankingTooltip = ({ active, payload }) => {
  if (!active || !payload?.length) return null;
  const d = payload[0].payload;
  return (
    <div style={{ background: TOOLTIP_BG, border: `1px solid ${BORDER}`, borderRadius: 8,
      padding: "10px 14px", fontFamily: FONT_STACK, fontSize: 12, color: TEXT }}>
      <div style={{ fontWeight: 700, marginBottom: 4 }}>{d.key}</div>
      <div style={{ color: typeColorMap[d.type] || MUTED, fontSize: 11, marginBottom: 6 }}>{d.type} · {d.status}</div>
      <div style={{ borderTop: `1px solid ${BORDER}`, paddingTop: 6 }}>
        <div>WIP age: <b style={{ color: outlierColor(d.wip) }}>{d.wip.toFixed(1)}d</b></div>
        <div>In status: <b style={{ color: SECONDARY }}>{d.statusAge}d</b></div>
        <div>Percentile: <span style={{ color: d.outlier ? ALARM : MUTED }}>P{d.pct}</span></div>
      </div>
      {d.outlier && <div style={{ color: ALARM, fontWeight: 700, marginTop: 4 }}>⚠ Aging outlier (P85+)</div>}
    </div>
  );
};

const StatusTooltip = ({ active, payload }) => {
  if (!active || !payload?.length) return null;
  const d = payload[0].payload;
  return (
    <div style={{ background: TOOLTIP_BG, border: `1px solid ${BORDER}`, borderRadius: 8,
      padding: "10px 14px", fontFamily: FONT_STACK, fontSize: 12, color: TEXT }}>
      <div style={{ fontWeight: 700, marginBottom: 6 }}>{d.fullStatus}</div>
      <div style={{ borderTop: `1px solid ${BORDER}`, paddingTop: 6 }}>
        <div>Outliers: <span style={{ color: ALARM }}>{d.outliers}</span></div>
        <div>Normal: <span style={{ color: PRIMARY }}>{d.normal}</span></div>
        <div style={{ color: MUTED }}>Total: {d.count}</div>
        <div>Max age: <span style={{ color: CAUTION }}>{d.maxAge}d</span></div>
      </div>
    </div>
  );
};

const TypeTooltip = ({ active, payload }) => {
  if (!active || !payload?.length) return null;
  const d = payload[0].payload;
  return (
    <div style={{ background: TOOLTIP_BG, border: `1px solid ${BORDER}`, borderRadius: 8,
      padding: "10px 14px", fontFamily: FONT_STACK, fontSize: 12, color: TEXT }}>
      <div style={{ fontWeight: 700, color: typeColorMap[d.type] || TEXT, marginBottom: 6 }}>{d.type}</div>
      <div style={{ borderTop: `1px solid ${BORDER}`, paddingTop: 6 }}>
        <div>Total WIP: {d.total}</div>
        <div>Outliers: <span style={{ color: ALARM }}>{d.outliers}</span></div>
        <div>Max age: <span style={{ color: CAUTION }}>{d.maxAge}d</span></div>
        <div>Avg age: <span style={{ color: SECONDARY }}>{d.avgAge.toFixed(1)}d</span></div>
      </div>
    </div>
  );
};

// ── MAIN EXPORT ───────────────────────────────────────────────────────────────

export default function WipItemAgeChart() {
  const [view, setView] = useState("ranking");
  const [typeFilter, setTypeFilter] = useState("All");

  const si = siVerdict(STABILITY);

  // Filtered data for Age Ranking and By Status views
  const filtered = typeFilter === "All" ? sorted : sorted.filter(d => d.type === typeFilter);

  // By Status data
  const byStatus = (STATUS_ORDER || [...new Set(AGING.map(d => d.status))]).map(s => {
    const items = AGING.filter(d => d.status === s && (typeFilter === "All" || d.type === typeFilter));
    if (!items.length) return null;
    return {
      status:     abbrevStatus(s),
      fullStatus: s,
      count:      items.length,
      outliers:   items.filter(d => d.outlier).length,
      normal:     items.filter(d => !d.outlier).length,
      maxAge:     Math.max(...items.map(d => d.wip)),
    };
  }).filter(Boolean);

  // By Type data (no type filter applied)
  const byType = presentTypes.map(t => {
    const items = AGING.filter(d => d.type === t);
    return {
      type:     t,
      total:    items.length,
      outliers: items.filter(d => d.outlier).length,
      normal:   items.filter(d => !d.outlier).length,
      maxAge:   items.length ? Math.max(...items.map(d => d.wip)) : 0,
      avgAge:   items.length ? items.reduce((s, d) => s + d.wip, 0) / items.length : 0,
    };
  });

  // Outlier table data
  const outlierRows = filtered.filter(d => d.outlier);

  const rankingHeight = Math.max(420, filtered.length * 14);

  return (
    <div style={{ background: PAGE_BG, minHeight: "100vh", padding: "24px 20px",
      fontFamily: FONT_STACK, color: TEXT }}>
      <div style={{ maxWidth: 1100, margin: "0 auto" }}>

        {/* Header */}
        <div style={{ fontSize: 11, color: MUTED, letterSpacing: "0.08em",
          textTransform: "uppercase", marginBottom: 6 }}>
          {PROJECT_KEY} · {BOARD_NAME} · Board {BOARD_ID}
        </div>
        <h1 style={{ fontSize: 22, fontWeight: 700, margin: "0 0 4px" }}>WIP Item Age</h1>
        <div style={{ fontSize: 12, color: MUTED, marginBottom: 16 }}>
          Age since commitment · {TOTAL} active items · P85 outlier threshold: {P85.toFixed(0)}d
        </div>

        {/* Stat cards */}
        <div style={{ display: "flex", flexWrap: "wrap", gap: 10, marginBottom: 14 }}>
          <StatCard label="TOTAL WIP"       value={TOTAL}                          color={MUTED} />
          <StatCard label="OUTLIERS (P85+)"  value={OUTLIER_COUNT}
            sub={`${TOTAL > 0 ? ((OUTLIER_COUNT / TOTAL) * 100).toFixed(0) : 0}% of WIP`} color={ALARM} />
          <StatCard label="P85 THRESHOLD"    value={`${P85.toFixed(0)}d`}
            sub="historical cycle time"                                             color={CAUTION} />
          <StatCard label="MEDIAN AGE"       value={`${medianItem.wip.toFixed(1)}d`} color={SECONDARY} />
          <StatCard label="STABILITY INDEX"  value={STABILITY.toFixed(2)}
            sub={`> 1.3 clogged / < 0.7 starving`}                                 color={si.color} />
        </div>

        {/* Badges + View toggles */}
        <div style={{ display: "flex", flexWrap: "wrap", gap: 6, marginBottom: 20, alignItems: "center" }}>
          <Badge text={`⚠ ${OUTLIER_COUNT} aging outliers — ${TOTAL > 0 ? ((OUTLIER_COUNT / TOTAL) * 100).toFixed(0) : 0}% of WIP exceeds P85`} color={ALARM} />
          <Badge text={`P85: ${P85.toFixed(0)}d`} color={CAUTION} />
          <div style={{ flex: 1 }} />
          {["ranking", "status", "type"].map(v => (
            <button key={v} onClick={() => setView(v)} style={{
              fontSize: 10, padding: "5px 12px", borderRadius: 6, cursor: "pointer",
              background: view === v ? `${PRIMARY}18` : PANEL_BG,
              border: `1.5px solid ${view === v ? PRIMARY : BORDER}`,
              color: view === v ? PRIMARY : MUTED,
              fontFamily: FONT_STACK, fontWeight: 700,
            }}>
              {v === "ranking" ? "AGE RANKING" : v === "status" ? "BY STATUS" : "BY TYPE"}
            </button>
          ))}
        </div>

        {/* Type filter — shown for ranking and status views */}
        {view !== "type" && (
          <div style={{ display: "flex", flexWrap: "wrap", gap: 6, marginBottom: 16 }}>
            {["All", ...presentTypes].map(t => {
              const active = typeFilter === t;
              const c = t === "All" ? TEXT : (typeColorMap[t] || TEXT);
              return (
                <button key={t} onClick={() => setTypeFilter(t)} style={{
                  fontSize: 10, padding: "4px 10px", borderRadius: 5, cursor: "pointer",
                  background: active ? `${c}18` : PANEL_BG,
                  border: `1.5px solid ${active ? c : BORDER}`,
                  color: active ? c : MUTED,
                  fontFamily: FONT_STACK,
                }}>
                  {t}
                </button>
              );
            })}
          </div>
        )}

        {/* Chart panel */}
        <div style={{ background: PANEL_BG, borderRadius: 12,
          border: `1px solid ${BORDER}`, padding: "16px 8px 12px", marginBottom: 16 }}>

          {view === "ranking" && (
            <>
              <div style={{ fontSize: 11, color: MUTED, marginBottom: 8 }}>
                WIP age per item, sorted descending — P50 / P85 / P95 thresholds marked
              </div>
              <ResponsiveContainer width="100%" height={rankingHeight}>
                <BarChart data={filtered} layout="vertical"
                  margin={{ top: 4, right: 80, bottom: 4, left: 10 }}>
                  <CartesianGrid strokeDasharray="3 3" stroke={BORDER} horizontal={false} />
                  <XAxis type="number" tickFormatter={v => v + "d"}
                    tick={{ fill: MUTED, fontSize: 10, fontFamily: FONT_STACK }} />
                  <YAxis type="category" dataKey="key" width={138}
                    tick={{ fill: MUTED, fontSize: 9, fontFamily: FONT_STACK }} />
                  <Tooltip content={RankingTooltip} cursor={{ fill: `${PRIMARY}0c` }} />
                  <Bar dataKey="wip" radius={[0, 3, 3, 0]} isAnimationActive={false}>
                    {filtered.map((d, i) => (
                      <Cell key={`cell-${i}`}
                        fill={outlierColor(d.wip)}
                        fillOpacity={d.outlier ? 0.9 : 0.45} />
                    ))}
                  </Bar>
                  <ReferenceLine x={P95} stroke={ALARM}    strokeDasharray="4 2" strokeWidth={1}
                    label={{ value: `P95 ${P95.toFixed(0)}d`, fill: ALARM,    fontSize: 10, position: "right" }} />
                  <ReferenceLine x={P85} stroke={CAUTION}  strokeDasharray="6 3" strokeWidth={1.5}
                    label={{ value: `P85 ${P85.toFixed(0)}d`, fill: CAUTION,  fontSize: 10, position: "right" }} />
                  <ReferenceLine x={P50} stroke={POSITIVE} strokeDasharray="3 3" strokeWidth={1}
                    label={{ value: `P50 ${P50.toFixed(0)}d`, fill: POSITIVE, fontSize: 10, position: "right" }} />
                </BarChart>
              </ResponsiveContainer>
              <div style={{ display: "flex", flexWrap: "wrap", gap: 12, justifyContent: "center", marginTop: 8 }}>
                <Swatch color={ALARM}   label=">= 300d — critical" />
                <Swatch color={SEVERE}  label="120-299d — severe" />
                <Swatch color={CAUTION} label="85-119d — outlier" />
                <Swatch color={PRIMARY} label="< P85 — normal (dimmed)" />
                <div style={{ width: 1, height: 14, background: BORDER }} />
                <div style={{ display: "flex", alignItems: "center", gap: 5 }}>
                  <div style={{ width: 24, height: 0, borderTop: `1.5px dashed ${ALARM}` }} />
                  <span style={{ fontSize: 11, color: MUTED }}>{`P95 ${P95.toFixed(0)}d`}</span>
                </div>
                <div style={{ display: "flex", alignItems: "center", gap: 5 }}>
                  <div style={{ width: 24, height: 0, borderTop: `1.5px dashed ${CAUTION}` }} />
                  <span style={{ fontSize: 11, color: MUTED }}>{`P85 ${P85.toFixed(0)}d`}</span>
                </div>
                <div style={{ display: "flex", alignItems: "center", gap: 5 }}>
                  <div style={{ width: 24, height: 0, borderTop: `1.5px dashed ${POSITIVE}` }} />
                  <span style={{ fontSize: 11, color: MUTED }}>{`P50 ${P50.toFixed(0)}d`}</span>
                </div>
              </div>
            </>
          )}

          {view === "status" && (
            <>
              <div style={{ fontSize: 11, color: MUTED, marginBottom: 8 }}>
                Items per workflow status — outliers vs normal
              </div>
              <ResponsiveContainer width="100%" height={310}>
                <BarChart data={byStatus}>
                  <CartesianGrid strokeDasharray="3 3" stroke={BORDER} vertical={false} />
                  <XAxis dataKey="status" angle={-35} textAnchor="end" height={70}
                    tick={{ fill: MUTED, fontSize: 10, fontFamily: FONT_STACK }} />
                  <YAxis tick={{ fill: MUTED, fontSize: 10, fontFamily: FONT_STACK }} />
                  <Tooltip content={StatusTooltip} cursor={{ fill: `${PRIMARY}0c` }} />
                  <Bar dataKey="outliers" stackId="s" fill={ALARM}   fillOpacity={0.85} isAnimationActive={false} />
                  <Bar dataKey="normal"   stackId="s" fill={PRIMARY} fillOpacity={0.5}  radius={[3, 3, 0, 0]} isAnimationActive={false} />
                </BarChart>
              </ResponsiveContainer>
              <div style={{ display: "flex", gap: 12, justifyContent: "center", marginTop: 8 }}>
                <Swatch color={ALARM}   label="Aging outliers (>= P85)" />
                <Swatch color={PRIMARY} label="Normal items (< P85)" />
              </div>
            </>
          )}

          {view === "type" && (
            <>
              <div style={{ fontSize: 11, color: MUTED, marginBottom: 8 }}>
                Items per issue type — outliers vs normal
              </div>
              <ResponsiveContainer width="100%" height={280}>
                <BarChart data={byType}>
                  <CartesianGrid strokeDasharray="3 3" stroke={BORDER} vertical={false} />
                  <XAxis dataKey="type"
                    tick={{ fill: MUTED, fontSize: 10, fontFamily: FONT_STACK }} />
                  <YAxis tick={{ fill: MUTED, fontSize: 10, fontFamily: FONT_STACK }} />
                  <Tooltip content={TypeTooltip} cursor={{ fill: `${PRIMARY}0c` }} />
                  <Bar dataKey="outliers" stackId="t" fill={ALARM}   fillOpacity={0.85} isAnimationActive={false} />
                  <Bar dataKey="normal"   stackId="t" fill={PRIMARY} fillOpacity={0.5}  radius={[3, 3, 0, 0]} isAnimationActive={false} />
                </BarChart>
              </ResponsiveContainer>
              <div style={{ display: "flex", gap: 12, justifyContent: "center", marginTop: 8 }}>
                <Swatch color={ALARM}   label="Aging outliers (>= P85)" />
                <Swatch color={PRIMARY} label="Normal items (< P85)" />
              </div>
            </>
          )}
        </div>

        {/* Outlier table */}
        {outlierRows.length > 0 && (
          <div style={{ background: PANEL_BG, borderRadius: 12,
            border: `1px solid ${BORDER}`, padding: "14px 12px", marginBottom: 16,
            overflowX: "auto" }}>
            <div style={{ fontSize: 11, color: MUTED, marginBottom: 10, letterSpacing: "0.05em",
              textTransform: "uppercase" }}>
              Aging Outliers ({outlierRows.length})
            </div>
            <table style={{ width: "100%", minWidth: 540, borderCollapse: "collapse", fontSize: 11 }}>
              <thead>
                <tr style={{ borderBottom: `1px solid ${BORDER}` }}>
                  <th style={{ padding: "6px 8px", textAlign: "left", color: MUTED, fontSize: 10, fontWeight: 700, whiteSpace: "nowrap" }}>ITEM</th>
                  <th style={{ padding: "6px 8px", textAlign: "left", color: MUTED, fontSize: 10, fontWeight: 700, whiteSpace: "nowrap" }}>TYPE</th>
                  <th style={{ padding: "6px 8px", textAlign: "left", color: MUTED, fontSize: 10, fontWeight: 700, whiteSpace: "nowrap" }}>STATUS</th>
                  <th style={{ padding: "6px 8px", textAlign: "left", color: MUTED, fontSize: 10, fontWeight: 700, whiteSpace: "nowrap" }}>WIP AGE</th>
                  <th style={{ padding: "6px 8px", textAlign: "left", color: MUTED, fontSize: 10, fontWeight: 700, whiteSpace: "nowrap" }}>IN STATUS</th>
                  <th style={{ padding: "6px 8px", textAlign: "left", color: MUTED, fontSize: 10, fontWeight: 700, whiteSpace: "nowrap" }}>PCT</th>
                </tr>
              </thead>
              <tbody>
                {outlierRows.map((d, i) => <OutlierRow key={d.key} d={d} i={i} />)}
              </tbody>
            </table>
          </div>
        )}

        {/* Footer */}
        <div style={{ fontSize: 11, color: MUTED, lineHeight: 1.7,
          borderTop: `1px solid ${BORDER}`, paddingTop: 14 }}>
          <div style={{ marginBottom: 6 }}>
            <b style={{ color: TEXT }}>Reading this chart: </b>
            WIP age is measured from the commitment point to today. Items flagged as aging
            outliers exceed the historical P85 cycle time — they have been in progress longer
            than 85% of all previously completed items. This does not necessarily mean they
            are blocked; it means they are statistically unusual and warrant attention.
            "Age in current status" shows how long an item has been sitting in its current
            workflow step — a high ratio relative to total WIP age suggests stalling at that step.
          </div>
          <div>
            <b style={{ color: TEXT }}>Stability Index: </b>
            {STABILITY.toFixed(2)} — {si.label}. Little's Law ratio (WIP ÷ Throughput ÷ Mean
            Cycle Time). Above 1.3 = clogged (more WIP than the system can handle); below
            0.7 = starving (underloaded). Between 0.7 and 1.3 = balanced flow.
          </div>
        </div>

      </div>
    </div>
  );
}
