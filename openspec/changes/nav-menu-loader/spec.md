# Nav Menu Loader Specification

## Purpose

Go loader (`pkg/ui/sidebar_loader.go`) that reads menu items from `golemui.menu_navegacion`, validates the tree is acyclic via DFS, and returns sorted `MenuItem` structs.

## Data Model

```go
type MenuItem struct {
    ID       string // menu_navegacion.id
    PadreID  string // menu_navegacion.padre_id ("" for roots, SQL NULL → "")
    Titulo   string // menu_navegacion.titulo
    VistaID  string // menu_navegacion.vista_id ("" when NULL)
    Orden    int    // menu_navegacion.orden
}
```

## Requirements

1. **LoadNavigationMenu**: `func LoadNavigationMenu(ctx context.Context, pool db.DatabasePool) ([]MenuItem, error)`
2. **Nil pool guard**: return `"LoadNavigationMenu: pool is nil"`
3. **SQL query**: `SELECT id, padre_id, titulo, vista_id, orden FROM golemui.menu_navegacion ORDER BY padre_id NULLS FIRST, orden, id`
4. **Row scanning**: SQL NULL → `""` for padre_id and vista_id; defer rows.Close()
5. **DFS cycle detection**: adjacency map, visiting/visited sets, cycle path in error message
6. **Logging**: `log.Printf` with `[UI/NavMenuLoader]` prefix

## Acceptance Criteria

| # | Criterion | Verification |
|---|---|---|
| AC-1 | Valid data returns sorted `[]MenuItem`, no error | Unit test |
| AC-2 | Nil pool returns `"LoadNavigationMenu: pool is nil"` | Exact match |
| AC-3 | Empty result returns `[]MenuItem{}`, nil error | Unit test |
| AC-4 | Cycle A→B→A returns error with `"cycle detected"` | Unit test |
| AC-5 | Self-loop X→X returns `"cycle detected: X → X"` | Unit test |
| AC-6 | SQL query is exact constant | Code inspection |
| AC-7 | Test file uses `package ui_test` | grep |
| AC-8 | `go test ./pkg/ui/...` passes | Test runner |
| AC-9 | `go vet ./pkg/ui/...` clean | Static analysis |
| AC-10 | No Fyne imports in sidebar_loader.go | grep |
