# Tasks: sidebar-hsplit-integration

**Change ID:** sidebar-hsplit-integration  
**Status:** tasks  
**Date:** 2026-06-07  
**TDD Mode:** Strict (RED → GREEN → REFACTOR)  
**Test Command:** `go test ./...`  

---

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~170 |
| 400-line budget risk | Low |
| Chained PRs recommended | No |
| Suggested split | Single PR |
| Highest risk item | Re-entrancy guard correctness (T-1.4) |

---

## Phase 1: NavTree Struct & BuildNavTree Refactor

### T-1.1 Define NavTree struct with unexported fields

| Field | Value |
|-------|-------|
| **File(s)** | `pkg/ui/sidebar_widget.go` |
| **TDD Phase** | GREEN (struct definition has no testable behavior alone; tested via T-1.2) |
| **Description** | Add the `NavTree` exported struct with four unexported fields: `tree *widget.Tree`, `vistaToNode map[string]string`, `parentOf map[string]string`, `navigating bool`. Place the struct definition above the `BuildNavTree` function. |
| **Acceptance Check** | Code compiles without error. Struct is exported but fields are unexported. |
| **Dependencies** | None |

### T-1.2 Add `Widget()` method on NavTree

| Field | Value |
|-------|-------|
| **File(s)** | `pkg/ui/sidebar_widget.go`, `pkg/ui/sidebar_widget_test.go` |
| **TDD Phase** | RED → GREEN |
| **Description** | **RED:** Write `TestNavTree_WidgetReturnsTree` that calls `BuildNavTree` with a single root item, asserts the returned `*NavTree` is non-nil, calls `Widget()`, and asserts the returned `*widget.Tree` is non-nil. This test will not compile until `NavTree` and `Widget()` exist. **GREEN:** Implement `func (nt *NavTree) Widget() *widget.Tree { return nt.tree }`. |
| **Acceptance Check** | `TestNavTree_WidgetReturnsTree` passes. `Widget()` returns the same `*widget.Tree` instance that `BuildNavTree` creates internally. |
| **Dependencies** | T-1.1 (struct must exist for method) |

### T-1.3 Modify BuildNavTree to return `*NavTree`

| Field | Value |
|-------|-------|
| **File(s)** | `pkg/ui/sidebar_widget.go`, `pkg/ui/sidebar_widget_test.go` |
| **TDD Phase** | GREEN (mechanical refactor; existing tests migrate in this same task) |
| **Description** | Change `BuildNavTree` signature from `→ *widget.Tree` to `→ *NavTree`. Inside the function body: (1) build `vistaToNode` map from items with non-empty `VistaID`, (2) build `parentOf` map from all items, (3) create `NavTree` struct with both maps, (4) assign `tree` field, (5) return `*NavTree`. Migrate all 9 existing tests: replace `tree := ui.BuildNavTree(items)` with `nt := ui.BuildNavTree(items); tree := nt.Widget()` in each test function. No test logic changes beyond the two-line replacement. |
| **Affected test functions** | `TestBuildNavTree_PopulatesCorrectTitles`, `TestBuildNavTree_LeafTriggersNavigate`, `TestBuildNavTree_BranchDoesNotTriggerNavigate`, `TestBuildNavTree_LeafWithoutVistaIDDoesNotNavigate`, `TestBuildNavTree_EmptyItems`, `TestBuildNavTree_ChildrenSortedByOrden`, `TestBuildNavTree_NilNavigateDoesNotPanic`, `TestBuildNavTree_NilSlice`, `TestBuildNavTree_UpdateNodeSetsTitulo` |
| **Acceptance Check** | All 9 migrated tests + `TestNavTree_WidgetReturnsTree` pass. `BuildNavTree` returns `*NavTree`. `vistaToNode` and `parentOf` maps populated correctly (verified indirectly through T-1.4 tests). |
| **Dependencies** | T-1.2 |

### T-1.4 Add `SelectByVistaID` method with ancestor walk + re-entrancy guard

| Field | Value |
|-------|-------|
| **File(s)** | `pkg/ui/sidebar_widget.go`, `pkg/ui/sidebar_widget_test.go` |
| **TDD Phase** | RED → GREEN |
| **Description** | **RED:** Write three failing tests: (1) `TestNavTree_SelectByVistaID_Valid` — builds tree with root+leaf, calls `SelectByVistaID("home")`, asserts no panic/deadlock (proves method exists and returns). (2) `TestNavTree_SelectByVistaID_EmptyIsNoOp` — calls `SelectByVistaID("")` and `SelectByVistaID("nonexistent")`, asserts no panic. (3) `TestNavTree_ReentrancyGuardPreventsLoop` — sets `ui.Navigate` to a function that increments a counter and calls `nt.SelectByVistaID(vistaID)`, triggers `tree.OnSelected("nav_home")`, asserts counter == 1 (not 2+). **GREEN:** Implement `SelectByVistaID`: early return on empty vistaID or not-in-map; set `nt.navigating = true` with `defer func() { nt.navigating = false }()`; walk `parentOf` chain building ancestors slice; reverse and call `tree.OpenBranch` for each ancestor; call `tree.Select` on target node. |
| **Acceptance Check** | All three new tests pass. Method handles empty/missing vistaID as no-op. Re-entrancy guard prevents infinite Navigate→Select→OnSelected→Navigate loop. Ancestor chain opened before selection. |
| **Dependencies** | T-1.3 (needs `*NavTree` with `vistaToNode` + `parentOf` maps) |

### T-1.5 Update OnSelected callback to check navigating guard

| Field | Value |
|-------|-------|
| **File(s)** | `pkg/ui/sidebar_widget.go` |
| **TDD Phase** | GREEN (covered by T-1.4 re-entrancy test) |
| **Description** | In `BuildNavTree`, inside the `tree.OnSelected` closure, add `if nt.navigating { return }` as the first statement. The closure already captures `nt` (the `NavTree` being built). This prevents `Navigate` from being called when `SelectByVistaID` triggers `tree.Select`. |
| **Acceptance Check** | `TestNavTree_ReentrancyGuardPreventsLoop` from T-1.4 passes (this is the test that validates the guard). All 9 migrated tests from T-1.3 still pass (regression: normal OnSelected → Navigate flow unchanged when `navigating == false`). |
| **Dependencies** | T-1.3 (closure must reference `nt`), T-1.4 (test already written for RED) |

---

## Phase 2: main.go HSplit Integration

### T-2.1 Add `container` import to main.go

| Field | Value |
|-------|-------|
| **File(s)** | `cmd/golemui/main.go` |
| **TDD Phase** | GREEN (mechanical; verified by compilation) |
| **Description** | Add `"fyne.io/fyne/v2/container"` to the import block. |
| **Acceptance Check** | Code compiles without error. |
| **Dependencies** | None |

### T-2.2 Restructure RunBootstrap — HSplit layout with sidebar

| Field | Value |
|-------|-------|
| **File(s)** | `cmd/golemui/main.go` |
| **TDD Phase** | GREEN (test in T-2.3 validates) |
| **Description** | Restructure `RunBootstrap` function body after event bus setup and window creation. New steps in order: (1) Call `ui.LoadNavigationMenu(ctx, ui.CorePool)` — on error, close DB and return error. (2) Call `ui.BuildNavTree(menuItems)` → `navTree`. (3) Create `mainContainer := container.NewMax()`. (4) Create `sidebarScroll := container.NewVScroll(navTree.Widget())`. (5) Create `split := container.NewHSplit(sidebarScroll, mainContainer)` and `split.SetOffset(0.2)`. (6) Call `win.SetContent(split)` — **once, never again**. (7) Assign `ui.Navigate` closure that captures `mainContainer`, `navTree`, `ctx`, `ui.CorePool`, `cfg.LayoutQuery` — inside: `LoadScreen` → `Compose` → `mainContainer.Objects = []fyne.CanvasObject{newUI}` → `mainContainer.Refresh()` → `navTree.SelectByVistaID(vID)`. (8) Load entry screen: `LoadScreen` → `Compose` → `mainContainer.Objects = []fyne.CanvasObject{homeUI}` → `navTree.SelectByVistaID(vistaID)`. Remove the old `win.SetContent(homeUI)` line and old `ui.Navigate` assignment. |
| **Acceptance Check** | Code compiles. Window content is set exactly once (at HSplit construction). Navigate closure updates `mainContainer` instead of calling `win.SetContent`. |
| **Dependencies** | T-1.5 (NavTree API complete), T-2.1 (import) |

### T-2.3 Update setupMockDB and add HSplit bootstrap test

| Field | Value |
|-------|-------|
| **File(s)** | `cmd/golemui/main_test.go` |
| **TDD Phase** | RED → GREEN |
| **Description** | **RED:** (1) Modify `setupMockDB` to register `ui.NavigationMenuQuery` mock returning empty menu (`[][]any{}`) with columns `["id", "padre_id", "titulo", "vista_id", "orden"]`. Add `ui.Navigate = nil` to the cleanup function. (2) Write `TestRunBootstrap_HSplitLayout` that calls `RunBootstrap` with a valid layout JSON and `runWindow=false`, then asserts `appInstance.Window.Content()` is `*container.Split`, that `split.Horizontal` is true, and `split.Offset` is approximately 0.2. This requires importing `"fyne.io/fyne/v2/container"` in the test file. **GREEN:** The test passes once T-2.2 restructuring is complete. |
| **Acceptance Check** | `TestRunBootstrap_HSplitLayout` passes. All existing bootstrap tests still pass (setupMockDB migration didn't break them). |
| **Dependencies** | T-2.2 (implementation must exist for test to pass) |

---

## Phase 3: Verification

### T-3.1 Run full test suite

| Field | Value |
|-------|-------|
| **File(s)** | All |
| **TDD Phase** | REFACTOR (regression check) |
| **Description** | Run `go test ./...` and verify zero failures. This catches any regression from the `BuildNavTree` signature change, test migration errors, or mock setup issues. |
| **Acceptance Check** | `go test ./...` exits with code 0, all tests pass. |
| **Dependencies** | T-2.3 |

### T-3.2 Run go vet

| Field | Value |
|-------|-------|
| **File(s)** | All |
| **TDD Phase** | REFACTOR (static analysis) |
| **Description** | Run `go vet ./...` to check for common Go issues (unused imports, shadowed variables, incorrect format verbs). |
| **Acceptance Check** | `go vet ./...` exits with code 0, no warnings. |
| **Dependencies** | T-3.1 |

---

## Task Dependency Graph

```
T-1.1 ─→ T-1.2 ─→ T-1.3 ─→ T-1.4 (SelectByVistaID tests)
                     │         │
                     └──────→ T-1.5 (OnSelected guard) ←──┘
                                  │
                            T-2.1 (import) ──→ T-2.2 (RunBootstrap restructure)
                                                    │
                                               T-2.3 (HSplit test + mock update)
                                                    │
                                               T-3.1 (full test suite)
                                                    │
                                               T-3.2 (go vet)
```

## Task Summary

| ID | Title | File(s) | Phase | Dependencies |
|----|-------|---------|-------|-------------|
| T-1.1 | Define NavTree struct | `sidebar_widget.go` | GREEN | — |
| T-1.2 | Add `Widget()` method | `sidebar_widget.go`, `sidebar_widget_test.go` | RED→GREEN | T-1.1 |
| T-1.3 | Modify BuildNavTree → `*NavTree` + migrate tests | `sidebar_widget.go`, `sidebar_widget_test.go` | GREEN | T-1.2 |
| T-1.4 | Add `SelectByVistaID` + 3 tests | `sidebar_widget.go`, `sidebar_widget_test.go` | RED→GREEN | T-1.3 |
| T-1.5 | OnSelected navigating guard | `sidebar_widget.go` | GREEN | T-1.3, T-1.4 |
| T-2.1 | Add container import | `main.go` | GREEN | — |
| T-2.2 | Restructure RunBootstrap | `main.go` | GREEN | T-1.5, T-2.1 |
| T-2.3 | setupMockDB + HSplit test | `main_test.go` | RED→GREEN | T-2.2 |
| T-3.1 | Full test suite | All | REFACTOR | T-2.3 |
| T-3.2 | go vet | All | REFACTOR | T-3.1 |

**Total tasks:** 10  
**Files changed:** 4 (`sidebar_widget.go`, `sidebar_widget_test.go`, `main.go`, `main_test.go`)  
**Estimated lines:** ~170  
