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
2. Construct the `CHART_ATTRS` object as described in the schema below.
3. Write the MCP tool result as JSON to `/home/claude/mcp_response.json`.
4. Write the CHART_ATTRS object as JSON to `/home/claude/chart_attrs.json`.
5. Copy `process_evolution.jsx` and `inject.py` from the skill bundle root to `/home/claude/`.
6. Run: `python3 /home/claude/inject.py /home/claude/process_evolution.jsx /home/claude/mcp_response.json /home/claude/chart_attrs.json`
7. Copy the result to `/mnt/user-data/outputs/process_evolution.jsx`.
8. Call `present_files` with `/mnt/user-data/outputs/process_evolution.jsx`.
9. Delete `/home/claude/mcp_response.json`, `/home/claude/chart_attrs.json`, `/home/claude/process_evolution.jsx`, and `/home/claude/inject.py`.

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
