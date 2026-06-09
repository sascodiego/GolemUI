# Proposal: four-layer-violations

## 1. Summary

The GolemUI renderer (`pkg/ui/compositor.go`) violates the project's own 4-layer decoupling model: it imports `database/sql/driver`, holds two package-level `db.DatabasePool` globals, runs raw SQL strings against the business database, scans `pgx`-specific row types, and hardcodes a 150 px column width with no metadata source. This change introduces a `DataSource` interface (Layer 4 → Layer 1 boundary) and a `ColumnWidthResolver` interface (Layer 4 → Layer 2/3 boundary), moves SQL execution, driver value normalization, and argument extraction out of the renderer, and replaces the hardcoded width with metadata-driven column sizing seeded in `golemui.componentes` and overridable via `golemui.mapeo_interfaz`. After this change, `pkg/ui` no longer imports `database/sql/driver` or references `db.DatabasePool`, and every `data_grid` column width originates from Layer 2/3 metadata with a single package-level constant fallback.

## 2. Problem Statement

### 2.1 Direct SQL and DB-driver awareness in Layer 4

`pkg/ui/compositor.go` contains two data-grid code paths (`loadMasterBuffer` at L351, `fetchGridDataAsync` at L518) that:

- Read the `BusinessPool` package-level global and call `pool.Query(ctx, rawSQL)` with SQL strings lifted directly from JSONB layout metadata.
- Iterate over `pgx.FieldDescriptions()` and `pgx.Rows.Values()`, coupling the renderer to the PostgreSQL wire protocol.
- Type-assert `driver.Valuer` inside `formatValue` (L587–603) to flatten `pgtype.*` values into strings — a concern that belongs to the data-access layer, not the UI.
- Build positional `[]any` arguments via `extractOrderedArgs` from `ScreenState.Snapshot()` and `node.FilterKeys`.
- In the dynamic-query path (`state:` prefix), interpolate user-supplied values into SQL strings via `fmt.Sprintf("%v", qVal)` (L267–268), which is both a SQL-injection vector and a layer violation.

This violates AGENTS.md §3 (4-layer decoupling) and §7.3 ("No acoplar UI en Datos"). The renderer should consume *resolved data* and *resolved metadata*, never raw SQL and driver values.

### 2.2 Hardcoded 150 px grid column width

`table.SetColumnWidth(i, 150)` appears at L411 and L580 with no metadata source, no constant, and no override path. In the 4-layer model, column width is a Layer 2/3 presentation concern: the default should live in `golemui.componentes` (Layer 2 catalog) and per-column overrides should live in `golemui.mapeo_interfaz` (Layer 3 overrides). The renderer should never invent presentation values.

### 2.3 Impact

- **Coupling**: Any change to the database driver (e.g., switching from pgx to pgx/v6) requires changes in the renderer.
- **Test surface**: ~12 tests in `compositor_test.go` assert raw SQL strings via `MockDBPool.Query`, coupling the test harness to implementation details.
- **Extensibility**: Adding per-column width overrides, alternative data sources, or structured query building requires touching the renderer.
- **Security**: The `fmt.Sprintf("%v", qVal)` interpolation is unparameterized SQL built from user input.

## 3. Goals and Non-Goals

### Goals

1. Remove `database/sql/driver` import and all `pgx`-specific API calls from `pkg/ui/compositor.go`.
2. Remove the `BusinessPool` and `CorePool` package-level globals from `pkg/ui/compositor.go`.
3. Replace raw-SQL execution in the compositor with calls to a `DataSource` interface that returns a clean `DataSet` struct (`[]string` headers + `[][]string` rows + optional `[]string` column-width metadata).
4. Move `formatValue` (driver-value normalization) out of the renderer into the data-source implementation.
5. Move `extractOrderedArgs` (argument building from ScreenState + FilterKeys) out of the renderer into the data-source implementation.
6. Eliminate the `fmt.Sprintf("%v", qVal)` SQL interpolation — the data-source receives the logical source string and args; dynamic-query handling moves into the data-source layer.
7. Replace both `table.SetColumnWidth(i, 150)` sites with metadata-driven resolution from `golemui.componentes` (Layer 2 default) and `golemui.mapeo_interfaz` (Layer 3 override), with a single `const defaultGridColWidth = 150` fallback in the renderer.
8. Add `default_column_width` column to `golemui.componentes` and `column_width` column to `golemui.mapeo_interfaz`.
9. Migrate the ~12 tests from asserting `BusinessPool.Query` SQL strings to asserting `DataSource.Fetch/FetchAll` calls.
10. Preserve the physical Business/Core pool segregation as mandated by AGENTS.md §2.

### Non-Goals

- **Refactoring the package-level global pattern into a struct**: `BusinessPool`/`CorePool` globals become `DataSource`/`ColumnWidthResolver` globals in this change. Converting `compositor.go` from package-level functions to a struct receiver is a separate, larger refactor.
- **Changing the `publish_selection` event contract**: The `map[string]any` payload shape stays the same.
- **Changing `pkg/db/db.go`**: The `DatabasePool`/`DBQuerier` interfaces remain the driver-level abstraction. The new `DataSource` sits above them.
- **Preserving native types in the data-grid model**: The `[][]string` internal storage stays. Native-type preservation (e.g., `[][]any`) is a separate change tracked independently.
- **Addressing thread-safety findings from audit §1 or memory-leak findings from audit §2**: Those are separate SDD changes.
- **Addressing global mutable state from audit §4.1** beyond removing the pool globals: converting to a struct is out of scope.

## 4. Proposed Solution

### 4.1 New abstractions

Two interfaces are introduced in a new file `pkg/ui/datasource.go`:

```go
package ui

import "context"

// DataSet is the clean data contract returned by DataSource.
// Layer 4 receives headers + string rows + optional column-width hints.
// No database driver types leak through.
type DataSet struct {
    Headers      []string
    Rows         [][]string
    ColumnWidths []string // "150px", "1fr", "auto", or "" (use fallback)
}

// DataSource replaces direct BusinessPool access in the compositor.
// Implementations own the pool, the SQL execution, the driver-value
// normalization (formatValue), and the argument extraction (extractOrderedArgs).
type DataSource interface {
    // Fetch executes a data-source query with positional arguments.
    // The source string is the logical data_source from NodeMeta
    // (a SQL query, a "state:" prefixed dynamic key, or future structured spec).
    Fetch(ctx context.Context, source string, args ...any) (DataSet, error)

    // FetchAll loads all data without filter arguments (client-mode master buffer).
    FetchAll(ctx context.Context, source string) (DataSet, error)
}

// ColumnWidthResolver reads column-width metadata from Layer 2/3.
// origen matches vistas_consulta.origen_datos; header matches the
// column name returned in DataSet.Headers.
type ColumnWidthResolver interface {
    // Resolve returns the effective width string for a column, checking
    // Layer 3 overrides first, then Layer 2 defaults.
    // Returns "" when no metadata exists (renderer uses its fallback).
    Resolve(origen string, header string) string
}
```

### 4.2 DataSource implementation

A new file `pkg/dataaccess/sql_datasource.go` (new package `dataaccess`) implements `DataSource`:

- **Owns the `BusinessPool`**: constructed with a `db.DatabasePool` and the `formatValue` function (moved from `pkg/ui/compositor.go`).
- **Owns `extractOrderedArgs`**: moved from `pkg/ui/compositor.go` into this package.
- **Handles dynamic queries**: the `state:` prefix logic currently in the compositor's eventbus subscriber (L263–271) moves into `Fetch`. The data source receives the raw `source` string and the `args`. For `state:`-prefixed sources, the caller passes the resolved query string as a single arg (or the data source resolves it internally from a snapshot). The key design decision (see §5 below) is that the compositor resolves the `state:` key and passes the resulting SQL to `Fetch`; the data source never sees `state:` prefixes.
- **Returns `DataSet`**: after executing the query, the implementation iterates `pgx.Rows`, normalizes values via `formatValue`, and builds `Headers`, `Rows`, and optionally `ColumnWidths` (populated by delegating to `ColumnWidthResolver`).
- **Preserves Business/Core segregation**: `sql_datasource.go` is constructed with a *single* pool. Two instances are created in `main.go`: one wrapping `BusinessPool`, one wrapping `CorePool`. The business data source is the one injected into the compositor's data-grid path.

### 4.3 ColumnWidthResolver implementation

A new file `pkg/dataaccess/column_width_resolver.go` implements `ColumnWidthResolver`:

- **Constructed with the Core pool**: reads from `golemui.mapeo_interfaz` (Layer 3) and `golemui.componentes` (Layer 2).
- **Resolve logic**: first queries `golemui.mapeo_interfaz.column_width` for `(origen_id, columna_fisica)`. If found, returns it. If not, queries `golemui.componentes.default_column_width` for the `data_grid` component row. Returns `""` if neither exists.
- **Caching**: resolution is query-heavy if called per-cell. The resolver caches results in a `sync.Map` keyed by `(origen, header)`. Cache is invalidated only on construction (acceptable for read-heavy workloads during screen lifetime).

### 4.4 Compositor changes

After this change, `pkg/ui/compositor.go`:

- **Removes**: `import "database/sql/driver"`, `var BusinessPool`, `var CorePool`, `func formatValue`, `func extractOrderedArgs`.
- **Adds**: `var DataSource DataSource` and `var ColumnWidthResolver ColumnWidthResolver` as package-level globals (injected from `main.go`).
- **`loadMasterBuffer`**: replaces `BusinessPool.Query(...)` with `DataSource.FetchAll(ctx, node.MasterDataSource)`. The returned `DataSet` populates the model directly. Column widths come from `DataSet.ColumnWidths` (pre-populated by the data source via the resolver) with the `defaultGridColWidth` constant as fallback.
- **`fetchGridDataAsync`**: replaces `BusinessPool.Query(...)` with `DataSource.Fetch(ctx, query, args...)`. Same `DataSet`-driven model update and width resolution.
- **`formatValue`**: deleted from this file. Moves to `pkg/dataaccess/sql_datasource.go`.
- **`extractOrderedArgs`**: deleted from this file. Moves to `pkg/dataaccess/sql_datasource.go`.
- **Dynamic query path**: the `state:` prefix resolution (L263–271 in the eventbus subscriber) stays in the compositor because it reads from `ScreenState.Snapshot()` (a Layer 4 concern). The compositor resolves the dynamic query string and passes it to `DataSource.Fetch`. The data source never receives a `state:` prefix.
- **Column width application**: both `SetColumnWidth` sites use:
  ```go
  for i, h := range ds.Headers {
      w := resolveWidth(ds.ColumnWidths, i, h)
      table.SetColumnWidth(i, w)
  }
  ```
  where `resolveWidth` checks `ColumnWidths[i]`, then calls `ColumnWidthResolver.Resolve(origen, h)`, then falls back to `defaultGridColWidth`.

### 4.5 Wiring in `main.go`

```go
// Current:
ui.BusinessPool = dbPool.BusinessPool
ui.CorePool = dbPool.CorePool

// After:
bizDataSource := dataaccess.NewSQLDataSource(dbPool.BusinessPool)
coreColResolver := dataaccess.NewColumnWidthResolver(dbPool.CorePool)
ui.DataSource = bizDataSource
ui.ColumnWidthResolver = coreColResolver
```

`screen_loader.go` and `sidebar_loader.go` continue to accept `db.DatabasePool` as a parameter — they are clean Layer 4 → Layer 2/3 boundaries that return Go structs. They do not need `DataSource`.

## 5. Design Decisions (Resolving Open Questions)

### Q1: Single pool or split? → Split instances, single interface

The `DataSource` interface is pool-agnostic. Two instances are constructed in `main.go`:
- `businessDataSource` wraps `dbPool.BusinessPool` (for data-grid queries against `negocio_production`).
- `coreDataSource` wraps `dbPool.CorePool` (for metadata queries against `golemui_core`).

The compositor receives the business data source for data fetching and the core-backed `ColumnWidthResolver` for width metadata. The physical DB segregation is preserved: each `DataSource` instance owns exactly one pool, and no cross-database query is possible through the interface.

`ui.CorePool` is retained temporarily for `screen_loader.go` and `sidebar_loader.go` (which use it as a function parameter, not a global), but it moves out of `compositor.go` and into `main.go` as a local variable passed directly to the loader functions.

### Q2: Where does `extractOrderedArgs` move? → Into the data-source caller

`extractOrderedArgs` reads `ScreenState.Snapshot()` (Layer 4) and `node.FilterKeys` (Layer 2/3 metadata). The function moves to `pkg/dataaccess` as an exported utility, but the compositor is responsible for calling it and passing the resulting `[]any` to `DataSource.Fetch`. This keeps the state-reading concern in Layer 4 while moving the argument-structure concern to the data-access layer.

Alternative considered: having `Fetch` accept a `map[string]any` snapshot and `[]string` filter keys. Rejected because it would leak `ScreenState` semantics into the data-access layer.

### Q3: Does the dynamic-query interpolation survive? → No, it moves and gets parameterized

The `fmt.Sprintf("%v", qVal)` interpolation (compositor L267–268) is eliminated from the renderer. The compositor resolves the `state:` key from `ScreenState.Snapshot()` and passes the resulting string as the `source` argument to `DataSource.Fetch`. The data source executes it as a parameterized query where possible. For the `query_runner` use case (user types raw SQL), the data source receives the SQL as a string and executes it directly — this is the legitimate use case. The difference is that the renderer no longer builds or interpolates SQL; it passes the resolved value to the data source.

The `state:` prefix resolution logic stays in the compositor (Layer 4) because it reads `ScreenState`. After resolution, the compositor calls `DataSource.Fetch(ctx, resolvedQuery)` with no args.

### Q4: Renderer-level fallback default for column width? → Yes, single constant

A package-level constant in `pkg/ui/compositor.go`:

```go
const defaultGridColWidth float32 = 150
```

This constant is used only when:
1. `DataSet.ColumnWidths[i]` is empty or out of range, AND
2. `ColumnWidthResolver.Resolve(origen, header)` returns `""`.

The constant preserves visual parity with the current behavior. The seeded `golemui.componentes` row for `data_grid` will include `default_column_width = '150px'`, so the constant is only a safety net for environments where the core DB is not yet seeded.

### Q5: Test migration strategy? → Dual-phase: new interface tests, then migrate

**Phase 1** (this change): Replace `ui.BusinessPool = mockPool` with a `MockDataSource` that records `(source, args)` calls and returns canned `DataSet` values. The ~12 renderer tests are rewritten to assert against the `DataSource` interface:
- Tests that verify "the right SQL was executed" become "the right source was fetched with the right args".
- Tests that verify error handling (nil pool, query error, scan error) become "DataSource.Fetch returns an error".
- The `MockDBPool` in `pkg/db/mock_db.go` is retained for `dataaccess` package tests (where `pgx.Rows` mocking is still needed).

**Phase 2** (data-access tests): New tests in `pkg/dataaccess/sql_datasource_test.go` verify that the SQL data source correctly executes queries, normalizes values, and builds `DataSet`. These tests use `MockDBPool` directly.

This ensures no test regression: every behavior verified by the current tests is verified by the new tests at the appropriate abstraction level.

## 6. Data Flow (Before/After)

### Before (current, dirty)

```
compositor.go data_grid case
  │
  ├─ loadMasterBuffer / fetchGridDataAsync
  │   ├─ reads BusinessPool global (pkg/ui level)
  │   ├─ calls pool.Query(ctx, rawSQL, args...)
  │   ├─ iterates pgx.FieldDescriptions() → headers
  │   ├─ iterates pgx.Rows.Values() → vals
  │   ├─ calls formatValue(val) → driver.Valuer → string
  │   ├─ sets table.SetColumnWidth(i, 150)
  │   └─ calls table.Refresh()
  │
  └─ extractOrderedArgs(snap, filterKeys) → []any
      └─ reads ScreenState.Snapshot() (map[string]any)
```

### After (proposed, clean)

```
compositor.go data_grid case
  │
  ├─ resolveWidth(ColumnWidths, i, header, origen) → float32
  │   ├─ checks DataSet.ColumnWidths[i]
  │   ├─ calls ColumnWidthResolver.Resolve(origen, header)
  │   │   ├─ queries golemui.mapeo_interfaz.column_width (Layer 3)
  │   │   └─ queries golemui.componentes.default_column_width (Layer 2)
  │   └─ falls back to defaultGridColWidth (150)
  │
  ├─ extractOrderedArgs(snap, filterKeys) → []any  [now in pkg/dataaccess]
  │
  ├─ loadMasterBuffer → DataSource.FetchAll(ctx, source) → DataSet
  ├─ fetchGridDataAsync → DataSource.Fetch(ctx, source, args...) → DataSet
  │
  └─ model ← DataSet.Headers, DataSet.Rows
     table.SetColumnWidth(i, resolvedWidth)
     table.Refresh()

pkg/dataaccess/sql_datasource.go (new, owns the dirty work)
  │
  ├─ NewSQLDataSource(pool db.DatabasePool)
  ├─ Fetch(ctx, source, args...) → DataSet
  │   ├─ pool.Query(ctx, source, args...)
  │   ├─ pgx.FieldDescriptions() → headers
  │   ├─ pgx.Rows.Values() → vals
  │   └─ formatValue(val) → driver.Valuer → string
  └─ FetchAll(ctx, source) → DataSet
      └─ delegates to Fetch(ctx, source)

pkg/dataaccess/column_width_resolver.go (new, Layer 2/3 reader)
  │
  ├─ NewColumnWidthResolver(corePool db.DatabasePool)
  └─ Resolve(origen, header) → string
      ├─ SELECT column_width FROM golemui.mapeo_interfaz
      │   WHERE origen_id=$1 AND columna_fisica=$2
      └─ SELECT default_column_width FROM golemui.componentes
          WHERE id='data_grid'
```

## 7. Layer Mapping (After Change)

| Piece | Layer | Package | Notes |
|-------|-------|---------|-------|
| `DataSource` interface | Boundary (4→1) | `pkg/ui` | Defines what the renderer needs |
| `ColumnWidthResolver` interface | Boundary (4→2/3) | `pkg/ui` | Defines what the renderer needs |
| `DataSet` struct | Boundary contract | `pkg/ui` | Clean data transfer object |
| `SQLDataSource` implementation | Layer 1 adapter | `pkg/dataaccess` | Owns pool, SQL, `formatValue` |
| `ColumnWidthResolver` implementation | Layer 2/3 reader | `pkg/dataaccess` | Reads `componentes` + `mapeo_interfaz` |
| `extractOrderedArgs` | Utility | `pkg/dataaccess` | State → positional args |
| `formatValue` | Layer 1 adapter | `pkg/dataaccess` | Driver-value normalization |
| `compositor.go` (renderer) | Layer 4 | `pkg/ui` | No DB imports, no SQL, no driver types |
| `screen_loader.go` | Layer 4→2/3 | `pkg/ui` | Unchanged (already clean) |
| `sidebar_loader.go` | Layer 4→2/3 | `pkg/ui` | Unchanged (already clean) |
| `golemui.componentes.default_column_width` | Layer 2 | DB schema | New column |
| `golemui.mapeo_interfaz.column_width` | Layer 3 | DB schema | New column |

## 8. Database Schema Changes

All changes target `docker/init-db/02_init_core.sql` (ephemeral, destructive, no migration files per AGENTS.md §8.2).

### 8.1 `golemui.componentes` — add `default_column_width`

```sql
CREATE TABLE IF NOT EXISTS golemui.componentes (
    id VARCHAR(50) PRIMARY KEY,
    descripcion TEXT NOT NULL,
    default_column_width VARCHAR(20)  -- NEW: Layer 2 default width for grid components
);

-- Seed: data_grid gets 150px to match current visual behavior
INSERT INTO golemui.componentes (id, descripcion, default_column_width) VALUES
('click_button', 'Botón de ejecución transaccional', NULL),
('text_input', 'Input de texto de una sola línea', NULL),
('text_area', 'Input de texto multilínea', NULL),
('numeric_stepper', 'Selector numérico con límites definidos', NULL),
('barcode_reader', 'Control optimizado para entrada de escáneres rápidos', NULL),
('data_grid', 'Grilla estructurada para visualización y selección de filas', '150px'),
('dropdown_select', 'Selector de opciones basado en claves foráneas', NULL),
('date_picker', 'Selector gráfico de fechas calendarizadas', NULL),
('checkbox_toggle', 'Selector booleano interactivo', NULL),
('numeric_keypad', 'Teclado numérico táctil para ingreso rápido de datos', NULL)
ON CONFLICT (id) DO NOTHING;
```

### 8.2 `golemui.mapeo_interfaz` — add `column_width`

```sql
CREATE TABLE IF NOT EXISTS golemui.mapeo_interfaz (
    origen_id VARCHAR(100) NOT NULL,
    columna_fisica VARCHAR(100) NOT NULL,
    component_ref VARCHAR(50) NOT NULL,
    label VARCHAR(150),
    placeholder VARCHAR(250),
    validation VARCHAR(250),
    column_width VARCHAR(20),  -- NEW: Layer 3 per-column width override
    PRIMARY KEY (origen_id, columna_fisica)
);
```

No seed data for `column_width` — all rows default to NULL, meaning "use Layer 2 default". Developers add overrides via standard `INSERT`/`UPDATE` on `golemui.mapeo_interfaz`.

### 8.3 No other schema changes

`golemui.estilos`, `golemui.vistas_consulta`, `golemui.sesion_borrador`, `golemui.menu_navegacion`, and `golemui.vistas_guardadas` are unchanged.

## 9. Migration Path

### 9.1 Code migration (single PR)

1. **Create `pkg/dataaccess/` package** with `sql_datasource.go`, `column_width_resolver.go`, and `format.go` (containing `formatValue` and `extractOrderedArgs`).
2. **Create `pkg/ui/datasource.go`** with `DataSet`, `DataSource`, and `ColumnWidthResolver` interfaces.
3. **Update `pkg/ui/compositor.go`**:
   - Replace `var BusinessPool db.DatabasePool` with `var DS DataSource`.
   - Replace `var CorePool db.DatabasePool` with `var CWR ColumnWidthResolver` (only if the compositor uses it directly; alternatively, the resolver is only used by the data source).
   - Replace `loadMasterBuffer` internals with `DS.FetchAll(...)`.
   - Replace `fetchGridDataAsync` internals with `DS.Fetch(...)`.
   - Delete `formatValue` and `extractOrderedArgs`.
   - Replace both `SetColumnWidth(i, 150)` with width-resolution logic.
   - Remove `import "database/sql/driver"` and `import "GolemUI/pkg/db"`.
4. **Update `cmd/golemui/main.go`**:
   - Replace `ui.BusinessPool = dbPool.BusinessPool` with `ui.DS = dataaccess.NewSQLDataSource(dbPool.BusinessPool)`.
   - Replace `ui.CorePool = dbPool.CorePool` with passing the core pool directly to loader functions and to `dataaccess.NewColumnWidthResolver(dbPool.CorePool)`.
   - Retain `ui.CorePool` only if `screen_loader.go` and `sidebar_loader.go` still reference it (they accept it as a parameter from `main.go`, so the global can be removed if the parameter path is used exclusively).
5. **Update `docker/init-db/02_init_core.sql`** with the new columns.
6. **Migrate tests** (see §5 Q5 above).

### 9.2 Test migration order (TDD)

Following strict TDD per `openspec/config.yaml`:

1. **RED**: Write `pkg/dataaccess/sql_datasource_test.go` — tests for `Fetch`, `FetchAll`, `formatValue`, `extractOrderedArgs` using `MockDBPool`.
2. **RED**: Write `pkg/dataaccess/column_width_resolver_test.go` — tests for `Resolve` using `MockDBPool`.
3. **RED**: Write `pkg/ui/datasource_test.go` — mock-based tests for the interfaces.
4. **GREEN**: Implement `pkg/dataaccess/` to pass data-access tests.
5. **GREEN**: Update `pkg/ui/compositor.go` to use `DataSource`.
6. **GREEN**: Rewrite `pkg/ui/compositor_test.go` to use `MockDataSource` instead of `MockDBPool`.
7. **GREEN**: Update `cmd/golemui/main.go` wiring.
8. **REFACTOR**: Clean up any remaining `CorePool` references.

### 9.3 Backward compatibility

- The `publish_selection` event payload (`map[string]any`) is unchanged.
- The `dataGridModel` struct stays `[][]string` internally.
- `NodeMeta` JSON shape is unchanged — no layout JSONB migration needed.
- The `pkg/db` package is untouched — existing driver abstraction is reused.

## 10. Risks and Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| **Test regression**: ~12 tests rewritten, risk of behavioral drift | Medium | High | Side-by-side comparison of old SQL assertions vs. new `Fetch` assertions before deleting old tests. Each old test maps 1:1 to a new test. |
| **Column-width cache stale reads**: `ColumnWidthResolver` caches per `(origen, header)` but overrides may change at runtime | Low | Low | Cache is per-screen-lifetime. Resolver is constructed per-screen or per-app. For ephemeral DB model, overrides change only between container restarts. Accept stale-ness for now; add cache invalidation if overrides become runtime-editable. |
| **Dynamic-query path (`state:` prefix)**: moving the resolution to the compositor means the compositor still sees SQL strings | Low | Medium | The compositor sees the *resolved* SQL string but does not execute it. It passes the string to `DataSource.Fetch`. The data-source layer is responsible for execution. The compositor no longer interpolates values into SQL — it reads the resolved value from state and passes it as the `source` parameter. |
| **Performance regression**: extra DB round-trip for column-width resolution on first data load | Low | Low | Resolver caches results. First query per `(origen, header)` pair adds ~1ms. For typical grids (5–10 columns), this is negligible. |
| **`CorePool` global removal scope creep**: `screen_loader.go` and `sidebar_loader.go` accept pool as parameter, but `main.go` currently sets `ui.CorePool` global | Medium | Low | Retain `ui.CorePool` global temporarily if removal inflates scope. It is not a layer violation (screen/sidebar loaders are already clean). Full removal is tracked as a separate cleanup. |

## 11. Success Criteria

These are binary, observable validation points:

1. **No `database/sql/driver` import in `pkg/ui/`**: `grep -r 'database/sql/driver' pkg/ui/` returns zero matches.
2. **No `db.DatabasePool` import in `pkg/ui/compositor.go`**: `grep 'GolemUI/pkg/db' pkg/ui/compositor.go` returns zero matches.
3. **No `BusinessPool` or `CorePool` globals in `pkg/ui/compositor.go`**: `grep -E 'var (BusinessPool|CorePool)' pkg/ui/compositor.go` returns zero matches.
4. **No hardcoded `150` literal in `pkg/ui/compositor.go`**: `grep -n '150' pkg/ui/compositor.go` returns zero matches (only the named constant `defaultGridColWidth`).
5. **All tests pass**: `go test ./...` exits 0.
6. **Build succeeds**: `go build ./...` exits 0.
7. **Column width from metadata**: a data-grid screen renders with column widths read from `golemui.componentes.default_column_width` (verified by integration test against ephemeral DB).
8. **Column width override works**: inserting a row into `golemui.mapeo_interfaz` with `column_width = '200px'` for a specific `(origen_id, columna_fisica)` pair causes the corresponding grid column to be 200px wide (verified by integration test).
9. **Dynamic query (`state:` prefix) still works**: the `query_runner` screen executes user-typed SQL and displays results (verified by existing test, migrated to new `DataSource` mock).

## 12. Rollback Plan

1. **Code rollback**: revert the PR. The change is self-contained in:
   - `pkg/ui/compositor.go` (modified)
   - `pkg/ui/datasource.go` (new, safe to delete)
   - `pkg/dataaccess/` (new, safe to delete)
   - `cmd/golemui/main.go` (modified wiring)
   - `docker/init-db/02_init_core.sql` (schema changes, ephemeral — `docker-compose down && docker-compose up -d` resets)
2. **Database rollback**: no action needed. The ephemeral DB is destroyed on `docker-compose down`. The schema changes are in the init script, which is reverted with the code.
3. **No data loss risk**: this change touches no business data tables. The new columns (`default_column_width`, `column_width`) are metadata-only and seeded from the init script.
4. **Partial rollback**: if only the column-width feature causes issues, the `ColumnWidthResolver` can be disabled by setting `ui.CWR = nil` (the compositor falls back to `defaultGridColWidth`). The `DataSource` change is independent and can be rolled back separately.
