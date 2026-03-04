# MCS-MCP: Monte-Carlo Simulation for Model Context Protocol

**MCS-MCP** is a Model Context Protocol (MCP) server that connects AI assistants like Claude to your Jira project history, enabling natural-language delivery analytics and probabilistic forecasting. Ask questions like _"When will we finish these 20 items?"_, _"Is our process getting more predictable?"_, or _"Where are items getting stuck?"_ — and get answers grounded in your team's actual historical performance, not estimates or gut feel.

Rather than relying on Jira's often-misconfigured built-in reports, MCS-MCP infers the true shape of your delivery process from raw transition logs, with a strong focus on **mathematical hardening and defensive design**.

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
- **Strategic Evolution Tracking**: Longitudinal audits using Three-Way Control Charts (weekly/monthly) detect systemic improvements or process drift over time.
- **Historical Time-Travel**: Set a specific past date as the analytical reference point to recreate the state of your process at that moment. Useful for retrospectives, post-mortems, or before/after comparisons following a process change.
- **Guided Analytical Roadmaps**: The server proactively suggests the right sequence of diagnostic steps for a given goal (forecasting, bottleneck analysis, capacity planning), preventing AI agents from guessing at the right path.

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
2. Copy `mcs-mcp.exe` and `.env-example` to a location of your choice (for builders, look into `dist/`).
3. Rename `.env-example` to `.env` and modify it accordingly. At a minimum, you need to provide information about your Jira instance and how to authenticate (see [Authentication](#authentication)).
4. Configure an AI Agent to use it as an MCP tool (see [Agent Configuration](#agent-configuration)). Typically, you need to restart the Agent for the changes to take effect.
5. Chat:
   - Tell the AI Agent which Project and which Board you want to look at.
   - Tell the Agent to **discover the workflow**. Carefully review whether the proposal matches your actual process. You should confirm the **Tiers** (Demand, Upstream, Downstream, Finished), which resolutions or terminal statuses mean _delivered_ vs. _abandoned_ (this determines what counts as throughput), and what your **Commitment Point** is (the status where work officially starts — this defines Cycle Time and WIP). These choices are cached, so you only need to confirm them once.
   - Ask the Agent for the **diagnostic roadmap** to get a goal-oriented sequence of tools (e.g., _"I want to forecast 15 items"_ or _"I want to understand what's slowing us down"_).
   - Optionally, ask the Agent to set an **evaluation date** if you want to analyze the system as it existed at a point in the past (e.g., for a retrospective or post-mortem).

### Authentication

The server supports 3 ways of Authentication through setting variables in the `.env` file.

Note: Please make sure that those auth-related variables, that don't apply, are commented out.

**Option A: Personal Access Token (PAT) for Jira Datacenter**

```env
JIRA_TOKEN_TYPE=pat
JIRA_TOKEN=your-personal-access-token
```

**Option B: API Token for Jira Cloud**

```env
JIRA_TOKEN_TYPE=api
JIRA_TOKEN=your-api-token
JIRA_USER_EMAIL=your-jira-account-email
```

**Option C: Session Cookies - Fallback**
If neither is available, provide session cookies extracted from an active browser session:

- `JIRA_SESSION_ID`: Your Jira session ID
- `JIRA_XSRF_TOKEN`: Your XSRF token
- `JIRA_REMEMBERME_COOKIE`: Your Jira RememberMe cookie
- (optional) `JIRA_GCILB`, `JIRA_GCLB`: Actually these are Google-Cloud Load Balancer Cookies

Note that these a) might expire from time to time and b) Atlassian may take measures to prevent the use of session cookies this way.

### Building from Sources

Download Sources or clone the repository.

**On Windows (PowerShell):**

```powershell
.\build.ps1 build
```

**On Unix/Linux (Make):**

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
			"command": "/path/to/mcs-mcp/dist/mcs-mcp.exe",
			"args": []
		}
	}
}
```

Make sure that the Server can write to this directory to create `cache` and `logs` folders - or reconfigure using `DATA_PATH`.

### Optional Settings

These optional variables can be set in the `.env` file:

| Variable                                | Default      | Description                                                           |
| :-------------------------------------- | :----------- | :-------------------------------------------------------------------- |
| `DATA_PATH`                             | (binary dir) | Base folder for logs and cache.                                       |
| `VERBOSE`                               | `false`      | Write detailed debug information to the log file.                     |
| `ENABLE_MERMAID_CHARTS`                 | `false`      | Include text-based Mermaid.js charts in analytical tool results.      |
| `COMMITMENT_POINT_BACKFLOW_RESET_CLOCK` | `true`       | Reset Cycle Time and WIP Age clock on backflow past commitment point. |
| `JIRA_REQUEST_DELAY_SECONDS`            | `10`         | Enforced delay (in seconds) between requests to the Jira REST API.    |

---

## Skills

MCS-MCP comes with Skills that can be used by AI Agents to create Charts and other visualizations. The skills are located in the `docs/skills/` directory of the MCP-Server release archive. See [Skill Installation Instructions](docs/skill-installation-instructions.md) for instructions on how to install them.

---

## Usage Tips

- Many Agents can create Charts directly from the data returned by the MCP-Server. In Claude Desktop, a prompt like _"Please create a Chart for WIP"_ works well. For specific chart types, install the relevant Skill from the `docs/skills/` directory.
- Ask the Agent for the **diagnostic roadmap** at the start of a session. It returns a goal-oriented sequence of tools so you always have a clear path of analysis rather than guessing what to run next.
- After a significant process change (team restructure, workflow overhaul), tell the Agent to re-run workflow discovery with a force-refresh to re-evaluate the semantic mapping against current patterns.
- The server caches all event history locally. After the initial ingestion, most queries run entirely offline — no Jira connection needed.

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

## 📜 License

This project is licensed under the **Apache License 2.0**. See the [LICENSE](LICENSE) and [NOTICE](NOTICE) files for details.

Copyright © 2026 Bruno Baketarić.
