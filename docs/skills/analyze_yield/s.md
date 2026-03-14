---
name: analyze_yield-chart
description: >
  Renders a Yield chart (yield rate bars + loss breakdown by tier + per-type detail cards with donuts)
  from an mcs-mcp:analyze_yield result.
---

# analyze_yield — Chart Skill

## Template file

`yield.jsx` (in the same directory as this skill file)

## Workflow

1. Ensure `mcs-mcp:analyze_yield` has been called and its result is available.
2. Create an output copy of the template file (e.g. `yield.jsx`).
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

- No optional parameters — tool always returns pool + full per-type stratification.
- Issue types derived from `Object.keys(data.stratified)` — never hardcoded.
- Tier names (Demand/Upstream/Downstream) and severity levels (Low/Medium/High) are API-fixed.
- `lossPoints` always looked up with `.find(l => l.tier === tier)` — never by index.
- Not every tier appears in lossPoints for every type.
- In-flight count = ingested - delivered - abandoned; stat card only shown if > 0.
- Yield color thresholds: >=80% POSITIVE, >=65% CAUTION, <65% ALARM.
- YieldDonut uses PieChart with 3 slices (delivered/abandoned/other), clockwise from top.
