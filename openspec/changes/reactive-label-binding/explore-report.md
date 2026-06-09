# SDD Explore Report — reactive-label-binding

> Mapping the current state of the `label` component, EventBus subscription plumbing, and cleanup/lifecycle patterns that the new reactive label binding (per spec `docs/specify/archived/017-reactive-label-binding.md`) will plug into. Scope: `pkg/ui/compositor.go`, `pkg/eventbus/eventbus.go`, `pkg/ui/screen_state.go`, all related test files, and the supporting utilities (data access, dot-notation / template parsing).

---

## 1. Executive Summary

The `label` component is **currently static**. It renders `node.Label` as a plain `widget.NewLabel(...)` and never subscribes to any channel. The data_grid case, on the other hand, already demonstrates the full reactive pattern that the new label needs:

- `EventBus.Subscribe(channel, handler) → string` (subID)
- Subscriber runs in its own goroutine (`go h(event)` at `eventbus.go:75`)
- UI-thread dispatch via `fyne.Do(func() { ... })` (mandated by the `compositor.go` package comment at lines 1-9)
- Cleanup func that calls `LocalEventBus.Unsubscribe(channel, subID)`, returned alongside the widget (already plumbed through `Compose` / `composeWithState`)

What the reactive label adds that does **not** exist anywhere in the codebase today:

- A dot-notation path resolver (`resolvePath(data any, path string) any`) — **not implemented** anywhere.
- A `{token}` template parser — **not implemented** anywhere.
- Any reactive binding for `label`, `button`, or any other non-data_grid widget.

The spec (`docs/specify/archived/017-reactive-label-binding.md`) is clear about scope:

- The binding is triggered by `node.DataSource` being non-empty (convention: `"event:..."` or `"publish_selection"`).
- `node.Label` is treated as a template with `{...}` placeholders.
- The handler must call `label.SetText(...)` **inside** `fyne.Do(func() { ... })`.
- Cleanup func must call `LocalEventBus.Unsubscribe` on screen teardown.

All the plumbing for the above already exists except for `resolvePath` and the template parser.

---

## 2. The `label` component today

### 2.1 File and exact lines

`pkg/ui/compositor.go` lines **182-183**:

```go
case "label":
    return widget.NewLabel(node.Label), func() {}, nil
```

This is the **complete** current label implementation. It:

- Creates a Fyne `*widget.Label` with the static text from `node.Label`.
- Returns a **no-op cleanup func** (in line with the "always non-nil cleanup" contract from `screen-lifecycle-cleanup`).
- Returns `nil` error.

The case lives inside the `composeWithState` switch at `compositor.go:140`. The container case precedes it (lines 142-180), and the `text_input` case follows (lines 185-196).

### 2.2 What the label does NOT do today

- No subscription to `LocalEventBus`.
- No re-render after composition.
- No template processing of `node.Label`.
- No dynamic state.

### 2.3 Test coverage today

- `pkg/ui/compositor_test.go:60-95` — `TestCompose_SimpleHierarchy` creates a `label` with `Label: "Username:"` and asserts `lbl.Text == "Username:"`. Static-only assertion.
- `pkg/ui/compositor_test.go:106-130` — `TestCompose_Fallback` tests the `default` case (unrecognized `ComponentRef`), which falls back to a `widget.Label` with a `[Fallback: ...]` text.
- `pkg/ui/compositor_test.go:1563-1589` — `TestCompose_NoOpCleanup_NoDataGrid` composes a label-only screen, asserts the cleanup func is non-nil, and calls it. It does **not** assert anything about the label being reactive.

No existing test exercises a `label` against `LocalEventBus` or a `DataSource` on a label.

---

## 3. `NodeMeta` struct — full definition

`pkg/ui/compositor.go` lines **47-66**:

```go
type NodeMeta struct {
    Area             string     `json:"area"`
    ComponentRef     string     `json:"component_ref"`
    Label            string     `json:"label,omitempty"`
    Placeholder      string     `json:"placeholder,omitempty"`
    DefaultValue     string     `json:"default_value,omitempty"`
    Min              float64    `json:"min,omitempty"`
    Max              float64    `json:"max,omitempty"`
    Validation       string     `json:"validation,omitempty"`
    DataSource       string     `json:"data_source,omitempty"`
    SubmitAction     string     `json:"submit_action,omitempty"`
    BindTo           string     `json:"bind_to,omitempty"`
    FilterMode       string     `json:"filter_mode,omitempty"`
    MasterDataSource string     `json:"master_data_source,omitempty"`
    FilterKeys       []string   `json:"filter_keys,omitempty"`
    Layout           LayoutMeta `json:"layout,omitempty"`
    Children         []NodeMeta `json:"children,omitempty"`
}
```

### 3.1 The `DataSource` field

**Type:** `string` (with `omitempty` JSON tag). Same field is reused for **three different purposes** depending on `ComponentRef`:

| Component | Meaning of `DataSource` | Where read |
| --- | --- | --- |
| `data_grid` | SQL source string for `DS.Fetch` / `DS.FetchAll`, or `"state:<key>"` for dynamic query resolution | `compositor.go:228, 232, 261` |
| `data_grid` (client mode) | `MasterDataSource` is used instead — different field | `compositor.go:224` |
| `label` (future reactive) | **Channel name for `EventBus.Subscribe`**, e.g. `"publish_selection"` or `"event:..."` | spec `017` §TAREA 1 |

**Layer-1 separation note:** `node.DataSource` on a `data_grid` is read by the **Go client (Capa 4) as a channel/SQL string**, while on a `data_source` plugin (Capa 1) it would be a SQL string. The reactive label reuses this field as a *channel name* (per spec 017), not as a SQL string. This is a deliberate convention — the spec at `archived/017-reactive-label-binding.md` line 38 says: "Preserva separada la propiedad JSON `node.DataSource` del backend global de datos representado por la interfaz Go `ui.DataSource`". The field is shared at the JSON level but interpreted by the compositor per-component.

### 3.2 Other relevant fields

- `Label string` — the text shown. Spec 017 wants to reuse this as a **template** (e.g. `"Monto: {monto} - Cliente: {cliente.nombre}"`).
- `Area string` — used for logging only.
- `ComponentRef string` — discriminator.

There are no new fields needed for reactive label binding — `DataSource` and `Label` are sufficient per the spec.

---

## 4. EventBus API — `pkg/eventbus/eventbus.go`

File length: 82 lines. Read in full.

### 4.1 Public surface (`eventbus.go:1-25`)

```go
// pkg/eventbus/eventbus.go:1-25
package eventbus

import (
    "fmt"
    "sync"
)

// SubmitChannelPrefix is the prefix for scoped submit channels.
// Actual channels are: "screen:submit:<vistaID>"
const SubmitChannelPrefix = "screen:submit"

type Event struct {
    Channel string
    Payload interface{}
}

type Handler func(Event)

type EventBus interface {
    Publish(channel string, payload interface{})
    Subscribe(channel string, h Handler) string // Returns unique sub ID
    Unsubscribe(channel string, subID string)
}
```

### 4.2 Subscribe signature and thread model

`eventbus.go:39-50`:

```go
func (b *InMemEventBus) Subscribe(channel string, h Handler) string {
    b.mu.Lock()
    defer b.mu.Unlock()

    if _, exists := b.subscribers[channel]; !exists {
        b.subscribers[channel] = make(map[string]Handler)
    }

    b.nextSubID++
    subID := fmt.Sprintf("%s:%d", channel, b.nextSubID)
    b.subscribers[channel][subID] = h
    return subID
}
```

- **Signature:** `Subscribe(channel string, h Handler) string`. Returns the unique `subID` to be used later for `Unsubscribe`.
- **Callback type:** `type Handler func(Event)`. The `Event` struct carries `Channel` and `Payload` (typed as `interface{}`).
- **Storage shape:** `map[channel]map[subID]Handler` — the closure is stored by value; any captured state (model, label, state, node) is pinned in memory until the entry is deleted.
- **Thread safety:** `b.mu.Lock()` held only during map mutation. The handler is invoked later, **outside** the lock.
- **SubID format:** `"<channel>:<monotonic uint64>"` — globally unique within a bus.

### 4.3 Unsubscribe signature

`eventbus.go:52-61`:

```go
func (b *InMemEventBus) Unsubscribe(channel string, subID string) {
    b.mu.Lock()
    defer b.mu.Unlock()

    if subs, exists := b.subscribers[channel]; exists {
        delete(subs, subID)
        if len(subs) == 0 {
            delete(b.subscribers, channel)
        }
    }
}
```

- **Signature:** `Unsubscribe(channel string, subID string)`. No return value.
- **Side effect:** if the inner map becomes empty, the outer channel key is also removed, releasing the inner map. Useful for diagnostics.
- **Idempotent:** deleting a non-existent key is a no-op.

### 4.4 Publish signature and per-handler goroutine

`eventbus.go:63-82`:

```go
func (b *InMemEventBus) Publish(channel string, payload interface{}) {
    b.mu.RLock()
    defer b.mu.RUnlock()

    subs, exists := b.subscribers[channel]
    if !exists {
        return
    }

    event := Event{
        Channel: channel,
        Payload: payload,
    }

    for _, handler := range subs {
        h := handler
        go h(event)
    }
}
```

- **Signature:** `Publish(channel string, payload interface{})`. No return.
- **Goroutine model:** **every** handler runs in a fresh `go h(event)`. This is the critical detail for the new label handler: the `label.SetText(...)` call inside the handler executes on a background goroutine, **not** on the Fyne UI thread.
- **Consequence:** any Fyne widget mutation from a Subscribe handler **must** be wrapped in `fyne.Do(func() { ... })`. This is the contract documented in the `compositor.go` package comment (lines 1-9) and the whole point of the recent `fyne-thread-safety-v2` change.

### 4.5 Test helper: `SubscriberCount`

`pkg/eventbus/test_helpers.go` (entire file, 10 lines):

```go
package eventbus

// SubscriberCount returns the number of active subscribers for a channel.
// Exported for testing and diagnostics.
func (b *InMemEventBus) SubscriberCount(channel string) int {
    b.mu.RLock()
    defer b.mu.RUnlock()
    return len(b.subscribers[channel])
}
```

- Exported method on `*InMemEventBus`, accessible from outside the package (e.g. `eb.(*eventbus.InMemEventBus).SubscriberCount("publish_selection")` in compositor tests).
- Concurrency-safe.

### 4.6 Test patterns for EventBus — `pkg/eventbus/eventbus_test.go` (156 lines)

- `TestEventBus_HappyPath` (line 22) — Subscribe, Publish, wait for handler with `sync.WaitGroup` + `time.After(1 * time.Second)` timeout.
- `TestEventBus_Unsubscribe` (line 54) — Subscribe, Unsubscribe, Publish, sleep 10ms, assert handler NOT called.
- `TestEventBus_SlowSubscriber` (line 73) — Two subscribers, one slow (100ms sleep), one fast. Asserts the fast one finishes before the slow one (proves goroutine-per-handler model). Uses `chan struct{}` for completion signals.
- `TestEventBus_Concurrency` (line 122) — 100 goroutines, mix of Subscribe / Publish / Unsubscribe, verifies no race.
- `TestSubmitChannelPrefix_Constant` (line 174) — Verifies the exported constant.
- Test infrastructure used: `sync.WaitGroup`, `time.Sleep`, `time.After`, `chan struct{}`, `atomic.AddInt32` (used in `compositor_test.go`).

---

## 5. Existing patterns the new label will follow

### 5.1 The data_grid Subscribe pattern — the canonical template

`pkg/ui/compositor.go:247-269` (simplified):

```go
// Subscribe to scoped SubmitChannel for reactivity
if LocalEventBus != nil {
    log.Printf("[UI/DataGrid] Subscribing data_grid at area %q to channel %q", node.Area, state.SubmitChannel())
    subID := LocalEventBus.Subscribe(state.SubmitChannel(), func(ev eventbus.Event) {
        snap, ok := ev.Payload.(map[string]any)
        if !ok {
            log.Printf("[UI/DataGrid] Warning: payload on channel %q is not map[string]any", state.SubmitChannel())
            return
        }
        // ... process snap ...
    })
    model.mu.Lock()
    model.unsubscribe = func() {
        LocalEventBus.Unsubscribe(state.SubmitChannel(), subID)
    }
    model.mu.Unlock()
}
```

**Key details to mirror in the new label handler:**

1. **Nil-guard the bus** with `if LocalEventBus != nil`.
2. **Subscribe once at compose time**, capture the returned `subID`.
3. **Type-assert the payload** to `map[string]any` (this is the universal payload shape for both `screen:submit:<vistaID>` and `publish_selection`).
4. **Log on subscribe and on every event** for traceability.
5. **Store an `unsubscribe` closure** that closes over the `subID` — invoked from cleanup.

### 5.2 The data_grid cleanup pattern — the canonical template

`pkg/ui/compositor.go:286-307`:

```go
var once sync.Once
cleanup := func() {
    once.Do(func() {
        model.mu.Lock()
        cancelFn := model.cancel
        unsubFn := model.unsubscribe
        model.cancel = nil
        model.unsubscribe = nil
        model.mu.Unlock()

        if cancelFn != nil {
            cancelFn()
        }
        if unsubFn != nil {
            unsubFn()
        }
        model.wg.Wait()
    })
}

return table, cleanup, nil
```

**Key details to mirror:**

1. **Use `sync.Once`** for idempotency. Re-entrant cleanup is safe (cleanup is called once and any further calls no-op).
2. **Snapshot the cleanup funcs under lock, then null them**, then call them **outside** the lock (avoids deadlock — `LocalEventBus.Unsubscribe` may try to take the bus's internal lock).
3. **Always return a non-nil cleanup** from `composeWithState`. The container case aggregates child cleanups; leaf cases return a no-op.

### 5.3 The fyne.Do pattern (mandatory for widget mutations)

The `compositor.go` package comment at **lines 1-9** is the authoritative contract:

```go
// Package ui implements the GolemUI rendering engine.
//
// Thread-safety contract for EventBus subscribers:
// Any LocalEventBus.Subscribe handler that mutates a Fyne widget (e.g. table.Refresh,
// label.SetText, button.Enable) must wrap the mutation in fyne.Do(func() { ... }).
// The EventBus dispatches each handler in a fresh goroutine (go h(event)).
// Fyne requires widget mutations on the UI thread. fyne.Do bridges the two.
// This applies to all current and future subscriber handlers, including the data_grid
// reactive filtering and upcoming reactive label (017) and button state (018) bindings.
```

This is the precedent the v2 spec (`openspec/changes/fyne-thread-safety-v2/proposal.md` §3.5) explicitly created for specs 017 and 018. **Any new reactive label handler must wrap its `label.SetText(...)` in `fyne.Do(func() { ... })`** to satisfy REQ-EB-01.

Existing `fyne.Do` sites in `compositor.go` for reference (modeled on `table.SetColumnWidth` + `table.Refresh()` pairs):

- `compositor.go:376` — `loadMasterBuffer` (model write lock unlocked first, then `fyne.Do`).
- `compositor.go:406` — `filterMasterRows` empty-snap path.
- `compositor.go:444` — `filterMasterRows` filtered path.
- `compositor.go:524` — `fetchGridDataAsync`.

The label handler will follow the same shape: model/state read under any necessary lock, then `fyne.Do(func() { label.SetText(resolved) })`.

### 5.4 The button's Publish pattern — what the label will listen to

`compositor.go:208-210` shows that the `submit` button already publishes `state.Snapshot()` to `state.SubmitChannel()` (the `screen:submit:<vistaID>` channel). Combined with `data_grid.OnSelected` publishing to `"publish_selection"` (`compositor.go:280`), the two channels the reactive label will most commonly listen to are:

- `"publish_selection"` (already exists, header→value map).
- `state.SubmitChannel()` = `"screen:submit:<vistaID>"` (already exists, `state.Snapshot()`).

The spec at 017 says the channel is configured via `node.DataSource`. This is the same convention the data_grid uses for the SQL source string; the renderer will need to disambiguate (e.g. by prefix `"event:"` for an explicit event channel, or by using `DataSource == "publish_selection"` as a literal).

### 5.5 Container cleanup aggregation

`compositor.go:178-180`:

```go
cleanup := func() {
    for _, c := range cleanups {
        c()
    }
}
```

When a label is nested inside a container, the label's cleanup will be in `cleanups` automatically. No new aggregation logic is needed.

---

## 6. Data flow — how reactive components subscribe and clean up

End-to-end picture for the new label:

```
[Compose time]
composeWithState case "label":
  if node.DataSource != "" && LocalEventBus != nil {
      subID := LocalEventBus.Subscribe(channel, handler)
      cleanup := sync.Once-wrapped func() {
          LocalEventBus.Unsubscribe(channel, subID)
      }
  } else {
      cleanup := func() {}    // no-op, like other leaf nodes
  }
  return widget.NewLabel(node.Label), cleanup, nil

container case (parent):
  aggregates label's cleanup into children's cleanups
  returns combined cleanup that calls each in order

main.go (ui.Navigate goroutine):
  prevCleanup = cleanup                  // captured before fyne.Do
  fyne.Do(mainContainer.Objects = ...)   // UI swap

[Runtime]
data_grid.OnSelected(row):
  LocalEventBus.Publish("publish_selection", headerMap)
  → for each subscriber h: go h(event)
    → label handler:
        payload, ok := ev.Payload.(map[string]any)
        resolved := renderTemplate(node.Label, payload, resolvePath)
        fyne.Do(func() { label.SetText(resolved) })

button.OnTapped:
  LocalEventBus.Publish(state.SubmitChannel(), state.Snapshot())
  → for each subscriber h: go h(event)
    → label handler: same as above

[Teardown]
ui.Navigate(newVistaID):
  prevCleanup()  // calls LocalEventBus.Unsubscribe for the old screen's label
```

This is identical to the data_grid lifecycle in `compositor.go:247-307`, with two simplifications:

- No `context.CancelFunc` (the label has no in-flight goroutines — the handler is short-lived, returns immediately after `fyne.Do` queues the UI update).
- No `sync.WaitGroup` (same reason — no goroutines to wait for).

So the label's cleanup can be a pure `Unsubscribe` call, no `cancel` + `wg.Wait` like data_grid's `compositor.go:294-303`.

---

## 7. Test infrastructure for compositor and eventbus

### 7.1 Fyne test app bootstrap — `compositor_test.go:20-23`

```go
func TestMain(m *testing.M) {
    test.NewApp()
    m.Run()
}
```

Every test in `pkg/ui` (test package `ui_test`) relies on this once-per-test-process Fyne app initialization. `test.NewApp()` (from `fyne.io/fyne/v2/test`) creates a headless Fyne app for tests.

### 7.2 `trackingMockDataSource` — recordable mock

`compositor_test.go:26-57`:

```go
type trackingMockDataSource struct {
    *dataaccess.MockDataSource
    mu         sync.Mutex
    fetchCalls []struct {
        source string
        args   []any
    }
    fetchAllCalls []string
}
```

Used in tests that need to assert what the compositor called on the data source. The new label handler does **not** call `DataSource` (it subscribes to an event bus channel), so this mock is irrelevant for reactive label tests. But the `dataaccess.MockDataSource` and `dataaccess.MockCWR` doubles in `pkg/ui/datasource.go:64-104` may be useful if the label test happens to share a screen with a data_grid.

### 7.3 `dataaccess.MockDataSource` and `MockCWR` — `pkg/ui/datasource.go:64-104`

```go
type MockDataSource struct {
    FetchCalled bool
    FetchSource string
    FetchArgs   []any
    FetchResult DataSet
    FetchError  error
    FetchAllCalled bool
    FetchAllSource string
    FetchAllResult DataSet
    FetchAllError  error
}

type MockCWR struct {
    ResolveFunc func(origen, header string) string
}
```

Plain recordable mocks; no goroutine control. Their zero value is safe to use.

### 7.4 EventBus tests — `pkg/eventbus/eventbus_test.go`

See §4.6. The new label tests will not need to add new EventBus tests — the bus primitive is already covered.

### 7.5 Compositor test patterns the label tests can copy

- `TestCompose_SimpleHierarchy` (`compositor_test.go:60`) — assert the type-cast result and the field values. Will be the model for `TestCompose_Label_Static_NoDataSource` (assertion-only).
- `TestCompose_NoOpCleanup_NoDataGrid` (`compositor_test.go:1571-1589`) — confirms cleanup is non-nil and safe to call. Will be the model for `TestCompose_Label_Static_CleanupIsNoop`.
- `TestCompose_DataGrid_RowSelection_PublishesToSelectionChannel` (`compositor_test.go:1167-1264`) — **the closest template for the reactive label tests.** Uses `eventbus.NewEventBus()`, sets `ui.LocalEventBus = eb`, composes a node, simulates the event source (`table.OnSelected(...)`), and asserts the handler received the expected payload. The new label tests will follow the same pattern, except:
  - The composition target is a `label` (not a `data_grid`).
  - The event source is `LocalEventBus.Publish(node.DataSource, payload)` (not `table.OnSelected`).
  - The assertion is on `label.Text` (not on a mock call list).

### 7.6 Test helpers

- `trackingMockDataSource` (`compositor_test.go:26-57`) — `Fetch` call recorder. Not needed for label tests.
- `eb.(*eventbus.InMemEventBus).SubscriberCount(channel)` — used in `TestCompose_CleanupRemovesSubscribers` (`compositor_test.go:1426-1473`) to assert subscribers are removed on cleanup. Will be the model for the label's "cleanup unsubscribes" test.

### 7.7 What is NOT in the test infrastructure

- No `label.SetText` assertion helpers (will be inline `if lbl.Text != "..." { t.Errorf(...) }`).
- No fyne-thread assertion helpers — the existing `fyne.Do` regression tests rely on **behavioral** outcomes (the table actually shows the new data) rather than structural checks. The new label tests can use the same approach: after publishing the event and sleeping for delivery, assert `lbl.Text` matches the expected resolved string.

---

## 8. Gaps — what needs to be created vs. what already exists

| # | Need | Status | Existing reference |
|---|------|--------|--------------------|
| 1 | Modify the `case "label":` branch to subscribe when `node.DataSource != ""` | **To create** | `compositor.go:182-183` (current 2-line implementation) |
| 2 | Return a real cleanup func from the label case (calling `Unsubscribe`) | **To create** | `compositor.go:286-307` (data_grid cleanup pattern) |
| 3 | `resolvePath(data any, path string) any` — recursive dot-notation resolver | **To create** | None — no existing path resolver anywhere in the codebase |
| 4 | `{token}` template parser — extracts tokens from `node.Label`, calls `resolvePath`, returns the resolved string | **To create** | None — no existing template parser anywhere in the codebase |
| 5 | `fyne.Do(func() { label.SetText(...) })` wrap in the Subscribe handler | **To create** (but trivial) | `compositor.go:376, 406, 444, 524` (four existing data_grid wraps) |
| 6 | `subID` capture and `sync.Once`-wrapped cleanup func with the Unsubscribe call | **To create** (mirroring data_grid) | `compositor.go:264-268, 287-307` |
| 7 | `NodeMeta.DataSource` interpretation convention for labels (e.g. `"publish_selection"`, `"event:..."`, `state.SubmitChannel()`) | **To create** (a parsing rule, not new code) | `compositor.go:212-216` for button, `compositor.go:259-263` for data_grid `"state:"` prefix |
| 8 | Compositor tests for reactive label | **To create** | `compositor_test.go:1167-1264` (`TestCompose_DataGrid_RowSelection_PublishesToSelectionChannel`) is the template |
| 9 | Unit tests for `resolvePath` | **To create** | None — new file or new section in `compositor_test_internal_test.go` |
| 10 | Unit tests for the template parser | **To create** | None — new file or new section in `compositor_test_internal_test.go` |
| 11 | Reaffirm the `compositor.go` package comment already covers the label's `fyne.Do` contract | **Already exists** | `compositor.go:1-9` (mentions "reactive label (017)" by name) |
| 12 | EventBus API support for subscribe/unsubscribe/publish | **Already exists** | `eventbus.go:39-82` |
| 13 | Cleanup func plumbing through `Compose` / `composeWithState` / container aggregation | **Already exists** | `compositor.go:62-82` (Compose return tuple), `compositor.go:178-180` (container aggregation) |
| 14 | The `subID` lifecycle (Subscribe returns string, Unsubscribe takes string) | **Already exists** | `eventbus.go:39-61` |
| 15 | `SubscriberCount` test helper for cleanup assertions | **Already exists** | `pkg/eventbus/test_helpers.go` |
| 16 | Fyne test app bootstrap in `TestMain` | **Already exists** | `compositor_test.go:20-23` |

**Net:** the only **new** code is the label case's Subscribe/Unsubscribe plumbing (mirroring data_grid) plus the two pure functions `resolvePath` and a template parser. Everything else — EventBus, cleanup plumbing, `fyne.Do` precedent, test infrastructure — is in place.

---

## 9. Files Retrieved (line ranges)

1. `pkg/ui/compositor.go` (lines 1-581, full file) — current label case at 182-183, data_grid Subscribe/Unsubscribe at 247-307, container cleanup aggregation at 178-180, package comment at 1-9.
2. `pkg/eventbus/eventbus.go` (lines 1-82, full file) — Subscribe/Unsubscribe/Publish signatures, goroutine-per-handler model, subID format.
3. `pkg/eventbus/eventbus_test.go` (lines 1-189, full file) — happy path, unsubscribe, slow subscriber, concurrency test patterns.
4. `pkg/eventbus/test_helpers.go` (lines 1-10, full file) — `SubscriberCount` helper used in compositor cleanup tests.
5. `pkg/ui/screen_state.go` (lines 1-58, full file) — `ScreenState` with `Set` / `Get` / `Snapshot` / `SubmitChannel` (returns `"screen:submit:<vistaID>"`). Thread-safe via `sync.RWMutex`.
6. `pkg/ui/screen_state_test.go` (lines 1-180, full file) — table of `Set` / `Get` / `Snapshot` / concurrent tests, useful as a template for any new test file.
7. `pkg/ui/compositor_test.go` (lines 1-2165, partial) — `TestCompose_SimpleHierarchy` (60-105), `TestCompose_Fallback` (106-135), `TestCompose_DataGrid_RowSelection_PublishesToSelectionChannel` (1167-1264), `TestCompose_NoOpCleanup_NoDataGrid` (1571-1589), `TestCompose_CleanupRemovesSubscribers` (1426-1473), `TestCompose_IdempotentCleanup` (1492-1569), `TestMain` (20-23), `trackingMockDataSource` (26-57).
8. `pkg/ui/compositor_internal_test.go` (lines 1-6, full file) — trivial test of `dataGridModel` existence.
9. `pkg/ui/compositor_test_internal_test.go` (lines 1-95, full file) — internal-package tests for `extractOrderedArgs` and `containsIgnoreCase`. Good template for any new internal-only tests like `resolvePath`.
10. `pkg/ui/datasource.go` (lines 1-99, full file) — `DataSet`, `DataSource`, `ColumnWidthResolver`, `MockDataSource`, `MockCWR`.
11. `pkg/dataaccess/interfaces.go` (lines 1-29, full file) — re-exports `ui.DataSet`, `ui.DataSource`, `ui.ColumnWidthResolver`, `ui.MockDataSource`, `ui.MockCWR` to avoid import cycles.
12. `pkg/dataaccess/extract_args.go` (lines 1-23, full file) — production equivalent of `compositor.extractOrderedArgs`. Shows the project's style for snapshot→arg mapping (the `resolvePath` function should be a similar small, well-tested helper).
13. `pkg/ui/sidebar_widget.go` (lines 1-156, full file) — `NavTree.SelectByVistaID` uses `fyne.DoAndWait` for re-entrancy-safe UI mutations. Reference for the `fyne.Do` pattern.
14. `pkg/ui/layout.go` (lines 1-69) — `parseMetric` function shows the project's style for parsing CSS-like spec strings (e.g. `"150px"`, `"1fr"`). Useful precedent for a `{token}` template parser.
15. `docs/specify/archived/017-reactive-label-binding.md` (lines 1-70, full file) — the spec being implemented. Quotes the exact acceptance criteria for `resolvePath` and the template parser (lines 56-69).
16. `openspec/changes/fyne-thread-safety-v2/proposal.md` (lines 1-220, partial) — establishes the `fyne.Do` contract and the package comment convention (REQ-EB-01) that the new label will follow.
17. `openspec/changes/fyne-thread-safety-v2/explore.md` (lines 1-365, partial) — confirms the label does **not** subscribe today (line 180, 227) and that the `fyne.Do` pattern is the established precedent for 017/018.
18. `openspec/changes/screen-lifecycle-cleanup/design.md` (lines 1-300, full file) — documents the cleanup func plumbing and the `sync.Once` idempotency pattern the label will mirror.

---

## 10. Open Questions / Decisions to make in proposal phase

1. **Channel name format.** Spec 017 says: "si inicia con el formato `"event:"` o define un nombre de canal como `"publish_selection"`". Need a deterministic parsing rule. Options:
   - **A:** `DataSource == "publish_selection"` → subscribe to literal channel; `strings.HasPrefix(DataSource, "event:")` → subscribe to the suffix.
   - **B:** All values are literal channel names; the `"event:"` prefix is cosmetic and stripped.
   - **C:** `"state:<key>"` means subscribe to `state.SubmitChannel()` and read `<key>` from the snapshot (different from data_grid's `"state:"` semantic — could be confusing).

   **Recommendation:** Option A — the prefix convention mirrors data_grid's `"state:"` prefix (`compositor.go:259-263`) and gives a clean way to express "this is a channel reference, not a SQL source string".

2. **Error / empty payload handling.** If `ev.Payload` is not `map[string]any` (or is `nil`), what should the label do?
   - **A:** Set the text to a literal `""` (clearing the label).
   - **B:** Set the text to the original `node.Label` template (effectively a no-op render).
   - **C:** Skip the update entirely (log warning).

   **Recommendation:** Option C — mirrors the data_grid handler's `if !ok { log.Printf(...); return }` pattern (`compositor.go:250-254`).

3. **Token not found in payload.** If `{cliente.nombre}` references a missing path, the spec does not specify the fallback. Options:
   - **A:** Render the literal token (`{cliente.nombre}` stays as text).
   - **B:** Render an empty string.
   - **C:** Render `"<missing>"` for debug visibility.

   **Recommendation:** Option A — preserves user intent and is the conventional behavior of template engines (e.g. Mustache).

4. **Scope of `resolvePath`.** Spec 017 limits recursion to `map[string]any` and scalar convertibles. Confirm: arrays (`[]any`) should also be supported? The spec's example (`"transaccion.detalles.valor"`) only uses maps. Recommendation: start with **maps-only** for the first slice; defer array index access (`{items.0.name}`) to a follow-up.

5. **Bidirectional bindings.** Out of scope per spec 017 line 33: "Mantén intacta la instanciación estática del widget `Label` cuando la propiedad `DataSource` de `NodeMeta` se encuentre vacía." Confirmed — empty `DataSource` → static `widget.NewLabel`, no Subscribe, no-op cleanup (already the current behavior).

6. **Cleanup when `LocalEventBus == nil`.** The data_grid case already handles this with `if LocalEventBus != nil { ... }` (`compositor.go:248`). The label should mirror this. When the bus is nil, the label's cleanup is the existing no-op (no subscription was made).

---

## 11. Supervisor coordination

No supervisor action required. The findings above are self-contained: the spec at `archived/017-reactive-label-binding.md` is precise about the `resolvePath` function signature and the template syntax; the `data_grid` reactive subscription in `compositor.go:247-307` is a complete template for the new label's plumbing; the `fyne-thread-safety-v2` change has already established the `fyne.Do` pattern and the package comment that the new label must follow. No blocking decisions, no external dependencies, no missing context.
