# SDD Spec — screen-lifecycle-cleanup

> Formal requirements and TDD scenarios for automatic EventBus unsubscription and resource cleanup on screen navigation in GolemUI.

## 1. Introduction

GolemUI leaks memory on every screen navigation. When `ui.Navigate` replaces the active screen in `mainContainer`, the previous screen's EventBus subscribers remain registered indefinitely. Each `data_grid` on the old screen pins its `dataGridModel`, `*widget.Table`, `*ScreenState`, and `NodeMeta` via the subscriber closure stored in `LocalEventBus.subscribers`. In-flight `loadMasterBuffer` and `fetchGridDataAsync` goroutines also continue running because `model.cancel` is never invoked on screen replacement.

The root cause is a call-site gap: the `EventBus` already provides `Unsubscribe(channel, subID)` and the `dataGridModel` already stores `model.unsubscribe func()` and `model.cancel context.CancelFunc` — but nothing invokes them when a screen is replaced.

This spec defines the formal requirements and TDD scenarios to plug the leak by making `Compose` return a cleanup function and having `ui.Navigate` invoke it before composing the new screen.

**Scope:** `pkg/ui/compositor.go` (Compose return tuple), `cmd/golemui/main.go` (Navigate teardown), and the affected test files.

**Originating spec:** `docs/specify/a014-screen-lifecycle-cleanup-and-unsubscribe.md`

---

## 2. Formal Requirements

| ID | Description | Priority | Verification |
|----|-------------|----------|--------------|
| **REQ-CLEANUP-01** | **Compose returns cleanup func.** `Compose(node NodeMeta, vistaID string) (fyne.CanvasObject, func(), error)`. The returned `func()` is an idempotent teardown that collects all per-screen cleanup operations. For `data_grid` nodes: calls `model.cancel()` (if non-nil) and `model.unsubscribe()` (if non-nil). For non-data_grid components: the cleanup is a no-op. Parent containers compose children's cleanup funcs recursively into a single aggregate func. | Critical | Test: compose a screen with a data_grid, assert returned func is non-nil. Invoke it, assert subscribers removed. |
| **REQ-CLEANUP-02** | **Navigate tears down previous screen.** `ui.Navigate` stores the previous screen's cleanup func in a closure variable. Before composing the new screen (after `LoadScreen`, before `Compose`), the previous cleanup func is invoked and then set to nil. The first `Navigate` invocation (from bootstrap home) tears down the home screen's cleanup. | Critical | Test: navigate to screen A (subscribe), navigate to screen B, assert A's submit channel has zero subscribers. |
| **REQ-CLEANUP-03** | **EventBus subscribers removed.** After cleanup, the `screen:submit:<oldVistaID>` channel has zero subscribers in `LocalEventBus`. If the old screen had N data_grids, all N subscriber entries are removed. If the channel's inner map becomes empty, the channel key is also removed from `LocalEventBus.subscribers` (existing `Unsubscribe` behavior). | Critical | Test: compose + subscribe, call cleanup, assert `Publish` on old channel triggers zero handlers. |
| **REQ-CLEANUP-04** | **In-flight goroutines cancelled.** `model.cancel()` cancels the context used by `loadMasterBuffer` and `fetchGridDataAsync` goroutines. After cleanup, in-flight goroutines observe `ctx.Err()` and return without touching the widget. This terminates unnecessary DB queries and prevents `table.Refresh()` on detached widgets. | High | Test: compose with a mock pool that blocks on query, call cleanup, assert goroutine terminates within timeout. |
| **REQ-CLEANUP-05** | **Idempotent cleanup.** Calling the cleanup func multiple times is safe: no double-unsubscribe panic, no double-cancel panic. Implementation uses `sync.Once` or nil-guard pattern. The second invocation is a no-op. | High | Test: call cleanup twice, assert no panic. |
| **REQ-CLEANUP-06** | **No EventBus API change.** The `EventBus` interface and `InMemEventBus` implementation are NOT modified. The existing `Subscribe` → `string` and `Unsubscribe(channel, subID)` primitives are sufficient. No new methods added. | High | Code audit: zero diff in `pkg/eventbus/eventbus.go`. |
| **REQ-CLEANUP-07** | **Backward compatibility.** All existing tests continue to pass. The `Compose` signature change from `(CanvasObject, error)` to `(CanvasObject, func(), error)` propagates to all callers (`main.go`, tests). Each caller captures the cleanup func and defers it where appropriate. | High | `go test ./... -count=1` passes. Grep audit confirms all `Compose` call sites updated. |

---

## 3. TDD Scenarios

### TDD-01: Compose returns non-nil cleanup func for data_grid screen

**Requirement:** REQ-CLEANUP-01

**Given** a `NodeMeta` tree containing one `data_grid` node with a valid `DataSource` and `SubmitChannel`, and a `LocalEventBus` initialized.

**When** `Compose(node, "test-vista")` is called.

**Then** the returned `(obj, cleanup, err)` tuple has `cleanup != nil`. The `obj` is a valid `*widget.Table`. No error is returned.

```go
// Pseudocode
obj, cleanup, err := Compose(dataGridNode, "test-vista")
if err != nil { t.Fatal(err) }
if cleanup == nil { t.Error("expected non-nil cleanup func for data_grid screen") }
```

### TDD-02: Cleanup removes EventBus subscribers

**Requirement:** REQ-CLEANUP-01, REQ-CLEANUP-03

**Given** a composed `data_grid` that has subscribed to `screen:submit:test-vista` via `LocalEventBus.Subscribe`.

**When** the cleanup func returned by `Compose` is invoked.

**Then** a subsequent `LocalEventBus.Publish("screen:submit:test-vista", ...)` triggers zero handler invocations. The old subscriber does not fire.

```go
// Pseudocode
eb := eventbus.NewEventBus()
ui.LocalEventBus = eb

obj, cleanup, err := Compose(dataGridNode, "test-vista")
// ... wait for subscription to register ...

// Verify subscriber exists: publish triggers handler
triggered := int32(0)
// (subscribe a spy, or count handlers via test helper)

cleanup()

// Publish after cleanup — no old handler should fire
// Assert: spy counter unchanged (zero subscriber activations)
```

### TDD-03: Cleanup cancels in-flight goroutines

**Requirement:** REQ-CLEANUP-04

**Given** a composed `data_grid` in client mode (`FilterMode == "client"`) with a `MasterDataSource` and a mock `BusinessPool` whose `Query` blocks on an unbuffered channel.

**When** the cleanup func is invoked (while `loadMasterBuffer` goroutine is blocked on the query).

**Then** the goroutine detects the cancelled context and returns within a reasonable timeout (e.g., 1 second). No `table.Refresh()` is called on the detached widget.

```go
// Pseudocode
blockCh := make(chan struct{})
mockPool.RegisterQuery("SELECT * FROM books", func() { <-blockCh })

obj, cleanup, err := Compose(clientModeDataGridNode, "test-vista")

// loadMasterBuffer goroutine is blocked on Query
cleanup()

// Assert goroutine terminated: e.g., via WaitGroup or goroutine count check
// blockCh was never closed — goroutine exited via ctx.Err()
```

### TDD-04: Navigate tears down previous screen's subscribers

**Requirement:** REQ-CLEANUP-02, REQ-CLEANUP-03

**Given** a `ui.Navigate` closure configured with mock `LoadScreen` + `Compose`, and an initial home screen composed via `RunBootstrap` that has one data_grid subscribed to `screen:submit:home`.

**When** `ui.Navigate("screen-b")` is called, composing a new screen that subscribes to `screen:submit:screen-b`.

**Then** the `screen:submit:home` channel has zero subscribers after navigation completes. The `screen:submit:screen-b` channel has exactly one subscriber (the new data_grid).

```go
// Pseudocode
// Bootstrap with home screen containing data_grid
_, cleanup, _ := Compose(homeDataGridNode, "home")

// Navigate to screen-b
ui.Navigate("screen-b")
// Wait for async navigation to complete

// Assert: Publish on "screen:submit:home" triggers zero handlers
// Assert: Publish on "screen:submit:screen-b" triggers one handler
```

### TDD-05: Idempotent cleanup — double invocation is safe

**Requirement:** REQ-CLEANUP-05

**Given** a composed `data_grid` with an active subscriber.

**When** the cleanup func is called twice in succession.

**Then** no panic occurs. The second invocation is a no-op. `LocalEventBus.Unsubscribe` is called exactly once (verified via spy or handler count).

```go
// Pseudocode
obj, cleanup, err := Compose(dataGridNode, "test-vista")
cleanup() // first call — removes subscriber, cancels context
cleanup() // second call — no-op, no panic
```

### TDD-06: No-op cleanup for non-data_grid screen

**Requirement:** REQ-CLEANUP-01

**Given** a `NodeMeta` tree containing only `container`, `label`, and `text_input` nodes — no `data_grid`.

**When** `Compose(node, "simple-screen")` is called.

**Then** the returned cleanup func is non-nil but is a no-op when invoked. Calling it produces no side effects and no panics.

```go
// Pseudocode
obj, cleanup, err := Compose(labelOnlyNode, "simple-screen")
cleanup() // no-op — no subscribers to remove, no goroutines to cancel
// No panic, no error
```

---

## 4. Out of Scope

- **No EventBus API change.** The `Subscribe`/`Unsubscribe` interface and `InMemEventBus` implementation remain untouched. No new methods, no return type changes on `Subscribe`.
- **No `UnsubscribeAll(channel)` method.** Per-subID cleanup via the existing `Unsubscribe` is sufficient and more precise.
- **No per-screen EventBus.** Each screen shares the process-wide `LocalEventBus`. Cleanup targets individual subscriptions via subID.
- **No widget tree walking at teardown time.** The cleanup func captures state during composition; no post-hoc tree traversal is needed.
- **No changes to `EventBus.Publish` goroutine model.** The `go h(event)` fan-out stays as-is.
- **No loading indicator or transition animation.** Out of scope for this change.
- **No sidebar or navigation menu changes.**
- **No GC finalizer or weak-reference patterns.** Go does not support them reliably; explicit cleanup is correct.
- **No navigation cancellation or debouncing.** Each navigation is independent; last `fyne.Do` wins. The cleanup func for the previous screen runs to completion before the new screen is composed.

---

## 5. Assumptions

1. **The `Compose` signature change is acceptable.** Changing from `(CanvasObject, error)` to `(CanvasObject, func(), error)` is a breaking change to all callers. All callers are internal to GolemUI (main.go and tests). The proposal evaluated and accepted this tradeoff over a package-level registry.

2. **`model.cancel()` and `model.unsubscribe()` are sufficient for cleanup.** These two operations cover all per-data_grid resources: the EventBus subscriber, the context for in-flight goroutines. No other per-data_grid resources need cleanup (no timers, no file handles, no network connections outside the pool's ownership).

3. **`LocalEventBus.Unsubscribe` is concurrency-safe.** It is (eventbus.go:52-61 uses `sync.RWMutex`). Calling it from a background goroutine inside `ui.Navigate`'s `go func()` is safe without additional synchronization.

4. **`context.CancelFunc` is safe to call multiple times.** The Go stdlib guarantees that calling `cancel()` after the first invocation is a no-op. Combined with `sync.Once` for the unsubscribe, the idempotency requirement is satisfied.

5. **The cleanup func does not perform UI mutations.** It only calls `model.cancel()` and `LocalEventBus.Unsubscribe()`. Neither touches Fyne widgets. Therefore, the cleanup can safely run from any goroutine without `fyne.Do()`.

6. **`prevCleanup` access is serialized by the goroutine lifecycle.** The `prevCleanup` variable is captured by the `ui.Navigate` closure. Rapid navigation clicks spawn multiple goroutines that may race on `prevCleanup`. For the first iteration, "last goroutine wins" semantics are acceptable — the worst case is a double-cleanup (safe by REQ-CLEANUP-05) or a missed cleanup of the very latest screen (one extra subscriber). A mutex can be added in a follow-up if needed.

7. **The bootstrap home screen's cleanup is stored in `prevCleanup`.** The initial `Compose` during `RunBootstrap` produces a cleanup func that is stored as `prevCleanup`. The first `ui.Navigate` invocation tears it down before composing the new screen.
