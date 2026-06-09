# Specification: four-layer-violations

| Field        | Value                             |
|-------------|-----------------------------------|
| Change      | `four-layer-violations`           |
| Date        | 2026-06-07                        |
| Status      | DRAFT                             |
| Dependencies| `openspec/config.yaml` (strict TDD enabled) |
| Proposal    | `openspec/changes/four-layer-violations/proposal.md` |
| Explore     | `openspec/changes/four-layer-violations/explore.md` |
| Audit       | `docs/specify/code_audit_report.md` §3.1, §3.2 |

---

## 1. Requirements Table

### Data Source (DS) — Removing Layer 4 → Layer 1 coupling

| ID       | Description | Acceptance Criteria | Priority |
|----------|-------------|---------------------|----------|
| REQ-DS-01 | `pkg/ui/compositor.go` does not import `database/sql/driver` | `grep -r 'database/sql/driver' pkg/ui/` returns 0 matches | P0 |
| REQ-DS-02 | `pkg/ui/compositor.go` does not reference `db.DatabasePool` | `grep 'GolemUI/pkg/db' pkg/ui/compositor.go` returns 0 matches | P0 |
| REQ-DS-03 | `pkg/ui/compositor.go` does not hold `BusinessPool` or `CorePool` globals | `grep -E 'var (BusinessPool\|CorePool)' pkg/ui/compositor.go` returns 0 matches | P0 |
| REQ-DS-04 | A `DataSource` interface is defined in `pkg/ui/datasource.go` with `Fetch` and `FetchAll` methods | Interface compiles; methods have signatures matching §3 below | P0 |
| REQ-DS-05 | A `DataSet` struct is defined in `pkg/ui/datasource.go` with `Headers []string`, `Rows [][]string`, `ColumnWidths []string` | Struct compiles; fields match §3 below | P0 |
| REQ-DS-06 | `SQLDataSource` in `pkg/dataaccess/` implements `DataSource` using a single `db.DatabasePool` | `var _ ui.DataSource = (*SQLDataSource)(nil)` compiles | P0 |
| REQ-DS-07 | `formatValue` is removed from `pkg/ui/compositor.go` and moved to `pkg/dataaccess/` | `grep 'func formatValue' pkg/ui/compositor.go` returns 0; function exists in `pkg/dataaccess/` | P0 |
| REQ-DS-08 | `extractOrderedArgs` is removed from `pkg/ui/compositor.go` and moved to `pkg/dataaccess/` as an exported function | `grep 'func extractOrderedArgs' pkg/ui/compositor.go` returns 0; `ExtractOrderedArgs` exists in `pkg/dataaccess/` | P0 |
| REQ-DS-09 | `loadMasterBuffer` calls `DataSource.FetchAll` instead of `BusinessPool.Query` | No `pool.Query` call in `loadMasterBuffer`; calls `DS.FetchAll(ctx, source)` | P0 |
| REQ-DS-10 | `fetchGridDataAsync` calls `DataSource.Fetch` instead of `BusinessPool.Query` | No `pool.Query` call in `fetchGridDataAsync`; calls `DS.Fetch(ctx, source, args...)` | P0 |
| REQ-DS-11 | `main.go` wires `dataaccess.NewSQLDataSource(dbPool.BusinessPool)` into `ui.DataSource` | Binary compiles and runs; `ui.DataSource` is non-nil after bootstrap | P0 |
| REQ-DS-12 | `fmt.Sprintf("%v", qVal)` SQL interpolation is removed from `pkg/ui/compositor.go` | `grep 'fmt.Sprintf.*qVal' pkg/ui/compositor.go` returns 0 | P0 |

### Column Width (CW) — Replacing hardcoded 150px with metadata-driven resolution

| ID       | Description | Acceptance Criteria | Priority |
|----------|-------------|---------------------|----------|
| REQ-CW-01 | No hardcoded `150` literal for column width in `pkg/ui/compositor.go` | `grep -n 'SetColumnWidth(i, 150)' pkg/ui/compositor.go` returns 0 matches | P0 |
| REQ-CW-02 | A `const defaultGridColWidth float32 = 150` exists in `pkg/ui/compositor.go` as the single fallback | `grep 'defaultGridColWidth' pkg/ui/compositor.go` returns exactly 1 definition line | P0 |
| REQ-CW-03 | A `ColumnWidthResolver` interface is defined in `pkg/ui/datasource.go` with a `Resolve(origen, header string) string` method | Interface compiles; method signature matches §3 below | P0 |
| REQ-CW-04 | `golemui.componentes` has a `default_column_width VARCHAR(20)` column | `\d golemui.componentes` in `golemui_core` shows the column | P0 |
| REQ-CW-05 | `golemui.mapeo_interfaz` has a `column_width VARCHAR(20)` column | `\d golemui.mapeo_interfaz` in `golemui_core` shows the column | P0 |
| REQ-CW-06 | `data_grid` row in `golemui.componentes` is seeded with `default_column_width = '150px'` | `SELECT default_column_width FROM golemui.componentes WHERE id='data_grid'` returns `'150px'` | P0 |
| REQ-CW-07 | `ColumnWidthResolver` implementation in `pkg/dataaccess/` queries Layer 3 then Layer 2 then returns `""` | Unit test verifies 3-tier resolution order | P0 |
| REQ-CW-08 | `main.go` wires `dataaccess.NewColumnWidthResolver(dbPool.CorePool)` into `ui.CWR` | Binary compiles; `ui.CWR` is non-nil after bootstrap | P1 |

### Architecture (ARCH) — Structural guarantees

| ID        | Description | Acceptance Criteria | Priority |
|-----------|-------------|---------------------|----------|
| REQ-ARCH-01 | `pkg/ui/` does not import `github.com/jackc/pgx` | `grep -r 'jackc/pgx' pkg/ui/` returns 0 matches | P0 |
| REQ-ARCH-02 | `publish_selection` event payload remains `map[string]any` of string values | Existing test `TestCompose_DataGrid_RowSelection_PublishesToSelectionChannel` passes unchanged | P0 |
| REQ-ARCH-03 | `NodeMeta` JSON shape is unchanged | `json.Marshal`/`json.Unmarshal` of `NodeMeta` produces identical output | P0 |
| REQ-ARCH-04 | `dataGridModel` internal storage stays `[][]string` | `dataGridModel.rows` field type is `[][]string` | P0 |
| REQ-ARCH-05 | `pkg/db/db.go` interfaces are NOT modified | `git diff pkg/db/db.go` shows no changes | P0 |
| REQ-ARCH-06 | All existing tests pass after migration | `go test ./...` exits 0 | P0 |
| REQ-ARCH-07 | Build succeeds | `go build ./...` exits 0 | P0 |
| REQ-ARCH-08 | `screen_loader.go` continues to accept `db.DatabasePool` as parameter | `LoadScreen` signature unchanged | P1 |

---

## 2. Interface Contracts

All interfaces are defined in `pkg/ui/datasource.go` (boundary contracts live with the consumer).

### 2.1 `DataSource` interface

```go
package ui

import "context"

// DataSource replaces direct BusinessPool access in the compositor.
// Implementations own the pool, SQL execution, driver-value normalization
// (formatValue), and argument extraction (extractOrderedArgs).
// The renderer never sees database driver types or SQL strings.
type DataSource interface {
    // Fetch executes a data-source query with positional arguments.
    //
    // Parameters:
    //   ctx   — context for cancellation and timeout; must be respected
    //   source — the logical data source string from NodeMeta
    //            (a SQL query or a resolved dynamic query string)
    //   args   — positional query arguments in filter-keys order
    //
    // Returns:
    //   DataSet — headers + string-normalized rows
    //   error   — non-nil if the query fails, context is cancelled,
    //             or source is empty
    //
    // Error semantics:
    //   - Returns error if ctx is cancelled before or during execution
    //   - Returns error if the underlying pool.Query fails
    //   - Returns error if row scanning fails
    //   - Returns empty DataSet (not error) if the query returns 0 rows
    Fetch(ctx context.Context, source string, args ...any) (DataSet, error)

    // FetchAll loads all data without filter arguments (client-mode master buffer).
    //
    // Parameters:
    //   ctx    — context for cancellation
    //   source — the logical data source string from NodeMeta.MasterDataSource
    //
    // Returns:
    //   DataSet — headers + string-normalized rows
    //   error   — non-nil if the query fails or context is cancelled
    //
    // Error semantics: same as Fetch.
    // Equivalent to Fetch(ctx, source) with no args.
    FetchAll(ctx context.Context, source string) (DataSet, error)
}
```

### 2.2 `ColumnWidthResolver` interface

```go
// ColumnWidthResolver reads column-width metadata from Layer 2/3.
// origen matches vistas_consulta.origen_datos; header matches the
// column name returned in DataSet.Headers.
type ColumnWidthResolver interface {
    // Resolve returns the effective width string for a column.
    //
    // Resolution order:
    //   1. Layer 3: golemui.mapeo_interfaz.column_width WHERE
    //      origen_id = $1 AND columna_fisica = $2
    //   2. Layer 2: golemui.componentes.default_column_width WHERE
    //      id = 'data_grid'
    //   3. Returns "" if neither lookup produces a value
    //
    // Parameters:
    //   origen — the data source identifier (origen_datos from vistas_consulta)
    //   header — the column name from DataSet.Headers
    //
    // Returns:
    //   string — width specification ("150px", "200px", "1fr", "auto")
    //            or "" when no metadata exists
    //
    // Error semantics:
    //   - Returns "" on any DB error (logs the error, does not propagate)
    //   - Returns "" when no matching row exists
    //   - Never returns a non-empty string that is not a valid metric spec
    Resolve(origen string, header string) string
}
```

### 2.3 `DataSet` struct

```go
// DataSet is the clean data contract returned by DataSource.
// Layer 4 receives headers + string rows + optional column-width hints.
// No database driver types leak through.
type DataSet struct {
    // Headers contains the column names from the query result.
    // Length determines the number of columns.
    // Zero value: nil (no columns, no data).
    Headers []string

    // Rows contains the string-normalized cell values.
    // Each inner slice has the same length as Headers.
    // Zero value: nil (no rows, but headers may be present).
    Rows [][]string

    // ColumnWidths contains per-column width hints from metadata.
    // Each entry corresponds to Headers[i] by index.
    // Values follow the metric convention: "150px", "1fr", "auto", or "".
    // Nil or empty string means "use fallback resolution".
    // Zero value: nil (all columns use fallback).
    ColumnWidths []string
}
```

**Zero-value semantics:**
- `DataSet{}` → Headers: nil, Rows: nil, ColumnWidths: nil → means "no data, no columns".
- `DataSet{Headers: []string{}, Rows: [][]string{}}` → explicitly empty result set (query returned 0 rows but has column definitions — note: this depends on driver behavior for empty result sets).
- `DataSet{Headers: []string{"id", "name"}, Rows: nil}` → headers present but no rows; compositor shows headers with 0 data rows.

### 2.4 Helper functions moved to `pkg/dataaccess/`

#### `ExtractOrderedArgs`

```go
package dataaccess

// ExtractOrderedArgs maps snapshot keys to positional args in filterKeys order.
// Missing keys default to empty string (so LIKE '' matches everything instead of NULL = false).
//
// Parameters:
//   snap       — a key-value snapshot (typically from ScreenState.Snapshot())
//   filterKeys — ordered list of keys to extract from the snapshot
//
// Returns:
//   []any — positional arguments in filterKeys order
//           Empty (non-nil) slice when filterKeys is empty
//
// Behavior:
//   - filterKeys empty → returns []any{} (empty slice, not nil)
//   - key present in snap → appends snap[key]
//   - key missing from snap → appends "" (empty string)
//   - snap is nil → treats as empty map; all keys get ""
func ExtractOrderedArgs(snap map[string]any, filterKeys []string) []any
```

#### `FormatValue`

```go
package dataaccess

// FormatValue normalizes a database driver value to a string.
//
// Parameters:
//   val — a value from pgx.Rows.Values(), which may be nil, a Go primitive,
//         or a pgx-specific type implementing driver.Valuer
//
// Returns:
//   string — the string representation of the value
//
// Behavior:
//   - val == nil → returns ""
//   - val implements driver.Valuer → calls .Value(), then:
//     - if underlying value is []byte → string(bytes)
//     - otherwise → fmt.Sprintf("%v", underlying)
//   - val does not implement driver.Valuer → fmt.Sprintf("%v", val)
func FormatValue(val any) string
```

---

## 3. Behavioral Contracts

### 3.1 `Fetch`

| Scenario | Given | When | Then |
|----------|-------|------|------|
| **Success** | DataSource wraps a pool with registered query result; source = `"SELECT id, name FROM t"` | `Fetch(ctx, source, "arg1")` | Returns `DataSet{Headers: ["id", "name"], Rows: [[...], [...]]}`, no error |
| **Empty result** | DataSource wraps a pool; query returns 0 rows | `Fetch(ctx, source)` | Returns `DataSet{Headers: [col names], Rows: [][]string{}}`, no error |
| **Query error** | DataSource wraps a pool; pool.Query returns error | `Fetch(ctx, source)` | Returns `DataSet{}`, wrapped error |
| **Nil source** | DataSource wraps a valid pool | `Fetch(ctx, "")` | Returns `DataSet{}`, error (empty source is invalid) |
| **Cancelled context** | DataSource wraps a valid pool; ctx is already cancelled | `Fetch(ctx, source)` | Returns `DataSet{}`, context.Canceled error |
| **Context cancelled during iteration** | DataSource wraps a valid pool; ctx is cancelled mid-row-scan | `Fetch(ctx, source)` | Returns `DataSet{}`, context.Canceled error (checked between rows) |
| **Row scan error** | DataSource wraps a pool; rows.Values() returns error | `Fetch(ctx, source)` | Returns partial `DataSet` (rows scanned before error), error logged, process continues until break |

### 3.2 `FetchAll`

| Scenario | Given | When | Then |
|----------|-------|------|------|
| **Success** | DataSource wraps a pool with registered query result; source = `"SELECT * FROM t"` | `FetchAll(ctx, source)` | Returns `DataSet{Headers: [...], Rows: [[...]]}`, no error |
| **Empty result** | DataSource wraps a pool; query returns 0 rows | `FetchAll(ctx, source)` | Returns `DataSet{Headers: [...], Rows: [][]string{}}`, no error |
| **Query error** | DataSource wraps a pool; pool.Query returns error | `FetchAll(ctx, source)` | Returns `DataSet{}`, wrapped error |
| **Delegates to Fetch** | Internal implementation | `FetchAll(ctx, source)` | Internally calls `Fetch(ctx, source)` with no additional args |

### 3.3 `Resolve` (Column Width)

| Scenario | Given | When | Then |
|----------|-------|------|------|
| **Layer 3 override exists** | `golemui.mapeo_interfaz` has row `(origen_id="transacciones_list", columna_fisica="status", column_width="200px")` | `Resolve("transacciones_list", "status")` | Returns `"200px"` |
| **No Layer 3, Layer 2 default exists** | No `mapeo_interfaz` row for `(origen, header)`; `componentes` row for `data_grid` has `default_column_width="150px"` | `Resolve("any_origen", "any_header")` | Returns `"150px"` |
| **Neither exists** | No `mapeo_interfaz` row; `componentes.data_grid.default_column_width` is NULL | `Resolve("origen", "header")` | Returns `""` |
| **DB error** | Core pool returns error on query | `Resolve("origen", "header")` | Returns `""`; logs error |
| **Cached result** | Same `(origen, header)` queried twice | `Resolve("origen", "header")` | Second call returns cached value; only 1 DB query executed |

### 3.4 `FormatValue`

| Scenario | Given | When | Then |
|----------|-------|------|------|
| **nil input** | `val = nil` | `FormatValue(nil)` | Returns `""` |
| **driver.Valuer with []byte** | `val` implements `driver.Valuer`; `.Value()` returns `([]byte)("hello")` | `FormatValue(val)` | Returns `"hello"` |
| **driver.Valuer with string** | `val` implements `driver.Valuer`; `.Value()` returns `string("world")` | `FormatValue(val)` | Returns `"world"` |
| **driver.Valuer with error** | `val` implements `driver.Valuer`; `.Value()` returns error | `FormatValue(val)` | Falls through to `fmt.Sprintf("%v", val)` |
| **driver.Valuer returns nil** | `val` implements `driver.Valuer`; `.Value()` returns `(nil, nil)` | `FormatValue(val)` | Falls through to `fmt.Sprintf("%v", val)` |
| **Go primitive (int)** | `val = 42` | `FormatValue(42)` | Returns `"42"` |
| **Go primitive (float)** | `val = 3.14` | `FormatValue(3.14)` | Returns `"3.14"` |
| **Go primitive (string)** | `val = "hello"` | `FormatValue("hello")` | Returns `"hello"` |
| **Go primitive (bool)** | `val = true` | `FormatValue(true)` | Returns `"true"` |

### 3.5 `ExtractOrderedArgs`

| Scenario | Given | When | Then |
|----------|-------|------|------|
| **Normal extraction** | `snap = {"title": "Alice", "author": "Bob"}`, `filterKeys = ["title", "author"]` | `ExtractOrderedArgs(snap, filterKeys)` | Returns `[]any{"Alice", "Bob"}` |
| **Missing key** | `snap = {"title": "Alice"}`, `filterKeys = ["title", "author"]` | `ExtractOrderedArgs(snap, filterKeys)` | Returns `[]any{"Alice", ""}` |
| **Empty filterKeys** | `snap = {"title": "Alice"}`, `filterKeys = []` | `ExtractOrderedArgs(snap, filterKeys)` | Returns `[]any{}` (empty, non-nil) |
| **Nil snapshot** | `snap = nil`, `filterKeys = ["title"]` | `ExtractOrderedArgs(nil, filterKeys)` | Returns `[]any{""}` |
| **Key ordering preserved** | `snap = {"b": "2", "a": "1", "c": "3"}`, `filterKeys = ["c", "a", "b"]` | `ExtractOrderedArgs(snap, filterKeys)` | Returns `[]any{"3", "1", "2"}` |

---

## 4. Data Flow Contracts

### 4.1 Client-mode data grid (loadMasterBuffer → FetchAll)

```
Compose(node) with node.FilterMode="client" && node.MasterDataSource != ""
  │
  ├─ compositor reads ui.DataSource (package-level global)
  │
  ├─ compositor calls DataSource.FetchAll(ctx, node.MasterDataSource)
  │   │
  │   └─ SQLDataSource.FetchAll(ctx, source)
  │       ├─ validates source is non-empty
  │       ├─ calls pool.Query(ctx, source)
  │       ├─ iterates pgx.FieldDescriptions() → headers
  │       ├─ iterates pgx.Rows; for each row:
  │       │   ├─ rows.Values() → []any
  │       │   └─ FormatValue(val) for each val → []string
  │       ├─ builds DataSet{Headers, Rows, ColumnWidths: nil}
  │       └─ returns DataSet, nil
  │
  ├─ compositor receives DataSet
  │   ├─ model.masterHeaders = DataSet.Headers
  │   ├─ model.masterRows = DataSet.Rows
  │   ├─ model.headers = DataSet.Headers
  │   ├─ model.rows = DataSet.Rows
  │   │
  │   └─ for each column i, header h:
  │       ├─ w = resolveWidth(DataSet.ColumnWidths, i, h, origen)
  │       │   ├─ if ColumnWidths[i] is non-empty → parseMetric(ColumnWidths[i])
  │       │   ├─ else if ui.CWR != nil → ColumnWidthResolver.Resolve(origen, h)
  │       │   │   ├─ parseMetric(resolved) if non-empty
  │       │   │   └─ fallback to defaultGridColWidth if ""
  │       │   └─ else → defaultGridColWidth (150)
  │       └─ table.SetColumnWidth(i, w)
  │
  └─ table.Refresh()
```

### 4.2 Server-mode data grid with filter (extractOrderedArgs → Fetch)

```
EventBus subscriber fires on "screen:submit:<vistaID>"
  │
  ├─ receives snap = ScreenState.Snapshot()
  │
  ├─ compositor checks node.DataSource
  │   ├─ NOT "state:" prefix → server-mode with filter keys
  │   │
  │   ├─ checks node.FilterKeys is non-empty (guard)
  │   │   └─ empty → logs warning, returns (no query)
  │   │
  │   ├─ args = dataaccess.ExtractOrderedArgs(snap, node.FilterKeys)
  │   │
  │   ├─ cancels previous context (model.cancel())
  │   ├─ creates new subCtx, subCancel
  │   │
  │   └─ calls DataSource.Fetch(subCtx, node.DataSource, args...)
  │       │
  │       └─ SQLDataSource.Fetch(ctx, source, args...)
  │           ├─ pool.Query(ctx, source, args...)
  │           ├─ same iteration as FetchAll
  │           └─ returns DataSet, nil
  │
  ├─ compositor receives DataSet
  │   ├─ model.headers = DataSet.Headers
  │   ├─ model.columns = DataSet.Headers
  │   ├─ model.rows = DataSet.Rows
  │   ├─ column width resolution (same as §4.1)
  │   └─ table.Refresh()
  │
  └─ done
```

### 4.3 Server-mode data grid with dynamic query (`state:` prefix)

```
EventBus subscriber fires on "screen:submit:<vistaID>"
  │
  ├─ receives snap = ScreenState.Snapshot()
  │
  ├─ compositor checks node.DataSource
  │   └─ starts with "state:" prefix → dynamic query path
  │       │
  │       ├─ stateKey = TrimPrefix(node.DataSource, "state:")
  │       ├─ qVal = snap[stateKey]
  │       ├─ if qVal is missing or empty → logs, returns (no query)
  │       │
  │       ├─ query = fmt.Sprintf("%v", qVal)  [resolved in compositor, Layer 4]
  │       │   NOTE: this resolves the state key, NOT SQL interpolation.
  │       │   The resolved value IS the SQL string the user typed.
  │       │
  │       ├─ cancels previous context
  │       ├─ creates new subCtx
  │       │
  │       └─ calls DataSource.Fetch(subCtx, query)  [no args]
  │           │
  │           └─ SQLDataSource.Fetch(ctx, query)
  │               ├─ pool.Query(ctx, query)  [no args — raw user SQL]
  │               ├─ same iteration
  │               └─ returns DataSet
  │
  └─ compositor receives DataSet
      ├─ model update + column width resolution
      └─ table.Refresh()
```

### 4.4 Column width resolution per grid column

```
resolveWidth(ColumnWidths []string, colIndex int, header string, origen string) float32
  │
  ├─ Step 1: Check inline hint from DataSet
  │   ├─ if colIndex < len(ColumnWidths) && ColumnWidths[colIndex] != ""
  │   │   └─ return parseMetric(ColumnWidths[colIndex]).value
  │   └─ else continue
  │
  ├─ Step 2: Check ColumnWidthResolver (Layer 3 → Layer 2)
  │   ├─ if ui.CWR != nil
  │   │   ├─ resolved = CWR.Resolve(origen, header)
  │   │   ├─ if resolved != ""
  │   │   │   └─ return parseMetric(resolved).value
  │   │   └─ else continue
  │   └─ else continue
  │
  └─ Step 3: Fallback to constant
      └─ return defaultGridColWidth  // 150
```

**`origen` parameter source:** The compositor passes `node.DataSource` (for server-mode) or `node.MasterDataSource` (for client-mode) as the `origen` value to `ColumnWidthResolver.Resolve`. This matches the `origen_datos` field in `golemui.vistas_consulta` which is the same SQL string stored in `NodeMeta.DataSource` / `NodeMeta.MasterDataSource`.

---

## 5. Database Schema Contracts

All DDL targets `docker/init-db/02_init_core.sql` (ephemeral destructive model per AGENTS.md §7.2).

### 5.1 `golemui.componentes` — add `default_column_width`

```sql
CREATE TABLE IF NOT EXISTS golemui.componentes (
    id VARCHAR(50) PRIMARY KEY,
    descripcion TEXT NOT NULL,
    default_column_width VARCHAR(20)  -- NEW: Layer 2 default width for grid components
);

-- Seed data: data_grid gets 150px to match current visual behavior
INSERT INTO golemui.componentes (id, descripcion, default_column_width) VALUES
('click_button',     'Botón de ejecución transaccional',                     NULL),
('text_input',       'Input de texto de una sola línea',                     NULL),
('text_area',        'Input de texto multilínea',                             NULL),
('numeric_stepper',  'Selector numérico con límites definidos',              NULL),
('barcode_reader',   'Control optimizado para entrada de escáneres rápidos', NULL),
('data_grid',        'Grilla estructurada para visualización y selección de filas', '150px'),
('dropdown_select',  'Selector de opciones basado en claves foráneas',       NULL),
('date_picker',      'Selector gráfico de fechas calendarizadas',            NULL),
('checkbox_toggle',  'Selector booleano interactivo',                        NULL),
('numeric_keypad',   'Teclado numérico táctil para ingreso rápido de datos', NULL)
ON CONFLICT (id) DO NOTHING;
```

### 5.2 `golemui.mapeo_interfaz` — add `column_width`

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

No seed data for `column_width` — all rows default to NULL ("use Layer 2 default").

### 5.3 No other schema changes

Tables `golemui.estilos`, `golemui.vistas_consulta`, `golemui.sesion_borrador`, `golemui.menu_navegacion`, and `golemui.vistas_guardadas` are unchanged.

---

## 6. Error Handling

### 6.1 `DataSource.Fetch` / `FetchAll` fails

| Condition | Behavior |
|-----------|----------|
| `pool.Query` returns error | Returns `DataSet{}`, error. Compositor logs the error and does not update the model. Table shows previous data (or empty if first load). |
| `ctx.Err() != nil` before query | Returns `DataSet{}`, `ctx.Err()`. Compositor logs "cancelled before start". |
| `ctx.Err() != nil` during row iteration | Returns `DataSet{}`, `ctx.Err()`. Compositor logs "cancelled during scan". Partial data is discarded. |
| `rows.Values()` returns error | Logs error, breaks iteration. Returns partial `DataSet` (rows scanned before error) and the error. |
| `source` is empty string | Returns `DataSet{}`, error (`"empty source"` or similar). Compositor does not call `Fetch`/`FetchAll` with empty source — it guards before calling. |

### 6.2 `ColumnWidthResolver.Resolve` fails or returns empty

| Condition | Behavior |
|-----------|----------|
| Core pool returns error on Layer 3 query | Logs error, falls through to Layer 2 query |
| Core pool returns error on Layer 2 query | Logs error, returns `""` |
| Both queries succeed but return NULL/empty | Returns `""` |
| Resolver returns `""` | Compositor uses `defaultGridColWidth` constant (150) |

### 6.3 DB pool is nil at construction time

| Condition | Behavior |
|-----------|----------|
| `db.DatabasePool` passed to `NewSQLDataSource` is nil | `NewSQLDataSource` accepts it (no nil check at construction). `Fetch`/`FetchAll` returns error when called: `"pool is nil"`. This matches the current behavior where `BusinessPool == nil` is checked at query time, not at assignment time. |
| `db.DatabasePool` passed to `NewColumnWidthResolver` is nil | `NewColumnWidthResolver` accepts it. `Resolve` returns `""` on any call (logs warning if pool is nil). |

### 6.4 Context cancelled during fetch

| Condition | Behavior |
|-----------|----------|
| `ctx.Err() != nil` at `Fetch` entry | Returns `DataSet{}`, `ctx.Err()` immediately (no pool call) |
| `ctx` cancelled between `pool.Query` call and first `rows.Next()` | `rows.Next()` returns false; `rows.Err()` returns cancellation error; `Fetch` returns `DataSet{}`, error |
| `ctx` cancelled mid-iteration | Checked between `rows.Next()` calls. Current row's values are discarded. Returns `DataSet{}`, `ctx.Err()` |

---

## 7. TDD Test Scenarios

### 7.1 Data Source Tests (unit, `pkg/dataaccess/sql_datasource_test.go`)

Uses `db.MockDBPool` to simulate `pgx.Rows` behavior.

| ID | Description | Given | When | Then | Expected Result |
|----|-------------|-------|------|------|-----------------|
| TDS-01 | Fetch with valid data returns DataSet | MockDBPool registered with `SELECT id, name FROM t` → columns `["id", "name"]`, rows `[[1, "Alice"], [2, "Bob"]]` | `ds.Fetch(ctx, "SELECT id, name FROM t")` | Returns `DataSet{Headers: ["id", "name"], Rows: [["1", "Alice"], ["2", "Bob"]]}`, nil error | Headers match; each row is string-normalized |
| TDS-02 | Fetch with args passes positional params | MockDBPool tracks query args | `ds.Fetch(ctx, "SELECT * FROM t WHERE x = $1", "hello")` | Mock receives `args = ["hello"]` | Args propagated correctly |
| TDS-03 | FetchAll delegates to Fetch with no args | MockDBPool registered | `ds.FetchAll(ctx, "SELECT * FROM t")` | Returns same result as `Fetch(ctx, "SELECT * FROM t")` | Identical DataSet |
| TDS-04 | Fetch with empty source returns error | Pool is valid | `ds.Fetch(ctx, "")` | Returns `DataSet{}`, non-nil error | Error indicates empty source |
| TDS-05 | Fetch with cancelled context returns error | `ctx, cancel := context.WithCancel(context.Background()); cancel()` | `ds.Fetch(ctx, "SELECT 1")` | Returns `DataSet{}`, `context.Canceled` | No pool query executed |
| TDS-06 | Fetch with pool error returns error | MockDBPool registered with error for query | `ds.Fetch(ctx, "SELECT 1")` | Returns `DataSet{}`, non-nil error | Error wraps pool error |
| TDS-07 | Fetch with nil pool returns error | `NewSQLDataSource(nil)` | `ds.Fetch(ctx, "SELECT 1")` | Returns `DataSet{}`, error | Error indicates nil pool |
| TDS-08 | Fetch returns empty rows for zero-result query | MockDBPool registered with columns but no rows | `ds.Fetch(ctx, "SELECT 1 WHERE false")` | Returns `DataSet{Headers: [...], Rows: [][]string{}}`, nil error | Headers present, rows empty |
| TDS-09 | Fetch normalizes pgx driver.Valuer types | MockDBPool returns `pgtype`-like values implementing `driver.Valuer` | `ds.Fetch(ctx, ...)` | All values are string via FormatValue | No `driver.Valuer` types in DataSet |
| TDS-10 | Fetch handles rows.Values error gracefully | MockDBPool rows return error on second Values() call | `ds.Fetch(ctx, ...)` | Returns partial DataSet + error | First row preserved, error logged |

### 7.2 FormatValue Tests (unit, `pkg/dataaccess/format_test.go`)

| ID | Description | Given | When | Then | Expected Result |
|----|-------------|-------|------|------|-----------------|
| TFV-01 | nil input | `val = nil` | `FormatValue(nil)` | Returns `""` | Empty string |
| TFV-02 | Go int | `val = 42` | `FormatValue(42)` | Returns `"42"` | String representation |
| TFV-03 | Go float64 | `val = 3.14` | `FormatValue(3.14)` | Returns `"3.14"` | String representation |
| TFV-04 | Go string | `val = "hello"` | `FormatValue("hello")` | Returns `"hello"` | Passthrough |
| TFV-05 | Go bool | `val = true` | `FormatValue(true)` | Returns `"true"` | String representation |
| TFV-06 | driver.Valuer with []byte | Mock Valuer returning `([]byte)("data")` | `FormatValue(mockValuer)` | Returns `"data"` | Byte-to-string conversion |
| TFV-07 | driver.Valuer with string | Mock Valuer returning `"text"` | `FormatValue(mockValuer)` | Returns `"text"` | Underlying value |
| TFV-08 | driver.Valuer returning nil | Mock Valuer returning `(nil, nil)` | `FormatValue(mockValuer)` | Returns `fmt.Sprintf("%v", mockValuer)` | Falls through |
| TFV-09 | driver.Valuer returning error | Mock Valuer returning `(nil, error)` | `FormatValue(mockValuer)` | Returns `fmt.Sprintf("%v", mockValuer)` | Falls through |

### 7.3 ExtractOrderedArgs Tests (unit, `pkg/dataaccess/extract_args_test.go`)

| ID | Description | Given | When | Then | Expected Result |
|----|-------------|-------|------|------|-----------------|
| TEA-01 | Normal extraction | `snap={"a":"1","b":"2"}`, `filterKeys=["a","b"]` | `ExtractOrderedArgs(snap, keys)` | Returns `[]any{"1","2"}` | Ordered by filterKeys |
| TEA-02 | Missing key defaults to empty string | `snap={"a":"1"}`, `filterKeys=["a","b"]` | `ExtractOrderedArgs(snap, keys)` | Returns `[]any{"1",""}` | Missing key → `""` |
| TEA-03 | Empty filterKeys | `snap={"a":"1"}`, `filterKeys=[]` | `ExtractOrderedArgs(snap, keys)` | Returns `[]any{}` | Empty non-nil slice |
| TEA-04 | Nil snapshot | `snap=nil`, `filterKeys=["a"]` | `ExtractOrderedArgs(nil, keys)` | Returns `[]any{""}` | Treats nil as empty map |
| TEA-05 | Key ordering preserved | `snap={"b":"2","a":"1","c":"3"}`, `filterKeys=["c","a","b"]` | `ExtractOrderedArgs(snap, keys)` | Returns `[]any{"3","1","2"}` | Order follows filterKeys |
| TEA-06 | Nil filterKeys | `snap={"a":"1"}`, `filterKeys=nil` | `ExtractOrderedArgs(snap, nil)` | Returns `[]any{}` | Empty non-nil slice |

### 7.4 Column Width Resolver Tests (unit, `pkg/dataaccess/column_width_resolver_test.go`)

Uses `db.MockDBPool` to simulate DB queries.

| ID | Description | Given | When | Then | Expected Result |
|----|-------------|-------|------|------|-----------------|
| TCW-01 | Layer 3 override returns width | MockDBPool registered for mapeo_interfaz query returning `"200px"` | `Resolve("transacciones_list", "status")` | Returns `"200px"` | Layer 3 value |
| TCW-02 | No Layer 3, Layer 2 default | MockDBPool: mapeo_interfaz returns 0 rows; componentes returns `"150px"` | `Resolve("any_origen", "any_col")` | Returns `"150px"` | Layer 2 default |
| TCW-03 | Neither Layer 3 nor Layer 2 | MockDBPool: both queries return 0 rows | `Resolve("x", "y")` | Returns `""` | Empty string |
| TCW-04 | Layer 3 query error falls through to Layer 2 | MockDBPool: mapeo_interfaz query returns error; componentes returns `"150px"` | `Resolve("x", "y")` | Returns `"150px"` | Error logged, Layer 2 used |
| TCW-05 | Both queries error returns empty | MockDBPool: both queries return errors | `Resolve("x", "y")` | Returns `""` | Errors logged |
| TCW-06 | Caching: second call uses cache | Same `(origen, header)` queried twice | `Resolve("x", "y")` twice | First call triggers 1-2 DB queries; second call returns cached value without DB query | Cache hit on second call |
| TCW-07 | Different origen/header bypasses cache | `(origen1, header1)` cached | `Resolve("origen2", "header2")` | Triggers fresh DB queries | No false cache hit |

### 7.5 Compositor Integration Tests (using MockDataSource)

These tests replace the current `MockDBPool`-based tests with `MockDataSource`-based tests. Each existing test maps 1:1.

| ID | Description | Given | When | Then | Expected Result |
|----|-------------|-------|------|------|-----------------|
| TCI-01 | Data grid loads data via DataSource | MockDataSource.Fetch returns `DataSet{Headers: ["id","title"], Rows: [["1","A"]]}` | Compose data_grid node with DataSource | Table shows 1 row, 2 cols with correct values | Headers and cells match DataSet |
| TCI-02 | Data grid with empty DataSource shows empty table | MockDataSource.Fetch returns `DataSet{Headers: nil, Rows: nil}` | Compose data_grid node | Table is 0×0 | No crash |
| TCI-03 | Nil DataSource does not panic | `ui.DataSource = nil` | Compose data_grid node with DataSource="SELECT 1" | Table is 0×0; no panic | Graceful nil check |
| TCI-04 | Client-mode grid calls FetchAll | MockDataSource records calls | Compose data_grid with FilterMode="client", MasterDataSource="SELECT *" | `FetchAll` called with `"SELECT *"` | Correct source string |
| TCI-05 | Server-mode grid calls Fetch with args on initial load | MockDataSource records calls | Compose data_grid with FilterMode="server", DataSource="SELECT * WHERE x=$1", FilterKeys=["x"] | `Fetch` called with source and initial args from ScreenState | Correct source + args |
| TCI-06 | Server-mode grid reacts to submit with Fetch | MockDataSource records calls; EventBus fires submit | User types in text_input, clicks submit | `Fetch` called with source and typed args | Args match typed values |
| TCI-07 | Server-mode grid without FilterKeys skips submit | MockDataSource records calls; EventBus fires submit | Submit with no filter_keys | No `Fetch` call after initial load | Guard works |
| TCI-08 | Dynamic query (`state:` prefix) resolves from state | MockDataSource records calls; EventBus fires submit | State contains `sql_query = "SELECT 1"`; DataSource is `"state:sql_query"` | `Fetch` called with `"SELECT 1"`, no args | State resolution works |
| TCI-09 | Dynamic query with empty state key skips | MockDataSource records calls | State key `sql_query` is empty | No `Fetch` call | Empty-state guard |
| TCI-10 | Column width uses ColumnWidthResolver | MockCWR returns `"200px"` for column "status" | Compose and load data_grid | `table.SetColumnWidth` called with parsed 200 | Metadata-driven width |
| TCI-11 | Column width falls back to defaultGridColWidth | MockCWR returns `""` for all columns | Compose and load data_grid | `table.SetColumnWidth` called with 150 | Fallback constant |
| TCI-12 | Row selection publishes header→value map | MockDataSource provides data; EventBus subscribed to `"publish_selection"` | Select row 0 | `map[string]any{"id": "42", "name": "Alice"}` published | Correct payload |
| TCI-13 | Out-of-bounds row selection publishes nothing | MockDataSource provides 1 row | Select row -1 or row 99 | No event published | Bounds check |
| TCI-14 | Reactive filtering with rapid submit cancels previous context | MockDataSource records context state | Rapid triple-submit | Earlier queries have cancelled contexts; last query has active context | Context cancellation |
| TCI-15 | Client-mode filter works in memory | MockDataSource.FetchAll provides master data | User types "Asimov", submits | Grid shows only matching rows; no additional Fetch calls | In-memory filter |

### 7.6 Main Wiring Tests (`cmd/golemui/main_test.go` or similar)

| ID | Description | Given | When | Then | Expected Result |
|----|-------------|-------|------|------|-----------------|
| TMW-01 | Bootstrap wires ui.DataSource | Valid config, valid DB pools | `RunBootstrap` completes | `ui.DataSource` is non-nil; type is `*dataaccess.SQLDataSource` | Data source wired |
| TMW-02 | Bootstrap wires ui.CWR | Valid config, valid DB pools | `RunBootstrap` completes | `ui.CWR` is non-nil; type is `*dataaccess.ColumnWidthResolver` | Column width resolver wired |
| TMW-03 | ui.BusinessPool and ui.CorePool globals removed from compositor | After bootstrap | `grep -E 'var (BusinessPool\|CorePool)' pkg/ui/compositor.go` | Returns 0 matches | Globals removed |
| TMW-04 | screen_loader still receives CorePool as parameter | Valid config | `LoadScreen` called during navigation | `LoadScreen(ctx, corePool, vistaID, query)` succeeds | Unchanged parameter path |

---

## 8. Package Structure After Change

```
pkg/
├── ui/
│   ├── compositor.go          — MODIFIED: removes DB imports, uses DataSource/CWR
│   ├── compositor_test.go     — MODIFIED: uses MockDataSource instead of MockDBPool
│   ├── datasource.go          — NEW: DataSource, ColumnWidthResolver, DataSet
│   ├── layout.go              — UNCHANGED
│   ├── screen_loader.go       — UNCHANGED
│   ├── screen_state.go        — UNCHANGED
│   ├── sidebar_loader.go      — UNCHANGED
│   └── sidebar_widget.go      — UNCHANGED
├── dataaccess/                — NEW PACKAGE
│   ├── sql_datasource.go      — NEW: SQLDataSource implements ui.DataSource
│   ├── sql_datasource_test.go — NEW: TDD tests for data source
│   ├── column_width_resolver.go      — NEW: ColumnWidthResolver impl
│   ├── column_width_resolver_test.go — NEW: TDD tests for resolver
│   ├── format.go              — NEW: FormatValue (moved from compositor)
│   ├── format_test.go         — NEW: TDD tests for FormatValue
│   ├── extract_args.go        — NEW: ExtractOrderedArgs (moved from compositor)
│   └── extract_args_test.go   — NEW: TDD tests for ExtractOrderedArgs
├── db/
│   ├── db.go                  — UNCHANGED
│   └── mock_db.go             — UNCHANGED (still used by dataaccess tests)
├── eventbus/
│   └── eventbus.go            — UNCHANGED
├── config/
│   └── loader.go              — UNCHANGED
cmd/
└── golemui/
    └── main.go                — MODIFIED: wires DataSource and CWR instead of pools
docker/
└── init-db/
    └── 02_init_core.sql       — MODIFIED: new columns, updated seed data
```

---

## 9. Wiring Contract (`cmd/golemui/main.go`)

### Before (current):

```go
ui.BusinessPool = dbPool.BusinessPool
ui.CorePool = dbPool.CorePool
```

### After (target):

```go
import (
    "GolemUI/pkg/dataaccess"
    "GolemUI/pkg/ui"
)

// Data source for business data queries (Layer 1 boundary)
ui.DataSource = dataaccess.NewSQLDataSource(dbPool.BusinessPool)

// Column width resolver for Layer 2/3 metadata (Core DB)
ui.CWR = dataaccess.NewColumnWidthResolver(dbPool.CorePool)
```

### `ui.CorePool` retention:

`ui.CorePool` global is retained temporarily only if `screen_loader.go` and `sidebar_loader.go` still reference it via package-level global. Since these functions already accept `db.DatabasePool` as a parameter (called from `main.go`), the global can be removed from `compositor.go` and the pool passed directly at call sites in `main.go`.

Changes in `main.go`:

```go
// Before:
menuItems, err := ui.LoadNavigationMenu(ctx, ui.CorePool)
// ...
node, err := ui.LoadScreen(ctx, ui.CorePool, vID, cfg.LayoutQuery)
// ...
homeNode, err := ui.LoadScreen(ctx, ui.CorePool, vistaID, cfg.LayoutQuery)

// After: identical — these functions already take pool as parameter
menuItems, err := ui.LoadNavigationMenu(ctx, dbPool.CorePool)
// ...
node, err := ui.LoadScreen(ctx, dbPool.CorePool, vID, cfg.LayoutQuery)
// ...
homeNode, err := ui.LoadScreen(ctx, dbPool.CorePool, vistaID, cfg.LayoutQuery)
```

The `ui.CorePool` global is removed from `compositor.go` entirely. It was only used in `compositor.go` as a package-level var; `screen_loader.go` and `sidebar_loader.go` receive the pool via function parameter, not via the global.

---

## 10. Compositor Internal Changes Detail

### 10.1 Package-level globals (compositor.go)

**Before:**
```go
var BusinessPool db.DatabasePool
var CorePool db.DatabasePool
var LocalEventBus eventbus.EventBus
var Navigate func(vistaID string)
```

**After:**
```go
var DS DataSource       // replaces BusinessPool
var CWR ColumnWidthResolver  // replaces CorePool (for column width only)
var LocalEventBus eventbus.EventBus
var Navigate func(vistaID string)

const defaultGridColWidth float32 = 150
```

### 10.2 `loadMasterBuffer` changes

**Removed:** `pool := BusinessPool`, `pool == nil` check, `pool.Query(ctx, ...)`, `rows.FieldDescriptions()`, `rows.Values()`, `formatValue(val)`.

**Added:** `DS.FetchAll(ctx, node.MasterDataSource)` call; model update from returned `DataSet`; column width resolution via `resolveWidth`.

### 10.3 `fetchGridDataAsync` changes

**Removed:** `pool := BusinessPool`, `pool == nil` check, `pool.Query(ctx, ...)`, `rows.FieldDescriptions()`, `rows.Values()`, `formatValue(val)`.

**Added:** `DS.Fetch(ctx, query, args...)` call; model update from returned `DataSet`; column width resolution via `resolveWidth`.

### 10.4 `resolveWidth` helper (new)

```go
func resolveWidth(columnWidths []string, colIndex int, header string, origen string) float32 {
    // Step 1: inline hint from DataSet
    if colIndex < len(columnWidths) && columnWidths[colIndex] != "" {
        spec := parseMetric(columnWidths[colIndex])
        if spec.mType == metricFixed {
            return spec.value
        }
    }
    // Step 2: metadata resolver
    if CWR != nil {
        resolved := CWR.Resolve(origen, header)
        if resolved != "" {
            spec := parseMetric(resolved)
            if spec.mType == metricFixed {
                return spec.value
            }
        }
    }
    // Step 3: fallback
    return defaultGridColWidth
}
```

### 10.5 EventBus subscriber changes (compositor.go:255-272)

The `state:` prefix resolution stays in the compositor (Layer 4 concern — reading `ScreenState`). After resolution, the compositor calls `DS.Fetch(subCtx, resolvedQuery)` with no args.

### 10.6 `extractOrderedArgs` call site

The compositor calls `dataaccess.ExtractOrderedArgs(snap, node.FilterKeys)` — the function is exported from the new package.

### 10.7 Deleted functions

- `formatValue` — moved to `pkg/dataaccess/format.go` as `FormatValue`
- `extractOrderedArgs` — moved to `pkg/dataaccess/extract_args.go` as `ExtractOrderedArgs`

---

## 11. Assumptions

1. **Single-column-width metric type:** Column widths from metadata are always `px`-based (`"150px"`, `"200px"`). The `parseMetric` function from `layout.go` supports `fr` and `auto` too, but the initial implementation only seeds `px` values. The `resolveWidth` function handles all metric types but defaults to `metricFixed` for `SetColumnWidth`.

2. **`origen` parameter equals `node.DataSource` / `node.MasterDataSource`:** The `origen_datos` field in `golemui.vistas_consulta` stores the same SQL string that appears in `NodeMeta.DataSource` / `NodeMeta.MasterDataSource`. This string is used as the `origen` key when querying `golemui.mapeo_interfaz`. This is a valid assumption because the current seed data confirms the match.

3. **`ColumnWidthResolver` cache does not need runtime invalidation:** Column width overrides change only when the ephemeral DB is rebuilt (container restart). The `sync.Map` cache in the resolver is populated on first access per `(origen, header)` and never invalidated during the application lifetime.

4. **`DataSet.ColumnWidths` is nil by default:** The `SQLDataSource` does NOT populate `ColumnWidths` in the returned `DataSet`. Column width resolution happens in the compositor via `ColumnWidthResolver`. This keeps the data source clean (Layer 1 concern only) and the width resolution in the compositor (Layer 4 → Layer 2/3). `DataSet.ColumnWidths` exists as an extension point for future data sources that provide inline width hints.

5. **`screen_loader.go` and `sidebar_loader.go` do not need `DataSource`:** These functions read layout metadata from the core DB, not business data. They continue to accept `db.DatabasePool` as a parameter.

6. **The `query_runner` use case (user types raw SQL) survives:** The dynamic query path (`state:` prefix) resolves the SQL string in the compositor and passes it to `DataSource.Fetch`. The data source executes whatever SQL it receives. The injection risk is the same as the current behavior — the user is explicitly typing SQL in a console. This is by design.

7. **`go test ./...` is the canonical test runner:** Per `openspec/config.yaml`, all tests must pass under this command.

8. **`dataGridModel` internal representation stays `[][]string`:** Native type preservation (`[][]any`) is explicitly out of scope for this change.

---

## 12. Out of Scope

1. **Refactoring package-level globals into a struct.** `DataSource` and `CWR` are package-level globals in this change. Converting `compositor.go` from package-level functions to a struct receiver is a separate, larger refactor.

2. **Changing the `publish_selection` event contract.** The `map[string]any` payload shape stays the same.

3. **Changing `pkg/db/db.go` interfaces.** The `DatabasePool`/`DBQuerier` interfaces remain the driver-level abstraction. The new `DataSource` sits above them.

4. **Native type preservation in data-grid model.** The `[][]string` internal storage stays. Changing to `[][]any` is a separate change.

5. **Thread-safety fixes from audit §1.** Concurrent UI updates from background goroutines (audit §1.1, §1.2, §1.3) are separate SDD changes.

6. **Memory-leak fixes from audit §2.** Concurrently overwritten cleanup closures and zombie screens (audit §2.1, §2.2) are separate SDD changes.

7. **Global mutable state refactoring from audit §4.1.** Converting all package-level globals to a struct is out of scope beyond removing `BusinessPool` and `CorePool` from the compositor.

8. **Sidebar re-entrancy guard from audit §4.2.** The `nt.navigating` data race is a separate fix.

9. **DB pool cleanup on shutdown from audit §4.3.** Graceful pool closing on window close is out of scope.

10. **Adding `ColumnWidths` population in `SQLDataSource`.** The `DataSet.ColumnWidths` field is an extension point. The initial implementation leaves it nil and relies on `ColumnWidthResolver` for all width resolution.

11. **Integration tests against the ephemeral PostgreSQL database.** All tests in this change are unit tests using mock pools and mock data sources. End-to-end integration tests against the real DB are a future concern.

12. **Modifying `NodeMeta` to carry column width metadata.** Column width is resolved from Layer 2/3 at render time, not embedded in the layout JSONB.

13. **Parameterized query building for the `query_runner` use case.** The user-typed SQL is executed as-is. Adding a query builder or parameterization layer is out of scope.

14. **Adding `golemui.mapeo_interfaz` seed data with column width overrides.** The schema supports it, but no seed overrides are provided. Developers add overrides via SQL statements.

---

## 13. TDD Execution Order

Following strict TDD per `openspec/config.yaml`:

### Phase 1: RED — New package tests (write first, no implementation)

1. `pkg/dataaccess/format_test.go` — TFV-01 through TFV-09
2. `pkg/dataaccess/extract_args_test.go` — TEA-01 through TEA-06
3. `pkg/dataaccess/sql_datasource_test.go` — TDS-01 through TDS-10
4. `pkg/dataaccess/column_width_resolver_test.go` — TCW-01 through TCW-07
5. `pkg/ui/datasource.go` — interface and struct definitions (compilation only)
6. Updated `pkg/ui/compositor_test.go` — TCI-01 through TCI-15

### Phase 2: GREEN — Implementation

1. `pkg/dataaccess/format.go` — pass TFV tests
2. `pkg/dataaccess/extract_args.go` — pass TEA tests
3. `pkg/dataaccess/sql_datasource.go` — pass TDS tests
4. `pkg/dataaccess/column_width_resolver.go` — pass TCW tests
5. `pkg/ui/compositor.go` — pass TCI tests
6. `cmd/golemui/main.go` — wire DataSource and CWR
7. `docker/init-db/02_init_core.sql` — schema changes

### Phase 3: REFACTOR

1. Remove dead code from compositor
2. Clean up any remaining `CorePool` references
3. Verify all tests still pass
4. Lint and format

---

## 14. Success Criteria (Binary Validation)

| # | Criterion | Validation Command |
|---|-----------|-------------------|
| 1 | No `database/sql/driver` in `pkg/ui/` | `grep -r 'database/sql/driver' pkg/ui/` → exit 0 |
| 2 | No `db.DatabasePool` in compositor | `grep 'GolemUI/pkg/db' pkg/ui/compositor.go` → exit 1 |
| 3 | No pool globals in compositor | `grep -E 'var (BusinessPool\|CorePool)' pkg/ui/compositor.go` → exit 1 |
| 4 | No hardcoded 150 literal | `grep -n 'SetColumnWidth(i, 150)' pkg/ui/compositor.go` → exit 1 |
| 5 | Named fallback constant exists | `grep 'defaultGridColWidth' pkg/ui/compositor.go` → 1 match |
| 6 | All tests pass | `go test ./...` → exit 0 |
| 7 | Build succeeds | `go build ./...` → exit 0 |
| 8 | DataSource interface exists | `grep 'type DataSource interface' pkg/ui/datasource.go` → 1 match |
| 9 | ColumnWidthResolver interface exists | `grep 'type ColumnWidthResolver interface' pkg/ui/datasource.go` → 1 match |
| 10 | `dataaccess` package compiles | `go build ./pkg/dataaccess/` → exit 0 |
| 11 | Schema changes in init script | `grep 'default_column_width' docker/init-db/02_init_core.sql` → exit 0 |
| 12 | Schema changes in init script | `grep 'column_width' docker/init-db/02_init_core.sql` → exit 0 |
