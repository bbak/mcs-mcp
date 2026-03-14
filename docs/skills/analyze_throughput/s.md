---
name: analyze_throughput-chart
description: >
  Renders a Throughput Stability chart (stacked bars + Moving Range)
  from an mcs-mcp:analyze_throughput result.
---

# analyze_throughput — Chart Skill

## Template file

`throughput.jsx` (in the same directory as this skill file)

## Workflow

1. Ensure `mcs-mcp:analyze_throughput` has been called and its result is available.
2. Create an output copy of the template file (e.g. `throughput.jsx`).
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

- `bucket` param (`week` / `month`) affects label format — no template change needed,
  the JSX detects it automatically from `@metadata[0].label`.
- Partial buckets (last week in progress) render at reduced opacity automatically.
- Signal highlighting is driven by `data.stability.signals` in the response — no manual input.
