---
name: forecast_backtest-chart
description: >
  Renders a Walk-Forward Backtest chart (accuracy gauge, actual vs predicted bars/lines,
  miss detail table) from mcs-mcp:forecast_backtest results.
---

# forecast_backtest — Chart Skill

## Template file

`backtest.jsx` (in the same directory as this skill file)

## Workflow

1. Call `mcs-mcp:forecast_backtest` in one or both modes (duration / scope) if not already done.
2. Construct the `CHART_ATTRS` object as described in the schema below,
   including the structured `duration` and/or `scope` objects from the responses.
   Set either to `null` if that mode was not run.
3. Write `{}` to `/home/claude/mcp_response.json`.
4. Write the CHART_ATTRS object as JSON to `/home/claude/chart_attrs.json`.
5. Copy `backtest.jsx` and `inject.py` from the skill bundle root to `/home/claude/`.
6. Run: `python3 /home/claude/inject.py /home/claude/backtest.jsx /home/claude/mcp_response.json /home/claude/chart_attrs.json`
7. Copy the result to `/mnt/user-data/outputs/backtest.jsx`.
8. Call `present_files` with `/mnt/user-data/outputs/backtest.jsx`.
9. Delete `/home/claude/mcp_response.json`, `/home/claude/chart_attrs.json`, `/home/claude/backtest.jsx`, and `/home/claude/inject.py`.

## CHART_ATTRS schema

```json
{
  "board_id":    4711,
  "project_key": "PROJKEY",
  "board_name":  "The Board Name",
  "duration":    { ... } or null,
  "scope":       { ... } or null
}
```

Five fields. `duration` and `scope` are structured objects prepared from each mode's
tool response. Set to `null` if that mode was not run.

### Mode object shape (same for both duration and scope)

```js
{
  simulation_mode: "scope" | "duration",
  accuracy_score:  data.accuracy.accuracy_score,
  hits:            checkpoints.filter(c => c.is_within_cone).length,
  total:           checkpoints.length,
  validation_msg:  data.accuracy.validation_message,
  checkpoints: [
    { date, actual, p50, p85, p95, hit, drift },  // flattened from tool response
    ...
  ],
}
```

Flattening: `actual` ← `actual_value`, `hit` ← `is_within_cone`, `drift` ← `drift_detected`.

## Notes

- Mode auto-detection: defaults to "duration" if available, else "scope".
- Toggle only rendered when BOTH modes are present.
- Checkpoints reversed to chronological order (API returns newest-first).
- Accuracy thresholds: >=80% reliable (POSITIVE), >=65% moderate (CAUTION), <65% unreliable (ALARM).
- Miss detail table only rendered if misses exist.
- Direction logic differs by mode: duration=actual<p50 is favorable, scope=actual>p50 is favorable.
- Cross-mode footer sentence only when bothModes === true.
- Drift badge only rendered if driftPts.length > 0.
