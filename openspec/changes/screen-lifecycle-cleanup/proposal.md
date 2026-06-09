# SDD Proposal — screen-lifecycle-cleanup

> PRD for automatic EventBus unsubscription and resource cleanup on screen navigation in GolemUI.

## 1. Problem Statement

GolemUI leaks memory on every screen navigation. When `ui.Navigate` replaces the active screen's widget tree in `mainContainer`, the previous screen's EventBus subscribers remain registered indefinitely. Each `data_grid` on the old screen pins its `dataGridModel`, `*widget.Table`, `*ScreenState`, and `NodeMeta` via the subscriber closure stored in `LocalEventBus.subscribers`.

Additionally, in-flight `loadMasterBuffer` and `fetchGridDataAsync` goroutines for the old screen continue running because `model.cancel` (a `context.CancelFunc`) is never invoked on screen replacement.

**Impact per navigation:**

| Resource leaked | Per data_grid | Compounds? |
|----------------|---------------|------------|
| EventBus subscriber closure | 1 | Yes — re-visiting a screen re-subscribes |
| `dataGridModel` struct (rows, headers, masterRows) | 1 | Yes |
| `*widget.Table` Fyne widget | 1 | Yes |
| `*ScreenState` and its `data` map | 1 (shared per screen) | Yes |
| `NodeMeta` for the data_grid | 1 | Yes |
| In-flight DB query goroutine | 1–2 | Yes |
| `context.CancelFunc` (never called) | 1 | Yes |

A screen with N data_grids leaks N subscriber closures (plus captured state) every time the user navigates away. The `LocalEventBus.subscribers` map grows monotonically for the process lifetime.

**Root cause:** The leak is purely a call-site problem. The EventBus already provides `Unsubscribe(channel, subID)` and the `dataGridModel` already stores `model.unsubscribe func()` and `model.cancel context.CancelFunc` — but nothing invokes them on screen replacement.

## 2. Proposed Solution

### 2.1 Compose returns a cleanup function

Change the `Compose` signature to return a teardown function alongside the widget:

```go
// Before
func Compose(node NodeMeta, vistaID string) (fyne.CanvasObject, error)

// After
func Compose(node NodeMeta, vistaID string) (fyne.CanvasObject, func(), error)
```

The returned `func()` is an idempotent cleanup that:
1. Calls `model.cancel()` for every `dataGridModel` in the composed tree — cancels in-flight goroutines.
2. Calls `model.unsubscribe()` for every `dataGridModel` — removes the subscriber from `LocalEventBus`.

The cleanup function captures all necessary state (models, cancel funcs, unsubscribe funcs) during composition. No package-level registry is needed.

### 2.2 Navigate invokes teardown before composing new screen

Modify `ui.Navigate` in `main.go` to:

1. Store the previous screen's cleanup func in a closure variable (`prevCleanup`).
2. Before composing the new screen, call `prevCleanup()` to release the old screen's resources.
3. After composing, update `prevCleanup` with the new screen's cleanup func.

```go
var prevCleanup func()

ui.Navigate = func(vID string) {
    log.Printf("[UI/Navigation] Navigating to screen %q", vID)
    go func() {
        // Teardown previous screen
        if prevCleanup != nil {
            prevCleanup()
            prevCleanup = nil
        }

        node, err := ui.LoadScreen(ctx, ui.CorePool, vID, cfg.LayoutQuery)
        if err != nil { ... return }

        newUI, cleanup, err := ui.Compose(node, vID)
        if err != nil { ... return }

        prevCleanup = cleanup

        fyne.Do(func() {
            mainContainer.Objects = []fyne.CanvasObject{newUI}
            mainContainer.Refresh()
            navTree.SelectByVistaID(vID)
        })
    }()
}
```

### 2.3 Bootstrap home screen also gets cleanup

The initial home screen composed during `RunBootstrap` also calls `Compose`. Its cleanup func is stored in `prevCleanup` so that the first `ui.Navigate` tears it down correctly.

## 3. Key Decisions and Rationale

### Decision 1: Compose returns `(CanvasObject, func(), error)` — not a registry

**Options considered:**
- (A) Package-level `ActiveUnsubscribes []func()` registry — compositor appends, Navigate drains.
- (B) `Compose` returns a cleanup func — caller stores and invokes.

**Chosen: B.** Rationale:
- No shared mutable state between packages.
- The cleanup func is self-contained and carries its own context.
- Easier to test: call `Compose`, call cleanup, assert zero subscribers.
- The original spec suggested option A, but the explore phase revealed that option B is simpler and sufficient given that `Compose` is called exactly once per navigation.

### Decision 2: Cleanup runs before new Compose, not after

The teardown cancels old goroutines and unsubscribes old handlers **before** `LoadScreen` + `Compose` for the new screen. Rationale:
- Old goroutines may be holding DB connections; cancelling them frees resources for the new screen's queries.
- Unsubscribing before compose prevents any race between a stale handler and the new composition.

### Decision 3: Cleanup func is idempotent

Double-invocation must not panic or cause side effects. This protects against edge cases where `prevCleanup` might be called more than once (e.g., rapid navigation before the goroutine finishes).

### Decision 4: No EventBus API change

The existing `Subscribe → string` and `Unsubscribe(channel, subID)` are sufficient. Adding a return-type change to `Subscribe` (returning `(string, func())` as the original spec suggested) would require updating all callers and tests for no additional benefit — the compositor already wraps the subID in a cleanup closure.

### Decision 5: No per-screen EventBus

Each screen shares the process-wide `LocalEventBus`. Cleanup targets individual subscriptions via subID, not by replacing the bus. This avoids changing the bus initialization or the `App` struct.

## 4. Non-Goals

- **No EventBus API refactor.** The `Subscribe`/`Unsubscribe` interface remains unchanged.
- **No widget tree walking at teardown time.** The cleanup func captures state during composition; no post-hoc tree traversal needed.
- **No `UnsubscribeAll(channel)` method on EventBus.** Per-subID cleanup is sufficient and more precise.
- **No loading indicator or transition animation.** Out of scope for this change.
- **No non-data_grid cleanup.** Other components (labels, buttons, text inputs) do not subscribe to the EventBus and have no resources to clean up.
- **No sidebar or navigation menu changes.**
- **No GC finalizer or weak-reference patterns.** Go does not support them reliably; explicit cleanup is correct.

## 5. Risks and Mitigations

| Risk | Severity | Mitigation |
|------|----------|------------|
| `Compose` signature change breaks all callers and tests | Medium | Signature change is mechanical: callers add a `func()` capture. All call sites are internal (main.go, tests). Grep shows ~5 call sites. |
| Cleanup func called from wrong goroutine (non-UI goroutine calling UI teardown) | Low | The cleanup func only calls `model.cancel()` and `LocalEventBus.Unsubscribe()` — both are concurrency-safe. No UI mutation happens inside cleanup. |
| Race between `prevCleanup` access from concurrent `Navigate` calls | Low | `Navigate` already runs inside a goroutine. Rapid clicks spawn multiple goroutines that race on `prevCleanup`. Mitigate with a mutex or sync the teardown inside `fyne.Do`. Alternatively, accept "last wins" semantics (same as current navigation) and protect `prevCleanup` with `sync.Once` or a channel. |
| In-flight goroutine calls `table.Refresh()` after cleanup | Low | `model.cancel()` sets the context to cancelled. Goroutines check `ctx.Err()` before calling `table.Refresh()`. If they race past the check, `table.Refresh()` on a detached widget is a no-op (widget not in the tree). |
| Existing tests fail after `Compose` signature change | Medium | All tests call `Compose` and capture `(obj, err)`. Update to `(obj, cleanup, err)` and `defer cleanup()` in each test. Mechanical change. |

## 6. Success Criteria

1. **No memory leak per navigation.** After navigating away from a screen with N data_grids, the `LocalEventBus` has zero subscribers on the old screen's submit channel (`screen:submit:<oldVistaID>`).

2. **Goroutine cleanup.** After teardown, in-flight `loadMasterBuffer` and `fetchGridDataAsync` goroutines for the old screen receive a cancelled context and terminate.

3. **Test coverage.**
   - Test that `Compose` returns a cleanup func, and invoking it reduces subscriber count to zero.
   - Test that `Navigate` tears down the previous screen's subscriptions.
   - Test that the cleanup func is idempotent (double-call does not panic).

4. **No regressions.** All existing tests pass with the updated `Compose` signature. `go vet ./...` clean.

5. **Minimal diff.** No new package-level state, no EventBus API changes, no structural refactor beyond the `Compose` return tuple and `Navigate` teardown call.

## 7. Files Affected

| File | Change |
|------|--------|
| `pkg/ui/compositor.go` | `Compose` signature → `(CanvasObject, func(), error)`. Internal: collect `model.cancel` + `model.unsubscribe` per data_grid, return composite cleanup. |
| `cmd/golemui/main.go` | `ui.Navigate` stores `prevCleanup`, calls it before new `Compose`. Bootstrap also captures cleanup. |
| `pkg/ui/compositor_test.go` | Update all `Compose` call sites to capture cleanup func. Add teardown tests. |
| `cmd/golemui/main_test.go` | Update `Compose` calls in bootstrap tests. Add navigation cleanup test. |

## 8. Estimated Scope

| Category | Lines |
|----------|-------|
| Production code | ~40–60 |
| Test code | ~80–100 |
| Total | ~120–160 |
