---
name: analyze_residence_time-chart
description: >
  Renders a Residence Time Analysis chart (dual panel: coherence gap + departure rate)
  from an mcs-mcp:analyze_residence_time result.
---

# analyze_residence_time — Chart Skill

## Template file

`residence_time.jsx` (in the same directory as this skill file)

## Workflow

1. Ensure `mcs-mcp:analyze_residence_time` has been called and its result is available.
2. Create an output copy of the template file (e.g. `residence_time.jsx`).
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

- `granularity`: "weekly" (recommended) or "daily". Detected automatically from label format.
- Daily data >120 points is downsampled to every 3rd point, retaining first and last.
- `a` (window arrivals) and `d` (total historical resolved) are NOT symmetric — never present as "arrivals vs departures".
- `d` is always labeled as "(W* denominator)" in badges.
- `lambda` is cumulative departure rate (d/h), not arrival rate.
- `convergence` values: "converging", "diverging", "metastable".
- Tool always applies backflow reset (last commitment date) — noted in footer.
- `issue_types` filter is optional; does not change response structure.
