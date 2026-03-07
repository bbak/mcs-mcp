---
name: mcs-charts-base
description: >
  Base skill for all MCS-MCP chart skills. Defines shared visual language, layout structure,
  component patterns, and constraints. Always read this skill in addition to the specific
  chart skill — never use this skill alone.
---

# MCS Charts — Base Skill

> **This skill is not standalone.** It defines shared conventions for all MCS-MCP chart
> skills. Each specific chart skill references this base and overrides where needed.

---

## Stack

- **React** functional components with hooks
- **Recharts** for all chart rendering
- **Inline styles only** — no CSS classes, no external stylesheets
- Single self-contained `.jsx` file with a default export
- No fetch calls — all data embedded as JS array/object literals hardcoded from the tool response

---

## Color Tokens

Use these named tokens consistently. Each chart skill maps its data series to these roles.

```
ALARM:      #ff6b6b   — XmR breaches, above-UNPL signals, error states
CAUTION:    #e2c97e   — process shifts, warnings, amber signals
PRIMARY:    #6b7de8   — main data series, process average reference line
SECONDARY:  #7edde2   — supporting series, secondary axis
POSITIVE:   #6bffb8   — below-LNPL signals, favourable deviations

PAGE_BG:    #080a0f
PANEL_BG:   #0c0e16
BORDER:     #1a1d2e
GRID:       #1a1d2e
TICK:       #404660
TEXT:       #dde1ef
MUTED:      #505878
MUTED_DARK: #4a5270
```

> **Semantic assignment is chart-specific.** Which series gets PRIMARY vs SECONDARY is
> decided per chart based on what the data story requires. Only ALARM is fixed: it always
> means "outside natural process limits / requires attention."

---

## Typography

`'Courier New', monospace` throughout — titles, labels, tooltips, badges, footers, everything.

---

## Page & Panel Wrapper

```jsx
// Page container
<div style={{
  background: "#080a0f",
  minHeight: "100vh",
  padding: "32px 24px",
  fontFamily: "'Courier New', monospace",
  color: "#dde1ef",
}}>
  <div style={{ maxWidth: 1100, margin: "0 auto" }}>
    {/* content */}
  </div>
</div>

// Chart panel
<div style={{
  background: "#0c0e16",
  borderRadius: 12,
  border: "1px solid #1a1d2e",
  padding: "20px 12px 12px 12px",
  marginBottom: 20,
}}>
  {/* optional panel subtitle */}
  {/* ResponsiveContainer + chart */}
  {/* manual legend */}
</div>
```

---

## Header Structure

Every chart follows this top-to-bottom header order:

1. **Breadcrumb** — project key · board name · board ID, muted, uppercase, letter-spaced, ~11px
2. **H1 title** — exact chart name as specified in the chart skill
3. **Subtitle** — chart type descriptor + date range (e.g. "XmR Process Behavior Chart · Jan 2024 – Mar 2026")
4. **Stat cards** — key summary values; colors and content defined per chart skill
5. **Badge row** — signal callouts + status verdict; see Badge System below

---

## Stat Cards

Horizontal flex row of cards. Each card:

```jsx
<div style={{
  background: "#0c0e16",
  border: `1px solid ${color}33`,   // color at ~20% opacity
  borderRadius: 8,
  padding: "10px 16px",
  minWidth: 100,
}}>
  <div style={{ fontSize: 10, color: "#505878", marginBottom: 4, letterSpacing: "0.05em" }}>
    {label}
  </div>
  <div style={{ fontSize: 20, fontWeight: 700, color: color }}>
    {value}
  </div>
</div>
```

Card content (labels, values, colors) is defined per chart skill.

---

## Badge System

Badges communicate signal episodes and status verdicts below the header.

```jsx
// Badge container
<div style={{ display: "flex", flexWrap: "wrap", gap: 8, marginBottom: 20 }}>
  {/* badges */}
</div>

// Badge template
<span style={{
  fontSize: 11,
  padding: "4px 10px",
  borderRadius: 4,
  background: `${color}15`,
  border: `1px solid ${color}40`,
  color: color,
}}>
  {text}
</span>
```

**Semantic color assignments for badges:**
- Signal above UNPL / alarm → ALARM `#ff6b6b`
- Signal below LNPL / positive → POSITIVE `#6bffb8`
- Process shift / caution → CAUTION `#e2c97e`
- Status / summary → PRIMARY `#6b7de8` or MUTED `#505878`

> **Note:** Whether a "Status: STABLE/UNSTABLE" badge is shown depends on the specific
> chart. Some tool outputs surface status as meaningful to the user; others use status
> as an internal classification only. The chart skill specifies which applies.

---

## CartesianGrid

```jsx
<CartesianGrid strokeDasharray="3 3" stroke="#1a1d2e" vertical={false} />
```

Horizontal grid lines only. Always.

---

## X-Axis Date Formatting

Format date strings as `DD MMM` (e.g. "15 Oct") for tick labels:

```js
const formatDate = (d) =>
  new Date(d).toLocaleDateString("en-GB", { day: "2-digit", month: "short" });
```

Use `interval` prop to avoid label crowding — tune based on data density.
For rotated labels: `angle={-45}`, `textAnchor="end"`, `height={60}`.

---

## Tooltip Base Style

Custom tooltip, never the Recharts default.

```jsx
const CustomTooltip = ({ active, payload }) => {
  if (!active || !payload?.length) return null;
  return (
    <div style={{
      background: "#0f1117",
      border: "1px solid #1a1d2e",
      borderRadius: 8,
      padding: "10px 14px",
      fontFamily: "'Courier New', monospace",
      fontSize: 12,
      color: "#dde1ef",
    }}>
      {/* chart-specific content */}
    </div>
  );
};
```

Tooltip content (fields, labels, colors) is defined per chart skill.

---

## Area Gradient Pattern

```jsx
<defs>
  <linearGradient id="{gradId}" x1="0" y1="0" x2="0" y2="1">
    <stop offset="5%"  stopColor="{color}" stopOpacity={0.25} />
    <stop offset="95%" stopColor="{color}" stopOpacity={0.02} />
  </linearGradient>
</defs>
```

Each chart skill specifies the gradient ID and color. Multiple gradients can coexist with distinct IDs.

---

## Legend

Never use the built-in Recharts `<Legend>` component. Always render legends manually as
inline SVG icons + labels below the chart panel:

```jsx
// Example legend item — line type
<div style={{ display: "flex", alignItems: "center", gap: 6 }}>
  <svg width={24} height={12}>
    <line x1={0} y1={6} x2={24} y2={6} stroke={color} strokeWidth={2} />
  </svg>
  <span style={{ fontSize: 11, color: "#505878" }}>{label}</span>
</div>

// Example legend item — area/fill type
<div style={{ display: "flex", alignItems: "center", gap: 6 }}>
  <svg width={24} height={12}>
    <rect x={0} y={2} width={24} height={8} fill={color} fillOpacity={0.25} rx={2} />
  </svg>
  <span style={{ fontSize: 11, color: "#505878" }}>{label}</span>
</div>
```

Legend items and groupings are defined per chart skill.

---

## Interactive Controls (e.g. Scale Toggle)

When a chart includes an interactive control (such as a log/linear scale toggle), style it
as a clearly interactive button — not a pill badge:

```jsx
<button onClick={handler} style={{
  fontSize: 10,
  padding: "5px 14px",
  borderRadius: 6,
  cursor: "pointer",
  background: active ? `${color}18` : "#1a1d2e",
  border: `1.5px solid ${active ? color : "#404660"}`,
  color: active ? color : "#dde1ef",
  fontFamily: "'Courier New', monospace",
  fontWeight: 700,
}}>
  {label}
</button>
```

Place interactive controls in the badge row, right-aligned via a flex spacer:
```jsx
<div style={{ flex: 1 }} />
<button ...>...</button>
```

Whether a scale toggle or other control is included is specified per chart skill.

---

## Footer

Every chart ends with a footer explanation section. Minimum content:
- What the chart shows and why it matters
- How to read the visual encoding (colors, shapes, reference lines)
- Any important caveats about data scope or interpretation

Style: small text (~11–12px), muted color (`#505878`), modest top margin.
Content is defined per chart skill.

---

## Universal Checklist Items

These apply to every chart. The chart skill's checklist covers only chart-specific items.

- [ ] All data embedded as JS array/object literals — no fetch calls
- [ ] XmR/summary constants hardcoded from the tool response
- [ ] Dark background applied to page (`#080a0f`) and all chart panels (`#0c0e16`)
- [ ] Monospace font used throughout
- [ ] CartesianGrid: horizontal only, `#1a1d2e`
- [ ] Custom tooltip (no Recharts default)
- [ ] Manual legend (no Recharts `<Legend>`)
- [ ] Header: breadcrumb → title → subtitle → stat cards → badges
- [ ] Footer explanation present
- [ ] Output is a single self-contained `.jsx` file with a default export
