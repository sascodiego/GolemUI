# Scout: fyne-sidebar-tree — Code Context

## Files Retrieved

1. `pkg/ui/compositor.go` (lines 1–25, 80–105) — `Navigate` global var, `NodeMeta`, Fyne imports
2. `pkg/ui/sidebar_loader.go` (lines 1–110) — `MenuItem` struct, `LoadNavigationMenu`, SQL query constant
3. `pkg/ui/compositor_test.go` (lines 1–60, full file) — Fyne widget test patterns, `test.NewApp`, `test.Type`, `test.Tap`
4. `pkg/ui/sidebar_loader_test.go` (full file) — sidebar loader unit test patterns, mock DB pool usage

## Key Code

### 1. `Navigate` — Declaration and Usage

**Declaration** (`pkg/ui/compositor.go`, line 21):
```go
var Navigate func(vistaID string)
```

**Type**: `func(string)` — a nilable package-level function variable, not a callback interface.

**Where it's called** (`pkg/ui/compositor.go`, `composeWithState`, `button` case, ~line 87):
```go
case "button":
    if strings.HasPrefix(node.SubmitAction, "navigate:") && Navigate != nil {
        targetVista := strings.TrimPrefix(node.SubmitAction, "navigate:")
        return widget.NewButton(node.Label, func() {
            Navigate(targetVista)
        }), nil
    }
```

**Pattern**: `Navigate` is a plain function var. Callers (tests, app bootstrap) assign it directly:
```go
ui.Navigate = func(vistaID string) { /* ... */ }
```

**Implication for tree sidebar**: The sidebar widget needs to call `Navigate(vistaID)` when a leaf node is tapped. It should follow the same nil-guard pattern as the button case.

### 2. Fyne Import Paths (exact)

All in `pkg/ui/compositor.go`:
```go
"fyne.io/fyne/v2"
"fyne.io/fyne/v2/container"
"fyne.io/fyne/v2/widget"
```

For `widget.Tree`, the import is `"fyne.io/fyne/v2/widget"`. The tree node ID type is `widget.TreeNodeID` (aliased from `string` in newer Fyne).

### 3. Fyne Widget Test Patterns (`pkg/ui/compositor_test.go`)

**App initialization** (required for all Fyne widget tests):
```go
func TestMain(m *testing.M) {
    test.NewApp()
    m.Run()
}
```

**Widget interaction helpers**:
- `test.Type(entry, "text")` — simulate keyboard input
- `test.Tap(btn)` — simulate button click

**Widget type assertions** (no renderer introspection needed for basic cases):
```go
btn, ok := obj.(*widget.Button)
if !ok {
    t.Fatalf("expected *widget.Button, got %T", obj)
}
```

**Mocking async loading** (polling pattern for data_grid):
```go
for start := time.Now(); time.Since(start) < 500*time.Millisecond; {
    rows, colsCount := table.Length()
    if rows == 2 && colsCount == 3 {
        loaded = true
        break
    }
    time.Sleep(10 * time.Millisecond)
}
```

**State isolation** via `defer` restore:
```go
ui.Navigate = mockFn
defer func() { ui.Navigate = nil }()
```

### 4. `MenuItem` Struct Definition

**File**: `pkg/ui/sidebar_loader.go`

```go
// NavigationMenuQuery retrieves all menu items ordered hierarchically:
const NavigationMenuQuery = "SELECT id, padre_id, titulo, vista_id, orden FROM golemui.menu_navegacion ORDER BY padre_id NULLS FIRST, orden, id"

// MenuItem represents a single node in the navigation menu hierarchy.
type MenuItem struct {
    ID      string // Stable identifier (menu_navegacion.id)
    PadreID string // Parent node ID; empty string for root nodes (SQL NULL → "")
    Titulo  string // Human-readable display label
    VistaID string // Linked view ID; empty string for structural/folder nodes (SQL NULL → "")
    Orden   int    // Sort order among siblings (ascending)
}
```

**Key invariant**: Roots have `PadreID == ""` (empty string, not nil). Folder/structural nodes have `VistaID == ""`. Leaf nodes have a non-empty `VistaID`.

**Ordering guarantee**: SQL query returns roots first (NULLS FIRST), then children grouped by parent, sorted by `orden` ascending, then by `id`.

**Cycle validation**: `LoadNavigationMenu` calls `validateNoCycles` via DFS before returning.

### 5. Existing Sidebar-Related Files

| File | Purpose |
|---|---|
| `pkg/ui/sidebar_loader.go` | DB query + `MenuItem` struct + cycle validation (no widget code) |
| `pkg/ui/sidebar_loader_test.go` | Unit tests for `LoadNavigationMenu` using `db.MockDBPool` |

**No existing sidebar widget files.** No `widget.Tree`, `tree.NewTree`, or `TreeNodeID` usage found anywhere in the codebase.

## Architecture

```
LoadNavigationMenu(ctx, CorePool)
        ↓
    []MenuItem  (flat, ordered: roots first → children by parent/orden)
        ↓
SidebarWidget (new file — TODO)
    ├── Builds parent→children map from flat []MenuItem
    ├── Creates widget.Tree with node IDs = MenuItem.ID strings
    ├── On tap: if VistaID != "" → Navigate(VistaID); else expand/collapse
    └── Injects into main app layout (NewContainerWithScroll or split container)
```

**Data flow**: `CorePool` → `LoadNavigationMenu` → `[]MenuItem` → `widget.Tree` model → Fyne rendering.

The flat `[]MenuItem` slice must be transformed into a tree model. Two approaches for `widget.Tree`:
- **Untyped tree** (`widget.NewTree`): `TreeNodeID` = `string` (MenuItem.ID). Parent lookup by building a `map[string]string` (childID → parentID) or `map[string][]string` (parentID → childrenIDs).
- **Custom struct tree**: define a `NavNode` struct and use `widget.NewTreeWithData`.

The untyped approach avoids a new struct and aligns with the existing flat data shape.

## Start Here

**Open `pkg/ui/sidebar_loader.go`** — this is the data source. It defines `MenuItem` and `LoadNavigationMenu`. The sidebar widget's only job is to transform `[]MenuItem` into a `widget.Tree` and wire tap events to `Navigate`.

Then read **`pkg/ui/compositor.go`** lines 80–105 to see how `Navigate` is called from button components — replicate that guard pattern (`if Navigate != nil`).

## Supervisor Coordination

No blocking decisions needed. The data contract (`MenuItem` flat list) is stable, and the navigation wiring pattern is established. The main open question — typed vs untyped tree — is an implementation detail that the implementing worker can decide based on Fyne API compatibility.
