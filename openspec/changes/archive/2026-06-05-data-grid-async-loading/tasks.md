# Tasks: Data Grid Async Loading

## Tasks Breakdown

### Phase 1: Shared Models & Types
- [x] Define `ui.BusinessPool` variable as `db.DatabasePool` in [compositor.go](file:///src/GolemUI/pkg/ui/compositor.go).
- [x] Define `dataGridModel` struct with fields `mu sync.RWMutex`, `columns []string`, and `rows [][]string` in [compositor.go](file:///src/GolemUI/pkg/ui/compositor.go).

### Phase 2: MockDB Update
- [x] Add `FieldDescriptions() []pgconn.FieldDescription` to `MockRows` in [mock_db.go](file:///src/GolemUI/pkg/db/mock_db.go) to return fields mapped from `m.columns`.

### Phase 3: Compositor Logic for data_grid
- [x] In `Compose()`, map the `data_grid` component ref to Fyne's `widget.NewTableWithHeaders`.
- [x] Configure `widget.Table` callbacks: `Length`, `CreateCell`, `UpdateCell`, `CreateHeader`, and `UpdateHeader`.
- [x] Implement `fetchGridDataAsync` function to run query, convert fields and rows to string data, update model state thread-safely, and call `table.Refresh()` inside `fyne.Do()`.
- [x] Trigger `fetchGridDataAsync` in `Compose()` when `node.DataSource` is not empty.

### Phase 4: Inject Business Pool
- [x] Inject `dbPool.BusinessPool` into `ui.BusinessPool` inside `RunBootstrap()` in [main.go](file:///src/GolemUI/cmd/golemui/main.go).

### Phase 5: Testing
- [x] Add tests in [compositor_test.go](file:///src/GolemUI/pkg/ui/compositor_test.go) to verify composition of `"data_grid"`.
- [x] In tests, inject a `MockDBPool` preloaded with column and row query stubs into `ui.BusinessPool`, call `Compose()`, poll/wait for async loading, and assert cell content and header names.

## Implementation Order Rationale
Phase 1 and 2 establish the structures and mocks needed for compiling and testing. Phase 3 implements the async table compositor, using the mocked components. Phase 4 integrates the pool in main. Phase 5 tests the integrated components in a headless environment.

## Review Workload Forecast
Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: stacked-to-main
400-line budget risk: Low
