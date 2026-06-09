# SDD Proposal — fyne-thread-safety-v2

**Change ID:** `fyne-thread-safety-v2`
**Originating spec:** `docs/specify/016-fyne-thread-safety-concurrency.md`
**Predecessor change:** `openspec/changes/fyne-thread-safety` (v1, PR #23 — verified then regressed)
**Date:** 2026-06-09

---

## 1. Problem Statement

GolemUI mutates Fyne widgets from background goroutines without dispatching onto the UI thread. This violates Fyne's threading contract and produces two failure modes in production:

1. **"Error in Fyne call thread"** warnings — emitted when a widget method is called from a non-UI goroutine.
2. **"concurrent map writes"** panics — triggered when two goroutines mutate shared widget state (e.g. font metrics caches, `expiringCache.setAlive`) simultaneously.

A previous change (`fyne-thread-safety`, v1, PR #23) wrapped all such mutations in `fyne.Do()`. That change was **verified correct** at the time (see `openspec/changes/fyne-thread-safety/verify-report.md`). However, two subsequent refactors silently removed every `fyne.Do` wrap:

| Regressing commit | What it did | Wraps removed |
|-------------------|-------------|---------------|
| `abc18d6` — "refactor(ui): remove direct DB access from compositor" | Replaced `fyne.Do` blocks with a `refreshMu sync.Mutex` on `dataGridModel` | 4 sites in `compositor.go` |
| `6f49bbf` — "fix(ui): add screen lifecycle cleanup and EventBus unsubscribe on Navigate" | Removed the `fyne.Do` wrap from the `ui.Navigate` UI swap | 1 site in `main.go` |

Current state: **zero** `fyne.Do` / `fyne.DoAndWait` calls exist in production code. Ten widget mutations execute on background goroutines with no UI-thread dispatch.

---

## 2. Root Cause (Why v1 Was Regressed)

The regressing commits were not attempting to remove thread safety — they were solving different problems (data-access cleanup, screen lifecycle management). The `fyne.Do` wraps were collateral damage:

- The data-access refactor introduced `refreshMu sync.Mutex` to serialize concurrent `table.Refresh()` calls from competing goroutines. This **partially addresses the symptom** (two goroutines calling `Refresh` simultaneously) but **does not address the root cause** (widget mutations must run on the Fyne UI thread per Fyne's contract). A mutex serializes on the calling goroutine; `fyne.Do` dispatches onto the correct thread.
- The lifecycle fix restructured the `ui.Navigate` closure and dropped the `fyne.Do` wrap in the process.

**Systemic cause:** no documented pattern or lint prevented the removal. Future specs (017, 018) will introduce more reactive subscriber handlers that mutate widgets from EventBus goroutines. Without an established, documented convention, the same regression is likely to recur.

---

## 3. Proposed Solution

### 3.1 Re-apply `fyne.Do` at all v1 sites

Wrap all 10 unwrapped UI-mutating calls (inventory in explore §3) in `fyne.Do(func() { ... })`, restoring the v1 threading model:

| Area | Sites | Wrap strategy |
|------|-------|---------------|
| **Navigate** (`main.go`) | `mainContainer.Objects` assignment, `mainContainer.Refresh()`, `navTree.SelectByVistaID(vID)` (lines 134-136) | Single `fyne.Do` block around all three statements |
| **Sidebar** (`sidebar_widget.go`) | `nt.tree.OpenBranch` loop + `nt.tree.Select` (lines 52, 57) | `fyne.DoAndWait` block — see §3.2 |
| **DataGrid — loadMasterBuffer** (`compositor.go`) | `table.SetColumnWidth` loop + `table.Refresh()` (lines 371, 373) | Single `fyne.Do` block; `refreshMu` removed |
| **DataGrid — filterMasterRows empty** (`compositor.go`) | `table.Refresh()` (line 399) | `fyne.Do` block; `refreshMu` removed |
| **DataGrid — filterMasterRows filtered** (`compositor.go`) | `table.Refresh()` (line 437) | `fyne.Do` block; `refreshMu` removed |
| **DataGrid — fetchGridDataAsync** (`compositor.go`) | `table.SetColumnWidth` loop + `table.Refresh()` (lines 519, 521) | Single `fyne.Do` block; `refreshMu` removed |

### 3.2 `SelectByVistaID` dispatch strategy — `fyne.DoAndWait`

`SelectByVistaID` (`sidebar_widget.go`) uses a `navigating` re-entrancy guard (`atomic.Bool`) to prevent the `tree.OnSelected` callback from re-entering `Navigate` during programmatic selection. The guard must be set **before** dispatch and cleared **after** the tree mutation completes.

- `fyne.Do` (async): the callback may execute later on the UI thread. The `defer navigating.Store(false)` runs immediately after `fyne.Do` returns, creating a window where the guard is false before the tree mutation executes — defeating the re-entrancy protection.
- `fyne.DoAndWait` (synchronous): blocks the calling goroutine until the callback completes on the UI thread. The re-entrancy guard semantics are preserved exactly.

**Decision:** use `fyne.DoAndWait` for `SelectByVistaID`. This is the only site requiring synchronous dispatch; all other sites use fire-and-forget `fyne.Do`.

### 3.3 Deadlock ordering — REQ-LOCK-01

At every DataGrid wrap site, `model.mu.Unlock()` **must complete before** the `fyne.Do` block is entered. Rationale: `table.Length`, `CreateCell`, and `UpdateHeader` callbacks acquire `model.mu.RLock()` when Fyne invokes them from the UI thread inside `Refresh()`. If the model write-lock is held when `fyne.Do` dispatches `table.Refresh()` onto the UI thread, the callbacks deadlock waiting for the read-lock.

```
model.mu.Unlock()          // ← MUST complete first
fyne.Do(func() {           // ← dispatches to UI thread
    table.SetColumnWidth(...)
    table.Refresh()         // triggers Length/CreateCell/UpdateHeader → model.mu.RLock()
})
```

This invariant is preserved from v1 (confirmed in v1 verify-report §7).

### 3.4 `refreshMu` removal

**Decision:** remove `refreshMu` entirely. `fyne.Do` alone provides both serialization (Fyne dispatches onto the single UI thread) and correctness (widget mutations run on the correct thread). Retaining two mechanisms for the same purpose adds confusion without safety benefit.

### 3.5 Documented pattern for reactive bindings (017/018)

Add a package-level comment in `pkg/ui/compositor.go` documenting the `fyne.Do` contract for all EventBus subscriber handlers:

> Any `LocalEventBus.Subscribe` handler that mutates a Fyne widget must wrap the mutation in `fyne.Do(func() { ... })`. The EventBus dispatches each handler in a fresh goroutine (`go h(event)`). Fyne requires widget mutations on the UI thread. `fyne.Do` bridges the two.

This gives specs 017 (reactive label binding) and 018 (reactive button state) a mechanical template to follow, preventing future regressions.

---

## 4. Scope

### In Scope

| # | Item | Traceability |
|---|------|-------------|
| 1 | Re-apply `fyne.Do` at the Navigate UI swap (`main.go:134-136`) | Spec 016 TAREA 1 |
| 2 | Add `fyne.DoAndWait` at `SelectByVistaID` (`sidebar_widget.go:52,57`) | Spec 016 TAREA 2 |
| 3 | Re-apply `fyne.Do` at 6 DataGrid sites in `compositor.go` | Spec 016 TAREA 3 |
| 4 | Document the `fyne.Do` subscriber handler pattern for reactive bindings | Spec 016 TAREA 4 |
| 5 | TDD tests asserting `fyne.Do` dispatch at each area | Acceptance criterion 2 |
| 6 | Strengthen `TestNavigate_DispatchesUISwapViaFyneDo` to assert behavioral dispatch | Acceptance criterion 1 |
| 7 | Complete removal of `refreshMu` field and all usages in compositor.go | Spec 016 TAREA 3 |

### Out of Scope

- Adding a loading indicator during navigation.
- Navigation cancellation/guard (rapid-click deduplication).
- Implementing specs 017 or 018 (future changes; v2 prepares the ground).
- Changing the EventBus goroutine-per-handler model (`go h(event)` is intentional).
- Fyne version changes (v2.7.4 already has `fyne.Do`).

---

## 5. Requirements

Traceable to spec 016 TAREAs 1–4 and acceptance criteria.

| ID | Requirement | TAREA | Acceptance Criterion |
|----|------------|-------|---------------------|
| **REQ-NAV-01** | `ui.Navigate` spawns `go func()` for `LoadScreen` + `Compose` (non-blocking) | 1 | AC-1: transitions without "Error in Fyne call thread" |
| **REQ-NAV-02** | `mainContainer.Objects` assignment, `mainContainer.Refresh()`, and `navTree.SelectByVistaID` are wrapped in a single `fyne.Do(func() { ... })` | 1 | AC-1 |
| **REQ-NAV-03** | Navigate errors are logged and the goroutine returns without calling `fyne.Do` | 1 | AC-1 |
| **REQ-SB-01** | `nt.tree.OpenBranch` loop and `nt.tree.Select` are wrapped in `fyne.DoAndWait(func() { ... })` | 2 | AC-1 |
| **REQ-SB-02** | The `navigating` re-entrancy guard is set before `fyne.DoAndWait` and cleared after it returns | 2 | AC-1 |
| **REQ-DG-01** | `loadMasterBuffer` goroutine: `table.SetColumnWidth` + `table.Refresh()` wrapped in `fyne.Do` | 3 | AC-2 |
| **REQ-DG-02** | `fetchGridDataAsync` goroutine: `table.SetColumnWidth` + `table.Refresh()` wrapped in `fyne.Do` | 3 | AC-2 |
| **REQ-DG-03** | `filterMasterRows` empty-snap path: `table.Refresh()` wrapped in `fyne.Do` | 3 | AC-2 |
| **REQ-DG-04** | `filterMasterRows` filtered path: `table.Refresh()` wrapped in `fyne.Do` | 3 | AC-2 |
| **REQ-LOCK-01** | `model.mu.Unlock()` completes before every `fyne.Do` block (deadlock prevention) | 3 | AC-2 |
| **REQ-EB-01** | Package comment in `compositor.go` documents the `fyne.Do` subscriber handler pattern | 4 | Forward-looking for 017/018 |
| **REQ-TEST-01** | TDD tests assert `fyne.Do` dispatch at Navigate, Sidebar, and DataGrid sites | — | AC-2 |
| **REQ-INV-01** | No public API signature changes (`ui.Navigate`, `NodeMeta`, `LoadScreen`, `Compose`, `ComposeWithState`) | — | — |

---

## 6. Affected Files

| File | Change Summary |
|------|---------------|
| `pkg/ui/compositor.go` | Re-apply `fyne.Do` at 6 DataGrid sites; remove `refreshMu` field and all its usages; add package comment documenting the subscriber pattern |
| `pkg/ui/sidebar_widget.go` | Wrap `OpenBranch` loop + `Select` in `fyne.DoAndWait`; preserve re-entrancy guard |
| `cmd/golemui/main.go` | Wrap Navigate UI swap (3 statements) in `fyne.Do` |
| `pkg/ui/compositor_test.go` | Add TDD tests for `fyne.Do` dispatch at DataGrid sites |
| `pkg/ui/sidebar_widget_test.go` | Add test calling `SelectByVistaID` from a background goroutine |
| `cmd/golemui/main_test.go` | Strengthen `TestNavigate_DispatchesUISwapViaFyneDo` to assert behavioral `fyne.Do` dispatch |

**No changes to:** `pkg/eventbus/eventbus.go`, `go.mod`, `go.sum`, or any public API signatures.

---

## 7. Risks and Mitigations

| # | Risk | Likelihood | Impact | Mitigation |
|---|------|-----------|--------|------------|
| 1 | **Re-regression**: future refactors remove `fyne.Do` wraps again (same as v1) | Medium | High | Documented pattern in package comment (REQ-EB-01); grep audit in verify phase; consider a future lint rule |
| 2 | **`fyne.Do` fire-and-forget vs. goroutine lifecycle**: data_grid goroutine exits before `fyne.Do` callback runs on the UI thread — `table` pointer could be stale if the screen is torn down | Low | High | The `cleanupMu` + `prevCleanup` lifecycle in `main.go` handles teardown; the `dataGridModel.wg` WaitGroup in `compositor.go` manages goroutine coordination. Verify that `fyne.Do` callbacks do not outlive the screen they belong to. |
| 3 | **`fyne.DoAndWait` blocks the calling goroutine**: if the UI thread is busy, the Navigate goroutine blocks, delaying cleanup | Low | Medium | Acceptable: `SelectByVistaID` is the last statement in the `fyne.Do` block inside Navigate, so the goroutine has no further work. The block is short (loop + select). |
| 4 | **Fyne test driver limitation**: `go test -race` surfaces Fyne-internal races (not GolemUI bugs) | Certain | Low | Document as known limitation (carried from v1 verify-report). GolemUI-side correctness is verified by structural grep audit, not race-clean test output. |
| 5 | **`refreshMu` removal changes serialization model**: any code that relied on `refreshMu` for correctness beyond thread dispatch will lose that guard | Low | Medium | `fyne.Do` provides the same serialization guarantee (UI thread is single-threaded). The `refreshMu` was never a correct substitute for `fyne.Do` per Fyne's contract. No non-UI code path depends on `refreshMu` for data safety (that role belongs to `model.mu`). |
| 6 | **Re-entrancy guard timing with `fyne.DoAndWait`**: the guard window (`navigating=true`) is longer than v0 because it spans the synchronous wait | Low | Low | Acceptable: the guard exists precisely to suppress re-entrant navigation during programmatic selection. A longer window is more conservative, not less. |
| 7 | **017/018 coordination**: if 018 lands concurrently, its Navigate wrap may conflict with v2's | Low | Medium | v2 is a prerequisite for 018. Coordinate sequencing: v2 first, then 017/018. |

---

## 8. Success Metrics

From spec 016 acceptance criteria:

| # | Metric | Verification Method |
|---|--------|-------------------|
| 1 | Application runs navigation transitions and table filtering without emitting "Error in Fyne call thread" | Manual smoke test + structural grep audit (zero unwrapped goroutine-context widget mutations) |
| 2 | Concurrent unit tests (`compositor_test.go` parallel loads, `sidebar_widget_test.go` background goroutine calls) complete without "concurrent map writes" panics | `go test ./pkg/ui/... ./cmd/golemui/... -count=1 -timeout 30s` passes |
| 3 | All 10 sites from the explore gap inventory (§3) are wrapped | Grep audit in verify phase confirms zero unwrapped mutations |
| 4 | REQ-LOCK-01 holds at all DataGrid sites | Verify `model.mu.Unlock()` precedes every `fyne.Do` block — no `Unlock()` inside any callback |
| 5 | No public API signature changes | `go vet ./...` clean; diff review confirms zero signature changes |

---

## 9. Rollback Plan

v2 is a focused thread-safety fix with no schema, API, or dependency changes. Rollback is straightforward:

1. **Revert the v2 commit** — restores the pre-v2 state (`refreshMu`-only, no `fyne.Do`).
2. **Pre-v2 state is not broken in a new way** — it is the current production state. The known issues (Fyne thread warnings, potential concurrent panics) are the same ones that exist today.
3. **No data migration** — no database or config changes are involved.
4. **No dependency changes** — `go.mod` and `go.sum` are not touched.

If a specific `fyne.Do` wrap introduces a regression (e.g. deadlock at a particular site), the wrap can be individually reverted while keeping the others — each site is independent.
