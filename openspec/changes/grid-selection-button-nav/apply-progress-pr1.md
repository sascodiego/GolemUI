# Apply Progress: PR-1 (Spec 019 — Datagrid Native Type Preservation)

**Change:** `grid-selection-button-nav`
**Date:** 2026-06-09
**Mode:** Strict TDD
**Test runner:** `go test ./...`

---

## Task Progress

- [x] T019-01: DataSet.Rows type change
- [x] T019-02: unwrapNumeric + Fetch refactor
- [x] T019-03: dataGridModel type changes
- [x] T019-04: UpdateCell FormatValue
- [x] T019-05: filterMasterRows FormatValue
- [x] T019-06: Test fixture migration
- [x] T019-07: Type preservation test
- [x] T019-08: Verify all pass

---

## TDD Cycle Evidence

| Task | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----|-------|-------------|----------|
| T019-01 | `TestDataSet_RowsTypeIsAnySlice` + `TestDataSet_RowsCanHoldNativeTypes` — compile failure on `[][]any` vs `[][]string` | Changed `Rows [][]string` → `[][]any` in `datasource.go` | `TestDataSet_RowsCanHoldNativeTypes` verifies `int64`, `float64`, `bool` storage | N/A — minimal type change |
| T019-02 | `TestUnwrapNumeric_*` + `TestSQLDataSource_FetchPreservesNativeTypes` — `unwrapNumeric` undefined | Added `unwrapNumeric` helper, refactored `Fetch` to store native `[]any` from `rows.Values()` | Bool preservation, nil passthrough, non-Numeric passthrough | `UnwrapNumeric` exported wrapper for test access |
| T019-03 | Existing grid tests fail to compile after type change | Changed `rows` and `masterRows` to `[][]any` | N/A — structural change | N/A |
| T019-04 | `UpdateCell` fails: `any` not assignable to `string` parameter | Added `formatCellValue(row[id.Col])` wrapper | `TestCompose_DataGrid_NativeTypes_RenderCorrectly` verifies visual output | N/A |
| T019-05 | `filterMasterRows` fails: `any` not assignable to `string` parameter | Added `formatCellValue(row[col])`, changed `filtered` to `[][]any` | `TestFilterMasterRows_NativeTypes_SubstringMatch` verifies substring match on native types | N/A |
| T019-06 | All `Rows: [][]string{...}` fixtures fail type check | Global `sed` replacement to `[][]any` | All existing tests pass with string values (strings satisfy `any`) | N/A |
| T019-07 | `TestCompose_DataGrid_SelectionPreservesNativeTypes` with typed mock data | Test passes immediately after T019-06 | Added nil cell edge consideration, render verification test, filter substring test | N/A |

---

## Deviations from Design

1. **Import cycle resolution**: Design specified adding `"GolemUI/pkg/dataaccess"` import to `compositor.go`. This is impossible because `dataaccess` imports `ui`. Instead, added `formatCellValue(val any) string` helper directly in `compositor.go` that mirrors `FormatValue` behavior for plain Go types. This is acceptable because `unwrapNumeric` (T019-02) already unwraps `pgtype.Numeric` at the fetch boundary, so only plain types reach the compositor.

---

## Files Changed

- `pkg/ui/datasource.go` — `Rows [][]string` → `[][]any`
- `pkg/dataaccess/sql_datasource.go` — Added `unwrapNumeric`, `UnwrapNumeric`, refactored `Fetch` to store native types, added `pgtype` import
- `pkg/ui/compositor.go` — `dataGridModel.rows/masterRows` → `[][]any`, `UpdateCell` with `formatCellValue`, `filterMasterRows` with `formatCellValue`, added `formatCellValue` helper
- `pkg/ui/compositor_test.go` — Migrated all `Rows: [][]string{...}` to `[][]any{...}`, added 3 new tests
- `pkg/dataaccess/sql_datasource_test.go` — Updated assertions for native types, removed compile-time string check, added new tests
- `pkg/ui/compositor_test_internal_test.go` — Added `TestDataSet_RowsTypeIsAnySlice` and `TestDataSet_RowsCanHoldNativeTypes`
