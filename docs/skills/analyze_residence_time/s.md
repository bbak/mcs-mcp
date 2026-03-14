---
name: analyze_residence_time-chart
description: >
  Renders a Residence Time Analysis chart (dual panel: coherence gap + flow rate balance)
  from an mcs-mcp:analyze_residence_time result.
---

# analyze_residence_time — Chart Skill

## Template file

`residence_time.jsx` (in the same directory as this skill file)

## Workflow

1. Ensure `mcs-mcp:analyze_residence_time` has been called and its result is available.
2. Create an output copy of the template file (e.g. `residence_time.jsx`).
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
  "board_name":  "The Board Name"
}
```

Only these three fields are required. The JSX derives everything else from MCP_RESPONSE.

## Notes

- `granularity`: "weekly" (recommended) or "daily". Detected automatically from label format.
- Daily data >120 points is downsampled to every 3rd point, retaining first and last.
- `a` (window arrivals) and `d` (total historical resolved) are NOT symmetric — never present as "arrivals vs departures".
- `d` is always labeled as "(W* denominator)" in badges.
- `lambda` (Λ) is the **arrival rate** = A(T)/T. It is NOT the departure rate.
- `theta` (Θ) is the **departure rate** = D(T)/T. When Λ > Θ, WIP is accumulating; when Θ > Λ, WIP is draining.
- `w_prime` (w′) is the **departure-denominated residence time** = H(T)/D(T). Diverges from `w` (arrival-denominated = H(T)/A(T)) when the system is unbalanced (arrivals ≠ departures). A large gap between w and w′ signals flow imbalance.
- `final_w_prime` and `final_theta` are the terminal values of w′ and Θ in the summary object.
- `convergence` values: "converging", "diverging", "metastable", "insufficient_data". Assessed via 1/T OLS regression on the w(T) tail.
- Tool always applies backflow reset (last commitment date) — noted in footer.
- `issue_types` filter is optional; does not change response structure.

## Panel descriptions

- **Panel 1 — Coherence Gap & Residence Time**: Shows w(T) (blue solid), w′(T) (violet dashed), and W*(T) (amber dashed). The gap between w(T) and W*(T) is the coherence gap (end-effect). The gap between w(T) and w′(T) signals arrival/departure imbalance.
- **Panel 2 — Flow Rate Balance**: Shows Λ(T) arrival rate (teal solid) and Θ(T) departure rate (green dashed). When the two lines diverge, WIP is either accumulating (Λ > Θ) or draining (Θ > Λ).
