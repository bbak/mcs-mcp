import { PANEL_BG, MUTED, FONT_STACK } from "mcs-mcp";

// TOOLTIP_BG is the dark background used by every chart tooltip.
export const TOOLTIP_BG = "#0f1117";

// StatCard renders a labeled metric tile used in all chart headers.
// valueSize defaults to 18; pass valueSize={20} for the larger variant.
export const StatCard = ({ label, value, sub, color, valueSize = 18 }) => (
  <div style={{ background: PANEL_BG, border: `1px solid ${color}33`,
    borderRadius: 8, padding: "8px 14px", minWidth: 110 }}>
    <div style={{ fontSize: 10, color: MUTED, marginBottom: 3, letterSpacing: "0.05em" }}>{label}</div>
    <div style={{ fontSize: valueSize, fontWeight: 700, color }}>{value}</div>
    {sub && <div style={{ fontSize: 9, color: MUTED, marginTop: 2 }}>{sub}</div>}
  </div>
);

// Badge renders an inline pill label used for status, warnings, and guardrails.
export const Badge = ({ text, color }) => (
  <span style={{ fontSize: 11, padding: "3px 8px", borderRadius: 4,
    background: `${color}15`, border: `1px solid ${color}40`, color,
    fontFamily: FONT_STACK }}>{text}</span>
);
