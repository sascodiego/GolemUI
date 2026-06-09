# SDD Verify Report — fyne-thread-safety-v2

**Change ID:** `fyne-thread-safety-v2`
**Reviewer:** Fresh-context adversarial review
**Date:** 2026-06-09
**Verdict:** **PASS**

---

## 1. Executive Summary (verdict)

**PASS** — All 13 requirements are correctly implemented. All 7 structural invariants hold. Build, vet, and full test suite pass cleanly. No critical or major issues found. Two minor observations documented below.

---

## 2. Structural Audit Results (grep outputs)

### INV-01: No `refreshMu` references in production code

```
$ grep -rn "refreshMu" --include="*.go" pkg/ui/ cmd/golemui/
```

**Result:** 4 matches, all in `pkg/ui/compositor_test.go` (test code referencing `refreshMu` only in error messages and comments for the structural check test). Zero matches in production source files. **PASS.**

### INV-02: Exactly 6 `fyne.Do`/`fyne.DoAndWait` dispatch calls

```
pkg/ui/compositor.go:376:		fyne.Do(func() {
pkg/ui/compositor.go:406:		fyne.Do(func() {
pkg/ui/compositor.go:444:	fyne.Do(func() {
pkg/ui/compositor.go:524:		fyne.Do(func() {
pkg/ui/sidebar_widget.go:59:	fyne.DoAndWait(func() {
cmd/golemui/main.go:135:			fyne.Do(func() {
```

**Result:** Exactly 6 dispatch calls. 4 `fyne.Do` in compositor, 1 `fyne.DoAndWait` in sidebar, 1 `fyne.Do` in main. **PASS.**

### INV-03: No `model.mu.Unlock()` inside any `fyne.Do` callback

Manual verification of all 4 compositor sites:

| Site | Line (Unlock) | Line (fyne.Do) | Unlock before Do? |
|------|---------------|----------------|-------------------|
| loadMasterBuffer | 374 | 376 | ✅ Yes |
| filterMasterRows empty-snap | 405 | 406 | ✅ Yes |
| filterMasterRows filtered | 442 | 444 | ✅ Yes |
| fetchGridDataAsync | 522 | 524 | ✅ Yes |

No `model.mu.Unlock()` found inside any `fyne.Do` callback body. Source-level structural test (`TestDataGrid_ModelMuUnlockedBeforeFyneDo`) also verifies this programmatically. **PASS.**

### INV-04: `navigating` guard before `fyne.DoAndWait`

```
pkg/ui/sidebar_widget.go:52:	nt.navigating.Store(true)
pkg/ui/sidebar_widget.go:53:	defer func() { nt.navigating.Store(false) }()
pkg/ui/sidebar_widget.go:59:	fyne.DoAndWait(func() {
```

`navigating.Store(true)` at line 52, `defer` at line 53, `fyne.DoAndWait` at line 59. Guard is set before dispatch and cleared after via defer. **PASS.**

---

## 3. Per-Requirement Status

| REQ-ID | Description | Status | Evidence |
|--------|-------------|--------|----------|
| **REQ-NAV-01** | Navigate retains `go func()` async dispatch | ✅ PASS | `cmd/golemui/main.go:133-140`: `go func() { ... }()` unchanged |
| **REQ-NAV-02** | 3 UI-mutating statements in single `fyne.Do` | ✅ PASS | `cmd/golemui/main.go:135-139`: `mainContainer.Objects`, `mainContainer.Refresh()`, `navTree.SelectByVistaID` all inside one `fyne.Do(func() { ... })` |
| **REQ-NAV-03** | Error paths return before `fyne.Do` | ✅ PASS | `cmd/golemui/main.go:121-128`: two error-return paths (`LoadScreen` and `Compose` errors) return before reaching `fyne.Do` at line 135 |
| **REQ-SB-01** | `OpenBranch` loop + `Select` in single `fyne.DoAndWait` | ✅ PASS | `pkg/ui/sidebar_widget.go:59-64`: both `OpenBranch` loop and `tree.Select` inside `fyne.DoAndWait(func() { ... })` |
| **REQ-SB-02** | `navigating` guard set before `fyne.DoAndWait`, cleared after | ✅ PASS | `pkg/ui/sidebar_widget.go:52-53`: `navigating.Store(true)` then `defer navigating.Store(false)`, followed by `fyne.DoAndWait` at line 59 |
| **REQ-SB-03** | Early-return guards outside any dispatch | ✅ PASS | `pkg/ui/sidebar_widget.go:34-39`: `vistaID == ""` and unknown `vistaID` returns happen before `navigating.Store` and `fyne.DoAndWait` |
| **REQ-DG-01** | `loadMasterBuffer`: `fyne.Do` wrap, `refreshMu` removed | ✅ PASS | `pkg/ui/compositor.go:376-382`: `fyne.Do(func() { SetColumnWidth loop; table.Refresh() })` |
| **REQ-DG-02** | `fetchGridDataAsync`: `fyne.Do` wrap, `refreshMu` removed | ✅ PASS | `pkg/ui/compositor.go:524-530`: `fyne.Do(func() { SetColumnWidth loop; table.Refresh() })` |
| **REQ-DG-03** | `filterMasterRows` empty-snap: `fyne.Do` wrap, `refreshMu` removed | ✅ PASS | `pkg/ui/compositor.go:406-408`: `fyne.Do(func() { table.Refresh() })` |
| **REQ-DG-04** | `filterMasterRows` filtered path: `fyne.Do` wrap, `refreshMu` removed | ✅ PASS | `pkg/ui/compositor.go:444-446`: `fyne.Do(func() { table.Refresh() })` |
| **REQ-DG-05** | `refreshMu` field removed from struct | ✅ PASS | `pkg/ui/compositor.go:27-38`: struct has no `refreshMu` field. `grep refreshMu compositor.go` returns 0 matches in production code |
| **REQ-LOCK-01** | `model.mu.Unlock()` before `fyne.Do` at all sites | ✅ PASS | Verified at all 4 sites: lines 374→376, 405→406, 442→444, 522→524 |
| **REQ-EB-01** | Package comment documenting thread-safety contract | ✅ PASS | `pkg/ui/compositor.go:1-9`: comprehensive package comment describing `fyne.Do` pattern for EventBus subscribers |

---

## 4. Deadlock Audit

Verified all 4 `fyne.Do` call sites in `compositor.go`:

### Site 1: `loadMasterBuffer` (line 376)

```go
// line 373-374
model.mu.Unlock()        // ← Unlock BEFORE fyne.Do

// line 376-382
fyne.Do(func() {         // ← No model.mu references inside callback
    for i, h := range ds.Headers { ... }
    table.Refresh()
})
```

**Safe.** `model.mu` unlocked at line 374, `fyne.Do` called at line 376. No other lock acquisition between them.

### Site 2: `filterMasterRows` empty-snap (line 406)

```go
// line 405
model.mu.Unlock()        // ← Unlock BEFORE fyne.Do

// line 406-408
fyne.Do(func() {
    table.Refresh()
})
```

**Safe.** `model.mu` unlocked at line 405, `fyne.Do` at line 406.

### Site 3: `filterMasterRows` filtered path (line 444)

```go
// line 442
model.mu.Unlock()        // ← Unlock BEFORE fyne.Do

// line 444-446
fyne.Do(func() {
    table.Refresh()
})
```

**Safe.** `model.mu` unlocked at line 442, `fyne.Do` at line 444.

### Site 4: `fetchGridDataAsync` (line 524)

```go
// line 522
model.mu.Unlock()        // ← Unlock BEFORE fyne.Do

// line 524-530
fyne.Do(func() {
    for i, h := range ds.Headers { ... }
    table.Refresh()
})
```

**Safe.** `model.mu` unlocked at line 522, `fyne.Do` at line 524.

**Deadlock audit result: NO DEADLOCK RISK.** All 4 sites unlock `model.mu` before entering `fyne.Do`. No `model.mu.Unlock()` appears inside any callback body. The source-level structural test (`TestDataGrid_ModelMuUnlockedBeforeFyneDo`) independently validates this.

---

## 5. Sidebar Re-entrancy Audit

Full analysis of `SelectByVistaID` (`pkg/ui/sidebar_widget.go:33-65`):

```go
func (nt *NavTree) SelectByVistaID(vistaID string) {
    if vistaID == "" {          // ← Early return guard (REQ-SB-03)
        return                  // ← No dispatch
    }
    nodeID, ok := nt.vistaToNode[vistaID]
    if !ok {                    // ← Early return guard (REQ-SB-03)
        return                  // ← No dispatch
    }

    // Ancestor computation on calling goroutine
    ancestors := []string{}
    for cur := nodeID; cur != ""; { ... }

    nt.navigating.Store(true)                    // ← BEFORE fyne.DoAndWait ✅
    defer func() { nt.navigating.Store(false) }() // ← AFTER fyne.DoAndWait returns ✅

    fyne.DoAndWait(func() {                      // ← Wraps BOTH OpenBranch AND Select ✅
        for i := len(ancestors) - 1; i >= 0; i-- {
            nt.tree.OpenBranch(...)
        }
        nt.tree.Select(widget.TreeNodeID(nodeID))
    })
}
```

| Aspect | Verified | Evidence |
|--------|----------|----------|
| `navigating.Store(true)` before `fyne.DoAndWait` | ✅ | Line 52 (Store) → Line 59 (DoAndWait) |
| `defer navigating.Store(false)` after `Store(true)` | ✅ | Line 53 (defer) follows line 52 (Store) |
| `fyne.DoAndWait` wraps both `OpenBranch` loop AND `Select` | ✅ | Lines 60-64: both inside single `DoAndWait` callback |
| Early returns (empty/unknown vistaID) bypass dispatch | ✅ | Lines 34-39: returns before reaching `navigating.Store` or `fyne.DoAndWait` |

**PASS.** Re-entrancy guard correctly holds across the entire `fyne.DoAndWait` dispatch.

---

## 6. Navigate Audit

Full analysis of the `ui.Navigate` closure (`cmd/golemui/main.go:131-141`):

```go
ui.Navigate = func(vID string) {
    log.Printf("[UI/Navigation] Navigating to screen %q", vID)
    go func() {                                               // REQ-NAV-01: async kickoff
        cleanupMu.Lock()                                      // ← Outside fyne.Do
        if prevCleanup != nil { ... }
        cleanupMu.Unlock()                                    // ← Outside fyne.Do

        node, err := ui.LoadScreen(...)                       // ← Goroutine
        if err != nil { log...; return }                      // REQ-NAV-03: error return before fyne.Do
        newUI, cleanup, err := ui.Compose(...)                // ← Goroutine
        if err != nil { log...; return }                      // REQ-NAV-03: error return before fyne.Do

        cleanupMu.Lock()                                      // ← Outside fyne.Do
        prevCleanup = cleanup
        cleanupMu.Unlock()                                    // ← Outside fyne.Do

        fyne.Do(func() {                                      // REQ-NAV-02: single block
            mainContainer.Objects = []fyne.CanvasObject{newUI}
            mainContainer.Refresh()
            navTree.SelectByVistaID(vID)
        })
    }()
}
```

| Aspect | Verified | Evidence |
|--------|----------|----------|
| 3 UI-mutating statements in single `fyne.Do` | ✅ | Lines 136-138 inside `fyne.Do` at line 135 |
| `cleanupMu` blocks outside `fyne.Do` | ✅ | Lines 115-118 and 131-133 are outside `fyne.Do` |
| Error paths return before `fyne.Do` | ✅ | Lines 121-128 return on error before reaching line 135 |
| `go func()` async kickoff preserved | ✅ | Line 133: `go func() { ... }()` |

**PASS.**

---

## 7. Test Quality Audit

### T-4.6: `TestLoadMasterBuffer_WrapsInFyneDo`
- **Purpose:** Verifies loadMasterBuffer loads data and updates table (REQ-DG-01).
- **Quality:** Polls for table dimensions (2×3) and verifies cell content ("Alice"). Not tautological — confirms data flows from FetchAll through model to table.
- **Verdict:** Meaningful. **PASS.**

### T-4.7: `TestFetchGridDataAsync_WrapsInFyneDo`
- **Purpose:** Verifies fetchGridDataAsync loads data and updates table (REQ-DG-02).
- **Quality:** Same pattern as T-4.6, verifies 2×2 table and cell content "pending".
- **Verdict:** Meaningful. **PASS.**

### T-4.8: `TestFilterMasterRows_EmptySnap_WrapsInFyneDo`
- **Purpose:** Verifies empty snapshot resets to master rows (REQ-DG-03).
- **Quality:** Three-phase test: (1) wait for master load (3 rows), (2) filter to 1 row, (3) clear filter and verify reset to 3 rows. Tests the full lifecycle.
- **Verdict:** Meaningful, thorough. **PASS.**

### T-4.9: `TestFilterMasterRows_Filtered_WrapsInFyneDo`
- **Purpose:** Verifies filtered path reduces rows correctly (REQ-DG-04).
- **Quality:** Loads 3 rows, filters with "ob" → expects 1 row (Bob), verifies cell content.
- **Verdict:** Meaningful. **PASS.**

### T-4.10: `TestDataGrid_ModelMuUnlockedBeforeFyneDo`
- **Purpose:** Structural source-level test verifying REQ-LOCK-01.
- **Quality:** Parses `compositor.go` source, tracks `fyne.Do` callback blocks by brace depth, checks no `model.mu.Unlock()` appears inside any callback. This is the most direct deadlock-prevention test.
- **Verdict:** High-quality structural test. **PASS.**

### T-4.11: `TestDataGrid_NoRefreshMuInModel`
- **Purpose:** Structural test verifying REQ-DG-05 (refreshMu removed).
- **Quality:** Reads `compositor.go` source and checks `refreshMu` is absent. Simple but effective.
- **Verdict:** Meaningful. **PASS.**

### T-4.3: `TestSelectByVistaID_DispatchesViaFyneDoAndWait`
- **Purpose:** Verifies `SelectByVistaID` works from a background goroutine (REQ-SB-01, REQ-SB-02).
- **Quality:** Calls from a goroutine with timeout, verifies no panic/deadlock, then verifies guard is cleared by checking `OnSelected` fires Navigate.
- **Verdict:** Meaningful. **PASS.**

### T-4.4: `TestSelectByVistaID_ReentrancyGuardHoldsAcrossDoAndWait`
- **Purpose:** Verifies re-entrancy guard prevents Navigate during programmatic selection (REQ-SB-02).
- **Quality:** Sets Navigate spy, calls `SelectByVistaID`, asserts Navigate was not called.
- **Verdict:** Directly tests the core anti-loop invariant. **PASS.**

### T-4.5: `TestSelectByVistaID_EmptyAndUnknown_NoFyneDo`
- **Purpose:** Verifies early-return guards bypass dispatch (REQ-SB-03).
- **Quality:** Wraps `OnSelected` spy, calls with `""` and `"nonexistent"`, asserts no selection occurred.
- **Verdict:** Meaningful. **PASS.**

### T-4.1: `TestNavigate_DispatchesUISwapViaFyneDo_Enhanced`
- **Purpose:** Verifies Navigate UI swap works without deadlock (REQ-NAV-02).
- **Quality:** Boots full RunBootstrap, calls Navigate, verifies no panic and split layout remains intact.
- **Verdict:** Integration-level test. **PASS.**

### T-4.2: `TestNavigate_ErrorPath_NoFyneDo`
- **Purpose:** Verifies error path skips UI mutation (REQ-NAV-03).
- **Quality:** Overrides Navigate with a failing version, verifies container content unchanged.
- **Minor note:** The test replaces the real Navigate closure rather than testing through the production code path directly. It verifies the pattern rather than the exact code.
- **Verdict:** Acceptable. **PASS.**

### Weak test assessment

No tests are tautological. The structural tests (T-4.10, T-4.11) directly parse source code and assert invariants — they would fail if the implementation regressed. The behavioral tests (T-4.6 through T-4.9) verify data flows correctly through the async pipeline. All tests make meaningful assertions.

---

## 8. Build & Test Results

```
$ go build ./...     → PASS (no output, no errors)
$ go vet ./...       → PASS (no output, no errors)
$ go test ./... -count=1 -timeout 30s
  ok  GolemUI/cmd/golemui    1.742s
  ok  GolemUI/pkg/config     0.016s
  ok  GolemUI/pkg/dataaccess 0.020s
  ok  GolemUI/pkg/db         1.005s
  ok  GolemUI/pkg/eventbus   0.119s
  ok  GolemUI/pkg/ui         1.916s
```

All packages build, vet, and test clean. No compilation errors, no vet warnings, no test failures.

---

## 9. Findings

### Critical
None.

### Major
None.

### Minor

**MIN-01: `go test -race` not run in verification**
The spec notes (§2, Assumption 4) that `go test -race` surfaces Fyne-internal races (`expiringCache.setAlive`, font metrics cache) that are accepted as known limitations. The verify run did not execute `-race` because the spec explicitly defers it. If the team wants additional confidence, run `go test -race ./...` and filter known Fyne-internal races.

**MIN-02: Navigate error-path test (T-4.2) replaces the Navigate closure**
The `TestNavigate_ErrorPath_NoFyneDo` test in `main_test.go` overrides `ui.Navigate` with a custom closure that simulates the error path. While this correctly verifies the pattern, it does not exercise the actual production Navigate code path. A more robust test would inject a failing `LoadScreen` mock and call through the real closure. This is a test-quality observation, not a correctness issue.

### Info

**INFO-01: Design risk R1 (fire-and-forget `fyne.Do` vs cleanup lifecycle)**
The design document identifies a medium-severity risk: the `fyne.Do` callbacks in `loadMasterBuffer` and `fetchGridDataAsync` are fire-and-forget, and the spawning goroutine exits (completing `wg.Done()`) before the callback necessarily runs on the UI thread. If screen cleanup occurs between dispatch and execution, the table pointer could reference a stale widget. The design argues this is low-risk because `mainContainer.Objects` replacement happens via a separate `fyne.Do` in Navigate, which Fyne serializes. This reviewer agrees the risk is low but recommends the team verify under rapid navigation stress testing when convenient.

**INFO-02: `refreshMu` appears only in test file strings**
The grep for `refreshMu` matches 4 lines in `pkg/ui/compositor_test.go`, all within the structural check test (`TestDataGrid_NoRefreshMuInModel`) which intentionally searches for the string to verify its absence. This is expected and not a concern.

---

## 10. Conclusion

The `fyne-thread-safety-v2` change is correctly implemented. All 13 requirements are satisfied with clear evidence from source code inspection. The 4 DataGrid `fyne.Do` wraps correctly unlock `model.mu` before dispatch (verified both manually and by structural test). The sidebar `SelectByVistaID` correctly uses `fyne.DoAndWait` with the re-entrancy guard positioned before dispatch and cleared after via defer. The Navigate closure correctly wraps the 3 UI-mutating statements in a single `fyne.Do` with error paths returning early. The `refreshMu` field is completely removed. Build, vet, and all tests pass.

**Verdict: PASS**
