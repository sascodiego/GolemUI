# Proposal: Screen Loading from DB

## Intent

Replace the hardcoded `homeNode` in `cmd/golemui/main.go` (lines 60-80) with a database-driven screen loader. Today every screen layout is compiled into the Go binary; this change makes the compositor read layouts from `golemui.vistas_consulta` at runtime, enabling UI changes without recompilation.

## Scope

### In Scope
- `pkg/ui/screen_loader.go` — standalone `LoadScreen(ctx, pool, vistaID) (NodeMeta, error)` that queries `golemui.vistas_consulta` and deserializes `config_columnas` JSONB into `NodeMeta`
- `pkg/ui/screen_loader_test.go` — table-driven tests: happy path, missing vista, malformed JSONB, empty filters
- `pkg/ui/compositor.go` — add `var CorePool db.DatabasePool` global (mirrors existing `BusinessPool` pattern)
- `cmd/golemui/main.go` — wire `CorePool` from bootstrap; replace hardcoded `homeNode` with `LoadScreen` call
- `pkg/lua/loader.go` — add `EntryPointViewID string` to `BootstrapConfig` (parallel to existing `EntryPointQuery`)
- `docker/init-db/02_init_core.sql` — INSERT sample row into `golemui.vistas_consulta` (id: `home`)

### Out of Scope
- Capa 3 auto-scaffolding (`mapeo_interfaz` overrides) — deferred to next phase
- Window title propagation from `vistas_consulta.titulo` — deferred
- Multi-screen navigation / routing — deferred
- `config_filtros` deserialization into filter widgets — deferred (load raw, don't interpret)

## Capabilities

### New Capabilities
- `screen-loading`: Loads screen layout definitions from `golemui.vistas_consulta` and deserializes them into `NodeMeta` trees for the compositor.

### Modified Capabilities
- `client-bootstrap`: Adds `EntryPointViewID` field to `BootstrapConfig` and wires `CorePool` into the `ui` package globals.

## Approach

Standalone function `LoadScreen` in `pkg/ui` — no `App` method, no import cycles. Uses `pool.QueryRow` to fetch `config_columnas` from `vistas_consulta` where `id = $1`, then `json.Unmarshal` into `NodeMeta`. A new `ui.CorePool` global (same pattern as `ui.BusinessPool`) provides the core DB connection. The Lua driver gains `EntryPointViewID` so the entry screen is configurable, not hardcoded.

JSONB contract for `config_columnas`: a single `NodeMeta` JSON object (recursive tree). `config_filtros` is stored but not interpreted in this phase.

TDD order: tests first using `MockDBPool.RegisterQuery`, then implementation.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `pkg/ui/screen_loader.go` | New | LoadScreen function |
| `pkg/ui/screen_loader_test.go` | New | Table-driven unit tests |
| `pkg/ui/compositor.go` | Modified | Add `CorePool` global variable |
| `cmd/golemui/main.go` | Modified | Replace homeNode with LoadScreen call |
| `pkg/lua/loader.go` | Modified | Add EntryPointViewID to BootstrapConfig |
| `docker/init-db/02_init_core.sql` | Modified | INSERT sample vista row |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| JSONB shape undefined — silent parse bugs | Medium | Loader validates `ComponentRef` and `Layout.Type` against known values; returns descriptive errors |
| `CorePool` nil at call time | Low | Explicit nil check with error in `LoadScreen`; test covers it |
| Existing `TestRunBootstrap_Success` breaks | Medium | Extend mock to include vista query registration; test both paths |
| ~250 LOC change size | Low | Single PR under 400-line budget |

## Rollback Plan

Revert to hardcoded `homeNode` in `main.go`. The `vistas_consulta` table already exists (no schema change); the INSERT is additive and harmless to remove. `CorePool` global defaults to nil, which is safe if LoadScreen is not called.

## Dependencies

- Existing `MockDBPool` in `pkg/db/mock_db.go` for testing
- `golemui.vistas_consulta` table (already created in init scripts)

## Success Criteria

- [ ] `LoadScreen` returns a valid `NodeMeta` tree from a mocked `vistas_consulta` row
- [ ] `LoadScreen` returns descriptive error when vista ID not found
- [ ] `LoadScreen` returns descriptive error when JSONB is malformed
- [ ] `go test ./...` passes with all new tests
- [ ] `main.go` renders the home screen from DB data instead of hardcoded struct
- [ ] `EntryPointViewID` is configurable from Lua driver
- [ ] Sample `home` vista row exists in init scripts
