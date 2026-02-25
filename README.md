# MCS-MCP: Monte-Carlo Simulation for Model Context Protocol

**MCS-MCP** is a sophisticated Model Context Protocol (MCP) server that empowers AI assistants with deep analytical and forecasting capabilities for software delivery projects. By leveraging historical Jira data and high-performance Monte-Carlo simulations, it transforms raw project history into actionable, probabilistic delivery insights with a strong focus on **mathematical hardening and defensive design**.

> [!WARNING]
> Currently, this must be considered _alpha_. While it works quite well,
> the Math is just partially verified. Don't bet your bonus on the
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

## 🛠️ How it Works

MCS-MCP operates on the principle of **Data-Driven Probabilism**. It avoids single-point averages, which often mask risk, and instead provides **Percentile-based outcomes** (e.g., P85 "Likely" confidence).

1. **Ingestion**: The server fetches full Jira changelogs via a centralized ingestion layer, using **Backward Boundary Scanning** to reconstruct project-local history while calculating exact residency time (in seconds) for every item across every status.
2. **Context Resolution**: Statuses are mapped to a meta-workflow (Demand → Upstream → Downstream → Finished) to ensure the simulation "clock" reflects actual value consumption.

3. **Simulation & Validation**: The engine simulates potential futures and optionally validates them via walk-forward backtesting to ensure historical reliability.
4. **Diagnostic Guidance**: An AI-orchestrated **Roadmap** tool guides agents through a sequence of diagnostic steps.

---

## ⚠️ Probabilistic Nature & Disclaimer

MCS-MCP is a statistical tool. It generates **probabilistic forecasts** based on historical performance, not guarantees.

- **No Direct Answer**: A forecast saying "85% confidence by Oct 12" means there is a 15% chance it will take longer.
- **Garbage In, Garbage Out**: Results are strictly dependent on the quality and consistency of your Jira data.
- **No Liability**: This tool is provided "AS IS". The authors and contributors are not responsible for any project delays, financial losses, or business decisions made based on its output.

---

## 🏃 Getting Started

### Prerequisites

- Access to Atlassian Jira (Data Center or Cloud)
- A MCP-capable AI Agent to chat with
- Recent Version of Go if you want to build yourself

### Mini-How-To

- Build or download a release
- Configure the server via `.env`
- Configure a AI Agent to use it as an MCP tool
- Chat:
    - Ask the Agent to look a Project and then a Board
    - Ask the Agent to discover the workflow
    - Ask the Agent for what the MCP-Server can do or the analytical roadmap

### Authentication

The server supports both Personal Access Tokens (PAT) and session-based (cookie) authentication.

**Option A: Personal Access Token (PAT) - Preferred**
Configure your Jira PAT in the `.env` file (example file included):

```env
JIRA_TOKEN=your-personal-access-token
```

**Option B: Session Cookies - Fallback**
If PAT is not available, provide session cookies extracted from an active browser:

- `JIRA_SESSION_ID`: Your Jira session ID.
- `JIRA_XSRF_TOKEN`: Your XSRF token.
- `JIRA_REMEMBERME_COOKIE`: Your Jira RememberMe cookie. (Optional, but recommended for long-running sessions)
- (optional) `JIRA_GCILB`, `JIRA_GCLB`: Actually these are Google-Cloud Load Balancer Cookies.

### Building from Sources

**On Windows (PowerShell):**

```powershell
.\build.ps1 build
```

**On Unix/Linux (Make):**

Untested, but should work.

```bash
make build
```

The resulting binary will be located in the `dist/` folder (e.g., `dist/mcs-mcp.exe`)
along with a exemplary `.env` file.

### Configuring as an MCP Tool

To use as a server for an AI Agent (like Claude or Gemini), point your MCP client configuration to the compiled binary:

```json
{
	"mcpServers": {
		"mcs-mcp": {
			"command": "C:/path/to/mcs-mcp/dist/mcs-mcp.exe",
			"args": []
		}
	}
}
```

Make sure that the Server can write to this directory to create `cache` and `logs` folders - or reconfigure using `DATA_PATH`.

---

## Usage Tips:

- Many Agents can create Charts right from the data that's passed from the MCP-Server. In Claude Desktop a prompt like _"Please create a Chart for WIP; use React and Recharts with a dark color scheme."_ works well.

---

## 🎲 Offline Testing & Simulation (mockgen)

If you do not have a live Jira connection (or simply want to test the server's analytical capabilities without using sensitive corporate data), MCS-MCP includes a built-in mock data generator called `mockgen`.

This tool produces a standardized, simulated dataset inside your `cache` folder that you can analyze using the `"MCSTEST"` project key. The server ships with this synthetic dataset configured, enabling immediate out-of-the-box evaluation.

For comprehensive details on how to use `mockgen`, configure data distributions (mild, chaotic, drifted), and rebuild the cached simulation, please check the **[Mock Data Generator Guide](docs/mockdata.md)**.

---

## 📖 Guided Interaction

MCS-MCP is designed to be used by AI Agents as a "Technical Co-Pilot". For detailed guidance on specific workflows, refer to:

- **[Project Charter](docs/charter.md)**: Conceptual foundations and architectural principles.
- **[Interaction Use Cases](docs/use-cases.md)**: Detailed scenarios for PMs and AI Agents (When, Scope, Bottlenecks, Backtesting, etc.).
- **[Architecture Deep-Dive](docs/architecture.md)**: Aging math, backflow policies, and the status-granular flow model.
- **[Mock Data Generator](docs/mockdata.md)**: Instructions for using `mockgen` and manipulating the `MCSTEST` synthetic sandbox.

---

## ⚖️ Conceptual Integrity

This project adheres to the core principles of **Cohesion, Coherence, and Consistency**. Every tool and analytical model is designed to provide a unified, reliable view of delivery performance without administrative noise.

---

## 📜 License

This project is licensed under the **Apache License 2.0**. See the [LICENSE](LICENSE) and [NOTICE](NOTICE) files for details.

Copyright © 2026 Bruno Baketarić.
