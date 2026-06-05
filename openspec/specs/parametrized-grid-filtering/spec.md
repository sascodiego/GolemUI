# Specification: Parametrized Grid Filtering (parametrized-grid-filtering)

## Introduction
The `parametrized-grid-filtering` capability enables dynamic, reactive filtering of data grids (`widget.Table`) in GolemUI. By subscribing to event channels via the event bus, a data grid listens to incoming input updates, cancels any outstanding database requests to prevent out-of-order rendering (race conditions), executes parametrized queries using native placeholders (e.g., `$1`), and safely updates the UI on the main thread.

## Requirements
1. The GolemUI compositor MUST subscribe a `"data_grid"` component to the event channel specified by its `bind_to` metadata if it is not empty.
2. When a filtering event payload is received, the data grid's query executor MUST initiate an asynchronous data retrieval process.
3. The query executor MUST cancel any active/stale query contexts (e.g. via `context.WithCancel`) for that data grid before executing the new database query.
4. The database query MUST be executed with native SQL parameter placeholders (`$1`) using the received filtering payload as the parameter.
5. The query execution MUST run in a separate background goroutine to avoid blocking the main UI thread.
6. Once the query completes, the data grid MUST acquire a write lock on its data cache mutex before updating the internal string matrix.
7. The table refresh notification MUST be dispatched to the main UI thread via Fyne's thread-safe scheduler (`fyne.Do`).

## Scenarios

### Scenario 1: Reactive Filter Execution and UI Refresh
*   **GIVEN** a data grid is subscribed to `"filter:title"` and has a query context active
*   **WHEN** an event payload `"Golang"` is published to `"filter:title"`
*   **THEN** the grid MUST cancel any current database query execution context
*   **AND** it MUST run the SQL query with `"Golang"` passed as parameter `$1` in a background thread
*   **AND** it MUST update the table data thread-safely and call `Refresh()` on the UI thread.

### Scenario 2: Stale Query Cancellation on Rapid Input
*   **GIVEN** a data grid subscribed to `"filter:title"`
*   **WHEN** the user rapidly publishes `"Go"`, then `"Gole"`, then `"GolemUI"` to the channel
*   **THEN** the grid MUST cancel the query execution contexts for `"Go"` and `"Gole"`
*   **AND** it MUST only complete and render the result corresponding to the final `"GolemUI"` payload.

### Scenario 3: Graceful Handling of Empty Event Payload
*   **GIVEN** a data grid subscribed to `"filter:title"`
*   **WHEN** an empty string `""` is published to the channel
*   **THEN** the grid MUST execute the SQL query with the empty string parameter safely without panicking
*   **AND** the UI table widget SHALL be refreshed with the resulting records.
