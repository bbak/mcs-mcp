---
name: analyze_wip_stability-chart
description: >
  Renders a WIP Count Stability chart (daily WIP line + XmR limits + signal dots)
  from an mcs-mcp:analyze_wip_stability result.
---

# analyze_wip_stability — Chart Skill

## Template file

`wip_stability.jsx` (in the same directory as this skill file)

## Workflow

1. Ensure `mcs-mcp:analyze_wip_stability` has been called and its result is available.
2. Create an output copy of the template file (e.g. `wip_stability.jsx`).
3. In that copy, find the string `"__MCP_RESPONSE__"` and replace it with the full
   tool result as an inline JSON literal.
4. Find the string `"__CHART_ATTRS__"` and replace it with the attrs object
   described below as an inline JSON literal.
5. Deliver the resulting `.jsx` file to the user.

## CHART_ATTRS schema

```json
{
  "board_id":    4711,
  "project_key": "PROJKEY",
  "board_name":  "The Board Name"
}
```

Only these three fields are required. The JSX derives everything else from MCP_RESPONSE.

## Notes

- `history_window_weeks` (default 26) only affects data volume, not response shape.
- `run_chart[].date` contains timezone offsets — the template strips to YYYY-MM-DD via `.slice(0,10)`.
- Signals are classified by `description` field: "above" → UNPL breach, "below" → LNPL breach.
- `signals` may be `null` — the template handles this defensively.
- Downsampling keeps every 2nd point but always retains signal points.
- Consecutive signal dates are grouped into ranges for badge display.
