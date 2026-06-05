# Screen Loading Specification

## Purpose

Loads screen layout definitions from `golemui.vistas_consulta` and deserializes the `config_columnas` JSONB column into `NodeMeta` trees for the compositor.

## Requirements

### Requirement: LoadScreen Function

The system SHALL provide a standalone function `LoadScreen(ctx context.Context, pool db.DatabasePool, vistaID string) (NodeMeta, error)` that queries `golemui.vistas_consulta` by ID and returns a fully deserialized `NodeMeta` tree.

The function MUST use `pool.QueryRow` to execute `SELECT config_columnas FROM golemui.vistas_consulta WHERE id = $1` with the given `vistaID`.

#### Scenario: Happy path — valid vista returns NodeMeta tree

- GIVEN a `MockDBPool` registered with a query returning valid `config_columnas` JSONB for vista ID `"home"`
- WHEN `LoadScreen(ctx, pool, "home")` is called
- THEN the function SHALL return a `NodeMeta` with `ComponentRef` equal to `"container"` and `Children` populated
- AND no error SHALL be returned

#### Scenario: Vista ID not found

- GIVEN a `MockDBPool` where `QueryRow` returns `pgx.ErrNoRows`
- WHEN `LoadScreen(ctx, pool, "nonexistent")` is called
- THEN the function SHALL return a descriptive error containing the vista ID
- AND the returned `NodeMeta` SHALL be the zero value

#### Scenario: Malformed JSONB in config_columnas

- GIVEN a `MockDBPool` returning `config_columnas` with invalid JSON (e.g., `{bad json`)
- WHEN `LoadScreen(ctx, pool, "home")` is called
- THEN the function SHALL return a descriptive JSON parse error
- AND the returned `NodeMeta` SHALL be the zero value

#### Scenario: Nil pool argument

- GIVEN `pool` is `nil`
- WHEN `LoadScreen(ctx, nil, "home")` is called
- THEN the function SHALL return an error indicating the pool is nil
- AND SHALL NOT attempt any database query

### Requirement: CorePool Global Variable

The `pkg/ui` package SHALL expose a `var CorePool db.DatabasePool` global variable, following the same pattern as the existing `BusinessPool`.

`CorePool` defaults to `nil` and MUST be explicitly assigned during bootstrap.

#### Scenario: CorePool defaults to nil

- GIVEN the `ui` package is imported without prior assignment
- WHEN `ui.CorePool` is read
- THEN its value SHALL be `nil`

### Requirement: Sample Home Vista in Init Scripts

The `docker/init-db/02_init_core.sql` script SHALL contain an `INSERT` statement into `golemui.vistas_consulta` with `id = 'home'`, a valid `titulo`, `origen_datos`, and a well-formed `config_columnas` JSONB describing a vertical container with a label child.

`config_filtros` SHALL be an empty JSON array `'[]'::jsonb`. The statement MUST use `ON CONFLICT (id) DO NOTHING`.

#### Scenario: Init script inserts home vista row

- GIVEN a fresh database initialization via `docker-compose up`
- WHEN the init scripts execute
- THEN `golemui.vistas_consulta` SHALL contain a row with `id = 'home'`
- AND `config_columnas` SHALL deserialize into a valid `NodeMeta` with `ComponentRef = "container"`
