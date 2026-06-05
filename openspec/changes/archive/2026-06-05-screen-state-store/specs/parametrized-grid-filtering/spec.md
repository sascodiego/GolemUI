# Delta for Parametrized Grid Filtering (parametrized-grid-filtering)

## MODIFIED Requirements

### Requirement: Grid subscribes to SUBMIT channel with positional args

The GolemUI compositor MUST subscribe each `data_grid` component (with non-empty `data_source`) to `eventbus.SubmitChannel` (`"screen:submit"`). On receiving a SUBMIT event, the grid MUST extract values from the snapshot map, convert them to positional arguments, and execute its `data_source` query with those args via `BusinessPool.Query(ctx, dataSource, args...)`. The grid MUST cancel any active/stale query context before executing the new query. Query execution MUST run in a background goroutine. Once complete, the grid MUST update its data under a write lock and dispatch `table.Refresh()` on the UI thread via `fyne.Do`.

(Previously: grid subscribed to the channel named by its own `bind_to` and received a single raw payload from the directly-publishing input.)

#### Scenario: Reactive filter with multiple positional parameters

- GIVEN a data grid subscribed to `eventbus.SubmitChannel` with `data_source "SELECT * FROM t WHERE title LIKE $1 AND amount >= $2"`
- WHEN a SUBMIT event is published with payload `{"filter:title": "Golang", "filter:min_amount": 50}`
- THEN the grid MUST cancel any current query context
- AND run the SQL query with positional args `["Golang", 50]` in a background thread
- AND update the table data thread-safely and call `Refresh()` on the UI thread

#### Scenario: Stale query cancellation on rapid submit

- GIVEN a data grid subscribed to `eventbus.SubmitChannel`
- WHEN SUBMIT fires with `{...v1}`, then `{...v2}`, then `{...v3}` in rapid succession
- THEN the grid MUST cancel the query contexts for v1 and v2
- AND only complete and render the result for v3

#### Scenario: Graceful handling of empty snapshot

- GIVEN a data grid subscribed to `eventbus.SubmitChannel`
- WHEN a SUBMIT event fires with an empty snapshot `{}`
- THEN the grid MUST execute the query with zero positional args safely
- AND the UI table SHALL be refreshed with the resulting records

## REMOVED Requirements

### Requirement: Grid subscribes to bind_to channel

(Reason: Grids no longer subscribe to individual `bind_to` channels. All grids receive filter data through the consolidated SUBMIT event on the fixed `eventbus.SubmitChannel`.)
