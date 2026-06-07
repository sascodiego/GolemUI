# Sidebar Tree Widget Specification

## Purpose

Transform a flat `[]MenuItem` slice (loaded via `LoadNavigationMenu`) into an interactive Fyne `widget.Tree` sidebar. Leaf nodes with a non-empty `VistaID` trigger the package-level `Navigate` function on selection. This is a **new domain** — no canonical sidebar-widget spec exists.

## Requirements

### Requirement: BuildNavTree Function Signature

The `pkg/ui` package MUST export a function with the following signature:

```go
func BuildNavTree(items []MenuItem) *widget.Tree
```

The function SHALL live in a new file `pkg/ui/sidebar_widget.go` with package declaration `package ui`.

#### Scenario: Function exists and is callable

- GIVEN the `pkg/ui` package is imported
- WHEN `BuildNavTree` is called with any `[]MenuItem` (including empty)
- THEN it MUST return a non-nil `*widget.Tree`

### Requirement: Parent-to-Children Index Construction

`BuildNavTree` MUST build an internal `parentToChildren map[string][]string` where:
- The key `""` (empty string) maps to the IDs of root nodes (items where `PadreID == ""`)
- Every other key is a `PadreID` mapping to the IDs of its children
- Children MUST be sorted ascending by the `Orden` field of each `MenuItem`, with `ID` as a stable tiebreaker

`BuildNavTree` MUST also build an internal `idToItem map[string]MenuItem` mapping each item's `ID` to its full `MenuItem` value.

#### Scenario: Root nodes indexed under empty-string key

- GIVEN `items = [{ID:"a", PadreID:"", Titulo:"Root A", Orden:1}, {ID:"b", PadreID:"", Titulo:"Root B", Orden:2}]`
- WHEN `BuildNavTree(items)` is called
- THEN the tree's `ChildUIDs("")` callback MUST return `["a", "b"]`

#### Scenario: Children sorted by Orden then ID

- GIVEN `items = [{ID:"c", PadreID:"a", Orden:2}, {ID:"d", PadreID:"a", Orden:1}]`
- WHEN `BuildNavTree(items)` is called
- THEN the tree's `ChildUIDs("a")` callback MUST return `["d", "c"]` (Orden 1 before 2)

### Requirement: Tree Callbacks

`BuildNavTree` MUST create the tree via `widget.NewTree` with four callbacks:

1. **ChildUIDs(uid widget.TreeNodeID) []widget.TreeNodeID**: Returns the children slice from `parentToChildren[uid]`. For leaf nodes not present in the map, MUST return an empty (non-nil) slice.

2. **IsBranch(uid widget.TreeNodeID) bool**: Returns `true` if `uid` exists as a key in `parentToChildren` (i.e., the node has at least one child). Returns `false` otherwise.

3. **CreateNode(isBranch bool) fyne.CanvasObject**: Returns `widget.NewLabel("")` regardless of branch/leaf status. Both branch and leaf nodes use the same widget type.

4. **UpdateNode(uid widget.TreeNodeID, isBranch bool, node fyne.CanvasObject)**: Sets the label's text to `idToItem[uid].Titulo`. MUST cast `node` to `*widget.Label` and set `.SetText(item.Titulo)`.

#### Scenario: Leaf node renders with correct title

- GIVEN `items = [{ID:"leaf1", PadreID:"", Titulo:"Dashboard", VistaID:"home", Orden:1}]`
- WHEN the tree renders node `"leaf1"`
- THEN the label widget for that node MUST display `"Dashboard"`

#### Scenario: Branch node renders with correct title

- GIVEN `items = [{ID:"folder", PadreID:"", Titulo:"Admin", VistaID:"", Orden:1}, {ID:"child", PadreID:"folder", Titulo:"Users", VistaID:"users", Orden:1}]`
- WHEN the tree renders node `"folder"`
- THEN the label widget for that node MUST display `"Admin"`

#### Scenario: Non-existent node in ChildUIDs returns empty slice

- GIVEN `items = [{ID:"leaf1", PadreID:"", Titulo:"L1", VistaID:"v1", Orden:1}]`
- WHEN `ChildUIDs("leaf1")` is called
- THEN the result MUST be an empty (non-nil) `[]widget.TreeNodeID`

### Requirement: Navigation on Leaf Selection

The tree's `OnSelected` callback MUST implement the following logic:

1. Look up the selected UID in `idToItem`.
2. Check if the node is a leaf: `!IsBranch(uid)`.
3. Check if the node has a non-empty `VistaID`.
4. Check if `Navigate != nil`.
5. Only when **all three** conditions are true, call `Navigate(item.VistaID)`.

The callback MUST NOT call `Navigate` for branch nodes, for nodes with empty `VistaID`, or when `Navigate` is nil.

#### Scenario: Selecting leaf with VistaID triggers Navigate

- GIVEN `items = [{ID:"leaf1", PadreID:"", Titulo:"Dashboard", VistaID:"home", Orden:1}]`
- AND `Navigate` is set to `func(id string) { record = id }`
- WHEN the tree selects node `"leaf1"`
- THEN `Navigate` MUST be called with `"home"`

#### Scenario: Selecting branch does NOT trigger Navigate

- GIVEN `items = [{ID:"folder", PadreID:"", Titulo:"Admin", VistaID:"admin", Orden:1}, {ID:"child", PadreID:"folder", Titulo:"Users", VistaID:"users", Orden:1}]`
- AND `Navigate` is set to `func(id string) { record = id }`
- WHEN the tree selects node `"folder"`
- THEN `Navigate` MUST NOT be called (because `"folder"` is a branch)

#### Scenario: Selecting leaf without VistaID does NOT trigger Navigate

- GIVEN `items = [{ID:"empty", PadreID:"", Titulo:"Spacer", VistaID:"", Orden:1}]`
- AND `Navigate` is set to `func(id string) { record = id }`
- WHEN the tree selects node `"empty"`
- THEN `Navigate` MUST NOT be called (because `VistaID` is empty)

#### Scenario: Navigate is nil does NOT panic

- GIVEN `items = [{ID:"leaf1", PadreID:"", Titulo:"Dashboard", VistaID:"home", Orden:1}]`
- AND `Navigate` is nil
- WHEN the tree selects node `"leaf1"`
- THEN no panic MUST occur

### Requirement: Empty Items Handling

When `BuildNavTree` is called with an empty `[]MenuItem` slice or `nil`, it MUST return a valid non-nil `*widget.Tree` with:
- `ChildUIDs("")` returning an empty slice
- `IsBranch` returning `false` for any UID
- A functional `OnSelected` that is a no-op (since no items exist)

#### Scenario: Empty slice returns valid tree

- GIVEN `items = []MenuItem{}`
- WHEN `BuildNavTree(items)` is called
- THEN the returned tree MUST be non-nil
- AND `ChildUIDs("")` MUST return an empty slice

#### Scenario: Nil slice returns valid tree

- GIVEN `items = nil` (nil `[]MenuItem`)
- WHEN `BuildNavTree(items)` is called
- THEN the returned tree MUST be non-nil

### Requirement: Test File Coverage

A new test file `pkg/ui/sidebar_widget_test.go` with package `ui_test` (external test package) MUST exist and cover the following cases:

1. **TestBuildNavTree_PopulatesCorrectTitles**: Build a tree from a multi-level `[]MenuItem`. Verify that labels for all nodes display the correct `Titulo` values.
2. **TestBuildNavTree_LeafTriggersNavigate**: Select a leaf node with a non-empty `VistaID`. Assert `Navigate` was called with the correct `VistaID`.
3. **TestBuildNavTree_BranchDoesNotTriggerNavigate**: Select a branch node. Assert `Navigate` was NOT called.
4. **TestBuildNavTree_LeafWithoutVistaIDDoesNotNavigate**: Select a leaf node with empty `VistaID`. Assert `Navigate` was NOT called.
5. **TestBuildNavTree_EmptyItems**: Call `BuildNavTree([]MenuItem{})`. Assert the tree is non-nil.

All tests MUST use `test.NewApp()` in `TestMain` for Fyne initialization. Tests that set `Navigate` MUST restore it to its previous value via `defer`.

#### Scenario: All five test functions exist and pass

- GIVEN the test file `pkg/ui/sidebar_widget_test.go`
- WHEN `go test ./pkg/ui/... -run TestBuildNavTree` is executed
- THEN all five test functions MUST pass without errors or panics

## Constraints

| ID | Constraint | Rationale |
|----|-----------|-----------|
| C-1 | New file: `pkg/ui/sidebar_widget.go` only | No modification to existing files |
| C-2 | New file: `pkg/ui/sidebar_widget_test.go` only | No modification to existing files |
| C-3 | Fyne imports allowed (`fyne.io/fyne/v2`, `fyne.io/fyne/v2/widget`) | This is Capa 4 (Renderizador Fyne) |
| C-4 | Package `ui` for production code, `ui_test` for tests | Standard Go convention for external tests |
| C-5 | No new exported types or global variables | Only `BuildNavTree` function is the public API |
| C-6 | `MenuItem` struct is consumed as-is from `sidebar_loader.go` | No duplication or redefinition |

## Acceptance Criteria Summary

| AC | Criterion | Verification |
|----|-----------|-------------|
| AC-1 | `BuildNavTree(items)` returns non-nil `*widget.Tree` for all inputs (including empty/nil) | Unit test |
| AC-2 | Tree nodes display correct `Titulo` from `idToItem` map | Unit test |
| AC-3 | Selecting a leaf with `VistaID != ""` calls `Navigate(VistaID)` | Unit test |
| AC-4 | Selecting a branch does NOT call `Navigate` | Unit test |
| AC-5 | Selecting a leaf with `VistaID == ""` does NOT call `Navigate` | Unit test |
| AC-6 | `Navigate == nil` does not cause a panic on selection | Unit test |
| AC-7 | Children sorted by `Orden` ascending, `ID` as tiebreaker | Unit test |
| AC-8 | No existing files modified | `git diff --name-only` |
| AC-9 | `go test ./pkg/ui/...` passes with zero failures | CI |
| AC-10 | `go vet ./pkg/ui/...` reports no issues | CI |
