# Specification: Database Mocking Infrastructure (database-mocking-infrastructure)

## Introduction
The `database-mocking-infrastructure` capability provides a mock connection pool implementation to enable unit and integration testing of GolemUI components without requiring a live PostgreSQL instance. By defining `MockDBPool`, `MockRows`, and `MockRow` in `pkg/db/mock_db.go`, test suites can stub database outputs and check code pathways in isolation.

## Requirements
1. The `pkg/db` package MUST provide a `MockDBPool` struct that fully implements the `DatabasePool` interface.
2. The `MockDBPool` MUST allow tests to register stubbed results and errors mapped to expected SQL queries or execution statements.
3. The `MockDBPool` MUST implement thread-safe structures to protect concurrent read and write operations on registered stubs.
4. The `MockDBPool` MUST support mocking `Query` by returning an instance of `MockRows` that implements `pgx.Rows`.
5. The `MockRows` struct MUST allow iteration and scanning of predefined column names and row values in the exact sequence configured by the test.
6. The `MockDBPool` MUST support mocking `QueryRow` by returning a `MockRow` instance that implements `pgx.Row`.
7. The `MockDBPool` MUST support mocking `Exec` by returning custom `pgconn.CommandTag` outputs and errors.
8. The `MockDBPool` MUST support mocking `SendBatch` by returning a mock implementation of `pgx.BatchResults`.

## Scenarios

### Scenario 1: Scanning Predefined Stub Data from MockRows
*   **GIVEN** a `MockDBPool` instance configured to return two rows with columns `["id", "name"]` and values `[[101, "core_widget"], [102, "layout_grid"]]` for the query `"SELECT id, name FROM components"`
*   **WHEN** the system calls the `Query` method with that query
*   **THEN** the mock pool MUST return a valid `pgx.Rows` implementation
*   **AND** the calling code SHALL iterate through the rows and scan the values `101`, `"core_widget"`, `102`, and `"layout_grid"` successfully.

### Scenario 2: Retrieving Exec Command Tag and Rows Affected
*   **GIVEN** a `MockDBPool` instance configured to return a command tag representing 1 row affected for the statement `"UPDATE components SET name = $1 WHERE id = $2"`
*   **WHEN** the system calls the `Exec` method with that statement
*   **THEN** the mock pool MUST return the configured `pgconn.CommandTag` and a nil error.

### Scenario 3: Simulating Database Query Error
*   **GIVEN** a `MockDBPool` instance configured to return a database connection timeout error for the query `"SELECT * FROM logs"`
*   **WHEN** the system calls the `Query` method with that query
*   **THEN** the query execution MUST fail
*   **AND** the return values MUST consist of a nil rows pointer and the configured connection timeout error.

### Scenario 4: QueryRow Single Record Matching
*   **GIVEN** a `MockDBPool` instance configured to return a mock row with the single value `"active"` for the query `"SELECT status FROM systems WHERE id = 1"`
*   **WHEN** the system calls `QueryRow` with that query
*   **THEN** the returned `pgx.Row` MUST scan the string value `"active"` into the target destination pointer without error.
