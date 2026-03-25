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
2. Construct the `CHART_ATTRS` object as described in the schema below.
3. Write the MCP tool result as JSON to `/home/claude/mcp_response.json`.
4. Write the CHART_ATTRS object as JSON to `/home/claude/chart_attrs.json`.
5. Copy `status_persistence.jsx` and `inject.py` from the skill bundle root to `/home/claude/`.
6. Run: `python3 /home/claude/inject.py /home/claude/status_persistence.jsx /home/claude/mcp_response.json /home/claude/chart_attrs.json`
7. Copy the result to `/mnt/user-data/outputs/status_persistence.jsx`.
8. Call `present_files` with `/mnt/user-data/outputs/status_persistence.jsx`.
9. Delete `/home/claude/mcp_response.json`, `/home/claude/chart_attrs.json`, `/home/claude/status_persistence.jsx`, and `/home/claude/inject.py`.

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
