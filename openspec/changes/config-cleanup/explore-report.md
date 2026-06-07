# Config Cleanup: pkg/lua → pkg/config + Remove EntryPointQuery
**Scout Report** | 2026-06-07

---

## Executive Summary

Two coordinated changes:
1. **Rename `pkg/lua` → `pkg/config`** — rename the directory, update package name and all import references.
2. **Remove `EntryPointQuery`** — delete the dead struct field and all YAML/test references.

The surface is small: **3 Go source files** directly touch `pkg/lua`, and **1 test file** asserts on `EntryPointQuery`. No production code reads this field.

---

## Change 1: Rename pkg/lua → pkg/config

### Files That Import `GolemUI/pkg/lua`

| File | Line | Import Statement |
|------|------|------------------|
| `cmd/golemui/main.go` | 17 | `"GolemUI/pkg/lua"` |
| `cmd/golemui/main_test.go` | 11 | `"GolemUI/pkg/lua"` |
| `pkg/lua/loader_test.go` | 8 | `"GolemUI/pkg/lua"` |

### Package Declarations (to change)

| File | Current | Required |
|------|---------|----------|
| `pkg/lua/loader.go` | `package lua` | `package config` |
| `pkg/lua/loader_test.go` | `package lua_test` | `package config_test` |

### `lua.` Prefix Usages (must become `config.`)

**`cmd/golemui/main.go`:**
- Line 22: `Config *lua.BootstrapConfig` → `Config *config.BootstrapConfig`
- Line 47: `func RunBootstrap(ctx context.Context, cfg *lua.BootstrapConfig, ...)` → `cfg *config.BootstrapConfig`
- Line 146: `lua.LoadConfig(configPath)` → `config.LoadConfig(configPath)`

**`cmd/golemui/main_test.go`:**
- Line 102: `func testConfig() *lua.BootstrapConfig` → `*config.BootstrapConfig`
- Line 103: `return &lua.BootstrapConfig{` → `&config.BootstrapConfig{`
- Line 104: `lua.ConfigConexion{...}` → `config.ConfigConexion{...}` (both UIDB and BusinessDB)
- Line 110: `func loadTestYAML(t *testing.T, content string) *lua.BootstrapConfig` → `*config.BootstrapConfig`
- Line 117: `lua.LoadConfig(tmpFile)` → `config.LoadConfig(tmpFile)`
- Line 158: `lua.LoadConfig(...)` → `config.LoadConfig(...)`
- Line 189: `lua.LoadConfig(...)` → `config.LoadConfig(...)`
- Line 223: `lua.LoadConfig(...)` → `config.LoadConfig(...)`

**`pkg/lua/loader_test.go`:**
- Line 12: `lua.LoadConfig(...)` → `config.LoadConfig(...)`
- Line 40: `lua.LoadConfig(...)` → `config.LoadConfig(...)`
- Line 67: `lua.LoadConfig(...)` → `config.LoadConfig(...)`
- Line 95: `lua.LoadConfig(...)` → `config.LoadConfig(...)`
- Line 124: `lua.LoadConfig(...)` → `config.LoadConfig(...)`
- Line 156: `lua.LoadConfig(...)` → `config.LoadConfig(...)`
- Line 189: `lua.LoadConfig(...)` → `config.LoadConfig(...)`
- Line 221: `lua.LoadConfig(...)` → `config.LoadConfig(...)`

### Physical Rename Required

```
pkg/lua/loader.go      →  pkg/config/loader.go
pkg/lua/loader_test.go →  pkg/config/loader_test.go
```

---

## Change 2: Remove `EntryPointQuery`

### Field Definition (to delete)

**`pkg/lua/loader.go` line 21:**
```go
EntryPointQuery  string         `mapstructure:"entry_point_query"`
```
Full struct context:
```go
type BootstrapConfig struct {
    UIDB             ConfigConexion `mapstructure:"uidb"`
    BusinessDB       ConfigConexion `mapstructure:"business_db"`
    EntryPointQuery  string         `mapstructure:"entry_point_query"`  // ← REMOVE
    EntryPointViewID string         `mapstructure:"entry_point_view_id"`
    LayoutQuery      string         `mapstructure:"layout_query"`
}
```

### Evidence: Field Is Dead

- `cmd/golemui/main.go` — `EntryPointQuery` is **never referenced**. Only `EntryPointViewID` and `LayoutQuery` are consumed.
- `golemui_driver.yaml` — does **not** contain `entry_point_query`; it is absent in the production config.
- Prior verify-report (`openspec/changes/config-lua-to-json/verify-report.md`) flagged it as M2: *"EntryPointQuery field exists in the struct and is tested, but is never consumed in main.go or RunBootstrap."*

### Test References to Remove/Update

**`pkg/lua/loader_test.go`:**

| Line(s) | Content | Action |
|---------|---------|--------|
| 32 | `entry_point_query: "SELECT * FROM golemui.layouts LIMIT 1"` (YAML fixture) | DELETE from YAML fixture |
| 53–54 | `if config.EntryPointQuery != "SELECT * FROM golemui.layouts LIMIT 1"` + error | DELETE entire if-block |
| 87 | `entry_point_query: "SELECT * FROM golemui.layouts LIMIT 1"` | DELETE |
| 115 | `entry_point_query: "SELECT * FROM golemui.layouts LIMIT 1"` | DELETE |
| 148 | `entry_point_query: "SELECT * FROM golemui.layouts LIMIT 1"` | DELETE |
| 180 | `entry_point_query: "SELECT * FROM golemui.layouts LIMIT 1"` | DELETE |
| 213 | `entry_point_query: "SELECT * FROM golemui.layouts LIMIT 1"` | DELETE |

**`cmd/golemui/main_test.go`:**
- Line 178: `entry_point_query: "SELECT * FROM golemui.layouts LIMIT 1"` in a YAML fixture → DELETE

---

## Change Summary

### Files to Modify

| File | Changes |
|------|---------|
| `pkg/lua/loader.go` → `pkg/config/loader.go` | Rename file. `package lua` → `package config`. Remove `EntryPointQuery` field (line 21). |
| `pkg/lua/loader_test.go` → `pkg/config/loader_test.go` | Rename file. `package lua_test` → `package config_test`. Update import. Remove all YAML `entry_point_query` lines. Remove assertion on `config.EntryPointQuery` (lines 53–54). |
| `cmd/golemui/main.go` | Change import `"GolemUI/pkg/lua"` → `"GolemUI/pkg/config"`. All `lua.` prefix → `config.` |
| `cmd/golemui/main_test.go` | Same import + prefix changes. Remove `entry_point_query` from YAML fixture at line 178. |
| `golemui_driver.yaml` | No change needed — `entry_point_query` already absent here. |

### go.mod Impact

`gopher-lua v1.1.2` is still listed as a direct require (from the pre-Viper era). It is **not imported by `pkg/config` now**. The cleanup of this stale transitive is a separate concern — leaving it untouched here avoids scope creep.

---

## Start Here

1. **`pkg/lua/loader.go`** — rename to `pkg/config/loader.go`, change `package lua` → `package config`, delete line 21 (`EntryPointQuery`).
2. **`pkg/lua/loader_test.go`** — rename to `pkg/config/loader_test.go`, update package and import, strip all `entry_point_query` YAML lines and the assertion block.
3. **`cmd/golemui/main.go`** — update import and `lua.` → `config.` references.
4. **`cmd/golemui/main_test.go`** — same import + prefix changes; remove the YAML fixture line with `entry_point_query`.

Run verification:
```bash
go build ./pkg/config/ ./cmd/golemui/
go test ./pkg/config/ ./cmd/golemui/ -v
go vet ./pkg/config/ ./cmd/golemui/
```
