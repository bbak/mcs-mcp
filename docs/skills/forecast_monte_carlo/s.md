---
name: forecast_monte_carlo-chart
description: >
  Renders a Monte Carlo Forecast chart (duration and/or scope mode with percentile bars,
  spread panel, stratification table) from mcs-mcp:forecast_monte_carlo results.
---

# forecast_monte_carlo — Chart Skill

## Template file

`monte_carlo.jsx` (in the same directory as this skill file)

## Workflow

1. Call `mcs-mcp:forecast_monte_carlo` in one or both modes (duration / scope) if not already done.
2. Construct the `CHART_ATTRS` object as described in the schema below,
   including the structured `duration` and/or `scope` objects from the responses.
   Set either to `null` if that mode was not run.
3. Write `{}` to `/home/claude/mcp_response.json`.
4. Write the CHART_ATTRS object as JSON to `/home/claude/chart_attrs.json`.
5. Copy `monte_carlo.jsx` and `inject.py` from the skill bundle root to `/home/claude/`.
6. Run: `python3 /home/claude/inject.py /home/claude/monte_carlo.jsx /home/claude/mcp_response.json /home/claude/chart_attrs.json`
7. Copy the result to `/mnt/user-data/outputs/monte_carlo.jsx`.
8. Call `present_files` with `/mnt/user-data/outputs/monte_carlo.jsx`.
9. Delete `/home/claude/mcp_response.json`, `/home/claude/chart_attrs.json`, `/home/claude/monte_carlo.jsx`, and `/home/claude/inject.py`.

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

### duration object shape

```js
{
  percentiles:      data.percentiles,
  labels:           data.percentile_labels,
  spread:           data.spread,
  predictability:   data.predictability,
  fat_tail_ratio:   data.fat_tail_ratio,
  composition:      data.composition,
  throughput_trend:  data.throughput_trend,
  context:          data.context,
  warnings:         guardrails.warnings,
}
```

### scope object shape

```js
{
  target_days:      N,  // the target_days parameter value
  percentiles:      data.percentiles,
  labels:           data.percentile_labels,
  spread:           data.spread,
  predictability:   data.predictability,
  fat_tail_ratio:   data.fat_tail_ratio,
  throughput_trend:  data.throughput_trend,
  context:          data.context,
  warnings:         guardrails.warnings,
}
```

## Notes

- Mode auto-detection: defaults to "duration" if available, else "scope".
- Toggle only rendered when BOTH modes are present.
- Duration: percentiles = days (lower = better). Scope: percentiles = items (higher = better).
- Scope axis is intentionally "reversed" — do NOT "fix" it.
- P85 ReferenceLine shown in both modes.
- Shared context cards always visible, sourced from active mode.
- Warnings rendered as CAUTION badges only if non-empty.
- Duration ADDITIONAL card only if additional_items > 0.
- Footer references actual injected values — never generic placeholders.
