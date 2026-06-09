# Design Document: grid-selection-button-nav

**Change ID:** `grid-selection-button-nav`
**Date:** 2026-06-09
**Status:** Design
**Specs:** 019 (Datagrid Native Type Preservation), 018 (Action Button State Navigation)
**Delivery:** Two chained PRs — PR-1 (Spec 019), PR-2 (Spec 018, depends on PR-1)

---

## 1. Overview

- **PR-1** migrates the datagrid data pipeline from `[][]string` to `[][]any`, preserving native Go types from database fetch through selection events.
- **PR-2** builds reactive button behavior on top of PR-1's native type selection payload.

### Key Constraints

- `Navigate func(vistaID string)` signature preserved — query string parsed internally.
- `FormatValue` function signature and internals preserved unchanged.
- All UI mutations dispatched via `fyne.Do()`.
- No database schema changes.

---

## 2. PR-1: Data Type Migration Plan (Spec 019)

### 2.1 DataSet.Rows Type Change — `pkg/ui/datasource.go`

```go
// Before
Rows [][]string
// After
Rows [][]any
```

### 2.2 SQLDataSource.Fetch Refactoring — `pkg/dataaccess/sql_datasource.go`

Remove `FormatValue` loop. Store `rows.Values()` (`[]any`) directly.

Add `unwrapNumeric` helper:
```go
func unwrapNumeric(val any) any {
    if pn, ok := val.(*pgtype.Numeric); ok {
        f64, err := pn.Float64Value()
        if err == nil { return f64.Float64 }
    }
    if pn, ok := val.(pgtype.Numeric); ok {
        f64, err := pn.Float64Value()
        if err == nil { return f64.Float64 }
    }
    return val
}
```

New import: `"github.com/jackc/pgx/v5/pgtype"`

### 2.3 dataGridModel Migration — `pkg/ui/compositor.go`

```go
rows          [][]any    // was [][]string
masterRows    [][]any    // was [][]string
```

### 2.4 UpdateCell — Render-Time FormatValue

```go
label.SetText(dataaccess.FormatValue(row[id.Col]))
```

New import: `"GolemUI/pkg/dataaccess"` in compositor.go

### 2.5 filterMasterRows — FormatValue for Comparison

```go
cellVal := dataaccess.FormatValue(row[col])
// ...
var filtered [][]any    // was [][]string
```

### 2.6 OnSelected — No Code Change Needed

`rowMap[headers[i]] = row[i]` naturally carries native `any` types after migration.

### 2.7 Test Fixture Migration

Pattern: `[][]string{{"1", "Book A", "25.5"}}` → `[][]any{{1, "Book A", 25.5}}`

Selection assertions: `"42"` → `42` (native int)

### 2.8 PR-1 Import Changes

| File | Added Import |
|------|-------------|
| `pkg/ui/compositor.go` | `"GolemUI/pkg/dataaccess"` |
| `pkg/dataaccess/sql_datasource.go` | `"github.com/jackc/pgx/v5/pgtype"` |

---

## 3. PR-2: Reactive Button Design (Spec 018)

### 3.1 NodeMeta Extension — `pkg/ui/compositor.go`

```go
ParamMapping     map[string]string `json:"param_mapping,omitempty"`
```

Reuses existing `DataSource string` field for selection channel resolution.

### 3.2 Button Composition Flow

Restructured into branches:

1. **Reactive nav button** (new): `DataSource != ""` + `navigate:` + EventBus present → `composeReactiveNavButton`
2. **Static nav button** (existing): `navigate:` without `DataSource`
3. **Submit button** (existing)
4. **Inert button** (existing)

### 3.3 `composeReactiveNavButton` — New Function

- Button starts disabled via `btn.Disable()`
- Subscribes to channel from `parseChannelName(node.DataSource)`
- Stores `lastSelection` under `sync.Mutex`
- Enables/disables via `fyne.Do()`
- Click handler: resolves `param_mapping` via `resolvePath`, builds query string, calls `Navigate`
- Cleanup: `sync.Once` unsubscribes from channel

### 3.4 `buildQueryParams` — New Helper

```go
func buildQueryParams(selection map[string]any, mapping map[string]string) string {
    var parts []string
    for key, path := range mapping {
        val := resolvePath(selection, path)
        if val == nil { continue }
        encoded := url.QueryEscape(fmt.Sprintf("%v", val))
        parts = append(parts, url.QueryEscape(key)+"="+encoded)
    }
    sort.Strings(parts)
    return strings.Join(parts, "&")
}
```

Imports: `"net/url"`, `"sort"`

### 3.5 Navigate Query String Parsing — `cmd/golemui/main.go`

New `parseNavigateTarget` function:
```go
func parseNavigateTarget(vID string) (string, map[string]string)
```

Edge cases: no `?` → plain vistaID; empty query → nil params; empty vistaID before `?` → treat as plain string.

### 3.6 ScreenState.Preload — `pkg/ui/screen_state.go`

```go
func (s *ScreenState) Preload(params map[string]any) {
    s.mu.Lock()
    defer s.mu.Unlock()
    for k, v := range params {
        if _, exists := s.data[k]; !exists {
            s.data[k] = v
        }
    }
}
```

"No overwrite" semantics: existing values preserved.

### 3.7 ComposeWithParams — `pkg/ui/compositor.go`

```go
func ComposeWithParams(node NodeMeta, vistaID string, params map[string]string) (fyne.CanvasObject, func(), error) {
    state := NewScreenState(vistaID)
    if len(params) > 0 {
        anyMap := make(map[string]any, len(params))
        for k, v := range params { anyMap[k] = v }
        state.Preload(anyMap)
    }
    return composeWithState(node, state)
}
```

---

## 4. Thread Safety Model

- EventBus handlers run in goroutines → `fyne.Do()` required for all widget mutations
- `lastSelection` guarded by `sync.Mutex`
- Pattern mirrors existing reactive label subscription

---

## 5. Error Handling

| Case | Behavior |
|------|----------|
| Invalid param_mapping path | Skip parameter (no `key=` in query string) |
| Malformed query string | Treat as plain vistaID |
| Nil EventBus with DataSource | Fall through to static navigation (no dead button) |
| Empty selection | Disable button |

---

## 6. File-by-File Change Inventory

### PR-1 (~103 lines)

| File | Change | Lines |
|------|--------|-------|
| `pkg/ui/datasource.go` | `Rows` type change | ~3 |
| `pkg/dataaccess/sql_datasource.go` | Remove FormatValue loop, add unwrapNumeric, pgtype import | ~25 |
| `pkg/ui/compositor.go` | dataGridModel types, UpdateCell/filterMasterRows FormatValue, dataaccess import | ~15 |
| `pkg/ui/compositor_test.go` | Fixture migration, selection assertions | ~50 |
| `pkg/dataaccess/sql_datasource_test.go` | Native type assertions, remove string check | ~10 |

### PR-2 (~505 lines)

| File | Change | Lines |
|------|--------|-------|
| `pkg/ui/compositor.go` | NodeMeta.ParamMapping, button refactor, composeReactiveNavButton, buildQueryParams, ComposeWithParams | ~95 |
| `pkg/ui/screen_state.go` | Preload method | ~15 |
| `cmd/golemui/main.go` | parseNavigateTarget, Navigate callback update | ~35 |
| `pkg/ui/compositor_button_test.go` | New file — 12+ test cases | ~300 |
| `pkg/ui/screen_state_test.go` | New file — Preload tests | ~60 |

---

## Appendix: Design Decisions

| Decision | Rationale |
|----------|-----------|
| Reuse `DataSource` for button selection channel | Semantic consistency; avoids new field |
| `unwrapNumeric` at fetch boundary | Keeps pgtype out of entire pipeline |
| `ComposeWithParams` separate from `Compose` | Preserves backward compatibility |
| `Preload` "no overwrite" | Matches spec; safe for future defaults |
| `sort.Strings(parts)` in buildQueryParams | Deterministic output for tests |
| Separate `compositor_button_test.go` | compositor_test.go already ~2500 lines |
| Nil EventBus → static fallback | Prevents dead buttons; graceful degradation |
