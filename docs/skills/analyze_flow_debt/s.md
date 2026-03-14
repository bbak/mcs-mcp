---
name: analyze_flow_debt-chart
description: >
  Renders a Flow Debt chart (arrivals vs departures + cumulative debt area)
  from an mcs-mcp:analyze_flow_debt result.
---

# analyze_flow_debt — Chart Skill

## Template file

`flow_debt.jsx` (in the same directory as this skill file)

## Workflow

1. Ensure `mcs-mcp:analyze_flow_debt` has been called and its result is available.
2. Create an output copy of the template file (e.g. `flow_debt.jsx`).
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

- `bucket_size` ("week" default, or "month") affects label format — detected automatically via regex.
- `history_window_weeks` (default 26) affects data volume, not response shape.
- No workflow mapping prerequisite — self-contained tool.
- Debt coloring is always computed from sign: positive=ALARM, negative=POSITIVE, zero=MUTED.
- X-tick thinning: ~8 visible labels regardless of bucket count.
- Cumulative debt computed via running sum in `useMemo`.
- Footer must reference actual TOTAL_DEBT value — never a generic placeholder.
