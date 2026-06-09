# Explore Report: code-quality-antipatterns

**Date:** 2026-06-07  
**Scope:** Audit Section 4 — Code Quality Anti-Patterns  
**Prior Changes Applied:** `fyne-thread-safety`, `screen-lifecycle-cleanup`, `four-layer-violations`

---

## 1. Audit Finding §4.1 — Global Mutable Package State

### Current State

Four package-level mutable `var` declarations remain in `pkg/ui/compositor.go` (lines 17–20):

```go
var DS DataSource                    // line 17
var CWR ColumnWidthResolver          // line 18
var LocalEventBus eventbus.EventBus  // line 19
var Navigate func(vistaID string)    // line 20
```

The original audit cited `BusinessPool` and `CorePool` as package-level globals. These have been **fully removed** by the `four-layer-violations` change — no direct `*pgxpool.Pool` references exist in `pkg/ui/`. DB pools are now encapsulated behind `DataSource` and `ColumnWidthResolver` interfaces, with implementations in `pkg/dataaccess/`.

### Remaining Globals Analysis

| Global | Type | Written By | Read By | Mutated at Runtime | Risk |
|--------|------|-----------|---------|-------------------|------|
| `DS` | `DataSource` (interface) | `main.go` once at bootstrap (line 78) | `compositor.go` in `fetchGridDataAsync`, `loadMasterBuffer` | **No** — set once | **Low** — nil-safe (guarded with `if DS == nil`) |
| `CWR` | `ColumnWidthResolver` (interface) | `main.go` once at bootstrap (line 81) | `compositor.go` in `resolveWidth` | **No** — set once | **Low** — nil-safe (guarded with `if CWR != nil`) |
| `LocalEventBus` | `eventbus.EventBus` (interface) | `main.go` once at bootstrap (line 84) | `compositor.go` throughout, `sidebar_widget.go` (via closure) | **No** — set once | **Low** — nil-safe (guarded with `if LocalEventBus != nil`) |
| `Navigate` | `func(vistaID string)` | `main.go` once at bootstrap (line 107) | `sidebar_widget.go` in `OnSelected`, `compositor.go` in button handler | **No** — set once | **Low** — nil-safe (guarded with `if Navigate != nil`) |

### Verdict

The `BusinessPool`/`CorePool` globals that motivated §4.1 are **gone**. The four remaining globals are all set exactly once during bootstrap (single-threaded, before `ShowAndRun`) and never mutated thereafter. They are nil-guarded at every use site. This is a **write-once, read-many** pattern, not the hazardous mutable state the audit originally flagged.

**However**, the globals still prevent:
- Instantiating multiple independent UI shells in the same process
- Parallel test execution (tests must manually set/teardown globals — see 40+ `defer func() { ui.DS = nil; ui.CWR = nil }()` calls in `compositor_test.go`)

### Structural Refactor Sketch

A `Compositor` struct would hold these as fields:

```go
type Compositor struct {
    DS          DataSource
    CWR         ColumnWidthResolver
    EventBus    eventbus.EventBus
    Navigate    func(vistaID string)
}
```

**Scope assessment:**
- `compositor.go` — `Compose` becomes a method on `*Compositor`; all internal functions (`composeWithState`, `fetchGridDataAsync`, `loadMasterBuffer`, `filterMasterRows`, `resolveWidth`) gain a `*Compositor` parameter or become methods
- `sidebar_widget.go` — `BuildNavTree` needs `Navigate` and `EventBus` injected (currently reads globals)
- `main.go` — constructs `Compositor`, passes it through
- `compositor_test.go` — ~40+ test functions change from setting globals to constructing a `Compositor` struct (~300 lines of test code)
- `sidebar_widget_test.go` — 13 test functions change to inject `Navigate`/`EventBus` through the builder

**Invasiveness:** High. Touches the core rendering pipeline, all tests, and the sidebar builder. Estimated 400–600 lines changed across 6 files.

---

## 2. Audit Finding §4.2 — Non-Atomic Re-entrancy Guard / Data Race

### Current State

The data race **still exists** in `pkg/ui/sidebar_widget.go`.

**The race pattern:**

1. **Writer goroutine** (`main.go` Navigate closure, line 108–130):
   ```go
   ui.Navigate = func(vID string) {
       go func() {
           // ... load and compose screen ...
           navTree.SelectByVistaID(vID)  // line 130
       }()
   }
   ```
   `SelectByVistaID` runs inside a `go func()` — a background goroutine.

2. **`SelectByVistaID`** (`sidebar_widget.go` line 58–59):
   ```go
   nt.navigating = true
   defer func() { nt.navigating = false }()
   nt.tree.Select(widget.TreeNodeID(nodeID))
   ```

3. **Reader — UI thread callback** (`sidebar_widget.go` line 86–87):
   ```go
   tree.OnSelected = func(uid widget.TreeNodeID) {
       if navTree.navigating {
           return
       }
       // ...
       Navigate(item.VistaID)
   }
   ```

**Race:** `nt.navigating` is a plain `bool` written by a background goroutine (`SelectByVistaID`) and read by the UI thread's `OnSelected` callback. No synchronization primitive protects it. Under Go's race detector (`go test -race`), this would be flagged as a data race.

**Note:** The `SelectByVistaID` call happens inside a `go func()` in `main.go` (not the UI thread), but `tree.Select()` is a Fyne widget call that may dispatch synchronously or asynchronously depending on Fyne's internal scheduling. Regardless of Fyne's behavior, the `nt.navigating = true` write itself occurs on the background goroutine with no memory barrier.

### Fix Scope

**Single-file fix** in `pkg/ui/sidebar_widget.go`:
- Change `navigating bool` (line 18) to `navigating atomic.Bool` (from `sync/atomic`)
- Change `nt.navigating = true` (line 58) to `nt.navigating.Store(true)`
- Change `nt.navigating = false` (line 59) to `nt.navigating.Store(false)`
- Change `if navTree.navigating` (line 86) to `if navTree.navigating.Load()`

**Test impact:**
- Existing tests in `sidebar_widget_test.go` exercise the re-entrancy guard (`TestReentrancyGuardPreventsLoop`, `TestSelectByVistaID_ValidSelectsNode`). These should pass unchanged since `atomic.Bool` is API-compatible in behavior.
- No new test file needed, but running `go test -race ./pkg/ui/` would confirm the race is resolved.

**Complexity:** Low. One type change + 4 call-site changes in a single file. The `NavTree` struct is only instantiated in `BuildNavTree` — no other constructors.

---

## 3. Audit Finding §4.3 — Missing Cleanup of Database Pools

### Current State

The missing cleanup issue **still exists** in `cmd/golemui/main.go`.

**Current flow:**
```go
func main() {
    // ... config parsing ...
    _, err = RunBootstrap(ctx, cfg, true, nil)  // line 181
    if err != nil {
        log.Fatalf("Bootstrap error: %v", err)
    }
    // main() exits here after ShowAndRun() returns
}
```

When `runWindow` is `true`, `RunBootstrap` calls `win.ShowAndRun()` (line 157), which blocks until the window is closed. When `ShowAndRun` returns, `RunBootstrap` returns the `*App` to `main()`, but `main()` discards the `*App` reference and exits without calling `dbPool.Close()`.

**Partial cleanup exists on error paths** (lines 93, 141, 147) — `dbPool.Close()` is called when bootstrap fails. But the **happy path** (successful startup → window lifecycle → graceful close) has no cleanup.

**What `db.DB.Close()` does** (`pkg/db/db.go` line 87–93):
```go
func (db *DB) Close() {
    if db.CorePool != nil {
        db.CorePool.Close()
    }
    if db.BusinessPool != nil {
        db.BusinessPool.Close()
    }
}
```

Both `CorePool` and `BusinessPool` are `pgxpool.Pool` instances. `pgxpool.Pool.Close()` gracefully drains connections. Without calling it, connections are terminated abruptly on process exit — which is a minor issue for local development (OS reclaims resources) but problematic for production deployments with connection-counted servers.

### Additional missing cleanup: `prevCleanup`

The Navigate closure tracks the current screen's cleanup function in a local `prevCleanup` variable (line 105). On window close, this final cleanup function is **never called**. This means:
- The home screen's EventBus subscriptions are never unsubscribed
- In-flight goroutines from the last composed screen are never cancelled
- The `ScreenState` may leak goroutines

### Fix Scope

**Two changes in `cmd/golemui/main.go`:**

1. **Window close hook** — register `win.SetOnClosed()` before `ShowAndRun()`:
   ```go
   win.SetOnClosed(func() {
       if prevCleanup != nil {
           prevCleanup()
       }
       dbPool.Close()
   })
   ```

2. **Non-blocking mode** — when `runWindow` is `false`, the caller receives `*App` and is responsible for calling `a.DB.Close()`. This is already the contract (tests can call `a.DB.Close()` directly). No change needed for this path.

**Constraints:**
- `SetOnClosed` is a Fyne API that fires when the user closes the window. It runs on the UI thread.
- The `prevCleanup` variable is captured by closure — it must be accessible in the `SetOnClosed` callback. Currently `prevCleanup` is declared at line 105, which is in the same scope as where `SetOnClosed` would be registered. This works naturally.
- The `Navigate` goroutine (line 108) may be in-flight when the window closes. The cleanup function cancels contexts via `context.CancelFunc`, so in-flight goroutines will exit gracefully.

**Test impact:**
- `main.go` has no unit tests currently. Integration testing would require verifying that `SetOnClosed` fires and pools are drained, which would need a test harness around `RunBootstrap`.
- The `runWindow=false` path already returns `*App` — tests using this path should call `a.DB.Close()` (they may already, or may not — needs verification).

**Complexity:** Low. ~8 lines added to `main.go`. Single-file change.

---

## 4. Cross-Cutting: `prevCleanup` Data Race in Navigate Closure

### Additional finding discovered during exploration

In `main.go` lines 105–130, the `prevCleanup` variable is shared between:
- **Writer:** The `go func()` inside the `Navigate` closure (background goroutine) — reads, calls, and sets `prevCleanup` to nil/new value
- **Proposed writer:** The `SetOnClosed` callback (UI thread) — would read and call `prevCleanup`

This is the same pattern as §4.2 — a non-atomic shared variable across goroutines. If the window close hook and a Navigate goroutine run concurrently, `prevCleanup` could be called twice or have its pointer read non-atomically.

**Mitigation:** Use `sync.Mutex` or `sync.Once` to serialize access to `prevCleanup`, or use `atomic.Pointer[func()]`. The simplest approach:

```go
var cleanupMu sync.Mutex
var prevCleanup func()

// In Navigate goroutine:
cleanupMu.Lock()
if prevCleanup != nil {
    prevCleanup()
    prevCleanup = nil
}
// ... compose new screen ...
prevCleanup = newCleanup
cleanupMu.Unlock()

// In SetOnClosed:
cleanupMu.Lock()
if prevCleanup != nil {
    prevCleanup()
    prevCleanup = nil
}
cleanupMu.Unlock()
```

This is a small additional scope item that should be bundled with the §4.3 fix.

---

## 5. Dependency Map

```
pkg/ui/compositor.go          ← defines DS, CWR, LocalEventBus, Navigate globals
pkg/ui/sidebar_widget.go      ← reads Navigate (global), has navigating data race
pkg/ui/datasource.go          ← defines DataSource, ColumnWidthResolver interfaces
pkg/ui/screen_state.go        ← per-screen state (thread-safe, no issues)
pkg/ui/screen_loader.go       ← LoadScreen (pure function, no globals)
pkg/ui/sidebar_loader.go      ← LoadNavigationMenu (pure function, no globals)
pkg/ui/layout.go              ← FractionalLayout (pure, no issues)
pkg/dataaccess/               ← implements DataSource, ColumnWidthResolver
pkg/db/db.go                  ← DB.Close() method exists, not called on happy path
cmd/golemui/main.go           ← bootstrap, wiring, missing pool cleanup, prevCleanup race
```

---

## 6. Summary and Recommendations

| Finding | Status | Fix Complexity | Priority | Files Changed |
|---------|--------|---------------|----------|---------------|
| §4.1 DB pool globals (BusinessPool/CorePool) | **Resolved** by `four-layer-violations` | N/A | N/A | N/A |
| §4.1 Remaining 4 globals (DS, CWR, EventBus, Navigate) | **Mitigated** — write-once, nil-guarded. Structural risk remains (no parallel tests, single-shell). | **High** — 400-600 lines across 6 files | **Low** — deferred to post-MVP | `compositor.go`, `sidebar_widget.go`, `main.go`, `compositor_test.go`, `sidebar_widget_test.go`, plus internal test files |
| §4.2 `navigating` data race | **Confirmed** — `bool` written by goroutine, read by UI thread | **Low** — 5 changes in 1 file | **High** — data race | `sidebar_widget.go` |
| §4.3 Missing pool cleanup | **Confirmed** — no shutdown hook on happy path | **Low** — ~8 lines in 1 file | **Medium** — resource leak | `main.go` |
| Cross-cut: `prevCleanup` race | **Discovered** — shared variable across goroutines | **Low** — bundled with §4.3 fix | **Medium** — data race | `main.go` |

### Recommended Fix Order

1. **§4.2** (`atomic.Bool` for sidebar) — smallest, most impactful (actual data race)
2. **§4.3** + cross-cut `prevCleanup` race — pool cleanup + mutex guard
3. **§4.1 structural refactor** (Compositor struct) — deferred; current write-once globals are safe in practice but block future scalability

### Risks

- **§4.2 atomic.Bool:** Minimal risk. `atomic.Bool` is zero-value `false`, matching current `bool` semantics. All existing tests exercise the guard path.
- **§4.3 SetOnClosed:** Must verify `SetOnClosed` fires reliably on all platforms (Fyne behavioral contract). The `prevCleanup` mutex introduces a minor lock ordering concern — the Navigate goroutine holds the lock while calling `prevCleanup()`, which may block on context cancellation. This is acceptable since cleanup is expected to be fast.
- **§4.1 refactor:** High regression surface. Should be proposed as a separate SDD change with its own spec/tasks, not bundled with the simpler fixes.
