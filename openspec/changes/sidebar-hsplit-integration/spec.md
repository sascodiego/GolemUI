# Spec: sidebar-hsplit-integration

**Change ID:** sidebar-hsplit-integration
**Status:** spec
**Date:** 2026-06-07
**Layer:** Capa 4 — Renderizador Fyne
**Depends on:** nav-menu-schema ✅, nav-menu-loader ✅, fyne-sidebar-tree ✅

---

## 1. Requirements

### REQ-SPLIT-01: HSplit Layout Construction

The main window content shall be a horizontal split (`container.NewHSplit`) composed of:
- **Leading panel**: the sidebar tree wrapped in `container.NewVScroll`
- **Trailing panel**: a `*fyne.Container` with `container.NewMax` layout holding the active screen view

The split shall be set as window content **once** at bootstrap via `win.SetContent(split)`. Navigation events shall never call `win.SetContent` again.

The sidebar scroll container shall declare a `MinSize` width of 220px. The initial split offset shall be set to 0.2 (20% left, 80% right).

### REQ-NAV-01: Partial Navigation (mainContainer Update)

The `ui.Navigate` closure shall update **only** the trailing `mainContainer`:

```go
mainContainer.Objects = []fyne.CanvasObject{newUI}
mainContainer.Refresh()
```

No `win.SetContent()` call shall occur during navigation. The sidebar and split layout remain untouched.

### REQ-BIDI-01: Bidirectional Navigation Sync

When `ui.Navigate` is called from **any** source (sidebar tree click, `navigate:` button, or external trigger), after composing the new screen into `mainContainer`, the closure shall call `navTree.SelectByVistaID(vistaID)` to synchronize the sidebar tree selection.

`SelectByVistaID` shall:
1. Look up the `vistaID` in the internal `vistaToNode` map.
2. Walk the ancestor chain via `parentOf` map, calling `tree.OpenBranch()` for each ancestor from root to immediate parent.
3. Call `tree.Select(targetNodeID)` for the target leaf.

If the `vistaID` is not found in the map, the call shall be a silent no-op (no error, no panic).

### REQ-GUARD-01: Re-entrancy Guard

`NavTree` shall contain a `navigating bool` field. `SelectByVistaID` shall:
1. Set `nt.navigating = true` before calling `tree.OpenBranch` / `tree.Select`.
2. Set `nt.navigating = false` after `tree.Select` returns (via defer).

The `OnSelected` callback installed by `BuildNavTree` shall check `nt.navigating` before calling `Navigate`. When `nt.navigating == true`, the callback shall return immediately without calling `Navigate`, preventing the infinite loop:

```
Navigate("home") → SelectByVistaID("home") → tree.Select("nav_home")
  → OnSelected("nav_home") → Navigate("home") → ... INFINITE
```

### REQ-API-01: NavTree Struct and BuildNavTree Return Type

`NavTree` shall be an exported struct in `pkg/ui/sidebar_widget.go` with the following public API:

| Method / Field | Signature | Description |
|---|---|---|
| `Widget()` | `func (nt *NavTree) Widget() *widget.Tree` | Returns the underlying `*widget.Tree` for adding to containers |
| `SelectByVistaID` | `func (nt *NavTree) SelectByVistaID(vistaID string)` | Programmatic selection by vista ID; no-op if not found |

`BuildNavTree` signature changes from:
```go
func BuildNavTree(items []MenuItem) *widget.Tree
```
to:
```go
func BuildNavTree(items []MenuItem) *NavTree
```

Internal fields of `NavTree` are unexported:
- `tree *widget.Tree`
- `vistaToNode map[string]string` — vistaID → menu item ID
- `parentOf map[string]string` — node ID → parent node ID
- `navigating bool` — re-entrancy guard

### REQ-PERSIST-01: Sidebar State Persistence Across Navigation

The sidebar tree shall retain its expanded/collapsed state and selection across navigation events. Since `mainContainer.Refresh()` does not touch the split or sidebar, and `tree.Select`/`tree.OpenBranch` only modify the tree's internal state, no additional state persistence mechanism is required.

The sidebar tree object shall be created **once** at bootstrap and never replaced.

### REQ-ENTRY-01: Entry Screen Loads Into mainContainer

The entry screen (resolved from `cfg.EntryPointViewID` or defaulting to `"home"`) shall be composed into `mainContainer` **after** the HSplit is set as window content. The flow:

1. Load navigation menu from DB → `BuildNavTree` → `*NavTree`.
2. Create `mainContainer = container.NewMax()`.
3. Create `split = container.NewHSplit(sidebarScroll, mainContainer)`.
4. `win.SetContent(split)`.
5. Load entry screen → compose → `mainContainer.Objects = []fyne.CanvasObject{homeUI}`.
6. `navTree.SelectByVistaID(entryVistaID)` — highlight entry screen in sidebar.

### REQ-BACKWARD-01: Backward-Compatible Test Migration

Existing tests in `pkg/ui/sidebar_widget_test.go` that call `BuildNavTree` shall be updated to use the new return type. Specifically:
- `tree := ui.BuildNavTree(items)` becomes `nt := ui.BuildNavTree(items)` followed by `tree := nt.Widget()` where tree method access is needed.
- All existing test assertions (`ChildUIDs`, `IsBranch`, `UpdateNode`, `OnSelected`) shall continue to pass without changes to their logic.

No existing test shall be deleted. Only mechanical signature adaptation is allowed.

---

## 2. Scenarios (TDD RED-GREEN-REFACTOR Order)

### S1: BuildNavTree Returns NavTree with Working Widget Accessor

**RED:** Call `BuildNavTree(items)` and assert the result is non-nil. Call `.Widget()` and assert the returned `*widget.Tree` is non-nil.

**GREEN:** `NavTree` struct exists with `Widget()` method returning the internal `*widget.Tree`.

**Assertions:**
- `nt != nil`
- `nt.Widget() != nil`
- `nt.Widget().ChildUIDs("")` returns expected root IDs (backward compat with existing tests)

### S2: SelectByVistaID Selects Leaf Node and Expands Ancestor Path

**RED:** Create a NavTree with hierarchical items (root → parent → leaf). Call `SelectByVistaID("home")`. Assert that `tree.OpenBranch` was called for each ancestor and `tree.Select` was called for the leaf.

**GREEN:** `SelectByVistaID` walks `parentOf` chain, calls `tree.OpenBranch` for each ancestor (root first), then `tree.Select` for the target.

**Test data:**
```go
items := []MenuItem{
    {ID: "root", PadreID: "", Titulo: "Root", VistaID: "", Orden: 0},
    {ID: "parent", PadreID: "root", Titulo: "Parent", VistaID: "", Orden: 0},
    {ID: "leaf_home", PadreID: "parent", Titulo: "Home", VistaID: "home", Orden: 0},
}
```

**Assertions:**
- After `SelectByVistaID("home")`, verify navigating flag is false (guard cleared).
- Verify the vistaToNode map correctly maps `"home"` → `"leaf_home"`.

### S3: SelectByVistaID With Empty or Invalid VistaID Is No-Op

**RED:** Call `SelectByVistaID("")` and `SelectByVistaID("nonexistent")`. Assert no panic, no tree mutation, no error.

**GREEN:** Method checks `vistaToNode` map; if key not found, returns immediately.

**Assertions:**
- `SelectByVistaID("")` → no panic, no tree.Select call.
- `SelectByVistaID("nonexistent")` → no panic, no tree.Select call.

### S4: Re-entrancy Guard Prevents Infinite Loop

**RED:** Set up `Navigate` callback that tracks call count. Call `Navigate("home")` (which triggers `SelectByVistaID` → `tree.Select` → `OnSelected`). Assert `Navigate` is called exactly **once**, not infinitely.

**GREEN:** `OnSelected` checks `nt.navigating`. When `true`, skips `Navigate` call.

**Test mechanism:**
```go
callCount := 0
ui.Navigate = func(vistaID string) {
    callCount++
    navTree.SelectByVistaID(vistaID)  // simulate real Navigate behavior
}
navTree.Widget().OnSelected("leaf_home")
// callCount must be 1, not >1
```

### S5: Navigate Closure Updates mainContainer Objects

**RED:** In a bootstrap test, call the `Navigate` closure with a valid vistaID. Assert that `mainContainer.Objects` was replaced (length 1) and the object differs from the previous one.

**GREEN:** Navigate closure sets `mainContainer.Objects = []fyne.CanvasObject{newUI}` and calls `mainContainer.Refresh()`.

**Assertions:**
- `len(mainContainer.Objects) == 1` after navigation.
- `mainContainer.Objects[0]` is the newly composed widget.

### S6: Navigate Closure Calls SelectByVistaID for Reverse Sync

**RED:** In a bootstrap test, call the Navigate closure. Assert that after navigation, the sidebar tree has the corresponding node selected (via `navTree` state or `tree` selection callback).

**GREEN:** Navigate closure calls `navTree.SelectByVistaID(vID)` after composing the new screen.

### S7: HSplit Layout Structure in Window Content

**RED:** In a bootstrap test, after `RunBootstrap` completes, assert `win.Content()` is a `*container.Split`. Assert the leading panel contains the scroll-wrapped tree and the trailing panel is the `mainContainer`.

**GREEN:** `RunBootstrap` creates `container.NewHSplit(sidebarScroll, mainContainer)` and sets it as window content once.

**Assertions:**
- `win.Content()` is of type `*container.Split`.
- Split offset is approximately 0.2.

### S8: Sidebar Width Constraint via VScroll MinSize

**RED:** Assert that the sidebar scroll container's `MinSize().Width` is ≥ 220.

**GREEN:** A custom wrapper or `ExtendBaseWidget` on the scroll container declares `MinSize()` returning width ≥ 220.

**Note:** If Fyne's `container.NewVScroll` does not allow easy MinSize override, the alternative is to set `split.SetOffset(0.2)` to fix the initial proportion. The test verifies the offset is within an acceptable range (0.15–0.25).

---

## 3. Data Flow

### 3.1 Initialization Flow

```
RunBootstrap(ctx, cfg, ...)
  │
  ├── initDB → ui.CorePool + ui.BusinessPool
  ├── eventbus.NewEventBus → ui.LocalEventBus
  ├── fyneApp.NewWindow → win
  │
  ├── LoadNavigationMenu(ctx, ui.CorePool) → []MenuItem
  ├── BuildNavTree(items) → *NavTree (navTree)
  │     ├── builds parentToChildren, idToItem internally
  │     ├── builds vistaToNode map: vistaID → nodeID
  │     ├── builds parentOf map: nodeID → parentNodeID
  │     └── installs OnSelected with re-entrancy guard
  │
  ├── mainContainer = container.NewMax()
  ├── sidebarScroll = container.NewVScroll(navTree.Widget())
  ├── split = container.NewHSplit(sidebarScroll, mainContainer)
  ├── split.SetOffset(0.2)
  ├── win.SetContent(split)              ← ONE TIME ONLY
  │
  ├── ui.Navigate = func(vID string) {
  │     node := LoadScreen(ctx, pool, vID, query)
  │     newUI := Compose(node, vID)
  │     mainContainer.Objects = []fyne.CanvasObject{newUI}
  │     mainContainer.Refresh()
  │     navTree.SelectByVistaID(vID)     ← reverse sync
  │   }
  │
  ├── LoadScreen(entryID) → homeNode
  ├── Compose(homeNode, entryID) → homeUI
  ├── mainContainer.Objects = []fyne.CanvasObject{homeUI}
  └── navTree.SelectByVistaID(entryID)   ← highlight entry
```

### 3.2 Bidirectional Navigation Flow

```
SOURCE A: Sidebar Tree Click
─────────────────────────────
  user clicks tree leaf "nav_home"
    → tree.OnSelected("nav_home")
      → check nt.navigating → false ✓
      → Navigate("home")
        → LoadScreen("home") → node
        → Compose(node, "home") → newUI
        → mainContainer.Objects = [newUI]
        → mainContainer.Refresh()
        → navTree.SelectByVistaID("home")
          → nt.navigating = true
          → tree.OpenBranch("nav_principal")
          → tree.Select("nav_home")
            → tree.OnSelected("nav_home")
              → check nt.navigating → true → SKIP ✓ (guard)
          → nt.navigating = false (defer)


SOURCE B: Button with navigate: prefix
────────────────────────────────────────
  user clicks button "navigate:transacciones_list"
    → widget.NewButton callback
      → Navigate("transacciones_list")
        → LoadScreen("transacciones_list") → node
        → Compose(node, "transacciones_list") → newUI
        → mainContainer.Objects = [newUI]
        → mainContainer.Refresh()
        → navTree.SelectByVistaID("transacciones_list")
          → nt.navigating = true
          → tree.OpenBranch("nav_principal")
          → tree.Select("nav_transacciones")
            → tree.OnSelected("nav_transacciones")
              → check nt.navigating → true → SKIP ✓ (guard)
          → nt.navigating = false (defer)
```

---

## 4. Edge Cases

### E1: vistaID Not Found in Menu

When `Navigate` is called with a `vistaID` that has no corresponding `MenuItem` (no menu entry links to that view):
- `SelectByVistaID` is a silent no-op.
- The screen still loads and displays in `mainContainer`.
- The sidebar retains its previous selection (Fyne default: no unselection on failed `Select`).

### E2: Root Menu Items (vista_id Empty/NULL)

Root/branch nodes (e.g., `nav_principal` with `VistaID: ""`) are never targets of `SelectByVistaID`. The `vistaToNode` map only contains entries for items with non-empty `VistaID`. Calling `SelectByVistaID("")` is a no-op since `""` is not in the map.

### E3: Multiple Rapid Navigation Calls

If `Navigate("home")` is called, then immediately `Navigate("transacciones_list")` before the first completes:
- Both calls are synchronous (no goroutines in Navigate).
- The second call overwrites `mainContainer.Objects` with the new screen.
- `SelectByVistaID` on the second call correctly selects the new target.
- The re-entrancy guard prevents the `tree.Select` from the first call's reverse sync from interfering (it completes before the second call starts, since everything is synchronous).

### E4: Navigation Before Tree Is Built

During bootstrap, `ui.Navigate` is assigned **after** `navTree` is created. If `Compose` or any other code path calls `Navigate` before the assignment completes, `Navigate` is `nil` and the call is guarded by the existing `Navigate != nil` checks in `compositor.go` button handlers.

### E5: Tree Selection for Deeply Nested Nodes

If the menu hierarchy grows deeper (root → parent → sub-parent → leaf), `SelectByVistaID` must walk the full ancestor chain via `parentOf` and call `OpenBranch` for **each** ancestor from root to immediate parent. The chain traversal is iterative, not recursive, to avoid stack overflow on very deep hierarchies.

### E6: LoadNavigationMenu Returns Empty or Error

If `LoadNavigationMenu` returns an empty slice:
- `BuildNavTree([]MenuItem{})` returns a valid `*NavTree` with an empty tree.
- The sidebar shows no items; `mainContainer` still works.
- `SelectByVistaID` is always a no-op since `vistaToNode` is empty.

If `LoadNavigationMenu` returns an error:
- `RunBootstrap` should return the error (fail fast).
- The window is never shown. This is acceptable behavior — a navigation menu is considered critical UI infrastructure.

---

## 5. Out of Scope

The following are explicitly **NOT** part of this change:

1. **Sidebar collapse/expand toggle** — sidebar is always visible.
2. **Sidebar width persistence** across app sessions — no config file read/write for split offset.
3. **Breadcrumbs or navigation history** — no back/forward stack.
4. **Multi-window support** — single main window only.
5. **Database schema changes** — `menu_navegacion`, `vistas_consulta` untouched.
6. **Event bus or data plugin changes** — no Capa 1-3 changes.
7. **Keyboard shortcuts for navigation** — future enhancement.
8. **Sidebar theming or custom styling** — default Fyne widget styling.
9. **Loading indicators during screen transitions** — synchronous compose.
10. **Configuration file changes** — `golemui_driver.yaml` untouched.
11. **Internationalization of sidebar labels** — uses `Titulo` from DB as-is.
12. **Drag-to-resize sidebar** — Fyne HSplit provides this natively; no custom logic needed.

---

## 6. File Impact Summary

| File | Change Type | Description |
|------|-------------|-------------|
| `pkg/ui/sidebar_widget.go` | Modify | Add `NavTree` struct, `Widget()`, `SelectByVistaID()`, re-entrancy guard, change `BuildNavTree` return type |
| `pkg/ui/sidebar_widget_test.go` | Modify | Adapt existing tests to `*NavTree` + `.Widget()`, add S2–S4 scenario tests |
| `cmd/golemui/main.go` | Modify | Add `container` import, restructure `RunBootstrap` for HSplit layout, rewrite `Navigate` closure |
| `cmd/golemui/main_test.go` | Modify | Update bootstrap tests to verify HSplit structure in window content |

---

## 7. Acceptance Criteria (Binary Validation)

- **AC-1**: On app launch, the window displays an HSplit with sidebar left and content right.
- **AC-2**: Clicking a sidebar leaf renders the corresponding view on the right; sidebar maintains its state.
- **AC-3**: Clicking a `navigate:` button (e.g., "Volver al Listado") renders the target view on the right AND auto-selects + auto-expands the corresponding sidebar node.
- **AC-4**: No `win.SetContent()` call occurs after initial bootstrap — only `mainContainer` updates.
- **AC-5**: All existing tests pass after migration to new `BuildNavTree` signature.
- **AC-6**: `go build ./...` and `go vet ./...` clean.
