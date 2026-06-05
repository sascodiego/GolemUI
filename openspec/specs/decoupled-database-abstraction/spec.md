# Specification: Decoupled Database Abstraction (decoupled-database-abstraction)

## Introduction
The `decoupled-database-abstraction` capability decouples the GolemUI database layer from concrete pgxpool connections. By defining generic `DBQuerier` and `DatabasePool` interfaces in `pkg/db/db.go`, the system allows tests to mock database interaction, conforming to GolemUI's 4-layer decoupling architecture.

## Requirements
1. The `pkg/db` package MUST define the `DBQuerier` interface to abstract SQL query execution.
2. The `DBQuerier` interface MUST declare exactly the following method signatures:
   - `Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)`
   - `QueryRow(ctx context.Context, sql string, args ...any) pgx.Row`
   - `Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)`
   - `SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults`
3. The `pkg/db` package MUST define the `DatabasePool` interface, which embeds `DBQuerier`.
4. The `DatabasePool` interface MUST declare the following lifecycle methods:
   - `Ping(ctx context.Context) error`
   - `Close()`
5. The `DB` struct in `pkg/db/db.go` MUST be updated to type its `CorePool` and `BusinessPool` fields as `DatabasePool` instead of `*pgxpool.Pool`.
6. The `InitDB` function MUST return a `*DB` containing connections that satisfy the `DatabasePool` interface.

## Scenarios

### Scenario 1: Initializing the Database Pools
*   **GIVEN** valid configuration parameters for both the Core and Business databases
*   **WHEN** the application calls `InitDB` at bootstrap
*   **THEN** the initialization sequence MUST instantiate two concrete pgx pools
*   **AND** the system SHALL verify health status by calling `Ping` on each pool through the `DatabasePool` interface
*   **AND** it MUST return a populated `*DB` where both pools are exposed as `DatabasePool`.

### Scenario 2: Standard Query Execution Through DBQuerier
*   **GIVEN** an active database pool typed as `DBQuerier`
*   **WHEN** the client executes a query using the `Query` method
*   **THEN** the call MUST delegate to the underlying pool and return `pgx.Rows` and a nil error
*   **AND** the caller SHALL process the results without referencing any concrete database pool types.

### Scenario 3: Terminating Database Connections
*   **GIVEN** an active `*DB` instance holding `DatabasePool` interfaces
*   **WHEN** the application invokes the `Close` method on the `*DB` instance
*   **THEN** the system MUST call `Close` on both `CorePool` and `BusinessPool`
*   **AND** all underlying network resources associated with the connection pools MUST be released.
