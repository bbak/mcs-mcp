# Mock Data Generator (mockgen)

The `mockgen` utility is a standalone tool included with the MCS MCP Server. It generates synthetic dataset files formatted as Jira exports (`.jsonl`), allowing you to test and interact with the MCP server's analytical capabilities without requiring a live connection to a Jira instance.

This is especially useful for:

- Testing the MCP server in a sandboxed, offline environment.
- Providing a reproducible set of data to an AI Assistant to verify statistical calculations.
- Verifying the mathematical bounds of Cycle Time, Throughput, and Process Stability without PII or sensitive corporate data.

## Getting Started

By default, the MCS MCP Server intercepts queries for a particular hardcoded project key — `"MCSTEST"` — and routes those requests to an internal, locally cached test dataset. `mockgen` is designed to produce this exact test dataset.

### 1. Build the Generator

If `mockgen.exe` is not already in your distribution folder after running the primary build script, you can compile it manually from the repository root:

```bash
go build -o mockgen.exe ./cmd/mockgen
```

### 2. Generate the Mock Dataset

Run the newly built executable from the root of the repository, pointing the output to the server's `.cache` folder:

```bash
./mockgen.exe --out=./cache
```

This will generate two files. Move them to the MCS-Server's `cache` directory in
the same folder as the MCS-Server executable (unless you specified a different cache
directory when starting the server):

- `MCSTEST_0.jsonl`: The simulated event log data.
- `MCSTEST_0_workflow.json`: The layout of statuses mimicking a typical Jira workflow.

### 3. Usage with MCP

Load your MCS MCP Server in your preferred AI Assistant (like Claude Desktop). Instead of querying a real Jira project key, ask the assistant to analyze the `"MCSTEST"` project and the `"MCSTEST"` board. Then approve the workflow 'as-is'.

The MCP server will use the generated mock data instead of hitting the live Jira API.

For example:

> "Run a cycle time assessment"
>
> "Analyze process stability and throughput"
>
> "How old is the current WIP?"

---

## Configuration Options

`mockgen` allows you to customize the severity and statistical distribution of the dataset to verify different mathematical behaviors.

| Flag             | Default     | Options                        | Description                                                                                                                                                                                            |
| ---------------- | ----------- | ------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `--count`        | `200`       | Any integer                    | The number of total work items (issues) to simulate.                                                                                                                                                   |
| `--scenario`     | `"mild"`    | `"mild"`, `"chaos"`, `"drift"` | The condition of the simulated system. `"mild"` presents a healthy stable process, `"chaos"` introduces random heavy variations, and `"drift"` simulates a process where cycle times double over time. |
| `--distribution` | `"uniform"` | `"uniform"`, `"weibull"`       | The statistical shape of the generated cycle times. `"weibull"` represents heavily right-skewed fat-tailed historical data, while `"uniform"` offers tightly controlled variation.                     |
| `--out`          | `"./cache"` | Any path                       | The target directory for the generated `.jsonl` and `_workflow.json` files.                                                                                                                            |

**Example: Generating a chaotic process with right-skewed cycle times:**

```bash
./mockgen.exe --scenario=chaos --distribution=weibull --count=500 --out=./cache
```
