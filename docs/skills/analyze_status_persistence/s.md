---
name: analyze_status_persistence-chart
description: >
  Renders a Status Persistence chart (tier summary + pool P85/P95 bars + per-type P85 bars)
  from an mcs-mcp:analyze_status_persistence result.
---

# analyze_status_persistence — Chart Skill

## Template file

`status_persistence.jsx` (in the same directory as this skill file)

## Workflow

1. Ensure `mcs-mcp:analyze_status_persistence` has been called and its result is available.
2. Create an output copy of the template file (e.g. `status_persistence.jsx`).
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
  "board_name":  "The Board Name",
  "status_order_names":  ["Backlog", "To Do", "In Dev", "Code Review", "Testing"]
}
```

Four fields required. `status_order_names` from `workflow_discover_mapping`,
Finished tier (Done/Closed) EXCLUDED. The template filters pool and stratified
data to only these statuses.

## Notes

- No optional parameters — tool always returns all statuses and types.
- Uses "persistence" and "dwell" terminology — NEVER "residence" (that's a different tool).
- Tier colors (Demand/Upstream/Downstream) and role colors (active/queue) are API-fixed — safe to hardcode by key.
- Issue type names are NEVER hardcoded — derived from `Object.keys(stratified_persistence)`.
- Pool panel uses stacked bars: P85 solid + P95 extension (dim).
- Each bar uses Cell for per-row role-based coloring (active=cyan, queue=red).
- Custom TierTick on Y-axis shows tier dot + role-colored status name.
- Per-type panel shows P85 only (grouped bars per issue type).
- Top bottleneck computed dynamically from Downstream tier entries.
