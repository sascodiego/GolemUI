# Proposal: Dynamic Grid Filtering

## Capabilities

| Capability | Type | Description |
|---|---|---|
| `reactive-input-publishing` | New | Emits input entry changes to the local event bus under a bound topic. |
| `parametrized-grid-filtering` | New | Subscribes to topics and runs parameterized queries using event payloads. |
| `component-event-binding` | Modified | Extends UI composition to bind widgets to event-driven communication. |

## Affected Areas

- [cmd/golemui/main.go](file:///src/GolemUI/cmd/golemui/main.go)
- [pkg/ui/compositor.go](file:///src/GolemUI/pkg/ui/compositor.go)
- [pkg/ui/compositor_test.go](file:///src/GolemUI/pkg/ui/compositor_test.go)

## Approach & Intent

1. **Bootstrap Wiring**: In `cmd/golemui/main.go`, assign the initialized `EventBus` to `ui.LocalEventBus` during application startup.
2. **Import eventbus**: In `pkg/ui/compositor.go`, import `GolemUI/pkg/eventbus` and declare `var LocalEventBus eventbus.EventBus`.
3. **TextInput Event Publishing**: Map `"text_input"` in `Compose` to publish changes on `entry.OnChanged` to `ui.LocalEventBus` under the channel defined in `node.BindTo`.
4. **DataGrid Subscription & Parameterized Filtering**: Map `"data_grid"` in `Compose` to subscribe to the channel defined in `node.BindTo`. When an event fires, trigger a background goroutine to execute `fetchGridDataAsync` with the event payload (using native parameterized query `$1` on `ui.BusinessPool.Query`).
5. **Race Condition Prevention**: Cancel stale/pending queries to prevent out-of-order writes using context cancellation. Track the active query context in `dataGridModel` and cancel it before executing a new filter query.

## Success Criteria

- Typing in a `text_input` bound to a topic causes the related `data_grid` to update with parameterized query results.
- Rapid typing triggers multiple updates but only the latest query's results are written to the UI (stale queries cancelled).
- Unit tests verify publication of input updates and reactive grid loading.

## Rollback Plan

- Revert changes in `cmd/golemui/main.go` and `pkg/ui/compositor.go` to remove `ui.LocalEventBus` references.
- Revert widget composition to use static non-parameterized queries and ignore event publications.
