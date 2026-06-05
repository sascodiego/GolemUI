# Specification: Composite Layout Engine (composite-layout-engine)

## Introduction
The `composite-layout-engine` is GolemUI's recursive layout rendering component. It runs on the client (Capa 4) using Fyne and is responsible for parsing a dynamic nested JSON schema (`composite_screen`), which represents the visual hierarchy, layout rules (such as rows, columns, fractional widths, or grids), and instantiating equivalent Fyne layout containers and widgets.

## Requirements
1. The engine MUST parse a nested JSON schema mapping (`composite_screen`) containing screen node definitions.
2. The engine MUST support recursive layout trees, where node containers can contain other nested node containers.
3. The engine MUST parse and support layout properties:
   - Grid layout configuration with row/column metric specifications (e.g. `"2fr, 1fr"`).
   - Flex layouts (horizontal and vertical).
   - Tab containers.
4. The engine MUST map leaf nodes to their corresponding logical UI widgets (e.g., text inputs, buttons, tables).
5. If a node is malformed or invalid in JSON, the engine MUST fail gracefully, fall back to a placeholder component, and log the syntax or validation warning.
6. The engine MUST construct the layout dynamically at runtime without requiring client recompilation.

## Scenarios

### Scenario 1: Successful Recursive Tree Layout Rendering
*   **GIVEN** a valid `composite_screen` JSON schema representing a vertical box container containing a text field and a nested horizontal grid with two columns (`"1fr, 1fr"`)
*   **WHEN** the layout engine is requested to compile and render the schema
*   **THEN** the engine MUST recursively parse the container nodes
*   **AND** the engine SHALL instantiate a vertical layout container as the root
*   **AND** the engine SHALL nest the text field widget and a two-column fractional layout container as children within the root container
*   **AND** the layout engine SHALL successfully display the resulting Fyne window with the correct spatial hierarchy.

### Scenario 2: Error Handling for Malformed Layout Nodes
*   **GIVEN** a `composite_screen` JSON payload containing a node with an unrecognized component type
*   **WHEN** the layout engine attempts to parse and render the tree
*   **THEN** the engine MUST log a validation warning detailing the unrecognized type
*   **AND** the engine SHALL render a visible fallback error label or placeholder component at the position of the malformed node
*   **AND** the remaining valid sections of the layout tree MUST still be compiled and displayed.

### Scenario 3: Custom Fractional Grid Metric Evaluation
*   **GIVEN** a grid container node specifying a columns layout definition of `"3fr, 1fr"`
*   **WHEN** the engine renders the screen container on a canvas of width `W`
*   **THEN** the engine MUST allocate 75% of the available width `W` (excluding layout gaps) to the first column canvas object
*   **AND** the engine MUST allocate 25% of the available width `W` to the second column canvas object.
