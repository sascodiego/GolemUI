# Delta for screen-loading

## ADDED Requirements

### Requirement: LayoutQuery Field in BootstrapConfig

The `BootstrapConfig` struct in `pkg/lua/loader.go` SHALL include a `LayoutQuery string` field. The `LoadConfig` function MUST parse this field from the `LayoutQuery` key of the `golemui_driver` Lua table using `getStringField`. When the key is absent or nil, the field MUST default to an empty string.

#### Scenario: LayoutQuery present in Lua config

- GIVEN a Lua config containing `LayoutQuery = "SELECT col FROM tbl WHERE id = $1"` in the `golemui_driver` table
- WHEN `LoadConfig` is called
- THEN `BootstrapConfig.LayoutQuery` SHALL equal `"SELECT col FROM tbl WHERE id = $1"`

#### Scenario: LayoutQuery absent from Lua config

- GIVEN a Lua config without a `LayoutQuery` key
- WHEN `LoadConfig` is called
- THEN `BootstrapConfig.LayoutQuery` SHALL equal `""`

### Requirement: DefaultLayoutQuery Constant

The `pkg/ui` package SHALL define an exported constant `DefaultLayoutQuery` equal to `"SELECT config_columnas FROM golemui.vistas_consulta WHERE id = $1"`. This constant MUST be non-empty.

#### Scenario: DefaultLayoutQuery equals current hardcoded SQL

- GIVEN the `DefaultLayoutQuery` constant in `pkg/ui/screen_loader.go`
- WHEN its value is inspected
- THEN it SHALL equal `"SELECT config_columnas FROM golemui.vistas_consulta WHERE id = $1"`

### Requirement: LayoutQuery Property in Lua Driver

The production `golemui_driver.lua` SHALL contain a `LayoutQuery` key in the `golemui_driver` table. Its value MUST be a valid SQL query string returning a single `config_columnas` JSONB column with `$1` as the vista ID parameter.

#### Scenario: Lua driver contains LayoutQuery property

- GIVEN the production `golemui_driver.lua` file
- WHEN the `golemui_driver` table is parsed
- THEN a `LayoutQuery` key SHALL be present with a non-empty string value

## MODIFIED Requirements

### Requirement: LoadScreen Function

The system SHALL provide a function `LoadScreen(ctx context.Context, pool db.DatabasePool, vistaID string, layoutQuery string) (NodeMeta, error)` that queries a layout view and returns a deserialized `NodeMeta` tree.

When `layoutQuery` is non-empty, the function MUST use it as the SQL query. When `layoutQuery` is empty, the function MUST fall back to `DefaultLayoutQuery`.

The function MUST use `pool.QueryRow` to execute the resolved query with the given `vistaID` as the `$1` parameter.

(Previously: `LoadScreen` used a hardcoded SQL with no external configuration path and no `layoutQuery` parameter.)

#### Scenario: Happy path — valid vista returns NodeMeta tree

- GIVEN a `MockDBPool` registered with a query returning valid `config_columnas` JSONB for vista ID `"home"`
- WHEN `LoadScreen(ctx, pool, "home", "")` is called
- THEN the function SHALL return a `NodeMeta` with `ComponentRef` equal to `"container"` and `Children` populated
- AND no error SHALL be returned

#### Scenario: Vista ID not found

- GIVEN a `MockDBPool` where `QueryRow` returns `pgx.ErrNoRows`
- WHEN `LoadScreen(ctx, pool, "nonexistent", "")` is called
- THEN the function SHALL return a descriptive error containing the vista ID
- AND the returned `NodeMeta` SHALL be the zero value

#### Scenario: Malformed JSONB in config_columnas

- GIVEN a `MockDBPool` returning `config_columnas` with invalid JSON
- WHEN `LoadScreen(ctx, pool, "home", "")` is called
- THEN the function SHALL return a descriptive JSON parse error
- AND the returned `NodeMeta` SHALL be the zero value

#### Scenario: Nil pool argument

- GIVEN `pool` is `nil`
- WHEN `LoadScreen(ctx, nil, "home", "")` is called
- THEN the function SHALL return an error indicating the pool is nil
- AND SHALL NOT attempt any database query

#### Scenario: Custom layoutQuery overrides default

- GIVEN a `MockDBPool` registered with `"SELECT layout FROM custom WHERE id = $1"` returning valid JSONB
- WHEN `LoadScreen(ctx, pool, "custom", "SELECT layout FROM custom WHERE id = $1")` is called
- THEN the function SHALL execute the custom query (not `DefaultLayoutQuery`)
- AND SHALL return the parsed `NodeMeta` without error

#### Scenario: Empty layoutQuery triggers fallback

- GIVEN a `MockDBPool` registered with the `DefaultLayoutQuery` SQL string
- WHEN `LoadScreen(ctx, pool, "home", "")` is called
- THEN the function SHALL use `DefaultLayoutQuery` as the SQL
- AND SHALL return the parsed `NodeMeta` without error
