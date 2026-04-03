# MCS-MCP: Monte-Carlo Simulation for Model Context Protocol

**MCS-MCP** is a Model Context Protocol (MCP) server that connects AI assistants like Claude to your Jira project history, enabling natural-language delivery analytics and probabilistic forecasting. Ask questions like _"When will we finish these 20 items?"_, _"Is our process getting more predictable?"_, or _"Where are items getting stuck?"_ — and get answers grounded in your team's actual historical performance, not estimates or gut feel.

> [!WARNING]
> Currently, this must be considered _beta_. While it works quite well,
> the Math is not thoroughly verified. Don't bet your bonus on the
> forecasts and analysis done by it. Concepts are subject to change, if necessary
> to make an AI Agent behave the way I envision.
> I run it in Claude Desktop and Antigravity Agents.

---

## 🚀 Key Capabilities

- **Monte-Carlo Forecasting**: Run 10,000+ simulations to answer "When will it be done?" (Duration) or "How much can we do?" (Scope). Uses your team's actual historical throughput, not estimates.
- **Forecast Backtesting**: Empirically validate how accurate the forecasts would have been by replaying them against your own historical data (Walk-Forward Analysis).
- **Predictability Guardrails**: Detect "Special Cause" variation using XmR Control Charts — assesses process stability for Cycle Time, WIP populations, and Delivery Cadence.
- **Workflow Semantic Discovery**: Automatically infer the purpose of each workflow status (active work, waiting queues, entry funnel, terminal exit) to identify true bottlenecks rather than administrative overhead.
- **Process Yield & Abandonment**: Quantify waste by identifying exactly where work is discarded — broken down by work type and workflow stage.
- **High-Fidelity Aging Analysis**: Identify "neglected" inventory by comparing current WIP age against historical norms at the individual status level.
- **Stratified Analytics**: Work item type stratification is pervasive across the suite. Separate Bugs from Stories in simulations, throughput, cycle time, and stability to surface capacity conflicts (the "Bug-Tax").
- **Sample Path Analysis (Residence Time)**: Compute the finite Little's Law identity L(T) = Λ(T) · w(T) to unify cycle time, WIP age, and flow debt into a single coherent view. Includes w'(T) (departure-denominated residence time) and Θ(T) (departure rate) to detect flow imbalance. The coherence gap between residence time and sojourn time reveals the "end effect" of active items on the system.
- **Strategic Evolution Tracking**: Longitudinal audits using Three-Way Control Charts (weekly/monthly) detect systemic improvements or process drift over time.
- **Historical Time-Travel**: Set a specific past date as the analytical reference point to recreate the state of your process at that moment. Useful for retrospectives, post-mortems, or before/after comparisons following a process change.
- **Guided Analytical Roadmaps**: The server proactively suggests the right sequence of diagnostic steps for a given goal (forecasting, bottleneck analysis, capacity planning), preventing AI agents from guessing at the right path.

---

## 🧱 Limitations and Assumptions

- The Server doesn't use Jira's workflow related REST APIs. It infers the workflow from actual issue transitions, which is very reliable if - and that is the assumption - all work item types on a board share the same workflow, or - to be more precise - share the same statuses in the same order. If that assumtion doesn't hold, workflow discovery will yield unpredictable results and analytics/diagnostic and forecasting functions might get confused. I don't know whether I will ever tackle such cases, because analyzing a flow across different workflows seems nonsensical to me (I'm open to be proven wrong).
- For the same reason, the Server currently works on Boards and doesn't allow you to pass JQL to it. A simple workaround is obvious: Create a board using your query as the Board-JQL. Just be mindful of what you're doing.

---

## 🛡️ Data Security & GRC Principles

Work-Management Systems like Atlassian Jira often contain sensitive project and personal data. MCS-MCP is built with a **Security-by-Design** approach, operating on two fundamental governing principles:

### 1. The "Need-to-Know" Principle (Data Minimization)

To protect intellectual property and privacy, the server strictly minimizes the data it ingests and persists.

- **What we ingest & persist**: Analytical metadata only — **Issue Keys, Issue Types, Status Transitions, Timestamps**, and **Resolution names**. This is the minimum set required for high-fidelity flow analysis.
- **What we DROP**: While the Jira API might return comprehensive issue objects, the system is designed to **immediately drop** sensitive content such as **Titles, Descriptions, Acceptance Criteria, or Assignees**.

This ensures that even if the server's cache were compromised, it contains no human-readable content that could leak project secrets or PII. Furthermore, because this data is never processed by the analytical engine or stored in memory, **it is impossible for sensitive content to leak to the AI Agent** during interaction.

### 2. The Transparency Principle (Auditability)

We believe in "No Black Boxes." The server operates primarily from its local caches after the initial ingestion.

- **Human-Readable Caches**: All persisted data (Event Logs, Workflow Mappings) is stored in standard, human-readable formats (JSON and JSON-Lines) in the data directory.
- **Verifiable Logic**: You can scan or monitor these files at any time to verify that no sensitive data has leaked into the server's long-term memory.

---

## 🛠️ How it Works (high-level)

1. **Ingestion**: The server fetches Jira items from a Board within a Project and reconstructs a complete, event-sourced history from their transition logs.

2. **Workflow Mapping**: Every Jira workflow is unique. Before analysis can begin, the server needs to understand the _purpose_ of each status. It proposes a mapping to a four-tier meta-workflow:
   - **Demand**: The entry funnel — ideas and backlog items not yet committed to.
   - **Upstream**: Analysis and refinement — work that has been picked up but not yet committed to delivery.
   - **Downstream**: Active execution — coding, testing, review. This is the "commitment zone."
   - **Finished**: Terminal states — done, cancelled, discarded.

   This mapping, along with what "done" means (delivered vs. abandoned) and where the **Commitment Point** is (the status where work is officially started — this defines Cycle Time and WIP), is proposed automatically and confirmed by you. It is cached so you only need to do this once per board.

3. **Diagnostics & Forecasting**: With the data and a clear understanding of what each status _means_, you can ask the AI a wide range of questions about your delivery system's health, predictability, and future throughput.

---

## 🏃 Getting Started

### Prerequisites

- Access to Atlassian Jira (Data Center or Cloud)
- A MCP-capable AI Agent to chat with (Claude Desktop is mine)
- Recent Version of Go if you want to build yourself

### How-To

1. Build from sources (see [Building from Sources](#building-from-sources)) or download a release.
2. Copy `mcs-mcp.exe` (or `mcs-mcp`) and `.env-example` to a location of your choice (for builders, look into `dist/`).
3. Rename `.env-example` to `.env` and modify it accordingly. At a minimum, you need to provide information about your Jira instance and how to authenticate (see [Authentication](#authentication)).
4. Especially for **MacOS Users**: Make sure that the MCP-Server either has write permissions on the path of the binary (`mcs-mcp`) or use the `DATA_PATH` setting in `.env` to point to directory, where it has, because it needs to create two folders (`cache` and `logs`) and write files to it.
5. Configure an AI Agent to use it as an MCP tool (see [Agent Configuration](#agent-configuration)). Typically, you need to restart the Agent for the changes to take effect.
6. Chat:
   - Tell the AI Agent which Project and which Board you want to look at.
   - Tell the Agent to **discover the workflow**. Carefully review whether the proposal matches your actual process. You should confirm the **Tiers** (Demand, Upstream, Downstream, Finished), which resolutions or terminal statuses mean _delivered_ vs. _abandoned_ (this determines what counts as throughput), and what your **Commitment Point** is (the status where work officially starts — this defines Cycle Time and WIP) and of course the order of workflow statuses. These choices are cached, so you only need to confirm them once (unlesss you empty the `cache` folder).
   - Ask the Agent for the **diagnostic roadmap** to get a goal-oriented sequence of tools (e.g., _"I want to forecast 15 items"_ or _"I want to understand what's slowing us down"_).
   - Optionally, ask the Agent to set an **evaluation date** if you want to analyze the system as it existed at a point in the past (e.g., for a retrospective or post-mortem).

### Authentication

The server supports 3 ways of Authentication through setting variables in the `.env` file.

Note: Please make sure that those auth-related variables, that don't apply, are commented out.

#### Option A: Personal Access Token (PAT) for Jira Datacenter

```env
JIRA_TOKEN_TYPE=pat
JIRA_TOKEN=your-personal-access-token
```

#### Option B: API Token for Jira Cloud

```env
JIRA_TOKEN_TYPE=api
JIRA_TOKEN=your-api-token
JIRA_USER_EMAIL=your-jira-account-email
```

#### Option C: Session Cookies - Fallback

If neither is available, provide session cookies extracted from an active browser session:

```env
JIRA_SESSION_ID=jira-session-ID
JIRA_XSRF_TOKEN=XSRF-token
JIRA_REMEMBERME_COOKIE=Jira-RememberMe-cookie
JIRA_GCILB=optional-google-cloud-load-balancer-cookie-GCILB
JIRA_GCLB=optional-google-cloud-load-balancer-cookie-GCLB
```

Note that Cookies a) might expire from time to time and b) Atlassian may take measures to prevent the use of session cookies this way.

### Building from Sources

Download Sources or clone the repository.

**On Windows (PowerShell):**

```powershell
.\build.ps1 build
```

**On Unix/Linux/MacOS (Make):**

Untested, but should work.

```bash
make build
```

The resulting binary will be located in the `dist/` folder along with a exemplary `.env` file.

### Agent Configuration

To use as a server for an AI Agent (like Claude or Gemini), point your MCP client configuration to the compiled binary:

```json
{
	"mcpServers": {
		"mcs-mcp": {
			"command": "/path/to/mcs-mcp/mcs-mcp.exe",
			"args": []
		}
	}
}
```

Make sure that the Server can write to this directory to create `cache` and `logs` folders - or reconfigure using `DATA_PATH`.

### Optional Settings

These optional variables can be set in the `.env` file:

| Variable                                | Default      | Description                                                                                 |
| :-------------------------------------- | :----------- | :------------------------------------------------------------------------------------------ |
| `DATA_PATH`                             | (binary dir) | Base folder for logs and cache.                                                             |
| `VERBOSE`                               | `false`      | Write detailed debug information to the log file.                                           |
| `ENABLE_MERMAID_CHARTS`                 | `false`      | Include text-based Mermaid.js charts in analytical tool results.                            |
| `COMMITMENT_POINT_BACKFLOW_RESET_CLOCK` | `true`       | Reset Cycle Time and WIP Age clock on backflow past commitment point.                       |
| `JIRA_REQUEST_DELAY_SECONDS`            | `5`          | Enforced delay (in seconds) between requests to the Jira REST API.                          |
| `MCS_CHARTS_BUFFER_SIZE`                | `0`          | Chart rendering buffer (0=off, 1-100=on). Starts HTTP server on localhost.                  |
| `MCS_ALLOW_EXPERIMENTAL`                | `false`      | Enable the experimental feature gate. See [Experimental Features](#-experimental-features). |

---

## 📊 Chart Rendering

MCS-MCP renders interactive charts server-side. When `MCS_CHARTS_BUFFER_SIZE` is set to a value between 1 and 100, the server starts an HTTP listener on a random localhost port (3000-4000). Each analytical tool response includes a `chart_url` in the `context` field. Opening that URL in a browser renders a self-contained React/Recharts chart with the tool's data.

The server buffers the most recent N tool results (MRU). Older entries are evicted when the buffer is full. Charts are rendered on demand when the URL is accessed.

### Opening Chart URLs from an AI Agent

For charts to open automatically (without manual copy-paste), the AI agent needs to be able to open a localhost URL in a browser:

- **Claude Desktop**: Install the [Claude Extension for Chrome/Firefox](https://claude.ai/download). With the browser extension active, Claude can open `chart_url` links directly in your browser.
- **Without the extension**: Copy the `chart_url` from the tool response and paste it into any browser manually. The chart is fully self-contained and requires no active server connection beyond the initial load.

> **Note for users of previous versions:** The old Skills-based chart rendering (`docs/skills/`, `inject.py`) has been removed. Charts are now rendered entirely server-side. If you have the `mcs-mcp.skill` file installed in Claude Desktop, you should uninstall or disable it to avoid conflicts.

---

## 👉 Usage Tips

- Ask the Agent for the **diagnostic roadmap** at the start of a session. It returns a goal-oriented sequence of tools so you always have a clear path of analysis rather than guessing what to run next.
- After a significant process change (team restructure, workflow overhaul), tell the Agent to re-run workflow discovery with a force-refresh to re-evaluate the semantic mapping against current patterns.
- The server caches all event history locally. After the initial ingestion, most queries run entirely offline — no Jira connection needed.

---

## 🧪 Experimental Features

MCS-MCP includes an experimental feature system for testing improvements to the forecasting engine before they become the default. Experimental features are **off by default** and require two deliberate steps to activate:

1. **Operator enables the gate** by setting `MCS_ALLOW_EXPERIMENTAL=true` in `.env`.
2. **The AI Agent activates experimental mode** for the current session by calling the `set_experimental` tool. This persists until explicitly disabled — it is not reset when switching boards.

When both steps are satisfied, the forecasting tools gain access to experimental code paths. When either step is missing, the server behaves identically to stable production mode.

For details on active experiments — what they do, parameters, known limitations, and graduation criteria — see **[Experimental Features Documentation](docs/experimental.md)**.

---

## 🎲 Offline Testing & Simulation (mockgen)

If you do not have a live Jira connection (or simply want to test the server's analytical capabilities without using sensitive corporate data), MCS-MCP includes a built-in mock data generator called `mockgen`.

This tool produces a standardized, simulated dataset that you can analyze using `MCSTEST` as project key and board name. The MCP-Server then operates based on the files `MCSTEST_0.jsonl` and `MCSTEST_0_workflow.json` which are expected to be in the `cache/` directory.

For comprehensive details on how to use `mockgen`, configure data distributions (mild, chaotic, drifted), and rebuild the cached simulation, please check the **[Mock Data Generator Guide](docs/mockdata.md)**.

---

## ⚠️ Probabilistic Nature & Disclaimer

MCS-MCP is a statistical tool. It generates **probabilistic forecasts** based on historical performance, not guarantees.

- **No Direct Answer**: A forecast saying "85% confidence by Oct 12" means there is a 15% chance it will take longer.
- **Garbage In, Garbage Out**: Results are strictly dependent on the quality and consistency of your Jira data.
- **No Liability**: This tool is provided "AS IS". The authors and contributors are not responsible for any project delays, financial losses, or business decisions made based on its output.

---

## 📖 Guided Interaction

MCS-MCP is designed to be used by AI Agents as a "Technical Co-Pilot". For detailed guidance on specific workflows, refer to:

- **[Project Charter](docs/charter.md)**: Conceptual foundations and architectural principles.
- **[Interaction Use Cases](docs/use-cases.md)**: Detailed scenarios for PMs and AI Agents (When, Scope, Bottlenecks, Backtesting, etc.).
- **[Architecture Deep-Dive](docs/architecture.md)**: Internal mechanics, tool directory, analytical pipeline, and data flow. The primary reference for AI Agents interacting with this server.
- **[Mock Data Generator](docs/mockdata.md)**: Instructions for using `mockgen` and manipulating the `MCSTEST` synthetic sandbox.

---

## Developing & Contributing

To be honest, I havent thought that through.

If you want to contribute, get in touch. I welcome any feedback, because tools like this get better by being exposed to many different projects. If any diagnosis or forecast feels off, let me know.

If you want to develop, make sure your Coding Agent reads `.agent/rules/preferences.md` - especially since this points to further relevant files.

---

## ⚖️ Conceptual Integrity

This project adheres to the core principles of **Cohesion, Coherence, and Consistency**. Every tool and analytical model is designed to provide a unified, reliable view of delivery performance without administrative noise.

---

## Acknowledgements

- **Redidence Time** is based on the work of Krishna Kumar and our implementation is heavily based on and validated against his reference implementation in the [Sample Path Analysis Toolkit](https://github.com/presence-calculus/samplepath-flow).

---

## 📜 License

This project is licensed under the **Apache License 2.0**. See the [LICENSE](LICENSE) and [NOTICE](NOTICE) files for details.

Copyright © 2026 Bruno Baketarić.
