---
trigger: always_on
description: General user preferences and coding guidelines for the agent to follow.
---

# User Preferences

- Conceptual integrity is important to me. In case some instruction in our conversation is in conflict with that or some solution that you envisioned is, make me aware of that and ask for explicit confirmation. That means that I care for these three things:
  - **Cohesion** measures how closely related the elements within a single module, class, or component are, ideally focusing on one well-defined purpose. High cohesion means functions and data inside a module work together tightly, improving readability, maintainability, and reusability — for example, a class handling only user authentication. Low cohesion scatters unrelated tasks, complicating debugging and changes.​
  - **Coherence** in programming refers to the logical flow and meaningful organization of code or system components, ensuring they form a unified, understandable whole. It emphasizes that responsibilities align sensibly across modules, similar to high cohesion but broader, promoting robustness and ease of comprehension. Coherent code avoids disjointed logic that confuses developers.
  - **Consistency** ensures uniformity in coding style, naming conventions, architecture, and behavior across a codebase or system. It covers standards like indentation, variable naming (e.g., camelCase throughout), and design patterns, reducing errors and easing collaboration in teams. In distributed systems, it also means all nodes share the same data view.​

- Don't assume, ask. Often, there's a good reason why things are as they are.

- Please apply typical Golang community practices regarding file, variable and functions naming conventions as well as directory layout.

- I prefer to use Unix-style line endings in my source files, as stated in my `.editorconfig` file in the root folder. Given that we are on Windows, some tools you use might use Windows-style line endings. Please convert them to Unix-style line endings.

- Given that we are on Windows and the Terminal is Powershell, we don't have Unix tools like `grep` and others available. Use their Powershell counterparts, except for:
  - `rg` - alias for `ripgrep`

- Refer to `docs\charter.md` for the Project Charter.

- Also refer to `docs\use-cases.md` for the Use Cases and `docs\architecture.md` for the Architecture and technical decisions. Make sure you update these, if needed. Don't assume that AI Agents that use the MCP-Server, will have access to these files; they are development artifacts.

- Keep `README.md` up to date when capabilities, setup steps, or key concepts change. It targets end users and operators; `docs\architecture.md` targets AI Agents and developers. Keep the audience distinction in mind when deciding where to document something.

- Never commit to `git` unless explicitly asked to do so. I will do that myself. If you think it is time to commit, ask me and provide a commit message.

- **Current Focus: Reliability & Hardening**: We have shifted focus from feature expansion to mathematical and statistical hardening. Future changes should prioritize:
  - **Defensive Design**: Safeguards against skewed data (e.g., small sample sizes, partially filled months).
  - **Anti-Hallucination**: Strict guardrails preventing AI from guessing when tools fail.
  - **Algorithmic Integrity**: Ensuring metrics (Cycle Time, Throughput) are defensible and calculated with high fidelity.
  - **Feature Freeze**: Avoid adding new analytical features unless they directly support the reliability of existing ones.

- **Development Workflow & Runtime**: I can apply code changes to the project files, but I cannot build, swap, or restart the MCP-Server executable myself. After making code changes that affect the server's behavior, I MUST ask the user to perform a build (e.g., via `build.ps1`) and swap the executable to ensure the changes are reflected in the running environment.

- Use Conventional Commits for git commit messages with the following prefixes:
  - `feat`: A new feature
  - `fix`: A bug fix
  - `refactor`: Code refactoring
  - `docs`: Documentation changes
  - `chore`: Maintenance, scaffolding, or project setup
  - `build`: Build system changes

- Let's optimize for Go 1.25 and newer versions. Don't use outdated patterns or libraries if there's a modern alternative unless it massively reduces the readability of the code.

# Application Constraints

- We're dealing primarily with time-series data. Therefore, we need to be careful with sorting data. Cycle-Times, Moving Ranges and similar may be heaviliy impacted, if the data is not sorted correctly.

- **Golden Test Enforcement for Mathematical Changes**: Any code change that alters analytical, statistical, or mathematical logic in `stats` or `simulation` MUST trigger a run of the end-to-end golden tests. If the mathematical change is intentional and correct, the developer/agent MUST adapt the golden test baseline files (`*.json.actual` -> `*.json`) to permanently lock in the intended behavior before committing.

# Experimental Feature Flag System

The codebase uses a two-layer gate to protect experimental code paths from production use:

- **Layer 1 — Operator gate**: `MCS_ALLOW_EXPERIMENTAL=true` in `.env`. When false (default), the `set_experimental` tool returns an error and experimental paths are completely unreachable.
- **Layer 2 — Session activation**: The agent/user calls `set_experimental(enabled: true)` once at the start of a session. This persists until `set_experimental(enabled: false)` is called — it is **not** reset on board switches. The user controls it explicitly.

Resolution in each handler: `s.allowExperimental && s.experimentalMode`

**Active experiments and their documentation** are in `docs/experimental.md`. Read that file before working on any experimental code path.

**Checklist when adding a new experiment:**

1. Implement the experimental path inline in the relevant handler, guarded by `s.experimentalMode`.
2. Add a structured log line at the call site:

   ```go
   log.Info().Str("tool", "<tool_name>").Bool("experimental", s.experimentalMode).Bool("gate_open", s.allowExperimental).Msg("tool executed")
   ```

3. Add a new section to `docs/experimental.md` with: hypothesis, what it changes, activation conditions, fallback behaviour, known limitations, graduation criteria.
4. Golden tests must pass with the gate off (default) — experimental paths must not affect stable baselines.

**Graduating an experiment to stable:**

Once validated, remove the `s.experimentalMode` guard, remove the entry from `docs/experimental.md`, migrate the documentation to `docs/architecture.md`, and delete `MCS_ALLOW_EXPERIMENTAL` handling if no other experiments remain.
