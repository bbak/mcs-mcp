---
name: analyze_wip_stability-chart
description: >
  Renders a WIP Count Stability chart (daily WIP line + XmR limits + signal dots)
  from an mcs-mcp:analyze_wip_stability result.
---

# analyze_wip_stability — Chart Skill

## Template file

`wip_stability.jsx` (in the same directory as this skill file)

## Workflow

1. Ensure `mcs-mcp:analyze_wip_stability` has been called and its result is available.
2. Construct the `CHART_ATTRS` object as described in the schema below.
3. Write the MCP tool result as JSON to `/home/claude/mcp_response.json`.
4. Write the CHART_ATTRS object as JSON to `/home/claude/chart_attrs.json`.
5. Copy `wip_stability.jsx` and `inject.py` from the skill bundle root to `/home/claude/`.
6. Run: `python3 /home/claude/inject.py /home/claude/wip_stability.jsx /home/claude/mcp_response.json /home/claude/chart_attrs.json`
7. Copy the result to `/mnt/user-data/outputs/wip_stability.jsx`.
8. Call `present_files` with `/mnt/user-data/outputs/wip_stability.jsx`.
9. Delete `/home/claude/mcp_response.json`, `/home/claude/chart_attrs.json`, `/home/claude/wip_stability.jsx`, and `/home/claude/inject.py`.

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

- `history_window_weeks` (default 26) only affects data volume, not response shape.
- `run_chart[].date` contains timezone offsets — the template strips to YYYY-MM-DD via `.slice(0,10)`.
- Signals are classified by `description` field: "above" → UNPL breach, "below" → LNPL breach.
- `signals` may be `null` — the template handles this defensively.
- Downsampling keeps every 2nd point but always retains signal points.
- Consecutive signal dates are grouped into ranges for badge display.
