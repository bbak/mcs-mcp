# Anatomy of a Jira Changelog

This document captures the technical characteristics and constraints of the Jira Changelog (History) logic, specifically in the context of the event-sourced analytical engine of MCS-MCP.

## Key Principles

### 1. Atomic Change-Sets

Jira processes updates in atomic "Histories". A single history entry (change-set) often contains multiple field changes sharing the **exact same timestamp** (microsecond precision).

- **Concurrent Field Updates**: It is guaranteed that a transition to a terminal status and the setting of a `resolution` field occur at the same microsecond if they are part of the same action. Within a change-set, the changes are assumed to be consistent.
- **Ordering Constraint**: Order within a change-set is insignificant. Order across change-sets obvisously is significant, as they necessarily have happened in different points in time. Analytical tools be aware that they MUST NOT rely any sorting within
  a change-set, as this is not guaranteed. As stated by the previous point, they can assume consistency within a change-set.

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

### 5. Move History Healing

Logic to "heal" the history from workflow status' originating from other workflows or Projects.

As soon as we encounter a change of either "Key" or "Project" or "Workflow" we:

1.  remember the Timestamp (ts) of the "Created" event of this work item.
2.  move forward in time until we see the next change-set that contains a "Status" change.
3.  We take the "from" Value of this one to create as "created" event with said "from" value as "to" value and the remebered "Created" Timestamp as "ts" for the event. Of course we create the respective "Change" event for the current Status-Change as usual.
4.  This way, we remove the history from a previous workflow and the work item will appear as having not been moved since it was created.

Obviously, we need to cleanup the "in memory" representation of the work item in the same way.

---

## The `TransformIssue` Logic (Backward History Scanning)

To ensure analytical integrity when issues move between projects or change workflows, MCS-MCP uses a **Backward Boundary Scanning** strategy. This approach eliminates the need for complex "healing" heuristics by correctly identifying the moment an issue enters the project's scope.

### Core Principles

1.  **Reverse Chronology**: The history is processed from the most recent change back towards the issue's creation.
2.  **Boundary Detection**: A process boundary is identified when a change-set contains a change in identity (`Key`) moving the issue into the target project.
3.  **Arrival Anchoring**:
    - Once a boundary is detected, processing of further (older) change-sets stops.
    - The state transition at this boundary defines the item's **Arrival Status** in the target project.
    - A synthetic **Created** event is anchored at the issue's biological creation date using this **Arrival Status**.
    - If a **Resolution** is set at the boundary, it is emitted as a distinct `Change` event to ensure correct throughput dating at the point of arrival.

### Processing Conditions (Ordered by Priority)

Within each change-set (traversed backwards):

#### Condition 1: Terminal Move (Project Boundary)

If the change-set contains a change of `Key` (moving into the target project):

- **Birth Status**: If a `status` change is present, use the `ToString` (Target/Arrival Value). This ensures the biological birth is anchored to the state in which it entered our project.
- **Arrival Transition**: We **suppress** the emission of the `status` change event itself, as it is now represented by the `Created` event's status.
- **Resolution**: If a `resolution` change is also present, it is emitted as a separate `Change` event (without status) to capture the completion signal at the arrival timestamp.
- **Termination**: Stop processing further change-sets.

#### Condition 2: Status & Resolution Change

If no boundary is hit, but the change-set contains both `status` and `resolution` changes:

- Emit a `Change` event capturing both.
- If `resolution` is an empty string, set `IsUnresolved: true`.

#### Condition 3: Status-Only Change

If only a `status` change is present:

- Emit a `Change` event for the transition.
- The `From` state of this change becomes the `initialStatus` for the next (older) iteration.

### Finalization

1.  **Reversal**: After processing stops or all history is exhausted, the resulting events are sorted chronologically (ascending).
2.  **Biological Birth**: The `Created` event uses the biological creation timestamp. To preserve correct age while ensuring accurate throughput dating, it is anchored to the **Arrival Status** (the status it had upon entering our scope).
3.  **Throughput Dating**: Analytical tools (like `BuildThroughputProjection`) strictly ignore `Created` events for delivery signals. This prevents items resolved during a move from being back-dated to their biological birth.
4.  **Snapshot Fallback**: If the Jira DTO has a `ResolutionDate` that isn't already captured in history (within a 2-second grace period), a final `Change` event is appended to capture the terminal resolution.

---

## Analytical Guardrails

1. **Never Sort by EventType String**: Alphabetical sorting (`Resolved` < `Transitioned`) is a logic error. If a tie-breaker is needed, use the original array order from the Jira DTO or a predefined semantic priority.
2. **Outcome Fallback**: When `resolution` is missing or inconsistent, the semantic mapping provided by the user (Statuses to Outcomes) takes precedence for throughput and yield calculations.
