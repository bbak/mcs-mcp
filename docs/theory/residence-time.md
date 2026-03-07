# Residence Time — Theory and Implementation

## Overview

**Sample Path Analysis** is a methodology for studying input-output systems by tracking the instantaneous count of active items over a continuous observation window. Rather than summarizing completed items (cycle time) or snapshotting current items (WIP age) separately, it constructs a single time series — the **sample path** N(t) — from which all flow metrics can be derived in a mathematically locked relationship.

The theoretical foundations come from Stidham (1972) and El-Taha & Stidham (1999). Krishna Kumar's work on applying these ideas to software delivery systems provides the practical bridge between queueing theory and flow metrics.

The core insight: there exists a **finite version of Little's Law** that holds unconditionally for any observation window, without requiring steady-state or equilibrium assumptions. This identity — `L(T) = Λ(T) · w(T)` — provides a unified view that ties together what existing tools measure separately.

## Key Definitions

### Sojourn Time (W)

The elapsed time for a specific item from its start event (commitment) to its end event (resolution). Sojourn time is **item-relative** — it measures the participant's complete journey through the system, regardless of when or how long the observer watches.

In MCS-MCP, this is what `analyze_cycle_time` measures for completed items.

### Residence Time (w)

The portion of each item's elapsed time that overlaps with a finite observation window [0, T]. Residence time is **observer-window-relative** — it measures what is visible during the measurement period, not the item's full journey.

For an individual item i with start time s_i and end time e_i:

```
r_i(T) = max(0, min(e_i, T) - max(s_i, 0))
```

Key consequences:

- For a **completed item** whose entire journey falls within [0, T]: residence time equals sojourn time.
- For a **completed item** that started before the window: residence time is shorter than sojourn time (clipped at window start).
- For an **active item** (no end date): residence time grows linearly with T — the longer you observe, the more time the item accumulates.

This distinction — observer-window-relative vs item-relative — is the single most important concept in sample path analysis.

### The Sample Path N(t)

The instantaneous count of active items at each point in time t:

```
N(t) = count of items where s_i <= t AND (e_i > t OR e_i is nil)
```

This is a step function that increments when items arrive (cross the commitment point) and decrements when items depart (reach resolution). It is the raw material from which everything else is derived.

## Mathematical Foundation

All quantities are derived from a single underlying measure: **H(T)**, the total accumulated element-time in the observation window.

### H(T) — Cumulative Element-Time

```
H(T) = integral from 0 to T of N(t) dt
```

In discrete daily terms: `H(T) = sum of N(t) over all days in [0, T]`.

H(T) can equivalently be computed as the sum of all individual residence times:

```
H(T) = sum of r_i(T) for all items i
```

This equivalence — integral of the count function equals sum of individual durations — is the foundation that makes the finite identity work. It is not an approximation; it is an exact equality by construction.

### Derived Quantities

From H(T) and the arrival count A(T), all other quantities follow:

| Symbol | Name | Formula | Meaning |
|--------|------|---------|---------|
| **L(T)** | Time-average WIP | H(T) / T | Average number of items present over the window |
| **A(T)** | Cumulative arrivals | Count of items with s_i <= T | Total items that entered during the window |
| **Λ(T)** | Arrival rate | A(T) / T | Items per unit time |
| **w(T)** | Average residence time | H(T) / A(T) | Average element-time per arriving item |
| **D(T)** | Cumulative departures | Count of items with e_i <= T | Total items that completed during the window |
| **W*(T)** | Average sojourn time | Sum of sojourn times / D(T) | Average complete duration for finished items |

## The Finite Little's Law Identity

```
L(T) = Λ(T) · w(T)
```

Expanding:

```
L(T) = H(T) / T
Λ(T) · w(T) = (A(T) / T) · (H(T) / A(T)) = H(T) / T
```

This identity holds **unconditionally** — for any finite window, any arrival pattern, any number of active items, with or without equilibrium. It is a mathematical tautology: both sides reduce to H(T)/T. There are no assumptions to violate.

This is fundamentally different from the classical steady-state version `L = λ · W`, which requires:

- Long-run averages to exist and be finite (convergence)
- The system to be in approximate equilibrium (stability)

The finite version trades generality for precision: it always holds, but it uses residence time w(T) rather than sojourn time W, and cumulative arrival rate Λ(T) rather than long-run arrival rate λ.

### Why the Identity Matters

Because it holds unconditionally, the identity serves two purposes:

1. **Computational verification**: If `|L(T) - Λ(T) · w(T)| > ε` at any point, there is a bug in the computation. This is a built-in consistency check.

2. **Causal decomposition**: When L(T) changes, you can determine whether the cause was a change in arrival rate Λ(T), average residence time w(T), or both. This decomposition is exact, not approximate.

## The Coherence Gap

The **coherence gap** is defined as:

```
coherence_gap(T) = w(T) - W*(T)
```

where w(T) is the average residence time (all items, including active) and W*(T) is the average sojourn time (completed items only).

### What the Gap Reveals

The coherence gap quantifies the **end effect** — the impact of still-active items on the system's average residence time:

- **Gap near zero**: Active items' ages are comparable to completed items' sojourn times. The system is behaving consistently — items in progress are aging at a rate consistent with historical completion times.

- **Large positive gap**: Active WIP is significantly inflating the average residence time beyond what completed items experienced. This signals either: (a) items are aging without completing (stagnation), (b) recent arrivals have not yet had time to complete (transient effect), or (c) the system has structurally changed and old completion rates no longer apply.

- **Negative gap**: Rare in practice, would indicate completed items had unusually long sojourn times relative to current active items.

### Coherence

When the coherence gap is small and stable, the flow metrics are **coherent** — residence time and sojourn time tell the same story. This is the condition under which the classical `L = λW` approximation becomes reliable.

When the gap is large or growing, the metrics are **incoherent** — the system's behavior cannot be summarized by completed-item statistics alone. The sample path analysis reveals dynamics that cycle time analysis misses.

## Convergence

**Convergence** asks: as the observation window T grows, do the cumulative averages approach stable limits?

- If Λ(T) approaches λ and w(T) approaches W as T grows, then the finite identity L(T) = Λ(T) · w(T) approaches the classical L = λW.
- If these limits do not exist (e.g., arrival rate is accelerating, or active items cause w(T) to grow unboundedly), the system is **divergent** and the classical form does not apply.

### Convergence vs Stability

Kumar draws an important distinction: convergence means long-run averages exist and are finite; stability requires these averages to remain constant over time. Software delivery systems are often **meta-stable** — they obey Little's Law locally but with parameters that drift. The system converges in any given observation window, but the converged values shift from window to window.

### Convergence Assessment

The implementation assesses convergence by examining the trend of the coherence gap over the final quarter of the observation window:

- **Converging**: The gap is shrinking — active items are completing and the system is approaching equilibrium.
- **Metastable**: The gap is roughly constant — the system is in a steady state (possibly with persistent active items).
- **Diverging**: The gap is growing — active items are accumulating faster than they are being resolved.

## Pre-Window Items and End Effects

Items that were committed before the observation window begins but are still active during it require special handling:

1. **Included in N(t)**: Pre-window items contribute to the instantaneous count if they are active during the window. This is correct — they are physically present in the system.

2. **Excluded from A(T)**: Pre-window items are not counted as arrivals within the window. Their commitment occurred before observation began.

3. **Contribute to H(T)**: Because they appear in N(t), their presence accumulates element-time in H(T). Their residence time within the window is clipped: `r_i(T) = min(e_i, T) - 0` (from window start to departure or window end).

4. **Effect on w(T)**: Because H(T) includes pre-window contributions but A(T) does not count pre-window items, the ratio `w(T) = H(T) / A(T)` is "inflated" relative to a pure in-window calculation. This inflation is **intentional** — it captures the burden that legacy items place on the system. The finite identity still holds exactly because it is a tautology: `Λ(T) · w(T) = (A/T) · (H/A) = H/T = L(T)`.

This is the "end effect" that Kumar describes: partial items at the boundaries of the observation window affect the averages in ways that pure sojourn-time analysis cannot capture. As Kumar notes, "estimation errors cancel out over any finite window of time" — the residence-time-based identity remains accurate despite these boundary effects.

## Relationship to Existing Tools

The sample path analysis complements rather than replaces existing MCS-MCP tools:

| Existing Tool | What It Measures | Sample Path Equivalent |
|---|---|---|
| `analyze_cycle_time` | Sojourn times of completed items | W*(T) — the empirical sojourn average |
| `analyze_work_item_age` | Ages of currently active items | The "end effect" term — total active age contributing to the coherence gap |
| `analyze_wip_stability` | N(t) over time via XmR | The raw sample path N(t), but without the time-averaging or identity decomposition |
| `analyze_flow_debt` | Arrival vs departure rates | Related to Λ(T) vs departure rate, but in windowed buckets rather than cumulative |
| `analyze_throughput` | Weekly completion counts | The departure process, weekly aggregated |

The sample path tool provides the **unified view** that ties these together through the finite identity, enabling the causal decomposition that no single existing tool can provide. When L(T) increases, you can determine whether arrivals accelerated (Λ rose), items are staying longer (w rose), or both.

## Implementation Notes

### Backflow Reset (Always-On)

The implementation always uses the **last** commitment date as the start anchor s_i. When an item crosses the commitment boundary, flows back to an upstream status, and then crosses the boundary again, only the final crossing is used.

This diverges from the configurable `commitmentBackflowReset` flag used by other tools like `analyze_work_item_age`. The rationale: for sample path analysis, backflow reset is mathematically necessary to avoid double-counting commitment events for the same item.

### Commitment Boundary Detection

The start anchor s_i is determined by walking the item's transition history forward and recording the **last** transition where:

- The origin status has weight below the commitment point weight
- The destination status has weight at or above the commitment point weight

If no such transition exists but the item is currently in a Downstream or Finished tier, the item's Created date is used as a fallback.

Items that never reached the commitment point are excluded entirely.

### Discovery Cutoff

The observation window is clamped by the discovery cutoff — a temporal boundary marking where "steady state" begins in the dataset (the timestamp of the 5th delivery). This prevents analyzing periods where the system had not yet demonstrated delivery capacity, which would produce misleading metrics showing apparent underutilization.

### Identity Verification

At every point in the time series, the implementation verifies `|L(T) - Λ(T) · w(T)| < ε` (with ε = 10^-9). If this check fails, it indicates a computation bug, not a theoretical violation — the identity holds by construction.

## References

- Stidham, S. (1972). "L = λW: A Discounted Analogue and a New Proof." *Operations Research*.
- El-Taha, M. & Stidham, S. (1999). *Sample-Path Analysis of Queueing Systems*. Springer.
- Kumar, K. (2025). "[What is Residence Time.](https://www.polaris-flow-dispatch.com/p/what-is-residence-time)" *The Polaris Flow Dispatch*.
- Kumar, K. (2025). "[The Many Faces of Little's Law.](https://www.polaris-flow-dispatch.com/p/the-many-faces-of-littles-law)" *The Polaris Flow Dispatch*.
