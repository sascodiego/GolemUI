# Design: nav-menu-loader

## 1. File Plan

| File | Action | Lines (est.) |
|---|---|---|
| `pkg/ui/sidebar_loader.go` | NEW | ~80 |
| `pkg/ui/sidebar_loader_test.go` | NEW | ~120 |

## 2. sidebar_loader.go Structure

```go
package ui

import (
    "context"
    "fmt"
    "log"
    "GolemUI/pkg/db"
)

const NavigationMenuQuery = "SELECT id, padre_id, titulo, vista_id, orden FROM golemui.menu_navegacion ORDER BY padre_id NULLS FIRST, orden, id"

type MenuItem struct {
    ID       string
    PadreID  string
    Titulo   string
    VistaID  string
    Orden    int
}

func LoadNavigationMenu(ctx context.Context, pool db.DatabasePool) ([]MenuItem, error) {
    // 1. Nil pool guard
    // 2. pool.Query(ctx, NavigationMenuQuery)
    // 3. Scan rows into []MenuItem (NULL → "")
    // 4. validateNoCycles(items)
    // 5. Return items
}

func validateNoCycles(items []MenuItem) error {
    // Build adjacency map: padre_id → []child_id
    // DFS with visiting/visited sets
    // Return fmt.Errorf("cycle detected: %s", path) on cycle
}
```

## 3. DFS Algorithm

1. Build `adjacency map[string][]string`: padre_id → children IDs
2. Build `itemMap map[string]*MenuItem` for lookup
3. Collect roots (PadreID == "")
4. For each root, DFS:
   - `visiting` set = current path
   - `visited` set = fully processed
   - If node in visiting → cycle: build path string and return error
   - If node in visited → skip
   - Add to visiting, recurse children, move to visited
5. Also handle orphans (padre_id referencing non-existent node) as individual roots

## 4. Nullable Column Handling

pgx cannot scan NULL directly into `string`. Use `*string` and convert:
```go
var padreID, vistaID *string
rows.Scan(&item.ID, &padreID, &item.Titulo, &vistaID, &item.Orden)
if padreID != nil { item.PadreID = *padreID }
if vistaID != nil { item.VistaID = *vistaID }
```

## 5. Test Cases

| Test | Setup | Assert |
|---|---|---|
| Valid hierarchy | Mock: nav_principal + 3 children | len=4, sorted, no error |
| Cyclic A→B→A | Mock: A(padre=B), B(padre=A) | error contains "cycle detected" |
| Self-loop | Mock: X(padre=X) | error "cycle detected: X → X" |
| Nil pool | nil pool | error "LoadNavigationMenu: pool is nil" |
| Empty result | Mock: 0 rows | []MenuItem{}, nil |

## 6. Rollback

Delete both new files. No existing code modified.
