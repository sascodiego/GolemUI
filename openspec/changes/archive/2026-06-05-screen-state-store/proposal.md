# Proposal: Screen State Store

## Intent

Evolve GolemUI from direct point-to-point reactivity (input → EventBus → grid) to a centralized per-screen state model with a consolidated SUBMIT event and polymorphic grid filtering (server-side parameterized queries vs client-side in-memory). This eliminates keystroke-level network chatter and enables client-side filtering for reference datasets.

## Scope

### In Scope
- `ScreenState` store: thread-safe `map[string]any` with `sync.RWMutex` + `Snapshot()`
- Indirect publishing: inputs write to store instead of publishing directly
- SUBMIT event: button publishes `state.Snapshot()` via `eventbus.SubmitChannel`
- Polymorphic filtering: `FilterMode` on `NodeMeta` dispatches server (`$1,$2,…` via `BusinessPool`) or client (in-memory filter over master buffer)
- Eager master-buffer loading for client-mode grids during `Compose`

### Out of Scope
- Pagination / streaming for large master buffers
- Views without grids
- Non-Fyne toolkits
- Multi-window SUBMIT isolation (single-window MVP)

## Capabilities

### New Capabilities
- `screen-state-store`: Per-screen centralized state with thread-safe read/write and snapshot semantics. Inputs write to store; button reads snapshot for SUBMIT.
- `consolidated-submit`: Button triggers a single SUBMIT event carrying the full state snapshot, replacing per-input broadcasting.
- `polymorphic-grid-filtering`: Grid dispatches to server-mode (parameterized SQL) or client-mode (in-memory row filter) based on `FilterMode` metadata.

### Modified Capabilities
- `reactive-input-publishing`: Input `OnChanged` writes to `*ScreenState` instead of publishing to EventBus directly. Old `bind_to` channel publishing is removed.
- `parametrized-grid-filtering`: Grid subscribes to `SubmitChannel` instead of `bind_to`; payload is a `map[string]any` snapshot (positional args extracted by key order) rather than a single string.
- `composite-layout-engine`: `Compose` threads a per-screen `*ScreenState` through the recursive factory via internal `composeWithState`.

## Approach

Thread a `*ScreenState` through `Compose` (A1). Inputs call `state.Set(node.BindTo, value)` on change. Buttons with `submit_action` publish `state.Snapshot()` to fixed `eventbus.SubmitChannel` (C1, B1). Grids subscribe to `SubmitChannel` and dispatch by `FilterMode` (D1): server-mode reuses existing `BusinessPool.Query` with positional args from snapshot; client-mode filters pre-loaded `masterRows` in-memory (E1). Public `Compose` signature is preserved; internal recursion uses `composeWithState(node, state)`.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `pkg/ui/screen_state.go` | New | `ScreenState` type with `Set`, `Get`, `Snapshot` |
| `pkg/ui/compositor.go` | Modified | Compose threads state; text_input writes to store; button publishes SUBMIT; grid subscribes to SubmitChannel with polymorphic dispatch |
| `pkg/ui/screen_loader.go` | Modified | `NodeMeta` gains `FilterMode`, `MasterDataSource` fields |
| `cmd/golemui/main.go` | Modified | Creates `ScreenState` per screen; eager master-data query for client-mode grids |
| `docker/init-db/02_init_core.sql` | Modified | Sample vista includes button with `submit_action` and multi-input grid |
| `pkg/ui/compositor_test.go` | Modified | New scenarios for state convergence, SUBMIT, and client-mode filtering |
| `pkg/eventbus/eventbus.go` | None | Already supports arbitrary payloads and fixed channel names |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Inputs with `bind_to` but no button stop notifying grid | Medium | Breaking change — remove old `bind_to` subscription path; require button for grid reactivity. Document in release notes. |
| PR budget breach (350–500 lines) | Med-High | Split into 3 chained PRs: (1) ScreenState + Compose threading, (2) SUBMIT button + grid subscription refactor, (3) client-mode filtering + SQL seed |
| Concurrency: UI goroutine vs EventBus goroutine | Low | `sync.RWMutex` on ScreenState; `Snapshot()` returns defensive copy; race-detector test |
| Master data memory cost | Low | Document limit; defer streaming to future change |

## Rollback Plan

Revert to direct `bind_to` publishing by removing `ScreenState` from `Compose`, restoring `entry.OnChanged → Publish(bind_to, value)`, and removing `SubmitChannel` subscription from grid. No database schema migration required — `FilterMode`/`MasterDataSource` are additive JSONB fields with safe defaults.

## Dependencies

- None external. Builds on existing `EventBus`, `BusinessPool`, `dataGridModel`, and `NodeMeta` primitives.

## Success Criteria

- [ ] All inputs on a screen converge into a single `ScreenState` snapshot
- [ ] Button click publishes exactly one SUBMIT event with full filter map
- [ ] Server-mode grid receives positional `$1, $2, …` from snapshot keys
- [ ] Client-mode grid filters in-memory without hitting `BusinessPool`
- [ ] Rapid SUBMIT clicks cancel stale queries (existing pattern preserved)
- [ ] `go test -race ./...` passes with zero data races
- [ ] All existing tests continue to pass
