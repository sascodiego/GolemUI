# Proposal: Reactive Label Binding

**Change ID:** `reactive-label-binding`
**Spec source:** `docs/specify/archived/017-reactive-label-binding.md`
**Status:** Draft

---

## 1. Summary

Make the `label` component reactive: when a label's `NodeMeta.DataSource` specifies an EventBus channel, the compositor subscribes the label to that channel at compose time and resolves `{token}` placeholders in `node.Label` against event payloads using dot-notation path navigation. The label text updates in real time on the Fyne UI thread via `fyne.Do`. Static labels (empty `DataSource`) remain unchanged.

---

## 2. Problem Statement

The `label` component (`compositor.go:182-183`) renders `node.Label` as a static `widget.NewLabel(...)` and never changes after composition. There is no mechanism for a label to react to user actions or data changes at runtime.

This means:

- A user selects a row in a data_grid, but no label can display the selected entity's details.
- A form submit fires, but no label can show a confirmation message derived from the submitted snapshot.
- The `data_grid` and `button` components already publish rich payloads to the EventBus (`"publish_selection"`, `"screen:submit:<vistaID>"`), but only other data_grids consume them.

The EventBus publish side is fully wired; the label subscribe side is missing.

---

## 3. Proposed Solution

Add reactive binding to the `label` case in `composeWithState` using three new pieces:

### 3.1 `resolvePath(data any, path string) any`

A recursive helper that navigates a `map[string]any` hierarchy using a dot-separated path string (e.g. `"transaccion.detalles.valor"` → `500.0`). Returns `nil` for missing or non-navigable segments.

**Scope:** maps-only. No array index access. Scalars are returned as-is. Non-map intermediate values short-circuit to `nil`.

### 3.2 `renderTemplate(tmpl string, data map[string]any) string`

A template processor that scans `tmpl` for `{...}` tokens, resolves each token's inner path against `data` via `resolvePath`, and replaces the token with `fmt.Sprintf("%v", value)`. Tokens whose paths resolve to `nil` are preserved as their original literal text (e.g. `{cliente.nombre}` stays if the path is missing).

### 3.3 Modified `case "label"` in `composeWithState`

When `node.DataSource != ""` and `LocalEventBus != nil`:

1. Parse the channel name from `node.DataSource` (strip `"event:"` prefix if present; otherwise use the literal value).
2. Create `widget.NewLabel(node.Label)`.
3. Subscribe to the channel with a handler that:
   - Type-asserts `ev.Payload` to `map[string]any`; logs a warning and returns if the assertion fails.
   - Calls `renderTemplate(node.Label, payload)` to produce the resolved text.
   - Wraps `label.SetText(resolved)` in `fyne.Do(func() { ... })` for thread safety.
4. Capture the `subID` and return a `sync.Once`-wrapped cleanup func that calls `LocalEventBus.Unsubscribe(channel, subID)`.

When `node.DataSource == ""` or `LocalEventBus == nil`, behavior is identical to the current implementation: static label, no-op cleanup.

---

## 4. Scope

### In Scope

- `resolvePath` function (recursive dot-notation resolver for `map[string]any`).
- `renderTemplate` function (`{token}` template parser).
- Modified `case "label"` in `composeWithState` with EventBus subscription.
- `sync.Once` cleanup func for proper lifecycle management.
- Unit tests for `resolvePath` and `renderTemplate` (internal package tests).
- Integration tests for reactive label composition (subscribe → publish → assert text).
- Cleanup tests (unsubscribe on teardown, idempotent cleanup).

### Out of Scope

- Array index access in `resolvePath` (e.g. `{items.0.name}`). Deferred to a follow-up.
- Reactive bindings for `button` or other non-label widgets (spec 018 covers button state).
- Bidirectional bindings (label → state). The label is read-only.
- New fields on `NodeMeta`. `DataSource` and `Label` are sufficient.
- Changes to `docker/init-db/` scripts. This change is entirely in the Go client (Capa 4).
- Changes to the EventBus API. Subscribe/Unsubscribe/Publish already support the needed semantics.
- Any modification to data_grid or button publish behavior.

---

## 5. Business Rules

### BR-01: Channel Name Resolution

| `node.DataSource` value | Subscribed channel |
|---|---|
| `""` (empty) | No subscription. Static label. |
| `"publish_selection"` | `"publish_selection"` (literal) |
| `"event:custom_channel"` | `"custom_channel"` (prefix stripped) |
| Any other non-empty string | Used as-is (literal channel name) |

This mirrors the data_grid's `"state:"` prefix convention (`compositor.go:259-263`).

### BR-02: Payload Type Assertion

The handler expects `ev.Payload` to be `map[string]any`. If the payload is `nil` or any other type, the handler logs a warning and skips the update. The label retains its previous text.

### BR-03: Template Token Resolution

- Tokens are delimited by `{` and `}`.
- The text between delimiters is treated as a dot-notation path.
- `resolvePath` navigates the payload map. If the path resolves to a non-nil value, `fmt.Sprintf("%v", value)` replaces the token.
- If the path resolves to `nil` (missing key, non-map intermediate, empty path), the original token text is preserved verbatim.

### BR-04: Thread Safety

All `label.SetText(...)` calls from the Subscribe handler MUST be wrapped in `fyne.Do(func() { ... })`. The EventBus invokes each handler in a fresh goroutine (`go h(event)` at `eventbus.go:75`), so direct widget mutation would race with the Fyne UI thread.

This is mandated by:
- The `compositor.go` package comment (lines 1-9).
- The `fyne-thread-safety-v2` change (REQ-EB-01).

### BR-05: Lifecycle — Subscribe at Compose, Unsubscribe at Teardown

- Subscription happens once during `composeWithState` when the label is created.
- The returned cleanup func calls `LocalEventBus.Unsubscribe(channel, subID)` wrapped in `sync.Once` for idempotency.
- When the label is inside a container, the container's cleanup aggregates the label's cleanup automatically (existing `compositor.go:178-180` pattern).
- No `context.CancelFunc` or `sync.WaitGroup` needed (the label handler is fire-and-forget — it queues a `fyne.Do` and returns).

### BR-06: Static Label Preservation

When `node.DataSource == ""`, the label is created as `widget.NewLabel(node.Label)` with a no-op cleanup func, identical to the current implementation. No subscription, no template processing.

### BR-07: Bus Nil Guard

When `LocalEventBus == nil`, the label falls back to static behavior regardless of `node.DataSource`. Mirrors the data_grid guard at `compositor.go:248`.

---

## 6. Acceptance Criteria

### AC-01: `resolvePath` correctness

Given `data = map[string]any{"transaccion": map[string]any{"id": 101, "detalles": map[string]any{"moneda": "USD", "valor": 500.0}}}`:

- `resolvePath(data, "transaccion.detalles.valor")` returns `500.0` (float64).
- `resolvePath(data, "transaccion.detalles.moneda")` returns `"USD"` (string).
- `resolvePath(data, "transaccion.id")` returns `101` (int).
- `resolvePath(data, "transaccion.inexistente")` returns `nil`.
- `resolvePath(data, "")` returns `nil`.
- `resolvePath(nil, "foo.bar")` returns `nil`.

### AC-02: `renderTemplate` correctness

Given the same `data`:

- `renderTemplate("Monto: {transaccion.detalles.valor} {transaccion.detalles.moneda}", data)` produces exactly `"Monto: 500 USD"`.
- `renderTemplate("ID: {transaccion.id}", data)` produces `"ID: 101"`.
- `renderTemplate("Sin tokens", data)` produces `"Sin tokens"`.
- `renderTemplate("Missing: {no.existe}", data)` produces `"Missing: {no.existe}"` (token preserved).

### AC-03: Reactive label updates on event

1. Compose a label with `Label: "Monto: {transaccion.detalles.valor} {transaccion.detalles.moneda}"` and `DataSource: "publish_selection"`.
2. Publish `{"transaccion": {"detalles": {"valor": 500.0, "moneda": "USD"}}}` to `"publish_selection"`.
3. After event delivery, assert `label.Text == "Monto: 500 USD"`.

### AC-04: Static label unaffected

Compose a label with `Label: "Username:"` and `DataSource: ""`. Assert `label.Text == "Username:"` and cleanup is a no-op (non-nil, callable, no side effects).

### AC-05: Cleanup unsubscribes

1. Compose a reactive label.
2. Assert `SubscriberCount(channel) >= 1`.
3. Call cleanup.
4. Assert `SubscriberCount(channel) == 0`.

### AC-06: Cleanup is idempotent

Call cleanup twice. Second call is a no-op. No panic, no double-unsubscribe side effects (verified via `sync.Once`).

---

## 7. Impact Analysis

### Files Modified

| File | Change |
|---|---|
| `pkg/ui/compositor.go` | Expand `case "label"` from 2 lines to ~30 lines (subscribe, handler, cleanup). Add `resolvePath` and `renderTemplate` as unexported helpers. |
| `pkg/ui/compositor_test_internal_test.go` | Add unit tests for `resolvePath` and `renderTemplate`. |
| `pkg/ui/compositor_test.go` | Add integration tests: reactive label composition, cleanup, idempotency. |

### Files NOT Modified

| File | Reason |
|---|---|
| `pkg/eventbus/eventbus.go` | API already supports subscribe/unsubscribe/publish. |
| `pkg/ui/screen_state.go` | No changes to state management. |
| `docker/init-db/*` | No schema changes; this is Capa 4 (Go client only). |
| `NodeMeta` struct | No new fields; `DataSource` + `Label` are sufficient. |

### Dependencies

- `fyne.Do` — already imported and used throughout `compositor.go`.
- `eventbus.Handler` — already imported.
- `sync.Once` — already imported (used in data_grid cleanup).
- `strings`, `fmt`, `regexp` or manual `{`/`}` scanning — standard library only.

### Risk

| Risk | Likelihood | Mitigation |
|---|---|---|
| `fyne.Do` not executed synchronously in tests | Medium | Use `time.Sleep` or channel-based wait after publish, matching existing test patterns (`compositor_test.go:1167-1264`). |
| Token parser edge cases (nested braces, escaped braces) | Low | Spec defines simple `{path}` tokens. No nesting, no escaping. Reject malformed tokens (unclosed `{`) by preserving literal text. |
| Goroutine leak if cleanup is never called | Low | Screen teardown in `ui.Navigate` always calls cleanup. Container aggregation ensures leaf cleanups run. |

---

## 8. Rollback Plan

This change is **Go client only** (Capa 4). No database schema changes, no migration files, no `docker/init-db/` modifications.

**Rollback:** Revert the `case "label"` branch in `compositor.go` to the original 2-line static implementation. Remove `resolvePath`, `renderTemplate`, and the new tests. Zero impact on other components, the EventBus, or the database.

---

## 9. Constraints

1. **Thread safety:** All Fyne widget mutations from Subscribe handlers MUST use `fyne.Do(func() { ... })`. No exceptions.
2. **No new NodeMeta fields:** The reactive binding is configured entirely through the existing `DataSource` and `Label` fields.
3. **Maps-only resolvePath:** `resolvePath` navigates `map[string]any` only. Array index access is deferred.
4. **No external dependencies:** The template parser uses standard library only (`strings`, `fmt`, possibly `regexp`). No template engine imports.
5. **Production-first:** No mocks, no shortcuts. Tests exercise real EventBus publish/subscribe with real Fyne widgets (headless via `fyne.io/fyne/v2/test`).
6. **Static label preservation:** Empty `DataSource` → identical behavior to current implementation. Zero regression risk for existing screens.
7. **Cleanup contract:** Every label returns a non-nil cleanup func. Reactive labels return a `sync.Once`-wrapped unsubscribe; static labels return a no-op.
8. **Separation of concerns:** `resolvePath` and `renderTemplate` are pure functions with no dependency on Fyne, EventBus, or compositor state. They are independently testable.
