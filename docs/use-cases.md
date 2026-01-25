# Interaction Scenarios and Use Cases

This document describes the primary interaction scenarios between the User (Project Manager), the AI (Assistant), and the MCS MCP Server. These use cases follow Alistair Cockburn's format to provide rigor and insight into the forecasting process.

---

## UC1: Forecast Initiative Duration (Backlog-based)

**Goal:** Determine how long it will take to deliver a known backlog of items (e.g., "When will these 50 stories be done?").

- **Primary Actor:** User (Project Manager)
- **Secondary Actors:** AI (Intermediary), MCP Server (Forecasting Engine), Jira (Data Source)
- **Trigger:** User asks a "When?" question regarding a specific number of items.
- **Preconditions:**
    - MCP Server is connected to Jira.
    - Historical data (at least 3-6 months) exists for the team/project.
- **Main Success Scenario:**
    1.  User asks: "How long will it take to finish 50 Story items in Project X?"
    2.  AI identifies the source (board/filter ID).
    3.  **AI Uncertainty Check**: AI evaluates if it has enough metadata (e.g., has it run a probe for this board recently? Does it know the `start_status`?).
    4.  **Reactive Trigger (UC4)**: If metadata is missing or stale, AI triggers **UC4 (Data Discovery)** to analyze historical reachability and available statuses.
    5.  AI presents the "Assessment" to the User: "I've analyzed your project history. To give you a rigorous forecast, I'll use 'In Progress' as the commitment point. Is that correct?"
    6.  AI calls `run_simulation` with the confirmed parameters.
    7.  MCP Server:
        - Fetches historical data.
        - Calculates throughput distribution (Histogram).
        - Runs 10,000 Monte-Carlo trials.
        - Returns percentile outcomes (50%, 85%, 95%).
    8.  AI presents the results: "There is an 85% probability that the 50 items will be completed within Y to Z days."
- **Extensions:**
    - **5a. No historical data:** MCP returns an error. AI suggests using a different time frame or source.
    - **5b. Low data volume:** MCP returns results with a "Low Confidence" warning. AI informs the user about the risk of using small datasets.
- **Implementation Drivers:**
    - _Sanity Check:_ Add a check to compare the requested backlog size against historical velocity. If backlog > 10x historical monthly throughput, flag as "High Uncertainty".
    - _Tool:_ Add a `validate_historical_data` tool to explicitly check for gaps in history before running simulations.

---

## UC2: Forecast Initiative Scope (Time-based)

**Goal:** Determine how many items can be delivered until a specific date (e.g., "What can we get done by end of Q1?").

- **Primary Actor:** User (Project Manager)
- **Trigger:** User asks a "How much?" question relative to a deadline.
- **Preconditions:**
    - Same as UC1.
- **Main Success Scenario:**
    1.  User asks: "How many Story items can we complete by March 31st?"
    2.  AI identifies the source and calculates `target_days`.
    3.  **Reactive Trigger (UC4)**: AI triggers discovery if it lacks confidence in the project's historical throughput stability or workflow.
    4.  AI aligns with the User on any anomalies found during the probe.
    5.  AI calls `run_simulation` and presents the results.
- **Implementation Drivers:**
    - _Improvement:_ Allow the user to specify "Probability of failure" (e.g., "Give me a conservative estimate").

---

## UC3: Predict Individual Item Delivery (Cycle Time)

**Goal:** Get a probabilistic estimate for a single high-priority item or understand typical lead times.

- **Primary Actor:** User (Project Manager)
- **Trigger:** User asks: "How long does a typical Bug take to fix?" or "When will issue PROJ-123 be done?"
- **Preconditions:**
    - Same as UC1.
- **Main Success Scenario:**
    1.  User asks about cycle time for "Bugs".
    2.  **AI triggers UC4 (Discovery)** to identify the specific workflow for "Bugs" and verify data quality.
    3.  AI calls `run_simulation` with `mode: "single"`, `issue_types: ["Bug"]`, and `start_status`.
    4.  MCP Server calculates cycle times for historical items and provides percentile analysis.
    5.  AI presents the "Service Level Expectation" (e.g., "85% of Bugs are resolved within 5 days").
- **Implementation Drivers:**
    - _Constraint:_ Requires a defined "Commitment Point" (start status) to calculate accurate cycle time.
    - _Improvement:_ Add a tool to visualize the Cycle Time Scatterplot or Histogram (via Markdown tables/charts).

---

## UC4: Data Quality & Workflow Discovery (The "Probe")

**Goal:** Assess if the historical data is suitable for simulation and identify workflow milestones.

- **Primary Actor:** AI (Autonomous) or User (Manual Request)
- **Trigger:** AI decides to "sanity check" a source before simulation, or User asks "Is my data good?".
- **Main Success Scenario:**
    1.  AI calls `get_data_metadata` for a `source_id`.
    2.  MCP Server:
        - Fetches the last 200 items.
        - Analyzes throughput stability.
        - Identifies all workflow statuses used in the project.
        - Checks for data "cleanliness" (e.g., items resolved without ever being 'In Progress').
    3.  AI reports: "The data shows a stable throughput, but I noticed 20% of items skip the 'In Progress' state. I also found these statuses: [To Do, In Dev, Testing, Done]. Which one should I use as the 'Commitment Point'?"
- **Postconditions:** User/AI are aligned on the workflow and data quality.
- **Implementation Drivers:**
    - _Sanity Check:_ Automatically detect "Long Tails" in the histogram which might skew MCS and warn the user.
    - _Auto-Hint:_ When a simulation fails due to an invalid or missing `start_status`, the server should provide "Likely Candidates" based on historical reachability and status categories.
    - _Refinement:_ This use case drives the need for more sophisticated "Data Cleansing" tools.

---

## UC5: Refine Simulation with WIP (Option A)

**Goal:** Account for the "head start" and age of current in-progress work to increase forecast accuracy.

- **Primary Actor:** AI (Proactive Suggestion)
- **Trigger:** AI notices significant active work items on the board.
- **Main Success Scenario:**
    1.  AI suggests: "I see 10 items currently in progress. Should I include their current age in the forecast for better accuracy?"
    2.  User agrees.
    3.  AI calls `run_simulation` with `include_wip: true` and `start_status: "In Progress"`.
    4.  MCP Server:
        - Analyzes the age of each active item (Time since it entered `start_status`).
        - Adjusts the simulation to "finish" these items first based on historical cycle times.
        - Adds remaining backlog items after.
    5.  AI presents a refined, more realistic forecast.
- **Implementation Drivers:**
    - _Complexity:_ This is the most complex scenario as it requires `SearchIssuesWithHistory` for active items.
    - _Improvement:_ Add "Stability Criteria" (Little's Law check) to warn if WIP is growing faster than throughput.
