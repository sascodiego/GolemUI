# Proposal: data-grid-row-selection

## Problem

The `data_grid` component (`widget.Table` in Fyne) renders tabular data but has no row selection capture. When a user clicks a row, nothing happens — the selected row data is lost. Downstream consumers (detail panels, action buttons, master-detail patterns) cannot react to the user's selection.

## Proposed Solution

Assign a `table.OnSelected` callback in the `data_grid` case of `composeWithState`. The callback:

1. Validates the selected row index against `model.rows` bounds (under `model.mu.RLock`)
2. Builds a `map[string]any` by pairing `model.headers[i]` with `model.rows[row][i]`
3. Publishes the map to `LocalEventBus.Publish("publish_selection", rowMap)`

### Thread Safety

- `OnSelected` fires on the Fyne UI thread (user click event)
- `model.mu.RLock` protects the read of `headers` and `rows`
- `EventBus.Publish` is goroutine-safe internally (dispatches handlers via `go`)
- No concurrent mutation risk: the callback only reads

### Scope

- **In scope**: `table.OnSelected` assignment, row-to-map conversion, EventBus publish
- **Out of scope**: fetchGridDataAsync, loadMasterBuffer, filterMasterRows, any other component, any DB changes, any UI layout changes

## Impact

- **Files changed**: `pkg/ui/compositor.go` (~15 lines added), `pkg/ui/compositor_test.go` (~50 lines added)
- **Risk**: Low — additive change, no existing behavior modified
- **No new types, no new files, no new dependencies**

## Rollback

Remove the `table.OnSelected` block. No schema, no migration, no config changes.
