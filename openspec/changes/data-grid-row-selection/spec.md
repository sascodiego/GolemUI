# Spec: data-grid-row-selection

## Requirements

### REQ-SEL-01: OnSelected callback assigned
The `widget.Table` created in the `data_grid` case MUST have its `OnSelected` callback assigned before returning from `composeWithState`.

### REQ-SEL-02: Row bounds validation
The callback MUST validate that `id.Row >= 0 && id.Row < len(model.rows)` under `model.mu.RLock`. If out of bounds, the callback MUST return without publishing.

### REQ-SEL-03: Header-value map construction
The callback MUST build a `map[string]any` by iterating over `model.headers` and `model.rows[id.Row]` positionally. Each key is `model.headers[i]`, each value is `model.rows[id.Row][i]` (as `string` cast to `any`). If a row has fewer columns than headers, the missing keys MUST be omitted from the map (no index-out-of-bounds).

### REQ-SEL-04: EventBus publish
The callback MUST call `LocalEventBus.Publish("publish_selection", rowMap)` if `LocalEventBus` is not nil. The channel name is exactly `"publish_selection"`.

### REQ-SEL-05: Nil EventBus safety
If `LocalEventBus` is nil, the callback MUST be a no-op (no panic, no error).

## Test Scenarios (TDD)

### S1: Row selection publishes correct map
- **RED**: Test that selects row 0 on a table with headers `["id","nombre","monto"]` and row `["42","Transaccion Test","1000.50"]` expects `map[string]any{"id":"42","nombre":"Transaccion Test","monto":"1000.50"}` on channel `"publish_selection"`.
- **GREEN**: Implement OnSelected callback.

### S2: Out-of-bounds row is no-op
- **RED**: Test selecting a row with index beyond `len(model.rows)` — no event published, no panic.
- **GREEN**: Bounds check in callback.

### S3: Nil LocalEventBus is safe
- **RED**: Set `LocalEventBus = nil`, trigger OnSelected — no panic.
- **GREEN**: Nil check before Publish.

### S4: Multiple rows, select second
- **RED**: Two rows, select row 1 — verify second row's data is published.
- **GREEN**: Triangulation that row index is used correctly.

### S5: Row with fewer columns than headers
- **RED**: Headers `["a","b","c"]`, row `["x","y"]` — verify map has only `{"a":"x","b":"y"}` (no panic on index 2).
- **GREEN**: min(headers, row) bounds in loop.

## Edge Cases

| ID | Case | Expected |
|----|------|----------|
| E1 | Empty model (no headers, no rows) | No publish, no panic |
| E2 | Headers exist but rows empty | Bounds check fails, no publish |
| E3 | Rapid row selection | Each click publishes its own map; EventBus handles concurrency |

## Out of Scope

- Column selection (`id.Col` is ignored — entire row is published)
- Multi-row selection
- Selection highlighting persistence
- Any changes to data loading, filtering, or other components
