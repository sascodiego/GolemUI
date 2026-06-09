# Apply Report: four-layer-violations

**Date:** 2026-06-07
**Change:** four-layer-violations
**Status:** COMPLETE

---

## Task Completion Summary

### Phase 1: RED — Interfaces and Tests

| Task | Status | Notes |
|------|--------|-------|
| 1.1 Create `pkg/ui/datasource.go` with interfaces and mocks | ✅ | Interfaces defined directly in `pkg/ui/datasource.go` |
| 1.2 Write `pkg/dataaccess/format_test.go` (TFV-01..09) | ✅ | 9 test cases |
| 1.3 Write `pkg/dataaccess/extract_args_test.go` (TEA-01..06) | ✅ | 6 test cases |
| 1.4 Write `pkg/dataaccess/sql_datasource_test.go` (TDS-01..10) | ✅ | 11 test cases (10 + interface check) |
| 1.5 Write `pkg/dataaccess/column_width_resolver_test.go` (TCW-01..07) | ✅ | 8 test cases (7 + interface check) |
| 1.6 Migrate `pkg/ui/compositor_test.go` | ✅ | All 26 tests migrated + 3 new tests |

### Phase 2: GREEN — Implementation

| Task | Status | Notes |
|------|--------|-------|
| 2.1 Implement `pkg/dataaccess/format.go` | ✅ | 25 lines |
| 2.2 Implement `pkg/dataaccess/extract_args.go` | ✅ | 23 lines |
| 2.3 Implement `pkg/dataaccess/sql_datasource.go` | ✅ | 73 lines, implements `ui.DataSource` |
| 2.4 Implement `pkg/dataaccess/column_width_resolver.go` | ✅ | 72 lines, `SQLColumnWidthResolver` implements `ui.ColumnWidthResolver` |
| 2.5 Verify all dataaccess tests pass | ✅ | 34/34 pass |
| 2.6 Modify `pkg/ui/compositor.go` | ✅ | Removed DB coupling, uses DataSource/CWR |
| 2.7 Modify `cmd/golemui/main.go` | ✅ | Wires DS and CWR via dataaccess constructors |
| 2.8 Update `docker/init-db/02_init_core.sql` | ✅ | Added `default_column_width` and `column_width` columns |

### Phase 3: REFACTOR — Cleanup and Validation

| Task | Status | Notes |
|------|--------|-------|
| 3.1–3.8 | ✅ | No BusinessPool/CorePool, no driver import, no hardcoded 150, all tests pass, build clean, vet clean, gofmt clean |

---

## Deviations from Design

1. **Import cycle resolution**: The design specified interfaces in `pkg/ui/datasource.go` with `pkg/dataaccess` importing `pkg/ui`, and `pkg/ui/compositor.go` importing `pkg/dataaccess`. This creates a cycle. Resolution: interfaces live in `pkg/ui/datasource.go`, `pkg/ui/compositor.go` does NOT import `pkg/dataaccess` (keeps a local `extractOrderedArgs`), and `pkg/dataaccess/interfaces.go` has type aliases to the `ui` types.

2. **Struct naming**: `ColumnWidthResolver` implementation renamed to `SQLColumnWidthResolver` to avoid collision with the interface name.

3. **Mock scan types**: Uses `string` (not `*string`) for `QueryRow.Scan` targets because `mock_db.go`'s `scanRow` requires exact type assignability.

4. **Local extractOrderedArgs**: Compositor keeps its own `extractOrderedArgs` function (identical logic to `dataaccess.ExtractOrderedArgs`) to avoid the import cycle. Both copies are independently tested.

---

## Build and Test Results

```
go build ./... → exit 0 (clean)
go test ./... -count=1 -timeout 120s → exit 0 (all 6 packages pass)
go vet ./... → exit 0 (clean)
gofmt -l . → empty (all formatted)
```

---

## All Verification Commands (exact commands from acceptance contract)

| Cmd | Result | Output |
|-----|--------|--------|
| V-1: `grep -r 'database/sql/driver' pkg/ui/` | exit 1 | No matches |
| V-2: `grep 'GolemUI/pkg/db' pkg/ui/compositor.go` | exit 1 | No matches |
| V-3: `grep -E 'var (BusinessPool\|CorePool)' pkg/ui/compositor.go` | exit 1 | No matches |
| V-4: `grep -n 'SetColumnWidth(i, 150)' pkg/ui/compositor.go` | exit 1 | No matches |
| V-5: `go test ./... -count=1 -timeout 120s` | exit 0 | All 6 packages ok |
| V-6: `go build ./...` | exit 0 | Clean build |
| V-7: `grep 'type DataSource interface' pkg/ui/datasource.go` | exit 0 | Found `type DataSource interface {` |
| V-8: `grep 'type ColumnWidthResolver interface' pkg/ui/datasource.go` | exit 0 | Found `type ColumnWidthResolver interface {` |
| V-9: `go build ./pkg/dataaccess/` | exit 0 | Compiles clean |
| V-10: `grep 'default_column_width' docker/init-db/02_init_core.sql` | exit 0 | Column + INSERT present |
| V-11: `grep 'column_width' docker/init-db/02_init_core.sql` | exit 0 | Column present in mapeo_interfaz |
