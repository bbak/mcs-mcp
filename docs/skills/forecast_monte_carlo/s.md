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

1. Call `mcs-mcp:forecast_monte_carlo` in one or both modes (duration / scope)
   if not already done.
2. Prepare the structured `duration` and `scope` objects from each mode's response
   (see shapes below). Set to `null` if that mode was not run.
3. Create an output copy of the template file (e.g. `monte_carlo.jsx`).
4. In that copy, find the string `"__MCP_RESPONSE__"` and replace it with `{}`
   (not used directly — both mode objects are in CHART_ATTRS).
5. Find the string `"__CHART_ATTRS__"` and replace it with the attrs object
   described below as an inline JSON literal.
6. Deliver the resulting `.jsx` file to the user.

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
