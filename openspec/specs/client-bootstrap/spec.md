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

## Requirements (continued)
7. The `main()` function in `cmd/golemui/main.go` MUST declare two CLI flags using the stdlib `flag` package:
   - `-config` (`string`, default `"golemui_driver.lua"`) — path to the Lua configuration driver file
   - `-view` (`string`, default `""`) — optional override for the initial view ID
   `flag.Parse()` MUST be called in `main()` before invoking `RunBootstrap`. Parsed values MUST be passed as arguments; `RunBootstrap` itself SHALL remain CLI-agnostic — no `flag` package dependency.
8. `RunBootstrap` MUST accept a `viewOverride string` parameter. The initial view ID SHALL be resolved by this precedence chain:
   1. `viewOverride != ""` → use `viewOverride`
   2. `cfg.EntryPointViewID != ""` → use `cfg.EntryPointViewID`
   3. Else → `"home"`
9. Adding `viewOverride` to `RunBootstrap` MUST NOT change the behavior of existing tests. All existing test call sites in `cmd/golemui/main_test.go` MUST pass after being updated to supply `""` as the 5th argument.

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

### Scenario 5: Custom Config Path via `-config` Flag
*   **GIVEN** a valid Lua config file at `/tmp/custom_config.lua`
*   **WHEN** `main()` is invoked with `-config=/tmp/custom_config.lua`
*   **THEN** `RunBootstrap` SHALL receive `/tmp/custom_config.lua` as `configPath`
*   **AND** bootstrap proceeds using that file

### Scenario 6: Missing Config File at Custom Path
*   **GIVEN** no file exists at `/tmp/missing.lua`
*   **WHEN** `main()` is invoked with `-config=/tmp/missing.lua`
*   **THEN** `RunBootstrap` SHALL return a file-not-found error

### Scenario 7: No Flags Provided — Defaults Applied
*   **GIVEN** no CLI flags are passed
*   **WHEN** `main()` is invoked
*   **THEN** `configPath` SHALL default to `"golemui_driver.lua"`
*   **AND** `viewOverride` SHALL default to `""`

### Scenario 8: View Override Wins Over Config
*   **GIVEN** a valid config with `EntryPointViewID = "dashboard"`
*   **WHEN** `RunBootstrap` is called with `viewOverride = "settings"`
*   **THEN** the initial screen SHALL be loaded with view ID `"settings"`

### Scenario 9: Empty Override Falls Through to Config
*   **GIVEN** a valid config with `EntryPointViewID = "transacciones_list"`
*   **WHEN** `RunBootstrap` is called with `viewOverride = ""`
*   **THEN** the initial screen SHALL be loaded with view ID `"transacciones_list"`

### Scenario 10: Both Empty — Defaults to "home"
*   **GIVEN** a valid config with `EntryPointViewID = ""`
*   **WHEN** `RunBootstrap` is called with `viewOverride = ""`
*   **THEN** the initial screen SHALL be loaded with view ID `"home"`

### Scenario 11: Existing Tests Pass with Empty viewOverride
*   **GIVEN** all existing `TestRunBootstrap_*` functions
*   **WHEN** each is updated to pass `""` as 5th argument to `RunBootstrap`
*   **THEN** all tests SHALL pass without behavioral change
