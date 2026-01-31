# Project Charter: MCS-MCP (Monte-Carlo Simulation Model Context Protocol)

## 1. Project Overview

MCS-MCP is a Model Context Protocol (MCP) server that provides AI agents with sophisticated forecasting and diagnostic capabilities for software delivery projects. It specializes in Monte-Carlo Simulations (MCS) using historical Jira data to provide probabilistic delivery dates, bottleneck identification, and flow stability analysis.

## 2. Core Architectural Principles

### Data-Driven Probabilism

- **No Averages**: The system rejects single-point averages in favor of ranges and percentiles.
- **Monte-Carlo Focus**: All forecasts are derived from 10,000+ random trials using historical throughput distributions.
- **Simulation Resilience (Safety Brakes)**: To ensure system availability, simulations implement safety brakes (e.g., 20,000 day cap) and early-exit warnings when historical throughput is zero, preventing infinite loops or resource exhaustion in sparse data scenarios.

### Operational Flow (The Interaction Model)

```mermaid
graph TD
    A["<b>1. Goal Identification</b><br/>get_diagnostic_roadmap"] --> B["<b>2. Discovery</b><br/>get_data_metadata"]
    B --> C["<b>3. Semantic Mapping</b><br/>get_workflow_discovery"]
    C --> D["<b>4. Selection</b><br/>User choosing Commitment Point"]
    D --> E["<b>5. Forecast & Diagnostics</b><br/>run_simulation / get_aging_analysis"]
```

1.  **Guidance Phase**: The AI uses `get_diagnostic_roadmap` to align with the user's goal (e.g., "forecasting", "bottlenecks"). This provides the recommended sequence of tools.
2.  **Discovery Phase**: The `find_jira_projects` and `find_jira_boards` tools are used to locate the target data source. Once identified, `get_data_metadata` is used to probe the source for scale and quality.
3.  **Semantic Mapping Phase**: The `get_workflow_discovery` tool identifies the residence time and Jira categories. The user/AI maps statuses to **Meta-Workflow Tiers** (Demand, Upstream, Downstream) and **Functional Roles** (Active, Queue, Ignore) using `set_workflow_mapping`.
4.  **Selection Phase**: Based on the discovery results, the user confirms the **Commitment Point**. If not explicitly confirmed, the system defaults to the first status in the **Downstream** tier to ensure continuity of analysis.
5.  **Analytics Phase**:
    - Use `run_simulation` for aggregate/bulk forecasts.
    - Use `get_aging_analysis` and `get_status_persistence` for process health.
    - Diagnostic tools respect the semantic tiers and roles to avoid misinterpreting the backlog or discovery phases as bottlenecks.

### Analytical Roadmap (AI Guidance)

To ensure the AI Agent selects the most reliable path for complex goals, the server provides a `get_diagnostic_roadmap` tool. This tool acts as an "Analytical Orchestrator," recommending a specific order of diagnostic steps:

- **Goal-Oriented**: Provides tailored workflows for `forecasting`, `bottlenecks`, `capacity_planning`, and `system_health`.
- **Prerequisite Enforcement**: Explicitly guides the AI to perform foundational steps (like stability checks or workflow verification) before running high-level simulations.

### Mandatory Workflow Verification (Inform & Veto)

To ensure conceptual integrity, the AI **must never assume** the semantic tiers or roles of a project. Before providing process diagnostics, the following loop is required:

1.  **AI Proposes**: Use `get_workflow_discovery` to present an inferred mapping. The server utilizes pattern matching (e.g., "Ready for X" → queue role)
    - **Inform & Veto**: Diagnostics (Simulation, Aging, Stability) require a verified Commitment Point. If unavailable, the tool uses a **Safe Default** (the first Downstream status) and clearly notifies the user in the results:
        - Patterns: Automatic "queue" role for "Ready for X" or "-ed" statuses if an active counterpart exists.
        - Residency: P50 spikes often indicate queuing or commitment boundaries.
        - User Approval: AI proposes the mapping; user confirms/refines via `set_workflow_mapping`.
    - **API Strategy**: The server intentionally avoids the Jira Board Configuration API (`/rest/agile/1.0/board/{id}/configuration`) for several reasons:
        - **Deprecation Risk**: The endpoint is not present in Jira REST API versions 2.0 and 3.0, indicating it may be removed or replaced.
        - **Structural Mismatch**: Board columns often group multiple statuses or use names that diverge from the underlying workflow, which would require complex resolution logic (like the `/status` endpoint) to maintain cohesion.
        - **Cognitive Load**: Mapping board-specific visualization metadata to universal process tiers might introduce unnecessary complexity and potentially confuse AI agents during analysis.

---

### Status-Granular Flow Model

The server employs high-fidelity residency tracking. Instead of calculating a single duration window, it parses the full Jira changelog to determine the **exact days** spent in every workflow step. This enables:

- **Range-based Metrics**: Subdividing cycle time (e.g., "Ready to Test" vs "UAT and Deploy").
- **Accurate Persistence**: Summing multiple sessions in the same status for "ping-pong" tickets.
- **Workflow Decoupling**: Commitment and Resolution points can be shifted dynamically without re-ingesting data.

---

### Workflow Semantic Tiers & Roles

To prevent the AI from misinterpreting administrative or storage stages as process bottlenecks, statuses are mapped to **Meta-Workflow Tiers** and specific **Roles**.

#### 1. Meta-Workflow Tiers

Every status belongs to one of four logical process layers:

| Tier           | Meaning                                                      | Commitment Insight                                                                          | Clock Behavior                                                                 |
| :------------- | :----------------------------------------------------------- | :------------------------------------------------------------------------------------------ | :----------------------------------------------------------------------------- |
| **Demand**     | Initial entry point to the system (e.g., "Backlog", "Open"). | Items here are uncommitted and unrefined. Identifies the primary "Source of Demand".        | Clock is pending; residency is stored but doesn't contribute to WIP.           |
| **Upstream**   | Analysis and refinement (e.g., "Refining").                  | Clock is running on "Discovery"; items in "To Do" category but NOT the primary entry point. | Active clock.                                                                  |
| **Downstream** | Actual implementation (e.g., "In Dev", "UAT").               | The primary process flow; where implementation capacity is consumed.                        | Active clock.                                                                  |
| **Finished**   | Items that have exited the process.                          | Terminal stage; used for throughput.                                                        | **Clock Stops**: Pin residency at entry point. Age becomes fixed "Cycle Time". |

#### 2. Functional Roles

Within these tiers, statuses can be further tagged:

| Role       | Meaning                           | Impact on Analytics                                          |
| :--------- | :-------------------------------- | :----------------------------------------------------------- |
| **Active** | Primary working stage.            | High residence here indicates a process bottleneck.          |
| **Queue**  | Passive waiting stage (Hand-off). | Persistence is flagged as "Flow Debt" or "Waiting Waste".    |
| **Ignore** | Administrative stage.             | Resident time is excluded from core cycle time calculations. |

#### 3. Abandonment & Outcome

The server distinguishes **how** and **where** work exits the process through **Workflow Outcome Calibration**. Because Jira workflows are often inconsistent, the server employs a dual-signal methodology:

- **The "Finished" Signal**: Detection of the terminal state.
    - **Primary**: Occurrence of a `resolutiondate` (The "Stop the Clock" event).
    - **Secondary**: Reaching a status mapped to the **Finished** tier.
- **Outcome Classification**: Once finished, items are classified into semantic outcomes:
    - **Outcome: Delivered**: Item reached "Finished" with a resolution or status outcome mapped as value-providing (e.g., "Fixed", "Done"). This is the only population used for **Throughput** and **Simulation**.
    - **Outcome: Abandoned**: Item reached "Finished" with a resolution or status outcome mapped as waste (e.g., "Won't Do", "Discarded").
- **Yield Analysis**: The server calculates the "Yield Rate" by attributing abandonment to specific workflow tiers:
    - **Explicit Attribution**: Uses outcome suffixes (e.g., `abandoned_upstream`, `abandoned_downstream`) defined in the calibration layer.
    - **Heuristic Attribution**: Falls back to backtracking the last status before entering terminal state if the outcome is generically marked as `abandoned`.

---

### Standardized Percentile Mapping

To ensure consistency and help non-statistical users interpret results, the server uses a standardized mapping of percentiles to "Human-Language" names across all tools (Simulations, Inventory Aging, Persistence).

| Naming           | Statistical Percentile | Meaning                                                 |
| :--------------- | :--------------------- | :------------------------------------------------------ |
| **Aggressive**   | P10                    | Best-case outlier; "A miracle occurred."                |
| **Unlikely**     | P30                    | Very optimistic; depends on everything going perfectly. |
| **Coin Toss**    | P50                    | Median; 50/50 chance of being right or wrong.           |
| **Probable**     | P70                    | Reasonable level of confidence; standard for planning.  |
| **Likely**       | P85                    | High confidence; recommended for commitment.            |
| **Conservative** | P90                    | Very cautious; accounts for significant friction.       |
| **Safe-bet**     | P95                    | Extremely likely; includes heavy tail protection.       |
| **Limit**        | P98                    | The practical upper bound of historical data.           |

## 3. Technology Stack

- **Language**: Go (Golang)
- **Primary Algorithm**: Monte-Carlo Simulation (MCS)
- **Data Source**: Jira Software (Data Center or Cloud)
- **Communication**: Model Context Protocol (Standard)

## 4. Aging Math & Precision

To ensure conceptual integrity and transparency, the server adheres to a strict definition of "Age" and employs high-precision integer math for residency tracking.

#### 1. Precision & Storage

- **Internal Resolution**: The server parses Jira's changelog and calculates residence time for every status in **Seconds** (`int64`).
- **Serialization**: Integer seconds are used for all caching and cross-process communication to avoid floating-point "noise" and serialization errors.
- **Conversion**: Conversion to "Days" only occurs at the analytical or reporting boundary: `Days = float64(Seconds) / 86400.0`.

#### 2. Aging Definitions

The server distinguishes between two types of duration:

| Term           | Strict Definition                                                            | Usage                                                                           |
| :------------- | :--------------------------------------------------------------------------- | :------------------------------------------------------------------------------ |
| **Status Age** | The time passed since the item entered its **current** workflow step.        | Bottleneck identification (Active/In-flight). Fixed at 0.0 for terminal items.  |
| **WIP Age**    | The time passed since the item crossed the **Commitment Point** (started).   | Forecast reliability & stability analysis. Only applies to Upstream/Downstream. |
| **Cycle Time** | The **pinned duration** of an item from commitment to the **Finished** tier. | Historical baseline & capability analysis. Represents "Finished Age".           |
| **Total Age**  | The time passed since the item was created in Jira.                          | Inventory hygiene. Pins at entry to "Finished" tier.                            |

#### 3. Rounding & The "Zero-Day" Safeguard

To avoid confusing users with "0.0 days" (for items visited on the same day) and to ensure a clean UI without sacrificing simulation precision, the following logic is applied:

- **Reporting Precision**: All day-based metrics in tool outputs are rounded to **1 decimal place**.
- **The "Round-Up" Rule**: For current aging metrics (`StatusAge`, `WIPAge`), the server applies a ceiling-based rounding:
  $$Age_{Reported} = \frac{\lceil Age_{Float} \times 10 \rceil}{10}$$
- **Result**: Any item that has actually transitioned into a status will show at least **0.1 days**, never 0.0, while still allowing for fractional accuracy (e.g., 1.2 days).

#### 4. Existence of WIP Age

- An item strictly **does not have** a WIP Age before it crosses the commitment point.
- The server reports WIP Age as `null/nil` for items in the **Demand** tier or items that haven't transitioned into an **Active/Started** status yet.

#### 5. Backflow Policy

The system employs a strict "Restart on Backflow" policy for items returning to the **Demand** tier:

- **Reset**: If an item that has previously crossed the commitment point is moved back into a status mapped to the **Demand** tier, it is treated as a "Reset".
- **History Consolidation**: Instead of wiping history or resetting the **Created** date, the system consolidates all time spent prior to the most recent backflow into the **Demand** tier. This preserves the original **Total Age** while ensuring **WIP Age** reflects only the most recent start.
- **Fresh Start**: The item will only regain a WIP Age if and when it crosses the commitment point **again**. The new WIP Age will be calculated from this most recent crossing.
- **Rationale**: This prevents "stale WIP" metrics from being skewed by failed starts, while accurately reflecting that the item has been "known" (Total Age) since its true creation.

#### 6. Project Move Boundary

To ensure conceptual integrity in cross-project environments (e.g., items moving from "Strategy" projects to "Delivery" projects), the system implements a process boundary for project moves:

- **Move Detection**: If an item's history shows a change in the `Key` or `project` fields, the system treats the latest move date as a **Process Boundary**.
- **Contextual Reset**: All `StatusResidency` and `Transitions` that occurred _under a different project key_ are discarded during analysis. This ensures that Workflow Discovery and process diagnostics accurately reflect only the current project's flow.
- **Tier Discovery Integrity**: Moved items are **discarded during 'Demand' tier discovery**. This prevents mid-process entry points (transferring an item into "Developing" in the new project) from being mis-detected as a system-wide source of demand.
- **Lead Time Preservation**: Critically, the original `Created` date is preserved. High-level metrics like **Total Age** remain valid, reflecting the item's entire lifecycle from original idea to delivery, while low-level process metrics (WIP Age, Status Persistence) are cleaned of "ghost" statuses from older projects.

#### 7. Terminal Pinning Policy (Stop the Clock)

To prevent archive data from skewing delivery metrics, the system implements a "Stop the Clock" policy for terminal statuses:

- **Pinned Residency**: When an item enters a status mapped to the **Finished** tier, the residency calculation for that status (and the total process age) is pinned to the point of entry (or the resolution date if available).
- **Cycle Time vs Aging**: Items in terminal statuses cease to "age". Their calculated duration is treated as a fixed **Cycle Time**.
- **WIP Exclusion**: Diagnostic tools like `get_aging_analysis` can explicitly filter out "Finished" items to ensure the focus remains on the active inventory (WIP).
- **Data Integrity**: This policy ensures that items delivered 6 months ago don't show an "Age" of 180 days in aging reports; they show the exact number of days they took to complete.

#### 7. History Fallback Policy

In cases where Jira data is incomplete (e.g., resolved items with missing or archived changelogs), the system applies a "Best Effort" residency model via the **Transformer**:

- **Birth Status Alignment**: The system automatically identifies the functional land-status (e.g., 'Open') for the creation event by analyzing the earliest available transition.
- **Residency Assumption**: If an item is resolved but has no transition history, the system assumes it spent its total duration in its birth status.
- **Analytical Inclusion**: This ensures these items are still included in throughput and total aging metrics, preserving the statistical volume of the dataset despite local data gaps.

---

### Volatility & Predictability Metrics

The server provides advanced statistical dispersion metrics to quantify the "stability" and "risk" of a process.

#### 1. Dispersion Metrics (The Spread)

| Metric        | Calculation | Meaning                                                                                                                       |
| :------------ | :---------- | :---------------------------------------------------------------------------------------------------------------------------- |
| **IQR**       | P75 - P25   | **Interquartile Range**: The density of the middle 50% of the data. Smaller IQR = higher predictability.                      |
| **Inner 80%** | P90 - P10   | **Robust Spread**: Shows the range where 80% of items fall. More robust than standard deviation for non-normal distributions. |

#### 2. Volatility Metrics (The Risk)

To identify process instability and the presence of extreme outliers (Fat-Tails), the server implements two key heuristics:

| Metric                   | Calculation | Stable Threshold | Indication of Failure                                                                                                                                      |
| :----------------------- | :---------- | :--------------- | :--------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Tail-to-Median Ratio** | P85 / P50   | **<= 3.0**       | **Highly Volatile**: If > 3.0, items in the high-confidence range (P85) take more than 3x the median time, indicating a heavy-tailed risk.                 |
| **Fat-Tail Ratio**       | P98 / P50   | **< 5.6**        | **Unstable / Out-of-Control**: Kanban University heuristic. If >= 5.6, extreme outliers are in control of the process, making forecasts highly unreliable. |

### 4.5. Process Stability & Evolution (XmR)

While Monte-Carlo simulations provide forecasts, Process Behavior Charts (XmR) assess the **validity** of those forecasts by identifying "Special Cause" variation.

#### The XmR Engine (Individual Chart)

The system employs Wheeler's XmR math (Individuals and Moving Range) to distinguish between:

- **Common Cause Variation (Noise)**: Inherent jitter within the Natural Process Limits (Avg +/- 2.66 \* AmR).
- **Special Cause Variation (Signal)**: Outliers (Rule 1) or Process Shifts (Rule 2: 8 consecutive points on one side).

#### Three-Way Process Behavior Charts

For longitudinal analysis (the "Strategic Audit"), the system uses Three-Way Charts:

1.  **Baseline Chart**: Monitors individual jitter.
2.  **Average Chart (The Third Way)**: Uses the moving range of _subgroup averages_ (e.g., Monthly averages) to detect long-term process drift or migration.

#### Integrated Time Analysis

A unique feature of the system is the integration of Done vs. WIP populations. Current **WIP Age** is monitored against the historical **UNPL** of resolved items, providing a proactive signal of instability _before_ the work is completed.

## 5. Event-Sourced Architecture & Staged Ingestion

To enable high-fidelity metrics and progressive data loading, the system utilizes an **Event-Sourced Architecture**. Instead of treating Jira issues as static snapshots, the server maintains a chronological log of atomic events for every work item.

#### The Event-Sourced Pipeline

```mermaid
graph TD
    A[JQL/SourceID] --> B[LogProvider]
    B --> C{Staged Ingestion}
    C -->|Stage 1: Probe| D[EventStore]
    C -->|Stage 2: WIP| D
    C -->|Stage 3: Baseline| D
    D --> E[Projections]
    E --> F[Domain Issues]
    F --> G[Analysis Tools]
```

1.  **LogProvider**: Orchestrates the data flow. It ensures that the required level of data detail is available in the local log before analysis begins.
2.  **Staged Ingestion**:
    - **Stage 1: Probe**: Fetches a small sample (50 items) to enable `get_data_metadata` and `get_workflow_discovery`.
    - **Stage 2: WIP**: Fetches all active items (those not in the 'Finished' status category) to ensure accurate `get_aging_analysis`.
    - **Stage 3: Baseline**: Fetches a deep historical baseline (defaulting to 6 months) for forecasting and stability analysis.
3.  **EventStore**: A thread-safe, chronological repository of `IssueEvents`. It handles deduplication and strictly orders events by `Timestamp` and `SequenceID`.
4.  **Transformer**: Converts Jira's snapshot DTOs and changelogs into atomic events (`Created`, `Transitioned`, `Resolved`, `Moved`). It ensures "Birth Status Alignment" by identifying the functional status an item landed in from its first transition.
5.  **Projections**: Reconstructs domain logic (like `jira.Issue` or `ThroughputBuckets`) by "replaying" the event stream through specific lenses (e.g., `ReconstructIssue`, `WIPProjection`).

### 5.1. The Event Log as Source of Truth

The event log, partitioned by board ID, is the definitive source of truth for the server. This design provides:

- **Immutability**: Historical events are objective facts (e.g., "Item X moved to Dev at 10:00").
- **Persistence (Long-term Cache)**: The log is persisted to disk using **JSON Lines (JSONL)**, enabling fast reloads between sessions and reducing reliance on Jira APIs.
- **Analytical Flexibility**: Metrics like "Cycle Time" or "Commitment Point" are just interpretations of the log and can be adjusted (via `set_workflow_mapping`) without re-fetching data.
- **Progressive Consistency**: The system becomes more "knowledgeable" as stages are completed, but always operates on a consistent, deduplicated log.

### 5.2. File-Backed JSONL Cache

To ensure performance and reliability across sessions, the server implements a file-backed cache:

- **Format: JSONL**: Data is stored as newline-delimited JSON objects. This format supports streaming (memory efficiency) and is resilient to partial write failures.
- **Atomic Persistence**: Saving to disk utilizes a "write-to-tmp and rename" pattern to ensure that the cache file is never left in a corrupted state if the process is interrupted.
- **Content Integrity**: The system automatically handles deduplication and chronological sorting during the `Load` operation, ensuring the in-memory `EventStore` remains consistent even if the cache contains overlapping data.

### 5.3. Incremental Synchronization

To minimize latency, the system utilizes an **Incremental Fetch** strategy:

- **Latest Timestamp Detection**: Upon loading a board/filter, the server identifies the timestamp of the most recent event in the local cache.
- **Delta Retrieval**: If a recent cache exists, the server updates its JQL to fetch only items modified since that timestamp (`updated >= {latest_ts}`).
- **Paginated Recovery**: For high-churn environments or deep catch-ups, the server implements robust pagination, fetching data in configurable pages to stay within Jira's `maxResults` constraints while ensuring no events are missed.

### 5.4. Recency Bias & Analysis Windowing

To ensure that forecasts and workflow discovery reflect the **active process** rather than historical artifacts, the system applies a mandatory 6-month recency window:

- **Workflow Discovery**: The `get_workflow_discovery` tool automatically filters the source data to items updated within the last **180 days**. This prevents stale or decommissioned statuses from cluttering the discovery results.
- **Simulation Baseline**: Forecasting tools default to a 180-day historical window for throughput and cycle time distributions, ensuring the "Capability" of the team reflects their recent performance.

### 5.2. Search-Driven Inventory (Discovery Memory)

To ensure high-performance discovery and maintain consistency during project setup, the server implements a **Sliding Window Inventory**:

- **Backend-Assisted Search**:
    - **Project Discovery**: Utilizes the Jira `/projects/picker` endpoint for server-side fuzzy matching.
    - **Board Discovery**: Utilizes the Agile `/board?name={filter}` parameter for optimized filtering.
- **Local Consistency (Memory)**:
    - The server maintains a thread-safe local repository of the last **1000 discovered items** (Projects and Boards).
    - Results from active tool calls (Search, GetProject, GetBoard) are "upserted" into this inventory using a **Most-Recently-Used (MRU)** policy.
- **Search Delivery**:
    - Search tools (e.g., `find_jira_projects`) perform a hybrid delivery: fetching the top 30 most-relevant matches from Jira while simultaneously searching the entire 1000-item local inventory.
    - This ensures that items once discovered remain "top of mind" for the AI agent even as they shift outside Jira's immediate search results.

### 5.3. Chronological Processing (Residency Math)

By moving residency calculation out of the Jira client and into a dedicated `processor.go`, the system achieves:

- **Testability**: Analytics logic can be tested with mock DTOs without hitting a Jira server.
- **Flexibility**: Changes to backflow policies or aging precision can be re-applied to cached DTOs instantly.
- **Heterogeneous Support**: `Issue.ProjectKey` ensures accurate per-item logic even when a board spans multiple Jira projects.

---

## 6. Codebase Structure & Modularization

The codebase follows a high-cohesion design, with logic strictly separated by functional responsibility.

### `internal/jira` (The Transport Layer)

- `client.go`: Interface definitions and domain models (`Issue`, `SourceContext`).
- `dc_client.go`: Implementation of the Jira Data Center / Server REST API.
- `dto.go`: Public Data Transfer Objects for JSON unmarshalling.

### `internal/eventlog` (The Persistence & Projection Layer)

- `store.go`: Thread-safe, cross-source repository for chronological `IssueEvents`.
- `event.go`: Schema definitions for atomic event types and partitioning logic.
- `provider.go`: `LogProvider` implementation for orchestrating staged ingestion.
- `transformer.go`: Critical logic for converting Jira snapshots into atomic events.
- `projections.go`: Logic for reconstructing domain models (`WIP`, `Throughput`, `Issue`) from the log.

### `internal/stats` (The Analytical Engine)

- `processor.go`: Internal residency math and historical baseline utilities.
- `stability.go`: XmR charts, Three-Way Control Charts, and Stability Index heuristics.
- `analyzer.go`: Foundational data types (`MetadataSummary`, `StatusMetadata`) and probe metrics.
- `persistence.go`: Status residency distributions (P50, P85, etc.).
- `aging.go`: Implementations for WIP Aging (`InventoryAge`) and Status Aging.
- `yield.go`: Calculations for process yield and abandonment waste.
- `cadence.go`: Logic for aggregating delivery throughput over time.

### `internal/mcp` (The Glue Layer)

- `server.go`: The core MCP server, managing `LogProvider` and `EventStore`.
- `tools.go`: AI-discoverable definitions and descriptions for all tools.
- `handlers.go`: Internal shared logic, roadmap tools, and backflow policies.
- `handlers_forecasting.go`: Tools for simulations and cycle time assessments (Stage 3).
- `handlers_diagnostics.go`: Tools for aging, stability, progress yield, and items journeys (Stage 2).
- `handlers_discovery.go`: Tools for metadata probing and workflow detection (Stage 1).
- `context.go`: Unified analysis context and default commitment point resolution.
- `helpers.go`: General utility methods and type conversion.

---

## 7. Conceptual Integrity Constraints

- **Cohesion**: Each tool must focus on a single aspect of flow (Ingestion, Simulation, Diagnostic).
- **Coherence**: Logical flow from data ingestion to statistical analysis to forecasting.
- **Consistency**: Adherence to Go community standards and naming conventions.

## 8. Observability & Logging Policy

To ensure high traceability without overwhelming the production logs, mcs-mcp follows a tiered logging strategy using **zerolog**:

| Level     | Usage                                              | Contents                                                                 |
| :-------- | :------------------------------------------------- | :----------------------------------------------------------------------- |
| **Error** | Critical failures that block a tool request.       | Stack traces, Jira API errors, Panic recoveries.                         |
| **Warn**  | Statistical anomalies or non-blocking data issues. | Fat-tails, zero-throughput warnings, simulation safety brake activation. |
| **Info**  | High-level operational flow (The "What").          | Tool entry/exit, server startup, major analytical milestones.            |
| **Debug** | Detailed data and calculated values (The "Value"). | AI arguments, generated JQL, exact simulation percentiles, cache traces. |
| **Trace** | Extreme granularity for internal troubleshooting.  | Logic-level noise (e.g., individual sliding window cache extensions).    |

### Conceptual Integrity in Logging

- **No Multi-line logs**: All logs must be structured JSON to ensure compatibility with log aggregators and terminal consoles.
- **Value Traceability**: Any value sent back to the AI or fetched from Jira should be visible at the `Debug` level to enable post-mortem verification of AI reasoning.

### 8.1 Response Metadata Semantics

To prevent "Instruction Leakage" in user conversations, mcs-mcp strictly separates internal guidance from user-relevant alerts:

- **`_guidance`**: (Internal) Instructions for the AI Agent on how to reason about the returned data. This should NEVER be shown to the user.
- **`warnings`**: (External) Data-driven alerts (e.g., "Zero throughput", "Fat-tails") that indicate risks in the forecast or analysis. These SHOULD be interpreted and potentially shared with the user.

Example:

```json
{
  "_guidance": "High persistence in Demand tier is normal backlog behavior. Do not flag as a bottleneck.",
  "warnings": ["Extreme outlier detected in 'refining' - check issue PROJ-123."],
  "data": { ... }
}
```
