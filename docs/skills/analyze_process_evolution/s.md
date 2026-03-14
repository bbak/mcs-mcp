---
name: analyze_process_evolution-chart
description: >
  Renders a Strategic Process Evolution chart (Three-Way Control Chart with scatter dots,
  disconnected avg bars, shift zone) from an mcs-mcp:analyze_process_evolution result.
---

# analyze_process_evolution — Chart Skill

## Template file

`process_evolution.jsx` (in the same directory as this skill file)

## Workflow

1. Ensure `mcs-mcp:analyze_process_evolution` has been called and its result is available.
2. Create an output copy of the template file (e.g. `process_evolution.jsx`).
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

- `history_window_months` (default 12, up to 60) affects data volume, not response shape.
- The tool does NOT provide issue type per item — do not claim type breakdown.
- Shift signal is optional — if absent, no shift badge, ReferenceArea, or shift coloring.
- Shift index = last subgroup in the 8-point run; shift starts at index - 7.
- Average bars are disconnected horizontal segments (paired left/right edge points).
- `useCallback` for DotShape and AvgBarShape, `useMemo` for pairPositions — required for stability.
- Log/Linear scale toggle built in.
- `data.evolution.status` is tool-internal — never surface as a badge.
- `cursor={false}` on ScatterChart to suppress default crosshair.
