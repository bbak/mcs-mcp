# MCS-MCP: Monte-Carlo Simulation for Model Context Protocol

**MCS-MCP** is a sophisticated Model Context Protocol (MCP) server that empowers AI assistants with deep analytical and forecasting capabilities for software delivery projects. By leveraging historical Jira data and high-performance Monte-Carlo simulations, it transforms raw project history into actionable, probabilistic delivery insights with a strong focus on **mathematical hardening and defensive design**.

> [!WARNING]
> Currently, this must be considered _beta_. While it works quite well,
> the Math is not thoroughly verified. Don't bet your bonus on the
> forecasts and analysis done by it. Concepts are subject to change, if necessary
> to make an AI Agent behave the way I envision.
> I run it in Claude Desktop and Antigravity Agents.

---

## 🚀 Key Capabilities

- **Stratified Analytics Baseline**: Type-stratification is pervasive across the suite. Detect "Capacity Clashes" (Bug-Tax) in simulations, identify type-specific bottlenecks in **Status Residency**, and assess **WIP Age** using type-aware benchmarks.
- **Monte-Carlo Forecasting**: Run 10,000+ simulations to answer "When will it be done?" (Duration) or "How much can we do?" (Scope). Automatically coordinates sampling across multiple work types to ensure realistic theoretical capacity.
- **Forecast Backtesting**: Perform **Walk-Forward Analysis** to empirically validate forecast accuracy by "time-travelling" into historical data.
- **Predictability Guardrails**: Use **XmR Control Charts** and **Stability Indices** (stratifiable by type) to detect "Special Cause" variation and assess process stability for **Cycle Time**, **Active WIP Populations**, and **Delivery Cadence (Throughput)**.
- **Workflow Semantic Discovery**: Automatically infer the roles of workflow statuses (Active, Queue, Demand, Finished) to identify true bottlenecks instead of administrative delays.
- **Process Yield & Abandonment**: Quantify "waste" by identifying exactly where work (broken down by type) is discarded in the discovery or execution pipeline.
- **High-Fidelity Aging Analysis**: Track **WIP Age** and status-level persistence to identify "neglected" inventory before it impacts delivery.
- **Strategic Evolution Tracking**: Perform longitudinal audits using **Three-Way Control Charts** (Weekly/Monthly) to detect systemic improvements or process drift over time.
- **Guided Analytical Roadmaps**: Proactively guide AI agents through the correct sequence of diagnostic steps (Stability -> Discovery -> Analysis) based on specific goals.

---

## 🛡️ Data Security & GRC Principles

Work-Management Systems like Atlassian Jira often contain sensitive project and personal data. MCS-MCP is built with a **Security-by-Design** approach, operating on two fundamental governing principles:

### 1. The "Need-to-Know" Principle (Data Minimization)

To protect intellectual property and privacy, the server strictly minimizes the data it ingests and persists.

- **What we ingest & persist**: Analytical metadata only—**Issue Keys, Issue Types, Status Transitions, Timestamps**, and **Resolution names**. This is the minimum set required for high-fidelity flow analysis.
- **What we DROP**: While the Jira API might return comprehensive issue objects, the system is designed to **immediately drop** sensitive content such as **Titles, Descriptions, Acceptance Criteria, or Assignees**.

This ensures that even if the server's cache were compromised, it contains no human-readable content that could leak project secrets or PII. Furthermore, because this data is never processed by the analytical engine or stored in memory, **it is impossible for sensitive content to leak to the AI Agent** during interaction.

### 2. The Transparency Principle (Auditability)

We believe in "No Black Boxes." The server operates primarily from its local caches after the initial ingestion.

- **Human-Readable Caches**: All persisted data (Event Logs, Workflow Mappings) is stored in standard, human-readable formats (JSON and JSON-Lines) in the data directory.
- **Verifiable Logic**: You can scan or monitor these files at any time to verify that no sensitive data has leaked into the server's long-term memory.

---

## 🛠️ How it Works (high-level)

1. **Ingestion**: The Server fetches Jira items from a Board within a Project and extracts all it needs into a event-sourced log, by reading the history of the work items.
2. **Context Resolution**: From that it infers the flow of work items and proposes a mapping to a meta-workflow (Demand → Upstream → Downstream → Finished), what _done_ means, and similar, giving you the option to change it. This mapping is cached so you don't have to do that every time.
3. **Flow diagnostics & forecasting**: Given that it now has the data and _understands_ the flow of work, you can start asking the AI various questions. Given that most flows are not in _statistical control_ you might want to dive into diagnostics first. The AI Agent can also suggest an analytical roadmap.

---

## 🏃 Getting Started

### Prerequisites

- Access to Atlassian Jira (Data Center or Cloud)
- A MCP-capable AI Agent to chat with (Claude Desktop is mine)
- Recent Version of Go if you want to build yourself

### How-To

1. Build from sources (see [Building from Sources](#building-from-sources)) or download a release.
2. Copy `mcs-mcp.exe` and `.env-example`to a location of your choice (for builders, look into `dist/`).
3. Rename `.env-example` to `.env` and modify it accordingly. At a minimum, you need to provide information about your Jira instance and how to authenticate (see [Authentication](#authentication)).
4. Configure a AI Agent to use it as an MCP tool (see [Agent Configuration](#agent-configuration)). Typically, you need to restart the Agent for the changes to take effect.
5. Chat:
    - Tell the AI Agent which Project and which Board you want to look at.
    - Tell the Agent to discover the workflow. Throughly check whether the proposal matches your expectations. You should be clear about the "Tiers" (Demand, Upstream, Downstream, Finished), which resolution or terminal status means _delivered_, and which means _abandoned_ (affects _throughput_). Also, what your _commitment point_ is (affects _cycle time_ and what _WIP_ means). Note, that any changes you make are cached, so you don't have to do this every time.
    - Ask the Agent for what the MCP-Server can do or the analytical roadmap. You can also ask to list all available tools of the MCP-Server.

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

---

## Skills

Since v.0.11.0 the MCP-Server comes with Skills that can be used by AI Agents to create Charts and other visualizations. The skills are located in the `docs/skills/` directory of the MCP-Server release archive. See [Skill Installation Instructions](docs/skill-installation-instructions.md) for instructions on how to install them.

---

## Usage Tips:

- Many Agents can create Charts right from the data that's passed from the MCP-Server. In Claude Desktop a prompt like _"Please create a Chart for WIP"_ works well. For some Chart types you may install a Skill that can be found in the `docs/skills/` directory of the MCP-Server release archive.

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
- **[Architecture Deep-Dive](docs/architecture.md)**: Aging math, backflow policies, and the status-granular flow model.
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
