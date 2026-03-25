---
name: analyze_cycle_time-chart
description: >
  Renders a Cycle Time SLE chart (four panels: predictability, pool SLE, per-type SLE,
  scatterplot) from an mcs-mcp:analyze_cycle_time result.
---

# analyze_cycle_time — Chart Skill

## Template file

`cycle_time.jsx` (in the same directory as this skill file)

## Workflow

1. Ensure `mcs-mcp:analyze_cycle_time` has been called and its result is available.
2. Construct the `CHART_ATTRS` object as described in the schema below.
3. Write the MCP tool result as JSON to `/home/claude/mcp_response.json`.
4. Write the CHART_ATTRS object as JSON to `/home/claude/chart_attrs.json`.
5. Copy `cycle_time.jsx` and `inject.py` from the skill bundle root to `/home/claude/`.
6. Run: `python3 /home/claude/inject.py /home/claude/cycle_time.jsx /home/claude/mcp_response.json /home/claude/chart_attrs.json`
7. Copy the result to `/mnt/user-data/outputs/cycle_time.jsx`.
8. Call `present_files` with `/mnt/user-data/outputs/cycle_time.jsx`.
9. Delete `/home/claude/mcp_response.json`, `/home/claude/chart_attrs.json`, `/home/claude/cycle_time.jsx`, and `/home/claude/inject.py`.

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

- Panels 1-3 are static SLE snapshots. Panel 4 is a cycle-time scatterplot showing
  individual items by completion date, with SLE reference lines (P50, P70, P85, P95).
- Issue types are derived dynamically from `context.stratification_decisions[]`.
- Percentile keys (aggressive, unlikely, etc.) are fixed by API contract — safe to hardcode.
- Issue type names are NOT safe to hardcode — always use dynamic arrays.
- `issue_types`, `start_status`, `end_status` are optional filters; they do not change response shape.
- Fat-tail thresholds: >=5.6 extreme (ALARM), >=3 significant (CAUTION), else moderate (POSITIVE).
- Tail-to-median thresholds: >=3 (ALARM), >=2 (CAUTION), else (POSITIVE).
- Per-type panel only shows eligible types; ineligible listed below with volume.
- P85 is always highlighted as the canonical SLE — no user toggle.
