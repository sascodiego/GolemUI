# Proposal: code-quality-antipatterns

**Change ID:** `code-quality-antipatterns`  
**Date:** 2026-06-07  
**Status:** Draft  
**Source Audit:** `docs/specify/code_audit_report.md` — Sections 4.2, 4.3  
**Explore Report:** `openspec/changes/code-quality-antipatterns/explore.md`  
**Config:** `openspec/config.yaml` — strict TDD enabled

---

## 1. Summary

Fix two confirmed concurrency anti-patterns in the GolemUI runtime:

1. **§4.2 — Data race on `NavTree.navigating`**: A plain `bool` is written by a background goroutine (`SelectByVistaID`) and read by the UI thread's `OnSelected` callback with no synchronization. Replace with `sync/atomic.Bool`.

2. **§4.3 — Missing pool cleanup + `prevCleanup` race**: No shutdown hook exists for the happy-path window close — database pools are never gracefully drained, and the last screen's cleanup function is never invoked. Additionally, `prevCleanup` is read/written across goroutines without synchronization. Fix with `win.SetOnClosed()` and `sync.Mutex` protection.

Both fixes are small, single-digit-line changes in two files. No structural refactoring is included.

---

## 2. Problem Statement

### 2.1 Non-Atomic Re-entrancy Guard (§4.2)

In [`pkg/ui/sidebar_widget.go`](file:///src/GolemUI/pkg/ui/sidebar_widget.go), the `NavTree` struct uses a plain `bool` field `navigating` as a re-entrancy guard:

```go
// sidebar_widget.go:19
navigating  bool
```

The field is written on a background goroutine (called from `ui.Navigate` → `go func()` in `main.go:109`) and read on the UI thread inside the `OnSelected` callback (`sidebar_widget.go:127`). Under Go's memory model, this constitutes a **data race** — the compiler and CPU may reorder or cache the value, and `go test -race` will flag it.

**Why it matters:** Data races are undefined behavior. In practice, the `OnSelected` callback may see a stale `false` when it should see `true`, causing spurious navigation calls (re-entrancy loop). Conversely, it may see a stale `true` when it should see `false`, suppressing legitimate user-initiated navigation.

### 2.2 Missing Pool Cleanup and `prevCleanup` Race (§4.3)

In [`cmd/golemui/main.go`](file:///src/GolemUI/cmd/golemui/main.go):

1. **No shutdown hook on the happy path.** When `runWindow` is `true`, `RunBootstrap` calls `win.ShowAndRun()` which blocks until the window closes. On return, `main()` exits the process without calling `dbPool.Close()`. The error paths (lines 93, 141, 147) correctly call `dbPool.Close()`, but the success path does not. `pgxpool.Pool` connections are terminated abruptly rather than gracefully drained.

2. **Last screen cleanup never called.** The `prevCleanup` closure (tracking the current screen's cancel/unsubscribe function) is only invoked during navigation transitions. When the window closes, the final screen's EventBus subscriptions and context cancellations are never cleaned up — leaking goroutines.

3. **`prevCleanup` data race.** The `prevCleanup` variable is read/written by the `Navigate` goroutine (background) and would also be accessed by the proposed `SetOnClosed` callback (UI thread). Concurrent access to the same `func()` variable without synchronization is a data race.

**Why it matters:** In production deployments with connection-counted PostgreSQL servers, leaked connections exhaust the pool. Uncleaned goroutines hold references to dead widget trees, causing memory pressure. The `prevCleanup` race can cause double-invocation of cleanup or missed cleanup under concurrent window-close + navigation.

---

## 3. Goals and Non-Goals

### Goals

| # | Goal | Audit Section |
|---|------|---------------|
| G1 | Eliminate the `navigating` data race in `NavTree` using `sync/atomic.Bool` | §4.2 |
| G2 | Add graceful database pool cleanup on window close via `win.SetOnClosed()` | §4.3 |
| G3 | Invoke `prevCleanup` on window close to release the last screen's resources | §4.3 |
| G4 | Protect `prevCleanup` access with `sync.Mutex` to prevent cross-goroutine races | §4.3 |
| G5 | Pass `go test -race ./...` with zero race detector warnings for the touched code paths | All |

### Non-Goals

| # | Non-Goal | Rationale |
|---|----------|-----------|
| NG1 | Refactor remaining globals (`DS`, `CWR`, `LocalEventBus`, `Navigate`) into a `Compositor` struct | Deferred to post-MVP — 400-600 lines across 6 files. Current write-once globals are nil-guarded and low risk. |
| NG2 | Address §1.x thread-safety violations (Fyne widget mutation from goroutines) | Separate SDD change (`fyne-thread-safety`) — already tracked. |
| NG3 | Address §2.x lifecycle issues (premature cleanup, zombie screens) | Separate SDD change (`screen-lifecycle-cleanup`) — already tracked. |
| NG4 | Address §3.x four-layer violations | Separate SDD change (`four-layer-violations`) — already resolved. |
| NG5 | Add integration tests for `main.go` window lifecycle | Out of scope — requires a test harness around `RunBootstrap` with a mock Fyne window. The existing `runWindow=false` path is the testable contract. |

---

## 4. Proposed Solution

### 4.1 §4.2: Replace `bool` with `atomic.Bool` in `NavTree`

**File:** `pkg/ui/sidebar_widget.go`

Change the `navigating` field from `bool` to `sync/atomic.Bool` and update all access sites:

| Line | Before | After |
|------|--------|-------|
| Struct field (L19) | `navigating  bool` | `navigating  atomic.Bool` |
| Import | `"fyne.io/fyne/v2/widget"` etc. | Add `"sync/atomic"` |
| Write true (L80) | `nt.navigating = true` | `nt.navigating.Store(true)` |
| Write false (L81) | `nt.navigating = false` | `nt.navigating.Store(false)` |
| Read (L127) | `if navTree.navigating {` | `if navTree.navigating.Load() {` |

`atomic.Bool` zero-value is `false`, matching the current `bool` semantics. No constructor changes needed.

### 4.2 §4.3: Pool Cleanup Hook + `prevCleanup` Mutex

**File:** `cmd/golemui/main.go`

**Step 1 — Protect `prevCleanup` with `sync.Mutex`:**

Wrap all `prevCleanup` accesses (read, call, assign, nil) inside a `cleanupMu sync.Mutex` lock:

```go
var cleanupMu sync.Mutex
var prevCleanup func()
```

The Navigate goroutine body becomes:

```go
ui.Navigate = func(vID string) {
    log.Printf("[UI/Navigation] Navigating to screen %q", vID)
    go func() {
        cleanupMu.Lock()
        if prevCleanup != nil {
            prevCleanup()
            prevCleanup = nil
        }
        cleanupMu.Unlock()

        node, err := ui.LoadScreen(ctx, dbPool.CorePool, vID, cfg.LayoutQuery)
        if err != nil {
            log.Printf("[UI/Navigation] Error loading screen %q: %v", vID, err)
            return
        }
        newUI, cleanup, err := ui.Compose(node, vID)
        if err != nil {
            log.Printf("[UI/Navigation] Error composing screen %q: %v", vID, err)
            return
        }

        cleanupMu.Lock()
        prevCleanup = cleanup
        cleanupMu.Unlock()

        mainContainer.Objects = []fyne.CanvasObject{newUI}
        mainContainer.Refresh()
        navTree.SelectByVistaID(vID)
    }()
}
```

**Step 2 — Register `win.SetOnClosed()` before `ShowAndRun()`:**

```go
if runWindow {
    win.SetOnClosed(func() {
        cleanupMu.Lock()
        if prevCleanup != nil {
            prevCleanup()
            prevCleanup = nil
        }
        cleanupMu.Unlock()
        dbPool.Close()
    })
    win.ShowAndRun()
}
```

This ensures:
- On window close, the final screen's cleanup runs (unsubscribing events, cancelling contexts).
- `dbPool.Close()` gracefully drains both `CorePool` and `BusinessPool` connections.
- `prevCleanup` is always accessed under the mutex, preventing races between `SetOnClosed` (UI thread) and `Navigate` (background goroutine).

---

## 5. Data Flow: Before and After

### 5.1 §4.2 — `navigating` Re-entrancy Guard

**Before (racy):**

```
Background Goroutine (Navigate → SelectByVistaID)    UI Thread (OnSelected)
──────────────────────────────────────────────────    ──────────────────────
nt.navigating = true          ───no barrier───►       reads navTree.navigating
nt.tree.Select(nodeID)                                if navTree.navigating {
defer: nt.navigating = false  ───no barrier───►         return  // may see stale value
                                                     }
```

`nt.navigating` has no memory barrier. The UI thread may read a stale `false` (causing re-entrancy) or a stale `true` (suppressing navigation).

**After (safe):**

```
Background Goroutine                                UI Thread
──────────────────────────────────────────          ──────────────────────────
nt.navigating.Store(true)   ───atomic barrier──►    if navTree.navigating.Load() {
nt.tree.Select(nodeID)                                return  // always sees true
defer: nt.navigating.Store(false)                   }
```

`atomic.Bool` provides `SeqCst` ordering by default. The UI thread always observes the latest value.

### 5.2 §4.3 — Window Close Lifecycle

**Before (leaky):**

```
main() → RunBootstrap(runWindow=true)
  ├── initDB → dbPool
  ├── ShowAndRun()  ← blocks until window closes
  ├── return *App   ← discarded by main()
  └── [process exits — no dbPool.Close(), no prevCleanup()]
```

**After (clean):**

```
main() → RunBootstrap(runWindow=true)
  ├── initDB → dbPool
  ├── win.SetOnClosed(func() {
  │     cleanupMu.Lock()
  │     prevCleanup()      ← unsubscribes events, cancels contexts
  │     prevCleanup = nil
  │     cleanupMu.Unlock()
  │     dbPool.Close()     ← gracefully drains both pools
  │ })
  ├── ShowAndRun()  ← blocks; OnClosed fires on window close
  └── return *App
```

`prevCleanup` is also protected during navigation:

```
Navigate goroutine                     SetOnClosed (UI thread)
─────────────────────                  ─────────────────────────
cleanupMu.Lock()                       cleanupMu.Lock()  ← blocks
prevCleanup()                          ...waits...
prevCleanup = nil
cleanupMu.Unlock()
...compose new screen...
cleanupMu.Lock()
prevCleanup = newCleanup
cleanupMu.Unlock()                     prevCleanup()
                                       prevCleanup = nil
                                       cleanupMu.Unlock()
                                       dbPool.Close()
```

The mutex serializes all access — no double-call, no stale reads.

---

## 6. Risks and Mitigations

| # | Risk | Likelihood | Impact | Mitigation |
|---|------|-----------|--------|------------|
| R1 | `atomic.Bool` changes `NavTree` zero-value semantics | Very Low | Low | `atomic.Bool` zero-value is `false`, identical to `bool`. No behavior change. |
| R2 | `SetOnClosed` does not fire on all platforms | Low | Medium | `SetOnClosed` is a stable Fyne API (present since v2.0). Verified to fire on desktop (GLFW) and mobile. Test with manual close. |
| R3 | `cleanupMu` Lock held while calling `prevCleanup()` blocks the other goroutine | Low | Low | `prevCleanup()` calls `cancel()` (context cancellation) and `unsubscribe()` (map deletion) — both are fast, non-blocking operations. No I/O or network calls inside cleanup. |
| R4 | Race between `SetOnClosed` and an in-flight `Navigate` goroutine that is mid-compose | Low | Medium | The mutex ensures only one runs at a time. The Navigate goroutine will complete its compose and assign `prevCleanup` before `SetOnClosed` can acquire the lock. The closed window means the composed UI is discarded — this is acceptable. |
| R5 | Existing tests in `sidebar_widget_test.go` break due to `atomic.Bool` | Very Low | Low | `atomic.Bool` is API-compatible in behavior. Existing tests (`TestReentrancyGuardPreventsLoop`, `TestSelectByVistaID_ValidSelectsNode`) exercise the guard path with no changes needed. |

---

## 7. Success Criteria

| # | Criterion | Verification Method |
|---|-----------|-------------------|
| SC1 | `go test -race ./pkg/ui/` passes with zero race detector warnings for `navigating` | Run with `-race` flag |
| SC2 | `go test -race ./cmd/golemui/` passes with zero race detector warnings for `prevCleanup` | Run with `-race` flag (if tests exist; otherwise manual verification) |
| SC3 | `go test ./...` passes — all existing tests green | CI test run |
| SC4 | `go build ./...` compiles without errors | Build check |
| SC5 | `golangci-lint run` produces no new warnings | Linter check |
| SC6 | Manual close of the GolemUI window triggers `dbPool.Close()` (verified via log or debug print) | Manual test or log assertion |
| SC7 | Existing re-entrancy guard tests (`TestReentrancyGuardPreventsLoop`) continue to pass unchanged | Test run |

---

## 8. Rollback Plan

Both changes are localized to two files with no schema or API modifications:

1. **§4.2 rollback:** Revert `navigating` from `atomic.Bool` back to `bool` in `sidebar_widget.go`, restore plain assignments and direct reads, remove `sync/atomic` import. 5 line changes.
2. **§4.3 rollback:** Remove `win.SetOnClosed()` block and `cleanupMu` declarations from `main.go`, restore plain `prevCleanup` variable. ~15 line changes.

No database migrations, no plugin changes, no interface modifications. A clean `git revert` of the change commit fully restores the prior state.

---

## 9. Dependency and Predecessor Map

```
Predecessor changes (already merged):
  ├── fyne-thread-safety
  ├── screen-lifecycle-cleanup
  └── four-layer-violations  ← removed BusinessPool/CorePool globals

This change:
  └── code-quality-antipatterns (§4.2 + §4.3)

Deferred (post-MVP):
  └── Structural refactor: Compositor struct (§4.1 remaining globals)
```

No other SDD changes depend on this work. It can be merged independently.

---

## 10. Files Changed (Estimated)

| File | Changes | Lines |
|------|---------|-------|
| `pkg/ui/sidebar_widget.go` | `bool` → `atomic.Bool`, import, 4 call-sites | ~6 lines |
| `cmd/golemui/main.go` | `sync.Mutex`, `SetOnClosed`, mutex-protected `prevCleanup` | ~20 lines |
| `pkg/ui/sidebar_widget_test.go` | No changes expected | 0 lines |
| **Total** | | **~26 lines** |
