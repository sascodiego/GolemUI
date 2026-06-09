# grid-selection-button-nav: Explore Report
**Scout Report** | 2026-06-09

---

## Executive Summary

Combined change covering two dependent specs:
1. **Spec 019 (Datagrid Native Type Preservation)**: Migrate `DataSet.Rows`, `dataGridModel.rows`, `dataGridModel.masterRows` from `[][]string` to `[][]any`. Defer `FormatValue` to render-time only. Selection events carry native Go types.
2. **Spec 018 (Action Button State Navigation)**: Reactive button enable/disable on grid selection, `param_mapping` with dot-notation resolution, extend `ui.Navigate` to parse query strings and inject params into destination `ScreenState`.

**Dependency:** 019 is prerequisite for 018 — native types from 019's selection payload feed into 018's `param_mapping` resolution.

**Total surface:** ~7 production files, ~3 test files, ~200-400 lines changed across both PRs.

---

## 1. Current Types and Function Signatures

### 1.1 `DataSet` (`pkg/ui/datasource.go`)

```go
type DataSet struct {
    Headers      []string
    Rows         [][]string    // ← TARGET: [][]any (Spec 019)
    ColumnWidths []string
}
```

`DataSource` interface:
```go
type DataSource interface {
    Fetch(ctx context.Context, source string, args ...any) (DataSet, error)
    FetchAll(ctx context.Context, source string) (DataSet, error)
}
```

`MockDataSource` uses the same `DataSet` — test fixtures must migrate from `[][]string` to `[][]any`.

### 1.2 `SQLDataSource` (`pkg/dataaccess/sql_datasource.go`)

```go
func (s *SQLDataSource) Fetch(ctx context.Context, source string, args ...any) (ui.DataSet, error) {
    // ... pool.Query → rows.Values() returns []any ...
    for rows.Next() {
        vals, _ := rows.Values()        // vals is []any — native types!
        stringRow := make([]string, len(vals))
        for i, val := range vals {
            stringRow[i] = FormatValue(val)  // ← EARLY CONVERSION (to remove in 019)
        }
        dataRows = append(dataRows, stringRow)
    }
    return ui.DataSet{Headers: headers, Rows: dataRows}, nil
}
```

**Key insight:** `pgx.Rows.Values()` returns `[]any` with native Go types (`int64`, `float64`, `bool`, `string`). The native types are available — they're just discarded at this boundary.

### 1.3 `FormatValue` (`pkg/dataaccess/format.go`)

```go
func FormatValue(val any) string {
    if val == nil { return "" }
    if valuer, ok := val.(driver.Valuer); ok {
        v, err := valuer.Value()
        if err == nil && v != nil {
            switch ts := v.(type) {
            case []byte: return string(ts)
            default: return fmt.Sprintf("%v", v)
            }
        }
    }
    return fmt.Sprintf("%v", val)
}
```

**Per specs:** Preserve this function's signature and internals intact. It moves from fetch-time to render-time only.

---

## 2. Current `dataGridModel` and Grid Data Flow

### 2.1 Structure (`compositor.go`)

```go
type dataGridModel struct {
    mu            sync.RWMutex
    headers       []string
    columns       []string
    rows          [][]string    // ← TARGET: [][]any
    masterHeaders []string
    masterRows    [][]string    // ← TARGET: [][]any
    filterKeys    []string
    cancel        context.CancelFunc
    unsubscribe   func()
    wg            sync.WaitGroup
}
```

### 2.2 Data Flow

| Step | Function | What It Does |
|------|----------|--------------|
| 1 | `fetchGridDataAsync` | `DS.Fetch()` → stores `ds.Rows` into `model.rows` |
| 2 | `loadMasterBuffer` | `DS.FetchAll()` → stores into `model.masterHeaders`/`model.masterRows` |
| 3 | `UpdateCell` closure | `label.SetText(row[id.Col])` — currently `string`, passed directly |
| 4 | `filterMasterRows` | iterates `masterRows` as `[][]string`, does `containsIgnoreCase` |
| 5 | `OnSelected` | builds `map[string]any` from `headers[i] → row[i]` — currently stores strings |

### 2.3 Render-Time Formatting Requirement (Spec 019)

After migration to `[][]any`:
- **`UpdateCell`**: `label.SetText(row[id.Col])` → `label.SetText(dataaccess.FormatValue(row[id.Col]))`
- **`filterMasterRows`**: `cellVal := row[col]` → `cellVal := dataaccess.FormatValue(row[col])` for substring comparison
- **`OnSelected`**: **no change needed** — `rowMap[headers[i]] = row[i]` will naturally carry native `any` types

---

## 3. Current Button Composition Flow

### 3.1 `"button"` Case (`compositor.go`)

```go
case "button":
    if strings.HasPrefix(node.SubmitAction, "navigate:") && Navigate != nil {
        targetVista := strings.TrimPrefix(node.SubmitAction, "navigate:")
        return widget.NewButton(node.Label, func() {
            Navigate(targetVista)
        }), func() {}, nil
    }
    if node.SubmitAction != "" && LocalEventBus != nil {
        return widget.NewButton(node.Label, func() {
            LocalEventBus.Publish(state.SubmitChannel(), state.Snapshot())
        }), func() {}, nil
    }
    return widget.NewButton(node.Label, func() {}), func() {}, nil
```

### 3.2 What Spec 018 Must Add

| Feature | Current State | Required State |
|---------|--------------|----------------|
| Reactive enable/disable | Always enabled | Starts disabled; subscribes to `"publish_selection"`; enables on valid payload |
| `param_mapping` | Not in `NodeMeta` | `map[string]string` field: dest param key → dot-notation source path |
| Query string navigation | `Navigate(targetVista)` sends plain string | Button resolves `param_mapping`, builds `vistaID?key=val&...` |
| EventBus cleanup | Returns `func() {}` | Returns cleanup that unsubscribes from selection channel |

---

## 4. Current `ui.Navigate` Callback

Implementation in `cmd/golemui/main.go`:
- Tears down previous screen, loads new layout via `LoadScreen`, composes via `Compose`, swaps container in `fyne.Do`.
- Currently receives plain `vistaID` — no query string parsing.

Spec 018 adds: parse `"detalle?id=99&tipo=debito"` → inject params into destination `ScreenState`.

---

## 5. Current `ScreenState`

```go
type ScreenState struct {
    mu            sync.RWMutex
    data          map[string]any
    submitChannel string
}
```

`Compose(node, vistaID)` calls `NewScreenState(vistaID)` internally — params must be injected between construction and child composition. Options: new constructor with params, `Preload` method, or variadic `Compose` signature.

---

## 6. Event Bus Mechanism

```go
type EventBus interface {
    Publish(channel string, payload interface{})
    Subscribe(channel string, h Handler) string
    Unsubscribe(channel string, subID string)
}
```

`InMemEventBus.Publish` dispatches handlers in fresh goroutines. All widget mutations **must** use `fyne.Do()`.

Active channels: `"screen:submit:<vistaID>"`, `"publish_selection"`.

---

## 7. Existing Test Inventory

### compositor_test.go (~2450 lines)
- All data_grid tests use `Rows: [][]string{...}` — must migrate to `[][]any`
- Selection test asserts `map[string]any{"id": "42"}` — must expect native types after 019
- Button navigation test asserts `Navigate("query_runner")` — must handle query strings for 018
- Zero tests for reactive button, param_mapping, query string parsing, ScreenState preload

### sql_datasource_test.go (~210 lines)
- 3-4 assertions use string comparisons — must expect native types
- Has compile-time `var _ string = cell` check — must be removed

---

## 8. Dependency Map

```
Spec 019 (Type Preservation)           Spec 018 (Button State Nav)
─────────────────────────              ─────────────────────────
✦ DataSet.Rows: [][]string → [][]any    ← DEPENDS ON 019
✦ dataGridModel: [][]string → [][]any   (needs native types
✦ Fetch: remove FormatValue              in publish_selection
✦ UpdateCell: add FormatValue            payload for
✦ filterMasterRows: add FormatValue       param_mapping
✦ Test fixtures: [][]string → [][]any     resolution)
                                      ✦ NodeMeta: add ParamMapping
                                      ✦ Button: reactive enable/disable
                                      ✦ Button: param_mapping → query string
                                      ✦ Navigate: parse query string
                                      ✦ ScreenState: preload params
```

---

## 9. Risks

1. **`pgtype.Numeric`** (medium): pgx returns `pgtype.Numeric` for NUMERIC columns. Consider unwrapping to `float64` at `Fetch` boundary.
2. **Thread safety** (low): Button subscriber must wrap `Enable()`/`Disable()` in `fyne.Do()`.
3. **Test file size** (low): compositor_test.go already ~2450 lines. Consider separate `button_test.go` for 018 tests.

---

## 10. Existing Patterns to Reuse

| Pattern | Location | Reuse For |
|---------|----------|-----------|
| `parseChannelName(dataSource)` | compositor.go | Button selection channel resolution |
| `resolvePath(data, path)` | compositor.go | param_mapping dot-notation resolution |
| `fyne.Do(func(){...})` wrapper | label subscriber | Button Enable/Disable |
| `sync.Once` cleanup pattern | label, data_grid | Button subscription cleanup |
| `LocalEventBus.Subscribe` + cleanup | label, data_grid | Button selection subscription |
