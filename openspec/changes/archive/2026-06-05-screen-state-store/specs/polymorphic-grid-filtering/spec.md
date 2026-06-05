# Polymorphic Grid Filtering Specification

## Purpose

Data grids dispatch filtering behavior based on their `filter_mode` metadata. Server-mode grids execute parameterized SQL queries with positional arguments derived from the SUBMIT snapshot. Client-mode grids filter an eagerly-loaded in-memory master buffer without any database call.

## Requirements

### Requirement: Grid Subscribes to Submit Channel

All data grids with a non-empty `data_source` MUST subscribe to `eventbus.SubmitChannel` instead of the old `bind_to` channel. On receiving a SUBMIT event, the grid SHALL dispatch based on its `filter_mode`.

#### Scenario: Grid receives SUBMIT event

- GIVEN a data grid with `filter_mode` set to `"server"` and `data_source` containing `$1` and `$2`
- WHEN a SUBMIT event is published with payload `{"filter:title": "Go", "filter:min_amount": 100}`
- THEN the grid MUST execute its `data_source` query with positional args `["Go", 100]`

### Requirement: Server-Side Filtering (FilterMode "server")

When `filter_mode` is `"server"` (the default), the grid MUST extract positional arguments from the SUBMIT snapshot and pass them as `$1, $2, ...` to `BusinessPool.Query`. Stale query cancellation MUST apply identically to the existing pattern.

#### Scenario: Multiple positional parameters

- GIVEN a data grid with `data_source` `"SELECT * FROM t WHERE title LIKE $1 AND amount >= $2"` and `filter_mode "server"`
- WHEN SUBMIT fires with snapshot `{"filter:title": "%Golem%", "filter:min_amount": 50}`
- THEN the query MUST be called with `args = ["%Golem%", 50]`

#### Scenario: Default filter_mode is server

- GIVEN a data grid with `data_source` set but `filter_mode` absent from metadata
- WHEN SUBMIT fires
- THEN the grid MUST behave identically to `filter_mode "server"`

### Requirement: Client-Side Filtering (FilterMode "client")

When `filter_mode` is `"client"`, the grid MUST filter its preloaded `masterRows` buffer in memory. The grid SHALL NOT call `BusinessPool.Query` on SUBMIT. The filter function MUST match rows where each column value contains (case-insensitive) the corresponding snapshot value as a substring.

#### Scenario: Client-side filter on loaded buffer

- GIVEN a data grid with `filter_mode "client"` and `master_data_source "SELECT * FROM transactions"`
- AND the master buffer contains rows `[["1", "Book A", "30"], ["2", "Game B", "50"]]`
- WHEN SUBMIT fires with snapshot `{"filter:title": "book"}`
- THEN the grid MUST display only the row containing `"Book A"`
- AND no `BusinessPool.Query` call SHALL be made for the SUBMIT event

#### Scenario: Empty filter shows all rows

- GIVEN a data grid with `filter_mode "client"` and 3 rows in master buffer
- WHEN SUBMIT fires with an empty snapshot `{}`
- THEN the grid MUST display all 3 rows unchanged

### Requirement: Eager Master Buffer Loading

When `filter_mode` is `"client"` and `master_data_source` is set, the compositor MUST execute `BusinessPool.Query(ctx, master_data_source)` exactly once during `Compose` and store the resulting rows as the grid's master buffer. This buffer SHALL NOT be modified by subsequent SUBMIT events.

#### Scenario: Master data loaded at compose time

- GIVEN a data grid with `filter_mode "client"` and `master_data_source "SELECT * FROM transactions"`
- WHEN `Compose` is called for that grid node
- THEN `BusinessPool.Query` MUST be called once with the `master_data_source` query
- AND the returned rows MUST be stored as `masterRows` on the grid model

### Requirement: NodeMeta Extension for FilterMode

The `NodeMeta` struct MUST accept two new optional JSON fields: `filter_mode` (string: `"server"` or `"client"`, default `"server"`) and `master_data_source` (string). Both fields MUST deserialize without error when absent from JSON.

#### Scenario: NodeMeta without new fields

- GIVEN a JSON payload for a data grid without `filter_mode` or `master_data_source`
- WHEN the JSON is unmarshaled into `NodeMeta`
- THEN `FilterMode` MUST default to `""` (treated as `"server"`)
- AND `MasterDataSource` MUST default to `""`
