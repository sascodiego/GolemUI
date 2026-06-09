# SDD Design — screen-lifecycle-cleanup

> Technical design for automatic EventBus unsubscription and resource cleanup on screen navigation in GolemUI.

## 1. Overview

The change plugs a memory leak where `ui.Navigate` replaces the active screen's widget tree without tearing down the previous screen's EventBus subscribers, in-flight goroutines, or captured resources. The EventBus primitive (`Unsubscribe`) already exists; the gap is call-site plumbing.

**Strategy:** `Compose` returns a cleanup `func()` alongside the widget. `ui.Navigate` stores and invokes the cleanup before composing the next screen. No new package-level state, no EventBus API changes.

**Estimated diff:** ~40–60 lines production, ~80–100 lines tests.

---

## 2. Compose Return Type Change

### 2.1 Before

```go
// compositor.go:62
func Compose(node NodeMeta, vistaID string) (fyne.CanvasObject, error) {
    state := NewScreenState(vistaID)
    return composeWithState(node, state)
}
```

### 2.2 After

```go
func Compose(node NodeMeta, vistaID string) (fyne.CanvasObject, func(), error) {
    state := NewScreenState(vistaID)
    obj, cleanup, err := composeWithState(node, state)
    if err != nil {
        return nil, nil, err
    }
    return obj, cleanup, nil
}
```

The returned `func()` is **always non-nil** — screens without data_grids receive a no-op func. This avoids nil-check sprawl at every call site.

### 2.3 composeWithState Signature Change

```go
// Before
func composeWithState(node NodeMeta, state *ScreenState) (fyne.CanvasObject, error)

// After
func composeWithState(node NodeMeta, state *ScreenState) (fyne.CanvasObject, func(), error)
```

Each case returns its own cleanup func. Containers aggregate children's cleanups.

---

## 3. Per-Component Cleanup Behavior

### 3.1 Container

Collect cleanup funcs from all children. Return a single composed func:

```go
case "container":
    var objects []fyne.CanvasObject
    var cleanups []func()
    for _, child := range node.Children {
        cObj, cCleanup, err := composeWithState(child, state)
        if err != nil {
            return nil, nil, err
        }
        objects = append(objects, cObj)
        if cCleanup != nil {
            cleanups = append(cleanups, cCleanup)
        }
    }
    // ... layout selection unchanged ...
    cleanup := func() {
        for _, c := range cleanups {
            c()
        }
    }
    return containerObj, cleanup, nil
```

**Ordering:** Children are cleaned up in forward order (same as composition order). There is no dependency between child cleanups, so order does not matter functionally, but forward order is predictable.

### 3.2 Leaf nodes (label, text_input, text_area, button)

Return a no-op cleanup:

```go
case "label":
    return widget.NewLabel(node.Label), func() {}, nil

case "text_input":
    // ... existing logic unchanged ...
    return entry, func() {}, nil

case "text_area":
    // ... existing logic unchanged ...
    return entry, func() {}, nil

case "button":
    // ... existing logic unchanged ...
    return btn, func() {}, nil
```

These components do not subscribe to the EventBus and hold no goroutines. The no-op func satisfies the "always non-nil" contract.

### 3.3 data_grid

The critical case. The data_grid already has `model.cancel` and `model.unsubscribe` — assigned but never invoked. The cleanup func wraps both in `sync.Once` for idempotency:

```go
case "data_grid":
    model := &dataGridModel{filterKeys: node.FilterKeys}
    table := widget.NewTableWithHeaders(/* ... unchanged ... */)

    // ... existing model init, ctx/cancel, loadMasterBuffer, fetchGridDataAsync ...
    // ... existing EventBus Subscribe ...
    // ... existing model.unsubscribe assignment ...
    // ... existing table.OnSelected ...

    var once sync.Once
    cleanup := func() {
        once.Do(func() {
            model.mu.Lock()
            cancelFn := model.cancel
            unsubFn := model.unsubscribe
            model.cancel = nil
            model.unsubscribe = nil
            model.mu.Unlock()

            if cancelFn != nil {
                cancelFn()
            }
            if unsubFn != nil {
                unsubFn()
            }
        })
    }

    return table, cleanup, nil
```

**Key details:**

1. **Lock granularity:** The model mutex is held only briefly to read and nil out the two func fields. The actual `cancelFn()` and `unsubFn()` calls happen **outside** the lock. This prevents deadlock: `cancelFn()` cancels the context, which may trigger goroutines that try to acquire `model.mu` — but those goroutines check `ctx.Err()` first and exit, so in practice no contention. However, keeping the calls outside the lock is safer.

2. **Nil-guard after Once:** Even though `sync.Once` prevents double execution, nil-ing out the fields after reading provides defense-in-depth if any code path bypasses `sync.Once` (it shouldn't, but the nil guard is free).

3. **No UI mutation inside cleanup:** `cancelFn()` and `unsubFn()` only touch the context and the EventBus — both concurrency-safe. No `table.Refresh()` or widget mutation happens inside cleanup. This means cleanup can safely run from any goroutine without `fyne.Do()`.

---

## 4. Navigate Teardown (main.go)

### 4.1 Closure variable for previous cleanup

```go
var prevCleanup func()

ui.Navigate = func(vID string) {
    log.Printf("[UI/Navigation] Navigating to screen %q", vID)
    go func() {
        // Tear down previous screen
        if prevCleanup != nil {
            prevCleanup()
            prevCleanup = nil
        }

        node, err := ui.LoadScreen(ctx, ui.CorePool, vID, cfg.LayoutQuery)
        if err != nil {
            log.Printf("[UI/Navigation] Error loading screen %q: %v", vID, err)
            return
        }
        newUI, cleanup, err := ui.Compose(node, vID)
        if err != nil {
            log.Printf("[UI/Navigation] Error composing screen %q: %v", vID, err)
            return
        }
        prevCleanup = cleanup
        fyne.Do(func() {
            mainContainer.Objects = []fyne.CanvasObject{newUI}
            mainContainer.Refresh()
            navTree.SelectByVistaID(vID)
        })
    }()
}
```

### 4.2 Bootstrap home screen

The initial home screen composition also returns a cleanup func:

```go
// Before
homeUI, err := ui.Compose(homeNode, vistaID)

// After
homeUI, homeCleanup, err := ui.Compose(homeNode, vistaID)
if err != nil {
    dbPool.Close()
    return nil, fmt.Errorf("failed to compose home UI: %w", err)
}
prevCleanup = homeCleanup
```

This ensures the first `ui.Navigate` tears down the home screen's subscribers.

### 4.3 Thread safety analysis for prevCleanup

The `prevCleanup` variable is accessed from two places:

1. **Read + write** inside the `ui.Navigate` goroutine (cleanup + assignment).
2. **No other access** — `prevCleanup` is local to `RunBootstrap`, captured by the `ui.Navigate` closure.

The async `fyne-thread-safety` change already runs `ui.Navigate`'s body inside a `go func()`. Rapid clicks spawn multiple goroutines that race on `prevCleanup`.

**Race scenario:** User clicks "Screen A" (goroutine G1 starts), then immediately clicks "Screen B" (goroutine G2 starts). G2 reads `prevCleanup` before G1 writes it — G2 cleans up the old screen (correct), G1 also cleans up the old screen (idempotent, safe). Then G1 writes its cleanup, G2 overwrites it — last writer wins. The net effect is one extra cleanup invocation (safe by idempotency) and the last screen's cleanup is stored (correct).

**Mitigation chosen for first iteration: accept "last wins" semantics.** The cleanup func is idempotent (`sync.Once`), so double-cleanup is a no-op. The worst case is a brief extra EventBus subscription for the screen that was overwritten, which will be cleaned up on the next navigation. This is acceptable because:

- Navigation is user-initiated and infrequent (not a hot loop).
- The leak without this change is unbounded; the race introduces at most one stale subscriber.
- Adding a mutex would add complexity and risk of deadlock for marginal benefit.

If this proves insufficient in production, a `sync.Mutex` protecting `prevCleanup` can be added in a follow-up with zero API changes.

---

## 5. Ordering Invariants

### 5.1 Cleanup before Compose

The teardown runs **before** `ui.Compose` for the new screen. This is critical because:

1. **Goroutine cancellation frees DB connections.** In-flight `loadMasterBuffer` / `fetchGridDataAsync` goroutines for the old screen may be holding DB connections from the pool. Cancelling them frees those connections for the new screen's queries.

2. **Unsubscribe prevents stale handler dispatch.** If the user navigates to a screen with the same `vistaID`, the old subscriber would fire alongside the new one without this ordering.

3. **No deadlock risk.** The cleanup only touches the EventBus and context — no UI operations, no `fyne.Do()`, no widget tree traversal.

### 5.2 prevCleanup assignment before fyne.Do

`prevCleanup = cleanup` is assigned **before** `fyne.Do(...)` swaps the UI. This means:

- If `fyne.Do` blocks (unlikely in production, but possible in tests), the cleanup is already stored.
- On the next navigation, the cleanup will be invoked regardless of whether the UI swap completed.

### 5.3 UI swap inside fyne.Do only

The `mainContainer.Objects` assignment and `Refresh()` remain inside `fyne.Do()` as established by the `fyne-thread-safety` change. The cleanup func does NOT touch the UI, so it runs outside `fyne.Do`.

---

## 6. No EventBus API Changes

The design deliberately avoids modifying `pkg/eventbus/eventbus.go`. The existing surface is sufficient:

| Primitive | Usage |
|-----------|-------|
| `Subscribe(channel, handler) string` | Called in compositor data_grid case (existing) |
| `Unsubscribe(channel, subID)` | Called inside `model.unsubscribe` closure (existing code, newly invoked) |
| `Publish(channel, payload)` | Unchanged — not involved in cleanup |

No new methods, no return type changes on `Subscribe`, no `UnsubscribeAll`, no diagnostics methods. The design is purely call-site plumbing.

---

## 7. Test Strategy

### 7.1 Subscriber count helper

To verify cleanup in tests, add a package-level test helper in `pkg/eventbus/eventbus_test.go`:

```go
// SubscriberCount returns the number of active subscribers for a channel.
// For testing only — requires the bus to be *InMemEventBus.
func SubscriberCount(t testing.TB, bus EventBus, channel string) int {
    t.Helper()
    b, ok := bus.(*InMemEventBus)
    if !ok {
        t.Fatalf("expected *InMemEventBus, got %T", bus)
    }
    b.mu.RLock()
    defer b.mu.RUnlock()
    return len(b.subscribers[channel])
}
```

This is a **test-file helper** (inside `eventbus_test` package) — it can access `InMemEventBus.mu` and `subscribers` because Go's test files in the same package can access unexported fields. No production code changes needed.

### 7.2 TDD-01: Compose returns non-nil cleanup func

Compose a data_grid screen, assert `cleanup != nil`. Invoke it, assert no panic.

### 7.3 TDD-02: Cleanup removes EventBus subscribers

Compose a data_grid, verify `SubscriberCount(bus, "screen:submit:test-vista") == 1`, call cleanup, verify count drops to 0. Publish on the old channel and assert zero handler invocations.

### 7.4 TDD-03: Cleanup cancels in-flight goroutines

Compose a client-mode data_grid with a mock pool that blocks on query. Call cleanup. Assert the blocked goroutine terminates within 1 second (via `sync.WaitGroup` or goroutine count).

### 7.5 TDD-04: Navigate tears down previous screen

Use `RunBootstrap` with a home screen containing a data_grid, then call `ui.Navigate("screen-b")`. Wait for async navigation. Assert `SubscriberCount(bus, "screen:submit:home") == 0`.

### 7.6 TDD-05: Idempotent cleanup

Call the cleanup func twice. Assert no panic, assert `Unsubscribe` was called exactly once (verified by subscriber count remaining 0 after both calls).

### 7.7 TDD-06: No-op cleanup for non-data_grid screen

Compose a label-only screen. Call cleanup. Assert no panic, no side effects.

### 7.8 Existing test migration

Every existing test that calls `Compose` must update from:

```go
obj, err := ui.Compose(node, vistaID)
```

to:

```go
obj, cleanup, err := ui.Compose(node, vistaID)
if err != nil { ... }
defer cleanup()
```

This is a mechanical change. Grep shows the call sites:

| File | Call sites | Pattern |
|------|-----------|---------|
| `cmd/golemui/main_test.go` | ~12 tests via `RunBootstrap` | `ui.Compose` called inside `RunBootstrap` — must update both the function and the tests that reference its internals |
| `pkg/ui/compositor_test.go` | ~20+ tests | Direct `Compose` calls |

---

## 8. Files Changed

| File | Change | Est. lines |
|------|--------|-----------|
| `pkg/ui/compositor.go` | `Compose` signature → `(CanvasObject, func(), error)`. `composeWithState` signature → same. Container case aggregates cleanups. data_grid case returns `sync.Once`-wrapped cleanup. Leaf cases return no-op func. | ~+25, ~-5 |
| `cmd/golemui/main.go` | `ui.Navigate`: add `prevCleanup` var, call before `Compose`, assign after. Bootstrap: capture home screen cleanup. | ~+10, ~-4 |
| `pkg/ui/compositor_test.go` | Update all `Compose` call sites (add `cleanup` capture + `defer cleanup()`). Add 4 new tests (TDD-01 through TDD-03, TDD-05, TDD-06). | ~+80 |
| `cmd/golemui/main_test.go` | Update `Compose` calls in `RunBootstrap` tests. Add TDD-04 test. | ~+30 |
| `pkg/eventbus/eventbus_test.go` | Add `SubscriberCount` test helper. | ~+12 |

**Total:** ~157 lines added, ~9 lines removed.

---

## 9. Risks

| Risk | Severity | Mitigation |
|------|----------|------------|
| `Compose` signature change breaks all callers | Medium | Mechanical update: add `cleanup` capture + `defer cleanup()`. All callers are internal. |
| Race on `prevCleanup` during rapid navigation | Low | Cleanup is idempotent (`sync.Once`). Worst case: one extra cleanup invocation (no-op) and at most one stale subscriber until next navigation. |
| `model.mu` deadlock if cleanup holds lock during `cancel()` | Low | Design explicitly reads + nils fields under lock, then calls `cancelFn()`/`unsubFn()` **outside** the lock. |
| Existing tests fail after signature change | Medium | Mechanical migration with `defer cleanup()`. All tests use the same pattern. |
| `sync.Once` adds allocation per data_grid | Negligible | One `sync.Once` struct per `dataGridModel` (already allocated). 8 bytes per grid. |

---

## 10. Implementation Order

```
1. compositor.go — signature change + cleanup plumbing
   ├── Compose return tuple
   ├── composeWithState return tuple
   ├── container case — aggregate children cleanups
   ├── data_grid case — sync.Once cleanup
   └── leaf cases — no-op func

2. compositor_test.go — migrate existing tests
   └── All Compose calls: add cleanup capture + defer

3. eventbus_test.go — add SubscriberCount helper

4. compositor_test.go — add new TDD tests
   ├── TDD-01: non-nil cleanup for data_grid
   ├── TDD-02: cleanup removes subscribers
   ├── TDD-03: cleanup cancels goroutines
   ├── TDD-05: idempotent double-cleanup
   └── TDD-06: no-op cleanup for label-only screen

5. main.go — prevCleanup in Navigate + bootstrap
   ├── Navigate closure: prevCleanup teardown
   └── Bootstrap: homeCleanup assignment

6. main_test.go — migrate + new test
   ├── Update RunBootstrap (Compose signature)
   └── TDD-04: Navigate tears down previous screen

7. Verification
   ├── go build ./...
   ├── go vet ./...
   ├── go test ./... -count=1
   └── Grep audit: zero Compose calls ignoring cleanup return
```

---

## 11. Before/After Comparison

### Before

```
Compose → (widget, error)
Navigate: go func() {
    LoadScreen → Compose → fyne.Do(swap)
}()
// Old screen's subscribers leak forever
// Old goroutines run to completion on detached widgets
```

### After

```
Compose → (widget, cleanup func, error)
Navigate: go func() {
    if prevCleanup != nil { prevCleanup(); prevCleanup = nil }
    LoadScreen → Compose → prevCleanup = cleanup
    fyne.Do(swap)
}()
// Old screen's subscribers removed
// Old goroutines cancelled via context
```
