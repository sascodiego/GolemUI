# Technical Specification: Reactive Label Binding

**Change ID:** `reactive-label-binding`
**Spec source:** `docs/specify/archived/017-reactive-label-binding.md`
**Proposal:** `openspec/changes/reactive-label-binding/proposal.md`
**Status:** Draft

---

## 1. Data Flow — 4-Layer Decoupling

### Layer 1: Data Origin

The reactive label does **not** introduce a new data origin. It consumes payloads already published by existing components:

| Publisher | Channel | Payload shape | Trigger |
|---|---|---|---|
| `data_grid` `OnSelected` | `"publish_selection"` | `map[string]any` (column header → cell value) | User selects a row in a data_grid |
| `button` `OnTapped` | `"screen:submit:<vistaID>"` | `map[string]any` (`state.Snapshot()`) | User taps a submit button |

The reactive label **subscribes** to one of these channels. It never publishes. The payload is always `map[string]any` — the universal shape for all EventBus payloads in GolemUI.

### Layer 2: Core Mapping (golemui_core)

No new catalog components, styles, or mapping rules are introduced. The `label` component already exists in the catalog. The reactive behavior is configured entirely through the existing `NodeMeta.DataSource` field:

- When `DataSource` is empty → static label (current behavior, no mapping needed).
- When `DataSource` is non-empty → the compositor interprets it as an EventBus channel name.

No schema changes to `golemui_core`. No new rows in `golemui.mapeo_interfaz`.

### Layer 3: Overrides (golemui.mapeo_interfaz)

No overrides needed. The `DataSource` field is interpreted per-component by the compositor (Capa 4). The same `DataSource` field means different things for `data_grid` (SQL source string) vs. `label` (EventBus channel name). This per-component interpretation is already the established pattern — see `compositor.go:259-263` where `data_grid` checks for a `"state:"` prefix on the same field.

### Layer 4: Fyne Renderer (Go client)

All changes are in this layer. The compositor's `case "label"` branch in `composeWithState` gains:

1. **Channel name parsing** from `node.DataSource`.
2. **EventBus subscription** at compose time.
3. **Template resolution** using `renderTemplate` + `resolvePath`.
4. **Thread-safe widget update** via `fyne.Do(func() { label.SetText(...) })`.
5. **Cleanup func** that unsubscribes from the EventBus.

### Data Flow Diagram

```
┌──────────────────────────────────────────────────────────────────┐
│ Layer 1: Data Origin                                             │
│  data_grid.OnSelected  ──Publish──▶  "publish_selection"        │
│  button.OnTapped       ──Publish──▶  "screen:submit:<vistaID>"  │
└──────────────────────────┬───────────────────────────────────────┘
                           │ EventBus (goroutine per handler)
                           ▼
┌──────────────────────────────────────────────────────────────────┐
│ Layer 4: Fyne Renderer — label handler                           │
│                                                                  │
│  1. Type-assert ev.Payload → map[string]any                      │
│  2. renderTemplate(node.Label, payload)                          │
│     └─ For each {token}: resolvePath(payload, token) → value     │
│  3. fyne.Do(func() { label.SetText(resolvedText) })             │
└──────────────────────────────────────────────────────────────────┘
```

---

## 2. Component Specification

### 2.1 `resolvePath(data any, path string) any`

**Package:** `pkg/ui` (unexported)
**Signature:** `func resolvePath(data any, path string) any`

#### Inputs

| Parameter | Type | Description |
|---|---|---|
| `data` | `any` | Root data object. Expected to be `map[string]any` for meaningful resolution. May be `nil`. |
| `path` | `string` | Dot-separated navigation path (e.g. `"transaccion.detalles.valor"`). May be empty. |

#### Output

| Condition | Return value |
|---|---|
| `data` is `nil` | `nil` |
| `path` is `""` (empty) | `nil` |
| Path fully resolves through nested `map[string]any` to a leaf value | The leaf value (any type: `string`, `float64`, `int`, `bool`, `nil`, etc.) |
| Path fully resolves through nested `map[string]any` to a `map[string]any` subtree | The subtree (`map[string]any`) |
| An intermediate key is missing in the current map | `nil` |
| An intermediate value is not `map[string]any` (e.g. a scalar) | `nil` |

#### Algorithm

```
func resolvePath(data any, path string) any:
    if data == nil || path == "":
        return nil

    current = data
    parts = strings.Split(path, ".")

    for each part in parts:
        if current == nil:
            return nil
        m, ok = current.(map[string]any)
        if !ok:
            return nil
        current, ok = m[part]
        if !ok:
            return nil

    return current
```

#### Constraints

- **Maps-only navigation.** Array index access (e.g. `{items.0.name}`) is not supported. If an intermediate value is `[]any`, `resolvePath` returns `nil` because the type assertion to `map[string]any` fails.
- **No type coercion.** The function returns the raw Go value from the map. Conversion to string is the responsibility of the caller (`renderTemplate`).
- **No panics.** All nil checks and type assertions use the comma-ok pattern. The function never panics on any input.

#### Examples

Given:
```go
data := map[string]any{
    "transaccion": map[string]any{
        "id": 101,
        "detalles": map[string]any{
            "moneda": "USD",
            "valor":  500.0,
        },
    },
}
```

| Expression | Result | Type |
|---|---|---|
| `resolvePath(data, "transaccion.detalles.valor")` | `500.0` | `float64` |
| `resolvePath(data, "transaccion.detalles.moneda")` | `"USD"` | `string` |
| `resolvePath(data, "transaccion.id")` | `101` | `int` |
| `resolvePath(data, "transaccion")` | `map[string]any{...}` | `map[string]any` |
| `resolvePath(data, "transaccion.inexistente")` | `nil` | `<nil>` |
| `resolvePath(data, "")` | `nil` | `<nil>` |
| `resolvePath(nil, "foo.bar")` | `nil` | `<nil>` |
| `resolvePath(data, "transaccion.id.nombre")` | `nil` | `<nil>` (intermediate `101` is not a map) |

---

### 2.2 `renderTemplate(tmpl string, data map[string]any) string`

**Package:** `pkg/ui` (unexported)
**Signature:** `func renderTemplate(tmpl string, data map[string]any) string`

#### Inputs

| Parameter | Type | Description |
|---|---|---|
| `tmpl` | `string` | Template string containing zero or more `{...}` tokens. May be empty. |
| `data` | `map[string]any` | Payload used to resolve tokens. May be `nil` or empty. |

#### Output

The template string with every resolved token replaced by its string representation. Unresolved tokens and literal text are preserved verbatim.

#### Token Format

- A token starts with `{` and ends with the **next** `}`.
- The text between `{` and `}` is treated as a dot-notation path (trimmed of leading/trailing whitespace).
- Tokens are processed left-to-right, one pass. No recursive resolution (a resolved value is not scanned for further tokens).

#### Resolution Rules

| Condition | Behavior |
|---|---|
| Token path resolves to a non-nil value | Replace with `fmt.Sprintf("%v", value)` |
| Token path resolves to `nil` | Preserve original token text (e.g. `{cliente.nombre}`) |
| Token path is empty (i.e. `{}`) | Preserve original token text (`{}`) |
| Token contains only whitespace (i.e. `{ }`) | Preserve original token text (`{ }`) |
| No closing `}` found | Treat `{` as literal text; no token extraction |
| `tmpl` contains no `{` | Return `tmpl` unchanged |
| `tmpl` is `""` | Return `""` |
| `data` is `nil` | All tokens resolve to `nil` → all tokens preserved verbatim |

#### Algorithm

```
func renderTemplate(tmpl string, data map[string]any) string:
    var result strings.Builder
    i = 0
    while i < len(tmpl):
        if tmpl[i] == '{':
            // find next '}'
            j = index of '}' after i
            if j == -1:
                // no closing brace — literal '{'
                result.WriteString(tmpl[i:])
                break
            path = tmpl[i+1 : j]  // text between braces
            trimmedPath = strings.TrimSpace(path)
            if trimmedPath == "":
                // empty path — preserve literal
                result.WriteString(tmpl[i : j+1])
            else:
                value = resolvePath(data, trimmedPath)
                if value != nil:
                    result.WriteString(fmt.Sprintf("%v", value))
                else:
                    result.WriteString(tmpl[i : j+1])  // preserve literal token
            i = j + 1
        else:
            result.WriteByte(tmpl[i])
            i++
    return result.String()
```

#### Constraints

- **No nesting.** `{a.{b.c}}` is not supported. The first `{` pairs with the next `}`.
- **No escaping.** There is no escape sequence for `{` or `}`. They always serve as delimiters when paired.
- **Standard library only.** No `text/template` or external template engine.
- **Single-pass.** Each `{` is processed once, left to right.

#### Examples

Given:
```go
data := map[string]any{
    "transaccion": map[string]any{
        "id": 101,
        "detalles": map[string]any{
            "moneda": "USD",
            "valor":  500.0,
        },
    },
}
```

| Template | Result |
|---|---|
| `"Monto: {transaccion.detalles.valor} {transaccion.detalles.moneda}"` | `"Monto: 500 USD"` |
| `"ID: {transaccion.id}"` | `"ID: 101"` |
| `"Sin tokens"` | `"Sin tokens"` |
| `"Missing: {no.existe}"` | `"Missing: {no.existe}"` |
| `""` | `""` |
| `"{}"` | `"{}"` |
| `"Value: { transaccion.id }"` | `"Value: 101"` (whitespace trimmed) |
| `"A {x} B {y} C"` where x→"1", y→"2" | `"A 1 B 2 C"` |
| `"Literal {"` | `"Literal {"` (unclosed brace) |
| `"{a} {b} {a}"` where a→"X", b→nil | `"X {b} X"` |

---

### 2.3 Modified Label Composition

**Package:** `pkg/ui`
**Location:** `compositor.go`, `composeWithState`, `case "label":` (currently lines 182-183)

#### Signature (unchanged return type)

```go
case "label":
    // ... new implementation
    return label, cleanup, nil
```

The return type remains `(*widget.Label, func(), error)` — same as all leaf cases.

#### Channel Name Parsing

A helper function `parseChannelName(dataSource string) string` extracts the EventBus channel name:

```go
func parseChannelName(dataSource string) string {
    if strings.HasPrefix(dataSource, "event:") {
        return strings.TrimPrefix(dataSource, "event:")
    }
    return dataSource
}
```

| `node.DataSource` value | `parseChannelName` result | Behavior |
|---|---|---|
| `""` | `""` | No subscription. Static label. |
| `"publish_selection"` | `"publish_selection"` | Subscribe to literal channel. |
| `"event:custom_channel"` | `"custom_channel"` | Prefix stripped. Subscribe to `"custom_channel"`. |
| `"screen:submit:vista_1"` | `"screen:submit:vista_1"` | Used as-is (literal). |

#### Decision Tree

```
case "label":
    label = widget.NewLabel(node.Label)

    if node.DataSource == "":
        // STATIC PATH — identical to current behavior
        return label, func() {}, nil

    if LocalEventBus == nil:
        // BUS-NIL GUARD — no subscription possible
        return label, func() {}, nil

    // REACTIVE PATH
    channel = parseChannelName(node.DataSource)
    subID   = LocalEventBus.Subscribe(channel, handler(label, node))
    cleanup = sync.Once-wrapped Unsubscribe(channel, subID)
    return label, cleanup, nil
```

#### Handler Contract

The handler closure captures: `label` (`*widget.Label`), `node.Label` (template string), `channel` (for logging).

```go
func(ev eventbus.Event) {
    payload, ok := ev.Payload.(map[string]any)
    if !ok {
        log.Printf("[UI/Label] Warning: payload on channel %q is not map[string]any, skipping update", channel)
        return
    }
    resolved := renderTemplate(node.Label, payload)
    fyne.Do(func() {
        label.SetText(resolved)
    })
}
```

**Critical:** `fyne.Do` is **mandatory**. The handler runs in a goroutine dispatched by `EventBus.Publish` (`go h(event)` at `eventbus.go:75`). Direct `label.SetText` would race with the Fyne UI thread. This is mandated by `compositor.go` package comment lines 1-9 and REQ-EB-01 from `fyne-thread-safety-v2`.

#### Cleanup Contract

```go
var once sync.Once
cleanup := func() {
    once.Do(func() {
        LocalEventBus.Unsubscribe(channel, subID)
    })
}
```

Properties:
- **Idempotent:** `sync.Once` guarantees the unsubscribe call happens exactly once.
- **Non-nil:** Always returns a cleanup func, even for reactive labels. The container case aggregates cleanups from all children; a nil cleanup would panic.
- **No `sync.WaitGroup`:** Unlike the data_grid cleanup (`compositor.go:294-303`), the label handler is fire-and-forget — it queues a `fyne.Do` and returns immediately. There are no long-running goroutines to wait for.
- **No `context.CancelFunc`:** Same reason — no goroutines to cancel.

#### Logging

For consistency with the data_grid pattern (`compositor.go:249`):

```go
log.Printf("[UI/Label] Subscribing label at area %q to channel %q", node.Area, channel)
```

Inside the handler, on payload type assertion failure:
```go
log.Printf("[UI/Label] Warning: payload on channel %q is not map[string]any, skipping update", channel)
```

---

## 3. Behavioral Specification — State Machine

### States

| State | Description |
|---|---|
| **Static** | Label was composed with empty `DataSource` or nil `LocalEventBus`. Widget shows `node.Label`. No subscription. Cleanup is no-op. |
| **Subscribed** | Label was composed with non-empty `DataSource` and non-nil `LocalEventBus`. Widget shows `node.Label` (initial template text). Handler is registered. Cleanup will unsubscribe. |
| **Updated** | At least one event has been processed. Widget shows the resolved template text. Handler remains active for further events. |
| **CleanedUp** | Cleanup has been called. Handler is unsubscribed. Widget retains its last text. No further events will be processed. |

### Transitions

```
Compose(DataSource="", bus=any)
  → Static

Compose(DataSource!="", bus=nil)
  → Static

Compose(DataSource!="", bus!=nil)
  → Subscribed
  → label.Text = node.Label (template text)

Subscribed + Event(map[string]any payload)
  → renderTemplate(node.Label, payload)
  → fyne.Do(label.SetText(resolved))
  → Updated

Updated + Event(map[string]any payload)
  → renderTemplate(node.Label, payload)
  → fyne.Do(label.SetText(resolved))
  → Updated (remains)

Subscribed/Updated + cleanup()
  → LocalEventBus.Unsubscribe(channel, subID)
  → CleanedUp

CleanedUp + cleanup() [second call]
  → sync.Once no-op
  → CleanedUp (unchanged)

Subscribed/Updated + Event(non-map payload)
  → log warning, skip
  → state unchanged (label retains previous text)
```

---

## 4. Error Handling

### 4.1 Payload Type Assertion Failure

**Condition:** `ev.Payload` is not `map[string]any` (including `nil`, `string`, `int`, `[]any`, etc.).

**Behavior:** Log a warning with `[UI/Label]` prefix, channel name. Skip the update entirely. The label retains its previous text.

**Rationale:** Mirrors the data_grid handler at `compositor.go:250-254`. Skipping is safer than clearing or resetting the label — the user sees the last valid state.

### 4.2 Token Path Not Found in Payload

**Condition:** `resolvePath(payload, "cliente.nombre")` returns `nil` because the path does not exist in the payload map.

**Behavior:** `renderTemplate` preserves the original token text (e.g. `{cliente.nombre}`) in the output string.

**Rationale:** Conventional template engine behavior (Mustache, Handlebars). Shows the user the unresolved reference rather than silently hiding it.

### 4.3 Empty Path Inside Token

**Condition:** Token is `{}` or `{ }` (empty or whitespace-only path after trimming).

**Behavior:** `renderTemplate` preserves the original token text as-is. No resolution attempted.

**Rationale:** Empty paths are meaningless; preserving the literal avoids silent data loss.

### 4.4 Unclosed Brace

**Condition:** Template contains `{` without a matching `}`.

**Behavior:** The `{` and all following text are treated as literal. No token extraction.

**Rationale:** Prevents scanning past the end of the string. Matches the single-pass left-to-right algorithm.

### 4.5 Nil EventBus

**Condition:** `LocalEventBus` is `nil` at compose time.

**Behavior:** Label falls back to static mode. No subscription. No-op cleanup. No error returned.

**Rationale:** Matches the data_grid guard at `compositor.go:248`. The bus may be nil in test environments or during initialization.

### 4.6 Nil Payload Map

**Condition:** `data` parameter to `renderTemplate` is `nil`.

**Behavior:** All `resolvePath` calls return `nil`. All tokens are preserved verbatim. The template text is returned as-is (minus any tokens that happen to resolve — which is none).

### 4.7 Channel With No Publishers

**Condition:** Label subscribes to a channel where no component ever publishes.

**Behavior:** Handler is never invoked. Label retains the initial template text indefinitely. No error, no log, no timeout. This is benign — the label simply stays static.

---

## 5. Thread Safety Contract

### 5.1 Goroutine Model

```
┌───────────────────────┐       ┌──────────────────────────────┐
│ EventBus.Publish()    │       │  goroutine (handler)          │
│ eventbus.go:63-82     │       │                                │
│                       │       │  1. Type-assert payload        │
│  go h(event)  ──────▶ │ ────▶ │  2. renderTemplate(...)        │
│  (fresh goroutine)    │       │  3. fyne.Do(func() {           │
│                       │       │       label.SetText(resolved)  │
│                       │       │     })                         │
└───────────────────────┘       └──────────┬─────────────────────┘
                                           │
                                           │ fyne.Do enqueues
                                           ▼
                                ┌──────────────────────────────┐
                                │  Fyne UI Thread               │
                                │  label.SetText(resolved)      │
                                │  Widget state updated safely  │
                                └──────────────────────────────┘
```

### 5.2 Shared State Analysis

| Variable | Written by | Read by | Protection |
|---|---|---|---|
| `subID` | Compose goroutine (once) | Cleanup func | Immutable after capture — no protection needed |
| `channel` | Compose goroutine (once) | Handler closure, Cleanup func | Immutable after capture — no protection needed |
| `node.Label` | N/A (read-only struct) | Handler closure | Immutable — no protection needed |
| `label` (`*widget.Label`) | Handler (via `fyne.Do`) | Compositor (initial creation), Tests | `fyne.Do` serializes all mutations on UI thread |
| `LocalEventBus` | Package-level var | Compose, Handler, Cleanup | Assumed set before compose and not changed during screen lifecycle |

**No mutex is needed for the label handler.** Unlike the data_grid, the label has no mutable model state — the handler is a pure function of the event payload and the immutable template string.

### 5.3 Forbidden Patterns

- **Direct `label.SetText(...)` from a Subscribe handler** — must use `fyne.Do`.
- **Reading `label.Text` from a Subscribe handler** — technically safe for reads but unnecessary; the resolved text is computed from the payload, not from the widget state.
- **Capturing the `*widget.Label` pointer and sharing it across screens** — the label belongs to a single screen; cleanup unsubscribes it.

---

## 6. Lifecycle Contract

### 6.1 Timeline

```
T0: Compose
    ├── widget.NewLabel(node.Label)
    ├── parseChannelName(node.DataSource)
    ├── LocalEventBus.Subscribe(channel, handler) → subID
    └── Return (label, cleanup, nil)

T1..Tn: Runtime
    ├── EventBus.Publish(channel, payload)
    │   └── go handler(event)
    │       └── fyne.Do(label.SetText(resolved))
    └── (repeats for each event)

Tend: Teardown (screen navigation)
    ├── prevCleanup() called by ui.Navigate
    │   └── sync.Once: LocalEventBus.Unsubscribe(channel, subID)
    └── label widget is discarded (GC)
```

### 6.2 Subscribe Timing

- **When:** During `composeWithState`, synchronously, before the widget is returned to the caller.
- **Where:** Inside the `case "label":` branch.
- **Guards:** `node.DataSource != ""` AND `LocalEventBus != nil`.

### 6.3 Unsubscribe Timing

- **When:** When the cleanup func is called by the screen teardown path (`ui.Navigate` → `prevCleanup()`).
- **Where:** The cleanup func closes over `channel` and `subID`.
- **Idempotency:** `sync.Once` ensures `Unsubscribe` is called exactly once, even if `cleanup()` is invoked multiple times.
- **After unsubscribe:** The handler goroutine may still be in-flight if `Publish` was called concurrently. The handler will complete and call `fyne.Do`, but the widget may already be detached from the window. This is safe — `fyne.Do` on a detached widget is a no-op.

### 6.4 Container Cleanup Aggregation

When a reactive label is inside a container:

```go
// container case (compositor.go:178-180):
cleanup := func() {
    for _, c := range cleanups {
        c()  // calls the label's sync.Once-wrapped Unsubscribe
    }
}
```

The container's cleanup iterates all child cleanups. The label's cleanup is automatically included. No changes to container aggregation logic.

### 6.5 Comparison with data_grid Lifecycle

| Aspect | data_grid | reactive label |
|---|---|---|
| Subscribe | Yes, at compose time | Yes, at compose time |
| `context.CancelFunc` | Yes (for `fetchGridDataAsync`) | **No** (no long-running goroutines) |
| `sync.WaitGroup` | Yes (waits for in-flight fetches) | **No** (handler is fire-and-forget) |
| `sync.Once` in cleanup | Yes | Yes |
| `Unsubscribe` | Yes | Yes |
| Cleanup complexity | ~20 lines (cancel + wait + unsub) | ~5 lines (unsub only) |

---

## 7. Test Specification

### 7.1 Unit Tests — `resolvePath`

**File:** `pkg/ui/compositor_test_internal_test.go` (package `ui`, internal test access)

| Test Case | Input `data` | Input `path` | Expected | Notes |
|---|---|---|---|---|
| Nested 3-level map | `{"transaccion": {"id": 101, "detalles": {"moneda": "USD", "valor": 500.0}}}` | `"transaccion.detalles.valor"` | `500.0` (float64) | Acceptance criterion from spec |
| Nested 3-level string | same | `"transaccion.detalles.moneda"` | `"USD"` (string) | |
| Nested 2-level int | same | `"transaccion.id"` | `101` (int) | |
| Map subtree | same | `"transaccion"` | `map[string]any{"id":101, "detalles":...}` | Returns subtree, not scalar |
| Missing key | same | `"transaccion.inexistente"` | `nil` | |
| Missing nested key | same | `"transaccion.detalles.inexistente"` | `nil` | |
| Nil data | `nil` | `"foo.bar"` | `nil` | |
| Empty path | any data | `""` | `nil` | |
| Non-map intermediate | `{"a": "scalar"}` | `"a.b.c"` | `nil` | `"scalar"` is not a map |
| Single-level key | `{"name": "test"}` | `"name"` | `"test"` | |
| Single-level missing | `{"name": "test"}` | `"other"` | `nil` | |
| Boolean leaf | `{"flag": true}` | `"flag"` | `true` (bool) | |
| Nil leaf value | `{"key": nil}` | `"key"` | `nil` | Distinguish from missing key — both return nil |
| Empty map | `{}` | `"anything"` | `nil` | |

### 7.2 Unit Tests — `renderTemplate`

**File:** `pkg/ui/compositor_test_internal_test.go` (package `ui`, internal test access)

| Test Case | Template | Data | Expected | Notes |
|---|---|---|---|---|
| Multi-token with scalars | `"Monto: {transaccion.detalles.valor} {transaccion.detalles.moneda}"` | (full nested map from §2.1) | `"Monto: 500 USD"` | Acceptance criterion from spec |
| Single token | `"ID: {transaccion.id}"` | same | `"ID: 101"` | |
| No tokens | `"Sin tokens"` | any | `"Sin tokens"` | |
| Missing token | `"Missing: {no.existe}"` | same | `"Missing: {no.existe}"` | Token preserved |
| Empty template | `""` | any | `""` | |
| Empty braces | `"Value: {}"` | `{"x": 1}` | `"Value: {}"` | Empty path preserved |
| Whitespace in braces | `"Value: { x }"` | `{"x": 1}` | `"Value: 1"` | Whitespace trimmed |
| Adjacent tokens | `"{a}{b}"` | `{"a": "X", "b": "Y"}` | `"XY"` | |
| Mixed literal and tokens | `"A {a} B {b} C"` | `{"a": "1", "b": "2"}` | `"A 1 B 2 C"` | |
| Unclosed brace | `"Literal {"` | any | `"Literal {"` | |
| Unclosed brace with text after | `"Start {middle end"` | `{"middle": "X"}` | `"Start {middle end"` | No closing `}` |
| Nil data | `"{a}"` | `nil` | `"{a}"` | All tokens preserved |
| Empty data map | `"{a}"` | `{}` | `"{a}"` | |
| Repeated token | `"{a} and {a}"` | `{"a": "X"}` | `"X and X"` | |
| Boolean value | `"Active: {active}"` | `{"active": true}` | `"Active: true"` | `fmt.Sprintf("%v", true)` |
| Integer value | `"Count: {n}"` | `{"n": 42}` | `"Count: 42"` | |
| Float value | `"Rate: {r}"` | `{"r": 3.14}` | `"Rate: 3.14"` | |

### 7.3 Integration Tests — Reactive Label Composition

**File:** `pkg/ui/compositor_test.go` (package `ui_test`, external test access)

All integration tests use the existing test infrastructure:
- `test.NewApp()` from `TestMain` (headless Fyne app).
- `eventbus.NewEventBus()` to create a fresh bus.
- `ui.LocalEventBus = eb` to inject the bus.
- Restore `ui.LocalEventBus = nil` in deferred cleanup.

#### 7.3.1 TestCompose_Label_Reactive_UpdatesOnEvent

**Purpose:** Verify that a reactive label receives an event, resolves the template, and updates its text.

**Setup:**
```go
eb := eventbus.NewEventBus()
defer func() { ui.LocalEventBus = nil }()
ui.LocalEventBus = eb

node := ui.NodeMeta{
    Area:         "detail",
    ComponentRef: "label",
    Label:        "Monto: {transaccion.detalles.valor} {transaccion.detalles.moneda}",
    DataSource:   "publish_selection",
}
```

**Steps:**
1. Compose the label: `obj, cleanup, err := ui.Compose(node, state, ds)`
2. Assert no error.
3. Assert `obj` is `*widget.Label`.
4. Assert initial text is the raw template string.
5. Publish: `eb.Publish("publish_selection", map[string]any{...the nested map...})`
6. Wait for goroutine delivery (e.g. `time.Sleep(100 * time.Millisecond)` or channel-based sync).
7. Assert `label.Text == "Monto: 500 USD"`.
8. Call cleanup.

#### 7.3.2 TestCompose_Label_Static_NoDataSource

**Purpose:** Verify that a label with empty DataSource remains static.

**Setup:**
```go
eb := eventbus.NewEventBus()
defer func() { ui.LocalEventBus = nil }()
ui.LocalEventBus = eb

node := ui.NodeMeta{
    Area:         "static_label",
    ComponentRef: "label",
    Label:        "Username:",
    DataSource:   "",
}
```

**Steps:**
1. Compose the label.
2. Assert `label.Text == "Username:"`.
3. Publish to a channel.
4. Wait.
5. Assert `label.Text == "Username:"` (unchanged).
6. Call cleanup (no-op).

#### 7.3.3 TestCompose_Label_Static_NilBus

**Purpose:** Verify that a label with non-empty DataSource but nil EventBus falls back to static.

**Setup:**
```go
ui.LocalEventBus = nil

node := ui.NodeMeta{
    Area:         "detail",
    ComponentRef: "label",
    Label:        "Value: {x}",
    DataSource:   "publish_selection",
}
```

**Steps:**
1. Compose the label.
2. Assert `label.Text == "Value: {x}"` (raw template, never resolved).
3. Call cleanup (no-op).

#### 7.3.4 TestCompose_Label_Reactive_CleanupUnsubscribes

**Purpose:** Verify that cleanup removes the subscription from the EventBus.

**Setup:** Same as 7.3.1.

**Steps:**
1. Compose the label.
2. Cast bus to `*eventbus.InMemEventBus`.
3. Assert `SubscriberCount("publish_selection") >= 1`.
4. Call cleanup.
5. Assert `SubscriberCount("publish_selection") == 0`.

#### 7.3.5 TestCompose_Label_Reactive_IdempotentCleanup

**Purpose:** Verify that calling cleanup twice is safe (sync.Once).

**Steps:**
1. Compose the label.
2. Assert `SubscriberCount >= 1`.
3. Call cleanup.
4. Call cleanup again (no panic, no error).
5. Assert `SubscriberCount == 0` (same as after first call).

#### 7.3.6 TestCompose_Label_Reactive_EventPrefix

**Purpose:** Verify that `DataSource: "event:custom_channel"` subscribes to `"custom_channel"`.

**Steps:**
1. Compose with `DataSource: "event:custom_channel"`.
2. Assert `SubscriberCount("custom_channel") >= 1`.
3. Assert `SubscriberCount("event:custom_channel") == 0` (prefix stripped).
4. Publish to `"custom_channel"` with a payload.
5. Assert label text updated.
6. Call cleanup.

#### 7.3.7 TestCompose_Label_Reactive_BadPayloadSkips

**Purpose:** Verify that a non-map payload is logged and the label retains its previous text.

**Steps:**
1. Compose a reactive label.
2. Publish initial valid payload → verify label updates.
3. Publish invalid payload (e.g. `"string"`, `42`, `nil`).
4. Wait.
5. Assert label text is still the text from step 2 (unchanged).

#### 7.3.8 TestCompose_Label_Reactive_MultipleEvents

**Purpose:** Verify that the label updates on each successive event.

**Steps:**
1. Compose a reactive label with template `"Total: {amount}"`.
2. Publish `{"amount": 100.0}` → assert `"Total: 100"`.
3. Publish `{"amount": 200.0}` → assert `"Total: 200"`.
4. Publish `{"amount": 0}` → assert `"Total: 0"`.

---

## 8. File Impact Summary

| File | Action | Scope |
|---|---|---|
| `pkg/ui/compositor.go` | **Modify** `case "label"` branch (lines 182-183). Add `resolvePath`, `renderTemplate`, `parseChannelName` as unexported functions. | ~80 lines added |
| `pkg/ui/compositor_test_internal_test.go` | **Add** unit tests for `resolvePath` and `renderTemplate`. | ~120 lines added |
| `pkg/ui/compositor_test.go` | **Add** integration tests for reactive label composition, cleanup, idempotency. | ~200 lines added |

**No changes to:** `pkg/eventbus/eventbus.go`, `pkg/ui/screen_state.go`, `docker/init-db/`, `NodeMeta` struct, or any database schema.

---

## 9. Dependencies and Imports

New imports needed in `compositor.go`:
- `strings` (already imported) — for `strings.Split`, `strings.HasPrefix`, `strings.TrimPrefix`, `strings.TrimSpace`, `strings.Builder`.
- `fmt` (already imported) — for `fmt.Sprintf` in `renderTemplate`.
- `sync` (already imported) — for `sync.Once` in cleanup.
- `log` (already imported) — for warning logs.
- `fyne.io/fyne/v2` — for `fyne.Do` (already used throughout the file).
- `fyne.io/fyne/v2/widget` — for `widget.Label` (already imported).

**No new external dependencies.** All new code uses standard library and existing Fyne imports only.
