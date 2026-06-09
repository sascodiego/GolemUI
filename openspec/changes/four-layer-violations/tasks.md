# Tasks: four-layer-violations

## Overview

Refactor `pkg/ui/compositor.go` to remove direct DB coupling by introducing `DataSource` and `ColumnWidthResolver` interfaces, moving SQL execution and driver-value normalization to a new `pkg/dataaccess/` package, and replacing hardcoded column widths with metadata-driven resolution.

**Estimated total:** ~915 lines across 13 files (5 new, 6 modified, 2 new test-only).

---

## Phase 1: RED — Interfaces and Tests

### Task 1.1: Create interface definitions

Create the boundary contracts needed for all downstream tests to compile.

- [ ] Create `pkg/ui/datasource.go` with `DataSet` struct (`Headers []string`, `Rows [][]string`, `ColumnWidths []string`)
- [ ] Add `DataSource` interface with `Fetch(ctx context.Context, source string, args ...any) (DataSet, error)` and `FetchAll(ctx context.Context, source string) (DataSet, error)`
- [ ] Add `ColumnWidthResolver` interface with `Resolve(origen string, header string) string`
- [ ] Add `MockDataSource` struct with `FetchCalled`, `FetchSource`, `FetchArgs`, `FetchResult`, `FetchError`, `FetchAllCalled`, `FetchAllSource`, `FetchAllResult`, `FetchAllError` fields and interface methods
- [ ] Add `MockCWR` struct with `ResolveFunc func(origen, header string) string` field and interface method

**Verify:**
```bash
go build ./pkg/ui/
go vet ./pkg/ui/
```

### Task 1.2: Write FormatValue tests

Write unit tests for `FormatValue` in the new `dataaccess` package.

- [ ] Create `pkg/dataaccess/format_test.go` with `package dataaccess_test`
- [ ] Add `mockValuer` test helper implementing `driver.Valuer` with `val any` and `err error` fields
- [ ] Write TFV-01: `FormatValue(nil)` → `""`
- [ ] Write TFV-02: `FormatValue(42)` → `"42"`
- [ ] Write TFV-03: `FormatValue(3.14)` → `"3.14"`
- [ ] Write TFV-04: `FormatValue("hello")` → `"hello"`
- [ ] Write TFV-05: `FormatValue(true)` → `"true"`
- [ ] Write TFV-06: `FormatValue(&mockValuer{val: []byte("data")})` → `"data"`
- [ ] Write TFV-07: `FormatValue(&mockValuer{val: "text"})` → `"text"`
- [ ] Write TFV-08: `FormatValue(&mockValuer{val: nil})` → `fmt.Sprintf("%v", mockValuer)` (falls through)
- [ ] Write TFV-09: `FormatValue(&mockValuer{err: fmt.Errorf("fail")})` → `fmt.Sprintf("%v", mockValuer)` (falls through)

**Verify:** `go test ./pkg/dataaccess/...` fails (package does not exist yet).

### Task 1.3: Write ExtractOrderedArgs tests

Write unit tests for `ExtractOrderedArgs`.

- [ ] Create `pkg/dataaccess/extract_args_test.go` with `package dataaccess_test`
- [ ] Write TEA-01: `snap={"a":"1","b":"2"}, filterKeys=["a","b"]` → `[]any{"1","2"}`
- [ ] Write TEA-02: `snap={"a":"1"}, filterKeys=["a","b"]` → `[]any{"1",""}`
- [ ] Write TEA-03: `snap={"a":"1"}, filterKeys=[]` → `[]any{}` (empty non-nil)
- [ ] Write TEA-04: `snap=nil, filterKeys=["a"]` → `[]any{""}`
- [ ] Write TEA-05: `snap={"b":"2","a":"1","c":"3"}, filterKeys=["c","a","b"]` → `[]any{"3","1","2"}`
- [ ] Write TEA-06: `snap={"a":"1"}, filterKeys=nil` → `[]any{}` (empty non-nil)

**Verify:** `go test ./pkg/dataaccess/...` fails.

### Task 1.4: Write SQLDataSource tests

Write unit tests for `SQLDataSource` using `db.MockDBPool`.

- [ ] Create `pkg/dataaccess/sql_datasource_test.go` with `package dataaccess_test`
- [ ] Add imports: `"context"`, `"testing"`, `"GolemUI/pkg/dataaccess"`, `"GolemUI/pkg/db"`
- [ ] Write TDS-01: Fetch with valid data returns `DataSet{Headers:["id","name"], Rows:[["1","Alice"],["2","Bob"]]}`
- [ ] Write TDS-02: Fetch passes positional args — verify mock receives `args=["hello"]`
- [ ] Write TDS-03: FetchAll delegates to Fetch — same result as `Fetch(ctx, source)`
- [ ] Write TDS-04: Fetch with empty source returns `DataSet{}`, non-nil error
- [ ] Write TDS-05: Fetch with cancelled context returns `DataSet{}`, `context.Canceled`
- [ ] Write TDS-06: Fetch with pool error returns `DataSet{}`, non-nil error wrapping pool error
- [ ] Write TDS-07: Fetch with nil pool (`NewSQLDataSource(nil)`) returns `DataSet{}`, error containing "pool is nil"
- [ ] Write TDS-08: Fetch returns empty rows for zero-result query — `Headers` present, `Rows: [][]string{}`
- [ ] Write TDS-09: Fetch normalizes `driver.Valuer` types — all values are strings
- [ ] Write TDS-10: Fetch handles `rows.Values()` error gracefully — partial DataSet + break

**Verify:** `go test ./pkg/dataaccess/...` fails.

### Task 1.5: Write ColumnWidthResolver tests

Write unit tests for `ColumnWidthResolver` using `db.MockDBPool`.

- [ ] Create `pkg/dataaccess/column_width_resolver_test.go` with `package dataaccess_test`
- [ ] Write TCW-01: Layer 3 override — mapeo_interfaz returns `"200px"` → `Resolve("transacciones_list", "status")` returns `"200px"`
- [ ] Write TCW-02: Layer 2 default only — mapeo_interfaz returns 0 rows; componentes returns `"150px"` → `Resolve("any","any")` returns `"150px"`
- [ ] Write TCW-03: Neither exists — both return 0 rows → `Resolve("x","y")` returns `""`
- [ ] Write TCW-04: Layer 3 error falls to Layer 2 — mapeo_interfaz error; componentes returns `"150px"` → `Resolve("x","y")` returns `"150px"`
- [ ] Write TCW-05: Both queries error → `Resolve("x","y")` returns `""`
- [ ] Write TCW-06: Caching — same `(origen, header)` called twice → second call returns cached value; verify only 1 DB round-trip via mock state
- [ ] Write TCW-07: Different `(origen, header)` → fresh DB lookups, no false cache hit

**Verify:** `go test ./pkg/dataaccess/...` fails.

### Task 1.6: Migrate compositor tests

Update `pkg/ui/compositor_test.go` to use `MockDataSource`/`MockCWR` instead of `MockDBPool`.

- [ ] Remove imports: `"GolemUI/pkg/db"`, `"github.com/jackc/pgx/v5"`
- [ ] Add `trackingMockDataSource` type embedding `*ui.MockDataSource` with `mu sync.Mutex`, `fetchCalls []struct{source string; args []any}`, `fetchAllCalls []string`
- [ ] Implement `Fetch` and `FetchAll` on `trackingMockDataSource` that record calls and delegate
- [ ] Migrate `TestBusinessPoolExists` → `TestDataSourceExists`: assert `var ds interface{} = ui.DS` compiles
- [ ] Migrate `TestCorePool_DefaultsNil` → `TestCWR_DefaultsNil`: assert `ui.CWR == nil`
- [ ] Migrate `TestCompose_DataGrid_Success`: set `ui.DS = &ui.MockDataSource{FetchResult: ui.DataSet{Headers:["id","title","amount"], Rows:[["1","Book A","25.5"],["2","Book B","35"]]}}`; `ui.CWR = &ui.MockCWR{}`
- [ ] Migrate `TestCompose_DataGrid_NoDataSource`: set `ui.DS = nil`
- [ ] Migrate `TestCompose_DataGrid_NilPool` → `TestCompose_DataGrid_NilDataSource`: set `ui.DS = nil`
- [ ] Migrate `TestCompose_DataGrid_ReactiveFiltering`: use `trackingDS.fetchCalls` instead of `trackingPool.queriesCalled`
- [ ] Migrate `TestCompose_DataGrid_ServerMode_SubmitChannelQuery`: assert `trackingDS.fetchCalls` source string + args
- [ ] Migrate `TestCompose_DataGrid_ClientMode_EagerLoadAndFilter`: assert `trackingDS.fetchAllCalls` contains `"SELECT * FROM books"`; no additional calls after submit
- [ ] Migrate `TestCompose_DataGrid_RowSelection_PublishesToSelectionChannel`: set `ui.DS` with `FetchResult` containing `Headers:["id","nombre","monto"]`, `Rows:[["42","Transaccion Test","1000.5"]]`
- [ ] Migrate `TestCompose_DataGrid_RowSelection_OutOfBounds_NoPublish`: set `ui.DS` with 1-row DataSet
- [ ] Migrate `TestCompose_DataGrid_RowSelection_NilEventBus_NoPanic`: set `ui.DS` with mock data
- [ ] Migrate `TestCompose_ServerMode_NoFilterKeys_SkipsSubmit`: use `trackingDS.fetchCalls` — assert no Fetch after submit
- [ ] Migrate `TestCompose_ClientMode_FilterMismatchColumn_LogsWarning`: set `ui.DS` with `FetchAllResult` for master data
- [ ] Migrate `TestCompose_ReturnsCleanupFunc`: set `ui.DS` with mock data
- [ ] Migrate `TestCompose_CleanupRemovesSubscribers`: set `ui.DS` with mock data
- [ ] Migrate `TestCompose_CleanupCancelsGoroutines`: set `ui.DS` with `FetchAllResult`
- [ ] Migrate `TestCompose_IdempotentCleanup`: set `ui.DS` with mock data
- [ ] Leave `TestCompose_ScopedSubmitChannel_NoCrossTalk` and `TestCompose_Button_SubmitAction_PublishesSnapshot` unchanged (no DS needed)
- [ ] Add new `TestCompose_DataGrid_ColumnWidthFromCWR`: `MockCWR` returns `"200px"` for column "status"; verify `table.SetColumnWidth` called with 200
- [ ] Add new `TestCompose_DataGrid_ColumnWidthFallback`: `MockCWR` returns `""`; verify `table.SetColumnWidth` called with 150
- [ ] Add new `TestCompose_DataGrid_DynamicQueryFromState`: `state:` prefix resolution; `Fetch` called with resolved SQL, no args
- [ ] Audit all tests for proper cleanup: `defer func() { ui.DS = nil; ui.CWR = nil }()`

**Verify:** `go test ./pkg/ui/...` fails (compositor still uses old `BusinessPool`/`CorePool` globals).

---

## Phase 2: GREEN — Implementation

### Task 2.1: Implement FormatValue

Create the `FormatValue` function to pass all TFV tests.

- [ ] Create `pkg/dataaccess/format.go` with `package dataaccess`
- [ ] Implement `FormatValue(val any) string`:
  - `val == nil` → `""`
  - `valuer, ok := val.(driver.Valuer)` → call `.Value()`; if `err == nil && v != nil`, switch on `[]byte` vs default
  - Fallback: `fmt.Sprintf("%v", val)`

**Verify:**
```bash
go test ./pkg/dataaccess/ -run TestFormatValue -v
```

### Task 2.2: Implement ExtractOrderedArgs

Create the `ExtractOrderedArgs` function to pass all TEA tests.

- [ ] Create `pkg/dataaccess/extract_args.go` with `package dataaccess`
- [ ] Implement `ExtractOrderedArgs(snap map[string]any, filterKeys []string) []any`:
  - `len(filterKeys) == 0` → `[]any{}`
  - Iterate `filterKeys`; missing keys → `""`; nil snap → `""` for all

**Verify:**
```bash
go test ./pkg/dataaccess/ -run TestExtractOrderedArgs -v
```

### Task 2.3: Implement SQLDataSource

Create the `SQLDataSource` struct to pass all TDS tests.

- [ ] Create `pkg/dataaccess/sql_datasource.go` with `package dataaccess`
- [ ] Add imports: `"context"`, `"fmt"`, `"log"`, `"GolemUI/pkg/db"`, `"GolemUI/pkg/ui"`
- [ ] Add compile-time check: `var _ ui.DataSource = (*SQLDataSource)(nil)`
- [ ] Define `SQLDataSource` struct with `pool db.DatabasePool`
- [ ] Implement `NewSQLDataSource(pool db.DatabasePool) *SQLDataSource`
- [ ] Implement `Fetch(ctx context.Context, source string, args ...any) (ui.DataSet, error)`:
  - Guard: empty source, nil pool, `ctx.Err()`
  - `pool.Query(ctx, source, args...)`
  - `rows.FieldDescriptions()` → headers
  - Iterate `rows.Next()` → `rows.Values()` → `FormatValue(val)` per cell
  - Return `ui.DataSet{Headers, Rows}`
- [ ] Implement `FetchAll(ctx context.Context, source string) (ui.DataSet, error)`: delegate to `Fetch(ctx, source)`

**Verify:**
```bash
go test ./pkg/dataaccess/ -run TestSQLDataSource -v
```

### Task 2.4: Implement ColumnWidthResolver

Create the `ColumnWidthResolver` to pass all TCW tests.

- [ ] Create `pkg/dataaccess/column_width_resolver.go` with `package dataaccess`
- [ ] Add imports: `"context"`, `"log"`, `"sync"`, `"GolemUI/pkg/db"`, `"GolemUI/pkg/ui"`
- [ ] Add compile-time check: `var _ ui.ColumnWidthResolver = (*ColumnWidthResolver)(nil)`
- [ ] Define `ColumnWidthResolver` struct with `pool db.DatabasePool`, `cache sync.Map`
- [ ] Implement `NewColumnWidthResolver(pool db.DatabasePool) *ColumnWidthResolver`
- [ ] Implement `cacheKey(origen, header string) string`
- [ ] Implement `Resolve(origen, header string) string`:
  - Check `sync.Map` cache first
  - Layer 3: `SELECT column_width FROM golemui.mapeo_interfaz WHERE origen_id = $1 AND columna_fisica = $2`
  - Layer 2: `SELECT default_column_width FROM golemui.componentes WHERE id = 'data_grid'`
  - Store result in cache, return

**Verify:**
```bash
go test ./pkg/dataaccess/ -run TestColumnWidthResolver -v
```

### Task 2.5: Verify all dataaccess tests pass

- [ ] Run full dataaccess test suite: `go test ./pkg/dataaccess/... -v`
- [ ] Confirm all TFV, TEA, TDS, TCW tests pass
- [ ] Run `go vet ./pkg/dataaccess/`
- [ ] Run `go build ./pkg/dataaccess/`

**Verify:**
```bash
go test ./pkg/dataaccess/... -v
go vet ./pkg/dataaccess/
go build ./pkg/dataaccess/
```

### Task 2.6: Modify compositor to use DataSource/CWR

Rewrite `pkg/ui/compositor.go` to remove all DB coupling and use the new interfaces.

- [ ] Update import block: remove `"database/sql/driver"` and `"GolemUI/pkg/db"`; add `"GolemUI/pkg/dataaccess"`
- [ ] Replace globals: delete `var BusinessPool db.DatabasePool` and `var CorePool db.DatabasePool`; add `var DS DataSource`, `var CWR ColumnWidthResolver`, `const defaultGridColWidth float32 = 150`
- [ ] Replace `loadMasterBuffer` internals:
  - `BusinessPool == nil` → `DS == nil`
  - `pool.Query(ctx, node.MasterDataSource)` → `DS.FetchAll(ctx, node.MasterDataSource)`
  - Manual row iteration → populate model from `DataSet` directly
  - `table.SetColumnWidth(i, 150)` → `resolveWidth(ds.ColumnWidths, i, h, node.MasterDataSource)`
- [ ] Replace `fetchGridDataAsync` internals:
  - `pool := BusinessPool` → `DS == nil` check
  - `pool.Query(ctx, query, args...)` → `DS.Fetch(ctx, query, args...)`
  - Manual row iteration → populate model from `DataSet` directly
  - `table.SetColumnWidth(i, 150)` → `resolveWidth(ds.ColumnWidths, i, h, node.DataSource)`
- [ ] Add `resolveWidth(columnWidths []string, colIndex int, header string, origen string) float32` helper using `parseMetric` + CWR + fallback
- [ ] Replace `extractOrderedArgs(snap, node.FilterKeys)` calls with `dataaccess.ExtractOrderedArgs(snap, node.FilterKeys)` in both EventBus subscriber and initial data load
- [ ] Delete `formatValue` function (moved to `dataaccess.FormatValue`)
- [ ] Delete `extractOrderedArgs` function (moved to `dataaccess.ExtractOrderedArgs`)

**Verify:**
```bash
go build ./pkg/ui/
go test ./pkg/ui/... -v
```

### Task 2.7: Modify main.go wiring

Update `cmd/golemui/main.go` to wire the new interfaces.

- [ ] Add import `"GolemUI/pkg/dataaccess"`
- [ ] Replace `ui.BusinessPool = dbPool.BusinessPool` with `ui.DS = dataaccess.NewSQLDataSource(dbPool.BusinessPool)`
- [ ] Replace `ui.CorePool = dbPool.CorePool` with `ui.CWR = dataaccess.NewColumnWidthResolver(dbPool.CorePool)`
- [ ] Replace `ui.CorePool` parameter references with `dbPool.CorePool` in `LoadNavigationMenu`, `LoadScreen` calls
- [ ] Remove the `ui.CorePool = dbPool.CorePool` line entirely

**Verify:**
```bash
go build ./cmd/golemui/
```

### Task 2.8: Update database init script

Modify `docker/init-db/02_init_core.sql` with schema changes for column width.

- [ ] Add `default_column_width VARCHAR(20)` column to `golemui.componentes` CREATE TABLE
- [ ] Update `golemui.componentes` INSERT: all rows get `NULL` except `data_grid` which gets `'150px'`
- [ ] Add `column_width VARCHAR(20)` column to `golemui.mapeo_interfaz` CREATE TABLE
- [ ] Verify no other tables are modified

**Verify:**
```bash
grep 'default_column_width' docker/init-db/02_init_core.sql
grep 'column_width' docker/init-db/02_init_core.sql
```

---

## Phase 3: REFACTOR — Cleanup and Validation

### Task 3.1: Remove dead imports and code

Clean up remaining artifacts from the migration.

- [ ] Check `pkg/ui/compositor_test.go` for unused imports (`"GolemUI/pkg/db"`, `"github.com/jackc/pgx/v5"`) and remove
- [ ] Check for any remaining `trackingMockDBPool` type definition and remove
- [ ] Remove `ui.CorePool` reference from `pkg/ui/compositor.go` if any remains
- [ ] Remove `ui.BusinessPool` reference from `pkg/ui/compositor.go` if any remains

**Verify:**
```bash
go vet ./pkg/ui/
```

### Task 3.2: Verify no BusinessPool/CorePool references in pkg/ui/

- [ ] Run: `grep -rn 'BusinessPool\|CorePool' pkg/ui/` → expect 0 matches
- [ ] If matches found, trace and eliminate each one

**Verify:**
```bash
grep -rn 'BusinessPool\|CorePool' pkg/ui/ | wc -l  # → 0
```

### Task 3.3: Verify no database/sql/driver in pkg/ui/

- [ ] Run: `grep -r 'database/sql/driver' pkg/ui/` → expect 0 matches

**Verify:**
```bash
grep -r 'database/sql/driver' pkg/ui/ | wc -l  # → 0
```

### Task 3.4: Verify no hardcoded 150 in SetColumnWidth

- [ ] Run: `grep -n 'SetColumnWidth(i, 150)' pkg/ui/compositor.go` → expect exit code 1 (no matches)

**Verify:**
```bash
grep -n 'SetColumnWidth(i, 150)' pkg/ui/compositor.go  # → exit 1
```

### Task 3.5: Full test suite

- [ ] Run: `go test ./...` → exit 0
- [ ] All TFV, TEA, TDS, TCW, TCI tests pass

**Verify:**
```bash
go test ./... -count=1
```

### Task 3.6: Build check

- [ ] Run: `go build ./...` → exit 0

**Verify:**
```bash
go build ./...
```

### Task 3.7: Lint

- [ ] Run: `golangci-lint run` → no errors

**Verify:**
```bash
golangci-lint run
```

### Task 3.8: Format

- [ ] Run: `gofmt -w .`
- [ ] Verify no diff: `gofmt -l .` → empty output

**Verify:**
```bash
gofmt -w .
gofmt -l .
```

---

## Final Validation Checklist

All success criteria from spec §14 must pass:

- [ ] **SC-1:** No `database/sql/driver` in `pkg/ui/` → `grep -r 'database/sql/driver' pkg/ui/` returns 0 matches
- [ ] **SC-2:** No `db.DatabasePool` in compositor → `grep 'GolemUI/pkg/db' pkg/ui/compositor.go` returns exit 1
- [ ] **SC-3:** No pool globals in compositor → `grep -E 'var (BusinessPool|CorePool)' pkg/ui/compositor.go` returns exit 1
- [ ] **SC-4:** No hardcoded 150 literal → `grep -n 'SetColumnWidth(i, 150)' pkg/ui/compositor.go` returns exit 1
- [ ] **SC-5:** Named fallback constant exists → `grep 'defaultGridColWidth' pkg/ui/compositor.go` returns exactly 1 definition
- [ ] **SC-6:** All tests pass → `go test ./...` exits 0
- [ ] **SC-7:** Build succeeds → `go build ./...` exits 0
- [ ] **SC-8:** `DataSource` interface exists → `grep 'type DataSource interface' pkg/ui/datasource.go` returns 1 match
- [ ] **SC-9:** `ColumnWidthResolver` interface exists → `grep 'type ColumnWidthResolver interface' pkg/ui/datasource.go` returns 1 match
- [ ] **SC-10:** `dataaccess` package compiles → `go build ./pkg/dataaccess/` exits 0
- [ ] **SC-11:** Schema: `default_column_width` column → `grep 'default_column_width' docker/init-db/02_init_core.sql` returns matches
- [ ] **SC-12:** Schema: `column_width` column → `grep 'column_width' docker/init-db/02_init_core.sql` returns matches
