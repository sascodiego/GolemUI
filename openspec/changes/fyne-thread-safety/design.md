# SDD Design — fyne-thread-safety

> Technical design for fixing Fyne thread-safety violations and making screen navigation asynchronous.

---

## 1. Overview

Two independent work streams, applied in dependency order:

| Order | Work Stream | Files | Description |
|-------|-------------|-------|-------------|
| 1st | **A: Compositor `fyne.Do` wrapping** | `pkg/ui/compositor.go` | Wrap 6 unsafe widget mutations (4 call sites) in `fyne.Do()` |
| 2nd | **B: Async Navigate** | `cmd/golemui/main.go` | Move `LoadScreen` + `Compose` into a goroutine, wrap UI swap in `fyne.Do()` |

**Why this order:** Work Stream A is the harder, more intricate change (4 sites, deadlock-sensitive lock ordering). It fixes the data races. Work Stream B is a simpler structural change to one closure. Fixing A first means the race detector is already clean when B is tested, making failures easier to attribute.

---

## 2. Work Stream A — Compositor `fyne.Do` wrapping

### 2.1 Principle

Every `table.Refresh()` and `table.SetColumnWidth()` call that executes from a background goroutine must be wrapped in `fyne.Do(func() { ... })`. The `fyne.Do` function queues the callback onto the Fyne UI thread and returns immediately — it does not block the calling goroutine.

### 2.2 Lock ordering constraint (REQ-LOCK-01)

**Critical invariant:** `model.mu.Unlock()` MUST complete before `fyne.Do()` is entered.

Why: `table.Refresh()` triggers Fyne to call the table's `Length()` and `CreateCell()` callbacks. These callbacks acquire `model.mu.RLock()`. If the write lock is still held when `fyne.Do` executes, the goroutine blocks waiting for the UI thread, the UI thread blocks trying to acquire `RLock` → deadlock.

Correct pattern:
```
model.mu.Lock()
// ... write model fields ...
model.mu.Unlock()   // ← MUST happen first
fyne.Do(func() {
    // table.Refresh() → Length()/CreateCell() → model.mu.RLock() — safe
})
```

Incorrect pattern (DEADLOCK):
```
model.mu.Lock()
// ... write model fields ...
fyne.Do(func() {
    model.mu.Unlock()   // ← WRONG: UI thread already trying to RLock inside Refresh
    table.Refresh()
})
```

### 2.3 Wrap Point 1 — `loadMasterBuffer` goroutine

**Location:** `compositor.go:361-366` (after `model.mu.Unlock()` at line 360)

**Before:**
```go
		model.mu.Lock()
		model.masterHeaders = headers
		model.masterRows = dataRows
		model.headers = headers
		model.rows = dataRows
		model.mu.Unlock()

		for i := 0; i < len(headers); i++ {
			table.SetColumnWidth(i, 150)
		}
		table.Refresh()
```

**After:**
```go
		model.mu.Lock()
		model.masterHeaders = headers
		model.masterRows = dataRows
		model.headers = headers
		model.rows = dataRows
		model.mu.Unlock()

		fyne.Do(func() {
			for i := 0; i < len(headers); i++ {
				table.SetColumnWidth(i, 150)
			}
			table.Refresh()
		})
```

**Requirements:** REQ-THREAD-01, REQ-LOCK-01
**Thread context:** Goroutine G1 (spawned at line 321)
**Captured variables:** `headers` (local slice, immutable after construction), `table` (pointer, safe to reference from closure)

### 2.4 Wrap Point 2 — `filterMasterRows` empty-snapshot early return

**Location:** `compositor.go:415-416` (inside the `if len(snap) == 0` branch)

**Before:**
```go
	if len(snap) == 0 {
		model.rows = model.masterRows
		model.mu.Unlock()
		table.Refresh()
		return
	}
```

**After:**
```go
	if len(snap) == 0 {
		model.rows = model.masterRows
		model.mu.Unlock()
		fyne.Do(func() {
			table.Refresh()
		})
		return
	}
```

**Requirements:** REQ-THREAD-03, REQ-LOCK-01
**Thread context:** Goroutine G3 (EventBus subscriber handler dispatched via `go h(event)` at eventbus.go:79)
**Note:** The `model.mu.Unlock()` stays before the `fyne.Do` block. The `return` exits `filterMasterRows` — the `fyne.Do` callback runs asynchronously on the UI thread after the function returns.

### 2.5 Wrap Point 3 — `filterMasterRows` normal exit

**Location:** `compositor.go:450` (end of function, after `model.mu.Unlock()`)

**Before:**
```go
	model.rows = filtered
	model.mu.Unlock()

	table.Refresh()
```

**After:**
```go
	model.rows = filtered
	model.mu.Unlock()

	fyne.Do(func() {
		table.Refresh()
	})
```

**Requirements:** REQ-THREAD-03, REQ-LOCK-01
**Thread context:** Goroutine G3 (same as Wrap Point 2)

### 2.6 Wrap Point 4 — `fetchGridDataAsync` goroutine

**Location:** `compositor.go:529-534` (after `model.mu.Unlock()` at line 528)

**Before:**
```go
		model.mu.Lock()
		model.headers = headers
		model.columns = headers
		model.rows = dataRows
		model.mu.Unlock()

		for i := 0; i < len(headers); i++ {
			table.SetColumnWidth(i, 150)
		}
		table.Refresh()
```

**After:**
```go
		model.mu.Lock()
		model.headers = headers
		model.columns = headers
		model.rows = dataRows
		model.mu.Unlock()

		fyne.Do(func() {
			for i := 0; i < len(headers); i++ {
				table.SetColumnWidth(i, 150)
			}
			table.Refresh()
		})
```

**Requirements:** REQ-THREAD-02, REQ-LOCK-01
**Thread context:** Goroutine G2 (spawned at line 480)
**Captured variables:** `headers` (local slice), `table` (pointer)

### 2.7 What does NOT change in compositor.go

| Code | Why it stays unchanged |
|------|----------------------|
| `model.mu.Lock()` / `model.mu.Unlock()` pattern | Already correct — only the post-unlock UI calls need wrapping |
| `loadMasterBuffer` goroutine spawn (`go func()` at L321) | Stays a goroutine — we wrap its UI calls, not its structure |
| `fetchGridDataAsync` goroutine spawn (`go func()` at L480) | Same |
| EventBus subscriber handler body (L216-258) | Calls `filterMasterRows` and `fetchGridDataAsync` — both fixed internally |
| `table.OnSelected` callback (L266) | Runs on the UI thread (Fyne dispatch), only calls `Publish` — safe |
| `composeWithState` data_grid branch (L128-290) | Structure unchanged; `loadMasterBuffer` and `fetchGridDataAsync` are called the same way |

---

## 3. Work Stream B — Async Navigate

### 3.1 Current code

**Location:** `cmd/golemui/main.go:101-116`

```go
ui.Navigate = func(vID string) {
    log.Printf("[UI/Navigation] Navigating to screen %q", vID)
    node, err := ui.LoadScreen(ctx, ui.CorePool, vID, cfg.LayoutQuery)
    if err != nil {
        log.Printf("[UI/Navigation] Error loading screen %q: %v", vID, err)
        return
    }
    newUI, err := ui.Compose(node, vID)
    if err != nil {
        log.Printf("[UI/Navigation] Error composing screen %q: %v", vID, err)
        return
    }
    mainContainer.Objects = []fyne.CanvasObject{newUI}
    mainContainer.Refresh()
    navTree.SelectByVistaID(vID)
}
```

### 3.2 Transformed code

```go
ui.Navigate = func(vID string) {
    log.Printf("[UI/Navigation] Navigating to screen %q", vID)
    go func() {
        node, err := ui.LoadScreen(ctx, ui.CorePool, vID, cfg.LayoutQuery)
        if err != nil {
            log.Printf("[UI/Navigation] Error loading screen %q: %v", vID, err)
            return
        }
        newUI, err := ui.Compose(node, vID)
        if err != nil {
            log.Printf("[UI/Navigation] Error composing screen %q: %v", vID, err)
            return
        }
        fyne.Do(func() {
            mainContainer.Objects = []fyne.CanvasObject{newUI}
            mainContainer.Refresh()
            navTree.SelectByVistaID(vID)
        })
    }()
}
```

### 3.3 Design decisions

| Decision | Rationale |
|----------|-----------|
| `go func()` wraps the entire body | The callback returns immediately to Fyne's button-tap dispatcher, unblocking the UI thread |
| `fyne.Do` wraps only the UI mutation | `LoadScreen` and `Compose` are pure data/logic operations — no Fyne widgets involved. Only the container swap needs the UI thread |
| Signature unchanged (`func(vistaID string)`) | REQ-INVARIANT-01 — callers (button callbacks) see no API change |
| No cancellation token for overlapping navigations | Out of scope for this change. Rapid clicks spawn multiple goroutines; the last `fyne.Do` to execute wins. Acceptable trade-off for a first fix |
| Error handling: log + return from goroutine | Previous UI stays visible and responsive. No modal dialog, no panic |

### 3.4 Closure capture safety

| Variable | Type | Safe to capture? | Reason |
|----------|------|-----------------|--------|
| `ctx` | `context.Context` | Yes | Immutable interface value set once in `RunBootstrap` |
| `cfg.LayoutQuery` | `string` | Yes | Value copied into closure at assignment time |
| `ui.CorePool` | `*pgxpool.Pool` | Yes | Set once during bootstrap, never reassigned |
| `mainContainer` | `*fyne.Container` | Yes | Pointer to a stable container created at bootstrap; only mutated inside `fyne.Do` |
| `navTree` | `*ui.NavTree` | Yes | Pointer to stable tree created at bootstrap; `SelectByVistaID` only called inside `fyne.Do` |
| `vID` | `string` | Yes | Value parameter, copied into closure at each call |

### 3.5 What does NOT change in main.go

| Code | Why |
|------|-----|
| `win.SetContent(split)` at L138 | Called once at bootstrap before `win.ShowAndRun()`. No thread-safety issue — event loop hasn't started |
| Bootstrap home screen load (L124-132) | Synchronous — runs before `ShowAndRun`. Correct as-is |
| `mainContainer` creation (L90) | Stable container, exists for the lifetime of the app |

---

## 4. Concurrency Model

### 4.1 Data flow diagram

```
┌─────────────────────────────────────────────────────────────┐
│  Fyne UI Thread                                              │
│                                                               │
│  ┌──────────┐    ┌──────────────┐    ┌──────────────────┐   │
│  │ Button   │───>│ ui.Navigate  │───>│ go func()        │   │
│  │ callback │    │ (returns     │    │   LoadScreen()   │   │
│  │ (tap)    │    │  immediately)│    │   Compose()      │   │
│  └──────────┘    └──────────────┘    └──────┬───────────┘   │
│                                              │               │
│                                     success  │  error        │
│                                              ▼               │
│                                     ┌────────────────┐       │
│                                     │ fyne.Do(func(){│       │
│                                     │   container    │       │
│                                     │   swap +       │       │
│                                     │   Refresh()    │       │
│                                     │ })             │       │
│                                     └────────────────┘       │
│                                              │               │
│  ┌───────────────────────────────────────────┘               │
│  │                                                             │
│  ▼  (UI update happens here on UI thread)                     │
└─────────────────────────────────────────────────────────────┘
```

```
┌─────────────────────────────────────────────────────────────┐
│  Background Goroutine (G1/G2/G3)                             │
│                                                               │
│  ┌──────────────┐                                            │
│  │ DB Query     │                                            │
│  │ (pgx)        │                                            │
│  └──────┬───────┘                                            │
│         │ results                                             │
│         ▼                                                     │
│  ┌──────────────────┐                                        │
│  │ model.mu.Lock()  │                                        │
│  │ write model      │                                        │
│  │ model.mu.Unlock()│  ← MUST complete before fyne.Do       │
│  └──────┬───────────┘                                        │
│         │                                                     │
│         ▼                                                     │
│  ┌──────────────────────────┐                                │
│  │ fyne.Do(func() {         │                                │
│  │   SetColumnWidth +       │─── queued onto UI thread       │
│  │   table.Refresh()        │                                │
│  │ })                       │                                │
│  └──────────────────────────┘                                │
│                                                               │
└─────────────────────────────────────────────────────────────┘
```

### 4.2 Goroutine inventory after change

| ID | Spawn point | What runs inside | UI mutation via |
|----|-------------|-----------------|----------------|
| G1 | `loadMasterBuffer` L321 | DB query → model write → `fyne.Do(SetColumnWidth + Refresh)` | `fyne.Do` |
| G2 | `fetchGridDataAsync` L480 | DB query → model write → `fyne.Do(SetColumnWidth + Refresh)` | `fyne.Do` |
| G3 | `EventBus.Publish` eventbus.go:79 | Handler → `filterMasterRows` or `fetchGridDataAsync` → `fyne.Do(Refresh)` | `fyne.Do` |
| G4 | `ui.Navigate` goroutine (new) | `LoadScreen` → `Compose` → `fyne.Do(container swap)` | `fyne.Do` |

All four goroutine types dispatch UI work through `fyne.Do`. No goroutine mutates Fyne widgets directly.

---

## 5. Deadlock Analysis

### 5.1 Scenario: `model.mu` vs `fyne.Do`

The only lock in the system is `model.mu` (per-data_grid `sync.RWMutex`). The deadlock risk arises from the interaction between the write lock and the UI thread's read lock during `table.Refresh()`.

**Safe execution order:**
```
Goroutine:   Lock → write → Unlock → fyne.Do(returns immediately)
UI Thread:                                ... → Refresh() → Length() → RLock → read → RUnlock
```

The `fyne.Do` call returns immediately (non-blocking). The UI thread later executes the callback. By the time `Refresh()` runs on the UI thread, the goroutine has already released `model.mu`. The `RLock` in the callback succeeds immediately.

**Deadlock scenario (if we got the ordering wrong):**
```
Goroutine:   Lock → write → fyne.Do(blocked, waiting for UI thread to execute callback)
UI Thread:                                ... → Refresh() → Length() → RLock(BLOCKED: write lock held)
```

This deadlock is prevented by the invariant in §2.2.

### 5.2 Scenario: `fyne.Do` within `fyne.Do`

If `table.Refresh()` inside a `fyne.Do` callback itself triggers code that calls `fyne.Do` again, the inner `fyne.Do` runs synchronously (Fyne detects we're already on the UI thread and executes inline). No deadlock.

### 5.3 Scenario: Multiple goroutines competing for the same `model.mu`

Multiple goroutines (G1, G2, G3) may target the same `dataGridModel`. However, they only acquire the write lock for a short, bounded section (copy headers + rows). After `Unlock`, they each call `fyne.Do` independently. The `fyne.Do` callbacks execute serially on the UI thread, and each `Refresh()` triggers `RLock` which succeeds because no write lock is held. No deadlock possible.

---

## 6. Error Handling

### 6.1 Work Stream A — Compositor

| Error location | Current behavior | Behavior after change |
|---------------|-----------------|---------------------|
| DB query fails in `loadMasterBuffer` | Log + return from goroutine. No UI mutation. | **Unchanged.** Error path never reaches `table.Refresh`, so no `fyne.Do` needed. |
| DB query fails in `fetchGridDataAsync` | Log + return from goroutine. No UI mutation. | **Unchanged.** Same reason. |
| Context cancelled before model write | Log + return. No UI mutation. | **Unchanged.** |
| `filterMasterRows` with empty master buffer | Early return at `if len(model.masterRows) == 0` — no `table.Refresh` called. | **Unchanged.** No `fyne.Do` needed on this path. |

No new error paths are introduced by the `fyne.Do` wrapping. The `fyne.Do` function itself does not return an error; it queues the callback and returns `nil`.

### 6.2 Work Stream B — Navigate

| Error location | Behavior |
|---------------|----------|
| `LoadScreen` returns error | Log error, return from goroutine. Previous `mainContainer` content unchanged. UI remains responsive. |
| `Compose` returns error | Log error, return from goroutine. Previous `mainContainer` content unchanged. |
| Panic inside goroutine | Would crash the application. We do NOT add `recover()` — panics in `LoadScreen`/`Compose` indicate a programming error that should surface, not be silently swallowed. This matches the existing behavior (a panic in the synchronous version also crashes the app). |

---

## 7. Test Strategy

### 7.1 Fyne test environment

`fyne.Do` in the Fyne test environment (`test.App()`) executes callbacks **synchronously** on the calling goroutine. This means:

- Tests can assert `fyne.Do` effects immediately after the triggering call, without polling.
- No real event loop is needed.
- The race detector still works — it validates that the model reads in `Length()`/`CreateCell()` don't race with the model writes.

### 7.2 TDD-01: Navigate returns immediately (REQ-ASYNC-01)

```go
func TestNavigate_NonBlocking(t *testing.T) {
    // Setup: configure ui.Navigate with a LoadScreen that blocks on a channel
    blockCh := make(chan struct{})
    var loadCalled atomic.Bool
    
    // ... setup Navigate closure with mock LoadScreen that:
    //   1. sets loadCalled = true
    //   2. blocks on <-blockCh
    
    done := make(chan struct{})
    go func() {
        ui.Navigate("test_screen")
        close(done)
    }()
    
    // Navigate must return (close done) before LoadScreen completes
    select {
    case <-done:
        // PASS: Navigate returned
    case <-time.After(3 * time.Second):
        t.Fatal("Navigate did not return within timeout — still blocking")
    }
    
    // Verify LoadScreen was actually started
    assert.True(t, loadCalled.Load())
    
    // Unblock the mock for cleanup
    close(blockCh)
}
```

**Validation:** `done` channel closes before `blockCh` is signaled → Navigate is non-blocking.

### 7.3 TDD-02: Navigate dispatches UI swap via fyne.Do (REQ-ASYNC-02)

```go
func TestNavigate_DispatchesUISwapViaFyneDo(t *testing.T) {
    // Setup: test app + Navigate closure with instant mock LoadScreen/Compose
    testApp := test.NewApp()
    _ = testApp   // ensures fyne.Do works synchronously
    
    mockUI := widget.NewLabel("composed-screen")
    // ... setup Navigate with mocks returning mockUI
    
    ui.Navigate("test_screen")
    
    // In test env, fyne.Do is synchronous + goroutine may need a brief wait
    // Poll for mainContainer.Objects to contain mockUI
    
    // Assert: mainContainer.Objects[0] == mockUI (or equivalent)
}
```

**Validation:** After `Navigate` returns and the goroutine completes, the container has been updated via `fyne.Do`.

### 7.4 TDD-03: Navigate logs errors (REQ-ASYNC-03)

```go
func TestNavigate_LogsErrorWithoutCrash(t *testing.T) {
    // Setup: Navigate with LoadScreen that returns an error
    originalObjects := mainContainer.Objects
    
    ui.Navigate("bad_screen")
    
    // Wait for goroutine to finish
    // Assert: mainContainer.Objects unchanged
    // Assert: log output contains error message
}
```

**Validation:** Previous UI state preserved, error logged, no panic.

### 7.5 TDD-04/TDD-05: Compositor wrap points (REQ-THREAD-01/02/03)

The existing data_grid tests already exercise all four wrap points (G1 via `loadMasterBuffer`, G2 via `fetchGridDataAsync`, G3 via EventBus filter). After the change:

- Run `go test -race ./pkg/ui/... -count=1` — must pass with zero race warnings.
- Existing tests should pass unchanged (the `fyne.Do` wrapper is transparent in the test environment).
- The key validation is the **race detector**, not behavioral assertions.

### 7.6 TDD-07/TDD-08: Concurrent access and race detector

```go
func TestCompose_DataGrid_ConcurrentOps_NoDeadlock(t *testing.T) {
    // Compose a data_grid with a BusinessPool mock that returns data
    // Trigger loadMasterBuffer (G1) + fetchGridDataAsync (G2) + EventBus filter (G3)
    // Assert all complete within 5 seconds (no deadlock)
}
```

```bash
go test -race ./pkg/ui/... -count=1
# Must exit 0 with zero race warnings
```

---

## 8. Files Changed

| File | Change type | Lines changed (est.) | Description |
|------|------------|---------------------|-------------|
| `pkg/ui/compositor.go` | Modify | ~12 lines | Wrap 6 unsafe calls in `fyne.Do()` at 4 sites. Add `"fyne.io/fyne/v2"` import. |
| `cmd/golemui/main.go` | Modify | ~8 lines | Wrap `LoadScreen+Compose` in `go func()`, wrap UI swap in `fyne.Do()`. |
| `cmd/golemui/main_test.go` | Add | ~80 lines | TDD-01 (non-blocking), TDD-02 (fyne.Do dispatch), TDD-03 (error handling) |
| `pkg/ui/compositor_test.go` | Add | ~40 lines | TDD-07 (concurrent ops no deadlock) |

**Total estimated diff:** ~140 lines of production code + tests. Well within the 400-line review budget.

---

## 9. Risks

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| **Deadlock from incorrect lock ordering** — `model.mu` not unlocked before `fyne.Do` | Low (pattern is explicit in all 4 sites) | High (app hangs) | Code review checklist: verify `Unlock` line precedes `fyne.Do` at each site. TDD-07 exercises concurrent access. |
| **`headers` variable captured by closure after `model.mu.Unlock()`** — could `headers` be stale? | None — `headers` is a local `[]string` constructed inside the goroutine, not read from the model after unlock | N/A | No mitigation needed — `headers` is immutable local data. |
| **Existing tests break** — tests that implicitly relied on synchronous `Navigate` may see stale state | Low (no test calls `Navigate` directly with real DB) | Medium | Audit `TestRunBootstrap_*` and `TestCompose_ButtonNavigation`. Add brief sync points if needed. |
| **`fyne.Do` semantics differ between test and production** — test app runs synchronously, real app queues | Medium | Low | Fyne's `fyne.Do` API is designed for exactly this pattern. The test app's synchronous behavior is a feature, not a limitation — it makes tests deterministic. |
| **Rapid navigation: last-write-wins** — user clicks quickly, sees unexpected screen briefly | Low | Low | Acceptable for first fix. Navigation guard/cancellation is a separate enhancement. |

---

## 10. Ordering Dependencies

```
Step 1: Add "fyne.io/fyne/v2" import to compositor.go
Step 2: Apply Wrap Point 1 (loadMasterBuffer)
Step 3: Apply Wrap Point 2 (filterMasterRows empty snapshot)
Step 4: Apply Wrap Point 3 (filterMasterRows normal exit)
Step 5: Apply Wrap Point 4 (fetchGridDataAsync)
Step 6: Run go test ./pkg/ui/... -race -count=1 → must pass
Step 7: Transform ui.Navigate in main.go
Step 8: Add Navigate tests to main_test.go
Step 9: Run go test ./... -race -count=1 → must pass
Step 10: grep audit — zero unwrapped table.Refresh/table.SetColumnWidth from goroutine context
```

Steps 2–5 can be done in a single commit. Step 7 is a separate logical change. Tests validate each step.
