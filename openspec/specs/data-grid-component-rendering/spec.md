# Specification: Data Grid Component Rendering (data-grid-component-rendering)

## Introduction
The `data-grid-component-rendering` capability specifies how the GolemUI compositor parses `"data_grid"` nodes and renders them into Fyne's virtualized `*widget.Table`. It ensures templates for headers and cells are correctly created, and that row/column metrics are derived dynamically to fit the target data structure.

## Requirements
1. The GolemUI compositor MUST recursively parse NodeMeta structures and render elements of type `"data_grid"` into a Fyne `*widget.Table` object.
2. The renderer MUST define templates for both header cells (column names) and body cells (row values).
3. The column count and row count metrics MUST be derived dynamically from the active data grid model to allocate table dimensions.
4. Cell renderers MUST use the derived row index and column index to fetch and map data from the local grid model.
5. If the layout containing the `"data_grid"` is composed recursively, the resulting widget MUST respect parent size and layout constraints.
6. The widget template creation SHALL occur before any asynchronous data queries begin.

## Scenarios

### Scenario 1: Recursive Layout Integration and Table Initialization
*   **GIVEN** a NodeMeta tree containing a `"data_grid"` element nested inside a vertical layout container
*   **WHEN** `Compose` is recursively executed on the root node
*   **THEN** the compositor MUST instantiate a Fyne `*widget.Table`
*   **AND** the table object MUST be successfully appended as a child of the parent vertical layout container.

### Scenario 2: Dynamic Dimension and Metric Derivation
*   **GIVEN** a data grid component linked to an active model with `C` columns and `R` rows
*   **WHEN** the Fyne table queries its dimensions
*   **THEN** the table widget MUST return `C` for the columns count
*   **AND** it MUST return `R` for the rows count
*   **AND** it SHALL dynamically adjust its layout metrics when the underlying model dimensions change.

### Scenario 3: Cell and Header Template Rendering
*   **GIVEN** an initialized table widget and templates for cell elements
*   **WHEN** the Fyne virtualized table requests cells to draw
*   **THEN** the table component MUST invoke the cell template renderer to instantiate new cell widgets
*   **AND** it SHALL populate each cell with the corresponding data value from the row and column index in the model.
