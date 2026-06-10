# Delta Spec: grid-selection-button-nav

**Change ID:** `grid-selection-button-nav`
**Date:** 2026-06-09
**Specs:** 019 (Datagrid Native Type Preservation), 018 (Action Button State Navigation)

---

## ADDED Requirements

### REQ-019-01: Native Type Preservation in Data Pipeline

The system MUST preserve native Go types (`int64`, `float64`, `bool`, `string`) throughout the datagrid data pipeline from fetch to selection, deferring string conversion to render-time only.

#### Scenario: Native types preserved in selection event

- GIVEN a datagrid with native data types in its rows
- WHEN a user selects a row containing numeric, boolean, and string values
- THEN the `OnSelected` callback MUST receive a `map[string]any` with original types preserved: `{"id": int64(42), "active": bool(true), "name": "record42"}`
- AND the event bus MUST publish these native types to the `"publish_selection"` channel

#### Scenario: SQLDataSource fetch returns native types

- GIVEN `SQLDataSource.Fetch()` is called on a query returning mixed data types
- WHEN the database returns values of types `int64`, `float64`, `bool`, and `string`
- THEN the returned `DataSet.Rows` MUST contain `[][]any` with original types preserved
- AND no early `FormatValue` conversion SHALL be applied during fetch

---

### REQ-019-02: Deferred String Formatting at Render Time

The system MUST convert native cell values to strings only during visual rendering using `dataaccess.FormatValue`, preserving native types for all internal operations.

#### Scenario: UpdateCell applies FormatValue during render

- GIVEN a datagrid cell with native `float64` value `123.45`
- WHEN the `UpdateCell` callback is executed for that cell
- THEN `label.SetText()` MUST receive `dataaccess.FormatValue(123.45)` which returns `"123.45"`
- AND the native `float64` type MUST remain preserved in `dataGridModel.rows`

#### Scenario: FilterMasterRows applies FormatValue for comparison

- GIVEN a datagrid with native data in `masterRows`
- WHEN `filterMasterRows` executes a substring search
- THEN cell comparison MUST use `dataaccess.FormatValue(row[col])` for substring matching
- AND the original native types MUST remain unmodified in `masterRows`

---

### REQ-018-01: Button Reactive Enable/Disable Based on Selection

The system MUST support buttons that start disabled and enable/disable based on grid selection events, with all UI state changes dispatched via `fyne.Do()`.

#### Scenario: Button initial state is disabled

- GIVEN a button node with `data_source` configured for selection subscription
- WHEN the button is composed and displayed
- THEN the button MUST be in a disabled state initially via `button.Disable()`
- AND no navigation SHALL occur when clicked while disabled

#### Scenario: Button enables on valid selection

- GIVEN a disabled button subscribed to `"publish_selection"`
- WHEN a valid selection payload is published to `"publish_selection"`
- THEN the button MUST transition to enabled state via `fyne.Do(func() { button.Enable() })`
- AND subsequent clicks MUST trigger navigation behavior

#### Scenario: Button disables on deselection

- GIVEN an enabled button with active subscription
- WHEN a deselection event (empty payload or no selection) is published
- THEN the button MUST transition to disabled state via `fyne.Do(func() { button.Disable() })`
- AND the button SHALL remain disabled until valid selection is received

---

### REQ-018-02: Param Mapping with Dot-Notation Resolution

The system MUST support dynamic parameter mapping from grid selection to navigation query strings using dot-notation path resolution.

#### Scenario: Button constructs query string from param_mapping

- GIVEN a button node with `param_mapping` set to `{"id": "row.id", "type": "row.transaction_type"}`
- AND a selected row with `{"id": 123, "transaction_type": "debit"}`
- WHEN the button is clicked
- THEN the navigation MUST build query string `"vista_destino?id=123&type=debit"`
- AND `ui.Navigate()` MUST be called with the full query string

#### Scenario: ResolvePath extracts nested values

- GIVEN a selection payload with nested structure `{"user": {"profile": {"id": 456}}}`
- AND a param mapping with path `"user.profile.id"`
- WHEN resolvePath is called on the payload with the path
- THEN the function MUST return `456` as a string value
- AND invalid paths MUST return empty string without panicking

---

### REQ-018-03: Navigate Query String Parsing

The system MUST extend `ui.Navigate` to parse query strings while preserving the original signature and extracting parameters for screen state injection.

#### Scenario: Navigate parses query string components

- GIVEN `ui.Navigate("detalle?id=99&tipo=debito")` is called
- THEN the system MUST extract vistaID `"detalle"` and query parameters `{"id": "99", "tipo": "debito"}`
- AND the layout for `"detalle"` MUST be loaded using the clean vistaID

#### Scenario: Navigate handles empty query string

- GIVEN `ui.Navigate("simple_vista")` is called without query parameters
- THEN the system MUST treat it as equivalent to `"simple_vista?"`
- AND no parameters SHALL be injected into the destination screen state

---

### REQ-018-04: ScreenState Preload for Parameter Injection

The system MUST support parameter injection into destination `ScreenState` before widget composition via a `Preload` method.

#### Scenario: Preload injects query parameters

- GIVEN `ui.Navigate("detalle?id=99")` is called
- AND a new `ScreenState` is created for `"detalle"`
- WHEN `state.Preload(map[string]any{"id": "99"})` is called
- THEN subsequent calls to `state.Get("id")` MUST return `"99"`
- AND the value MUST be available during child widget composition

#### Scenario: Preload merges with existing state

- GIVEN a `ScreenState` already contains `{"existing": "value"}`
- WHEN `state.Preload(map[string]any{"new": "param"})` is called
- THEN the state MUST contain both `{"existing": "value", "new": "param"}`
- AND existing values MUST NOT be overwritten by preload

---

## Cross-Cutting Requirements

### XREQ-01: Thread Safety for UI Updates

All UI state changes (button enable/disable, screen composition) MUST be dispatched to the Fyne UI thread using `fyne.Do()`.

#### Scenario: Button state change in goroutine

- GIVEN a selection event is published in a background goroutine
- WHEN the button enable/disable callback is executed
- THEN `fyne.Do(func() { button.Enable() })` MUST be used
- AND the UI update MUST be thread-safe

### XREQ-02: Backward Compatibility

The system MUST preserve existing datagrid and button behavior while adding new features.

#### Scenario: Existing datagrid and button functionality preserved

- GIVEN a datagrid or button without new features configured
- WHEN it operates with existing patterns
- THEN it MUST behave identically to the original system

### XREQ-03: Test Coverage

All new functionality MUST have test coverage with happy path and edge cases.

#### Scenario: MockDataSource migration to [][]any

- GIVEN test fixtures use `[][]any` instead of `[][]string`
- WHEN tests run with native type data
- THEN assertions MUST validate both type preservation and correct string rendering

---

## Acceptance Criteria Mapping

| Scenario | Spec 019 AC | Spec 018 AC |
|----------|-------------|-------------|
| Native types preserved in selection event | ✅ Validates `int64`, `bool`, `float64` in payload | ✅ Supports parameter mapping |
| UpdateCell applies FormatValue | ✅ Visual parity | ✅ Proper formatting for display |
| Button initial disabled state | — | ✅ Disabled on compose |
| Button enables on valid selection | — | ✅ Enables via fyne.Do |
| Param mapping constructs query string | — | ✅ Query string built correctly |
| Navigate parses query string | — | ✅ vistaID + params extracted |
| ScreenState Preload injection | — | ✅ Params available via Get() |
| Thread safety with fyne.Do | ✅ UI thread safety | ✅ Reactive button safety |

---

## Related Artifacts

- **Proposal**: `openspec/changes/grid-selection-button-nav/proposal.md`
- **Explore Report**: `openspec/changes/grid-selection-button-nav/explore-report.md`
- **Spec 018**: `docs/specify/018-action-button-state-navigation.md`
- **Spec 019**: `docs/specify/019-datagrid-native-type-preservation.md`

## Delivery Strategy

- **PR-1**: Spec 019 — Type migration, deferred formatting, native selection
- **PR-2**: Spec 018 — Reactive buttons, param mapping, query strings, ScreenState preload (depends on PR-1)
