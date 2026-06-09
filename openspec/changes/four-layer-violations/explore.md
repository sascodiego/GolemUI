# SDD Explore — four-layer-violations

> Mapping the current state of `pkg/ui/compositor.go` (Layer 4) against the 4-layer decoupling model defined in `AGENTS.md`. Scope: the two audit findings about direct SQL/DB-driver awareness in the compositor and the hardcoded 150 px grid column width, plus the data flow that produces them, the abstractions that already exist, and a sketch of a clean Layer 4 boundary.

## 1. Summary of findings (TL;DR)

Both audit findings are **confirmed in the current codebase** at the exact cited line ranges:

- **Finding 1 — Direct SQL & DB-driver awareness in Layer 4**: confirmed. `pkg/ui/compositor.go` holds two package-level `db.DatabasePool` globals (`BusinessPool` L19, `CorePool` L20), imports `database/sql/driver` (L8), calls `BusinessPool.Query` with raw SQL strings in two data-grid code paths (L364 in `loadMasterBuffer`, L534 in `fetchGridDataAsync`), and type-asserts `driver.Valuer` to flatten `pgx`-typed values to strings in `formatValue` (L587–L603). The audit citations match the current line numbers exactly.
- **Finding 2 — Hardcoded 150 px grid column width**: confirmed. `table.SetColumnWidth(i, 150)` appears twice (`L411` after `loadMasterBuffer`, `L580` after `fetchGridDataAsync`) with no metadata source and no override hook.

The `pkg/db` package provides a driver abstraction (`DatabasePool`, `DBQuerier`) but **not a domain/service abstraction** — the compositor still constructs raw SQL, builds positional `[]any` arguments via `extractOrderedArgs`, scans `pgx.FieldDescriptions` and `pgx.Rows` directly, and normalizes `pgx`-specific value types through `driver.Valuer`. There is no repository, service, or data-source layer between the renderer and the pools.

The 4-layer rule in `AGENTS.md §6` is the canonical guide ("Solo se permite el uso de Fyne como toolkit visual del core. No incluir dependencias ni menciones a otros frameworks…"). The complementary separation rule in `AGENTS.md §7.3` ("No acoplar UI en Datos") is the rule the audit findings are checking. Both findings are violations of the same boundary: the Layer 4 renderer is reaching across the line into Layer 1 driver semantics.

---

## 2. Files retrieved

Exact files and line ranges read for this exploration:

1. `pkg/ui/compositor.go` (1–603) — full file. The Layer 4 renderer; where both audit findings live.
2. `cmd/golemui/main.go` (1–158) — full file. Bootstrap and pool wiring (`ui.BusinessPool = dbPool.BusinessPool` and `ui.CorePool = dbPool.CorePool`).
3. `pkg/db/db.go` (1–95) — full file. The driver abstraction (`DatabasePool` interface, `DB` struct, `InitDB`).
4. `pkg/db/mock_db.go` (1–358) — full file. `MockDBPool` used by every `data_grid` test to assert direct-SQL behavior.
5. `pkg/config/loader.go` (1–60) — full file. YAML config loader (`BootstrapConfig.UIDB`, `BootstrapConfig.BusinessDB`, `EntryPointViewID`, `LayoutQuery`).
6. `pkg/ui/screen_loader.go` (1–45) — full file. The legitimate Layer 4 → Layer 2/3 access point (reads `config_columnas` JSONB).
7. `pkg/ui/sidebar_loader.go` (1–150) — full file. Navigation menu loader.
8. `pkg/ui/screen_state.go` (1–60) — full file. Per-screen form state (no DB access).
9. `pkg/ui/layout.go` (1–230) — full file. `FractionalLayout` (uses `1fr`/`px`/`auto` metrics from layout JSONB).
10. `pkg/ui/sidebar_widget.go` (1–130) — full file. `BuildNavTree` + `SelectByVistaID`.
11. `pkg/eventbus/eventbus.go` (1–82) — full file. The reactivity broker (no DB access).
12. `docker/init-db/02_init_core.sql` (1–126) — full file. The `golemui.componentes`, `golemui.estilos`, `golemui.mapeo_interfaz`, `golemui.vistas_consulta`, `golemui.menu_navegacion` schemas and seed rows.
13. `openspec/config.yaml` (full file) — strict TDD enabled, `execution_mode: interactive`, `delivery_strategy: ask-on-risk`.
14. `openspec/changes/screen-lifecycle-cleanup/explore.md` (1–120) — format reference for the explore artifact.
15. `openspec/changes/screen-loading-db/design.md` (1–80) — prior decision context: pool access pattern uses package-level globals (the same pattern the audit is questioning).
16. `openspec/changes/data-grid-row-selection/proposal.md` (full file) — prior decision context for `data_grid` extension patterns.

Targeted greps:

- `pkg/ui/compositor.go` for `BusinessPool`, `CorePool`, `driver.`, `DatabasePool`, `SetColumnWidth`, `150` — confirms the cited line numbers.
- Repo-wide for `DatabasePool`, `BusinessPool`, `CorePool` — confirms compositor is the *only* UI-layer consumer; `cmd/golemui/main.go` and `pkg/ui/screen_loader.go` and `pkg/ui/sidebar_loader.go` are the only other access points.
- Repo-wide for `SetColumnWidth`, `150`, `width` — confirms no other place defines a default grid column width.
- Repo-wide for `master_data_source`, `filter_mode`, `filter_keys` — confirms the JSONB fields the compositor reads.

---

## 3. Audit Finding 1 — Direct SQL & DB-driver awareness in Layer 4 (CONFIRMED)

### 3.1 Package-level DB pool globals

```go
// pkg/ui/compositor.go:7-22
import (
    "context"
    "database/sql/driver"   // ← L8: imports database/sql/driver
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

var BusinessPool db.DatabasePool   // ← L19
var CorePool db.DatabasePool        // ← L20
var LocalEventBus eventbus.EventBus
var Navigate func(vistaID string)
```

The `database/sql/driver` import (L8) is the giveaway. A pure Layer 4 renderer should never reach for the stdlib `database/sql/driver` package — that interface is the contract between Go's `database/sql` and concrete drivers (pgx, mysql, etc.). Its presence means the renderer is performing driver-level value normalization that belongs to the data-source layer.

The two package-level `db.DatabasePool` vars are the second giveaway. `pkg/ui/screen_loader.go:14` accepts the core pool as a parameter, which is correct for a testable boundary, but the data-grid path bypasses this and reads the global directly. The pre-existing decision (`openspec/changes/screen-loading-db/design.md` "Pool access pattern" row) rationalized the global pattern for symmetry with the older `BusinessPool`; that prior decision does not justify importing `driver.Valuer` in the renderer.

### 3.2 `loadMasterBuffer` — direct SQL via `BusinessPool`

```go
// pkg/ui/compositor.go:351-414
func loadMasterBuffer(ctx context.Context, node NodeMeta, model *dataGridModel, table *widget.Table) {
    pool := BusinessPool                          // ← L353: reads global
    if pool == nil {
        log.Printf("[UI/DataGrid] Warning: BusinessPool is nil; cannot load master buffer for data_grid at area %q", node.Area)
        return
    }
    log.Printf("[UI/DataGrid] Requesting master buffer eagerly for area %q. SQL: %q", node.Area, node.MasterDataSource)
    go func() {
        if err := ctx.Err(); err != nil { ... }
        rows, err := pool.Query(ctx, node.MasterDataSource)   // ← L364: raw SQL from JSONB layout
        if err != nil { ... }
        defer rows.Close()

        fds := rows.FieldDescriptions()                      // ← pgx API in renderer
        var headers []string
        for _, fd := range fds {
            headers = append(headers, fd.Name)
        }

        var dataRows [][]string
        for rows.Next() {
            ...
            vals, err := rows.Values()                       // ← pgx API in renderer
            if err != nil { ... }
            var stringRow []string
            for _, val := range vals {
                stringRow = append(stringRow, formatValue(val))  // ← driver.Valuer inside
            }
            dataRows = append(dataRows, stringRow)
        }
        ...
        model.mu.Lock()
        model.masterHeaders = headers
        model.masterRows = dataRows
        model.headers = headers
        model.rows = dataRows
        model.mu.Unlock()

        model.refreshMu.Lock()
        for i := 0; i < len(headers); i++ {
            table.SetColumnWidth(i, 150)                     // ← L411: hardcoded width
        }
        table.Refresh()
        model.refreshMu.Unlock()
    }()
}
```

`node.MasterDataSource` is a raw SQL string lifted from the JSONB layout in `golemui.vistas_consulta.config_columnas` (seeded in `docker/init-db/02_init_core.sql:88` as `"data_source":"SELECT id, emp_cod, monto, status FROM public.transacciones WHERE emp_cod LIKE '%' || $1 || '%' ..."`). The renderer has no idea what database it's hitting, what driver it's using, or what value types it will get back — it just runs the SQL and trusts the result. The `rows.FieldDescriptions()`, `rows.Next()`, `rows.Values()` calls are `pgx.Rows` API surface leaking into Layer 4.

### 3.3 `fetchGridDataAsync` — same pattern, different trigger

```go
// pkg/ui/compositor.go:518-585
func fetchGridDataAsync(ctx context.Context, node NodeMeta, model *dataGridModel, table *widget.Table, query string, args ...any) {
    if query == "" { return }
    pool := BusinessPool                                          // ← L523
    log.Printf("[UI/DataGrid] Requesting data async for area %q. SQL: %q, Args: %+v", node.Area, query, args)
    go func() {
        if pool == nil { ... }
        if err := ctx.Err(); err != nil { ... }
        rows, err := pool.Query(ctx, query, args...)              // ← L534: raw SQL + positional []any
        if err != nil { ... }
        defer rows.Close()

        fds := rows.FieldDescriptions()                          // pgx API in renderer
        var headers []string
        for _, fd := range fds {
            headers = append(headers, fd.Name)
        }

        var dataRows [][]string
        for rows.Next() {
            ...
            vals, err := rows.Values()                            // pgx API in renderer
            ...
            for _, val := range vals {
                stringRow = append(stringRow, formatValue(val))   // driver.Valuer inside
            }
            dataRows = append(dataRows, stringRow)
        }
        ...
        model.mu.Lock()
        model.headers = headers
        model.columns = headers
        model.rows = dataRows
        model.mu.Unlock()

        model.refreshMu.Lock()
        for i := 0; i < len(headers); i++ {
            table.SetColumnWidth(i, 150)                          // ← L580: hardcoded width
        }
        table.Refresh()
        model.refreshMu.Unlock()
    }()
}
```

`fetchGridDataAsync` is the server-mode counterpart: it is invoked from the `LocalEventBus` subscriber at `pkg/ui/compositor.go:255-272` when a `screen:submit:<vistaID>` event fires, and once at `pkg/ui/compositor.go:223-224` during initial composition. The `query` argument is either `node.DataSource` (the raw SQL field) or, for the dynamic query case, a value that arrived through `snap[stateKey]` and is then `fmt.Sprintf("%v", qVal)`-formatted into a SQL string (`pkg/ui/compositor.go:267-268`). The renderer is therefore *interpolating user-supplied values into SQL strings* and then executing them against a global pool.

The `args []any` is built by `extractOrderedArgs` at `pkg/ui/compositor.go:316-340` from the `ScreenState.Snapshot()` map, using `node.FilterKeys` to pick keys positionally. This is the only place in the file that builds query arguments, and it lives in Layer 4.

### 3.4 `formatValue` — `driver.Valuer` in the renderer

```go
// pkg/ui/compositor.go:587-603
func formatValue(val any) string {
    if val == nil {
        return ""
    }
    if valuer, ok := val.(driver.Valuer); ok {       // ← L591: type-asserts database/sql/driver.Valuer
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

`driver.Valuer` is the `database/sql` value-normalization interface. Real `pgx.Rows.Values()` returns entries like `pgtype.Numeric`, `pgtype.UUID`, `pgtype.Timestamp`, etc., all of which implement `driver.Valuer`. The renderer is responsible for flattening them. A clean boundary would do that flattening inside a data-source adapter and hand Layer 4 a `[]string` (or `[]any` of Go primitives).

### 3.5 Test surface that codifies the leak

`pkg/ui/compositor_test.go` has **at least 12 separate tests** that assign a `db.MockDBPool` directly to `ui.BusinessPool` and assert raw SQL string behavior:

- L161 `TestBusinessPoolExists`
- L187 `ui.BusinessPool = mockPool`
- L277 `ui.BusinessPool = nil` (defaults test)
- L333 `ui.BusinessPool = trackingPool` (call-counting)
- L460 `// subscriber fires before the goroutine reaches BusinessPool.Query`
- L687, L793, L989, L1085, L1217, L1296, L1350, L1432, L1469, L1526, L1565 — all set `ui.BusinessPool` directly

This is a strong signal that the global is load-bearing: removing it would require redesigning the test harness too. Any clean-Layer-4 refactor must update the test surface in the same change.

---

## 4. Audit Finding 2 — Hardcoded 150 px grid column width (CONFIRMED)

### 4.1 The two sites

```go
// pkg/ui/compositor.go:409-412  (loadMasterBuffer)
model.refreshMu.Lock()
for i := 0; i < len(headers); i++ {
    table.SetColumnWidth(i, 150)        // ← L411
}
table.Refresh()
model.refreshMu.Unlock()
```

```go
// pkg/ui/compositor.go:578-582  (fetchGridDataAsync)
model.refreshMu.Lock()
for i := 0; i < len(headers); i++ {
    table.SetColumnWidth(i, 150)        // ← L580
}
table.Refresh()
model.refreshMu.Unlock()
```

The number `150` is a Go float32 literal (`fyne.Table.SetColumnWidth` takes `float32`). It is duplicated in both data-fetch code paths. There is no constant, no config field, no metadata read, and no override path.

### 4.2 No metadata source exists

- `NodeMeta` (`pkg/ui/compositor.go:31-60`) has fields for `area`, `component_ref`, `label`, `placeholder`, `default_value`, `min`, `max`, `validation`, `data_source`, `submit_action`, `bind_to`, `filter_mode`, `master_data_source`, `filter_keys`, `layout`, and `children`. **No `column_width` or `width` field.**
- The container `LayoutMeta` (`pkg/ui/compositor.go:35-40`) has `type`, `columns`, `rows`, `gap` — and `columns`/`rows` are pixel/fractional/auto metrics used by `FractionalLayout.MinSize` and `FractionalLayout.Layout` (`pkg/ui/layout.go`). They control *container* column widths, not *data-grid* column widths. They use the unit suffix convention `1fr`/`30px`/`auto` (parsed by `parseMetric` at `pkg/ui/layout.go:31-50`).
- `golemui.componentes` (`docker/init-db/02_init_core.sql:4-25`) catalogs logical component types but has **no presentation metadata** (no `default_width`, no `style_id`). It is purely a `id` + `descripcion` lookup.
- `golemui.estilos` (`docker/init-db/02_init-core.sql:28-43`) defines semantic design tokens (`primary_action`, `success`, etc.) with colors, border-radius, font-size, font-weight. **No column width token.**
- `golemui.mapeo_interfaz` (`docker/init-db/02_init_core.sql:46-52`) is the Layer 3 override table. Schema: `(origen_id, columna_fisica, component_ref, label, placeholder, validation)`. **No width column.** The override is per `(origen, column)`, which is the natural granularity for "this column needs a wider default", and is the right place to add a width override.
- `golemui.vistas_consulta` (`docker/init-db/02_init_core.sql:68-71`) stores the screen layout as `config_columnas JSONB` and is the carrier of `NodeMeta` for the compositor. Could carry a per-grid width, but doing so would scatter the value across every vista that needs a grid.

### 4.3 Where the width *should* live in the 4-layer model

The cleanest mapping is to treat grid column width as **Layer 2/3 metadata**, not Layer 4 hardcoding. Two viable locations:

1. **`golemui.componentes` (Layer 2)** — add `default_column_width VARCHAR(20)` to the `data_grid` row. Parsed by the Layer 4 renderer via the same `parseMetric` convention used by `LayoutMeta.columns` (`1fr`/`30px`/`auto`).
2. **`golemui.mapeo_interfaz` (Layer 3)** — add `column_width VARCHAR(20)` so a developer can override the default per `(origen_id, columna_fisica)`. The natural join key (`origen_id` = `origen_datos` from `vistas_consulta`, `columna_fisica` = header from `pgx.FieldDescriptions`) is already implicit in the data flow.

Option 2 is more powerful and matches the AGENTS.md 4-layer model precisely: the default lives in Layer 2 (catalog), the override lives in Layer 3 (`mapeo_interfaz`), and the renderer (Layer 4) just reads metadata and never invents its own default.

A weaker fallback (acceptable but less pure) is to add `column_width` to `NodeMeta` and let it travel inside `config_columnas` JSONB. This avoids touching the SQL schema, but the value becomes per-vista instead of per-(origen, column) — which is wrong, because the same physical column from `negocio_production.public.transacciones` should have the same default in every screen that shows it.

---

## 5. Current DB interaction pattern

### 5.1 What the compositor receives vs. what it does

| Path | Direction | Function | Source | Abstractions used |
|------|-----------|----------|--------|-------------------|
| Navigation menu | Layer 4 → Layer 2/3 | `LoadNavigationMenu` (`pkg/ui/sidebar_loader.go:27`) | Core pool, raw SQL `NavigationMenuQuery` constant | `db.DatabasePool` (param) |
| Screen layout | Layer 4 → Layer 2/3 | `LoadScreen` (`pkg/ui/screen_loader.go:14`) | Core pool, raw SQL `DefaultLayoutQuery` constant | `db.DatabasePool` (param) |
| Grid master buffer (client mode) | Layer 4 → Layer 1 | `loadMasterBuffer` (`pkg/ui/compositor.go:351`) | **BusinessPool global**, raw SQL from `node.MasterDataSource` | `db.DatabasePool` (global) + `pgx.Rows` + `driver.Valuer` |
| Grid filter (server mode) | Layer 4 → Layer 1 | `fetchGridDataAsync` (`pkg/ui/compositor.go:518`) | **BusinessPool global**, raw SQL from `node.DataSource` or runtime state | `db.DatabasePool` (global) + `pgx.Rows` + `driver.Valuer` |
| Filter args | Layer 4 → Layer 1 | `extractOrderedArgs` (`pkg/ui/compositor.go:316`) | `ScreenState.Snapshot()` map + `node.FilterKeys` | none — pure Go |
| Row selection publish | Layer 4 internal | `table.OnSelected` (`pkg/ui/compositor.go:299-312`) | `model.headers` + `model.rows` | `eventbus.EventBus` |

The pattern: **`pkg/ui/screen_loader.go` and `pkg/ui/sidebar_loader.go` get the pool as a parameter (clean), but `compositor.go`'s data-grid path reads `BusinessPool` as a global and runs raw SQL inline (dirty)**. The clean paths return Go structs (`NodeMeta`, `[]MenuItem`) and the renderer does not see SQL; the dirty paths return `pgx.Rows` and the renderer does the scanning itself.

### 5.2 What abstractions exist in `pkg/db/`

```go
// pkg/db/db.go:11-26
type DBQuerier interface {
    Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
    Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
    QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
    SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
}

type DatabasePool interface {
    DBQuerier
    Ping(ctx context.Context) error
    Close()
}
```

The interfaces expose **`pgx.Rows` and `pgconn.CommandTag`** as return types, not domain types. This is a *driver abstraction*, not a *domain abstraction*. It hides `pgxpool.Pool` behind a name (`DatabasePool`) but does not hide pgx semantics. A `MockDBPool` (`pkg/db/mock_db.go:30-358`) is the test double, and it implements the same `pgx.Rows` surface.

There is **no repository, no service, no data-source adapter** between the compositor and these interfaces. The cleanest existing data access is `LoadScreen` and `LoadNavigationMenu`, but those still take a `db.DatabasePool` and run raw SQL inline — they are just functions that happen to be in the UI package, not abstractions.

### 5.3 Is there any repository/service layer between compositor and DB?

**No.** The full path from the data-grid case in the compositor to the database is:

```
compositor.go:data_grid case
  → BusinessPool (global)              pkg/ui/compositor.go:19
  → pool.Query(ctx, rawSQL, args...)   pkg/ui/compositor.go:364 / L534
  → pgxpool.Pool.Query                 pkg/db/db.go:60 (init) → pgx driver
  → negocio_production / golemui_core
```

A repository layer would intercept the second arrow: a function like `dataSource.Fetch(ctx, source, args) (Headers, Rows, error)` that owns the pool and the `formatValue` flattening, and returns a struct the renderer can consume directly. That is what the next section sketches.

---

## 6. What a clean Layer 4 boundary would look like

### 6.1 Target shape of the renderer

The compositor should consume *resolved data* and *resolved metadata*, never *raw SQL* and *driver values*. The minimum set of abstractions is:

```go
// pkg/ui/datasource.go (new file) — defines the boundary

// DataSource is the Layer 4 → Layer 1/2 boundary.
// The renderer never imports database/sql, pgx, or db.DatabasePool.
type DataSource interface {
    // Fetch runs a logical source with positional args and returns a flat
    // header + string-matrix result. All driver-specific value types are
    // normalized to strings (or to Go primitives via RenderedValue) before
    // they reach the renderer.
    Fetch(ctx context.Context, source string, args ...any) (DataSet, error)
    // FetchAll is the client-mode equivalent: no filter args, load everything.
    FetchAll(ctx context.Context, source string) (DataSet, error)
}

type DataSet struct {
    Headers    []string
    Rows       [][]string
    // Optional: column-width metadata resolved from Layer 2/3.
    // Each entry is in the same "1fr"/"30px"/"auto" convention used by
    // FractionalLayout. Nil/empty means "use renderer default".
    ColumnWidths []string
}

// ColumnWidthResolver is the Layer 4 → Layer 2/3 boundary for presentation
// defaults. It hides the schema details of golemui.componentes and
// golemui.mapeo_interfaz and returns a parsed metric spec.
type ColumnWidthResolver interface {
    Resolve(origen string, header string) string  // "150px", "1fr", "auto", ""
}
```

The compositor would consume:

```go
// pkg/ui/compositor.go (target shape)
model.dataSource.Fetch(ctx, node.DataSource, args...)   // returns DataSet
for i, w := range ds.ColumnWidths {
    if w == "" { w = defaultColumnWidth }                // fallback only
    table.SetColumnWidth(i, parseMetricToFloat(w))
}
```

### 6.2 Where `BusinessPool` and `CorePool` should live

They should live in the **data-source implementation** of the `DataSource` interface, in a new package — e.g. `pkg/dataaccess/pgsql/`. That implementation would be the only place that imports `github.com/jackc/pgx/v5` and `database/sql/driver`. The `pkg/db` driver abstraction can be re-used as a building block.

The current globals `ui.BusinessPool` and `ui.CorePool` should be deleted from the compositor. The new `DataSource` instance is constructed in `main.go` (where the pools are already known) and injected into the compositor via a package-level setter, a constructor parameter, or a struct field on a renderer type. The cleanest move is to convert the renderer's package-level vars to a struct, but that is a larger refactor; a minimum-disruption approach is to replace `ui.BusinessPool` with `ui.DataSource` (a single interface, not two pools).

### 6.3 Where the column-width metadata should live

Recommended (matches AGENTS.md 4-layer model exactly):

1. **`golemui.componentes` (Layer 2)** — add `default_column_width VARCHAR(20)` to the row `('data_grid', '...')`. Seed value: `'150px'`. Parsed by `parseMetric` (`pkg/ui/layout.go:31-50`).
2. **`golemui.mapeo_interfaz` (Layer 3)** — add `column_width VARCHAR(20)` to the override table. Migration: a single `ALTER TABLE golemui.mapeo_interfaz ADD COLUMN column_width VARCHAR(20)`. Seed: leave all rows NULL. The `origen_id` matches `vistas_consulta.origen_datos`; the `columna_fisica` matches the `pgx.FieldDescription.Name` of the grid header.
3. **`pkg/ui/compositor.go` (Layer 4)** — no `150` literal. The renderer reads metadata through `ColumnWidthResolver` and falls back to a constant `defaultColumnWidth` only if both lookups return empty.

This puts the value in the *right* place: changing "all status columns in all screens are 200px wide" is one `UPDATE golemui.mapeo_interfaz SET column_width = '200px' WHERE columna_fisica = 'status'`, no recompile, no Go change.

### 6.4 What `formatValue` should become

The `driver.Valuer` type-assertion and the `[]byte` switch should move into the data-source implementation. The data source returns `[][]string` (or `[][]any` of Go primitives) and the renderer never sees `pgtype.*` types. The `[]byte → string` case becomes a data-source concern: it is the binary-data interpretation, not a UI formatting decision.

### 6.5 Test surface impact

- `pkg/ui/compositor_test.go` currently sets `ui.BusinessPool = trackingPool` and asserts raw SQL string match. Under a clean boundary, the tests would inject a `MockDataSource` that records `Fetch(source, args...)` calls and returns canned `DataSet` values. The test surface becomes **smaller and more focused**: tests assert "the right source was requested with the right args", not "the right SQL string was passed to pgx".
- The `pkg/db/mock_db.go` test double stays useful for the data-source implementation tests (it still has to be tested against `pgx.Rows`).
- `cmd/golemui/main_test.go` already exercises the bootstrap wiring (L136 `ui.BusinessPool = nil`, L162-163 mock setup); that test stays valid and gets extended to assert the new `ui.DataSource` is wired.

---

## 7. Open questions for the proposal phase

1. **Single pool or split?** The two-pool split (Business vs. Core) is structurally important — `AGENTS.md §2` says "el ecosistema de base de datos se separa físicamente en dos componentes para evitar la contaminación de los dominios". A clean boundary preserves the split: the data-source service can have a `Business()` and `Core()` accessor, or two separate `DataSource` instances. The proposal must decide.
2. **Where does `extractOrderedArgs` move?** The function takes `ScreenState.Snapshot()` (a `map[string]any`) and `node.FilterKeys` (a `[]string`). The state is a Layer 4 concern; the keys are layout metadata. The args-building logic should live with the data-source service, but it needs to read from `ScreenState`, which means either (a) the data-source takes the snapshot as a parameter, or (b) the keys are passed in and the snapshot is read inside. The proposal must pick.
3. **Should the dynamic-query path (`fmt.Sprintf("%v", qVal)` interpolation in `pkg/ui/compositor.go:268`) survive?** Interpolating user values into SQL strings is a SQL-injection concern *and* a 4-layer leak. The proposal should make it the data-source service's job to receive either a SQL string with named/positional placeholders *or* a structured query spec. The current "state-driven SQL" is fragile and is the strongest argument for a service layer.
4. **Renderer-level fallback default?** If the data-source and the column-width resolver both return no width, what does the renderer use? A package-level constant in `pkg/ui` is acceptable (it is the "no metadata at all" fallback, not a hardcoded policy); a magic literal like `150` in the call site is not. The cleanest default is `parseMetric("auto")` (let Fyne size the column), but that changes the visual default and may surprise users.
5. **What about the existing tests that assert `BusinessPool.Query` was called with a specific SQL string?** The proposal needs to either (a) port those tests to the new abstraction ("the data source was asked to fetch the right logical source") or (b) preserve them at the data-source-implementation test layer. Option (a) is the right move for renderer tests; option (b) is the right move for service tests.

---

## 8. Start here

**For the next agent (proposal writer):** open `pkg/ui/compositor.go` lines 19–22 (the globals), 351–414 (`loadMasterBuffer`), 518–585 (`fetchGridDataAsync`), and 587–603 (`formatValue`). Then read `pkg/ui/screen_loader.go:14–44` to see the *clean* Layer 4 → Layer 2/3 pattern that `data_grid` should mirror. Then read `docker/init-db/02_init_core.sql:4–70` to see the schema for `componentes` (Layer 2) and `mapeo_interfaz` (Layer 3) — those are the right places to add column-width metadata, and reading them now will save a re-read during design.

**For the reviewer:** the load-bearing questions are (1) does the data-source abstraction need to be a struct/interface or just a function set, (2) does the column-width metadata live in `componentes` + `mapeo_interfaz` or in the layout JSONB, and (3) does the dynamic-query interpolation (`fmt.Sprintf` in `fetchGridDataAsync`) survive the refactor. The audit findings are correct and the violations are real; the open questions above are the design choices, not the diagnosis.
