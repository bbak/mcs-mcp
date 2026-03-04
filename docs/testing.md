# Testing & Verification Strategy

The MCS-MCP project uses a high-fidelity mocking system to ensure the reliability and hardening of its mathematical core. This allows developers to verify statistical models (Cycle Time, Throughput, Stability) without requiring access to a live Jira instance.

## The MCSTEST Sandbox

The project key `MCSTEST` is a reserved "Sandbox" identifier. When the server encounters this project key:

1.  **Jira Bypass**: No actual Jira API calls are made.
2.  **Cache-Driven**: The server attempts to load results exclusively from the local `.cache` directory.
3.  **Mock Metadata**: Predefined workflow mappings and project/board metadata are returned.

This allows for deterministic, repeatable testing of analytical tools.

## Mock Data Generator

The generator at `cmd/mockgen/main.go` creates synthetic datasets to simulate different process behaviors.

### Distributions

The generator supports two mathematical models for residency time:

- **Uniform (Default)**: Simple random noise within a fixed range. Good for baseline logic verification.
- **Weibull**: A realistic model for knowledge work cycle times.
    - **Shape ($k$)**: Controls the "tail" of the distribution. $k < 1$ creates heavy tails (highly unpredictable).
    - **Scale ($\lambda$)**: Proportional to the expected duration.

### Scenarios

| Scenario | Behavior (Uniform)  | Behavior (Weibull)                | Purpose                   |
| :------- | :------------------ | :-------------------------------- | :------------------------ |
| `mild`   | CT 6-11 days        | $k=2.5, \lambda=9.5$ (Stable)     | Baseline verification.    |
| `chaos`  | Controlled outliers | $k=0.8, \lambda=12.0$ (Fat Tails) | Outlier/Variance testing. |
| `drift`  | Systemic slowdown   | $k=2.5 \to 0.8$                   | Systemic drift detection. |

### Usage

```powershell
# Generate a Weibull-based Chaos dataset
go run cmd/mockgen/main.go --scenario chaos --distribution weibull --count 300 --out ./.cache
```

## Statistical Regression Suite

We maintain an integration test at `internal/mcp/mock_test.go` that exercises all three scenarios. This suite:

- Automatically generates the required truth data for each scenario.
- Calls the JSON-RPC tools (`import_board_context`, `analyze_status_persistence`, etc.).
- Asserts that the statistical output (P50, P85) matches the expected mathematical profile of the scenario.

### Running the Suite

```powershell
go test -v ./internal/mcp -run TestMCSTEST_Integration
```

This suite ensures that refactors to the internal stats engine or its data structures do not introduce mathematical regressions.

## Golden Handler Tests

Per-handler golden tests at the MCP layer capture the raw `data` + `guardrails` payload each analytical handler sends to the AI Agent (without the `ResponseEnvelope` wrapper). Each handler has its own baseline file under `internal/testdata/golden/mcp/`.

### Running

```powershell
# Verify against existing baselines
go test ./internal/mcp/... -run TestHandlers_Golden -v

# Regenerate baselines after intentional changes
go test ./internal/mcp/... -run TestHandlers_Golden -update
```

### Fixture Integrity

The tests track a SHA-256 hash of `simulated_events.jsonl` in a `.fixtures.sha256` sidecar file. When the fixture changes:

1. All existing handler baselines become invalid.
2. Tests fail with a hash mismatch error.
3. Re-run with `-update` to regenerate baselines.

## Pseudonymize Tool

A CLI utility at `cmd/pseudonymize/main.go` anonymizes real Jira cache files for use as test fixtures. It replaces all issue key prefixes (e.g. `PROJ-123`) with `MOCK-123`, producing files safe to commit to the repository.

### How It Works

1. **Discover**: Scans the input JSONL file for all distinct issue key prefixes (everything before the first `-`).
2. **Rewrite events**: Replaces every `<PREFIX>-` occurrence with `MOCK-` via raw byte substitution.
3. **Rewrite workflow**: Auto-detects the companion workflow file (`<basename>_workflow.json` in the same directory) and replaces the project key portion of `source_id`.

Both output files are written to the target directory with fixed names: `simulated_events.jsonl` and `simulated_workflow.json`.

### Usage

```powershell
go run ./cmd/pseudonymize [--out <dir>] <path/to/SOURCE_BOARDID.jsonl>
```

| Flag    | Default                        | Description                           |
| :------ | :----------------------------- | :------------------------------------ |
| `--out` | `internal/testdata/golden/`    | Output directory for anonymized files |

#### Example

```powershell
go run ./cmd/pseudonymize .cache/ACME_42.jsonl
```

This reads `.cache/ACME_42.jsonl` and `.cache/ACME_42_workflow.json`, replaces all `ACME-` occurrences with `MOCK-`, and writes the results to `internal/testdata/golden/`.

After updating fixtures, regenerate the golden baselines:

```powershell
go test ./internal/mcp/... -run TestHandlers_Golden -update
```

### Warnings

- **Multiple prefixes**: If the input contains events from more than one project, all are mapped to `MOCK-`. The tool warns about potential ID collisions.
- **Already anonymized**: If all keys already use `MOCK-`, the tool reports that no replacements were needed.
