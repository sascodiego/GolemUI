# Design: sidebar-hsplit-integration

**Change ID:** sidebar-hsplit-integration  
**Status:** design  
**Date:** 2026-06-07  
**Layer:** Capa 4 — Renderizador Fyne  
**Depends on:** nav-menu-schema ✅, nav-menu-loader ✅, fyne-sidebar-tree ✅  

---

## 1. Overview

This design specifies the exact Go struct layouts, method implementations, and `main.go` bootstrap restructuring required to:

1. Replace the full-window `win.SetContent(newUI)` pattern with a persistent HSplit layout (sidebar left, content right).
2. Make `ui.Navigate` update only the right-side `mainContainer` — no full-window flash.
3. Synchronize sidebar tree selection bidirectionally via `NavTree.SelectByVistaID`.
4. Prevent infinite re-entrancy when `tree.Select()` calls `OnSelected`.

---

## 2. Type Definitions

### 2.1 NavTree struct — `pkg/ui/sidebar_widget.go`

```go
// NavTree wraps a Fyne widget.Tree with navigation-specific metadata.
// It provides bidirectional sync between vista IDs and tree node selection.
type NavTree struct {
    tree        *widget.Tree
    vistaToNode map[string]string // vistaID → menu item ID (tree node UID)
    parentOf    map[string]string // nodeID → parent nodeID (for ancestor expansion)
    navigating  bool              // re-entrancy guard: true during SelectByVistaID
}
```

**Field semantics:**

| Field | Purpose | Populated by |
|-------|---------|--------------|
| `tree` | The underlying Fyne `*widget.Tree` — used for `Select`, `OpenBranch`, and container embedding | `BuildNavTree` constructor |
| `vistaToNode` | Reverse index: given a `vistaID` (e.g., `"home"`), find the tree node UID (e.g., `"nav_home"`). Only items with non-empty `VistaID` are included. | `BuildNavTree` from `MenuItem.VistaID` |
| `parentOf` | Ancestor index: given a node ID (e.g., `"nav_home"`), return its parent ID (e.g., `"nav_principal"`). Root nodes map to `""`. Used to walk the ancestor chain for `OpenBranch` calls. | `BuildNavTree` from `MenuItem.PadreID` |
| `navigating` | Re-entrancy guard. Set to `true` before `tree.Select()` call in `SelectByVistaID`, checked in `OnSelected` to skip re-entry. Always restored to `false` via `defer`. | `SelectByVistaID` sets; `OnSelected` reads |

**No mutex needed.** All access is on the Fyne UI thread (single-threaded event loop). Both `OnSelected` (user click) and `SelectByVistaID` (called from `Navigate` closure) run on the same thread. No concurrent access is possible.

### 2.2 Exported Methods

```go
// Widget returns the underlying *widget.Tree for embedding in Fyne containers.
func (nt *NavTree) Widget() *widget.Tree

// SelectByVistaID programmatically selects the tree node associated with the
// given vistaID. Expands all ancestor branches and selects the target leaf.
// No-op if vistaID is empty or not found in vistaToNode map.
func (nt *NavTree) SelectByVistaID(vistaID string)
```

### 2.3 BuildNavTree Signature Change

**Before:**
```go
func BuildNavTree(items []MenuItem) *widget.Tree
```

**After:**
```go
func BuildNavTree(items []MenuItem) *NavTree
```

The internal construction remains identical (`parentToChildren`, `idToItem` maps). Two additional maps are populated (`vistaToNode`, `parentOf`) and the return value wraps everything in `*NavTree`.

---

## 3. Method Implementations

### 3.1 BuildNavTree — Updated Constructor

```go
func BuildNavTree(items []MenuItem) *NavTree {
    // Existing index building (unchanged)
    parentToChildren := make(map[string][]string)
    idToItem := make(map[string]MenuItem)
    for _, item := range items {
        idToItem[item.ID] = item
        parentToChildren[item.PadreID] = append(parentToChildren[item.PadreID], item.ID)
    }
    // Sort children by Orden then ID (unchanged)
    for parentID, childIDs := range parentToChildren {
        sort.Slice(childIDs, func(i, j int) bool {
            a, okA := idToItem[childIDs[i]]
            b, okB := idToItem[childIDs[j]]
            if !okA || !okB {
                return childIDs[i] < childIDs[j]
            }
            if a.Orden != b.Orden {
                return a.Orden < b.Orden
            }
            return a.ID < b.ID
        })
        parentToChildren[parentID] = childIDs
    }

    // NEW: Build vistaToNode map (vistaID → nodeID, only non-empty VistaID)
    vistaToNode := make(map[string]string)
    for _, item := range items {
        if item.VistaID != "" {
            vistaToNode[item.VistaID] = item.ID
        }
    }

    // NEW: Build parentOf map (nodeID → parentID, includes all nodes)
    parentOf := make(map[string]string)
    for _, item := range items {
        parentOf[item.ID] = item.PadreID
    }

    nt := &NavTree{
        vistaToNode: vistaToNode,
        parentOf:    parentOf,
    }

    tree := widget.NewTree(
        func(uid widget.TreeNodeID) []widget.TreeNodeID {
            children := parentToChildren[string(uid)]
            result := make([]widget.TreeNodeID, len(children))
            for i, id := range children {
                result[i] = widget.TreeNodeID(id)
            }
            return result
        },
        func(uid widget.TreeNodeID) bool {
            _, ok := parentToChildren[string(uid)]
            return ok
        },
        func(branch bool) fyne.CanvasObject {
            return widget.NewLabel("")
        },
        func(uid widget.TreeNodeID, branch bool, node fyne.CanvasObject) {
            label, ok := node.(*widget.Label)
            if !ok {
                return
            }
            item, exists := idToItem[string(uid)]
            if exists {
                label.SetText(item.Titulo)
            }
        },
    )

    // MODIFIED: OnSelected now checks re-entrancy guard
    tree.OnSelected = func(uid widget.TreeNodeID) {
        // Guard: skip if SelectByVistaID triggered this
        if nt.navigating {
            return
        }
        item, exists := idToItem[string(uid)]
        if !exists {
            return
        }
        _, isBranch := parentToChildren[string(uid)]
        if isBranch {
            return
        }
        if item.VistaID == "" {
            return
        }
        if Navigate != nil {
            Navigate(item.VistaID)
        }
    }

    nt.tree = tree
    return nt
}
```

**Key change:** The closure captures `nt` (the `NavTree` being constructed) to read `nt.navigating`. This is safe because the closure is only called after `BuildNavTree` returns and the tree is rendered.

### 3.2 Widget()

```go
func (nt *NavTree) Widget() *widget.Tree {
    return nt.tree
}
```

Trivial accessor. Needed because `container.NewVScroll` requires `fyne.CanvasObject`, and `*widget.Tree` implements that interface. The caller (`main.go`) passes `navTree.Widget()` to `container.NewVScroll`.

### 3.3 SelectByVistaID

```go
func (nt *NavTree) SelectByVistaID(vistaID string) {
    if vistaID == "" {
        return
    }
    nodeID, exists := nt.vistaToNode[vistaID]
    if !exists {
        return
    }

    // Set re-entrancy guard before any tree mutation
    nt.navigating = true
    defer func() { nt.navigating = false }()

    // Walk ancestor chain from root to immediate parent, open each branch
    // Build path: [immediate_parent, grandparent, ..., root]
    var ancestors []string
    for current := nt.parentOf[nodeID]; current != ""; current = nt.parentOf[current] {
        ancestors = append(ancestors, current)
    }
    // Reverse: open from root → ... → immediate parent
    for i := len(ancestors) - 1; i >= 0; i-- {
        nt.tree.OpenBranch(widget.TreeNodeID(ancestors[i]))
    }

    // Select the target leaf
    nt.tree.Select(widget.TreeNodeID(nodeID))
}
```

**Algorithm for ancestor traversal:**

Given a tree hierarchy: `nav_principal → nav_transacciones` where `nodeID = "nav_transacciones"`:
1. `parentOf["nav_transacciones"] = "nav_principal"` → ancestors = `["nav_principal"]`
2. `parentOf["nav_principal"] = ""` → stop
3. Reverse: `["nav_principal"]` → open `nav_principal`
4. Select `nav_transacciones`

For deeper hierarchies (e.g., `root → parent → child → leaf`):
1. Walk: `leaf → child → parent → root`
2. Reverse: `root → parent → child`
3. Open: root, then parent, then child
4. Select: leaf

**Why reverse?** Branches must be opened from root outward. If a parent is not yet open when we try to open a grandchild, Fyne may not have the grandchild rendered yet.

**Re-entrancy flow:**
1. `nt.navigating = true`
2. `tree.OpenBranch(ancestors...)` — `OpenBranch` does NOT call `OnSelected` (confirmed from Fyne v2.5.5 source: it calls `OnBranchOpened` if set, not `OnSelected`).
3. `tree.Select(nodeID)` — **calls `OnSelected`** (confirmed from Fyne v2.5.5 source: line 311 `if f := t.OnSelected; f != nil { f(uid) }`).
4. Inside `OnSelected`: `nt.navigating == true` → return immediately → no `Navigate` call.
5. `defer` restores `nt.navigating = false`.

---

## 4. main.go Restructuring

### 4.1 New Import

```go
import (
    // ... existing imports ...
    "fyne.io/fyne/v2/container"
)
```

### 4.2 RunBootstrap — Restructured Flow

The current flow (annotated with line references from the original):

```
L72-84:  ui.Navigate assigned (closes over win, ctx, cfg)
L87-89:  vistaID resolution
L91-95:  LoadScreen(home) → homeNode
L97-100: Compose(homeNode) → homeUI
L102:    win.SetContent(homeUI)      ← FULL WINDOW
```

**New flow:**

```go
func RunBootstrap(ctx context.Context, cfg *config.BootstrapConfig, runWindow bool, fyneApp fyne.App) (*App, error) {
    // 0. Sanitize locale
    sanitizeLocale()

    // 1. Convert config to db Config (unchanged)
    coreCfg := db.Config{ /* ... unchanged ... */ }
    bizCfg := db.Config{ /* ... unchanged ... */ }

    // 2. Database pool initialization (unchanged)
    dbPool, err := initDB(ctx, coreCfg, bizCfg)
    if err != nil {
        return nil, fmt.Errorf("failed to initialize database: %w", err)
    }
    ui.BusinessPool = dbPool.BusinessPool
    ui.CorePool = dbPool.CorePool

    // 3. Event bus setup (unchanged)
    eb := eventbus.NewEventBus()
    ui.LocalEventBus = eb

    // 3.5. Initialize Fyne app & Window (unchanged)
    if fyneApp == nil {
        fyneApp = app.New()
    }
    win := fyneApp.NewWindow("GolemUI Client")

    // ── NEW: Build sidebar ──────────────────────────────────────

    // 3.6. Load navigation menu from core database
    menuItems, err := ui.LoadNavigationMenu(ctx, ui.CorePool)
    if err != nil {
        dbPool.Close()
        return nil, fmt.Errorf("failed to load navigation menu: %w", err)
    }

    // 3.7. Build sidebar NavTree
    navTree := ui.BuildNavTree(menuItems)

    // 3.8. Create layout containers
    mainContainer := container.NewMax()
    sidebarScroll := container.NewVScroll(navTree.Widget())
    split := container.NewHSplit(sidebarScroll, mainContainer)
    split.SetOffset(0.2)

    // 3.9. Set window content ONCE — HSplit with sidebar + empty mainContainer
    win.SetContent(split)

    // ── NEW: Navigate closure (captures mainContainer + navTree) ─

    // Setup navigation callback — updates mainContainer only
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

    // ── Load entry screen ────────────────────────────────────────

    // 4. Resolve entry vistaID
    vistaID := cfg.EntryPointViewID
    if vistaID == "" {
        vistaID = "home"
    }

    // 4.1. Load and compose entry screen into mainContainer
    homeNode, err := ui.LoadScreen(ctx, ui.CorePool, vistaID, cfg.LayoutQuery)
    if err != nil {
        dbPool.Close()
        return nil, fmt.Errorf("failed to load screen %q: %w", vistaID, err)
    }

    homeUI, err := ui.Compose(homeNode, vistaID)
    if err != nil {
        dbPool.Close()
        return nil, fmt.Errorf("failed to compose home UI: %w", err)
    }

    mainContainer.Objects = []fyne.CanvasObject{homeUI}
    navTree.SelectByVistaID(vistaID)

    a := &App{
        Config:   cfg,
        DB:       dbPool,
        EventBus: eb,
        FyneApp:  fyneApp,
        Window:   win,
    }

    if runWindow {
        win.ShowAndRun()
    }

    return a, nil
}
```

### 4.3 Key Differences from Current Code

| Aspect | Current | New |
|--------|---------|-----|
| `Navigate` closure captures | `win` (for `SetContent`) | `mainContainer`, `navTree`, `ctx`, `CorePool`, `cfg.LayoutQuery` |
| Navigation updates | `win.SetContent(newUI)` | `mainContainer.Objects = []fyne.CanvasObject{newUI}; mainContainer.Refresh()` |
| Sidebar sync | None | `navTree.SelectByVistaID(vID)` after compose |
| Window content set | Twice (Navigate + initial home) | **Once** at HSplit construction |
| Menu loading | Not present | `LoadNavigationMenu` before tree build |
| Error handling | Navigate errors logged only | Same (navigate errors logged; menu/loadscreen errors fatal) |

---

## 5. Fyne API Reference

All APIs confirmed available in `fyne.io/fyne/v2@v2.5.5`:

### 5.1 container.NewHSplit

```go
// container/split.go:29
func NewHSplit(leading, trailing fyne.CanvasObject) *Split

// Split struct fields:
type Split struct {
    widget.BaseWidget
    Offset     float64    // 0.0–1.0, proportion for leading panel
    Horizontal bool
    Leading    fyne.CanvasObject
    Trailing   fyne.CanvasObject
}

// SetOffset sets the split position (0.0 = all leading, 1.0 = all trailing)
func (s *Split) SetOffset(offset float64)
```

### 5.2 container.NewVScroll

```go
// container/scroll.go:53
func NewVScroll(content fyne.CanvasObject) *Scroll
```

### 5.3 container.NewMax

```go
// container/layouts.go:95
func NewMax(objects ...fyne.CanvasObject) *fyne.Container
```

### 5.4 widget.Tree Selection APIs

```go
// widget/tree.go:228 — opens branch, does NOT call OnSelected
func (t *Tree) OpenBranch(uid TreeNodeID)

// widget/tree.go:303 — selects node, DOES call OnSelected
func (t *Tree) Select(uid TreeNodeID)

// widget/tree.go:419 — clears all selections
func (t *Tree) UnselectAll()
```

**Critical Fyne behavior confirmed from source (v2.5.5 `tree.go:303-314`):**
```go
func (t *Tree) Select(uid TreeNodeID) {
    if len(t.selected) > 0 {
        if uid == t.selected[0] {
            return // no change
        }
        if f := t.OnUnselected; f != nil {
            f(t.selected[0])
        }
    }
    t.selected = []TreeNodeID{uid}
    t.ScrollTo(uid)
    if f := t.OnSelected; f != nil {   // ← THIS CALLS OnSelected
        f(uid)
    }
}
```

This confirms the re-entrancy risk is real. `tree.Select(uid)` always invokes `OnSelected(uid)` (unless the node is already selected, in which case it returns early with "no change"). The guard is essential.

**Also confirmed:** `tree.Select` has an early return when `uid == t.selected[0]` (same node already selected). This means re-selecting the same node is a no-op and does NOT call `OnSelected`. The guard is still needed for the case where the user navigates to a *different* view via a button, then the sidebar selects the new node.

---

## 6. Sidebar Width Strategy

The sidebar width is controlled by `split.SetOffset(0.2)`, which sets the initial proportion to 20% left / 80% right. The user can drag the split divider to resize.

No custom `MinSize` wrapper is needed. The `container.NewVScroll` wraps the tree and provides its own scrolling behavior. The tree's natural `MinSize` (based on the widest label plus padding) provides a reasonable minimum.

**Fallback consideration:** If the tree's natural MinSize is too narrow, a future enhancement can wrap `navTree.Widget()` in a custom canvas object with `MinSize()` returning `fyne.NewSize(220, 0)`. This is explicitly out of scope for this change — `SetOffset(0.2)` is sufficient.

---

## 7. Test Migration Plan

### 7.1 sidebar_widget_test.go — Mechanical Changes

Every test function that currently does:
```go
tree := ui.BuildNavTree(items)
```

Must change to:
```go
nt := ui.BuildNavTree(items)
tree := nt.Widget()
```

This is a mechanical find-and-replace. The rest of each test function remains identical because all assertions use `tree.ChildUIDs`, `tree.IsBranch`, `tree.UpdateNode`, and `tree.OnSelected` — all accessed through the `*widget.Tree` returned by `nt.Widget()`.

**Affected test functions (9 total):**

| Test Function | Change |
|---------------|--------|
| `TestBuildNavTree_PopulatesCorrectTitles` | `tree :=` → `nt :=; tree := nt.Widget()` |
| `TestBuildNavTree_LeafTriggersNavigate` | Same |
| `TestBuildNavTree_BranchDoesNotTriggerNavigate` | Same |
| `TestBuildNavTree_LeafWithoutVistaIDDoesNotNavigate` | Same |
| `TestBuildNavTree_EmptyItems` | Same |
| `TestBuildNavTree_ChildrenSortedByOrden` | Same |
| `TestBuildNavTree_NilNavigateDoesNotPanic` | Same |
| `TestBuildNavTree_NilSlice` | Same |
| `TestBuildNavTree_UpdateNodeSetsTitulo` | Same |

### 7.2 sidebar_widget_test.go — New Test Functions

#### TestNavTree_WidgetReturnsTree
```go
func TestNavTree_WidgetReturnsTree(t *testing.T) {
    items := []ui.MenuItem{
        {ID: "root", PadreID: "", Titulo: "Root", VistaID: "", Orden: 0},
    }
    nt := ui.BuildNavTree(items)
    if nt == nil {
        t.Fatal("expected non-nil NavTree")
    }
    tree := nt.Widget()
    if tree == nil {
        t.Fatal("expected non-nil *widget.Tree from Widget()")
    }
}
```

#### TestNavTree_SelectByVistaID_Valid
```go
func TestNavTree_SelectByVistaID_Valid(t *testing.T) {
    items := []ui.MenuItem{
        {ID: "root", PadreID: "", Titulo: "Root", VistaID: "", Orden: 0},
        {ID: "nav_home", PadreID: "root", Titulo: "Home", VistaID: "home", Orden: 1},
    }
    nt := ui.BuildNavTree(items)
    
    // Should not panic or infinite loop
    nt.SelectByVistaID("home")
    
    // Verify guard was cleared
    // (no public accessor needed — the method returns and no deadlock proves it)
}
```

#### TestNavTree_SelectByVistaID_EmptyIsNoOp
```go
func TestNavTree_SelectByVistaID_EmptyIsNoOp(t *testing.T) {
    items := []ui.MenuItem{
        {ID: "root", PadreID: "", Titulo: "Root", VistaID: "", Orden: 0},
    }
    nt := ui.BuildNavTree(items)
    // Should not panic
    nt.SelectByVistaID("")
    nt.SelectByVistaID("nonexistent")
}
```

#### TestNavTree_ReentrancyGuardPreventsLoop
```go
func TestNavTree_ReentrancyGuardPreventsLoop(t *testing.T) {
    items := []ui.MenuItem{
        {ID: "root", PadreID: "", Titulo: "Root", VistaID: "", Orden: 0},
        {ID: "nav_home", PadreID: "root", Titulo: "Home", VistaID: "home", Orden: 1},
    }
    nt := ui.BuildNavTree(items)

    navigateCount := 0
    ui.Navigate = func(vistaID string) {
        navigateCount++
        // This simulates the real Navigate behavior: SelectByVistaID
        nt.SelectByVistaID(vistaID)
    }
    defer func() { ui.Navigate = nil }()

    // Trigger selection from OnSelected (simulating user click)
    nt.Widget().OnSelected("nav_home")

    // Navigate should be called exactly once
    if navigateCount != 1 {
        t.Errorf("expected Navigate to be called exactly once, got %d", navigateCount)
    }
}
```

### 7.3 main_test.go — Updated Tests

The existing bootstrap tests use `setupMockDB` which registers only `ui.DefaultLayoutQuery`. With the new flow, `RunBootstrap` also calls `ui.LoadNavigationMenu` which queries `ui.NavigationMenuQuery`. The mock must be updated to register both queries.

**Updated `setupMockDB` helper:**

```go
func setupMockDB(t *testing.T, layoutJSON string, layoutErr error) (*db.MockDBPool, *db.MockDBPool) {
    t.Helper()

    oldInitDB := initDB
    t.Cleanup(func() {
        initDB = oldInitDB
        ui.BusinessPool = nil
        ui.CorePool = nil
        ui.Navigate = nil  // NEW: cleanup Navigate
    })

    coreMock := db.NewMockDBPool()
    bizMock := db.NewMockDBPool()

    if layoutJSON != "" || layoutErr != nil {
        coreMock.RegisterQuery(
            ui.DefaultLayoutQuery,
            []string{"config_columnas"},
            [][]any{{layoutJSON}},
            layoutErr,
        )
    }

    // NEW: Register navigation menu query (returns empty menu by default)
    coreMock.RegisterQuery(
        ui.NavigationMenuQuery,
        []string{"id", "padre_id", "titulo", "vista_id", "orden"},
        [][]any{},  // empty menu — tests don't need sidebar items
        nil,
    )

    initDB = func(ctx context.Context, coreCfg db.Config, bizCfg db.Config) (*db.DB, error) {
        return &db.DB{
            CorePool:     coreMock,
            BusinessPool: bizMock,
        }, nil
    }

    return coreMock, bizMock
}
```

**New test: TestRunBootstrap_HSplitLayout**

```go
func TestRunBootstrap_HSplitLayout(t *testing.T) {
    coreMock, _ := setupMockDB(t, `{"area":"root","component_ref":"container","layout":{"type":"vertical"},"children":[{"area":"header","component_ref":"label","label":"Welcome"}]}`, nil)

    cfg := testConfig()
    cfg.EntryPointViewID = "home"

    ctx := context.Background()
    testApp := test.NewApp()

    appInstance, err := RunBootstrap(ctx, cfg, false, testApp)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    // Verify window content is a Split container
    content := appInstance.Window.Content()
    if content == nil {
        t.Fatal("expected window content, got nil")
    }

    // The HSplit is implemented as *container.Split which embeds widget.BaseWidget.
    // We verify the content type by checking it's not nil and has expected structure.
    // Since container.Split is in a different package, we use type assertion.
    split, ok := content.(*container.Split)
    if !ok {
        t.Fatalf("expected window content to be *container.Split, got %T", content)
    }

    if !split.Horizontal {
        t.Error("expected horizontal split")
    }

    if split.Offset < 0.15 || split.Offset > 0.25 {
        t.Errorf("expected split offset ~0.2, got %f", split.Offset)
    }
}
```

**Note on `container` import in `main_test.go`:** The test file must import `"fyne.io/fyne/v2/container"` to use `*container.Split` type assertion. This is a new import.

---

## 8. Error Handling

### 8.1 LoadNavigationMenu Failure

If `LoadNavigationMenu` returns an error, `RunBootstrap` returns the error immediately. The window is never shown. This is a fatal condition — the sidebar is critical UI infrastructure.

```go
menuItems, err := ui.LoadNavigationMenu(ctx, ui.CorePool)
if err != nil {
    dbPool.Close()
    return nil, fmt.Errorf("failed to load navigation menu: %w", err)
}
```

### 8.2 Empty Menu Items

If `LoadNavigationMenu` returns an empty slice (no menu items in DB):
- `BuildNavTree([]MenuItem{})` returns a valid `*NavTree` with an empty tree.
- The sidebar shows no items — empty panel.
- `SelectByVistaID` is always a no-op (empty `vistaToNode` map).
- The app still loads the entry screen into `mainContainer`.

This is acceptable behavior. A warning log is emitted by `LoadNavigationMenu` implicitly (it logs the count).

### 8.3 Navigate Errors

Navigation errors (LoadScreen/Compose failures) are logged and the function returns early. The `mainContainer` retains its previous content. The sidebar is not updated (no `SelectByVistaID` call on error).

### 8.4 SelectByVistaID Not Found

Silent no-op. No error, no panic, no log message. The sidebar retains its previous selection. This handles edge case E1 from the spec.

---

## 9. File Change Manifest

### 9.1 `pkg/ui/sidebar_widget.go`

| Action | Item | Description |
|--------|------|-------------|
| **ADD** | `NavTree` struct | New exported struct with `tree`, `vistaToNode`, `parentOf`, `navigating` fields |
| **ADD** | `Widget() *widget.Tree` | Method on `*NavTree` — returns underlying tree |
| **ADD** | `SelectByVistaID(vistaID string)` | Method on `*NavTree` — programmatic selection with ancestor expansion |
| **MODIFY** | `BuildNavTree` | Build `vistaToNode` + `parentOf` maps; return `*NavTree` instead of `*widget.Tree`; `OnSelected` checks `nt.navigating` |

### 9.2 `pkg/ui/sidebar_widget_test.go`

| Action | Item | Description |
|--------|------|-------------|
| **MODIFY** | All 9 existing tests | `tree := ui.BuildNavTree(items)` → `nt := ui.BuildNavTree(items); tree := nt.Widget()` |
| **ADD** | `TestNavTree_WidgetReturnsTree` | Verify `Widget()` returns non-nil tree |
| **ADD** | `TestNavTree_SelectByVistaID_Valid` | Verify selection + ancestor expansion without panic |
| **ADD** | `TestNavTree_SelectByVistaID_EmptyIsNoOp` | Verify no-op on empty/invalid vistaID |
| **ADD** | `TestNavTree_ReentrancyGuardPreventsLoop` | Verify Navigate called exactly once despite re-entrant Select |

### 9.3 `cmd/golemui/main.go`

| Action | Item | Description |
|--------|------|-------------|
| **ADD** | `"fyne.io/fyne/v2/container"` import | For `NewHSplit`, `NewVScroll`, `NewMax` |
| **MODIFY** | `RunBootstrap` body | Insert menu loading, NavTree build, HSplit construction; rewrite `Navigate` closure; change initial screen load to set `mainContainer.Objects` instead of `win.SetContent` |

### 9.4 `cmd/golemui/main_test.go`

| Action | Item | Description |
|--------|------|-------------|
| **ADD** | `"fyne.io/fyne/v2/container"` import | For `*container.Split` type assertion |
| **MODIFY** | `setupMockDB` | Register `NavigationMenuQuery` mock; add `ui.Navigate = nil` cleanup |
| **ADD** | `TestRunBootstrap_HSplitLayout` | Verify window content is HSplit with correct properties |

---

## 10. Review Workload Forecast

| Metric | Value |
|--------|-------|
| Estimated changed lines | ~170 (50 new in sidebar_widget.go, 40 changed in main.go, 40 new tests, 20 test migration, 20 main_test updates) |
| 400-line budget risk | Low — well within budget |
| Chained PRs recommended | No |
| Suggested split | Single PR |
| Highest risk item | Re-entrancy guard correctness |

---

## 11. Sequence Diagrams

### 11.1 Initialization Sequence

```
main() → RunBootstrap(ctx, cfg, true, nil)
  │
  ├─ initDB(ctx, coreCfg, bizCfg) → dbPool
  ├─ ui.BusinessPool = dbPool.BusinessPool
  ├─ ui.CorePool = dbPool.CorePool
  ├─ eventbus.NewEventBus() → eb → ui.LocalEventBus = eb
  ├─ fyneApp.NewWindow("GolemUI Client") → win
  │
  ├─ ui.LoadNavigationMenu(ctx, ui.CorePool) → menuItems
  ├─ ui.BuildNavTree(menuItems) → navTree
  │   ├─ builds parentToChildren, idToItem (existing)
  │   ├─ builds vistaToNode (NEW)
  │   ├─ builds parentOf (NEW)
  │   ├─ creates widget.Tree (existing)
  │   └─ installs OnSelected with navigating guard (MODIFIED)
  │
  ├─ container.NewMax() → mainContainer
  ├─ container.NewVScroll(navTree.Widget()) → sidebarScroll
  ├─ container.NewHSplit(sidebarScroll, mainContainer) → split
  ├─ split.SetOffset(0.2)
  ├─ win.SetContent(split)  ← SET ONCE
  │
  ├─ ui.Navigate = closure(mainContainer, navTree, ctx, ...)
  │
  ├─ ui.LoadScreen(ctx, pool, vistaID, query) → homeNode
  ├─ ui.Compose(homeNode, vistaID) → homeUI
  ├─ mainContainer.Objects = []fyne.CanvasObject{homeUI}
  └─ navTree.SelectByVistaID(vistaID)
```

### 11.2 Navigation Sequence (Button Click)

```
User clicks button "navigate:transacciones_list"
  │
  ├─ widget.NewButton callback fires
  │   └─ Navigate("transacciones_list")   // from compositor.go:button case
  │
  ├─ LoadScreen(ctx, pool, "transacciones_list", query) → node
  ├─ Compose(node, "transacciones_list") → newUI
  ├─ mainContainer.Objects = []fyne.CanvasObject{newUI}
  ├─ mainContainer.Refresh()
  │
  └─ navTree.SelectByVistaID("transacciones_list")
      ├─ vistaToNode["transacciones_list"] = "nav_transacciones"
      ├─ nt.navigating = true
      ├─ parentOf chain: "nav_transacciones" → "nav_principal"
      ├─ tree.OpenBranch("nav_principal")
      ├─ tree.Select("nav_transacciones")
      │   └─ Fyne internal: OnSelected("nav_transacciones")
      │       └─ nt.navigating == true → RETURN (guard!)
      ├─ nt.navigating = false (defer)
      └─ done
```

### 11.3 Navigation Sequence (Sidebar Click)

```
User clicks tree leaf "nav_home"
  │
  ├─ tree.OnSelected("nav_home")
  │   ├─ nt.navigating == false → proceed
  │   ├─ idToItem["nav_home"].VistaID = "home" → non-empty
  │   └─ Navigate("home")
  │
  ├─ LoadScreen(ctx, pool, "home", query) → node
  ├─ Compose(node, "home") → newUI
  ├─ mainContainer.Objects = []fyne.CanvasObject{newUI}
  ├─ mainContainer.Refresh()
  │
  └─ navTree.SelectByVistaID("home")
      ├─ vistaToNode["home"] = "nav_home"
      ├─ nt.navigating = true
      ├─ parentOf chain: "nav_home" → "nav_principal"
      ├─ tree.OpenBranch("nav_principal")
      ├─ tree.Select("nav_home")
      │   └─ Fyne internal: OnSelected("nav_home")
      │       └─ nt.navigating == true → RETURN (guard!)
      ├─ nt.navigating = false (defer)
      └─ done
```

---

## 12. Risks and Mitigations

| Risk | Severity | Mitigation |
|------|----------|------------|
| Re-entrancy guard not cleared (UI hang) | High | `defer` ensures guard is always cleared, even on panic |
| `tree.Select` same-node optimization causes guard mismatch | Low | When selecting the same node, Fyne returns early — guard is still cleared by `defer`. No issue. |
| `LoadNavigationMenu` query not registered in test mock | Medium | `setupMockDB` updated to register `NavigationMenuQuery` with empty results |
| `container.Split` type assertion fails in test | Low | Import `"fyne.io/fyne/v2/container"` in test file; `*container.Split` is the concrete type returned by `NewHSplit` |
| Sidebar too narrow/wide | Low | `SetOffset(0.2)` is tunable; user can drag to resize |
| Deep hierarchy causes many `OpenBranch` calls | Low | Iterative traversal (not recursive); depth limited by menu_navegacion data |
