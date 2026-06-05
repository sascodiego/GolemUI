# Specification: Asynchronous Data Source Querying (asynchronous-data-source-querying)

## Introduction
The `asynchronous-data-source-querying` capability specifies how GolemUI performs data loading for the `"data_grid"` component. The system must execute database queries on background threads to prevent UI lockups, formats the output into a thread-safe string matrix cached locally, and uses Fyne's scheduling mechanism to notify the main thread when rendering updates are ready.

## Requirements
1. The GolemUI compositor MUST execute SQL queries specified in a `"data_grid"`'s `data_source` parameter asynchronously using the business database connection pool `ui.BusinessPool`.
2. The asynchronous execution MUST run in a background goroutine separate from the main Fyne event dispatching thread.
3. The query result rows MUST be formatted into a dynamic two-dimensional string matrix (`[][]string`).
4. Access to the localized string matrix cache MUST be synchronized using a mutual exclusion lock (`sync.RWMutex` or equivalent) to prevent race conditions during reads and writes.
5. Once data processing in the background goroutine is complete, the update notification (calling `Refresh()` on the table widget) MUST be pushed to the main UI thread via Fyne's safe thread scheduler (`fyne.Do` or equivalent).
6. If the database query fails, the background goroutine MUST capture the error, log it, and the data grid SHALL display an error indicator without causing a crash or panic.
7. If the `ui.BusinessPool` is not initialized or is nil, the query execution MUST abort immediately and return a safe error state.

## Scenarios

### Scenario 1: Non-blocking Query Execution
*   **GIVEN** a GolemUI client window is active and a `"data_grid"` widget is rendered
*   **WHEN** the data query is triggered on `ui.BusinessPool`
*   **THEN** the query execution MUST run in a separate background goroutine
*   **AND** the main UI thread MUST remain fully responsive to user interactions such as window resize or clicks.

### Scenario 2: Formatting and Mutex Protected Storage
*   **GIVEN** a background goroutine retrieves raw database rows containing diverse data types
*   **WHEN** the background routine converts the rows into a string matrix
*   **THEN** it MUST lock the model mutex for writing during the data update
*   **AND** the main UI thread MUST acquire a read lock when reading data for rendering
*   **AND** all cell values MUST be formatted into their corresponding string representations.

### Scenario 3: Pushing Refresh to UI Thread
*   **GIVEN** a background query has successfully completed and updated the local cache matrix
*   **WHEN** the background thread finishes its execution
*   **THEN** it MUST schedule the table refresh on the main UI thread using Fyne's scheduler
*   **AND** the table widget SHALL refresh its visual state to show the updated rows.

### Scenario 4: Graceful Query Error Handling
*   **GIVEN** a query execution that fails due to a database syntax error or network disconnect
*   **WHEN** the background worker attempts to run the query
*   **THEN** the error MUST be caught and logged safely
*   **AND** the table widget SHALL render a query error state instead of panicking or stalling the client.
