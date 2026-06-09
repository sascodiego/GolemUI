# SDD Spec — fyne-thread-safety-v2

**Change ID:** `fyne-thread-safety-v2`
**Originating spec:** `docs/specify/016-fyne-thread-safety-concurrency.md`
**Predecessor:** `openspec/changes/fyne-thread-safety` (v1 — PR #23, verified then regressed)
**Date:** 2026-06-09

---

## 1. Requirements (traceable REQ-IDs)

### Navigate — main.go (TAREA 1)

| ID | Requirement |
|----|------------|
| **REQ-NAV-01** | The `ui.Navigate` closure retains the `go func()` async dispatch for `LoadScreen` + `Compose`. No change to the async kickoff. |
| **REQ-NAV-02** | The three UI-mutating statements at `main.go` (lines 134–136: `mainContainer.Objects` assignment, `mainContainer.Refresh()`, `navTree.SelectByVistaID(vID)`) are wrapped in a single `fyne.Do(func() { ... })` block. |
| **REQ-NAV-03** | Navigation errors (from `LoadScreen` or `Compose`) are logged and the goroutine returns early — no `fyne.Do` block is entered on error. |

### Sidebar — sidebar_widget.go (TAREA 2)

| ID | Requirement |
|----|------------|
| **REQ-SB-01** | Inside `SelectByVistaID`, the `nt.tree.OpenBranch` loop (ancestors, root→parent) and `nt.tree.Select` are wrapped in a single `fyne.DoAndWait(func() { ... })` block. |
| **REQ-SB-02** | The `navigating` re-entrancy guard (`atomic.Bool`) is set to `true` **before** calling `fyne.DoAndWait` and reset to `false` **after** `fyne.DoAndWait` returns (via `defer`). This preserves the v0 ordering: the guard is held for the entire duration of the UI-thread dispatch. |
| **REQ-SB-03** | Early-return guards (empty `vistaID`, unknown `vistaID`) remain outside any `fyne.DoAndWait` — no dispatch when there is nothing to mutate. |

### DataGrid — compositor.go (TAREA 3)

| ID | Requirement |
|----|------------|
| **REQ-DG-01** | `loadMasterBuffer` goroutine: the `table.SetColumnWidth` loop and `table.Refresh()` are wrapped in a single `fyne.Do(func() { ... })` block. The `model.refreshMu.Lock/Unlock` around this block is **removed**. |
| **REQ-DG-02** | `fetchGridDataAsync` goroutine: the `table.SetColumnWidth` loop and `table.Refresh()` are wrapped in a single `fyne.Do(func() { ... })` block. The `model.refreshMu.Lock/Unlock` around this block is **removed**. |
| **REQ-DG-03** | `filterMasterRows` empty-snap early return: `table.Refresh()` is wrapped in `fyne.Do(func() { ... })`. The `model.refreshMu.Lock/Unlock` around this block is **removed**. |
| **REQ-DG-04** | `filterMasterRows` filtered path: `table.Refresh()` is wrapped in `fyne.Do(func() { ... })`. The `model.refreshMu.Lock/Unlock` around this block is **removed**. |
| **REQ-DG-05** | The `refreshMu sync.Mutex` field is **removed** from the `dataGridModel` struct definition and all references to it are eliminated. `fyne.Do` is the single serialization mechanism for widget mutations. |

### Lock ordering invariant (TAREA 3)

| ID | Requirement |
|----|------------|
| **REQ-LOCK-01** | At every DataGrid wrap site, `model.mu.Unlock()` completes **before** the `fyne.Do(...)` call is entered. This prevents deadlock: `table.Length`, `CreateCell`, and `UpdateHeader` callbacks acquire `model.mu.RLock()` when Fyne invokes them during `table.Refresh()` on the UI thread. If the write-lock is held during `fyne.Do`, those callbacks deadlock. |

### EventBus subscriber pattern (TAREA 4)

| ID | Requirement |
|----|------------|
| **REQ-EB-01** | A package-level comment is added to `pkg/ui/compositor.go` documenting the thread-safety contract: any `LocalEventBus.Subscribe` handler that mutates a Fyne widget must wrap the mutation in `fyne.Do(func() { ... })`. This serves as the canonical pattern for specs 017 (reactive label binding) and 018 (reactive button state). |

### Testing

| ID | Requirement |
|----|------------|
| **REQ-TEST-01** | TDD tests assert that `fyne.Do` / `fyne.DoAndWait` dispatch occurs at Navigate, Sidebar, and all DataGrid sites. Each area has at least one dedicated test case. |
| **REQ-TEST-02** | No public API signatures change (`ui.Navigate`, `NodeMeta`, `LoadScreen`, `Compose`, `ComposeWithState`). |

---

## 2. Assumptions

1. **Fyne v2.7.4 is the target runtime.** `fyne.Do` and `fyne.DoAndWait` are available (introduced in v2.6.0). No `go.mod` changes required.
2. **The EventBus goroutine-per-handler model (`go h(event)`) is correct and unchanged.** Thread safety is the subscriber's responsibility, not the bus's.
3. **The Fyne test driver runs `fyne.Do` callbacks inline on the calling goroutine** (not on a serialized UI thread). Tests can verify structural correctness but cannot fully reproduce production thread serialization. Grep audit is the primary verification mechanism.
4. **`go test -race` will surface Fyne-internal races** (e.g. `expiringCache.setAlive`, font metrics cache). These are not GolemUI bugs and are accepted as a known limitation.
5. **v2 lands before specs 017 and 018.** The documented pattern (REQ-EB-01) is a prerequisite for reactive binding implementations.
6. **The `cleanupMu` + `prevCleanup` lifecycle in `main.go` is orthogonal** to this change. It manages screen teardown; `fyne.Do` manages thread dispatch. No interaction expected.

---

## 3. Data Flow

### 3.1 Navigate (main.go)

```
User clicks sidebar item
  → tree.OnSelected callback (UI thread)
    → ui.Navigate(vistaID) (UI thread, returns immediately)
      → go func() {
            cleanupMu: run prevCleanup
            LoadScreen(...)     // goroutine: DB query
            Compose(...)        // goroutine: widget tree construction
            cleanupMu: store new cleanup
            model.mu.Unlock()   // (if applicable inside Compose)
            fyne.Do(func() {                   // dispatch to UI thread
                mainContainer.Objects = [...]  // UI thread
                mainContainer.Refresh()        // UI thread
                navTree.SelectByVistaID(vID)   // UI thread → see §3.2
            })
        }()
```

Key property: `LoadScreen` and `Compose` block the goroutine, not the UI. Only the final 3 UI mutations cross `fyne.Do` onto the UI thread.

### 3.2 Sidebar — SelectByVistaID (sidebar_widget.go)

```
SelectByVistaID(vistaID)           // called from Navigate's fyne.Do block (UI thread)
  → early-return guards            // no dispatch needed
  → navigating.Store(true)
  → defer navigating.Store(false)
  → fyne.DoAndWait(func() {        // re-dispatch onto UI thread (synchronous wait)
        for ... { nt.tree.OpenBranch(...) }
        nt.tree.Select(...)
    })
  → navigating.Store(false) via defer // after DoAndWait returns
```

Note: `SelectByVistaID` is called from within a `fyne.Do` block in Navigate. When already on the UI thread, `fyne.DoAndWait` executes the callback inline and returns immediately — no re-dispatch overhead. The `fyne.DoAndWait` ensures correctness when `SelectByVistaID` is called from *any* goroutine context.

### 3.3 DataGrid (compositor.go)

#### loadMasterBuffer

```
go func() {
    ds, err := dsource.FetchAll(...)   // goroutine: DB query
    model.mu.Lock()
    // update model.masterHeaders, masterRows, headers, rows
    model.mu.Unlock()                  // REQ-LOCK-01: unlock BEFORE fyne.Do
    fyne.Do(func() {                   // dispatch to UI thread
        for i, h := range ds.Headers {
            table.SetColumnWidth(i, w)
        }
        table.Refresh()                // triggers Length/CreateCell/UpdateHeader → model.mu.RLock()
    })
}()
```

#### fetchGridDataAsync

Same pattern as `loadMasterBuffer`. The `model.refreshMu` Lock/Unlock is removed and replaced by `fyne.Do`.

#### filterMasterRows

```
func filterMasterRows(...) {
    model.mu.Lock()
    // filter or restore rows
    model.rows = ...
    model.mu.Unlock()                  // REQ-LOCK-01: unlock BEFORE fyne.Do
    fyne.Do(func() {                   // dispatch to UI thread
        table.Refresh()
    })
}
```

Both call sites within `filterMasterRows` (empty-snap early return and filtered exit) follow this pattern.

---

## 4. Behavioral Contracts

### BC-01: Navigate non-blocking

`ui.Navigate(vistaID)` returns to the caller (the Fyne `tree.OnSelected` callback) within microseconds. The `go func()` goroutine owns all blocking work. The caller never blocks on DB queries, widget composition, or `fyne.Do`.

### BC-02: Navigate UI swap is atomic

The three UI-mutating statements in the Navigate goroutine (`Objects` assignment, `Refresh`, `SelectByVistaID`) execute inside a single `fyne.Do` callback. The Fyne UI thread processes them as a contiguous block — no interleaving with other UI events.

### BC-03: Sidebar re-entrancy guard holds across dispatch

`SelectByVistaID` sets `navigating = true` before `fyne.DoAndWait` and clears it after. Any `tree.OnSelected` callback triggered during the dispatch window is suppressed. The guard is not defeated by the async dispatch boundary.

### BC-04: DataGrid no concurrent widget mutations

`fyne.Do` serializes all `table.SetColumnWidth` and `table.Refresh` calls onto the single Fyne UI thread. Two competing goroutines (e.g. `loadMasterBuffer` completion racing with a `filterMasterRows` triggered by a submit event) cannot mutate the table concurrently.

### BC-05: No deadlock on model.mu

At every DataGrid site, `model.mu` is unlocked before `fyne.Do` is entered. The UI thread's `table.Length`/`CreateCell`/`UpdateHeader` callbacks acquire `model.mu.RLock` during `Refresh`. Since the write-lock is never held across the `fyne.Do` boundary, read-lock acquisition never blocks.

### BC-06: Single serialization mechanism

`refreshMu` is removed. `fyne.Do` is the only mechanism for ensuring thread-safe widget mutations. No redundant mutexes guard UI operations.

---

## 5. TDD Scenarios (RED-GREEN test cases)

### 5.1 Navigate

**Test: `TestNavigate_DispatchesUISwapViaFyneDo`** (strengthen existing)

- **RED:** The test injects a spy into the `ui.Navigate` closure that records whether the UI mutations execute inside a `fyne.Do` context. Currently passes without asserting `fyne.Do` — must fail when the wrap is absent.
- **Setup:** Replace `mainContainer` with a spy container. Run `RunBootstrap` with a test app. Call `ui.Navigate("home")`. Poll until the spy records the mutation.
- **Assert:** The mutation callback ran. The spy records that `fyne.Do` (or `fyne.DoAndWait`) was invoked between the goroutine start and the mutation.
- **GREEN:** After REQ-NAV-02 is implemented, the `fyne.Do` wrap is present and the spy assertion passes.

**Test: `TestNavigate_ErrorPath_NoFyneDo`**

- **RED:** Override `ui.LoadScreen` to return an error. Call `ui.Navigate("bad-screen")`. Assert that no `fyne.Do` dispatch occurs.
- **Assert:** `fyne.Do` is never called. Only the error log fires.
- **GREEN:** REQ-NAV-03 ensures the goroutine returns before reaching the `fyne.Do` block.

### 5.2 Sidebar

**Test: `TestSelectByVistaID_DispatchesViaFyneDoAndWait`**

- **RED:** Call `SelectByVistaID` from a background goroutine (not the UI thread). Assert that `fyne.DoAndWait` was invoked and the tree mutations completed.
- **Setup:** Build a `NavTree` with a known hierarchy. Record `fyne.DoAndWait` invocations via a test spy or global counter.
- **Assert:** `fyne.DoAndWait` is called exactly once. After return, `nt.navigating.Load()` is `false`.

**Test: `TestSelectByVistaID_ReentrancyGuardHoldsAcrossDoAndWait`**

- **RED:** Call `SelectByVistaID` from a background goroutine. Inside a hooked `tree.OnSelected`, assert that `navigating.Load()` is `true`.
- **Assert:** The re-entrancy guard is `true` during the `fyne.DoAndWait` callback execution.
- **GREEN:** REQ-SB-02 ensures the guard is set before `fyne.DoAndWait` and cleared after.

**Test: `TestSelectByVistaID_EmptyAndUnknown_NoFyneDo`**

- **Assert:** Calling with `""` or an unknown `vistaID` does not invoke `fyne.DoAndWait`. Zero dispatch.

### 5.3 DataGrid — loadMasterBuffer

**Test: `TestLoadMasterBuffer_WrapsInFyneDo`**

- **RED:** Provide a `DataSource` that returns test data. Trigger `loadMasterBuffer` from `Compose` for a `data_grid` node with `MasterDataSource`. Assert that `table.SetColumnWidth` and `table.Refresh` execute inside a `fyne.Do` callback.
- **Assert:** `fyne.Do` is called. The table receives column widths and a refresh after the model is populated. `model.mu` is not held during the `fyne.Do` callback.

### 5.4 DataGrid — fetchGridDataAsync

**Test: `TestFetchGridDataAsync_WrapsInFyneDo`**

- **RED:** Trigger a server-mode data_grid with a query. Assert that `table.SetColumnWidth` and `table.Refresh` execute inside a `fyne.Do` callback.
- **Assert:** Same structure as `TestLoadMasterBuffer_WrapsInFyneDo` but via the `fetchGridDataAsync` path.

### 5.5 DataGrid — filterMasterRows

**Test: `TestFilterMasterRows_EmptySnap_WrapsInFyneDo`**

- **RED:** Populate a model with master data. Call `filterMasterRows` with an empty snapshot. Assert `table.Refresh` is inside `fyne.Do`.
- **Assert:** `fyne.Do` called. `model.mu` unlocked before `fyne.Do`.

**Test: `TestFilterMasterRows_Filtered_WrapsInFyneDo`**

- **RED:** Populate a model with master data. Call `filterMasterRows` with a matching snapshot. Assert `table.Refresh` is inside `fyne.Do`.
- **Assert:** Same as above. Filtered rows are correct. `model.mu` unlocked before `fyne.Do`.

### 5.6 Lock ordering

**Test: `TestDataGrid_ModelMuUnlockedBeforeFyneDo`**

- **RED:** At each DataGrid site, instrument `model.mu.TryLock` inside the `fyne.Do` callback. If `TryLock` succeeds, the write-lock was not held — pass. If it fails, the write-lock is still held — fail (deadlock risk).
- **Assert:** `TryLock` inside every `fyne.Do` callback succeeds, confirming `model.mu` is unlocked before dispatch.

### 5.7 refreshMu removal

**Test: `TestDataGrid_NoRefreshMuInModel`**

- **RED:** Assert that the `dataGridModel` struct has no field named `refreshMu` (using reflection or structural check).
- **GREEN:** After REQ-DG-05 is implemented, the field is gone.

---

## 6. Invariants

1. **INV-01: Every widget mutation from a background goroutine crosses `fyne.Do`.** Grep audit: `grep -rn "table\.\|mainContainer\.\|nt\.tree\." --include="*.go" pkg/ui/ cmd/golemui/` must find zero unwrapped mutation calls inside goroutines or EventBus subscriber handlers.

2. **INV-02: `model.mu` is never held across a `fyne.Do` boundary.** Static assertion: no `model.mu.Unlock()` appears inside any `fyne.Do` callback body. The unlock must precede the `fyne.Do` call.

3. **INV-03: `refreshMu` does not exist.** The `dataGridModel` struct contains no `refreshMu sync.Mutex` field.

4. **INV-04: `SelectByVistaID` uses `fyne.DoAndWait`, not `fyne.Do`.** Synchronous dispatch preserves the re-entrancy guard semantics.

5. **INV-05: No public API signature changes.** `go vet ./...` is clean. Function signatures for `Navigate`, `Compose`, `ComposeWithState`, `LoadScreen`, `SelectByVistaID`, `BuildNavTree` are unchanged.

6. **INV-06: EventBus (`go h(event)`) is unchanged.** The `InMemEventBus.Publish` method retains its goroutine-per-handler dispatch model.

---

## 7. Out of Scope

- **Loading indicator during navigation** — no visual feedback during screen loads.
- **Navigation cancellation or guard** — rapid-click deduplication is not addressed.
- **Specs 017 and 018 implementation** — reactive label binding and reactive button state are future changes; this spec only establishes the `fyne.Do` pattern they must follow.
- **Fyne version upgrade** — v2.7.4 is current and sufficient.
- **Lint rule for `fyne.Do`** — a static analysis tool to prevent future regressions is deferred to a follow-up.
- **Race-clean test output** — Fyne test driver internal races (`expiringCache`, font metrics) are accepted as a known limitation.
