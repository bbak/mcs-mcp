---
name: mcs-charts-router
description: >
  Router skill for all MCS-MCP chart visualizations. Trigger this skill whenever
  any mcs-mcp analysis tool result is present in the conversation and the user asks
  to visualize, chart, plot, or show it. This router maps the tool that produced
  the result to the correct chart sub-skill. Do NOT attempt to build any chart
  ad-hoc — always read the sub-skill first.
---

# MCS Charts Router

This is the sole entry point for all MCS-MCP chart skills. When a chart request
arrives, identify which analysis tool produced the data, then read and follow the
matching sub-skill before writing any code.

---

## Step 1 — Identify the data source

Look at the conversation for the most recent mcs-mcp tool result. Match it to one
of the tools in the routing table below.

---

## Step 2 — Routing Table

```
Tool that produced the data          Sub-skill path (relative to this file)
────────────────────────────────     ──────────────────────────────────────────────
analyze_process_stability            analyze_process_stability/s.md
analyze_throughput                   analyze_throughput/s.md
analyze_wip_stability                analyze_wip_stability/s.md
analyze_wip_age_stability            analyze_wip_age_stability/s.md
analyze_work_item_age                analyze_work_item_age/s.md
analyze_process_evolution            analyze_process_evolution/s.md
analyze_residence_time               analyze_residence_time/s.md
generate_cfd_data                    generate_cfd_data/s.md
analyze_cycle_time                   analyze_cycle_time/s.md
analyze_status_persistence           analyze_status_persistence/s.md
analyze_flow_debt                    analyze_flow_debt/s.md
analyze_yield                        analyze_yield/s.md
forecast_monte_carlo                 forecast_monte_carlo/s.md
forecast_backtest                    forecast_backtest/s.md
```

Sub-folder names match the exact tool name as registered in the MCP server.

---

## Step 3 — Read the sub-skill, then build

Use the `view` tool to read the matched sub-skill file in full before writing any
code. The sub-skill contains the complete specification: data preparation, color
tokens, chart architecture, injection checklist, and the pre-delivery checklist.

Do not skip the sub-skill read even if you believe you remember the spec — the
sub-skill is the authoritative source and may have been updated.

---

## Disambiguation Rules

### Multiple tool results in the conversation

If more than one mcs-mcp result is present, use the one the user most recently
referenced, or the one most recently produced by a tool call. If ambiguous, ask
the user which result to visualize.

### "Show me that" / "chart this" follow-ups

These are implicit chart requests. Apply the routing table to the last mcs-mcp
tool result visible in the conversation.

### Unsupported tool results

If the tool is not in the routing table (e.g. `import_boards`,
`workflow_discover_mapping`, `guide_diagnostic_roadmap`, `import_project_context`,
`workflow_set_mapping`, `workflow_set_order`), no chart sub-skill exists. Explain
this to the user and offer a text summary instead.

---

## Delivery Rules (apply to ALL charts)

These rules apply regardless of which sub-skill is used:

```
1. Present the chart as a JSX Artifact via create_file + present_files.
   NEVER deliver a chart as a code block in chat.

2. All data embedded as JS literals inside the .jsx file — no fetch calls.

3. Single self-contained .jsx file with a default export.

4. Dark theme throughout: PAGE_BG=#080a0f page, PANEL_BG=#0c0e16 panels,
   BORDER=#1a1d2e grid lines.

5. Monospace font ('Courier New', monospace) throughout.

6. No Recharts Legend component — always use manual SVG/div legends.

7. Run the sub-skill's "Checklist Before Delivering" before presenting the file.
```

---

## Shared Visual Language

All charts share these color tokens. They are defined in each sub-skill and must
not be overridden:

```js
const ALARM = "#ff6b6b"; // UNPL breaches, above signals, critical thresholds
const CAUTION = "#e2c97e"; // process shifts, warnings, P85 SLE highlight
const PRIMARY = "#6b7de8"; // mean reference line, main series, left axis
const SECONDARY = "#7edde2"; // WIP line, supporting series, right axis
const POSITIVE = "#6bffb8"; // LNPL, below signals, stable/good status
const TEXT = "#dde1ef"; // body text, labels
const MUTED = "#505878"; // secondary labels, dimmed elements
const PAGE_BG = "#080a0f"; // page background
const PANEL_BG = "#0c0e16"; // panel/card background
const BORDER = "#1a1d2e"; // grid lines, card borders
```

These tokens are defined at the top of every generated JSX file. Sub-skills may
define additional tokens (e.g. `SEVERE = "#ff9b6b"` in `analyze_work_item_age`)
but must never redefine the shared tokens above.
