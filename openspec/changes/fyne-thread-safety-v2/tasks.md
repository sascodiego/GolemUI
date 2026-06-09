# SDD Tasks — fyne-thread-safety-v2

**Change ID:** `fyne-thread-safety-v2`
**Date:** 2026-06-09

---

## 1. Overview

| Phase | File | Est. lines | REQ-IDs | Description |
|-------|------|-----------|---------|-------------|
| 1 | `pkg/ui/compositor.go` | ~20 added, ~14 removed | REQ-DG-01, REQ-DG-02, REQ-DG-03, REQ-DG-04, REQ-DG-05, REQ-LOCK-01, REQ-EB-01 | Remove `refreshMu`; wrap 4 sites in `fyne.Do`; add package comment |
| 2 | `pkg/ui/sidebar_widget.go` | ~10 added, ~6 removed | REQ-SB-01, REQ-SB-02, REQ-SB-03 | Wrap `OpenBranch`+`Select` in `fyne.DoAndWait`; reposition re-entrancy guard |
| 3 | `cmd/golemui/main.go` | ~4 added, ~2 removed | REQ-NAV-01, REQ-NAV-02, REQ-NAV-03 | Wrap Navigate UI swap in `fyne.Do` |
| 4 | Test files (new/updated) | ~300 | REQ-TEST-01 | 11 TDD test cases from spec §5 |

**Total estimated diff:** ~34 lines added, ~22 removed across production files (+12 net). Tests are a separate phase.

**Dependency order:** Phase 1 → Phase 2 → Phase 3 → Phase 4. Phases 1–3 are compile-independent but should be applied in this order to allow incremental verification.

---

## 2. Phase 1 — compositor.go

### T-1.1 — Add package-level doc comment (REQ-EB-01)

- [ ] **T-1.1** Add the thread-safety contract package comment before `package ui` in `pkg/ui/compositor.go`. This documents the `fyne.Do` pattern for all current and future EventBus subscriber handlers.

**REQ-IDs:** REQ-EB-01
**Verification:** `head -12 pkg/ui/compositor.go | grep -c "Thread-safety contract"`

---

### T-1.2 — Remove `refreshMu` field from `dataGridModel` struct (REQ-DG-05)

- [ ] **T-1.2** Remove the `refreshMu sync.Mutex` field (and its comment) from the `dataGridModel` struct definition at line 27 of `pkg/ui/compositor.go`.

**REQ-IDs:** REQ-DG-05
**Verification:** `grep -c "refreshMu" pkg/ui/compositor.go` returns 0

---

### T-1.3 — Replace `refreshMu` block in `loadMasterBuffer` with `fyne.Do` (REQ-DG-01, REQ-LOCK-01)

- [ ] **T-1.3** In `loadMasterBuffer` goroutine (~line 368–374), replace `model.refreshMu.Lock()` / `table.SetColumnWidth` loop / `table.Refresh()` / `model.refreshMu.Unlock()` with a single `fyne.Do(func() { ... })` block. Ensure `model.mu.Unlock()` (line 366) remains before the `fyne.Do` call — never inside the callback.

**REQ-IDs:** REQ-DG-01, REQ-LOCK-01
**Verification:** `grep -A8 "model.masterRows = ds.Rows" pkg/ui/compositor.go | grep "fyne.Do"`

---

### T-1.4 — Replace `refreshMu` block in `filterMasterRows` empty-snap path with `fyne.Do` (REQ-DG-03, REQ-LOCK-01)

- [ ] **T-1.4** In `filterMasterRows`, the `if len(snap) == 0` branch (~line 381–400), replace `model.refreshMu.Lock()` / `table.Refresh()` / `model.refreshMu.Unlock()` with `fyne.Do(func() { table.Refresh() })`. Ensure `model.mu.Unlock()` remains before the `fyne.Do` call.

**REQ-IDs:** REQ-DG-03, REQ-LOCK-01
**Verification:** `grep -B2 "table.Refresh" pkg/ui/compositor.go | grep -c "fyne.Do"` (count should be ≥2 after this task)

---

### T-1.5 — Replace `refreshMu` block in `filterMasterRows` filtered path with `fyne.Do` (REQ-DG-04, REQ-LOCK-01)

- [ ] **T-1.5** In `filterMasterRows`, the filtered-result exit path (~line 434–438), replace `model.refreshMu.Lock()` / `table.Refresh()` / `model.refreshMu.Unlock()` with `fyne.Do(func() { table.Refresh() })`. Ensure `model.mu.Unlock()` (line 434) remains before the `fyne.Do` call.

**REQ-IDs:** REQ-DG-04, REQ-LOCK-01
**Verification:** `grep -n "refreshMu" pkg/ui/compositor.go` returns 0 matches

---

### T-1.6 — Replace `refreshMu` block in `fetchGridDataAsync` with `fyne.Do` (REQ-DG-02, REQ-LOCK-01)

- [ ] **T-1.6** In `fetchGridDataAsync` goroutine (~line 516–522), replace `model.refreshMu.Lock()` / `table.SetColumnWidth` loop / `table.Refresh()` / `model.refreshMu.Unlock()` with a single `fyne.Do(func() { ... })` block. Ensure `model.mu.Unlock()` (line 514) remains before the `fyne.Do` call.

**REQ-IDs:** REQ-DG-02, REQ-LOCK-01
**Verification:** `grep -c "refreshMu" pkg/ui/compositor.go` returns 0 AND `grep -c "fyne.Do" pkg/ui/compositor.go` returns 4

---

### T-1.7 — Phase 1 compile and invariant check

- [ ] **T-1.7** Run compilation and structural invariant checks for Phase 1.

**Verification commands:**
```bash
go build ./...
go vet ./...
grep -rn "refreshMu" --include="*.go" pkg/ui/ cmd/golemui/ | wc -l  # must be 0
grep -c "fyne.Do" pkg/ui/compositor.go  # must be 4
```

---

## 3. Phase 2 — sidebar_widget.go

### T-2.1 — Restructure `SelectByVistaID` to use `fyne.DoAndWait` (REQ-SB-01, REQ-SB-02, REQ-SB-03)

- [ ] **T-2.1** In `pkg/ui/sidebar_widget.go`, restructure the `SelectByVistaID` function body:
  1. Keep early-return guards (`vistaID == ""`, unknown `vistaID`) unchanged — outside any dispatch (REQ-SB-03).
  2. Move `nt.navigating.Store(true)` to **before** the `fyne.DoAndWait` call. Keep `defer func() { nt.navigating.Store(false) }()` immediately after (REQ-SB-02).
  3. Move the ancestor computation (`for cur := nodeID` loop) before `fyne.DoAndWait` so it runs on the calling goroutine.
  4. Wrap the `OpenBranch` loop and `nt.tree.Select(widget.TreeNodeID(nodeID))` inside `fyne.DoAndWait(func() { ... })` (REQ-SB-01).

**REQ-IDs:** REQ-SB-01, REQ-SB-02, REQ-SB-03
**Verification:**
```bash
grep -c "fyne.DoAndWait" pkg/ui/sidebar_widget.go  # must be 1
grep -B3 "fyne.DoAndWait" pkg/ui/sidebar_widget.go | grep "navigating.Store(true)"  # guard before dispatch
```

---

### T-2.2 — Phase 2 compile and invariant check

- [ ] **T-2.2** Run compilation and structural invariant checks for Phase 2.

**Verification commands:**
```bash
go build ./...
grep -c "fyne.DoAndWait" pkg/ui/sidebar_widget.go  # must be 1
grep -A2 "navigating.Store(true)" pkg/ui/sidebar_widget.go | grep "defer"  # defer follows guard
```

---

## 4. Phase 3 — main.go

### T-3.1 — Wrap Navigate UI swap in `fyne.Do` (REQ-NAV-02, REQ-NAV-03)

- [ ] **T-3.1** In `cmd/golemui/main.go`, inside the `ui.Navigate` goroutine, wrap the three UI-mutating statements (`mainContainer.Objects = [...]`, `mainContainer.Refresh()`, `navTree.SelectByVistaID(vID)`) in a single `fyne.Do(func() { ... })` block. The `cleanupMu.Lock/Unlock` blocks and error-early-return paths remain outside `fyne.Do` (REQ-NAV-03). The `go func()` async kickoff is unchanged (REQ-NAV-01).

**REQ-IDs:** REQ-NAV-01, REQ-NAV-02, REQ-NAV-03
**Verification:**
```bash
grep -c "fyne.Do" cmd/golemui/main.go  # must be 1
grep -A5 "fyne.Do(func()" cmd/golemui/main.go | grep "mainContainer.Objects"
grep -A6 "fyne.Do(func()" cmd/golemui/main.go | grep "navTree.SelectByVistaID"
```

---

### T-3.2 — Phase 3 compile and full invariant check

- [ ] **T-3.2** Run full compilation and all structural invariant checks across all 3 changed files.

**Verification commands:**
```bash
go build ./...
go vet ./...
grep -rn "refreshMu" --include="*.go" pkg/ui/ cmd/golemui/ | wc -l  # must be 0
grep -rn "fyne\.Do\|fyne\.DoAndWait" --include="*.go" pkg/ui/compositor.go pkg/ui/sidebar_widget.go cmd/golemui/main.go | wc -l  # must be 6
```

---

## 5. Phase 4 — TDD Tests

All test files go in `pkg/ui/` or `cmd/golemui/` as appropriate. Tests validate that the `fyne.Do`/`fyne.DoAndWait` wraps are structurally present and behave correctly. Each test maps to one spec §5 TDD scenario.

---

### T-4.1 — TestNavigate_DispatchesUISwapViaFyneDo (spec §5.1)

- [ ] **T-4.1** Strengthen the existing Navigate test. Inject a spy container that records whether UI mutations execute inside a `fyne.Do` context. Run `RunBootstrap` with a test app, call `ui.Navigate("home")`, poll until the spy records the mutation, then assert that `fyne.Do` was invoked between goroutine start and mutation execution.

**REQ-IDs:** REQ-TEST-01, REQ-NAV-02
**Verification:** `go test -run TestNavigate_DispatchesUISwapViaFyneDo ./...`

---

### T-4.2 — TestNavigate_ErrorPath_NoFyneDo (spec §5.1)

- [ ] **T-4.2** Override `ui.LoadScreen` (or inject a failing screen) to return an error. Call `ui.Navigate("bad-screen")`. Assert that `fyne.Do` is never called — only the error log fires and the goroutine returns early.

**REQ-IDs:** REQ-TEST-01, REQ-NAV-03
**Verification:** `go test -run TestNavigate_ErrorPath_NoFyneDo ./...`

---

### T-4.3 — TestSelectByVistaID_DispatchesViaFyneDoAndWait (spec §5.2)

- [ ] **T-4.3** Build a `NavTree` with a known hierarchy. Record `fyne.DoAndWait` invocations via a test spy or global counter. Call `SelectByVistaID` from a background goroutine. Assert that `fyne.DoAndWait` is called exactly once. After return, assert `nt.navigating.Load()` is `false`.

**REQ-IDs:** REQ-TEST-01, REQ-SB-01, REQ-SB-02
**Verification:** `go test -run TestSelectByVistaID_DispatchesViaFyneDoAndWait ./...`

---

### T-4.4 — TestSelectByVistaID_ReentrancyGuardHoldsAcrossDoAndWait (spec §5.2)

- [ ] **T-4.4** Call `SelectByVistaID` from a background goroutine. Inside a hooked `tree.OnSelected`, assert that `navigating.Load()` is `true` during the `fyne.DoAndWait` callback execution. The re-entrancy guard must be held for the full dispatch duration.

**REQ-IDs:** REQ-TEST-01, REQ-SB-02
**Verification:** `go test -run TestSelectByVistaID_ReentrancyGuardHoldsAcrossDoAndWait ./...`

---

### T-4.5 — TestSelectByVistaID_EmptyAndUnknown_NoFyneDo (spec §5.2)

- [ ] **T-4.5** Call `SelectByVistaID("")` and with an unknown vistaID. Assert that `fyne.DoAndWait` is never invoked — zero dispatch. Both early-return paths bypass the dispatch block entirely.

**REQ-IDs:** REQ-TEST-01, REQ-SB-03
**Verification:** `go test -run TestSelectByVistaID_EmptyAndUnknown_NoFyneDo ./...`

---

### T-4.6 — TestLoadMasterBuffer_WrapsInFyneDo (spec §5.3)

- [ ] **T-4.6** Provide a `DataSource` that returns test data. Trigger `loadMasterBuffer` via `Compose` for a `data_grid` node with `MasterDataSource`. Assert that `table.SetColumnWidth` and `table.Refresh` execute inside a `fyne.Do` callback. Verify `model.mu` is not held during the `fyne.Do` callback (via `TryLock` inside the callback).

**REQ-IDs:** REQ-TEST-01, REQ-DG-01, REQ-LOCK-01
**Verification:** `go test -run TestLoadMasterBuffer_WrapsInFyneDo ./...`

---

### T-4.7 — TestFetchGridDataAsync_WrapsInFyneDo (spec §5.4)

- [ ] **T-4.7** Trigger a server-mode data_grid with a query via the `fetchGridDataAsync` path. Assert that `table.SetColumnWidth` and `table.Refresh` execute inside a `fyne.Do` callback. Same structural assertions as T-4.6 but via the `fetchGridDataAsync` code path.

**REQ-IDs:** REQ-TEST-01, REQ-DG-02, REQ-LOCK-01
**Verification:** `go test -run TestFetchGridDataAsync_WrapsInFyneDo ./...`

---

### T-4.8 — TestFilterMasterRows_EmptySnap_WrapsInFyneDo (spec §5.5)

- [ ] **T-4.8** Populate a `dataGridModel` with master data. Call `filterMasterRows` with an empty snapshot. Assert `table.Refresh` is wrapped in `fyne.Do`. Verify `model.mu` is unlocked before `fyne.Do` via `TryLock` inside the callback.

**REQ-IDs:** REQ-TEST-01, REQ-DG-03, REQ-LOCK-01
**Verification:** `go test -run TestFilterMasterRows_EmptySnap_WrapsInFyneDo ./...`

---

### T-4.9 — TestFilterMasterRows_Filtered_WrapsInFyneDo (spec §5.5)

- [ ] **T-4.9** Populate a `dataGridModel` with master data. Call `filterMasterRows` with a matching snapshot. Assert `table.Refresh` is wrapped in `fyne.Do`. Verify filtered rows are correct and `model.mu` is unlocked before `fyne.Do`.

**REQ-IDs:** REQ-TEST-01, REQ-DG-04, REQ-LOCK-01
**Verification:** `go test -run TestFilterMasterRows_Filtered_WrapsInFyneDo ./...`

---

### T-4.10 — TestDataGrid_ModelMuUnlockedBeforeFyneDo (spec §5.6)

- [ ] **T-4.10** At each DataGrid site (loadMasterBuffer, fetchGridDataAsync, filterMasterRows empty, filterMasterRows filtered), instrument `model.mu.TryLock` inside the `fyne.Do` callback. If `TryLock` succeeds, the write-lock was not held — pass. If it fails, the write-lock is still held — fail (deadlock risk). This validates REQ-LOCK-01 structurally at all 4 sites.

**REQ-IDs:** REQ-TEST-01, REQ-LOCK-01
**Verification:** `go test -run TestDataGrid_ModelMuUnlockedBeforeFyneDo ./...`

---

### T-4.11 — TestDataGrid_NoRefreshMuInModel (spec §5.7)

- [ ] **T-4.11** Assert that the `dataGridModel` struct has no field named `refreshMu` using reflection or a structural check. This validates REQ-DG-05 (complete removal of `refreshMu`).

**REQ-IDs:** REQ-TEST-01, REQ-DG-05
**Verification:** `go test -run TestDataGrid_NoRefreshMuInModel ./...`

---

### T-4.12 — Phase 4 full test suite run

- [ ] **T-4.12** Run the complete test suite to confirm all 11 tests pass and no regressions are introduced.

**Verification commands:**
```bash
go test ./... -count=1
go vet ./...
```

---

## 6. Final Verification

After all phases are complete, run the following invariant checklist:

```bash
# INV-01: No refreshMu references remain
grep -rn "refreshMu" --include="*.go" pkg/ui/ cmd/golemui/

# INV-02: Exactly 6 fyne.Do/fyne.DoAndWait calls across the 3 files
grep -rn "fyne\.Do\|fyne\.DoAndWait" --include="*.go" pkg/ui/compositor.go pkg/ui/sidebar_widget.go cmd/golemui/main.go

# INV-03: No model.mu.Unlock() inside any fyne.Do callback body
grep -A10 "fyne.Do\b" pkg/ui/compositor.go | grep "model.mu.Unlock"

# INV-04: navigating.Store(true) appears before fyne.DoAndWait in sidebar
grep -B3 "fyne.DoAndWait" pkg/ui/sidebar_widget.go | grep "navigating.Store(true)"

# INV-05: No public API signature changes
go vet ./...

# INV-06: All 11 TDD tests pass
go test -run "Navigate_Dispatch\|Navigate_Error\|SelectByVistaID\|LoadMasterBuffer\|FetchGridDataAsync\|FilterMasterRows\|ModelMuUnlocked\|NoRefreshMu" ./... -count=1

# INV-07: Full suite passes
go test ./... -count=1
```

### Invariant checklist (manual sign-off)

- [ ] INV-01: `refreshMu` does not exist in any source file
- [ ] INV-02: Exactly 6 `fyne.Do`/`fyne.DoAndWait` calls (4 compositor + 1 sidebar + 1 main)
- [ ] INV-03: `model.mu.Unlock()` never inside a `fyne.Do` callback
- [ ] INV-04: `navigating.Store(true)` before `fyne.DoAndWait` in `SelectByVistaID`
- [ ] INV-05: `go vet ./...` is clean — no signature changes
- [ ] INV-06: EventBus `go h(event)` dispatch model is unchanged
- [ ] INV-07: All 11 TDD tests pass
