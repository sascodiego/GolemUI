# Config Cleanup: Rename `pkg/lua` → `pkg/config` + Remove Dead `EntryPointQuery`

**Proposal** | 2026-06-07

## Problem Statement

The bootstrap config package still carries the name `pkg/lua` — a leftover from the original Lua-based config loader. The package has since migrated to Viper/YAML and no longer contains any Lua logic, making the name misleading.

Additionally, the `BootstrapConfig` struct retains the field `EntryPointQuery` (defined at `loader.go:21`), which is **never consumed** in production code (`main.go` and `RunBootstrap` ignore it). It only appears in test YAML fixtures and one assertion block. A prior verify report flagged it as dead code (M2).

## Proposed Solution

1. **Rename `pkg/lua` → `pkg/config`** — rename the physical directory, update `package` declarations, and update all import paths and qualified-name prefixes (`lua.` → `config.`).
2. **Remove `EntryPointQuery`** — delete the dead struct field, its `mapstructure` tag, all YAML fixture entries (`entry_point_query:`), and the test assertion that checks it.

No behavioral changes. Pure refactor.

## Scope

### In Scope

| # | Item | Detail |
|---|------|--------|
| 1 | Directory rename | `pkg/lua/` → `pkg/config/` |
| 2 | Package declarations | `package lua` → `package config`; `package lua_test` → `package config_test` |
| 3 | Import paths | `"GolemUI/pkg/lua"` → `"GolemUI/pkg/config"` in 3 files |
| 4 | Qualified prefixes | All `lua.` references → `config.` (3 files: main.go, main_test.go, loader_test.go) |
| 5 | Dead field removal | Delete `EntryPointQuery` from `BootstrapConfig` struct |
| 6 | Test cleanup | Remove `entry_point_query` YAML lines (7 occurrences) and 1 assertion block from `loader_test.go` |

### Out of Scope

- `gopher-lua` dependency in `go.mod` (unrelated, no callers remain)
- `golemui_driver.yaml` — already absent of `entry_point_query`; no changes needed
- `validateConexion` logic — unchanged
- Any behavioral, API, or schema changes

## Risks

| Risk | Likelihood | Mitigation |
|------|-----------|------------|
| Missed import reference breaks build | Low | `go build ./...` and `go vet ./...` catch all stale imports |
| Renamed package breaks external consumers | None | Package is internal; no public API consumers outside this repo |
| Test regression from fixture cleanup | Low | Straightforward deletion; `go test ./pkg/config/` validates immediately |

## Affected Files

| File | Action | Description |
|------|--------|-------------|
| `pkg/lua/loader.go` → `pkg/config/loader.go` | **RENAME + EDIT** | Package decl, remove `EntryPointQuery` field |
| `pkg/lua/loader_test.go` → `pkg/config/loader_test.go` | **RENAME + EDIT** | Package decl, import, strip `entry_point_query` YAML + assertion |
| `cmd/golemui/main.go` | **EDIT** | Import path, `lua.` → `config.` prefix |
| `cmd/golemui/main_test.go` | **EDIT** | Import path, `lua.` → `config.` prefix |

## Estimated Lines Changed

~100 lines (imports, declarations, fixture lines, one assertion block).
