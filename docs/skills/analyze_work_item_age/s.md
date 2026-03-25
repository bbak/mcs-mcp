---
name: analyze_work_item_age-chart
description: >
  Renders a WIP Item Age chart (age ranking + by status + by type views, outlier table)
  from an mcs-mcp:analyze_work_item_age result.
---

# analyze_work_item_age — Chart Skill

## Template file

`work_item_age.jsx` (in the same directory as this skill file)

## Workflow

1. Ensure `mcs-mcp:analyze_work_item_age` has been called and its result is available.
2. Construct the `CHART_ATTRS` object as described in the schema below.
3. Write the MCP tool result as JSON to `/home/claude/mcp_response.json`.
4. Write the CHART_ATTRS object as JSON to `/home/claude/chart_attrs.json`.
5. Copy `work_item_age.jsx` and `inject.py` from the skill bundle root to `/home/claude/`.
6. Run: `python3 /home/claude/inject.py /home/claude/work_item_age.jsx /home/claude/mcp_response.json /home/claude/chart_attrs.json`
7. Copy the result to `/mnt/user-data/outputs/work_item_age.jsx`.
8. Call `present_files` with `/mnt/user-data/outputs/work_item_age.jsx`.
9. Delete `/home/claude/mcp_response.json`, `/home/claude/chart_attrs.json`, `/home/claude/work_item_age.jsx`, and `/home/claude/inject.py`.

## CHART_ATTRS schema

```json
{
  "board_id":    4711,
  "project_key": "PROJKEY",
  "board_name":  "The Board Name",
  "status_order":  ["To Do", "In Development", "Code Review", "Testing", "Done"]
}
```

Four fields required. `status_order` is the canonical workflow status order from
`workflow_discover_mapping` / `workflow_set_order`. If not available, the template
falls back to the unique statuses found in the data (unordered).

## Notes

- `age_type` defaults to "wip" — uses `age_since_commitment_days` as primary metric.
- `tier_filter` defaults to "WIP" — excludes Demand and Finished items.
- `is_aging_outlier` is used directly from the tool — never recalculated.
- Issue types are detected dynamically; unknown types get fallback colors.
- Three views: Age Ranking (horizontal bars), By Status (stacked), By Type (stacked).
- Recharts Rule: Cell only in Age Ranking (single Bar, no stackId). By Status/Type use uniform fill per Bar.
- Outlier table always visible below chart, filtered by active type toggle.
- Severity scale: >=300d critical (red), 120-299d severe (orange), 85-119d outlier (amber), <P85 normal (dimmed).
