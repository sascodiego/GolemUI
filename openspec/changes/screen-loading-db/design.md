# Design: Screen Loading from DB

## Technical Approach

Replace the hardcoded `homeNode` in `main.go` with a standalone `LoadScreen` function that queries `golemui.vistas_consulta` by ID, deserializes the `config_columnas` JSONB into a `NodeMeta` tree, and returns it for composition. A new `ui.CorePool` global (parallel to `ui.BusinessPool`) provides the core database connection. The Lua driver gains `EntryPointViewID` for configurable entry screens.

## Architecture Decisions

| Decision | Choice | Rejected | Rationale |
|----------|--------|----------|-----------|
| Loader location | Standalone func in `pkg/ui` | Method on `App` struct | No import cycles; testable with `MockDBPool`; matches existing global pool pattern |
| Pool access pattern | `var CorePool db.DatabasePool` global | Pass pool per call in compositor | Mirrors `BusinessPool` pattern; `LoadScreen` receives pool as parameter for testability |
| JSONB shape for `config_columnas` | 1:1 with `NodeMeta` struct tags | Custom intermediate schema | Zero translation — `json.Unmarshal` directly into NodeMeta; missing fields default to Go zero values |
| `config_filtros` handling | Store `'[]'::jsonb`, don't interpret | Parse into filter widgets | Explicitly out of scope per proposal; deferred to next phase |
| Entry screen config | `EntryPointViewID` in Lua config | Hardcode `"home"` in Go | Avoids accumulating hardcoded defaults; follows existing `EntryPointQuery` pattern |

## Data Flow

```
golemui_driver.lua
       │
       ▼
  LoadConfig() ──→ BootstrapConfig.EntryPointViewID
       │
       ▼
  RunBootstrap()
       │
       ├── ui.CorePool = dbPool.CorePool
       ├── ui.BusinessPool = dbPool.BusinessPool
       │
       ▼
  LoadScreen(ctx, CorePool, vistaID)
       │
       ├── pool.QueryRow("SELECT config_columnas FROM golemui.vistas_consulta WHERE id = $1", vistaID)
       ├── row.Scan(&jsonBytes)
       ├── json.Unmarshal(&nodeMeta)
       │
       ▼
  Compose(nodeMeta) ──→ fyne.CanvasObject
       │
       ▼
  window.SetContent(canvasObj)
```

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `pkg/ui/screen_loader.go` | Create | `LoadScreen(ctx, pool, vistaID) (NodeMeta, error)` — queries vistas_consulta, deserializes JSONB |
| `pkg/ui/screen_loader_test.go` | Create | Table-driven tests: happy path, missing vista, malformed JSONB, nil pool |
| `pkg/ui/compositor.go` | Modify | Add `var CorePool db.DatabasePool` at line 18 (next to BusinessPool) |
| `cmd/golemui/main.go` | Modify | Wire `ui.CorePool`, replace homeNode with `LoadScreen` call, default vistaID to `"home"` |
| `cmd/golemui/main_test.go` | Modify | Extend `TestRunBootstrap_Success`: register vista query in mock, assert CorePool wired and screen loaded |
| `pkg/lua/loader.go` | Modify | Add `EntryPointViewID string` field to `BootstrapConfig`; parse with `getStringField(tbl, "EntryPointViewID")` |
| `pkg/lua/loader_test.go` | Modify | Add tests for EntryPointViewID present and absent |
| `docker/init-db/02_init_core.sql` | Modify | INSERT sample `home` vista with valid JSONB config_columnas |

## Interfaces / Contracts

### LoadScreen Function

```go
// pkg/ui/screen_loader.go
func LoadScreen(ctx context.Context, pool db.DatabasePool, vistaID string) (NodeMeta, error)
```

- Nil pool → `fmt.Errorf("LoadScreen: pool is nil")` (no DB call attempted)
- `pool.QueryRow` with SQL: `SELECT config_columnas FROM golemui.vistas_consulta WHERE id = $1`
- `pgx.ErrNoRows` → `fmt.Errorf("LoadScreen: vista %q not found", vistaID)`
- Invalid JSON → wrap `json.SyntaxError` with context

### JSONB Contract for config_columnas

```json
{
  "area": "home_root",
  "component_ref": "container",
  "layout": {"type": "vertical"},
  "children": [
    {"area": "header", "component_ref": "label", "label": "Welcome to GolemUI Desktop Client"}
  ]
}
```

Maps 1:1 to `NodeMeta` via existing `json` struct tags. Recursive `children` supported. Unrecognized `component_ref` values fall through to `Compose`'s existing fallback handler (line 172).

### JSONB Contract for config_filtros (Phase 1)

```sql
'[]'::jsonb
```

Stored but not interpreted. Future phases parse into filter widget descriptors.

### Sample INSERT for Init Script

```sql
INSERT INTO golemui.vistas_consulta (id, titulo, origen_datos, config_columnas, config_filtros)
VALUES ('home', 'Home', 'SELECT 1',
  '{"area":"home_root","component_ref":"container","layout":{"type":"vertical"},"children":[{"area":"header","component_ref":"label","label":"Welcome to GolemUI Desktop Client"}]}'::jsonb,
  '[]'::jsonb)
ON CONFLICT (id) DO NOTHING;
```

## Testing Strategy

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Unit | LoadScreen happy path | `MockDBPool.RegisterQuery` with valid JSONB; assert NodeMeta tree shape |
| Unit | LoadScreen missing vista | Register query returning `pgx.ErrNoRows`; assert descriptive error |
| Unit | LoadScreen malformed JSONB | Register query with `{bad json`; assert parse error wrap |
| Unit | LoadScreen nil pool | Call with nil; assert error, no panic |
| Unit | CorePool defaults nil | Assert `ui.CorePool == nil` at package init |
| Unit | EntryPointViewID parsed | Lua config with key; assert field value |
| Unit | EntryPointViewID absent | Lua config without key; assert empty string |
| Integration | Bootstrap wires CorePool | Extend `TestRunBootstrap_Success`: register vista query, assert `ui.CorePool` assigned |
| Integration | Home loaded from DB | Mock returns valid JSONB; assert window content from `LoadScreen` not hardcoded |
| Integration | LoadScreen failure | Mock returns `ErrNoRows`; assert pool closed, error returned |

Mock registration pattern (from existing codebase):
```go
mockPool := db.NewMockDBPool()
mockPool.RegisterQuery(
    "SELECT config_columnas FROM golemui.vistas_consulta WHERE id = $1",
    []string{"config_columnas"},
    [][]any{{`{"component_ref":"container","layout":{"type":"vertical"}}`}},
    nil,
)
```

## Migration / Rollout

No migration required. The `vistas_consulta` table already exists in init scripts. The INSERT is additive with `ON CONFLICT DO NOTHING`. Ephemeral DB rebuilds automatically via `docker-compose up`. Rollback: revert to hardcoded `homeNode` in `main.go`.

## Open Questions

None — all scope decisions finalized in proposal and specs.
