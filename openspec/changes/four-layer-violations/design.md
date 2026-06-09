# Design: four-layer-violations

| Field        | Value                              |
|-------------|-------------------------------------|
| Change      | `four-layer-violations`             |
| Date        | 2026-06-07                          |
| Status      | DRAFT                               |
| Dependencies| `openspec/config.yaml` (strict TDD) |
| Spec        | `openspec/changes/four-layer-violations/spec.md` |
| Proposal    | `openspec/changes/four-layer-violations/proposal.md` |
| Explore     | `openspec/changes/four-layer-violations/explore.md` |

---

## 1. Package Layout

| Path | Status | Purpose |
|------|--------|---------|
| `pkg/ui/datasource.go` | **NEW** | `DataSource`, `ColumnWidthResolver` interfaces; `DataSet` struct |
| `pkg/ui/compositor.go` | **MODIFIED** | Remove DB imports/pools, use `DataSource`/`CWR` globals, add `resolveWidth`, add `defaultGridColWidth` constant |
| `pkg/ui/compositor_test.go` | **MODIFIED** | Replace `MockDBPool` with `MockDataSource`/`MockCWR`; migrate ~20 tests |
| `pkg/dataaccess/sql_datasource.go` | **NEW** | `SQLDataSource` struct implements `ui.DataSource` |
| `pkg/dataaccess/sql_datasource_test.go` | **NEW** | Unit tests using `db.MockDBPool` |
| `pkg/dataaccess/column_width_resolver.go` | **NEW** | `ColumnWidthResolver` implementation with `sync.Map` cache |
| `pkg/dataaccess/column_width_resolver_test.go` | **NEW** | Unit tests using `db.MockDBPool` |
| `pkg/dataaccess/format.go` | **NEW** | `FormatValue` — moved from `compositor.formatValue` |
| `pkg/dataaccess/format_test.go` | **NEW** | Unit tests for `FormatValue` |
| `pkg/dataaccess/extract_args.go` | **NEW** | `ExtractOrderedArgs` — moved from `compositor.extractOrderedArgs` |
| `pkg/dataaccess/extract_args_test.go` | **NEW** | Unit tests for `ExtractOrderedArgs` |
| `pkg/ui/layout.go` | UNCHANGED | `parseMetric` reused by `resolveWidth` |
| `pkg/ui/screen_loader.go` | UNCHANGED | Accepts `db.DatabasePool` parameter |
| `pkg/ui/screen_state.go` | UNCHANGED | No DB access |
| `pkg/ui/sidebar_loader.go` | UNCHANGED | Accepts `db.DatabasePool` parameter |
| `pkg/ui/sidebar_widget.go` | UNCHANGED | No DB access |
| `pkg/db/db.go` | UNCHANGED | Interfaces untouched |
| `pkg/db/mock_db.go` | UNCHANGED | Reused by `dataaccess` package tests |
| `pkg/eventbus/eventbus.go` | UNCHANGED | No DB access |
| `pkg/config/loader.go` | UNCHANGED | YAML config only |
| `cmd/golemui/main.go` | **MODIFIED** | Wire `DataSource` and `CWR`; pass `dbPool.CorePool` directly to loaders |
| `docker/init-db/02_init_core.sql` | **MODIFIED** | Add columns + update seed data |

---

## 2. Interface Definitions

### 2.1 `pkg/ui/datasource.go`

```go
package ui

import "context"

// DataSet is the clean data contract returned by DataSource.
// Layer 4 receives headers + string rows + optional column-width hints.
// No database driver types leak through.
type DataSet struct {
	// Headers contains the column names from the query result.
	// Length determines the number of columns.
	Headers []string

	// Rows contains the string-normalized cell values.
	// Each inner slice has the same length as Headers.
	Rows [][]string

	// ColumnWidths contains per-column width hints from metadata.
	// Each entry corresponds to Headers[i] by index.
	// Values follow the metric convention: "150px", "1fr", "auto", or "".
	// Nil or empty string means "use fallback resolution".
	ColumnWidths []string
}

// DataSource replaces direct BusinessPool access in the compositor.
// Implementations own the pool, SQL execution, driver-value normalization
// (FormatValue), and argument extraction (ExtractOrderedArgs).
// The renderer never sees database driver types or SQL strings.
type DataSource interface {
	// Fetch executes a data-source query with positional arguments.
	//
	// Parameters:
	//   ctx    - context for cancellation and timeout
	//   source - the logical data source string from NodeMeta
	//   args   - positional query arguments in filter-keys order
	//
	// Returns:
	//   DataSet - headers + string-normalized rows
	//   error   - non-nil if the query fails or context is cancelled
	Fetch(ctx context.Context, source string, args ...any) (DataSet, error)

	// FetchAll loads all data without filter arguments (client-mode master buffer).
	// Equivalent to Fetch(ctx, source) with no args.
	FetchAll(ctx context.Context, source string) (DataSet, error)
}

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
	// Returns "" on any DB error (logs the error, does not propagate).
	Resolve(origen string, header string) string
}

// MockDataSource is a test double for compositor tests.
// It records calls and returns canned DataSet values.
type MockDataSource struct {
	FetchCalled    bool
	FetchSource    string
	FetchArgs      []any
	FetchResult    DataSet
	FetchError     error

	FetchAllCalled bool
	FetchAllSource string
	FetchAllResult DataSet
	FetchAllError  error
}

func (m *MockDataSource) Fetch(_ context.Context, source string, args ...any) (DataSet, error) {
	m.FetchCalled = true
	m.FetchSource = source
	m.FetchArgs = args
	return m.FetchResult, m.FetchError
}

func (m *MockDataSource) FetchAll(_ context.Context, source string) (DataSet, error) {
	m.FetchAllCalled = true
	m.FetchAllSource = source
	return m.FetchAllResult, m.FetchAllError
}

// MockCWR is a test double for ColumnWidthResolver in compositor tests.
type MockCWR struct {
	ResolveFunc func(origen, header string) string
}

func (m *MockCWR) Resolve(origen, header string) string {
	if m.ResolveFunc != nil {
		return m.ResolveFunc(origen, header)
	}
	return ""
}
```

### 2.2 `pkg/dataaccess/sql_datasource.go`

```go
package dataaccess

import (
	"context"
	"fmt"
	"log"

	"GolemUI/pkg/db"
	"GolemUI/pkg/ui"
)

// Compile-time interface check.
var _ ui.DataSource = (*SQLDataSource)(nil)

// SQLDataSource implements ui.DataSource using a single db.DatabasePool.
// It owns SQL execution, driver-value normalization, and result building.
type SQLDataSource struct {
	pool db.DatabasePool
}

// NewSQLDataSource creates a DataSource backed by the given pool.
// The pool may be nil; Fetch/FetchAll will return an error in that case.
func NewSQLDataSource(pool db.DatabasePool) *SQLDataSource {
	return &SQLDataSource{pool: pool}
}

// Fetch executes a data-source query with positional arguments.
func (s *SQLDataSource) Fetch(ctx context.Context, source string, args ...any) (ui.DataSet, error) {
	if source == "" {
		return ui.DataSet{}, fmt.Errorf("dataaccess: empty source")
	}
	if s.pool == nil {
		return ui.DataSet{}, fmt.Errorf("dataaccess: pool is nil")
	}
	if err := ctx.Err(); err != nil {
		return ui.DataSet{}, fmt.Errorf("dataaccess: context cancelled before query: %w", err)
	}

	rows, err := s.pool.Query(ctx, source, args...)
	if err != nil {
		return ui.DataSet{}, fmt.Errorf("dataaccess: query failed: %w", err)
	}
	defer rows.Close()

	fds := rows.FieldDescriptions()
	headers := make([]string, len(fds))
	for i, fd := range fds {
		headers[i] = fd.Name
	}

	var dataRows [][]string
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			log.Printf("[DataAccess] Context cancelled during row scan: %v", err)
			return ui.DataSet{}, fmt.Errorf("dataaccess: context cancelled during scan: %w", err)
		}
		vals, err := rows.Values()
		if err != nil {
			log.Printf("[DataAccess] Error scanning row values: %v", err)
			break
		}
		stringRow := make([]string, len(vals))
		for i, val := range vals {
			stringRow[i] = FormatValue(val)
		}
		dataRows = append(dataRows, stringRow)
	}

	log.Printf("[DataAccess] Query successful. Loaded %d columns, %d rows.", len(headers), len(dataRows))
	return ui.DataSet{Headers: headers, Rows: dataRows}, nil
}

// FetchAll loads all data without filter arguments.
// Delegates to Fetch with no args.
func (s *SQLDataSource) FetchAll(ctx context.Context, source string) (ui.DataSet, error) {
	return s.Fetch(ctx, source)
}
```

### 2.3 `pkg/dataaccess/column_width_resolver.go`

```go
package dataaccess

import (
	"context"
	"log"
	"sync"

	"GolemUI/pkg/ui"
)

// Compile-time interface check.
var _ ui.ColumnWidthResolver = (*ColumnWidthResolver)(nil)

// ColumnWidthResolver reads column-width metadata from Layer 2/3
// using the core database pool.
type ColumnWidthResolver struct {
	pool  db.DatabasePool
	cache sync.Map // key: "origen|header" → value: string
}

// NewColumnWidthResolver creates a resolver backed by the core pool.
// The pool may be nil; Resolve will return "" for all calls.
func NewColumnWidthResolver(pool db.DatabasePool) *ColumnWidthResolver {
	return &ColumnWidthResolver{pool: pool}
}

// cacheKey builds a composite key for the sync.Map cache.
func cacheKey(origen, header string) string {
	return origen + "|" + header
}

// Resolve returns the effective width string for a column.
// Resolution order: Layer 3 override → Layer 2 default → "".
func (r *ColumnWidthResolver) Resolve(origen, header string) string {
	key := cacheKey(origen, header)

	// Check cache first
	if cached, ok := r.cache.Load(key); ok {
		return cached.(string)
	}

	ctx := context.Background()
	result := ""

	// Step 1: Layer 3 — golemui.mapeo_interfaz.column_width
	if r.pool != nil {
		var cw *string
		err := r.pool.QueryRow(ctx,
			"SELECT column_width FROM golemui.mapeo_interfaz WHERE origen_id = $1 AND columna_fisica = $2",
			origen, header,
		).Scan(&cw)
		if err == nil && cw != nil && *cw != "" {
			result = *cw
		}
		// Log errors from Layer 3 but fall through to Layer 2
	}

	// Step 2: Layer 2 — golemui.componentes.default_column_width
	if result == "" && r.pool != nil {
		var dcw *string
		err := r.pool.QueryRow(ctx,
			"SELECT default_column_width FROM golemui.componentes WHERE id = 'data_grid'",
		).Scan(&dcw)
		if err == nil && dcw != nil && *dcw != "" {
			result = *dcw
		}
		if err != nil {
			log.Printf("[ColumnWidthResolver] Layer 2 lookup error: %v", err)
		}
	}

	// Cache the result (including empty string)
	r.cache.Store(key, result)
	return result
}
```

**Note:** The `db.DatabasePool` import path is `GolemUI/pkg/db`. The full import block:

```go
import (
	"context"
	"log"
	"sync"

	"GolemUI/pkg/db"
	"GolemUI/pkg/ui"
)
```

### 2.4 `pkg/dataaccess/format.go`

```go
package dataaccess

import (
	"database/sql/driver"
	"fmt"
)

// FormatValue normalizes a database driver value to a string.
//
// Parameters:
//   val - a value from pgx.Rows.Values(), which may be nil, a Go primitive,
//         or a pgx-specific type implementing driver.Valuer
//
// Returns:
//   string - the string representation of the value
//
// Behavior:
//   - val == nil → returns ""
//   - val implements driver.Valuer → calls .Value(), then:
//     - if underlying value is []byte → string(bytes)
//     - otherwise → fmt.Sprintf("%v", underlying)
//   - val does not implement driver.Valuer → fmt.Sprintf("%v", val)
func FormatValue(val any) string {
	if val == nil {
		return ""
	}
	if valuer, ok := val.(driver.Valuer); ok {
		v, err := valuer.Value()
		if err == nil && v != nil {
			switch ts := v.(type) {
			case []byte:
				return string(ts)
			default:
				return fmt.Sprintf("%v", v)
			}
		}
	}
	return fmt.Sprintf("%v", val)
}
```

### 2.5 `pkg/dataaccess/extract_args.go`

```go
package dataaccess

// ExtractOrderedArgs maps snapshot keys to positional args in filterKeys order.
// Missing keys default to empty string (so LIKE '' matches everything instead of NULL = false).
//
// Parameters:
//   snap       - a key-value snapshot (typically from ScreenState.Snapshot())
//   filterKeys - ordered list of keys to extract from the snapshot
//
// Returns:
//   []any - positional arguments in filterKeys order.
//           Empty (non-nil) slice when filterKeys is empty.
//
// Behavior:
//   - filterKeys empty → returns []any{} (empty slice, not nil)
//   - key present in snap → appends snap[key]
//   - key missing from snap → appends "" (empty string)
//   - snap is nil → treats as empty map; all keys get ""
func ExtractOrderedArgs(snap map[string]any, filterKeys []string) []any {
	if len(filterKeys) == 0 {
		return []any{}
	}
	args := make([]any, 0, len(filterKeys))
	for _, key := range filterKeys {
		if snap == nil {
			args = append(args, "")
			continue
		}
		val, exists := snap[key]
		if !exists {
			args = append(args, "")
		} else {
			args = append(args, val)
		}
	}
	return args
}
```

---

## 3. Compositor Changes

### 3.1 Package-level globals — before/after

**BEFORE** (`pkg/ui/compositor.go:17-20`):
```go
var BusinessPool db.DatabasePool
var CorePool db.DatabasePool
var LocalEventBus eventbus.EventBus
var Navigate func(vistaID string)
```

**AFTER**:
```go
var DS DataSource
var CWR ColumnWidthResolver
var LocalEventBus eventbus.EventBus
var Navigate func(vistaID string)

const defaultGridColWidth float32 = 150
```

### 3.2 Import block — before/after

**BEFORE** (`pkg/ui/compositor.go:7-16`):
```go
import (
	"context"
	"database/sql/driver"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"

	"GolemUI/pkg/db"
	"GolemUI/pkg/eventbus"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)
```

**AFTER**:
```go
import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"

	"GolemUI/pkg/dataaccess"
	"GolemUI/pkg/eventbus"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)
```

Key removals: `"database/sql/driver"`, `"GolemUI/pkg/db"`.
Key addition: `"GolemUI/pkg/dataaccess"` (for `dataaccess.ExtractOrderedArgs`).

### 3.3 `loadMasterBuffer` — before/after

**BEFORE** (`pkg/ui/compositor.go:351-414`):

```go
func loadMasterBuffer(ctx context.Context, node NodeMeta, model *dataGridModel, table *widget.Table) {
	pool := BusinessPool
	if pool == nil {
		log.Printf("[UI/DataGrid] Warning: BusinessPool is nil; cannot load master buffer for data_grid at area %q", node.Area)
		return
	}
	log.Printf("[UI/DataGrid] Requesting master buffer eagerly for area %q. SQL: %q", node.Area, node.MasterDataSource)
	go func() {
		if err := ctx.Err(); err != nil {
			log.Printf("[UI/DataGrid] Master buffer load cancelled before start for area %q", node.Area)
			return
		}
		rows, err := pool.Query(ctx, node.MasterDataSource)
		if err != nil { ... return }
		defer rows.Close()

		fds := rows.FieldDescriptions()
		var headers []string
		for _, fd := range fds {
			headers = append(headers, fd.Name)
		}

		var dataRows [][]string
		for rows.Next() {
			... vals, err := rows.Values()
			... stringRow = formatValue(val)
		}

		model.mu.Lock()
		model.masterHeaders = headers
		model.masterRows = dataRows
		model.headers = headers
		model.rows = dataRows
		model.mu.Unlock()

		model.refreshMu.Lock()
		for i := 0; i < len(headers); i++ {
			table.SetColumnWidth(i, 150)           // ← hardcoded
		}
		table.Refresh()
		model.refreshMu.Unlock()
	}()
}
```

**AFTER** (complete new implementation):

```go
func loadMasterBuffer(ctx context.Context, node NodeMeta, model *dataGridModel, table *widget.Table) {
	if DS == nil {
		log.Printf("[UI/DataGrid] Warning: DataSource is nil; cannot load master buffer for data_grid at area %q", node.Area)
		return
	}
	log.Printf("[UI/DataGrid] Requesting master buffer eagerly for area %q. Source: %q", node.Area, node.MasterDataSource)
	go func() {
		if err := ctx.Err(); err != nil {
			log.Printf("[UI/DataGrid] Master buffer load cancelled before start for area %q", node.Area)
			return
		}
		ds, err := DS.FetchAll(ctx, node.MasterDataSource)
		if err != nil {
			log.Printf("[UI/DataGrid] Error loading master buffer %q: %v", node.MasterDataSource, err)
			return
		}

		if err := ctx.Err(); err != nil {
			log.Printf("[UI/DataGrid] Master buffer load cancelled before model write for area %q", node.Area)
			return
		}

		log.Printf("[UI/DataGrid] Master buffer execution successful for area %q. Loaded %d columns, %d rows.", node.Area, len(ds.Headers), len(ds.Rows))

		model.mu.Lock()
		model.masterHeaders = ds.Headers
		model.masterRows = ds.Rows
		model.headers = ds.Headers
		model.rows = ds.Rows
		model.mu.Unlock()

		model.refreshMu.Lock()
		for i, h := range ds.Headers {
			w := resolveWidth(ds.ColumnWidths, i, h, node.MasterDataSource)
			table.SetColumnWidth(i, w)
		}
		table.Refresh()
		model.refreshMu.Unlock()
	}()
}
```

**Changes summary:**
- `pool := BusinessPool` → `DS == nil` check
- `pool.Query(ctx, node.MasterDataSource)` → `DS.FetchAll(ctx, node.MasterDataSource)`
- Manual row iteration + `formatValue` → DataSet returned directly
- `table.SetColumnWidth(i, 150)` → `resolveWidth(...)` per column

### 3.4 `fetchGridDataAsync` — before/after

**BEFORE** (`pkg/ui/compositor.go:518-585`):

```go
func fetchGridDataAsync(ctx context.Context, node NodeMeta, model *dataGridModel, table *widget.Table, query string, args ...any) {
	if query == "" { return }
	pool := BusinessPool
	log.Printf("[UI/DataGrid] Requesting data async for area %q. SQL: %q, Args: %+v", node.Area, query, args)
	go func() {
		if pool == nil { ... return }
		if err := ctx.Err(); err != nil { ... return }
		rows, err := pool.Query(ctx, query, args...)
		if err != nil { ... return }
		defer rows.Close()

		... iterate rows, formatValue ...

		model.mu.Lock()
		model.headers = headers
		model.columns = headers
		model.rows = dataRows
		model.mu.Unlock()

		model.refreshMu.Lock()
		for i := 0; i < len(headers); i++ {
			table.SetColumnWidth(i, 150)           // ← hardcoded
		}
		table.Refresh()
		model.refreshMu.Unlock()
	}()
}
```

**AFTER** (complete new implementation):

```go
func fetchGridDataAsync(ctx context.Context, node NodeMeta, model *dataGridModel, table *widget.Table, query string, args ...any) {
	if query == "" {
		return
	}
	if DS == nil {
		log.Printf("[UI/DataGrid] Warning: DataSource is nil; cannot execute query for data_grid at area %q", node.Area)
		return
	}
	log.Printf("[UI/DataGrid] Requesting data async for area %q. Source: %q, Args: %+v", node.Area, query, args)
	go func() {
		if err := ctx.Err(); err != nil {
			log.Printf("[UI/DataGrid] Query cancelled before start for area %q", node.Area)
			return
		}
		ds, err := DS.Fetch(ctx, query, args...)
		if err != nil {
			log.Printf("[UI/DataGrid] Error running query %q: %v", query, err)
			return
		}

		if err := ctx.Err(); err != nil {
			log.Printf("[UI/DataGrid] Query cancelled before model write for area %q", node.Area)
			return
		}

		log.Printf("[UI/DataGrid] Query execution successful for area %q. Loaded %d columns, %d rows.", node.Area, len(ds.Headers), len(ds.Rows))

		model.mu.Lock()
		model.headers = ds.Headers
		model.columns = ds.Headers
		model.rows = ds.Rows
		model.mu.Unlock()

		model.refreshMu.Lock()
		for i, h := range ds.Headers {
			w := resolveWidth(ds.ColumnWidths, i, h, node.DataSource)
			table.SetColumnWidth(i, w)
		}
		table.Refresh()
		model.refreshMu.Unlock()
	}()
}
```

**Changes summary:**
- `pool := BusinessPool` → `DS == nil` check
- `pool.Query(ctx, query, args...)` → `DS.Fetch(ctx, query, args...)`
- Manual row iteration → DataSet returned directly
- `table.SetColumnWidth(i, 150)` → `resolveWidth(...)` per column

### 3.5 `resolveWidth` — new helper (full implementation)

```go
// resolveWidth determines the pixel width for a data grid column.
// Resolution order:
//   1. Inline hint from DataSet.ColumnWidths[colIndex]
//   2. ColumnWidthResolver (Layer 3 → Layer 2) via the CWR global
//   3. Fallback to defaultGridColWidth (150)
func resolveWidth(columnWidths []string, colIndex int, header string, origen string) float32 {
	// Step 1: inline hint from DataSet
	if colIndex < len(columnWidths) && columnWidths[colIndex] != "" {
		spec := parseMetric(columnWidths[colIndex])
		if spec.mType == metricFixed {
			return spec.value
		}
	}
	// Step 2: metadata resolver (Layer 3 → Layer 2)
	if CWR != nil {
		resolved := CWR.Resolve(origen, header)
		if resolved != "" {
			spec := parseMetric(resolved)
			if spec.mType == metricFixed {
				return spec.value
			}
		}
	}
	// Step 3: fallback constant
	return defaultGridColWidth
}
```

Uses `parseMetric` from `pkg/ui/layout.go` (same package, no import needed).

### 3.6 EventBus subscriber — changes needed

The `state:` prefix resolution stays in the compositor (Layer 4 concern — reading `ScreenState`). The only change is replacing `extractOrderedArgs` with `dataaccess.ExtractOrderedArgs`:

**BEFORE** (inside the `LocalEventBus.Subscribe` callback, compositor.go:278):
```go
args = extractOrderedArgs(snap, node.FilterKeys)
```

**AFTER**:
```go
args = dataaccess.ExtractOrderedArgs(snap, node.FilterKeys)
```

Everything else in the subscriber remains identical. The `state:` prefix resolution logic (`strings.HasPrefix(query, "state:")` → `query = fmt.Sprintf("%v", qVal)`) stays because it reads `ScreenState.Snapshot()` which is a Layer 4 concern.

### 3.7 Initial data load in `data_grid` case — change

**BEFORE** (compositor.go:223-224):
```go
if !strings.HasPrefix(node.DataSource, "state:") {
    args := extractOrderedArgs(state.Snapshot(), node.FilterKeys)
    fetchGridDataAsync(ctx, node, model, table, node.DataSource, args...)
}
```

**AFTER**:
```go
if !strings.HasPrefix(node.DataSource, "state:") {
    args := dataaccess.ExtractOrderedArgs(state.Snapshot(), node.FilterKeys)
    fetchGridDataAsync(ctx, node, model, table, node.DataSource, args...)
}
```

### 3.8 Deleted functions

Two functions are removed from `compositor.go`:

1. **`extractOrderedArgs`** (current lines 316-340) — moved to `pkg/dataaccess/extract_args.go` as `ExtractOrderedArgs`
2. **`formatValue`** (current lines 587-603) — moved to `pkg/dataaccess/format.go` as `FormatValue`

---

## 4. Database Schema — DDL Changes

### 4.1 `golemui.componentes` — full modified section

**BEFORE** (`docker/init-db/02_init_core.sql:4-25`):
```sql
CREATE TABLE IF NOT EXISTS golemui.componentes (
    id VARCHAR(50) PRIMARY KEY,
    descripcion TEXT NOT NULL
);

INSERT INTO golemui.componentes (id, descripcion) VALUES
('click_button', 'Botón de ejecución transaccional'),
('text_input', 'Input de texto de una sola línea'),
('text_area', 'Input de texto multilínea'),
('numeric_stepper', 'Selector numérico con límites definidos'),
('barcode_reader', 'Control optimizado para entrada de escáneres rápidos'),
('data_grid', 'Grilla estructurada para visualización y selección de filas'),
('dropdown_select', 'Selector de opciones basado en claves foráneas'),
('date_picker', 'Selector gráfico de fechas calendarizadas'),
('checkbox_toggle', 'Selector booleano interactivo'),
('numeric_keypad', 'Teclado numérico táctil para ingreso rápido de datos')
ON CONFLICT (id) DO NOTHING;
```

**AFTER**:
```sql
CREATE TABLE IF NOT EXISTS golemui.componentes (
    id VARCHAR(50) PRIMARY KEY,
    descripcion TEXT NOT NULL,
    default_column_width VARCHAR(20)  -- Layer 2 default width for grid components
);

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

### 4.2 `golemui.mapeo_interfaz` — full modified section

**BEFORE** (`docker/init-db/02_init_core.sql:46-52`):
```sql
CREATE TABLE IF NOT EXISTS golemui.mapeo_interfaz (
    origen_id VARCHAR(100) NOT NULL,
    columna_fisica VARCHAR(100) NOT NULL,
    component_ref VARCHAR(50) NOT NULL,
    label VARCHAR(150),
    placeholder VARCHAR(250),
    validation VARCHAR(250),
    PRIMARY KEY (origen_id, columna_fisica)
);
```

**AFTER**:
```sql
CREATE TABLE IF NOT EXISTS golemui.mapeo_interfaz (
    origen_id VARCHAR(100) NOT NULL,
    columna_fisica VARCHAR(100) NOT NULL,
    component_ref VARCHAR(50) NOT NULL,
    label VARCHAR(150),
    placeholder VARCHAR(250),
    validation VARCHAR(250),
    column_width VARCHAR(20),  -- Layer 3 per-column width override
    PRIMARY KEY (origen_id, columna_fisica)
);
```

No seed data for `column_width` — all rows default to NULL ("use Layer 2 default").

### 4.3 No other schema changes

`golemui.estilos`, `golemui.vistas_consulta`, `golemui.sesion_borrador`, `golemui.menu_navegacion`, and `golemui.vistas_guardadas` remain unchanged.

---

## 5. Main.go Wiring — Exact Code Changes

### 5.1 Import block

**BEFORE**:
```go
import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"GolemUI/pkg/config"
	"GolemUI/pkg/db"
	"GolemUI/pkg/eventbus"
	"GolemUI/pkg/ui"
)
```

**AFTER**:
```go
import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"GolemUI/pkg/config"
	"GolemUI/pkg/dataaccess"
	"GolemUI/pkg/db"
	"GolemUI/pkg/eventbus"
	"GolemUI/pkg/ui"
)
```

Added: `"GolemUI/pkg/dataaccess"`.

### 5.2 Pool wiring — before/after

**BEFORE** (`cmd/golemui/main.go:92-93`):
```go
	ui.BusinessPool = dbPool.BusinessPool
	ui.CorePool = dbPool.CorePool
```

**AFTER**:
```go
	// Wire DataSource for business data queries (Layer 1 boundary)
	ui.DS = dataaccess.NewSQLDataSource(dbPool.BusinessPool)

	// Wire ColumnWidthResolver for Layer 2/3 metadata
	ui.CWR = dataaccess.NewColumnWidthResolver(dbPool.CorePool)
```

### 5.3 CorePool call sites — before/after

The `ui.CorePool` global is removed from `compositor.go`. In `main.go`, direct pool references replace the global:

**BEFORE** (`cmd/golemui/main.go:110, 130, 140`):
```go
	menuItems, err := ui.LoadNavigationMenu(ctx, ui.CorePool)
	// ...
	node, err := ui.LoadScreen(ctx, ui.CorePool, vID, cfg.LayoutQuery)
	// ...
	homeNode, err := ui.LoadScreen(ctx, ui.CorePool, vistaID, cfg.LayoutQuery)
```

**AFTER**:
```go
	menuItems, err := ui.LoadNavigationMenu(ctx, dbPool.CorePool)
	// ...
	node, err := ui.LoadScreen(ctx, dbPool.CorePool, vID, cfg.LayoutQuery)
	// ...
	homeNode, err := ui.LoadScreen(ctx, dbPool.CorePool, vistaID, cfg.LayoutQuery)
```

No functional change — these functions already accept `db.DatabasePool` as a parameter. Only the source variable changes from `ui.CorePool` (global) to `dbPool.CorePool` (local).

### 5.4 Removed lines

- `ui.BusinessPool = dbPool.BusinessPool` — replaced by `ui.DS = ...`
- `ui.CorePool = dbPool.CorePool` — replaced by `ui.CWR = ...` and direct `dbPool.CorePool` parameter passing

---

## 6. Test Design

### 6.1 `pkg/dataaccess/format_test.go`

Test structure:

```go
package dataaccess_test

import (
	"database/sql/driver"
	"testing"

	"GolemUI/pkg/dataaccess"
)

// mockValuer implements driver.Valuer for testing.
type mockValuer struct {
	val any
	err error
}

func (m *mockValuer) Value() (driver.Value, error) {
	return m.val, m.err
}

var _ driver.Valuer = (*mockValuer)(nil)
```

| Case | Input | Expected |
|------|-------|----------|
| nil input | `FormatValue(nil)` | `""` |
| Go int | `FormatValue(42)` | `"42"` |
| Go float64 | `FormatValue(3.14)` | `"3.14"` |
| Go string | `FormatValue("hello")` | `"hello"` |
| Go bool | `FormatValue(true)` | `"true"` |
| driver.Valuer with []byte | `FormatValue(&mockValuer{val: []byte("data")})` | `"data"` |
| driver.Valuer with string | `FormatValue(&mockValuer{val: "text"})` | `"text"` |
| driver.Valuer returning nil | `FormatValue(&mockValuer{val: nil})` | `fmt.Sprintf("%v", mockValuer)` |
| driver.Valuer returning error | `FormatValue(&mockValuer{err: fmt.Errorf("fail")})` | `fmt.Sprintf("%v", mockValuer)` |

### 6.2 `pkg/dataaccess/extract_args_test.go`

| Case | snap | filterKeys | Expected |
|------|------|------------|----------|
| Normal extraction | `{"a":"1","b":"2"}` | `["a","b"]` | `[]any{"1","2"}` |
| Missing key defaults empty | `{"a":"1"}` | `["a","b"]` | `[]any{"1",""}` |
| Empty filterKeys | `{"a":"1"}` | `[]` | `[]any{}` |
| Nil snapshot | `nil` | `["a"]` | `[]any{""}` |
| Key ordering preserved | `{"b":"2","a":"1","c":"3"}` | `["c","a","b"]` | `[]any{"3","1","2"}` |
| Nil filterKeys | `{"a":"1"}` | `nil` | `[]any{}` |

### 6.3 `pkg/dataaccess/sql_datasource_test.go`

Uses `db.MockDBPool` directly (not `MockDataSource`).

```go
package dataaccess_test

import (
	"context"
	"testing"

	"GolemUI/pkg/dataaccess"
	"GolemUI/pkg/db"
)
```

| Case | Mock Setup | Call | Expected |
|------|-----------|------|----------|
| Fetch returns DataSet | `MockDBPool` with `SELECT id, name FROM t` → cols `["id","name"]`, rows `[[1,"Alice"],[2,"Bob"]]` | `ds.Fetch(ctx, "SELECT id, name FROM t")` | `DataSet{Headers:["id","name"], Rows:[["1","Alice"],["2","Bob"]]}`, nil error |
| Fetch passes args | `MockDBPool` tracks args | `ds.Fetch(ctx, "SELECT * FROM t WHERE x=$1", "hello")` | Mock receives `args=["hello"]` |
| FetchAll delegates to Fetch | Same mock setup | `ds.FetchAll(ctx, "SELECT * FROM t")` | Same result as Fetch |
| Empty source returns error | Valid pool | `ds.Fetch(ctx, "")` | `DataSet{}`, non-nil error |
| Cancelled context | `ctx` cancelled before call | `ds.Fetch(ctx, "SELECT 1")` | `DataSet{}`, context error |
| Pool error | `MockDBPool` returns error | `ds.Fetch(ctx, "SELECT 1")` | `DataSet{}`, non-nil error |
| Nil pool | `NewSQLDataSource(nil)` | `ds.Fetch(ctx, "SELECT 1")` | `DataSet{}`, error containing "pool is nil" |
| Empty rows | `MockDBPool` with cols but no rows | `ds.Fetch(ctx, "SELECT 1 WHERE false")` | `DataSet{Headers:["..."], Rows:[][]string{}}`, nil error |
| driver.Valuer normalization | Mock returns `pgtype`-like values | `ds.Fetch(ctx, ...)` | All values are strings via FormatValue |

**MockDBPool setup pattern:**

```go
mockPool := db.NewMockDBPool()
mockPool.RegisterQuery("SELECT id, name FROM t",
    []string{"id", "name"},
    [][]any{{1, "Alice"}, {2, "Bob"}},
    nil,
)
ds := dataaccess.NewSQLDataSource(mockPool)
```

### 6.4 `pkg/dataaccess/column_width_resolver_test.go`

Uses `db.MockDBPool` directly. The resolver issues two different SQL queries, so the mock must register both:

```go
mockPool := db.NewMockDBPool()
// Register Layer 3 query
mockPool.RegisterQuery(
    "SELECT column_width FROM golemui.mapeo_interfaz WHERE origen_id = $1 AND columna_fisica = $2",
    []string{"column_width"},
    [][]any{{"200px"}},
    nil,
)
// Register Layer 2 fallback query
mockPool.RegisterQuery(
    "SELECT default_column_width FROM golemui.componentes WHERE id = 'data_grid'",
    []string{"default_column_width"},
    [][]any{{"150px"}},
    nil,
)
```

**Note:** `MockDBPool.Query` matches on exact SQL string but the resolver uses `QueryRow`. Tests must use `RegisterQuery` (which is what `QueryRow` uses internally in the mock). The mock's `QueryRow` method looks up the same `queries` map.

| Case | Mock Setup | Call | Expected |
|------|-----------|------|----------|
| Layer 3 override | mapeo_interfaz returns `"200px"` | `Resolve("transacciones_list", "status")` | `"200px"` |
| Layer 2 default only | mapeo_interfaz returns 0 rows; componentes returns `"150px"` | `Resolve("any", "any")` | `"150px"` |
| Neither exists | Both return 0 rows | `Resolve("x", "y")` | `""` |
| Layer 3 error falls to Layer 2 | mapeo_interfaz returns error; componentes returns `"150px"` | `Resolve("x", "y")` | `"150px"` |
| Both error | Both return errors | `Resolve("x", "y")` | `""` |
| Caching | Same key called twice | Second call | Same result; only 1 DB round-trip verified via mock state |
| Different key | Different `(origen, header)` | `Resolve("a", "b")` then `Resolve("c", "d")` | Fresh lookups |

### 6.5 `pkg/ui/compositor_test.go` — migrated test mapping

The test file replaces `ui.BusinessPool = mockPool` with `ui.DS = mockDataSource` and `ui.CWR = mockCWR`.

**Key test infrastructure changes:**

1. Remove import `"GolemUI/pkg/db"` and `"github.com/jackc/pgx/v5"`
2. Add `MockDataSource` and `MockCWR` usage from `pkg/ui/datasource.go`
3. Remove `trackingMockDBPool` type (no longer needed at this level)
4. Add a new `trackingMockDataSource` that records calls:

```go
type trackingMockDataSource struct {
	*ui.MockDataSource
	mu           sync.Mutex
	fetchCalls   []struct{ source string; args []any }
	fetchAllCalls []string
}

func (t *trackingMockDataSource) Fetch(ctx context.Context, source string, args ...any) (ui.DataSet, error) {
	t.mu.Lock()
	t.fetchCalls = append(t.fetchCalls, struct{ source string; args []any }{source, args})
	t.mu.Unlock()
	return t.MockDataSource.Fetch(ctx, source, args...)
}

func (t *trackingMockDataSource) FetchAll(ctx context.Context, source string) (ui.DataSet, error) {
	t.mu.Lock()
	t.fetchAllCalls = append(t.fetchAllCalls, source)
	t.mu.Unlock()
	return t.MockDataSource.FetchAll(ctx, source)
}
```

**Test-by-test migration map:**

| Old Test | Old Setup | New Test | New Setup |
|----------|-----------|----------|-----------|
| `TestBusinessPoolExists` | `var pool interface{} = ui.BusinessPool` | `TestDataSourceExists` | `var ds interface{} = ui.DS` |
| `TestCorePool_DefaultsNil` | `ui.CorePool != nil` | `TestCWR_DefaultsNil` | `ui.CWR != nil` |
| `TestCompose_DataGrid_Success` | `ui.BusinessPool = mockPool; mockPool.RegisterQuery(...)` | Same name | `ui.DS = &ui.MockDataSource{FetchResult: ui.DataSet{Headers:["id","title","amount"], Rows:[["1","Book A","25.5"],["2","Book B","35"]]}}; ui.CWR = &ui.MockCWR{}` |
| `TestCompose_DataGrid_NoDataSource` | No pool setup | Same name | `ui.DS = nil` |
| `TestCompose_DataGrid_NilPool` | `ui.BusinessPool = nil` | `TestCompose_DataGrid_NilDataSource` | `ui.DS = nil` |
| `TestCompose_DataGrid_ReactiveFiltering` | `trackingPool.queriesCalled` | Same name | `trackingDS.fetchCalls` — asserts source string + args instead of SQL + args |
| `TestCompose_DataGrid_ServerMode_SubmitChannelQuery` | `trackingPool.queriesCalled` with SQL string assertion | Same name | `trackingDS.fetchCalls` — asserts `source == "SELECT * FROM books WHERE title LIKE $1 AND author = $2"` and `args == ["%Sci-fi%", "Asimov"]` |
| `TestCompose_DataGrid_ClientMode_EagerLoadAndFilter` | `trackingPool.queriesCalled` count check | Same name | `trackingDS.fetchAllCalls` — asserts `FetchAll` called with `"SELECT * FROM books"`; no additional calls after submit |
| `TestCompose_DataGrid_RowSelection_PublishesToSelectionChannel` | `ui.BusinessPool = mockPool` | Same name | `ui.DS = &ui.MockDataSource{FetchResult: ui.DataSet{Headers:["id","nombre","monto"], Rows:[["42","Transaccion Test","1000.5"]]}}` |
| `TestCompose_DataGrid_RowSelection_OutOfBounds_NoPublish` | `ui.BusinessPool = mockPool` | Same name | `ui.DS = mockDS` with 1-row DataSet |
| `TestCompose_DataGrid_RowSelection_NilEventBus_NoPanic` | `ui.BusinessPool = mockPool` | Same name | `ui.DS = mockDS` |
| `TestCompose_ServerMode_NoFilterKeys_SkipsSubmit` | `trackingPool.queriesCalled` | Same name | `trackingDS.fetchCalls` — asserts no additional `Fetch` after submit |
| `TestCompose_ClientMode_FilterMismatchColumn_LogsWarning` | `ui.BusinessPool = mockPool` | Same name | `ui.DS = mockDS` with `FetchAllResult` set to master data |
| `TestCompose_ReturnsCleanupFunc` | `ui.BusinessPool = mockPool` | Same name | `ui.DS = mockDS` |
| `TestCompose_CleanupRemovesSubscribers` | `ui.BusinessPool = mockPool` | Same name | `ui.DS = mockDS` |
| `TestCompose_CleanupCancelsGoroutines` | `ui.BusinessPool = mockPool` | Same name | `ui.DS = mockDS` with `FetchAllResult` |
| `TestCompose_IdempotentCleanup` | `ui.BusinessPool = mockPool` | Same name | `ui.DS = mockDS` |
| `TestCompose_ScopedSubmitChannel_NoCrossTalk` | No pool setup | Same name | No DS setup needed (no data_grid) |
| `TestCompose_Button_SubmitAction_PublishesSnapshot` | No pool setup | Same name | No DS setup needed |
| All non-data_grid tests | No pool setup | Same name | No DS setup needed |

**New tests to add:**

| New Test | Purpose |
|----------|---------|
| `TestCompose_DataGrid_ColumnWidthFromCWR` | `MockCWR` returns `"200px"` for column "status"; verify `table.SetColumnWidth` called with 200 |
| `TestCompose_DataGrid_ColumnWidthFallback` | `MockCWR` returns `""`; verify `table.SetColumnWidth` called with 150 (default) |
| `TestCompose_DataGrid_DynamicQueryFromState` | `state:` prefix resolution; `Fetch` called with resolved SQL string, no args |

### 6.6 `cmd/golemui/main_test.go` — wiring tests

| Case | Verification |
|------|-------------|
| Bootstrap wires `ui.DS` | After `RunBootstrap`, `ui.DS` is non-nil and is `*dataaccess.SQLDataSource` |
| Bootstrap wires `ui.CWR` | After `RunBootstrap`, `ui.CWR` is non-nil and is `*dataaccess.ColumnWidthResolver` |
| `ui.BusinessPool` removed | Grep/compile check — the global no longer exists |
| `screen_loader` still works | `LoadScreen(ctx, dbPool.CorePool, vistaID, query)` succeeds with parameter |

---

## 7. Implementation Order (Strict TDD)

### Phase 1: RED — Write tests first, no implementation

**Step 1.1:** Create `pkg/ui/datasource.go` with interface definitions only (needed for compilation):
- `DataSet` struct
- `DataSource` interface
- `ColumnWidthResolver` interface
- `MockDataSource` test double
- `MockCWR` test double

**Step 1.2:** Write `pkg/dataaccess/format_test.go` — TFV-01 through TFV-09.
Verify: `go test ./pkg/dataaccess/...` fails (package doesn't exist).

**Step 1.3:** Write `pkg/dataaccess/extract_args_test.go` — TEA-01 through TEA-06.
Verify: `go test ./pkg/dataaccess/...` fails.

**Step 1.4:** Write `pkg/dataaccess/sql_datasource_test.go` — TDS-01 through TDS-09.
Verify: `go test ./pkg/dataaccess/...` fails.

**Step 1.5:** Write `pkg/dataaccess/column_width_resolver_test.go` — TCW-01 through TCW-07.
Verify: `go test ./pkg/dataaccess/...` fails.

**Step 1.6:** Update `pkg/ui/compositor_test.go` — migrate tests to use `MockDataSource`/`MockCWR`.
Verify: `go test ./pkg/ui/...` fails (compositor still uses old globals).

### Phase 2: GREEN — Make tests pass

**Step 2.1:** Implement `pkg/dataaccess/format.go` — pass TFV tests.

**Step 2.2:** Implement `pkg/dataaccess/extract_args.go` — pass TEA tests.

**Step 2.3:** Implement `pkg/dataaccess/sql_datasource.go` — pass TDS tests.

**Step 2.4:** Implement `pkg/dataaccess/column_width_resolver.go` — pass TCW tests.

**Step 2.5:** Verify all `dataaccess` tests pass: `go test ./pkg/dataaccess/...`

**Step 2.6:** Modify `pkg/ui/compositor.go`:
- Remove `database/sql/driver` and `GolemUI/pkg/db` imports
- Add `GolemUI/pkg/dataaccess` import
- Replace globals: `BusinessPool` → `DS`, `CorePool` → removed, add `CWR`, add `defaultGridColWidth`
- Replace `loadMasterBuffer` internals
- Replace `fetchGridDataAsync` internals
- Add `resolveWidth` helper
- Replace `extractOrderedArgs` calls with `dataaccess.ExtractOrderedArgs`
- Delete `formatValue` and `extractOrderedArgs` functions

**Step 2.7:** Verify `pkg/ui` tests pass: `go test ./pkg/ui/...`

**Step 2.8:** Modify `cmd/golemui/main.go`:
- Add `dataaccess` import
- Replace `ui.BusinessPool = dbPool.BusinessPool` with `ui.DS = dataaccess.NewSQLDataSource(dbPool.BusinessPool)`
- Add `ui.CWR = dataaccess.NewColumnWidthResolver(dbPool.CorePool)`
- Replace `ui.CorePool` references with `dbPool.CorePool`
- Remove `ui.CorePool = dbPool.CorePool` line

**Step 2.9:** Update `docker/init-db/02_init_core.sql`:
- Add `default_column_width VARCHAR(20)` to `golemui.componentes`
- Update seed data for `data_grid` row with `'150px'`
- Add `column_width VARCHAR(20)` to `golemui.mapeo_interfaz`

### Phase 3: REFACTOR

**Step 3.1:** Clean up dead imports in compositor test file.

**Step 3.2:** Verify no remaining references to `BusinessPool` or `CorePool` in `pkg/ui/`:
```bash
grep -r 'BusinessPool\|CorePool' pkg/ui/
```

**Step 3.3:** Verify no `database/sql/driver` in `pkg/ui/`:
```bash
grep -r 'database/sql/driver' pkg/ui/
```

**Step 3.4:** Verify no hardcoded `150` in SetColumnWidth:
```bash
grep -n 'SetColumnWidth(i, 150)' pkg/ui/compositor.go
```

**Step 3.5:** Full test suite: `go test ./...`

**Step 3.6:** Build check: `go build ./...`

**Step 3.7:** Lint: `golangci-lint run`

**Step 3.8:** Format: `gofmt -w .`

---

## 8. Risk Areas

### 8.1 `MockDBPool.QueryRow` vs `MockDBPool.Query` for CWR tests

The `ColumnWidthResolver` uses `pool.QueryRow()` internally, but `MockDBPool.QueryRow` looks up the same `queries` map as `Query`. Tests must register SQL strings using `RegisterQuery`. If the mock's `QueryRow` doesn't match the exact SQL string (including whitespace), the resolver will get `pgx.ErrNoRows` instead of the test data. **Mitigation:** Use the exact SQL string from the resolver implementation in test registration.

### 8.2 Context cancellation timing in async tests

The current `TestCompose_DataGrid_ReactiveFiltering` relies on timing-sensitive checks for context cancellation during rapid submits. After migration to `MockDataSource`, the timing window changes because `Fetch` returns instantly (no goroutine scheduling). The test may need adjusted timeouts or a different assertion strategy. **Mitigation:** The `trackingMockDataSource` records the context state at call time; assert on context state directly rather than on timing.

### 8.3 `MockDBPool` SQL string matching is exact

The `MockDBPool.Query` method matches on the exact SQL string passed to `RegisterQuery`. If the resolver's SQL query has slightly different formatting (extra spaces, different case), the mock returns an error. **Mitigation:** Copy the exact SQL strings from the implementation into test fixtures.

### 8.4 Test ordering dependency

The compositor tests currently use shared global state (`ui.BusinessPool`). After migration, they use `ui.DS` and `ui.CWR`. Tests must reset these globals in `defer` blocks. Missing cleanup will cause cross-test contamination. **Mitigation:** Audit every test for proper cleanup of `ui.DS` and `ui.CWR` globals.

### 8.5 `FetchAll` vs `Fetch` call distinction in client-mode tests

The current tests verify `trackingPool.queriesCalled` (a flat list). After migration, client-mode calls `FetchAll` and server-mode calls `Fetch`. Tests must assert the correct method was called. **Mitigation:** The `trackingMockDataSource` tracks `fetchCalls` and `fetchAllCalls` separately.

### 8.6 `filterMasterRows` uses `model.masterHeaders` and `model.masterRows`

This function does NOT call `DataSource` — it operates on the model's in-memory master buffer. After the change, `loadMasterBuffer` populates `model.masterHeaders` and `model.masterRows` from `DataSet.Headers` and `DataSet.Rows`. `filterMasterRows` remains unchanged. **Mitigation:** Verify client-mode filter tests still pass by ensuring `FetchAll` returns the correct data that populates the master buffer.

### 8.7 `fmt.Sprintf("%v", qVal)` stays in compositor for `state:` prefix

The dynamic query resolution (`state:` prefix) uses `fmt.Sprintf("%v", qVal)` to extract the user-typed SQL from the state snapshot. This is NOT SQL interpolation — the resolved value IS the SQL string the user typed. The compositor passes it to `DataSource.Fetch(ctx, resolvedQuery)` with no args. **Mitigation:** This is by design (spec §4.3, proposal §5 Q3). No change needed, but ensure the comment in code clarifies this is state resolution, not injection.

### 8.8 `origen` parameter equals `node.DataSource` / `node.MasterDataSource`

The `resolveWidth` function passes `node.DataSource` (server-mode) or `node.MasterDataSource` (client-mode) as the `origen` parameter to `CWR.Resolve`. This matches `golemui.vistas_consulta.origen_datos`, which stores the same SQL string. **Mitigation:** Document this assumption in code comments. If the data source string ever changes format, the `origen` parameter must be updated too.

---

## 9. File Inventory Summary

| File | Action | Lines Changed (est.) |
|------|--------|---------------------|
| `pkg/ui/datasource.go` | CREATE | ~80 |
| `pkg/ui/compositor.go` | MODIFY | ~60 changed, ~40 deleted |
| `pkg/ui/compositor_test.go` | MODIFY | ~100 changed |
| `pkg/dataaccess/format.go` | CREATE | ~25 |
| `pkg/dataaccess/format_test.go` | CREATE | ~80 |
| `pkg/dataaccess/extract_args.go` | CREATE | ~25 |
| `pkg/dataaccess/extract_args_test.go` | CREATE | ~70 |
| `pkg/dataaccess/sql_datasource.go` | CREATE | ~80 |
| `pkg/dataaccess/sql_datasource_test.go` | CREATE | ~150 |
| `pkg/dataaccess/column_width_resolver.go` | CREATE | ~70 |
| `pkg/dataaccess/column_width_resolver_test.go` | CREATE | ~120 |
| `cmd/golemui/main.go` | MODIFY | ~10 changed |
| `docker/init-db/02_init_core.sql` | MODIFY | ~15 changed |
| **Total** | | ~915 lines |
