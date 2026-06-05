# Tasks: Screen State Store

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | 380–480 |
| 400-line budget risk | Medium |
| Chained PRs recommended | No |
| Suggested split | single PR (monitor at apply) |
| Delivery strategy | auto-forecast |
| Chain strategy | pending |

Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: pending
400-line budget risk: Medium

### Suggested Work Units

| Unit | Goal | Likely PR | Notes |
|------|------|-----------|-------|
| 1 | ScreenState type + SubmitChannel constant | PR 1 | Foundation; no existing code breaks |
| 2 | NodeMeta extensions + Compose rewrite | PR 1 | Core wiring; depends on Unit 1 |
| 3 | DB seed + integration regression | PR 1 | Verification; depends on Unit 2 |

## Phase 1: Foundation (TDD — RED first)

- [x] 1.1 RED: `pkg/ui/screen_state_test.go` — test `NewScreenState()`, `Set`/`Get`, `Snapshot` defensive copy, concurrent access with `-race` (spec: screen-state-store R1–R3)
- [x] 1.2 GREEN: `pkg/ui/screen_state.go` — implement `ScreenState` struct with `sync.RWMutex`, `Set`, `Get`, `Snapshot` returning `map[string]any`
- [x] 1.3 REFACTOR: clean up ScreenState if needed
- [x] 1.4 RED: `pkg/eventbus/eventbus_test.go` — test `SubmitChannel` constant equals `"screen:submit"` (spec: consolidated-submit R2)
- [x] 1.5 GREEN: `pkg/eventbus/eventbus.go` — add `const SubmitChannel = "screen:submit"`
- [x] 1.6 RED: `pkg/ui/screen_loader_test.go` — test `NodeMeta` deserializes `filter_mode` and `master_data_source` with defaults (spec: polymorphic-grid-filtering R5)
- [x] 1.7 GREEN: `pkg/ui/screen_loader.go` — add `FilterMode string` and `MasterDataSource string` fields to `NodeMeta` in `compositor.go`

## Phase 2: Core Wiring (TDD — RED first)

- [x] 2.1 RED: `pkg/ui/compositor_test.go` — test `text_input` writes to `*ScreenState` via `Set(bind_to, value)` instead of `Publish` (spec: reactive-input-publishing MODIFIED R1)
- [x] 2.2 RED: `pkg/ui/compositor_test.go` — test `button` with `submit_action` publishes `state.Snapshot()` to `eventbus.SubmitChannel` (spec: consolidated-submit R1, R3)
- [x] 2.3 RED: `pkg/ui/compositor_test.go` — test `data_grid` subscribes to `eventbus.SubmitChannel` and dispatches server-mode query with ordered positional args (spec: polymorphic-grid-filtering R1–R2)
- [x] 2.4 GREEN: `pkg/ui/compositor.go` — add `composeWithState(node NodeMeta, state *ScreenState)`; `Compose` creates state and delegates; text_input→`state.Set`; button→`state.Snapshot()`→`Publish(SubmitChannel)`; grid→`Subscribe(SubmitChannel)`
- [x] 2.5 Resolve key-ordering risk: server-mode must use explicit ordered key list (e.g., `FilterKeys []string` on NodeMeta or deterministic alphabetical sort) for `$1, $2, ...` mapping — NOT `map[string]any` iteration
- [x] 2.6 RED: `pkg/ui/compositor_test.go` — test client-mode grid: eager master buffer load at Compose, in-memory filter on SUBMIT (spec: polymorphic-grid-filtering R3–R4)
- [x] 2.7 GREEN: `pkg/ui/compositor.go` — implement client-mode: `FilterMode=="client"` → load `MasterDataSource` once, filter `masterRows` on SUBMIT without `BusinessPool.Query`

## Phase 3: Integration & Regression

- [x] 3.1 Update `docker/init-db/02_init_core.sql` — add sample vista with text_input + button `submit_action` + data_grid to verify end-to-end flow
- [x] 3.2 Run `go test -race ./...` — verify all existing tests pass (breaking change: old `bind_to` direct wiring removed; update `TestCompose_DataGrid_ReactiveFiltering`)
- [x] 3.3 Adapt `TestCompose_DataGrid_ReactiveFiltering` to new SUBMIT-based flow (button click triggers grid re-query)
- [x] 3.4 Verify `cmd/golemui/main.go` — `Compose` now creates `*ScreenState` internally; no bootstrap changes needed per design
