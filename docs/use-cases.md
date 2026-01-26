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
    2.  AI identifies the source and calls `run_simulation` with `mode: "duration"`, `additional_items: 50`.
    3.  MCP Server runs 10,000 Monte-Carlo trials using historical throughput.
    4.  AI presents results using risk terminology: "There is a **Likely (85%)** probability that the work will be done by [Date]."

---

## UC2: Forecast Delivery Volume (Time-based Scope)

**Goal:** Determine how many items (scope) can be delivered within a fixed timeframe (e.g., "What can we get done by end of Q1?").

- **Primary Actor:** User (Project Manager)
- **Trigger:** User asks: "How many items can we complete by March 31st?"
- **Main Success Scenario:**
    1.  User asks about scope for a fixed date.
    2.  AI calculates `target_days` and calls `run_simulation` in `scope` mode.
    3.  AI presents the results (e.g., "With **Probable (70%)** confidence, you can deliver 45 items").

---

## UC3: Predict Individual Item Delivery (Cycle Time)

**Goal:** Get a probabilistic estimate for a single high-priority item.

- **Primary Actor:** User (Project Manager)
- **Trigger:** User asks: "When will issue PROJ-123 be done?"
- **Main Success Scenario:**
    1.  AI calls `run_simulation` with `mode: "single"`.
    2.  MCP Server utilizes the **Status-Granular Flow Model** to calculate lead times.
    3.  AI presents the Service Level Expectation (e.g., "85% of similar items are resolved within 5 days").

---

## UC6: Workflow Bottleneck Discovery

**Goal:** Identify which status in the workflow is causing the most delay (Persistence).

- **Primary Actor:** AI (Proactive) or User
- **Trigger:** User asks "Where are we stuck?"
- **Main Success Scenario:**
    1. AI calls `get_status_persistence`.
    2. MCP Server utilizes the **Status-Granular residency map** to identify the bottleneck.
    3. AI identifies the status with the highest **Safe-bet (P95)** age.
    4. AI reports: "Items typically spend **12 days (Likely)** in 'Peer Review', which is 4x longer than any other stage."

---

## UC7: System Pulse & Flow Stability

**Goal:** Detect if the team is delivering consistently or in erratic batches.

- **Primary Actor:** AI (Autonomous Analysis)
- **Trigger:** AI prepares a forecast and wants to validate the "Stability" assumption of MCS.
- **Main Success Scenario:**
    1. AI calls `get_delivery_cadence`.
    2. MCP Server returns weekly throughput counts.
    3. AI detects "Batching" (e.g., three weeks of 0, then one week of 20).
    4. AI warns: "Your delivery pulse is currently **Batch-based**. While the forecast says you'll finish by March, be aware that this assumes a massive delivery at the very end rather than a steady flow."

---

## UC8: Workflow Semantic Enrichment (The Mapping)

**Goal:** Provide the AI with the semantic context needed to distinguish real bottlenecks from administrative stages.

- **Primary Actor:** AI (Proactive)
- **Trigger:** AI sees high persistence in a "To Do" category status.
- **Main Success Scenario:**
    1. AI calls `get_workflow_discovery`.
    2. AI notices status "Open" has high residency but is categorized as "To Do".
    3. AI informs User: "I've mapped 'Open' as your **Backlog**. I will treat its high persistence as expected storage time unless you tell me otherwise."
    4. User confirms or vetos.
    5. AI calls `set_workflow_mapping` if changes are needed.

---

## UC9: Granular Journey Discovery

**Goal:** Understand the exact path and delays an individual item took through the workflow.

- **Primary Actor:** User or AI
- **Trigger:** Investigating a "Long Tail" outlier item.
- **Main Success Scenario:**
    1. AI calls `get_item_journey` for a specific issue key.
    2. MCP Server return a chronological path with residency days for each step.
    3. AI identifies exactly which step caused the outlier behavior (e.g., "PROJ-123 took 40 days, but 35 of those were spent in 'Blocked'").

---

## UC10: Process Yield & Abandonment Analysis

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
