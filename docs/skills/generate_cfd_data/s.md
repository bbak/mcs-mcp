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
2. Create an output copy of the template file (e.g. `cfd.jsx`).
3. In that copy, find the string `"__MCP_RESPONSE__"` and replace it with the full
   `generate_cfd_data` tool result as an inline JSON literal.
4. Find the string `"__CHART_ATTRS__"` and replace it with the attrs object
   described below as an inline JSON literal.
5. Deliver the resulting `.jsx` file to the user.

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
