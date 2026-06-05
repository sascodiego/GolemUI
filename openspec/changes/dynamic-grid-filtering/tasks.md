# Tasks: Dynamic Grid Filtering

## Workload Forecast
Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: stacked-to-main
400-line budget risk: Low

## Implementation Tasks

### Phase 1: Structs & Package Variables
- [ ] In [pkg/ui/compositor.go](file:///src/GolemUI/pkg/ui/compositor.go), import `"GolemUI/pkg/eventbus"` and `context`.
- [ ] Declare global package variable `var LocalEventBus eventbus.EventBus`.
- [ ] Update `dataGridModel` struct with `cancel context.CancelFunc` and `unsubscribe func()`.

### Phase 2: Refactor fetchGridDataAsync & Text Input Publisher
- [ ] In [pkg/ui/compositor.go](file:///src/GolemUI/pkg/ui/compositor.go), update signature of `fetchGridDataAsync` to accept `args ...any` and pass to `BusinessPool.Query(ctx, query, args...)`.
- [ ] Add `ctx.Err()` checks in `fetchGridDataAsync`: before querying, in the row scan loop, and before writing results.
- [ ] In `Compose()`, modify case `"text_input"` to publish changed text to `LocalEventBus` under `node.BindTo` if `node.BindTo != ""` and `LocalEventBus != nil`.

### Phase 3: Update Compose() data_grid Subscription
- [ ] In [pkg/ui/compositor.go](file:///src/GolemUI/pkg/ui/compositor.go) case `"data_grid"`, wrap the initial loading context with `context.WithCancel` and save to `model.cancel`.
- [ ] Subscribe `"data_grid"` to `LocalEventBus` if `node.BindTo != ""` and `LocalEventBus != nil`.
- [ ] In the subscriber callback, lock, cancel the previous context, create a new cancellable context, save to `model.cancel`, unlock, and call `fetchGridDataAsync` with the new event payload.
- [ ] Store the unsubscribe function on `model.unsubscribe`.

### Phase 4: Wire LocalEventBus in Bootstrap
- [ ] In [cmd/golemui/main.go](file:///src/GolemUI/cmd/golemui/main.go), assign `ui.LocalEventBus = eb` in `RunBootstrap` after `eb := eventbus.NewEventBus()`.

### Phase 5: Testing & Verification
- [ ] In [pkg/ui/compositor_test.go](file:///src/GolemUI/pkg/ui/compositor_test.go), add `TestCompose_DataGrid_ReactiveFiltering` using `MockDBPool` to register query stubs with query parameters.
- [ ] Verify entry changes with `test.Type(entry, "Book A")` trigger parameterized grid queries, and sequential rapid entries cancel previous query contexts.
- [ ] Run `go test ./...` and verify test suite passes.
