# Delta for Client Bootstrap

## MODIFIED Requirements

### Requirement: Bootstrap Wiring of CorePool and Screen Loading

The client bootstrap sequence SHALL initialize both `ui.BusinessPool` and `ui.CorePool` from the database pool returned by `initDB`. After pool assignment, the bootstrap MUST call `ui.LoadScreen(ctx, ui.CorePool, config.EntryPointViewID)` to obtain the home screen `NodeMeta`, replacing the hardcoded `homeNode`.

If `EntryPointViewID` is empty, the bootstrap SHALL default to `"home"`.

If `LoadScreen` returns an error, the bootstrap MUST close the database pool and return the error.

(Previously: Bootstrap hardcoded a `homeNode` struct inline and composed it directly without database access.)

#### Scenario: Successful Bootstrap and Database Parameter Extraction

- GIVEN a valid `golemui_driver.lua` file containing the connection details for `golemui_core` and `negocio_production`
- WHEN the client bootstrap process is initiated
- THEN the ephemeral Lua engine SHALL load the driver file
- AND the Lua engine SHALL extract the correct host, port, database name, username, and password parameters for both the Core and Business databases
- AND the client SHALL terminate the Lua VM, releasing all occupied memory
- AND the bootstrap process SHALL return the database connection structures successfully.

#### Scenario: Ephemeral VM Lifetime and Memory Release

- GIVEN a running bootstrap sequence
- WHEN the connection parameters have been successfully read from the Lua VM
- THEN the client MUST call `L.Close()` or the engine equivalent to release all resources
- AND any subsequent attempt to query the closed Lua engine state SHALL return a closed-state error.

#### Scenario: Missing Driver File Error Handling

- GIVEN that the driver file `golemui_driver.lua` does not exist at the specified path
- WHEN the client bootstrap process is initiated
- THEN the bootstrap process MUST abort immediately
- AND it SHALL return a file-not-found error code and log the failure.

#### Scenario: Corrupt or Invalid Driver File

- GIVEN a driver file `golemui_driver.lua` that contains invalid Lua syntax or is missing required database fields
- WHEN the client bootstrap process is initiated
- THEN the Lua VM parser MUST fail during compilation
- AND the bootstrap process SHALL exit with a parser validation error.

#### Scenario: CorePool wired during bootstrap

- GIVEN a successful bootstrap with mocked database pools
- WHEN `RunBootstrap` completes
- THEN `ui.CorePool` SHALL be assigned from `dbPool.CorePool`
- AND `ui.CorePool` SHALL NOT be nil

#### Scenario: Home screen loaded from database

- GIVEN a successful bootstrap with a `MockDBPool` that has a registered query for `vistas_consulta` returning valid `config_columnas`
- WHEN `RunBootstrap` completes
- THEN the window content SHALL be composed from the `NodeMeta` returned by `LoadScreen`
- AND the hardcoded `homeNode` struct SHALL NOT be used

#### Scenario: LoadScreen failure during bootstrap

- GIVEN a bootstrap where `LoadScreen` returns an error (e.g., vista not found)
- WHEN `RunBootstrap` processes the error
- THEN the database pool SHALL be closed
- AND the function SHALL return a descriptive error wrapping the LoadScreen failure

## ADDED Requirements

### Requirement: EntryPointViewID in BootstrapConfig

The `BootstrapConfig` struct in `pkg/lua/loader.go` SHALL include an `EntryPointViewID string` field. The Lua config parser MUST read the `EntryPointViewID` key from the `golemui_driver` table using `getStringField`. If the key is absent, the field SHALL default to an empty string.

#### Scenario: EntryPointViewID parsed from Lua config

- GIVEN a valid `golemui_driver.lua` with `EntryPointViewID = "dashboard"`
- WHEN `LoadConfig` parses the file
- THEN `BootstrapConfig.EntryPointViewID` SHALL equal `"dashboard"`

#### Scenario: EntryPointViewID absent defaults to empty string

- GIVEN a valid `golemui_driver.lua` without an `EntryPointViewID` key
- WHEN `LoadConfig` parses the file
- THEN `BootstrapConfig.EntryPointViewID` SHALL equal `""`
