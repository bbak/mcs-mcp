package mcp

// serverInstructions is the server-level `instructions` string returned in the
// MCP `InitializeResult`. Clients typically inject this into the LLM system
// prompt as a global hint about how to use this server. Keep it terse —
// per-tool guidance belongs in toolDescriptions.
const serverInstructions = `MCS-MCP is a Flow Metrics and Monte-Carlo Simulation server for Jira (Cycle Time, Throughput, WIP, Process Stability, probabilistic forecasts).

OPERATIONAL FLOW — follow in order, do not skip:
  1. import_projects / import_boards      — identify the target.
  2. import_board_context                  — eager-fetch history, anchor project context.
  3. workflow_discover_mapping             — propose tier mapping (Demand / Upstream / Downstream / Finished).
                                             YOU MUST present the proposed mapping to the user and obtain explicit confirmation
                                             (or adjustment via workflow_set_mapping) BEFORE running diagnostics or forecasts.
  4. guide_diagnostic_roadmap              — recommends next tools given the user goal.
  5. Diagnostics / forecasts               — run against the confirmed mapping.

WHEN SETUP FAILS — a failed call and an empty result are NOT the same thing:
  - CONNECTION OR CONFIGURATION FAILURE (the tool reports an error mentioning credentials, 404, or an
    unreachable Jira): STOP the entire workflow. Do NOT retry, do NOT try different search terms, do NOT
    call another tool hoping it works. Tell the user their Jira configuration or authentication needs
    fixing, quoting the tool's instruction, and wait. No analysis is possible until they resolve it.
  - JIRA TEMPORARILY UNAVAILABLE (overloaded, rate-limited, or down): do NOT retry now and do NOT retry in
    a few seconds — such a window outlasts this conversation, and retrying makes rate-limiting worse. Tell
    the user to try again in an hour or so, and end the current line of analysis.
  - EMPTY RESULT (the call succeeded, nothing matched): this is a search miss, not a failure. Ask the user
    for a different name, key, or a shorter fragment, and search again. Do not advance past the current
    step until the target is found.
  Never present an empty result as "the connection failed", and never present a failure as "nothing found" —
  the second tells the user their data is missing when in fact it was never queried.

STRICT GUARDRAILS (anti-hallucination):
  - NEVER compute percentiles, probabilities, SLEs, forecast dates, or stability signals using your own reasoning.
  - NEVER render charts or other visualizations by yourself. Use the open_in_browser tool to render data returned by tools.
  - If a tool returns an error, empty result, or warning about insufficient data, REPORT it to the user and ask for guidance.
    Do not substitute internal estimates. Do not "fill in" missing numbers.
  - Respect every warning emitted in the response — small samples, partial months, and unstable processes invalidate forecasts.

TOOL SELECTION:
  - Predictability of Cycle Time        → analyze_process_stability (short term) or analyze_process_evolution (long term)
  - Delivery volume / cadence           → analyze_throughput
  - Per-item duration / SLE             → analyze_cycle_time
  - Active WIP health                   → analyze_wip_stability, analyze_wip_age_stability, analyze_work_item_age
  - Bottlenecks / queueing              → analyze_status_persistence, analyze_residence_time
  - Probabilistic forecast              → forecast_monte_carlo (requires a stable process)
  - Backtesting accuracy                → forecast_backtest
  Prefer the per-tool description for detailed WHEN TO USE / WHEN NOT TO USE rules.

CHART RENDERING:
  Analytical responses carry a chart_url. Call open_in_browser with that URL to show the interactive chart.
  Tool JSON responses no longer include any embedded chart markup — render via chart_url only.

DATA INTEGRITY:
  Series are time-ordered; do not reorder results. Cycle Time, Moving Range, and WIP Age are sensitive to ordering and
  to the commitment-point configuration (COMMITMENT_POINT_BACKFLOW_RESET_CLOCK). Trust the tool output over intuition.`
