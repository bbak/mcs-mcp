# Anatomy of a Jira Changelog

This document captures the technical characteristics and constraints of the Jira Changelog (History) logic, specifically in the context of the event-sourced analytical engine of MCS-MCP.

## Key Principles

### 1. Atomic Change-Sets

Jira processes updates in atomic "Histories". A single history entry (change-set) often contains multiple field changes sharing the **exact same timestamp** (microsecond precision).

- **Concurrent Field Updates**: It is guaranteed that a transition to a terminal status and the setting of a `resolution` field occur at the same microsecond if they are part of the same action.
- **Ordering Constraint**: Within a single `History` entry, the order of `Items` is significant. Analytical tools MUST NOT rely on alphabetical sorting of `EventType` for tie-breaking, as this destroys the semantic sequence of the change-set.

### 2. Resolution Lifecycle

The `resolution` field is the primary signal for delivery outcome, but its reliability depends on Jira workflow configuration.

- **Terminal Entry**: On transition INTO a terminal status, the `resolution` field should be set.
- **Terminal Exit**: On transition OUT of a terminal status, the `resolution` field MUST be cleared.
- **Timestamp Synchronization**: If the `resolution` is set or cleared in a change-set, the associated `resolutionDate` (if provided by the Jira API) aligns with the change-set `Created` timestamp (allowing for lower clock precision).
- **Inconsistency Flags**: If an item is found in a terminal status without a resolution, or in a non-terminal status WITH a resolution, the system should treat the `resolution` field as unreliable for that source and fall back to status-based outcome mapping (`delivered` vs `abandoned`).

### 3. Event-Sourced Duality

The system derives its state by replaying two primary event types:

- **Status Events (`Transitioned`)**: Capture the location of the work item in the process tiers.
- **Outcome Events (`Resolved` / `Unresolved`)**: Capture the objective result of the work.

### 4. The "Resolution Wipe" Risk

Reconstruction logic that wipes `resolutionDate` upon seeing a `Transitioned` event must be extremely defensive.

- **Wait for Terminal Decision**: If a `Transitioned` event occurs at the same timestamp as a `Resolved` event, the decision to clear the resolution must wait until the FINAL state of that timestamp "transaction" is known.
- **Terminal-to-Terminal**: Moves between two statuses marked as "Finished" MUST NOT clear the resolution.

---

## Analytical Guardrails

1. **Never Sort by EventType String**: Alphabetical sorting (`Resolved` < `Transitioned`) is a logic error. If a tie-breaker is needed, use the original array order from the Jira DTO or a predefined semantic priority.
2. **Outcome Fallback**: When `resolution` is missing or inconsistent, the semantic mapping provided by the user (Statuses to Outcomes) takes precedence for throughput and yield calculations.
