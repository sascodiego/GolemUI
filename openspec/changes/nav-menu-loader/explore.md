# Explore: nav-menu-loader

## 1. Current State

### Reference: `pkg/db/db.go`
- `DatabasePool` interface: `Exec`, `Query`, `QueryRow`, `SendBatch`, `Ping`, `Close`
- `DB` struct holds `CorePool` and `BusinessPool` — menu loader uses `CorePool`
- Mock system: `MockDBPool` with `RegisterQuery(sql, columns, rows, err)` pattern

### Reference: `pkg/ui/screen_loader.go`
- Pattern: `LoadScreen(ctx, pool db.DatabasePool, vistaID string, layoutQuery string) (NodeMeta, error)`
- Nil pool guard, empty query fallback to const, `QueryRow` + `Scan`, JSON unmarshal
- Logging via `log.Printf` with `[UI/ScreenLoader]` prefix
- Tests in `screen_loader_test.go` use external test package (`ui_test`), import `db.NewMockDBPool()`

### Existing test patterns
- Table-driven tests with `setupMock` func
- Mock registration: `mock.RegisterQuery(sql, columns, [][]any{values}, err)`
- Error message assertions: exact string match
- Error type assertions: `errors.As` for wrapped errors

### DB Schema (from nav-menu-schema change, merged)
```sql
CREATE TABLE golemui.menu_navegacion (
    id VARCHAR(100) PRIMARY KEY,
    padre_id VARCHAR(100) REFERENCES golemui.menu_navegacion(id) ON DELETE CASCADE,
    titulo VARCHAR(150) NOT NULL,
    vista_id VARCHAR(100) REFERENCES golemui.vistas_consulta(id) ON DELETE SET NULL,
    orden INTEGER DEFAULT 0 NOT NULL
);
```

## 2. Feasibility

- `MenuItem` struct: straightforward, maps to DB columns
- `LoadNavigationMenu`: uses `pool.Query()` (not `QueryRow`) since we expect multiple rows
- DFS cycle detection: traverse parent-child edges, track visited set per path
- Mock: `RegisterQuery` with multi-row response — already supported by `MockDBPool`

## 3. File Targets

| File | Action |
|---|---|
| `pkg/ui/sidebar_loader.go` | NEW — MenuItem struct + LoadNavigationMenu + DFS validation |
| `pkg/ui/sidebar_loader_test.go` | NEW — unit tests (happy path + cycle detection) |

## 4. Risks

- Mock `Query` returns `MockRows` — need to verify multi-row iteration works correctly
- DFS must handle: self-loops, A→B→A cycles, orphan nodes (padre_id pointing to non-existent)
- Test file must use `ui_test` package (external) to match existing convention
