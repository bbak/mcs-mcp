# Interaction Scenarios and Use Cases

This document describes the primary interaction scenarios between the User (Project Manager), the AI (Assistant), and the MCS MCP Server. These use cases follow Alistair Cockburn's format to provide rigor and insight into the forecasting process.

---

## UC1: Forecast Completion Date (Backlog-based Duration)

**Goal:** Determine how long it will take to deliver a fixed scope/backlog (e.g., "When will these 50 stories be done?").

- **Primary Actor:** User (Project Manager)
- **Secondary Actors:** AI (Intermediary), MCP Server (Forecasting Engine), Jira (Data Source)
- **Trigger:** User asks a "When?" question regarding a specific number of items.
- **Main Success Scenario:**
    1.  User asks: "How long will it take to finish 50 Story items in Project X?"
    2.  AI calls `import_projects` and identifies "PROJX".
    3.  AI automatically calls `import_board_context` which anchors on the data shape and confirms volume (e.g., "500 totalIngestedIssues found").
    4.  AI calls `workflow_discover_mapping` to establish the semantic mapping.
    5.  AI identifies the goal and calls `guide_diagnostic_roadmap` (Forecasting).
    6.  AI calls `forecast_monte_carlo` with `mode: "duration"`.
    7.  MCP Server runs 10,000 Monte-Carlo trials using historical throughput and type distribution.
    8.  AI presents results using risk terminology: "There is a **Likely (85%)** probability that the work will be done by [Date]."
    9.  **Integrity Check**: The MCP Server surfaces a `_data_quality` block if the simulation window was clamped by the **Discovery Cutoff**. If individual item filters have collapsed the throughput, the server issues a **Throughput Collapse** warning (detected by >10 year duration).

---

## UC2: Forecast Delivery Volume (Time-based Scope)

**Goal:** Determine how many items (scope) can be delivered within a fixed timeframe (e.g., "What can we get done by end of Q1?").

- **Primary Actor:** User (Project Manager)
- **Trigger:** User asks: "How many items can we complete by March 31st?"
- **Main Success Scenario:**
    1.  User asks about scope for a fixed date.
    2.  AI validates process stability using `analyze_process_stability`.
    3.  AI calculates `target_days` and calls `forecast_monte_carlo` in `scope` mode.
    4.  AI presents the results (e.g., "With **Probable (70%)** confidence, you can deliver 45 items").

---

## UC3: Predict Individual Item Delivery (Cycle Time)

**Goal:** Get a probabilistic estimate for a single high-priority item.

- **Primary Actor:** User (Project Manager)
- **Trigger:** User asks: "When will issue PROJ-123 be done?"
- **Main Success Scenario:**
    1.  AI calls `analyze_cycle_time` (potentially filtered by issue type).
    2.  MCP Server utilizes the **Status-Granular Flow Model** to calculate historical lead times.
    3.  AI presents the Service Level Expectation (e.g., "85% of similar items are resolved within 5 days").
    4.  **Post-Resolution Logic**: If the item is already finished, the AI uses `analyze_work_item_age` with `tier_filter: "Finished"` to report its actual, fixed Cycle Time. If an item was previously finished but has recently backflowed into an active state, the system automatically detects this and reports its new, running **Status Age** and **WIP Age**.

---

## UC4: Process Stability Validation (Predictability Guardrail)

**Goal:** Assess if the historical data used for a forecast is "In Control" and thus predictable.

- **Primary Actor:** AI (Proactive) or User
- **Trigger:** AI is about to run a simulation or User asks "Is our process stable?"
- **Main Success Scenario:**
    1.  AI calls `analyze_process_stability`.
    2.  MCP Server performs **XmR analysis** (Individuals and Moving Range) on Cycle Time and Throughput.
    3.  AI identifies **Special Cause** signals (Outliers beyond UNPL or Process Shifts).
    4.  AI reports: "Your process is currently **Unstable**. We detected a 9-day shift in cycle time starting last week. Any forecast run now will have a higher margin of error until the process is brought back into control."

---

## UC4b: WIP Stability Validation (Little's Law Guardrail)

**Goal:** Assess if the system's "Population" (Work-In-Progress) is actively managed and stable over time.

- **Primary Actor:** AI (Proactive) or User
- **Trigger:** AI is confirming prerequisites for forecasting, or User asks "Is our WIP under control?"
- **Main Success Scenario:**
    1.  AI calls `analyze_wip_stability`.
    2.  MCP Server calculates a daily run chart of active WIP and computes Natural Process Limits derived strictly from weekly samples.
    3.  AI identifies periods where daily WIP breached the Upper Limit.
    4.  AI reports: "Your WIP is historically **Unstable**. There was an unmanaged WIP spike in October that violated Little's Law. Cycle times established during this period are mathematically unreliable for forward-looking forecasts."

---

## UC5: Strategic Process Evolution (The Strategic Audit)

**Goal:** Perform a longitudinal analysis of process behavior to detect long-term shifts or maturity changes.

- **Primary Actor:** User (Operations Lead / Coach)
- **Trigger:** Quarterly review or after a major "Way of Working" change.
- **Main Success Scenario:**
    1.  User asks: "How has our delivery performance evolved over the last 12 months?"
    2.  AI calls `analyze_process_evolution` with `window_months: 12`.
    3.  MCP Server calculates **Three-Way Control Charts** (Baseline and Average Chart).
    4.  AI detects a systemic "Migration" signal.
    5.  AI reports: "Your process has successfully **Migrated** to a new state of stability. Since June, the average cycle time has dropped from 15 to 10 days, and the 'Third Way' chart shows this change is a sustained systemic improvement, not just noise."
    6.  **Rational Subgrouping**: The analysis defaults to **Monthly Subgrouping** for strategic audits and automatically **excludes the current calendar month** from the Natural Process Limits calculation, ensuring that partial data doesn't skew the long-term capability baseline.

---

## UC6: Proactive WIP Aging Warning

**Goal:** Identify "Special Cause" items that are becoming outliers before they are finished.

- **Primary Actor:** AI (Autonomous)
- **Trigger:** AI monitors active WIP health.
- **Main Success Scenario:**
    1.  AI calls `analyze_process_stability`.
    2.  MCP Server calculates the **Stability Index** (WIP / Throughput / Avg Cycle Time).
    3.  AI identifies that the index is > 1.3, indicating the system is "clogged" (WIP is aging relative to delivery capacity).
    4.  AI uses `analyze_work_item_age` or `analyze_cycle_time` to identify specific items that have exceeded the 85% or 95% probability limits.
    5.  AI warns: "Your system is becoming **Clogged** (Stability Index 1.4). PROJ-456 is a notable outlier, exceeding the 98% probability limit of your historical process."

---

## UC7: Workflow Bottleneck Discovery

**Goal:** Identify which status in the workflow is causing the most delay (Persistence).

- **Primary Actor:** AI (Proactive) or User
- **Trigger:** User asks "Where are we stuck?"
- **Main Success Scenario:**
    1. AI calls `analyze_status_persistence`.
    2. MCP Server utilizes the **Status-Granular residency map** to identify the bottleneck.
    3. AI identifies the status with the highest **Safe-bet (P95)** age.
    4. AI reports: "Items typically spend **12 days (Likely)** in 'Peer Review', which is 4x longer than any other stage."

---

## UC8: System Pulse & Flow Stability

**Goal:** Detect if the team is delivering consistently or in erratic batches.

- **Primary Actor:** AI (Autonomous Analysis)
- **Trigger:** AI prepares a forecast and wants to validate the "Stability" assumption of MCS.
- **Main Success Scenario:**
    1. AI calls `analyze_throughput`.
    2. MCP Server returns weekly throughput counts.
    3. AI detects "Batching" (e.g., three weeks of 0, then one week of 20).
    4. AI warns: "Your delivery pulse is currently **Batch-based**. While the forecast says you'll finish by March, be aware that this assumes a massive delivery at the very end rather than a steady flow."

---

## UC9: Workflow Semantic Enrichment (The Mapping)

**Goal:** Provide the AI with the semantic context needed to distinguish real bottlenecks from administrative stages.

- **Primary Actor:** AI (Proactive)
- **Trigger:** AI sees high persistence in a "To Do" category status.
- **Main Success Scenario:**
    1. AI identifies a new board via `import_boards`.
    2. AI calls `import_board_context` to anchor on data metadata and volume.
    3. AI calls `workflow_discover_mapping` and notices status "Open" has high residency but is categorized as "To-Do".
    4. AI informs User: "I've mapped 'Open' as your **Backlog**. I will treat its high persistence as expected storage time unless you tell me otherwise."
    5. User confirms or vetos.
    6. AI calls `workflow_set_mapping` if changes are needed.

---

## UC10: Granular Journey Discovery

**Goal:** Understand the exact path and delays an individual item took through the workflow.

- **Primary Actor:** User or AI
- **Trigger:** Investigating a "Long Tail" outlier item.
- **Main Success Scenario:**
    1. AI calls `analyze_item_journey` for a specific issue key.
    2. MCP Server performs a **Cache Lookup** within the currently anchored board log before calling Jira for missing details.
    3. MCP Server utilizes the **Event Log** to reconstruct a chronological path with residency days for each step.
    4. AI identifies exactly which step caused the outlier behavior (e.g., "PROJ-123 took 40 days, but 35 of those were spent in 'Blocked' / 'Flagged'").

---

## UC11: Process Yield & Abandonment Analysis

**Goal:** Identify where value is being lost in the process and quantify the "Abandonment Rate" by tier.

- **Primary Actor:** AI (Autonomous)
- **Trigger:** AI reviews throughput trends or User asks "Why is our throughput dropping?"
- **Main Success Scenario:**
    1. AI calls diagnostic tools that filter for "Abandoned" outcomes.
    2. AI correlates abandonment points with the **Meta-Workflow Tiers** (Demand, Upstream, Downstream).
    3. AI reports: "In the last 90 days, your process yield was 65%.
        - 10% was abandoned from **Demand** (Standard Backlog grooming).
        - 20% was abandoned from **Upstream** (Healthy discovery discard).
        - **5% was abandoned from Downstream** (Wasteful implementation rework).
    4. AI identifies the cost of Downstream abandonment: "Items abandoned in 'Downstream' had an average age of 15 days, representing significant wasted implementation capacity."

---

## UC12: Post-Delivery Cycle Time Analysis

**Goal:** Analyze the historical lead times of recently delivered items without them "aging" relative to today.

- **Primary Actor:** User (Project Lead)
- **Trigger:** User asks: "How long did it take us to deliver our last 10 items?"
- **Main Success Scenario:**
    1. AI calls `analyze_work_item_age` with `aging_type: "wip"` and `tier_filter: "Finished"`.
    2. MCP Server identifies items in terminal statuses but returns their **pinned Cycle Time** (time spent from commitment to finish point).
    3. AI reports: "The last 10 items had a median cycle time of 12.5 days. Note that these are fixed delivery metrics; they do not increase as time passes since their delivery."

---

## UC13: Analytical Workflow Guidance (The Roadmap)

**Goal:** Provide the AI with a structured, reliable path for complex analytical objectives.

- **Primary Actor:** AI (Orchestrator)
- **Trigger:** User asks a broad goal-oriented question (e.g., "Analyze our bottlenecks").
- **Main Success Scenario:**
    1.  AI identifies the goal and calls `guide_diagnostic_roadmap` with `goal: "bottlenecks"`.
    2.  MCP Server returns a prioritized sequence of tools:
        - `workflow_discover_mapping` (Verify semantics)
        - `analyze_status_persistence` (Identify local queues)
        - `analyze_work_item_age` (Identify current WIP risk)
    3.  AI follows the sequence, explaining each step to the user.
    4.  AI synthesizes the results into a cohesive diagnostic report.

---

## UC14: Strategic Scenario Planning

**Goal:** Multi-Type Shared Capacity Forecasting: Forecast stories while background bugs consume throughput.

- **Dynamic Sampling & Baseline Shifting**: Ignore holiday dips or focus on high-velocity periods for realistic forecasting.
- **Statistical Guardrails**: Detection of fat-tails and extreme volatility.

- **Primary Actor**: User (Project Manager/Product Owner)
- **Trigger**: User asks "What if we spend more time on Bugs?" or "How does our forecast change if we target 20 Stories and 5 Improvements?"
- **Main Success Scenario**:
    1. AI calls `forecast_monte_carlo` with `targets` (e.g., `{"Story": 20, "Improvement": 5}`) to define an explicit backlog mix.
    2. AI optionally applies `mix_overrides` (e.g., `{"Bug": 0.25}`) to simulate a strategic shift in capacity towards bug-fixing.
    3. MCP Server runs Monte-Carlo trials where background capacity is re-allocated based on the overrides and remaining work is sampled from the structured targets.
    4. AI presents a comparative analysis, highlighting how the delivery of the main backlog is impacted by the background "friction" or strategic capacity shifts.
    5. AI identifies potential risks or opportunities associated with each scenario, leveraging statistical guardrails to flag extreme volatility or fat-tail distributions.
    6. **Resolution Density Safeguard**: If the strategic scenario relies on a `resolutions` filter that ignores >80% of historical throughput, the server warns of **Low Resolution Density**, identifying that the "delivered" baseline may be non-representative of actual system capacity.

---

## UC15: Forensic Forecast Accuracy Backtesting

**Goal:** Empirically validate the reliability of forecasts by "time-travelling" into the past and comparing predictions against actual outcomes.

- **Primary Actor:** AI (Proactive) or User
- **Trigger:** AI wants to verify if MCS is reliable for a specific board before presenting a high-stakes forecast.
- **Main Success Scenario:**
    1.  AI calls `forecast_backtest` with `simulation_mode: "scope"`.
    2.  MCP Server utilizes **Time-Travel Logic** to reconstruct the state of the system at several past checkpoints (e.g., every 14 days for the last 6 months).
    3.  For each checkpoint, the server runs a simulation and compares it to the **actual** completion history that followed.
    4.  AI detects if the accuracy score is below the **70% threshold**.
    5.  AI reports: "I've backtested my forecasting model against your last 6 months of data. It achieved **67% accuracy**. While helpful, you should treat these dates with caution due to the high volatility detected in late 2025."

---

## UC16: Deep Historical Exploration (Expanding Cache)

**Goal:** Analyze deep historical trends for long-running projects where the initial hydration was insufficient.

- **Primary Actor:** User (Operations Lead / Coach)
- **Trigger:** User asks: "I need to see the process evolution for the last 4 years."
- **Main Success Scenario:**
    1. AI identifies that the current cache is truncated at the default limit.
    2. AI calls `import_history_expand` with additional `chunks`.
    3. MCP Server fetches older items backwards from the **OMRC (Oldest Most Recent Change)** boundary to ensure no gaps or overlaps.
    4. MCP Server automatically performs a **Catch-Up** to ensure the latest state is also preserved.
    5. MCP Server recalculates the **Discovery Cutoff** (Warmup Period) for the extended dataset.
    6. AI provides the requested deep-history analysis (e.g., `analyze_process_evolution`).

---

## UC17: Targeted Cache Synchronization (Catch-Up)

**Goal:** Ensure the local cache is perfectly synchronized with the latest Jira updates before a high-stakes forecast.

- **Primary Actor:** AI (Proactive) or User
- **Trigger:** AI prepares a forecast or User asks "Is this analysis based on the latest data?".
- **Main Success Scenario:**
    1. AI identifies the need for a synchronization pulse.
    2. AI calls `import_history_update`.
    3. MCP Server fetches all items updated since the **NMRC (Newest Most Recent Change)** boundary.
    4. MCP Server uses **Purge-before-Merge** to replace old history for those items, ensuring Jira deletions or corrections are reflected.
    5. AI confirms synchronization: "I've fetched 12 updates from Jira. We are now perfectly synced as of [Timestamp]."

---

## UC18: Friction Heatmap Analysis (Impediment Identification)

**Goal:** Identify workflow steps that are systemic centers of friction (frequent or deep blocking).

- **Primary Actor:** Project Manager / Operations Lead
- **Trigger:** User asks "Where are we getting blocked most often?" or "Which stage has the most friction?"
- **Main Success Scenario:**
    1.  User asks for a friction analysis.
    2.  AI calls `analyze_status_persistence`.
    3.  MCP Server calculates the **Impediment Count** and **Impediment Depth** using geometric intersection of blocked intervals.
    4.  AI identifies the stage with the highest friction (e.g., "Peer Review is your primary friction center: 60% of items encountered a blocker here, with a median (P50) blocked duration of 4 days").
    5.  AI contrasts this with total residency to provide context: "While 'In Testing' has high total residency, 'Peer Review' has the highest _impediment_ density."
    6.  **Stability Guardrail Awareness:** If the overall system is under high stress (>25% WIP flagged), the AI automatically surfaces a "SYSTEM PRESSURE WARNING" during forecasting simulations to alert the user that historical throughput may be overly optimistic.
