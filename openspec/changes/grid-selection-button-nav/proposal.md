# Proposal: grid-selection-button-nav

**Change ID:** `grid-selection-button-nav`
**Date:** 2026-06-09
**Status:** Draft
**Specs:** 019 (Datagrid Native Type Preservation), 018 (Action Button State Navigation)
**Delivery Strategy:** Two chained PRs — PR-1 for Spec 019, PR-2 for Spec 018

---

## 1. Intent

GolemUI's datagrid transport layer degrades native database types (`int64`, `float64`, `bool`) to strings at fetch time via `FormatValue`, losing type fidelity before the data reaches the client. Meanwhile, buttons cannot react to grid selection state — they are always enabled — and navigation is static, with no mechanism for passing dynamic parameters from the selected row to the destination screen.

This combined change restores native type preservation through the entire data pipeline and builds reactive button behavior on top of it, enabling dynamic parameter-driven navigation between screens.

---

## 2. Business Problem

| Pain Point | Current State | Impact |
|---|---|---|
| **Type loss at fetch** | `SQLDataSource.Fetch` converts all cell values to `string` via `FormatValue` immediately | Selection events carry `"42"` instead of `int64(42)`, preventing numeric comparisons, type-safe parameter passing, and future logical expressions |
| **Buttons always enabled** | No subscription to grid selection events | Users can click navigation buttons with no row selected, leading to blank or error screens |
| **Static navigation** | `Navigate(vistaID)` receives a plain screen ID | No way to pass the selected transaction's ID to a detail screen; all screens receive identical context |
| **Operational cost** | Workarounds require custom Lua scripts or hard-coded navigation per screen | Every new master-detail flow requires developer intervention instead of declarative configuration |

---

## 3. Proposed Solution

### Phase 1 — PR-1 (Spec 019): Datagrid Native Type Preservation

**Goal:** Migrate the transport layer from `[][]string` to `[][]any`, deferring `FormatValue` to render-time only, so selection events carry native Go types.

#### 3.1.1 DataSet.Rows Migration
- Change `DataSet.Rows` in `pkg/ui/datasource.go` from `[][]string` to `[][]any`.
- Update `MockDataSource` fixtures and all test data to use `[][]any`.

#### 3.1.2 SQLDataSource.Fetch — Remove Early Conversion
- In `pkg/dataaccess/sql_datasource.go`, stop calling `FormatValue` in `Fetch`/`FetchAll`.
- Store `rows.Values()` (`[]any`) directly into `DataSet.Rows`.
- Handle `pgtype.Numeric` by unwrapping to `float64` at the fetch boundary for safety.

#### 3.1.3 dataGridModel Migration
- Change `rows` and `masterRows` fields in `dataGridModel` (`pkg/ui/compositor.go`) from `[][]string` to `[][]any`.

#### 3.1.4 Render-Time Formatting
- `UpdateCell` callback: wrap cell value with `dataaccess.FormatValue(row[id.Col])` before `label.SetText(...)`.
- `filterMasterRows`: wrap cell value with `dataaccess.FormatValue(row[col])` for substring comparison.
- All visual mutations remain dispatched via `fyne.Do()`.

#### 3.1.5 Selection Event — Native Types
- `OnSelected` callback: `rowMap[headers[i]] = row[i]` naturally carries native `any` types.

#### 3.1.6 FormatValue — Untouched
- `pkg/dataaccess/format.go`: signature and internals preserved.

### Phase 2 — PR-2 (Spec 018): Action Button State Navigation

**Depends on PR-1.** Native types from `publish_selection` feed into `param_mapping` resolution.

#### 3.2.1 Reactive Button Enable/Disable
- Button starts disabled. Subscribes to `"publish_selection"`. Enables on valid payload, disables on deselection. Cleanup unsubscribes.

#### 3.2.2 Param Mapping (Dot-Notation)
- Add `ParamMapping` field to `NodeMeta` as `map[string]string` (dest param key → dot-notation source path).
- On click: evaluate paths against selection via `resolvePath`. Build query string.

#### 3.2.3 Navigate — Query String Parsing
- Preserve signature `func(vistaID string)`. Parse internally. Load layout with clean `vistaID`. Inject params into `ScreenState`.

#### 3.2.4 ScreenState Parameter Injection
- Add `Preload(map[string]any)` method to `ScreenState`. Called before composition.

---

## 4. Success Metrics

### Spec 019
1. Selection payload preserves native types (`int64`, `bool`, `float64`).
2. Visual parity: table cells render identically to current system.

### Spec 018
3. Button transitions from Disabled to Enabled on valid selection via `fyne.Do`.
4. `Navigate("detalle?id=99&tipo=debito")` loads screen `"detalle"`.
5. `ScreenState` for `"detalle"` has `"id"` → `"99"` and `"tipo"` → `"debito"`.

---

## 5. Affected Areas

| Layer | Files | PR |
|-------|-------|----|
| Capa 4 | `pkg/ui/datasource.go` | PR-1 |
| Capa 4 | `pkg/dataaccess/sql_datasource.go` | PR-1 |
| Capa 4 | `pkg/ui/compositor.go` | PR-1 + PR-2 |
| Capa 4 | `pkg/ui/screen_state.go` | PR-2 |
| Capa 4 | `cmd/golemui/main.go` | PR-2 |
| Tests | `pkg/ui/compositor_test.go` | PR-1 + PR-2 |
| Tests | `pkg/dataaccess/sql_datasource_test.go` | PR-1 |
| Tests | `pkg/ui/screen_state_test.go` | PR-2 |

---

## 6. Risks and Mitigations

| Risk | Severity | Mitigation |
|------|----------|------------|
| `pgtype.Numeric` from NUMERIC columns | Medium | Unwrap to `float64` at fetch boundary |
| Thread safety — button state from EventBus goroutines | Low | All Enable/Disable in `fyne.Do()` |
| `compositor_test.go` already ~2450 lines | Low | Separate `button_test.go` for Spec 018 |
| Query string special characters | Low | URL-encode values; sanitize on parse |
| Breaking change in `DataSet.Rows` type | Medium | `DataSet` is internal; all consumers updated in same PR |

---

## 7. Rollback Plan

Go client-only change. No DB schema changes. `git revert` per PR independently.

---

## 8. Non-Goals

- ❌ No DB schema changes.
- ❌ No plugin interface changes.
- ❌ No new GolemUI catalog components.
- ❌ No `FormatValue` changes.
- ❌ No multi-select support.
- ❌ No navigation history/back-navigation.

---

## 9. Delivery Plan

| PR | Spec | Depends On | Description |
|----|------|------------|-------------|
| PR-1 | 019 | None | Type preservation: `[][]any` migration, deferred formatting, native selection |
| PR-2 | 018 | PR-1 | Reactive buttons, param_mapping, Navigate query strings, ScreenState preload |
