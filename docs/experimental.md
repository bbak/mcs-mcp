# Experimental Features

This file documents active experimental features — code paths that are under hypothesis
validation and have not yet been declared stable. Experimental features are:

- **Off by default** — the operator must explicitly enable the gate.
- **Opt-in per session** — the agent/user activates them deliberately via `set_experimental`.
- **Subject to removal** — if a hypothesis is rejected, the code and this entry are deleted.
- **Documented here, not in `architecture.md`** — once an experiment graduates to stable, its
  documentation moves to `architecture.md` and the flag gate is removed.

## Lifecycle

```
hypothesis → implemented (flag-gated) → validated → stable (gate removed, docs moved)
                                       ↓
                                  rejected (code deleted, entry removed)
```

## Activation

Two steps are required — both must be satisfied for experimental paths to execute:

**Step 1 — Operator enables the gate** (in `.env` next to the binary):

```env
MCS_ALLOW_EXPERIMENTAL=true
```

When false (default), `set_experimental` returns an error and experimental paths are
unreachable regardless of what the agent requests.

**Step 2 — Agent activates experimental mode for the session**:

```
set_experimental(enabled: true)
```

This persists for the duration of the session. It is not reset when switching boards — the
user controls it explicitly. Call `set_experimental(enabled: false)` to return to stable
behavior.

---

## Active Experiments

*No active experiments.*
