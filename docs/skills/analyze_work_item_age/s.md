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
2. Create an output copy of the template file (e.g. `work_item_age.jsx`).
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
