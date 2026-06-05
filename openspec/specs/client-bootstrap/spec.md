# Specification: Client Bootstrap (client-bootstrap)

## Introduction
The `client-bootstrap` capability defines the initialization sequence for the GolemUI Go client. It initializes the execution context, spins up an ephemeral Lua virtual machine to load database connection parameters from `golemui_driver.lua`, and returns connection details for the Core and Business databases.

## Requirements
1. The client bootstrap sequence MUST initialize the Go application environment.
2. The client MUST initialize an ephemeral Lua virtual machine to load the configuration driver.
3. The Lua engine MUST load `golemui_driver.lua` from a configurable path or default path.
4. The Lua engine MUST parse and return connection string parameters for two distinct databases:
   - Core Database (`golemui_core`)
   - Business Database (`negocio_production`)
5. The Lua engine MUST be fully terminated and its memory resources freed immediately after connection parameters are retrieved.
6. If the driver file is missing or invalid, the bootstrap process MUST fail gracefully and log a detailed error message.

## Scenarios

### Scenario 1: Successful Bootstrap and Database Parameter Extraction
*   **GIVEN** a valid `golemui_driver.lua` file containing the connection details for `golemui_core` and `negocio_production`
*   **WHEN** the client bootstrap process is initiated
*   **THEN** the ephemeral Lua engine SHALL load the driver file
*   **AND** the Lua engine SHALL extract the correct host, port, database name, username, and password parameters for both the Core and Business databases
*   **AND** the client SHALL terminate the Lua VM, releasing all occupied memory
*   **AND** the bootstrap process SHALL return the database connection structures successfully.

### Scenario 2: Ephemeral VM Lifetime and Memory Release
*   **GIVEN** a running bootstrap sequence
*   **WHEN** the connection parameters have been successfully read from the Lua VM
*   **THEN** the client MUST call `L.Close()` or the engine equivalent to release all resources
*   **AND** any subsequent attempt to query the closed Lua engine state SHALL return a closed-state error.

### Scenario 3: Missing Driver File Error Handling
*   **GIVEN** that the driver file `golemui_driver.lua` does not exist at the specified path
*   **WHEN** the client bootstrap process is initiated
*   **THEN** the bootstrap process MUST abort immediately
*   **AND** it SHALL return a file-not-found error code and log the failure.

### Scenario 4: Corrupt or Invalid Driver File
*   **GIVEN** a driver file `golemui_driver.lua` that contains invalid Lua syntax or is missing required database fields
*   **WHEN** the client bootstrap process is initiated
*   **THEN** the Lua VM parser MUST fail during compilation
*   **AND** the bootstrap process SHALL exit with a parser validation error.
