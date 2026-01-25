# Interaction Scenarios and Use Cases

This document describes the primary interaction scenarios between the User (Project Manager), the AI (Assistant), and the MCS MCP Server. These use cases follow Alistair Cockburn's format to provide rigor and insight into the forecasting process.

---

## UC1: Forecast Completion Date (Backlog-based Duration)

**Goal:** Determine how long it will take to deliver a fixed scope/backlog (e.g., "When will these 50 stories be done?").

- **Primary Actor:** User (Project Manager)
- **Secondary Actors:** AI (Intermediary), MCP Server (Forecasting Engine), Jira (Data Source)
- **Trigger:** User asks a "When?" question regarding a specific number of items.
- **Preconditions:**
    - MCP Server is connected to Jira.
    - Historical data (at least 3-6 months) exists for the team/project.
- **Main Success Scenario:**
    1.  User asks: "How long will it take to finish 50 Story items in Project X?"
    2.  AI identifies the source (board/filter ID).
    3.  **AI Uncertainty Check**: AI evaluates if it has enough metadata.
    4.  **Reactive Trigger (UC4)**: AI triggers **UC4 (Data Discovery)** to analyze historical reachability, available statuses, and **current backlog size**.
    5.  AI presents the "Assessment" to the User: "I've analyzed your project history. I found **126 unstarted items** in your backlog. To give you a rigorous forecast, I'll ALSO include your current **WIP**. Does that sound right?"
    6.  AI calls `run_simulation` with `mode: "duration"`, `backlog_size: 126`, `include_wip: true`, and the confirmed parameters.
    7.  MCP Server:
        - Fetches historical data.
        - Calculates throughput distribution.
        - Runs 10,000 Monte-Carlo trials.
        - Returns results with explicit **Composition** (126 Backlog + 10 WIP = 136 Total) and **Throughput Trend**.
    8.  AI presents results using risk terminology: "There is a **Likely (85%)** probability that the work will be done by [Date]. Note that your throughput is currently **Declining** by 15%, which I've accounted for."
- **Extensions:**
    - **5a. No historical data:** MCP returns an error. AI suggests using a different time frame or source.
    - **5b. Low data volume:** MCP returns results with a "Low Confidence" warning. AI informs the user about the risk of using small datasets.
- **Implementation Drivers:**
    - _Sanity Check:_ Add a check to compare the requested backlog size against historical velocity. If backlog > 10x historical monthly throughput, flag as "High Uncertainty".
    - _Tool:_ Add a `validate_historical_data` tool to explicitly check for gaps in history before running simulations.

---

## UC2: Forecast Delivery Volume (Time-based Scope)

**Goal:** Determine how many items (scope) can be delivered within a fixed timeframe (e.g., "What can we get done by end of Q1?").

- **Primary Actor:** User (Project Manager)
- **Trigger:** User asks a "How much?" question relative to a deadline.
- **Preconditions:**
    - Same as UC1.
- **Main Success Scenario:**
    1.  User asks: "How many Story items can we complete by March 31st?"
    2.  AI identifies the source and calculates `target_days` (or extracts `target_date`).
    3.  **Reactive Trigger (UC4)**: AI triggers discovery if it lacks confidence.
    4.  AI aligns with the User on the forecast window.
    5.  AI calls `run_simulation` (with `target_date` or `target_days`) and presents the results.
    6.  **WIP Transparency**: AI uses the returned `insights` to explain that the forecast volume includes the time needed to clear current active work.
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
    3.  AI calls `run_simulation` with `include_wip: true`.
    4.  MCP Server:
        - Analyzes active items and calculates **Context-Aware WIP Aging**.
        - Buckets items into **Inconspicuous**, **Aging**, **Warning**, and **Extreme**.
        - Calculates **Stability Index** (Little's Law).
    5.  AI presents forecast with **Actionable Insights**: "Included 10 WIP items. Note: 2 items are in the 'Extreme' bucket (>P95 age). Resolving these outliers would improve your throughput by an estimated 10%."
- **Implementation Drivers:**
    - _Complexity:_ This is the most complex scenario as it requires `SearchIssuesWithHistory` for active items.
    - _Improvement:_ Add "Stability Criteria" (Little's Law check) to warn if WIP is growing faster than throughput.
