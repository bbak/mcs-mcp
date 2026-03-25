---
name: generate_cfd_data-chart
description: >
  Renders a Cumulative Flow Diagram (stacked area with interactive toggles)
  from an mcs-mcp:generate_cfd_data result + workflow mapping.
---

# generate_cfd_data — Chart Skill

## Template file

`cfd.jsx` (in the same directory as this skill file)

## Workflow

1. Ensure both `mcs-mcp:workflow_discover_mapping` and `mcs-mcp:generate_cfd_data`
   have been called and their results are available.
2. Construct the `CHART_ATTRS` object as described in the schema below.
3. Write the `generate_cfd_data` MCP tool result as JSON to `/home/claude/mcp_response.json`.
4. Write the CHART_ATTRS object as JSON to `/home/claude/chart_attrs.json`.
5. Copy `cfd.jsx` and `inject.py` from the skill bundle root to `/home/claude/`.
6. Run: `python3 /home/claude/inject.py /home/claude/cfd.jsx /home/claude/mcp_response.json /home/claude/chart_attrs.json`
7. Copy the result to `/mnt/user-data/outputs/cfd.jsx`.
8. Call `present_files` with `/mnt/user-data/outputs/cfd.jsx`.
9. Delete `/home/claude/mcp_response.json`, `/home/claude/chart_attrs.json`, `/home/claude/cfd.jsx`, and `/home/claude/inject.py`.

## CHART_ATTRS schema

```json
{
  "board_id":    4711,
  "project_key": "PROJKEY",
  "board_name":  "The Board Name",
  "status_order_names":  ["Backlog", "To Do", "In Dev", "Code Review", "Testing", "Done"]
}
```

Four fields required. `status_order_names` is derived from `workflow_discover_mapping`:
`status_order.map(id => status_mapping[id].name)`. This is the SOLE authoritative source
for stack order — never derive order from the CFD data's `statuses[]` array.

## Notes

- `granularity`: "weekly" (recommended, ~26 points) or "daily" (~182 points, auto-downsampled).
- Status names and issue types are NEVER hardcoded — all derived dynamically from data.
- `statusesForChart` is the REVERSE of `statusesForLegend` (Recharts stacks bottom-to-top).
- Done is normalized to Day 1 of the window (baseline subtracted).
- DELIVERED stat card is computed from raw data, not from normalized chartData.
- Issue type and status toggles guard against reducing selection to zero.
- Both ISSUE TYPES and DELIVERED stat cards update reactively on toggle.
