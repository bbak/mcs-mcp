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
- Calls the JSON-RPC tools (`get_board_details`, `get_status_persistence`, etc.).
- Asserts that the statistical output (P50, P85) matches the expected mathematical profile of the scenario.

### Running the Suite

```powershell
go test -v ./internal/mcp -run TestMCSTEST_Integration
```

This suite ensures that refactors to the internal stats engine or its data structures do not introduce mathematical regressions.
