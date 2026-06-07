# Design: data-grid-row-selection

## Insertion Point

**File**: `pkg/ui/compositor.go`
**Function**: `composeWithState`
**Case**: `"data_grid"` (line ~130)
**Location**: After the EventBus subscription block (after `model.mu.Unlock()` following the unsubscribe assignment, ~line 237) and before `return table, nil` (line ~238).

## Code Change

Insert the following block between the subscription block and `return table, nil`:

```go
// Row selection: publish selected row data to "publish_selection" channel
table.OnSelected = func(id widget.TableCellID) {
    if LocalEventBus == nil {
        return
    }
    model.mu.RLock()
    if id.Row < 0 || id.Row >= len(model.rows) {
        model.mu.RUnlock()
        return
    }
    row := model.rows[id.Row]
    headers := model.headers
    model.mu.RUnlock()

    rowMap := make(map[string]any, len(headers))
    for i := 0; i < len(headers) && i < len(row); i++ {
        rowMap[headers[i]] = row[i]
    }
    LocalEventBus.Publish("publish_selection", rowMap)
}
```

## Thread Safety Analysis

| Concern | Resolution |
|---------|-----------|
| `OnSelected` fires on UI thread | Yes — Fyne dispatches click events on UI thread |
| `model.mu.RLock` protects headers/rows read | Yes — same pattern as `UpdateCell` and `Length` callbacks |
| `LocalEventBus.Publish` is goroutine-safe | Yes — `InMemEventBus` uses `RLock` internally and dispatches via `go h(event)` |
| No concurrent mutation in callback | Correct — callback only reads under RLock |
| `rowMap` is local to callback | Correct — new allocation per click, no shared state |

## Why this location

- After all model/table setup is complete
- After subscription block — model is fully configured
- `model` and `table` are both in closure scope
- `LocalEventBus` is a package-level var — accessible
- Does not interfere with async data loading or reactive filtering

## Test Design

The acceptance test (S1) follows the existing pattern in `compositor_test.go`:

1. Create `eventbus.NewEventBus()` and assign to `ui.LocalEventBus`
2. Subscribe to `"publish_selection"` with a capturing handler
3. Compose a `data_grid` node with a mock `BusinessPool` returning known data
4. Wait for async data load (poll `table.Length()` up to 500ms)
5. Call `table.OnSelected(widget.TableCellID{Row: 0, Col: 0})`
6. Wait for EventBus goroutine dispatch
7. Assert received payload matches expected `map[string]any`

**Key difference from existing data_grid tests**: No mock pool needed for S2-S5 — manually populate `dataGridModel` via exported table callbacks, OR compose with empty DataSource and set model internals. Since `dataGridModel` is unexported, tests must use the same pattern as existing tests: compose a grid, wait for load, then trigger OnSelected.

For S5 (row shorter than headers), this scenario cannot naturally occur with the current data loading (all rows come from SQL with same column count). The test can be skipped or noted as a defensive guard — the `i < len(row)` check is defensive programming, not testable from outside without modifying the internal model.

## File Manifest

| File | Change | Lines |
|------|--------|-------|
| `pkg/ui/compositor.go` | Add `table.OnSelected` block | +15 |
| `pkg/ui/compositor_test.go` | Add 4 new test functions | +60 |

Total: ~75 lines added. No other files modified.
