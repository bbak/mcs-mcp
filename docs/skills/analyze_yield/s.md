---
name: analyze_yield-chart
description: >
  Renders a Yield chart (yield rate bars + loss breakdown by tier + per-type detail cards with donuts)
  from an mcs-mcp:analyze_yield result.
---

# analyze_yield — Chart Skill

## Template file

`yield.jsx` (in the same directory as this skill file)

## Workflow

1. Ensure `mcs-mcp:analyze_yield` has been called and its result is available.
2. Construct the `CHART_ATTRS` object as described in the schema below.
3. Write the MCP tool result as JSON to `/home/claude/mcp_response.json`.
4. Write the CHART_ATTRS object as JSON to `/home/claude/chart_attrs.json`.
5. Copy `yield.jsx` and `inject.py` from the skill bundle root to `/home/claude/`.
6. Run: `python3 /home/claude/inject.py /home/claude/yield.jsx /home/claude/mcp_response.json /home/claude/chart_attrs.json`
7. Copy the result to `/mnt/user-data/outputs/yield.jsx`.
8. Call `present_files` with `/mnt/user-data/outputs/yield.jsx`.
9. Delete `/home/claude/mcp_response.json`, `/home/claude/chart_attrs.json`, `/home/claude/yield.jsx`, and `/home/claude/inject.py`.

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

- No optional parameters — tool always returns pool + full per-type stratification.
- Issue types derived from `Object.keys(data.stratified)` — never hardcoded.
- Tier names (Demand/Upstream/Downstream) and severity levels (Low/Medium/High) are API-fixed.
- `lossPoints` always looked up with `.find(l => l.tier === tier)` — never by index.
- Not every tier appears in lossPoints for every type.
- In-flight count = ingested - delivered - abandoned; stat card only shown if > 0.
- Yield color thresholds: >=80% POSITIVE, >=65% CAUTION, <65% ALARM.
- YieldDonut uses PieChart with 3 slices (delivered/abandoned/other), clockwise from top.
