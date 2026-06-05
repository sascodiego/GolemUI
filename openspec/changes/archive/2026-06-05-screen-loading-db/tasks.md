# Tasks: Screen Loading from DB

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~265 (135 new + 130 changed) |
| 400-line budget risk | Low |
| Chained PRs recommended | Yes (user chose force-chained) |
| Suggested split | PR 1: Foundation + Core → PR 2: Integration |
| Delivery strategy | force-chained |
| Chain strategy | stacked-to-main |

Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: stacked-to-main
400-line budget risk: Low

### Suggested Work Units

| Unit | Goal | Likely PR | Notes |
|------|------|-----------|-------|
| 1 | Foundation + LoadScreen (purely additive, no main.go changes) | PR 1 | base: main; self-contained, all unit tests pass |
| 2 | Integration wiring (main.go + main_test.go) | PR 2 | base: main (after PR 1 merges); replaces hardcoded homeNode |

## Phase 1: Foundation (PR 1)

- [x] 1.1 Add `var CorePool db.DatabasePool` to `pkg/ui/compositor.go:18` (next to BusinessPool)
- [x] 1.2 Write test: `TestCorePool_DefaultsNil` in `pkg/ui/compositor_test.go` — assert `ui.CorePool == nil`
- [x] 1.3 Add `EntryPointViewID string` field to `BootstrapConfig` in `pkg/lua/loader.go:21`; parse with `getStringField(tbl, "EntryPointViewID")` in `LoadConfig`; add to return struct
- [x] 1.4 Write test: `TestLoadConfig_EntryPointViewID_Present` in `pkg/lua/loader_test.go` — Lua config with `EntryPointViewID = "dashboard"`, assert field equals `"dashboard"`
- [x] 1.5 Write test: `TestLoadConfig_EntryPointViewID_Absent` in `pkg/lua/loader_test.go` — Lua config without key, assert field equals `""`
- [x] 1.6 Add INSERT sample `home` vista to `docker/init-db/02_init_core.sql` after line 72 — `id='home'`, valid JSONB `config_columnas`, `'[]'::jsonb` for `config_filtros`, `ON CONFLICT DO NOTHING`

## Phase 2: Core Implementation — LoadScreen (PR 1, TDD)

- [x] 2.1 RED: Create `pkg/ui/screen_loader_test.go` with table-driven `TestLoadScreen` — cases: happy path (valid JSONB → NodeMeta tree), missing vista (pgx.ErrNoRows → descriptive error), malformed JSONB (parse error wrap), nil pool (error, no DB call)
- [x] 2.2 RED: Add `TestCorePool_DefaultsNil` to same file or `compositor_test.go`
- [x] 2.3 GREEN: Create `pkg/ui/screen_loader.go` — `LoadScreen(ctx, pool, vistaID) (NodeMeta, error)` with nil check, `pool.QueryRow` SQL, `pgx.ErrNoRows` mapping, `json.Unmarshal`, error wrapping
- [x] 2.4 Run `go test ./pkg/ui/...` — all LoadScreen tests pass

## Phase 3: Integration Wiring (PR 2)

- [x] 3.1 Wire `ui.CorePool = dbPool.CorePool` in `cmd/golemui/main.go:55` (next to BusinessPool assignment)
- [x] 3.2 Replace hardcoded `homeNode` with `LoadScreen(ctx, ui.CorePool, vistaID)` where vistaID defaults to `"home"` if `config.EntryPointViewID` is empty
- [x] 3.3 Extend `TestRunBootstrap_Success` in `cmd/golemui/main_test.go` — register vista query in mock CorePool, assert `ui.CorePool` wired, assert window content composed from LoadScreen
- [x] 3.4 Add `TestRunBootstrap_LoadScreenFailure` — mock returns `pgx.ErrNoRows`, assert pool closed, error returned
- [x] 3.5 Run `go test ./...` — full suite passes

## Phase 4: Verify

- [ ] 4.1 `go test ./...` — zero failures
- [ ] 4.2 `go vet ./...` — no issues
- [ ] 4.3 Manual: `docker-compose up -d` — verify init script inserts home vista row
