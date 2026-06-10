# Tasks: grid-selection-button-nav

**Change ID:** `grid-selection-button-nav`
**Date:** 2026-06-09
**Specs:** 019 (Datagrid Native Type Preservation), 018 (Action Button State Navigation)
**Delivery:** Two chained PRs — PR-1 (Spec 019), PR-2 (Spec 018, depends on PR-1)
**Test runner:** `go test ./...`

---

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines (PR-1) | ~160 (prod: ~43 + tests: ~70 migrated + ~47 new) |
| Estimated changed lines (PR-2) | ~520 (prod: ~145 + tests: ~375 new) |
| Estimated changed lines (total) | ~680 |
| 400-line budget risk | **Medium** (PR-1 is safe; PR-2 exceeds threshold) |
| Chained PRs recommended | **Yes** |
| Suggested split | PR-1 → PR-2 (strict sequential, PR-2 depends on PR-1 merge) |
| Delivery strategy | ask-on-risk (PR-2 exceeds 400-line budget) |
| Chain strategy | stacked-to-main |

```
Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: stacked-to-main
400-line budget risk: Medium
```

---

## PR-1: Spec 019 — Datagrid Native Type Preservation

### T019-01: Change DataSet.Rows type from `[][]string` to `[][]any` ✅

**Description:** Change the `Rows` field in `DataSet` struct from `[][]string` to `[][]any`. Update the doc comment from "string-normalized cell values" to "cell values preserving native Go types". This is the foundational type change — all downstream consumers must be updated before compilation succeeds.

**Files affected:**
- `pkg/ui/datasource.go` (~3 lines changed)

**Dependencies:** None (first task)

**Estimated changed lines:** 3

**TDD phases:**
- **RED:** Write a compile-time assertion test in a new function `TestDataSet_RowsTypeIsAnySlice` that confirms `reflect.TypeOf(ui.DataSet{}.Rows).Elem().Elem().Kind() == reflect.Interface` (or equivalent). This test will fail until `Rows` is `[][]any`.
- **GREEN:** Change `Rows [][]string` to `Rows [][]any` in `datasource.go`. Update comment.
- **TRIANGULATE:** Add assertion that mock data with `int64(42)` and `float64(25.5)` values can be stored in `Rows` without conversion.
- **REFACTOR:** No refactor needed; type is minimal.

---

### T019-02: Add `unwrapNumeric` helper and refactor `SQLDataSource.Fetch` to store native types ✅

**Description:** In `pkg/dataaccess/sql_datasource.go`, remove the `FormatValue` loop from `Fetch`. Store `rows.Values()` directly into `DataSet.Rows` as `[][]any`. Add an `unwrapNumeric` helper function to unwrap `pgtype.Numeric` values to `float64` at the fetch boundary, keeping pgtype types out of the rest of the pipeline. Add the `"github.com/jackc/pgx/v5/pgtype"` import.

**Files affected:**
- `pkg/dataaccess/sql_datasource.go` (~25 lines changed)

**Dependencies:** T019-01

**Estimated changed lines:** 25

**TDD phases:**
- **RED:** Write `TestSQLDataSource_FetchPreservesNativeTypes` in `sql_datasource_test.go` that registers mock data with `{int64(42), "Alice", float64(99.5)}`, calls `Fetch`, and asserts `result.Rows[0][0] == int64(42)` and `result.Rows[0][2] == float64(99.5)`. Also write `TestUnwrapNumeric_ConvertsPgtypeNumericToFloat64` for the helper. Both fail initially.
- **GREEN:** Implement `unwrapNumeric(val any) any` and refactor `Fetch` to store native values via `rows.Values()` with `unwrapNumeric` applied per value.
- **TRIANGULATE:** Add test case with `bool(true)` in mock data to verify boolean preservation. Add test for non-Numeric passthrough (string, int, nil).
- **REFACTOR:** Ensure `unwrapNumeric` is clean and handles both `*pgtype.Numeric` and `pgtype.Numeric` value types.

---

### T019-03: Migrate `dataGridModel` fields from `[][]string` to `[][]any` ✅

**Description:** Change `rows` and `masterRows` fields in `dataGridModel` struct in `compositor.go` from `[][]string` to `[][]any`. Add import `"GolemUI/pkg/dataaccess"` for the FormatValue calls in the next task. This will cause compilation failures until UpdateCell and filterMasterRows are updated in T019-04 and T019-05.

**Files affected:**
- `pkg/ui/compositor.go` (~5 lines changed)

**Dependencies:** T019-01

**Estimated changed lines:** 5

**TDD phases:**
- **RED:** No new test needed — existing grid tests will fail to compile after this type change. Record the compilation failure.
- **GREEN:** Change `rows [][]string` to `rows [][]any` and `masterRows [][]string` to `masterRows [][]any`.
- **TRIANGULATE:** N/A (structural change)
- **REFACTOR:** N/A

---

### T019-04: Wrap `UpdateCell` callback with `FormatValue` for render-time conversion ✅

**Description:** In the `UpdateCell` closure of the `data_grid` case in `compositor.go`, change `label.SetText(row[id.Col])` to `label.SetText(dataaccess.FormatValue(row[id.Col]))`. This is the key render-time formatting step — cells now display string-formatted versions of native types.

**Files affected:**
- `pkg/ui/compositor.go` (~3 lines changed)

**Dependencies:** T019-03

**Estimated changed lines:** 3

**TDD phases:**
- **RED:** Existing test `TestCompose_DataGrid_Success` will fail at compilation because `row[id.Col]` is now `any` not `string`. After fixing compilation, the cell rendering assertion `lblCell.Text != "Book A"` must still pass.
- **GREEN:** Add `dataaccess.FormatValue(...)` wrapping. Verify all existing grid cell rendering tests pass with identical output.
- **TRIANGULATE:** New test `TestCompose_DataGrid_NativeTypes_RenderCorrectly` — mock with `int64(1)`, `"Book A"`, `float64(25.5)`, verify rendered cells are `"1"`, `"Book A"`, `"25.5"`.
- **REFACTOR:** Verify `containsIgnoreCase` still works with string inputs (it does — filterMasterRows handles the conversion).

---

### T019-05: Wrap `filterMasterRows` with `FormatValue` for substring comparison ✅

**Description:** In `filterMasterRows` function, change the type of `filtered` from `[][]string` to `[][]any`. Change `cellVal := row[col]` to `cellVal := dataaccess.FormatValue(row[col])` so substring matching works on string-formatted values. The `containsIgnoreCase` function already expects strings.

**Files affected:**
- `pkg/ui/compositor.go` (~5 lines changed)

**Dependencies:** T019-03

**Estimated changed lines:** 5

**TDD phases:**
- **RED:** Existing `TestCompose_DataGrid_ClientMode_EagerLoadAndFilter` will fail to compile after T019-03 due to type mismatch. Record the failure.
- **GREEN:** Change `filtered` type, wrap cell value with `FormatValue`. Verify all client-mode filter tests pass.
- **TRIANGULATE:** New test `TestFilterMasterRows_NativeTypes_SubstringMatch` — master data with `int64(1)`, `"Foundation"`, `"Asimov"`; filter by "Found"; verify row matches.
- **REFACTOR:** No additional refactor needed.

---

### T019-06: Migrate test fixtures from `[][]string` to `[][]any` ✅

**Description:** Update ALL test fixture `Rows` values across `compositor_test.go` and `sql_datasource_test.go` from `[][]string{{"1", "Book A", "25.5"}}` to `[][]any{{1, "Book A", 25.5}}` with appropriate native types. Update selection assertion tests to expect native types instead of strings (e.g., `"42"` → `int(42)`). Remove the compile-time string check `var _ string = cell` from `TestSQLDataSource_FetchNormalizesValuerTypes`.

**Files affected:**
- `pkg/ui/compositor_test.go` (~50 lines changed)
- `pkg/dataaccess/sql_datasource_test.go` (~15 lines changed)

**Dependencies:** T019-02, T019-04, T019-05

**Estimated changed lines:** 65

**TDD phases:**
- **RED:** All tests should be failing (either compilation errors or assertion mismatches) from the type changes in T019-01 through T019-05. Run `go test ./pkg/ui/ ./pkg/dataaccess/...` and record failures.
- **GREEN:** Convert all fixture data to native types. Update the selection assertion in `TestCompose_DataGrid_RowSelection_PublishesToSelectionChannel` from `map[string]any{"id": "42", ...}` to `map[string]any{"id": 42, ...}`. Update `TestSQLDataSource_FetchNormalizesValuerTypes` to assert native types instead of strings.
- **TRIANGULATE:** Verify each test individually passes. No additional edge cases needed here — this is fixture migration.
- **REFACTOR:** Review for consistency across all tests (all `int` values should be `int`, not `int64`, matching what `MockDBPool` returns).

---

### T019-07: Add dedicated type preservation test ✅

**Description:** Write a new focused test `TestCompose_DataGrid_SelectionPreservesNativeTypes` that verifies the complete pipeline: mock data with mixed native types → Fetch → grid display → row selection → `publish_selection` event carries native types. Assert `int`, `float64`, `bool`, and `string` types in the published payload.

**Files affected:**
- `pkg/ui/compositor_test.go` (~30 lines added)

**Dependencies:** T019-06

**Estimated changed lines:** 30

**TDD phases:**
- **RED:** Write the test with assertions for `int64(42)`, `bool(true)`, `float64(1000.5)`, `"text"` in the selection payload. This test should already pass after T019-06 migration, but write it first to confirm the behavior contract.
- **GREEN:** Test should pass from RED if T019-06 is complete.
- **TRIANGULATE:** Add edge case: nil value in a cell should remain nil in payload, not empty string.
- **REFACTOR:** Extract common grid+eventbus setup into a test helper if it reduces duplication.

---

### T019-08: Verify all tests pass for PR-1 ✅

**Description:** Run full test suite `go test ./...`. Verify `go build ./...` passes. Verify `go vet ./...` passes. All existing and new tests must pass with native type preservation.

**Files affected:** None (verification only)

**Dependencies:** All T019 tasks (T019-01 through T019-07)

**Estimated changed lines:** 0

**TDD phases:**
- **RED:** N/A
- **GREEN:** N/A
- **TRIANGULATE:** N/A
- **REFACTOR:** N/A

---

### PR-1 Summary

| Task | File(s) | Est. Lines | Type |
|------|---------|-----------|------|
| T019-01 | `datasource.go` | 3 | Prod |
| T019-02 | `sql_datasource.go`, `sql_datasource_test.go` | 25 | Prod + Test |
| T019-03 | `compositor.go` | 5 | Prod |
| T019-04 | `compositor.go` | 3 | Prod |
| T019-05 | `compositor.go` | 5 | Prod |
| T019-06 | `compositor_test.go`, `sql_datasource_test.go` | 65 | Test |
| T019-07 | `compositor_test.go` | 30 | Test |
| T019-08 | — | 0 | Verify |
| **Total** | | **~136** | |

---

## PR-2: Spec 018 — Action Button State Navigation (depends on PR-1)

### T018-01: ✅ Add `ParamMapping` field to `NodeMeta`

**Description:** Add `ParamMapping map[string]string` with `json:"param_mapping,omitempty"` tag to the `NodeMeta` struct in `compositor.go`. This is a pure data structure addition — no behavioral change yet.

**Files affected:**
- `pkg/ui/compositor.go` (~2 lines added)

**Dependencies:** PR-1 merged

**Estimated changed lines:** 2

**TDD phases:**
- **RED:** Write `TestNodeMeta_ParamMappingField` that creates a `NodeMeta` with `ParamMapping: map[string]string{"id": "row.id"}`, marshals to JSON, and verifies the key `param_mapping` exists. Fails until field is added.
- **GREEN:** Add `ParamMapping map[string]string \`json:"param_mapping,omitempty"\`` to `NodeMeta`.
- **TRIANGULATE:** Verify `omitempty` — empty mapping produces no JSON key.
- **REFACTOR:** N/A

---

### T018-02: ✅ Add `ScreenState.Preload` method

**Description:** Add `Preload(params map[string]any)` method to `ScreenState` in `screen_state.go`. Method takes a lock, iterates params, and only writes keys that do NOT already exist in `data` (no-overwrite semantics). This enables injecting navigation query parameters before composition.

**Files affected:**
- `pkg/ui/screen_state.go` (~10 lines added)

**Dependencies:** PR-1 merged

**Estimated changed lines:** 10

**TDD phases:**
- **RED:** Write `TestScreenState_Preload_InjectsNewKeys` (calls Preload, verifies Get returns value) and `TestScreenState_Preload_NoOverwrite` (calls Set first, then Preload with different value for same key, verifies original preserved). Both fail until method exists.
- **GREEN:** Implement `Preload` with mutex lock, range loop, existence check.
- **TRIANGULATE:** Add `TestScreenState_Preload_NilMap` (no-op, no panic). Add `TestScreenState_Preload_MergeExisting` (existing key untouched, new key added).
- **REFACTOR:** N/A

---

### T018-03: ✅ Write Preload tests (additional edge cases)

**Description:** Write comprehensive tests for `ScreenState.Preload` including concurrent Preload+Set scenarios, Preload with empty map, and Preload then Snapshot contains all keys.

**Files affected:**
- `pkg/ui/screen_state_test.go` (~40 lines added)

**Dependencies:** T018-02

**Estimated changed lines:** 40

**TDD phases:**
- **RED:** Write `TestScreenState_Preload_ConcurrentWithSet` and `TestScreenState_Preload_SnapshotAfterPreload`. Both should compile but may fail until T018-02 GREEN.
- **GREEN:** Verify all Preload tests pass.
- **TRIANGULATE:** `TestScreenState_Preload_GetReturnsStringType` — preload `map[string]any{"id": "99"}`, verify `Get("id")` returns `"99"`.
- **REFACTOR:** Group all Preload tests together in the file.

---

### T018-04: ✅ Implement `buildQueryParams` and `composeReactiveNavButton` helper

**Description:** Add two new functions to `compositor.go`:
1. `buildQueryParams(selection map[string]any, mapping map[string]string) string` — iterates mapping, resolves each path via `resolvePath`, URL-encodes key and value, sorts parts, joins with `&`. Returns empty string if no valid params.
2. `resolvePath` already exists — reuse it.

Also add `composeReactiveNavButton` function (full implementation deferred to T018-06 — this task focuses on `buildQueryParams` and its unit tests).

New imports: `"net/url"`, `"sort"`.

**Files affected:**
- `pkg/ui/compositor.go` (~25 lines added for buildQueryParams)

**Dependencies:** T018-01, PR-1 merged

**Estimated changed lines:** 25

**TDD phases:**
- **RED:** Write `TestBuildQueryParams_SimpleMapping` (selection `{"id": 42, "type": "debit"}`, mapping `{"id": "row.id", "type": "row.type"}`, expects `"id=42&type=debit"` or sorted order). Write `TestBuildQueryParams_NestedPath` (nested selection with `user.profile.id`). Write `TestBuildQueryParams_InvalidPath_Skipped`. Write `TestBuildQueryParams_EmptyMapping`. All fail.
- **GREEN:** Implement `buildQueryParams`.
- **TRIANGULATE:** `TestBuildQueryParams_URLSpecialChars` — value with spaces/special chars is URL-encoded. `TestBuildQueryParams_NilValue_Skipped` — nil resolved value skips that param.
- **REFACTOR:** Verify deterministic sort order for test stability.

---

### T018-05: ✅ Export `ComposeWithParams` function

**Description:** Add exported `ComposeWithParams(node NodeMeta, vistaID string, params map[string]string) (fyne.CanvasObject, func(), error)` to `compositor.go`. Internally creates `NewScreenState(vistaID)`, calls `state.Preload(anyMap)` if params non-empty, then delegates to `composeWithState`. This preserves backward compatibility of `Compose` while enabling parameter injection for navigation.

**Files affected:**
- `pkg/ui/compositor.go` (~12 lines added)

**Dependencies:** T018-02, T018-04

**Estimated changed lines:** 12

**TDD phases:**
- **RED:** Write `TestComposeWithParams_InjectsParams` — compose with params `{"id": "99"}`, then verify a child label with `BindTo: "id"` (or direct state access) receives the value. Fails until function exists.
- **GREEN:** Implement `ComposeWithParams`. Convert `map[string]string` to `map[string]any`, call `Preload`, delegate to `composeWithState`.
- **TRIANGULATE:** `TestComposeWithParams_EmptyParams` — behaves identically to `Compose`. `TestComposeWithParams_NoOverwrite` — existing state keys preserved.
- **REFACTOR:** Ensure `Compose` and `ComposeWithParams` share the same `composeWithState` delegate.

---

### T018-06: ✅ Implement `composeReactiveNavButton` — reactive button composition

**Description:** Add `composeReactiveNavButton` function to `compositor.go` and integrate it into the `"button"` case of `composeWithState`. Branch logic: when `node.SubmitAction` has `navigate:` prefix AND `node.DataSource != ""` AND `ui.LocalEventBus != nil`, call `composeReactiveNavButton`. Otherwise fall through to existing static/submit/inert branches.

`composeReactiveNavButton`:
- Creates button with `btn.Disable()` (starts disabled)
- Subscribes to `parseChannelName(node.DataSource)` (typically `"publish_selection"`)
- On valid `map[string]any` payload: stores in `lastSelection` (guarded by `sync.Mutex`), enables via `fyne.Do`
- On nil/empty payload: disables via `fyne.Do`
- Click handler: if button enabled, resolves `node.ParamMapping` via `buildQueryParams`, calls `Navigate(vistaID + "?" + queryParams)`
- Cleanup: `sync.Once` unsubscribes from channel

**Files affected:**
- `pkg/ui/compositor.go` (~55 lines added)

**Dependencies:** T018-01, T018-04, PR-1 merged

**Estimated changed lines:** 55

**TDD phases:**
- **RED:** Write `TestCompose_Button_ReactiveNav_StartsDisabled` — compose a button with `DataSource: "publish_selection"`, `SubmitAction: "navigate:detalle"`, verify `btn.Disabled() == true`. Write `TestCompose_Button_ReactiveNav_EnablesOnSelection` — publish valid selection, verify enabled. Both fail.
- **GREEN:** Implement `composeReactiveNavButton` and the branching in the button case.
- **TRIANGULATE:** `TestCompose_Button_ReactiveNav_DisablesOnDeselection` — enable via selection, then publish empty/nil payload, verify disabled. `TestCompose_Button_ReactiveNav_ClickNavigatesWithParams` — click after selection with param_mapping, verify Navigate called with `"detalle?id=42"`.
- **REFACTOR:** Extract common patterns (fyne.Do enable/disable, sync.Once cleanup) if reusable.

---

### T018-07: ✅ Implement `parseNavigateTarget` and update `Navigate` callback in `main.go`

**Description:** Add `parseNavigateTarget(vID string) (string, map[string]string)` to `cmd/golemui/main.go`. Splits on first `?`, URL-parses query string. Edge cases: no `?` → clean vistaID, empty query → nil params, empty vistaID before `?` → treat as plain string.

Update the `ui.Navigate` callback to:
1. Call `parseNavigateTarget(vID)`
2. Load layout with clean vistaID
3. If params non-nil, call `ui.ComposeWithParams(node, cleanVistaID, params)` instead of `ui.Compose`

**Files affected:**
- `cmd/golemui/main.go` (~35 lines changed)

**Dependencies:** T018-05

**Estimated changed lines:** 35

**TDD phases:**
- **RED:** Write `TestParseNavigateTarget_PlainVistaID` (no `?` → clean ID, nil params), `TestParseNavigateTarget_WithParams` (`"detalle?id=99&tipo=debito"` → `"detalle"`, `{"id":"99","tipo":"debito"}`), `TestParseNavigateTarget_EmptyQuery` (`"vista?"` → `"vista"`, nil). Write `TestParseNavigateTarget_Malformed` (no panic). All fail.
- **GREEN:** Implement `parseNavigateTarget`.
- **TRIANGULATE:** `TestParseNavigateTarget_URLDecodedValues` — encoded values are properly decoded.
- **REFACTOR:** Keep `parseNavigateTarget` as a pure function, testable without Fyne.

---

### T018-08: ✅ Write reactive button integration tests

**Description:** Write comprehensive tests in new file `pkg/ui/compositor_button_test.go` covering the full reactive button lifecycle. At least 12 test cases covering: initial disabled state, enable on valid selection, disable on empty selection, enable/disable toggle cycles, click with param_mapping builds correct query string, click while disabled does nothing, nil EventBus fallback to static navigation, cleanup unsubscribes, idempotent cleanup, multiple buttons on same channel, button with navigate but no DataSource stays static.

**Files affected:**
- `pkg/ui/compositor_button_test.go` (~250 lines added, new file)

**Dependencies:** T018-06, T018-07

**Estimated changed lines:** 250

**TDD phases:**
- **RED:** Write all 12+ test cases. Most will fail until T018-06 GREEN. Record failures.
- **GREEN:** All tests pass after T018-06 and T018-07 implementation.
- **TRIANGULATE:** Add edge cases: selection with nested param_mapping paths, param_mapping with unresolvable path (button still navigates without that param), rapid selection/deselection events.
- **REFACTOR:** Extract test helpers for common setup (eventbus, mockDS, compose).

---

### T018-09: ✅ Write Navigate query string parsing tests

**Description:** Write tests for `parseNavigateTarget` covering edge cases: empty string, string with only `?`, multiple `?` characters (only first matters), special characters in values, empty values (`"key="`), no value after `=`.

**Files affected:**
- `cmd/golemui/main_test.go` (~60 lines added, check if file exists)

**Dependencies:** T018-07

**Estimated changed lines:** 60

**TDD phases:**
- **RED:** Write edge case tests.
- **GREEN:** Tests pass after T018-07 implementation.
- **TRIANGULATE:** Add `TestParseNavigateTarget_MultipleQuestionMarks` — only first `?` splits.
- **REFACTOR:** N/A

---

### T018-10: ✅ Verify all tests pass for PR-2

**Description:** Run full test suite `go test ./...`. Verify `go build ./...` passes. Verify `go vet ./...` passes. Optionally run `golangci-lint run`. All existing and new tests must pass.

**Files affected:** None (verification only)

**Dependencies:** All T018 tasks (T018-01 through T018-09)

**Estimated changed lines:** 0

**TDD phases:**
- **RED:** N/A
- **GREEN:** N/A
- **TRIANGULATE:** N/A
- **REFACTOR:** N/A

---

### PR-2 Summary

| Task | File(s) | Est. Lines | Type |
|------|---------|-----------|------|
| T018-01 | `compositor.go` | 2 | Prod |
| T018-02 | `screen_state.go` | 10 | Prod |
| T018-03 | `screen_state_test.go` | 40 | Test |
| T018-04 | `compositor.go` | 25 | Prod |
| T018-05 | `compositor.go` | 12 | Prod |
| T018-06 | `compositor.go` | 55 | Prod |
| T018-07 | `main.go` | 35 | Prod |
| T018-08 | `compositor_button_test.go` (new) | 250 | Test |
| T018-09 | `main_test.go` | 60 | Test |
| T018-10 | — | 0 | Verify |
| **Total** | | **~489** | |

---

## Execution Order (Full Dependency Graph)

```
PR-1 (Spec 019 — Type Preservation):
  T019-01 ──→ T019-02 ──┐
         ──→ T019-03 ──→ T019-04 ──┐
                           T019-05 ──┼──→ T019-06 ──→ T019-07 ──→ T019-08
                                        │
                                        └──→ T019-06 (both must complete)

PR-2 (Spec 018 — Button State Nav, after PR-1 merge):
  T018-01 ──→ T018-04 ──→ T018-06 ──→ T018-08 ──→ T018-10
  T018-02 ──→ T018-03 ──→ T018-05 ──→ T018-07 ──→ T018-09 ──→ T018-10
```

Parallelizable within each PR:
- T019-02 and T019-03 can proceed in parallel after T019-01
- T019-04 and T019-05 can proceed in parallel after T019-03
- T018-01 and T018-02 can proceed in parallel (no dependency between them)
- T018-04 and T018-03 can proceed in parallel
- T018-06 and T018-05 can proceed in parallel after their respective deps

---

## Risk Notes

1. **PR-2 exceeds 400-line budget** (~489 lines). Delivery strategy is `ask-on-risk` per `openspec/config.yaml`. Recommend confirming with user before starting PR-2 apply.
2. **Test file size**: `compositor_test.go` is already ~2461 lines. PR-2 creates a separate `compositor_button_test.go` to avoid growing it further.
3. **pgtype dependency**: PR-1 adds `"github.com/jackc/pgx/v5/pgtype"` import. Verify this module is already a dependency (it should be, via the existing pgx pool usage).
4. **Thread safety**: All reactive button UI mutations must use `fyne.Do()`. Test with async EventBus delivery to confirm.
5. **Backward compatibility**: `Compose` signature unchanged. `Navigate` signature unchanged. `ComposeWithParams` is additive.