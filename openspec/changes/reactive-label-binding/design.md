# Design: Reactive Label Binding

**Change ID:** `reactive-label-binding`
**Spec source:** `docs/specify/archived/017-reactive-label-binding.md`
**Proposal:** `openspec/changes/reactive-label-binding/proposal.md`
**Spec:** `openspec/changes/reactive-label-binding/spec.md`
**Status:** Draft

---

## 1. Module Layout

All production code resides in the existing `pkg/ui` package. No new files or packages are created.

| Function | Package | Exported | File | Location |
|---|---|---|---|---|
| `resolvePath` | `pkg/ui` | No | `compositor.go` | After `extractOrderedArgs` (end of file), before package closing |
| `renderTemplate` | `pkg/ui` | No | `compositor.go` | Immediately after `resolvePath` |
| `parseChannelName` | `pkg/ui` | No | `compositor.go` | Immediately after `renderTemplate` |
| `case "label"` expansion | `pkg/ui` | N/A | `compositor.go` | Replaces lines 182-183 inside `composeWithState` |

### Test locations

| Test type | File | Package | Access |
|---|---|---|---|
| Unit tests for `resolvePath` | `pkg/ui/compositor_test_internal_test.go` | `package ui` | Internal — can call unexported functions directly |
| Unit tests for `renderTemplate` | `pkg/ui/compositor_test_internal_test.go` | `package ui` | Internal — can call unexported functions directly |
| Unit tests for `parseChannelName` | `pkg/ui/compositor_test_internal_test.go` | `package ui` | Internal — can call unexported functions directly |
| Integration tests (compose → publish → assert) | `pkg/ui/compositor_test.go` | `package ui_test` | External — uses exported `Compose` function |

---

## 2. Function Signatures and Implementation Design

### 2.1 `resolvePath`

**Signature:**
```go
func resolvePath(data any, path string) any
```

**Implementation (iterative, maps-only):**

```go
func resolvePath(data any, path string) any {
    if data == nil || path == "" {
        return nil
    }

    current := data
    parts := strings.Split(path, ".")

    for _, part := range parts {
        m, ok := current.(map[string]any)
        if !ok {
            return nil
        }
        val, exists := m[part]
        if !exists {
            return nil
        }
        current = val
    }

    return current
}
```

**Key design decisions:**
- Iterative loop (not recursive) — avoids stack depth concerns for deeply nested paths.
- `strings.Split("a", ".")` returns `["a"]` — single-level path works naturally.
- `strings.Split("", ".")` returns `[""]` — but the early `path == ""` guard prevents this.
- All type assertions use comma-ok pattern — never panics.
- Returns raw Go value from the map. String conversion is the caller's responsibility.

**Complexity:** O(d) where d = depth of the dot-path (number of segments). Each iteration does one type assertion and one map lookup.

---

### 2.2 `renderTemplate`

**Signature:**
```go
func renderTemplate(tmpl string, data map[string]any) string
```

**Implementation (single-pass scanner with `strings.Builder`):**

```go
func renderTemplate(tmpl string, data map[string]any) string {
    var result strings.Builder
    i := 0

    for i < len(tmpl) {
        if tmpl[i] == '{' {
            closeIdx := strings.IndexByte(tmpl[i+1:], '}')
            if closeIdx == -1 {
                // No closing brace — write remaining as literal
                result.WriteString(tmpl[i:])
                break
            }
            closeIdx += i + 1 // absolute index of '}'

            path := strings.TrimSpace(tmpl[i+1 : closeIdx])
            if path == "" {
                // Empty path — preserve literal token
                result.WriteString(tmpl[i : closeIdx+1])
            } else {
                value := resolvePath(data, path)
                if value != nil {
                    result.WriteString(fmt.Sprintf("%v", value))
                } else {
                    result.WriteString(tmpl[i : closeIdx+1]) // preserve literal
                }
            }
            i = closeIdx + 1
        } else {
            result.WriteByte(tmpl[i])
            i++
        }
    }

    return result.String()
}
```

**Key design decisions:**
- `strings.IndexByte(tmpl[i+1:], '}')` searches from after the `{` to find the **next** `}`. This implements "first `{` pairs with next `}`" — no nesting.
- `strings.TrimSpace` on the inner path allows `{ transaccion.id }` with whitespace.
- `fmt.Sprintf("%v", value)` converts any scalar to its default string representation: `500` for int, `500` for float64 (when whole), `USD` for string, `true` for bool.
- Unresolved tokens (nil value, empty path) are preserved as their original text.
- No regex, no `text/template` — standard library only.

**Complexity:** O(n) where n = len(tmpl). Single pass. `resolvePath` is called once per token.

---

### 2.3 `parseChannelName`

**Signature:**
```go
func parseChannelName(dataSource string) string
```

**Implementation:**

```go
func parseChannelName(dataSource string) string {
    if strings.HasPrefix(dataSource, "event:") {
        return strings.TrimPrefix(dataSource, "event:")
    }
    return dataSource
}
```

**Behavior table:**

| Input | Output |
|---|---|
| `"publish_selection"` | `"publish_selection"` |
| `"event:custom_channel"` | `"custom_channel"` |
| `"screen:submit:vista_1"` | `"screen:submit:vista_1"` |
| `""` | `""` (but never called when empty — early return in case "label") |

**Key design decision:** The `"event:"` prefix is the only special prefix. All other values (including `"screen:submit:..."`) are used as-is. This mirrors the data_grid's `"state:"` prefix convention at `compositor.go:259-263`.

---

### 2.4 Modified `case "label"` — Full Implementation

**Replaces `compositor.go` lines 182-183:**

```go
// Before (current):
case "label":
    return widget.NewLabel(node.Label), func() {}, nil
```

**After (new implementation):**

```go
case "label":
    label := widget.NewLabel(node.Label)

    if node.DataSource == "" || LocalEventBus == nil {
        return label, func() {}, nil
    }

    channel := parseChannelName(node.DataSource)
    log.Printf("[UI/Label] Subscribing label at area %q to channel %q", node.Area, channel)

    tmpl := node.Label // capture template string in closure
    subID := LocalEventBus.Subscribe(channel, func(ev eventbus.Event) {
        payload, ok := ev.Payload.(map[string]any)
        if !ok {
            log.Printf("[UI/Label] Warning: payload on channel %q is not map[string]any, skipping update", channel)
            return
        }
        resolved := renderTemplate(tmpl, payload)
        fyne.Do(func() {
            label.SetText(resolved)
        })
    })

    var once sync.Once
    cleanup := func() {
        once.Do(func() {
            LocalEventBus.Unsubscribe(channel, subID)
        })
    }

    return label, cleanup, nil
```

**Design notes:**

1. **Static path guard** (`node.DataSource == "" || LocalEventBus == nil`): both conditions result in identical static behavior — no subscription, no-op cleanup. Combined into a single early return to keep the reactive path indentation flat.

2. **`tmpl := node.Label`**: captures the template string explicitly. While `node` is also captured (it's a value, not a pointer), this makes it clear that the template is immutable state used by the handler.

3. **Handler closure captures:** `label` (pointer to `*widget.Label`), `tmpl` (template string), `channel` (for logging). All are immutable after capture.

4. **`fyne.Do` is mandatory** — the handler runs in a goroutine dispatched by `EventBus.Publish`. Direct `label.SetText` would race with the Fyne UI thread. Mandated by `compositor.go` package comment lines 1-9.

5. **`sync.Once` cleanup** — simpler than data_grid's cleanup because:
   - No `context.CancelFunc` (label handler is fire-and-forget).
   - No `sync.WaitGroup` (no long-running goroutines to wait for).
   - No `model.mu` lock needed (no mutable model state).
   - Just a direct `Unsubscribe(channel, subID)` call.

6. **Logging pattern** mirrors data_grid: `[UI/Label]` prefix for subscribe, `[UI/Label] Warning:` for type assertion failures.

---

## 3. Code Placement — Exact Insertion Points

### 3.1 Production code — `pkg/ui/compositor.go`

#### Insertion point 1: `case "label"` expansion

**Location:** Lines 182-183 (current)
**Before:**
```go
	case "label":
		return widget.NewLabel(node.Label), func() {}, nil
```
**After:** Replace these 2 lines with the ~25-line implementation from §2.4 above. The new code sits between the `case "container"` block (ending at line 180 with `return containerObj, cleanup, nil`) and the `case "text_input"` block (starting at line 185).

#### Insertion point 2: Helper functions

**Location:** End of file, after `extractOrderedArgs` (currently the last function, ending at line 580).

Add the three helper functions in this order:

1. `resolvePath` — ~18 lines (including doc comment)
2. `renderTemplate` — ~30 lines (including doc comment)
3. `parseChannelName` — ~8 lines (including doc comment)

**No new imports required.** All three functions use only `strings`, `fmt`, which are already imported.

### 3.2 Test code — `pkg/ui/compositor_test_internal_test.go`

**Location:** Append after the existing `TestContainsIgnoreCase` function (currently the last function, ending at line 95).

Add in this order:
1. `TestResolvePath` — table-driven test with all cases from spec §7.1 (~60 lines)
2. `TestRenderTemplate` — table-driven test with all cases from spec §7.2 (~70 lines)
3. `TestParseChannelName` — table-driven test (~25 lines)

### 3.3 Test code — `pkg/ui/compositor_test.go`

**Location:** Append after the existing tests (file is 2165 lines).

Add in this order (all in `package ui_test`):
1. `TestCompose_Label_Static_NoDataSource` (~20 lines)
2. `TestCompose_Label_Static_NilBus` (~20 lines)
3. `TestCompose_Label_Reactive_UpdatesOnEvent` (~40 lines)
4. `TestCompose_Label_Reactive_CleanupUnsubscribes` (~25 lines)
5. `TestCompose_Label_Reactive_IdempotentCleanup` (~25 lines)
6. `TestCompose_Label_Reactive_EventPrefix` (~30 lines)
7. `TestCompose_Label_Reactive_BadPayloadSkips` (~35 lines)
8. `TestCompose_Label_Reactive_MultipleEvents` (~35 lines)

**Total estimated new test lines:** ~230 lines in `compositor_test.go` + ~155 lines in `compositor_test_internal_test.go` = ~385 lines.

---

## 4. Composite Screen Structure — JSON Example

A screen with a data_grid and a reactive label that displays the selected row's details:

```json
{
  "area": "root",
  "component_ref": "container",
  "layout": {
    "type": "vertical"
  },
  "children": [
    {
      "area": "selection_detail",
      "component_ref": "label",
      "label": "Selected: {cliente.nombre} — Amount: {monto} {moneda}",
      "data_source": "publish_selection"
    },
    {
      "area": "transactions_grid",
      "component_ref": "data_grid",
      "data_source": "SELECT cliente_nombre, monto, moneda FROM transacciones",
      "filter_keys": ["cliente_nombre"]
    }
  ]
}
```

**Data flow:**
1. User selects a row in `transactions_grid`.
2. `table.OnSelected` publishes `{"cliente_nombre": "Juan", "monto": "1500", "moneda": "USD"}` to `"publish_selection"`.
3. The reactive label's handler receives the event, calls `renderTemplate("Selected: {cliente.nombre} — Amount: {monto} {moneda}", payload)`.
4. Since the payload keys are `cliente_nombre` (not `cliente.nombre`), `resolvePath` would return `nil` for `cliente.nombre`. The label would show: `"Selected: {cliente.nombre} — Amount: 1500 USD"`.

**For matching payloads** (flat map from `publish_selection`):

```json
{
  "area": "detail_panel",
  "component_ref": "container",
  "layout": {
    "type": "vertical"
  },
  "children": [
    {
      "area": "amount_label",
      "component_ref": "label",
      "label": "Amount: {monto}",
      "data_source": "publish_selection"
    },
    {
      "area": "currency_label",
      "component_ref": "label",
      "label": "Currency: {moneda}",
      "data_source": "publish_selection"
    },
    {
      "area": "static_title",
      "component_ref": "label",
      "label": "Transaction Details:",
      "data_source": ""
    }
  ]
}
```

In this layout:
- `amount_label` and `currency_label` are reactive — they subscribe to `"publish_selection"` and update when a row is selected.
- `static_title` has no `DataSource` — remains static, unchanged from current behavior.
- The container aggregates all three cleanups.

**With event: prefix** (subscribing to a custom channel):

```json
{
  "area": "custom_detail",
  "component_ref": "label",
  "label": "Status: {status} — {message}",
  "data_source": "event:status_updates"
}
```

This subscribes to channel `"status_updates"` (prefix stripped by `parseChannelName`).

**Nested payload example** (matching the spec acceptance criteria):

```json
{
  "area": "nested_detail",
  "component_ref": "label",
  "label": "Monto: {transaccion.detalles.valor} {transaccion.detalles.moneda}",
  "data_source": "publish_selection"
}
```

When `publish_selection` carries a payload structured as:
```json
{
  "transaccion": {
    "id": 101,
    "detalles": {
      "moneda": "USD",
      "valor": 500.0
    }
  }
}
```

The label resolves to: `"Monto: 500 USD"`.

---

## 5. No SQL Schema Changes

**This change is entirely within Capa 4 (Fyne Renderer / Go client).** No modifications are required to:

- `docker/init-db/` scripts (`01_create_databases.sh`, `02_init_core.sql`, `03_load_transactions.sh`).
- `golemui_core` database schema.
- `negocio_production` database schema.
- `golemui.mapeo_interfaz` (no new overrides).
- Any SQL catalog tables or stored procedures.

The `NodeMeta` struct requires no new fields — `DataSource` (already exists, `json:"data_source,omitempty"`) and `Label` (already exists, `json:"label,omitempty"`) are sufficient for all reactive label configuration.

---

## 6. Line Impact Summary

| File | Action | Lines Added (est.) | Total Change |
|---|---|---|---|
| `pkg/ui/compositor.go` | Replace 2 lines (label case) with ~25 lines; append ~56 lines (3 helpers) at end | ~79 | 580 → ~659 |
| `pkg/ui/compositor_test_internal_test.go` | Append unit tests for `resolvePath`, `renderTemplate`, `parseChannelName` | ~155 | 95 → ~250 |
| `pkg/ui/compositor_test.go` | Append 8 integration test functions | ~230 | 2165 → ~2395 |

**Total new lines:** ~464 (within the 600-line review budget).

**No other files are modified.**

---

## 7. Dependency Verification

### Existing imports in `compositor.go` (already present, no changes needed):

```go
import (
    "context"   // used by data_grid
    "fmt"       // used by renderTemplate (fmt.Sprintf)
    "log"       // used by label handler logging
    "strconv"   // used by container layout
    "strings"   // used by resolvePath, renderTemplate, parseChannelName
    "sync"      // used by label cleanup (sync.Once)

    "GolemUI/pkg/eventbus"  // used by label handler
    "fyne.io/fyne/v2"       // used by fyne.Do
    "fyne.io/fyne/v2/container"
    "fyne.io/fyne/v2/widget" // used by widget.Label
)
```

All required imports are already present. **Zero new import statements needed.**

### Test file imports:

`compositor_test_internal_test.go` currently imports only `"testing"`. The new tests need:
- `"testing"` (already imported)
- No additional imports needed — all test helpers use only `testing` and the package's own functions.

`compositor_test.go` already imports `time`, `sync`, `fyne.io/fyne/v2/test`, `fyne.io/fyne/v2/widget`, `GolemUI/pkg/eventbus`, and `GolemUI/pkg/ui`. All required for the new integration tests.

---

## 8. Execution Order (for tasks phase)

The implementation should follow this order to satisfy strict TDD:

1. **RED:** Write `TestResolvePath` in `compositor_test_internal_test.go` → verify it fails (functions don't exist yet).
2. **GREEN:** Implement `resolvePath` in `compositor.go` → verify tests pass.
3. **RED:** Write `TestRenderTemplate` → verify it fails.
4. **GREEN:** Implement `renderTemplate` → verify tests pass.
5. **RED:** Write `TestParseChannelName` → verify it fails.
6. **GREEN:** Implement `parseChannelName` → verify tests pass.
7. **RED:** Write integration tests (`TestCompose_Label_Reactive_UpdatesOnEvent`, etc.) → verify they fail.
8. **GREEN:** Expand `case "label"` in `composeWithState` → verify all tests pass.
9. **REFACTOR:** Review for any shared test helpers or consolidation.
10. **BUILD:** `go build ./...` and `go vet ./...` → verify clean compilation.
