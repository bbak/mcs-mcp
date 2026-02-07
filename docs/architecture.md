# MCS-MCP Architecture & Operational Manual

This document provides a comprehensive overview of the MCS-MCP (Monte-Carlo Simulation Model Context Protocol) server. It is designed to serve as both a high-level conceptual map and a technical reference for AI agents.

---

## 1. Operational Flow (The Interaction Model)

To achieve reliable forecasts, the interaction with MCS-MCP follows a specific analytical sequence.

```mermaid
graph TD
    A["<b>1. Identification</b><br/>find_jira_boards"] --> B["<b>2. Context Anchoring</b><br/>get_board_details"]
    B --> C["<b>3. Semantic Mapping</b><br/>get_workflow_discovery"]
    C --> D["<b>4. Planning</b><br/>get_diagnostic_roadmap"]
    D --> E["<b>5. Forecast & Diagnostics</b><br/>run_simulation / get_aging_analysis"]
```

1.  **Identification**: Use `find_jira_projects/boards` to locate the target.
2.  **Context Anchoring**: `get_board_details` performs an **Eager Fetch** of history and stabilizes the project context via the **Data Shape Anchor**.
3.  **Semantic Mapping**: `get_workflow_discovery` uses **Data Archeology** to propose logical process tiers (Demand, Upstream, Downstream, Finished). **AI agents must verify this mapping before proceeding.**
4.  **Planning**: `get_diagnostic_roadmap` recommends a sequence of tools based on the user's goal (e.g., forecasting, bottleneck analysis).
5.  **Analytics**: High-fidelity diagnostics (Aging, Stability, Simulation) are performed against confirmed tiers.

---

## 2. Core analytical Principles: "Fact-Based Archeology"

MCS-MCP rejects reliance on often-misconfigured Jira metadata (like `statusCategory`). Instead, it infers process reality from objective transition logs.

### 2.1 The 4-Tier Meta-Workflow Model

Every status is mapped to a logical process layer to ensure specialized clock behavior:

| Tier           | Meaning                          | Clock Behavior                                        |
| :------------- | :------------------------------- | :---------------------------------------------------- |
| **Demand**     | Unrefined entry point (Backlog). | Clock pending.                                        |
| **Upstream**   | Analysis/Refinement.             | Active clock (Discovery).                             |
| **Downstream** | Actual Implementation (WIP).     | Active clock (Execution).                             |
| **Finished**   | Terminal exit point.             | **Clock Stops**. Duration becomes fixed "Cycle Time". |

### 2.2 Discovery Heuristics

- **Birth Status**: The earliest entry point identifies the system's primary source of demand.
- **Terminal Sinks**: Statuses with high entry-vs-exit ratios identify logical completion points even if Jira resolutions are missing.
- **Backbone Order**: The "Happy Path" is derived from the most frequent sequence of transitions (Market-Share confidence > 15%).
- **Unified Regex Stemming**: Automatically links paired statuses (e.g., "Ready for QA" and "In QA") via semantic cores.

---

## 3. Workflow Outcome Alignment (Throughput Integrity)

The server distinguishes **how** and **where** work exits the process to ensure throughput accurately reflects value-providing capacity.

### 3.1 Outcome Classification

Once an item reaches the **Finished** tier, it is classified into semantic outcomes:

- **Outcome: Delivered**: Items with a resolution or status outcome mapped as value-providing (e.g., "Fixed", "Done"). **Only these are used for Throughput and Simulation.**
- **Outcome: Abandoned**: Items mapped as waste (e.g., "Won't Do", "Discarded"). These are excluded from delivery metrics but vital for **Yield Analysis**.

### 3.2 Detection Methodology

- **Resolution Mapping (Primary)**: The system prioritizes the explicit Jira `resolution` field.
- **Status Mapping (Fallback)**: If resolution data is missing, it falls back to the status-level outcome mapping.
- **Gold Standard Benchmark**: This precedence is verified against industry benchmarks (**Nave**) and must be maintained for statistical integrity.

### 3.3 Yield Analysis

The server calculates the "Yield Rate" by attributing abandonment to specific tiers:

- **Explicit Attribution**: Uses outcome suffixes (e.g., `abandoned_upstream`).
- **Heuristic Attribution**: Backtracks to the last active status if the outcome is generically `abandoned`.

---

## 4. High-Fidelity Simulation Engine

MCS-MCP uses a **Hybrid Simulation Model** that integrates historical capability with current reality.

### 3.1 Three Layers of Accuracy

1.  **Statistical Capability**: Builds a throughput distribution using **Delivered-Only** outcomes from a sliding window (default: 180 days).
2.  **Current Reality (WIP Analysis)**: Explicitly analyzes the stability and age of in-flight work.
3.  **Demand Expansion**: Automatically models the "Invisible Friction" of background work (Bugs, Admin) based on historical type distribution.

### 3.2 Standardized Percentile Interpretation

To ensure consistency across simulations, aging, and persistence, the following standardized mapping is used:

| Naming           | Percentile | Meaning                                                 |
| :--------------- | :--------- | :------------------------------------------------------ |
| **Aggressive**   | P10        | Best-case outlier; "A miracle occurred."                |
| **Unlikely**     | P30        | Very optimistic; depends on everything going perfectly. |
| **Coin Toss**    | P50        | Median; 50/50 chance of being right or wrong.           |
| **Probable**     | P70        | Reasonable level of confidence; standard for planning.  |
| **Likely**       | P85        | High confidence; recommended for commitment.            |
| **Conservative** | P90        | Very cautious; accounts for significant friction.       |
| **Safe-bet**     | P95        | Extremely likely; includes heavy tail protection.       |
| **Limit**        | P98        | The practical upper bound of historical data.           |

### 4.3 Simulation Safeguards

To prevent nonsensical forecasts, the engine implements several integrity thresholds:

- **Throughput Collapse Barrier**: If the median simulation result exceeds 10 years, a `WARNING` is issued. This usually indicates that filters (`issue_types` or `resolutions`) have reduced the sample size so much that outliers dominate.
- **Resolution Density Check**: Monitors the ratio of "Delivered" items vs. "Dropped" items. If **Resolution Density < 20%**, a `CAUTION` flag is raised, warning that the throughput baseline may be unrepresentative.

---

## 5. Volatility & Predictability Metrics

The server provides statistical dispersion metrics to quantify process stability and risk.

### 5.1 Dispersion Metrics (The Spread)

- **IQR (Interquartile Range)**: P75 - P25. Measures the density of the middle 50%. Smaller = higher predictability.
- **Inner 80%**: P90 - P10. Shows the range where 80% of items fall, providing a robust view of the "middle" without extreme outlier noise.

### 5.2 Volatility Heuristics (The Risk)

| Metric                       | Stable Threshold | Indication of Failure                                                                                                    |
| :--------------------------- | :--------------- | :----------------------------------------------------------------------------------------------------------------------- |
| **Tail-to-Median (P85/P50)** | **<= 3.0**       | **Highly Volatile**: If > 3.0, high-confidence items take >3x the median, indicating heavy-tailed risk.                  |
| **Fat-Tail Ratio (P98/P50)** | **< 5.6**        | **Unstable**: Kanban University heuristic. If >= 5.6, extreme outliers control the process, making forecasts unreliable. |

---

## 6. Stability & Evolution (XmR)

Process Behavior Charts (XmR) assess whether the system is "in control."

- **XmR Individual Chart**: Detects outliers (points above Natural Process Limits) and shifts (8 consecutive points on one side).
- **Three-Way Tactical Audit**: Uses subgroup averages (weekly/monthly) to detect long-term strategic process drift.
- **WIP Age Monitoring**: Compares current WIP against historical limits to provide early warnings of a "Clogged" system.

---

## 7. Internal Mechanics (The Event-Sourced Engine)

### 6.1 Staged Ingestion & Persistent Cache

- **Event-Sourced Architecture**: The system maintains an immutable, chronological log of atomic events (`Change`, `Created`, `Unresolved`).
- **Two-Stage Hydration**:
    - **Stage 1 (Recent Updates)**: Fetches the last 1000 items sorted by `updated DESC`.
    - **Stage 2 (Baseline Depth)**: Explicitly fetches resolved items to ensure a minimum baseline (default 200 items).
- **Cache Integrity**:
    - **2-Month Rule**: If the latest cached event is > 2 months old, the system performs a full re-ingestion to clear potential "ghost" items (moved/deleted).
    - **24-Month Horizon**: Initial hydration is bounded to 24 months.
    - **8-Page Cap**: Ingestion is capped at 2400 items to prevent memory exhaustion in legacy projects.
- **Dynamic Discovery Cutoff**: Automatically calculates a "Warmup Period" (Dynamic Discovery Cutoff) to exclude noisy bootstrapping periods from analysis.

### 6.2 Discovery Sampling Rules

To ensures discovery reflect the **active process**, the system applies recency bias:

- **Priority Window**: Items created within the last **365 days** are prioritized.
- **Adaptive Fallback**: Expands to 2 or 3 years only if the priority window has < 100 items. Items older than 3 years are strictly excluded from discovery.

### 6.3 Move History Healing

When items move between projects, the system implements **History Healing**:

- **Process Boundary**: Deters noise from the old project workflow.
- **Synthetic Birth**: Re-anchors the item at its original creation date but in the context of the new project's arrival status, preserving **Lead Time** while cleaning process metrics.

### 6.4 Technical Precision

- **Microsecond Sequencing**: Changlogs are processed with integer microsecond precision for deterministic ordering.
- **Nave-Aligned Residency**: Residency tracking uses exact seconds (`int64`), converted to days only at the reporting boundary (`Days = seconds / 86400`).
- **Zero-Day Safeguard**: Current aging metrics are rounded up to the nearest 0.1 to avoid misleading "0.0 days" results.

---

## 8. System Safety & Development

- **Safety Brake**: Heavy analytical queries are throttled to protect Jira load.
- **Burst Mode**: Metadata discovery bypasses throttles for high-performance agent interaction.
- **Observability**: Structured JSON logging via **zerolog**. Internal guidance (`_guidance`) is strictly separated from data-driven `warnings`.
- **Anti-Hallucination**: Agents are strictly forbidden from "guessing" forecasts if tools return insufficient data.

---

## 9. Codebase Overview

- `internal/jira`: API transport and domain models.
- `internal/eventlog`: Persistence and "Signal-Aware" projections.
- `internal/stats`: Core math (Control charts, Aging, Yield, Simulations).
- `internal/mcp`: Tool definitions and task orchestration.

---
