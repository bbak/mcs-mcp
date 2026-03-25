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
2. Construct the `CHART_ATTRS` object as described in the schema below.
3. Write the MCP tool result as JSON to `/home/claude/mcp_response.json`.
4. Write the CHART_ATTRS object as JSON to `/home/claude/chart_attrs.json`.
5. Copy `flow_debt.jsx` and `inject.py` from the skill bundle root to `/home/claude/`.
6. Run: `python3 /home/claude/inject.py /home/claude/flow_debt.jsx /home/claude/mcp_response.json /home/claude/chart_attrs.json`
7. Copy the result to `/mnt/user-data/outputs/flow_debt.jsx`.
8. Call `present_files` with `/mnt/user-data/outputs/flow_debt.jsx`.
9. Delete `/home/claude/mcp_response.json`, `/home/claude/chart_attrs.json`, `/home/claude/flow_debt.jsx`, and `/home/claude/inject.py`.

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
