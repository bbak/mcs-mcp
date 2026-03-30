# Experimental Features

This file documents active experimental features — code paths that are under hypothesis
validation and have not yet been declared stable. Experimental features are:

- **Off by default** — the operator must explicitly enable the gate.
- **Subject to removal** — if a hypothesis is rejected, the code and this entry are deleted.
- **Documented here, not in `architecture.md`** — once an experiment graduates to stable, its
  documentation moves to `architecture.md` and the flag gate is removed.

## Lifecycle

```
hypothesis → implemented (flag-gated) → validated → stable (gate removed, docs moved)
                                       ↓
                                  rejected (code deleted, entry removed)
```

---

## Graduated Experiments

### SPA Pipeline → Engine "bbak" (Regime-Aware Sampling)

**Graduated in v0.19.0**

The SPA (Sample-Path Approach) pipeline has been promoted from an experimental feature
to a first-class simulation engine named `"bbak"`. It is no longer gated by
`MCS_ALLOW_EXPERIMENTAL` or the `set_experimental` tool — both have been removed.

**Migration:**

| Before | After |
|--------|-------|
| `MCS_ALLOW_EXPERIMENTAL=true` + `set_experimental(enabled: true)` | `MCS_ENGINE=bbak` |
| No engine selection | `MCS_ENGINE=auto` (backtest-driven selection between crude and bbak) |

See `docs/architecture.md` sections 4.6–4.8 for full documentation of the engine
framework, the Crude engine, and the Bbak engine.

---

## Active Experiments

_No active experiments._
