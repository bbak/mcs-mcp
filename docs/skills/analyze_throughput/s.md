---
name: analyze_throughput-chart
description: >
  Renders a Throughput Stability chart (stacked bars + Moving Range)
  from an mcs-mcp:analyze_throughput result.
---

# analyze_throughput — Chart Skill

## Template file

`throughput.jsx` (in the same directory as this skill file)

## Workflow

1. Ensure `mcs-mcp:analyze_throughput` has been called and its result is available.
2. Construct the `CHART_ATTRS` object as described in the schema below.
3. Write the MCP tool result as JSON to `/home/claude/mcp_response.json`.
4. Write the CHART_ATTRS object as JSON to `/home/claude/chart_attrs.json`.
5. Copy `throughput.jsx` and `inject.py` from the skill bundle root to `/home/claude/`.
6. Run: `python3 /home/claude/inject.py /home/claude/throughput.jsx /home/claude/mcp_response.json /home/claude/chart_attrs.json`
7. Copy the result to `/mnt/user-data/outputs/throughput.jsx`.
8. Call `present_files` with `/mnt/user-data/outputs/throughput.jsx`.
9. Delete `/home/claude/mcp_response.json`, `/home/claude/chart_attrs.json`, `/home/claude/throughput.jsx`, and `/home/claude/inject.py`.

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

- `bucket` param (`week` / `month`) affects label format — no template change needed,
  the JSX detects it automatically from `@metadata[0].label`.
- Partial buckets (last week in progress) render at reduced opacity automatically.
- Signal highlighting is driven by `data.stability.signals` in the response — no manual input.
