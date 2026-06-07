# Tasks: data-grid-row-selection

## Review Workload

| Field | Value |
|-------|-------|
| Estimated changed lines | ~75 (15 prod + 60 test) |
| 400-line budget risk | None |
| Chained PRs recommended | No |

---

## Phase 1: Implementation (TDD)

### T-1.1: RED — Test row selection publishes correct map
**File**: `pkg/ui/compositor_test.go`
**Action**: Write `TestCompose_DataGrid_RowSelection_PublishesToSelectionChannel`
- Set up `LocalEventBus = eventbus.NewEventBus()`
- Subscribe to `"publish_selection"` with sync.WaitGroup handler
- Compose `data_grid` with mock pool returning `["id","nombre","monto"]` headers and `[["42","Transaccion Test","1000.50"]]` row
- Wait for async data load
- Call `table.OnSelected(widget.TableCellID{Row: 0, Col: 0})`
- Assert payload is `map[string]any{"id":"42","nombre":"Transaccion Test","monto":"1000.50"}`

**Acceptance**: Test fails to compile or fails at assertion (OnSelected is nil).

### T-1.2: GREEN — Implement OnSelected callback
**File**: `pkg/ui/compositor.go`
**Action**: Add `table.OnSelected` block after EventBus subscription, before `return table, nil`:
- Nil check on `LocalEventBus`
- RLock model, bounds check row index, read headers + row, RUnlock
- Build `map[string]any` with min(headers, row) loop
- Publish to `"publish_selection"`

**Acceptance**: T-1.1 passes.

### T-1.3: RED — Test out-of-bounds row is no-op
**File**: `pkg/ui/compositor_test.go`
**Action**: Write `TestCompose_DataGrid_RowSelection_OutOfBounds`
- Same setup as T-1.1 but call `table.OnSelected(widget.TableCellID{Row: 99, Col: 0})`
- Assert no event received (timeout on WaitGroup)

**Acceptance**: Test passes immediately (bounds check was implemented in T-1.2).

### T-1.4: RED — Test nil LocalEventBus is safe
**File**: `pkg/ui/compositor_test.go`
**Action**: Write `TestCompose_DataGrid_RowSelection_NilEventBus`
- Set `ui.LocalEventBus = nil`
- Compose data_grid with mock pool
- Wait for load
- Call `table.OnSelected(widget.TableCellID{Row: 0, Col: 0})`
- Assert no panic

**Acceptance**: Test passes immediately (nil check was implemented in T-1.2).

### T-1.5: RED — Test second row selection
**File**: `pkg/ui/compositor_test.go`
**Action**: Write `TestCompose_DataGrid_RowSelection_SecondRow`
- Mock pool returns 2 rows
- Select row 1
- Assert second row's data is published

**Acceptance**: Test passes immediately (row index is used correctly from T-1.2).

---

## Phase 2: Verification

### T-2.1: Full test suite
**Command**: `go test ./... -count=1`
**Acceptance**: All tests pass.

### T-2.2: go vet clean
**Command**: `go vet ./...`
**Acceptance**: No output.
