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
2. Construct the `CHART_ATTRS` object as described in the schema below.
3. Write the MCP tool result as JSON to `/home/claude/mcp_response.json`.
4. Write the CHART_ATTRS object as JSON to `/home/claude/chart_attrs.json`.
5. Copy `process_stability.jsx` and `inject.py` from the skill bundle root to `/home/claude/`.
6. Run: `python3 /home/claude/inject.py /home/claude/process_stability.jsx /home/claude/mcp_response.json /home/claude/chart_attrs.json`
7. Copy the result to `/mnt/user-data/outputs/process_stability.jsx`.
8. Call `present_files` with `/mnt/user-data/outputs/process_stability.jsx`.
9. Delete `/home/claude/mcp_response.json`, `/home/claude/chart_attrs.json`, `/home/claude/process_stability.jsx`, and `/home/claude/inject.py`.

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
