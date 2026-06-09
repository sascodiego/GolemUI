# SDD Spec — fyne-thread-safety

> Formal requirements and TDD scenarios for establishing Fyne thread safety and asynchronous screen loading in GolemUI.

## 1. Introduction

GolemUI currently violates Fyne's threading model in two ways:

1. **Six widget mutations** (`table.Refresh()` and `table.SetColumnWidth()`) execute from background goroutines without dispatching onto the Fyne UI thread, producing data races confirmed by `go test -race`.
2. **`ui.Navigate`** runs synchronously on the UI thread, blocking all interaction while the database query and widget composition complete.

This spec defines the formal requirements and test scenarios to fix both issues while preserving all existing function signatures and architectural patterns.

**Scope:** `cmd/golemui/main.go` (Navigate callback) and `pkg/ui/compositor.go` (data_grid widget mutations).

**Originating spec:** `docs/specify/a013-fyne-thread-safety-and-async-screen-loading.md`

---

## 2. Formal Requirements

| ID | Description | Priority | Verification |
|----|-------------|----------|--------------|
| **REQ-ASYNC-01** | **Non-blocking Navigate.** `ui.Navigate(vistaID)` must return immediately to the caller. `LoadScreen` and `Compose` execute inside a background goroutine. The caller (Fyne button-tap callback) is unblocked. | Critical | Test: Navigate returns before LoadScreen completes (channel-block mock). |
| **REQ-ASYNC-02** | **UI thread dispatch in Navigate.** The final UI swap — `mainContainer.Objects` assignment, `mainContainer.Refresh()`, and `navTree.SelectByVistaID(vID)` — must execute inside `fyne.Do()`. | Critical | Test: fyne.Do callback observed containing container mutation. Code audit. |
| **REQ-ASYNC-03** | **Error handling in Navigate goroutine.** Errors from `LoadScreen` or `Compose` are logged. The previous UI remains visible and responsive. No panic, no freeze. | High | Test: inject failing LoadScreen, assert log output and no panic. |
| **REQ-THREAD-01** | **Safe table mutations in loadMasterBuffer.** `table.SetColumnWidth()` loop and `table.Refresh()` at `compositor.go:370-374` (goroutine G1) must be wrapped in `fyne.Do()`. | Critical | Test: verify fyne.Do dispatch around refresh/setColumnWidth. `go test -race` clean. |
| **REQ-THREAD-02** | **Safe table mutations in fetchGridDataAsync.** `table.SetColumnWidth()` loop and `table.Refresh()` at `compositor.go:533-536` (goroutine G2) must be wrapped in `fyne.Do()`. | Critical | Test: verify fyne.Do dispatch. `go test -race` clean. |
| **REQ-THREAD-03** | **Safe table.Refresh in filterMasterRows.** Both `table.Refresh()` calls — early return at `compositor.go:398` and normal exit at `compositor.go:434` (goroutine G3) — must be wrapped in `fyne.Do()`. | Critical | Test: verify fyne.Do dispatch on both code paths. `go test -race` clean. |
| **REQ-LOCK-01** | **Unlock before fyne.Do (deadlock prevention).** At every wrap site (G1, G2, G3), `model.mu.Unlock()` must complete **before** entering the `fyne.Do()` block. This prevents deadlock: `table.Refresh()` triggers `Length()` / `CreateCell()` callbacks that acquire `model.mu.RLock()`. | Critical | Test: concurrent test completes within timeout (no deadlock). Code audit of unlock ordering. |
| **REQ-INVARIANT-01** | **No signature changes.** `ui.Navigate` remains `func(vistaID string)`. `NodeMeta` struct unchanged. `LoadScreen` and `Compose` signatures unchanged. `EventBus.Publish` goroutine model unchanged. | High | Code audit: diff contains zero signature changes to public APIs. |

---

## 3. TDD Scenarios

### TDD-01: Navigate returns immediately (non-blocking)

**Requirement:** REQ-ASYNC-01

**Given** a `ui.Navigate` closure configured with a mock `LoadScreen` that blocks on an unbuffered channel before returning.

**When** `ui.Navigate("test_screen")` is called from the test goroutine.

**Then** the call returns **before** the channel is signaled (i.e., `LoadScreen` has not yet completed). A `select` with a short timeout on the return confirms non-blocking behavior.

```go
// Pseudocode
blockCh := make(chan struct{})
mockLoadScreen := func(...) { <-blockCh; ... }
ui.Navigate = buildNavigateWithMock(mockLoadScreen)

done := make(chan struct{})
go func() { ui.Navigate("test"); close(done) }()

select {
case <-done:
    // PASS: Navigate returned before LoadScreen finished
case <-time.After(5 * time.Second):
    t.Fatal("Navigate blocked — did not return immediately")
}
```

### TDD-02: Navigate dispatches UI swap via fyne.Do

**Requirement:** REQ-ASYNC-02

**Given** a `ui.Navigate` closure configured with a mock `LoadScreen` and `Compose` that return immediately with valid results, and a spy on `mainContainer` mutation.

**When** `ui.Navigate("test_screen")` is called.

**Then** the `mainContainer.Objects` assignment and `mainContainer.Refresh()` are observed to have executed inside a `fyne.Do` callback. In the Fyne test environment (`test.App()`), `fyne.Do` runs synchronously, so after the goroutine completes, the container contains the new UI object.

```go
// Pseudocode
newUI := widget.NewLabel("composed")
navigate("test_screen")
// Wait for goroutine + fyne.Do to complete (poll or sync)
// Assert mainContainer.Objects[0] == newUI
// Assert mainContainer.Refresh() was called
```

### TDD-03: Navigate logs errors without crashing

**Requirement:** REQ-ASYNC-03

**Given** a `ui.Navigate` closure configured with a `LoadScreen` that returns an error.

**When** `ui.Navigate("bad_screen")` is called.

**Then** the error is logged (captured via `log.Output` or equivalent), no panic occurs, the goroutine exits cleanly, and the previous `mainContainer` content remains unchanged.

```go
// Pseudocode
originalContent := mainContainer.Objects
ui.Navigate("bad_screen")
// Wait for goroutine to finish
// Assert mainContainer.Objects == originalContent
// Assert log contains error message
```

### TDD-04: loadMasterBuffer wraps UI calls in fyne.Do

**Requirement:** REQ-THREAD-01, REQ-LOCK-01

**Given** a composed `data_grid` with a `BusinessPool` that returns rows, and a captured reference to the widget `Table`.

**When** `loadMasterBuffer` completes its background query.

**Then** the `table.SetColumnWidth` loop and `table.Refresh()` are observed to have executed inside `fyne.Do`. The model data (`headers`, `rows`) is populated correctly under the lock. `model.mu` is unlocked **before** the `fyne.Do` block executes (verified by: the test completes without deadlock within a timeout).

```go
// Pseudocode — after data_grid compose, wait for loadMasterBuffer
// Assert model.headers is populated
// Assert table visual state reflects the data
// Assert no deadlock (test completes within 2s)
```

### TDD-05: fetchGridDataAsync wraps UI calls in fyne.Do

**Requirement:** REQ-THREAD-02, REQ-LOCK-01

**Given** a composed `data_grid` with a `BusinessPool` that returns rows for the main query, and a captured reference to the widget `Table`.

**When** `fetchGridDataAsync` completes its background query (triggered via server-mode data source).

**Then** the `table.SetColumnWidth` loop and `table.Refresh()` are observed to have executed inside `fyne.Do`. Model data is populated under the lock. `model.mu` is unlocked before the `fyne.Do` block.

```go
// Pseudocode — compose data_grid with server-mode data source
// Wait for fetchGridDataAsync goroutine
// Assert model data populated
// Assert no deadlock within 2s
```

### TDD-06: filterMasterRows wraps Refresh in fyne.Do (both paths)

**Requirement:** REQ-THREAD-03, REQ-LOCK-01

**Given** a composed `data_grid` with a pre-loaded master buffer (populated via `loadMasterBuffer`), and a captured reference to the widget `Table`.

**When** a filter event is published to the submit channel with a snapshot containing a search term.

**Then** `table.Refresh()` is observed to have executed inside `fyne.Do`. This is tested for:

- **Path A (empty filter / show all):** snapshot with empty search term → `filterMasterRows` takes the early-return branch, wraps `table.Refresh()` in `fyne.Do`.
- **Path B (filtered rows):** snapshot with matching term → `filterMasterRows` filters rows, wraps `table.Refresh()` in `fyne.Do`.

Both paths: `model.mu` is unlocked before `fyne.Do`.

```go
// Pseudocode — Path A
publish(emptyFilterSnapshot)
// Wait for handler goroutine
// Assert table refresh dispatched via fyne.Do

// Pseudocode — Path B
publish(filterSnapshotWithTerm)
// Wait for handler goroutine
// Assert filtered rows in model
// Assert table refresh dispatched via fyne.Do
```

### TDD-07: No deadlock under concurrent access

**Requirement:** REQ-LOCK-01

**Given** a composed `data_grid` with a `BusinessPool` that returns rows.

**When** multiple concurrent operations are triggered: `loadMasterBuffer` (G1), `fetchGridDataAsync` (G2), and a filter event via EventBus (G3) all running simultaneously.

**Then** all three operations complete within a reasonable timeout (e.g., 5 seconds). No goroutine deadlocks. The model ends in a consistent state (headers and rows match). `table.Refresh()` calls inside `fyne.Do` successfully acquire `model.mu.RLock()` via the table callbacks without blocking.

```go
// Pseudocode
var wg sync.WaitGroup
wg.Add(3)
// Trigger loadMasterBuffer (G1)
// Trigger fetchGridDataAsync (G2)
// Publish filter event (G3)
done := make(chan struct{})
go func() { wg.Wait(); close(done) }()
select {
case <-done:
    // PASS — no deadlock
case <-time.After(5 * time.Second):
    t.Fatal("Deadlock detected — concurrent operations did not complete")
}
```

### TDD-08: Race detector clean

**Requirement:** REQ-THREAD-01, REQ-THREAD-02, REQ-THREAD-03

**Given** the full test suite with all `data_grid` tests passing.

**When** `go test -race ./pkg/ui/... -count=1` is executed.

**Then** no `tableRenderer.Refresh` data race is reported. No other race conditions related to widget mutation are reported. The command exits with code 0.

```bash
go test -race ./pkg/ui/... -count=1
# Expected: PASS, exit 0, no race warnings
```

---

## 4. Out of Scope

- **No `LoadScreen` internal refactoring.** Database query logic, SQL parameters, and connection pool usage remain untouched.
- **No changes to `EventBus.Publish`.** The goroutine-per-handler dispatch model stays as-is.
- **No changes to non-`data_grid` widget cases.** Container, label, text_input, text_area, and button do not spawn goroutines and are already thread-safe.
- **No changes to `NodeMeta` struct or its properties.**
- **No loading indicator or transition animation.** This is a thread-safety fix, not a UX polish.
- **No `win.SetContent` changes.** Called once at bootstrap before the event loop starts; no thread-safety issue.
- **No cancellation or debouncing for rapid navigation.** Each navigation spawns a goroutine; last `fyne.Do` wins. A navigation guard is a separate enhancement.
- **No `fyne.DoAndWait` usage.** All dispatches use `fyne.Do` (fire-and-forget) to avoid blocking background goroutines.

---

## 5. Assumptions

1. **`fyne.Do` works correctly in the Fyne test environment.** Fyne's `test.App()` provides a synchronous `fyne.Do` implementation. Tests that assert `fyne.Do` dispatch can rely on synchronous execution under the test app.

2. **`ctx` and `cfg.LayoutQuery` are immutable after bootstrap.** The `ui.Navigate` closure captures these by reference, but they are set once during `RunBootstrap` and never mutated. The background goroutine can safely read them without synchronization.

3. **`mainContainer` and `navTree` are only mutated by `Navigate`'s `fyne.Do` block.** No other code path mutates these objects after bootstrap. The `fyne.Do` block serializes mutations correctly.

4. **Model writes under `model.mu` are correct and will remain correct.** The existing lock-protected writes in `loadMasterBuffer`, `fetchGridDataAsync`, and `filterMasterRows` are safe. This change only affects what happens **after** the unlock — wrapping the subsequent UI calls in `fyne.Do`.

5. **The archived `tableRenderer.Refresh` race is the same race fixed by this change.** The verify report from `2026-06-05-screen-state-store` attributes the race to "pre-existing Fyne test driver race." This spec assumes that wrapping the calls in `fyne.Do` eliminates that race by ensuring all widget mutations happen on the UI thread.

6. **Sequential goroutine completion is acceptable for navigation.** If the user clicks multiple navigation buttons rapidly, multiple goroutines will complete and each will call `fyne.Do` to swap the container. The last one to execute wins. This is acceptable behavior for a first fix; debouncing or cancellation is out of scope.
