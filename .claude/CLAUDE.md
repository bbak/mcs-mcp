---
trigger: always_on
description: General user preferences and coding guidelines for the agent to follow.
---

# User Preferences

## Conceptual Integrity

- Conceptual integrity is important. I care for these three things:
  - **Cohesion** measures how closely related the elements within a single module, class, or component are, ideally focusing on one well-defined purpose. High cohesion means functions and data inside a module work together tightly, improving readability, maintainability, and reusability.
  - **Coherence** in programming refers to the logical flow and meaningful organization of code or system components, ensuring they form a unified, understandable whole. Coherent code avoids disjointed logic that confuses developers.
  - **Consistency** ensures uniformity in coding style, naming conventions, architecture, and behavior across a codebase or system. It covers standards like indentation, variable naming (e.g., camelCase throughout), and design patterns, reducing errors and easing collaboration in teams.

## Think Before Coding

**Don't assume. Don't hide confusion. Surface tradeoffs.**

Before implementing:

- State your assumptions explicitly. If uncertain, ask.
- If multiple interpretations exist, present them - don't pick silently.
- If a simpler approach exists, say so. Push back when warranted.
- If something is unclear, stop. Name what's confusing. Ask.

## Simplicity First

**Minimum code that solves the problem. Nothing speculative.**

- No features beyond what was asked.
- No abstractions for single-use code.
- No "flexibility" or "configurability" that wasn't requested.
- No error handling for impossible scenarios.
- If you write 200 lines and it could be 50, rewrite it.

Ask yourself: "Would a senior engineer say this is overcomplicated?" If yes, simplify

## Surgical Changes

**Touch only what you must. Clean up only your own mess.**

When editing existing code:

- Don't "improve" adjacent code, comments, or formatting.
- Don't refactor things that aren't broken.
- Match existing style, even if you'd do it differently.
- If you notice unrelated dead code, mention it - don't delete it.

When your changes create orphans:

- Remove imports/variables/functions that YOUR changes made unused.
- Don't remove pre-existing dead code unless asked.

The test: Every changed line should trace directly to the user's request.

## Goal-Driven Execution

**Define success criteria. Loop until verified.**

Transform tasks into verifiable goals:

- "Add validation" → "Write tests for invalid inputs, then make them pass"
- "Fix the bug" → "Write a test that reproduces it, then make it pass"
- "Refactor X" → "Ensure tests pass before and after"

For multi-step tasks, state a brief plan:
```
1. [Step] → verify: [check]
2. [Step] → verify: [check]
3. [Step] → verify: [check]
```

Strong success criteria let you loop independently. Weak criteria ("make it work") require constant clarification.

## Core Development Documentation Files

- `docs\charter.md`: the project charter. Don't touch.
- `README.md`: User-facing high level project and feature description and usage. Keep up to date.
- `docs\architecture.md`: Guidance for architectural concepts and decisions in the code. **MANDATORY: Read this file at the start of every new session before responding to the first user request.** Keep up to date.
- `docs\use-cases.md` for typical Use Cases, primarily user facing.

# Project Preferences

- Apply typical Golang community practices regarding file, variable and functions naming conventions as well as directory layout.
- Ensure Unix-style line endings.
- I don't have Unix tools like `grep` and others available. Use their Powershell counterparts, except for:
  - `rg` - alias for `ripgrep`
- Never commit to `git` unless explicitly asked to do so. I will do that myself. If you think it is time to commit, ask me and provide a commit message.
- Use Conventional Commits for git commit messages with the following prefixes:
  - `feat`: A new feature
  - `fix`: A bug fix
  - `refactor`: Code refactoring
  - `docs`: Documentation changes
  - `chore`: Maintenance, scaffolding, or project setup
  - `build`: Build system changes
- Optimize for Go 1.25 and newer versions. Don't use outdated patterns or libraries if there's a modern alternative unless it massively reduces the readability of the code.

# Application Constraints

- We're dealing primarily with time-series data. Therefore, we need to be careful with sorting data. Cycle-Times, Moving Ranges and similar may be heaviliy impacted, if the data is not sorted correctly.
- **Golden Test Enforcement for Mathematical Changes**: Any code change that alters analytical, statistical, or mathematical logic in `stats` or `simulation` MUST trigger a run of the end-to-end golden tests. If the mathematical change is intentional and correct, the developer/agent MUST adapt the golden test baseline files (`*.json.actual` -> `*.json`) to permanently lock in the intended behavior before committing.
