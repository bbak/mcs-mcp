---
name: analyze_process_stability-chart
description: >
  Renders a Process Stability chart (Cycle Time Scatterplot with XmR, tabs, mini-charts)
  from an mcs-mcp:analyze_process_stability result.
---

# analyze_process_stability — Chart Skill

## Template file

`process_stability.jsx` (in the same directory as this skill file)

## Workflow

1. Ensure `mcs-mcp:analyze_process_stability` has been called and its result is available.
2. Create an output copy of the template file (e.g. `process_stability.jsx`).
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

- `include_raw_series` is optional — the template works correctly regardless of whether
  it is `true` or `false`. The template uses only `data.scatterplot[]` for chart data.
- Issue types are detected dynamically from `data.scatterplot[].issue_type` — no hardcoding.
- Stratified tabs only appear for types present in `data.stratified`.
- The Overall tab always appears first.
- Log/Linear scale toggle is built in — no parameter needed.
- Signal objects have shape `{ index, key, type: "outlier"|"shift" }`.
