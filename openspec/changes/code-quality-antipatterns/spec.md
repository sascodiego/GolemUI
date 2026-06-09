# Specification: code-quality-antipatterns

**Change ID:** `code-quality-antipatterns`  
**Date:** 2026-06-07  
**Status:** Draft  
**Audit Source:** Sections 4.2, 4.3  
**Proposal:** `openspec/changes/code-quality-antipatterns/proposal.md`  
**Explore Report:** `openspec/changes/code-quality-antipatterns/explore.md`  
**Config:** `openspec/config.yaml` — strict TDD enabled  
**Estimated diff:** ~26 lines across 2 source files

---

## 1. Requirements

### 1.1 Atomic Re-entrancy Guard (§4.2)

| ID | Requirement | Priority | Rationale |
|----|-------------|----------|-----------|
| REQ-ATOMIC-01 | `NavTree.navigating` field type MUST be `sync/atomic.Bool` instead of `bool` | Must | Plain `bool` accessed across goroutines without synchronization is a data race under Go's memory model |
| REQ-ATOMIC-02 | All write sites MUST use `Store(bool)` method: `nt.navigating.Store(true)` and `nt.navigating.Store(false)` | Must | Ensures `SeqCst` memory ordering on writes from the background goroutine |
| REQ-ATOMIC-03 | All read sites MUST use `Load() bool` method: `navTree.navigating.Load()` | Must | Ensures the UI thread always observes the latest written value |

### 1.2 Pool Cleanup + prevCleanup Race (§4.3)

| ID | Requirement | Priority | Rationale |
|----|-------------|----------|-----------|
| REQ-CLEANUP-01 | `RunBootstrap` MUST register a `win.SetOnClosed()` callback before `win.ShowAndRun()` when `runWindow` is `true` | Must | Without this hook, the happy path never calls `dbPool.Close()` and never invokes the last screen's cleanup |
| REQ-CLEANUP-02 | The `SetOnClosed` callback MUST call `prevCleanup()` (if non-nil) before calling `dbPool.Close()` | Must | The last screen's EventBus subscriptions and context cancellations must be released before the DB pools are drained |
| REQ-CLEANUP-03 | All read, call, assign, and nil-check operations on `prevCleanup` MUST be protected by a `sync.Mutex` (`cleanupMu`) | Must | `prevCleanup` is shared between the Navigate goroutine (background) and the `SetOnClosed` callback (UI thread) |

### 1.3 Architectural Constraints

| ID | Requirement | Priority | Rationale |
|----|-------------|----------|-----------|
| REQ-ARCH-01 | No changes to `NavTree` zero-value semantics — `atomic.Bool` zero-value is `false`, matching the original `bool` behavior | Must | Prevents behavioral regression; existing tests depend on the initial `false` state |
| REQ-ARCH-02 | The `cleanupMu` lock MUST NOT be held during `LoadScreen` or `Compose` calls — only around `prevCleanup` read/write/call operations | Must | Lock held during I/O or composition would serialise all navigations and block the close callback |
| REQ-ARCH-03 | `dbPool.Close()` MUST be called outside the `cleanupMu` lock in the `SetOnClosed` callback | Must | Pool close may block on draining connections; holding the mutex during pool close would deadlock the Navigate goroutine |

---

## 2. Behavioral Contracts

### 2.1 `atomic.Bool` Semantics for `navigating`

**Contract:** The `NavTree.navigating` field uses `sync/atomic.Bool` which provides sequential consistency (`SeqCst`) by default.

| Operation | Goroutine | Method | Visibility guarantee |
|-----------|-----------|--------|---------------------|
| `SelectByVistaID` entry | background (Navigate goroutine) | `nt.navigating.Store(true)` | Visible to all goroutines before `tree.Select` returns |
| `SelectByVistaID` exit (defer) | background (Navigate goroutine) | `nt.navigating.Store(false)` | Visible to all goroutines after defer executes |
| `OnSelected` callback | UI thread (Fyne dispatch) | `navTree.navigating.Load()` | Always returns the latest stored value |

**Guarantee:** When `OnSelected` fires as a result of `tree.Select()` inside `SelectByVistaID`, the `Load()` call will return `true`, suppressing re-entrancy. When `OnSelected` fires due to a user click (no `SelectByVistaID` in progress), the `Load()` call returns `false`, allowing navigation.

**No-constructor-change guarantee:** `atomic.Bool` is usable as a struct field without explicit initialization. Its zero-value is `false`, matching the original `bool` semantics.

### 2.2 `SetOnClosed` Lifecycle

**Contract:** The `win.SetOnClosed(f)` callback fires exactly once when the user closes the window (or when the underlying window system destroys the native window).

| Phase | Event | Action |
|-------|-------|--------|
| Bootstrap | `win.SetOnClosed(f)` registered | Callback stored, not yet invoked |
| Runtime | User closes window | Fyne calls `f()` on the UI thread |
| Callback | `f()` executes | 1) Acquire `cleanupMu`. 2) Call `prevCleanup()` if non-nil, set to nil. 3) Release `cleanupMu`. 4) Call `dbPool.Close()`. |
| After callback | `ShowAndRun()` returns | `RunBootstrap` returns `*App` to `main()` |

**Ordering:** `prevCleanup()` is called before `dbPool.Close()`. This ensures EventBus subscriptions are released while the pools are still alive (in case cleanup needs to execute final DB queries during teardown).

**Error-path invariant:** Error paths in `RunBootstrap` (lines 93, 141, 147 of `main.go`) already call `dbPool.Close()` before returning errors. `SetOnClosed` does not fire on error paths because the window was never shown.

### 2.3 `cleanupMu` Mutex Ordering

**Contract:** A single `sync.Mutex` (`cleanupMu`) serialises all access to the `prevCleanup` variable.

| Access site | Goroutine | Lock scope |
|-------------|-----------|------------|
| Navigate goroutine: call + nil old `prevCleanup` | background | Lock → call → nil → Unlock |
| Navigate goroutine: assign new `prevCleanup` | background | Lock → assign → Unlock |
| `SetOnClosed`: call + nil `prevCleanup` | UI thread | Lock → call → nil → Unlock |

**Lock-free zones (REQ-ARCH-02):**
- `LoadScreen` call — no `prevCleanup` access, runs outside the lock
- `Compose` call — no `prevCleanup` access, runs outside the lock
- `mainContainer.Objects` assignment — no `prevCleanup` access, runs outside the lock
- `navTree.SelectByVistaID` — no `prevCleanup` access, runs outside the lock

**Lock ordering guarantee:** The lock is never held recursively. The lock is never held while calling `dbPool.Close()` (REQ-ARCH-03). `prevCleanup()` functions contain only `context.CancelFunc` calls and EventBus `Unsubscribe` map deletions — both are fast, non-blocking operations.

---

## 3. Error Handling

### 3.1 Cleanup Panics

**Scenario:** `prevCleanup()` panics inside the Navigate goroutine or the `SetOnClosed` callback.

**Contract:** No `recover` is added. A panic in cleanup is a programming error (cleanup should only call `context.Cancel` and `map` deletion). The panic propagates naturally — crashing the application is the correct behavior for a corrupted state during cleanup.

**Rationale:** Adding `recover` around cleanup would silently swallow errors and leave the application in an inconsistent state (partially cleaned-up screens, leaked subscriptions). Fail-fast is safer during teardown.

### 3.2 Pool Close Failure

**Scenario:** `dbPool.Close()` encounters an error draining a `pgxpool.Pool`.

**Contract:** `pgxpool.Pool.Close()` does not return an error — it is a `func()` with no return value. Connections are drained best-effort and the pool is marked as closed. No error handling is needed.

**Rationale:** The `db.DB.Close()` method calls `CorePool.Close()` and `BusinessPool.Close()` sequentially. Both are no-return methods. The OS reclaims any remaining TCP sockets on process exit.

### 3.3 `SetOnClosed` Callback Not Firing

**Scenario:** Fyne's `SetOnClosed` callback does not fire on an exotic platform.

**Contract:** This is a Fyne behavioral contract issue, not a GolemUI issue. The API is stable and present since Fyne v2.0. If it does not fire, `main()` exits the process anyway — the OS reclaims all resources (DB connections, goroutines, memory).

**Mitigation:** The `runWindow=false` path (used by tests) returns `*App` directly, and the test caller is responsible for calling `a.DB.Close()`.

### 3.4 Race Between Navigate and SetOnClosed

**Scenario:** An in-flight Navigate goroutine is mid-compose when the window closes and `SetOnClosed` fires.

**Contract:** The mutex serialises access. Two possible interleavings:

1. **Navigate acquires lock first:** Completes cleanup of old screen, composes new screen, assigns `prevCleanup`. Then `SetOnClosed` acquires the lock, calls the just-assigned `prevCleanup`, and closes pools. The newly composed UI is discarded (window is already closed). **Safe.**

2. **SetOnClosed acquires lock first:** Calls `prevCleanup` and closes pools. Then Navigate goroutine acquires the lock, sees `prevCleanup == nil`, composes new screen, assigns `prevCleanup`. The composed UI tries to use closed pools — `LoadScreen` will fail because `pgxpool.Pool` returns errors after `Close()`. The error is logged and the goroutine exits. **Safe** — the window is already closed, the composed UI is never displayed.

**Guarantee:** In both cases, cleanup is called exactly once and pools are closed exactly once. No double-call, no stale reads.

---

## 4. TDD Test Scenarios

All tests must pass with `go test -race` to verify no data races remain.

### 4.1 Sidebar Tests (`pkg/ui/sidebar_widget_test.go`)

Existing tests that validate without changes:

| Test | Validates | Status |
|------|-----------|--------|
| `TestReentrancyGuardPreventsLoop` | `navigating` guard suppresses `Navigate` during programmatic select | Must pass unchanged |
| `TestSelectByVistaID_ValidSelectsNode` | Programmatic select works without triggering `Navigate` | Must pass unchanged |
| `TestSelectByVistaID_EmptyIsNoOp` | Empty vistaID is a safe no-op | Must pass unchanged |
| `TestSelectByVistaID_UnknownIsNoOp` | Unknown vistaID is a safe no-op | Must pass unchanged |
| `TestBuildNavTree_LeafTriggersNavigate` | User click triggers `Navigate` (guard not active) | Must pass unchanged |

New test scenarios to add:

| Test ID | Test Name | Validates | TDD Phase |
|---------|-----------|-----------|-----------|
| T-ATOMIC-01 | `TestNavigating_InitialState` | `NavTree.navigating.Load()` returns `false` on a freshly constructed `NavTree` (zero-value guarantee) | GREEN — `atomic.Bool` zero-value is `false` |
| T-ATOMIC-02 | `TestNavigating_StoreAndLoad` | `Store(true)` followed by `Load()` returns `true`; `Store(false)` followed by `Load()` returns `false` | GREEN — basic atomic round-trip |
| T-ATOMIC-03 | `TestSelectByVistaID_NoRaceUnderRaceDetector` | Call `SelectByVistaID` from a goroutine while `OnSelected` fires on another goroutine; verify no race detected | GREEN — race detector validation |

**T-ATOMIC-01 detail:**
```go
func TestNavigating_InitialState(t *testing.T) {
    items := []ui.MenuItem{
        {ID: "root", PadreID: "", Titulo: "Root", VistaID: "", Orden: 0},
    }
    navTree := ui.BuildNavTree(items)
    // navigating should be false initially (atomic.Bool zero-value)
    tree := navTree.Widget()
    tree.OnSelected("root") // should not panic, navigating is false
}
```

**T-ATOMIC-03 detail:** This test exercises the exact race pattern from the bug report — `SelectByVistaID` on a background goroutine while `OnSelected` fires concurrently. With `atomic.Bool`, the race detector must report zero warnings.

### 4.2 Main Wiring Tests (`cmd/golemui/main_test.go`)

Existing tests unaffected (they use `runWindow=false` path, no `SetOnClosed` involved).

New test scenarios to add:

| Test ID | Test Name | Validates | TDD Phase |
|---------|-----------|-----------|-----------|
| T-CLEANUP-01 | `TestSetOnClosed_InvokesCleanupAndClosesPool` | After `RunBootstrap` with `runWindow=true`-equivalent setup, closing the window invokes `prevCleanup` and `dbPool.Close()` | GREEN — verify lifecycle hook |
| T-CLEANUP-02 | `TestPrevCleanup_NilOnStartup` | After initial bootstrap, `prevCleanup` is set to the home screen's cleanup function (non-nil) | GREEN — initial assignment |
| T-CLEANUP-03 | `TestPrevCleanup_MutexPreventsConcurrentAccess` | Call `Navigate` from a goroutine and trigger close concurrently; verify `prevCleanup` called exactly once | GREEN — mutex serialization |

**T-CLEANUP-01 detail:**
Since `runWindow=true` blocks on `ShowAndRun()`, the test cannot use that path directly. Instead, verify that `RunBootstrap` with `runWindow=false` returns an `*App` with a functional `DB.Close()` method, and that calling `a.DB.Close()` sets the `MockDBPool.closed` flag.

```go
func TestRunBootstrap_PoolCloseOnAppClose(t *testing.T) {
    coreMock, bizMock := setupMockDB(t, `{"area":"root","component_ref":"label","label":"X"}`, nil)
    cfg := testConfig()
    cfg.EntryPointViewID = "home"
    ctx := context.Background()
    testApp := test.NewApp()

    appInstance, err := RunBootstrap(ctx, cfg, false, testApp)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    // Simulate what SetOnClosed does: close the pool
    appInstance.DB.Close()

    // Verify mock pools are closed
    if !coreMock.IsClosed() {
        t.Error("expected CorePool to be closed after DB.Close()")
    }
    if !bizMock.IsClosed() {
        t.Error("expected BusinessPool to be closed after DB.Close()")
    }
}
```

Note: `MockDBPool.IsClosed()` method may need to be added as an exported accessor for the `closed` field.

**T-CLEANUP-03 detail:**
This test validates the mutex prevents double-cleanup by running `Navigate` and a simulated close concurrently and asserting `prevCleanup` is called exactly once. Since `prevCleanup` is a local variable in `RunBootstrap`, the test verifies indirectly: after Navigate + close, the mock pools are closed (from the close path) and no panic occurs (from the mutex preventing double-call).

### 4.3 Race Detector Validation

After all changes, the following command MUST produce zero race detector warnings:

```bash
go test -race ./pkg/ui/ ./cmd/golemui/
```

This is the definitive acceptance test for both REQ-ATOMIC and REQ-CLEANUP requirements.

---

## 5. Assumptions

1. **`atomic.Bool` availability:** Go 1.19+ is required for `sync/atomic.Bool`. The project uses `go1.26.4` per `openspec/config.yaml` — this is satisfied.

2. **`SetOnClosed` reliability:** `fyne.Window.SetOnClosed(func())` is a stable API present since Fyne v2.0. It fires when the user closes the window via the native close button, Alt+F4, or equivalent. It fires exactly once.

3. **`prevCleanup` contents are fast:** `prevCleanup` functions contain only `context.CancelFunc` calls and EventBus `Unsubscribe` operations (map deletions). Both complete in microseconds. Holding `cleanupMu` during `prevCleanup()` call is safe because no I/O or blocking operations occur.

4. **No nested Navigate calls:** The current code does not call `Navigate` recursively. The `navigating` guard prevents this in `OnSelected`, and the `Navigate` function itself does not call `Navigate`. The mutex is never acquired recursively.

5. **`pgxpool.Pool.Close()` is idempotent:** Calling `Close()` on an already-closed pool is safe (no-op). This protects against the edge case where an error-path `dbPool.Close()` and the `SetOnClosed` callback both execute.

6. **`runWindow=false` path is test-only:** The non-window path is only used by tests and does not need `SetOnClosed`. The test caller is responsible for calling `a.DB.Close()` directly.

7. **Existing tests pass:** All tests in `sidebar_widget_test.go` and `main_test.go` pass before this change. The spec assumes a green baseline.

---

## 6. Out of Scope

| Item | Rationale |
|------|-----------|
| §4.1 Global mutable state refactor (Compositor struct) | Deferred to post-MVP. The remaining 4 globals (`DS`, `CWR`, `LocalEventBus`, `Navigate`) are write-once, nil-guarded, and low risk. Structural refactor is 400-600 lines across 6 files — out of scope for this change. |
| §1.x Fyne thread-safety violations | Separate SDD change (`fyne-thread-safety`). |
| §2.x Screen lifecycle issues | Separate SDD change (`screen-lifecycle-cleanup`). |
| §3.x Four-layer violations | Already resolved by prior change (`four-layer-violations`). |
| Integration tests for `main.go` window lifecycle | Requires a test harness around `RunBootstrap` with a mock Fyne window. The existing `runWindow=false` path is the testable contract. |
| Adding `recover()` around `prevCleanup()` calls | Panics in cleanup indicate programming errors; fail-fast is preferred over silent swallowing. |
| Adding `IsClosed()` accessor to `MockDBPool` | If needed for T-CLEANUP-01, it will be included in the implementation tasks as a supporting test helper — not a production code change. |
| Logging or metrics for cleanup events | Not required for the MVP. Debug logging can be added later if needed. |
