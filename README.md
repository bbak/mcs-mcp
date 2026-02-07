# MCS-MCP: Monte-Carlo Simulation for Model Context Protocol

**MCS-MCP** is a sophisticated Model Context Protocol (MCP) server that empowers AI assistants with deep analytical and forecasting capabilities for software delivery projects. By leveraging historical Jira data and high-performance Monte-Carlo simulations, it transforms raw project history into actionable, probabilistic delivery insights with a strong focus on **mathematical hardening and defensive design**.

> [!WARNING]
> Currently, this must be considered _alpha_. While it works quite well,
> the Math just partially verified. Don't bet your bonus on the
> forecasts and analysis done by it. Concepts are subject to change, if necessary
> to make an AI Agent behave the way I envision.
> I run it in Claude Desktop and Antigravity Agents.

---

## 🚀 Key Capabilities

- **Monte-Carlo Forecasting**: Run 10,000+ simulations to answer "When will it be done?" (Duration) or "How much can we do?" (Scope) with high statistical confidence.
- **Forecast Backtesting**: Perform **Walk-Forward Analysis** to empirically validate forecast accuracy by "time-travelling" into historical data.
- **Predictability Guardrails**: Use **XmR Control Charts** and **Stability Indices** to detect "Special Cause" variation and assess if a process is stable enough to forecast.
- **Workflow Semantic Discovery**: Automatically infer the roles of workflow statuses (Active, Queue, Demand, Finished) to identify true bottlenecks instead of administrative delays.
- **High-Fidelity Aging Analysis**: Track **WIP Age** and status-level persistence to identify "neglected" inventory before it impacts delivery.
- **Strategic Evolution Tracking**: Perform longitudinal audits using **Three-Way Control Charts** (Weekly/Monthly) to detect systemic improvements or process drift over time.
- **Process Yield & Abandonment**: Quantify "waste" by identifying exactly where work is discarded in the discovery or execution pipeline.
- **Guided Analytical Roadmaps**: Proactively guide AI agents through the correct sequence of diagnostic steps (Stability -> Discovery -> Analysis) based on specific goals.

---

## 🛠️ How it Works

MCS-MCP operates on the principle of **Data-Driven Probabilism**. It avoids single-point averages, which often mask risk, and instead provides **Percentile-based outcomes** (e.g., P85 "Likely" confidence).

1. **Ingestion**: The server fetches full Jira changelogs via a centralized ingestion layer, calculating exact residency time (in seconds) for every item across every status.
2. **Context Resolution**: Statuses are mapped to a meta-workflow (Demand -> Upstream -> Downstream -> Finished) to ensure the simulation "clock" reflects actual value consumption.
3. **Simulation & Validation**: The engine simulates potential futures and optionally validates them via walk-forward backtesting to ensure historical reliability.
4. **Diagnostic Guidance**: An AI-orchestrated **Roadmap** tool guides agents through the correct sequence of diagnostic steps.

---

## ⚠️ Probabilistic Nature & Disclaimer

MCS-MCP is a statistical tool. It generates **probabilistic forecasts** based on historical performance, not guarantees.

- **No Direct Answer**: A forecast saying "85% confidence by Oct 12" means there is a 15% chance it will take longer.
- **Garbage In, Garbage Out**: Results are strictly dependent on the quality and consistency of your Jira data.
- **No Liability**: This tool is provided "AS IS". The authors and contributors are not responsible for any project delays, financial losses, or business decisions made based on its output.

---

## 🏃 Getting Started

### Prerequisites

- Go 1.25+
- Access to Atlassian Jira (Data Center or Cloud)
- A MCP-capable AI Agent to chat with

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
- (Optional) `JIRA_REMEMBERME_COOKIE`, `JIRA_GCILB`, `JIRA_GCLB`.

### Building and Running the Server

You should build the project into an executable before configuring it as an MCP tool.

**On Windows (PowerShell):**

```powershell
.\build.ps1 build
```

**Developer Verification:**

```powershell
.\build.ps1 verify
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

## 📖 Guided Interaction

MCS-MCP is designed to be used by AI Agents as a "Technical Co-Pilot". For detailed guidance on specific workflows, refer to:

- **[Project Charter](docs/charter.md)**: Conceptual foundations and architectural principles.
- **[Interaction Use Cases](docs/use-cases.md)**: Detailed scenarios for PMs and AI Agents (When, Scope, Bottlenecks, Backtesting, etc.).
- **[Architecture Deep-Dive](docs/architecture.md)**: Aging math, backflow policies, and the status-granular flow model.

---

## ⚖️ Conceptual Integrity

This project adheres to the core principles of **Cohesion, Coherence, and Consistency**. Every tool and analytical model is designed to provide a unified, reliable view of delivery performance without administrative noise.

---

## 📜 License

This project is licensed under the **Apache License 2.0**. See the [LICENSE](LICENSE) and [NOTICE](NOTICE) files for details.

Copyright © 2026 Bruno Baketarić.
