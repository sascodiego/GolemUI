# Design: Config Cleanup — Rename `pkg/lua` → `pkg/config` & Remove `EntryPointQuery`

## Overview

Mechanical refactor with two goals:

1. **Rename package** `pkg/lua` → `pkg/config` — the package has zero Lua logic; the name is misleading.
2. **Remove `EntryPointQuery`** — the field is populated from YAML but never consumed at runtime. Entry-point resolution uses `EntryPointViewID` + a SQL query inside `ui.LoadScreen`. The config key `entry_point_query` is dead weight.

No behavioral change. No schema change. No new files beyond the directory move.

---

## Affected Files

| File | Change Type |
|---|---|
| `pkg/lua/loader.go` | move + rename package + remove field |
| `pkg/lua/loader_test.go` | move + rename package + update import + remove `entry_point_query` from fixtures + remove `EntryPointQuery` assertion |
| `cmd/golemui/main.go` | update import + rename types/functions |
| `cmd/golemui/main_test.go` | update import + rename types + remove `entry_point_query` from YAML fixtures |
| `pkg/lua/` directory | remove (empty after move) |

---

## Step-by-Step Changes

### Step 1 — Create `pkg/config/` and move files

```bash
mkdir -p pkg/config
git mv pkg/lua/loader.go pkg/config/loader.go
git mv pkg/lua/loader_test.go pkg/config/loader_test.go
```

**Depends on:** nothing.
**Estimated diff:** 0 logical lines changed (pure rename, git detects history).

---

### Step 2 — Update `pkg/config/loader.go`

| Location | Before | After |
|---|---|---|
| line 1 | `package lua` | `package config` |
| `BootstrapConfig` struct | `EntryPointQuery  string \`mapstructure:"entry_point_query"\`` | *(remove entire line)* |

Resulting struct:

```go
type BootstrapConfig struct {
	UIDB             ConfigConexion `mapstructure:"uidb"`
	BusinessDB       ConfigConexion `mapstructure:"business_db"`
	EntryPointViewID string         `mapstructure:"entry_point_view_id"`
	LayoutQuery      string         `mapstructure:"layout_query"`
}
```

**Depends on:** Step 1.
**Estimated diff:** 2 lines.

---

### Step 3 — Update `pkg/config/loader_test.go`

| Location | Before | After |
|---|---|---|
| line 1 | `package lua_test` | `package config_test` |
| import | `"GolemUI/pkg/lua"` | `"GolemUI/pkg/config"` |
| all call sites | `lua.LoadConfig(...)` | `config.LoadConfig(...)` |
| YAML fixture in `TestLoadConfig_Success` | `entry_point_query: "SELECT * FROM golemui.layouts LIMIT 1"` | *(remove line)* |
| assertion in `TestLoadConfig_Success` | `if config.EntryPointQuery != "SELECT * FROM golemui.layouts LIMIT 1" { ... }` | *(remove 3-line block)* |
| YAML fixture in `TestLoadConfig_MissingFields` | `entry_point_query: "SELECT * FROM golemui.layouts LIMIT 1"` | *(remove line)* |
| YAML fixture in `TestLoadConfig_EntryPointViewID_Present` | `entry_point_query: "SELECT * FROM golemui.layouts LIMIT 1"` | *(remove line)* |
| YAML fixture in `TestLoadConfig_EntryPointViewID_Absent` | `entry_point_query: "SELECT * FROM golemui.layouts LIMIT 1"` | *(remove line)* |
| YAML fixture in `TestLoadConfig_LayoutQuery_Present` | `entry_point_query: "SELECT * FROM golemui.layouts LIMIT 1"` | *(remove line)* |
| YAML fixture in `TestLoadConfig_LayoutQuery_Absent` | `entry_point_query: "SELECT * FROM golemui.layouts LIMIT 1"` | *(remove line)* |

**Depends on:** Step 1.
**Estimated diff:** ~15 lines.

---

### Step 4 — Update `cmd/golemui/main.go`

| Location | Before | After |
|---|---|---|
| import block | `"GolemUI/pkg/lua"` | `"GolemUI/pkg/config"` |
| `App` struct field | `Config *lua.BootstrapConfig` | `Config *config.BootstrapConfig` |
| `RunBootstrap` param | `cfg *lua.BootstrapConfig` | `cfg *config.BootstrapConfig` |
| `main()` call | `cfg, err := lua.LoadConfig(configPath)` | `cfg, err := config.LoadConfig(configPath)` |

**Depends on:** Step 2 (package must exist).
**Estimated diff:** 4 lines.

---

### Step 5 — Update `cmd/golemui/main_test.go`

| Location | Before | After |
|---|---|---|
| import block | `"GolemUI/pkg/lua"` | `"GolemUI/pkg/config"` |
| `testConfig()` return | `*lua.BootstrapConfig` | `*config.BootstrapConfig` |
| `testConfig()` body | `lua.BootstrapConfig{...}`, `lua.ConfigConexion{...}` | `config.BootstrapConfig{...}`, `config.ConfigConexion{...}` |
| `loadTestYAML()` return | `*lua.BootstrapConfig` | `*config.BootstrapConfig` |
| `loadTestYAML()` call | `lua.LoadConfig(tmpFile)` | `config.LoadConfig(tmpFile)` |
| `TestRunBootstrap_MissingConfig` | `lua.LoadConfig(...)` | `config.LoadConfig(...)` |
| YAML in `TestRunBootstrap_DatabaseFailure` | `entry_point_query: "SELECT * FROM golemui.layouts LIMIT 1"` | *(remove line)* |
| `TestRunBootstrap_DatabaseFailure` body | `lua.LoadConfig(tmpFile)` | `config.LoadConfig(tmpFile)` |
| YAML in `TestRunBootstrap_InvalidConfigMissingFields` | *(none present)* | *(no change needed)* |
| `TestRunBootstrap_InvalidConfigMissingFields` | `lua.LoadConfig(tmpFile)` | `config.LoadConfig(tmpFile)` |

**Depends on:** Step 2.
**Estimated diff:** ~10 lines.

---

### Step 6 — Remove empty `pkg/lua/` directory

```bash
rmdir pkg/lua
```

**Depends on:** Steps 1–5.
**Estimated diff:** 0 lines.

---

### Step 7 — Verify

```bash
go build ./...
go test ./pkg/config/ ./cmd/golemui/ -v
go vet ./...
grep -rn "pkg/lua" --include="*.go" .   # must return 0 matches
```

**Depends on:** Step 6.
**Estimated diff:** 0 lines.

---

## Migration Steps Table

| ID | Description | Files | Depends On | Est. Lines |
|----|-------------|-------|------------|------------|
| T1 | `git mv` loader files to `pkg/config/` | `pkg/lua/loader.go`, `pkg/lua/loader_test.go` | — | 0 |
| T2 | Rename package + remove `EntryPointQuery` field | `pkg/config/loader.go` | T1 | 2 |
| T3 | Rename package + update import + remove dead YAML/assertions | `pkg/config/loader_test.go` | T1 | ~15 |
| T4 | Update import + rename types and calls | `cmd/golemui/main.go` | T2 | 4 |
| T5 | Update import + rename types + remove `entry_point_query` from fixtures | `cmd/golemui/main_test.go` | T2 | ~10 |
| T6 | Remove empty `pkg/lua/` | `pkg/lua/` | T1–T5 | 0 |
| T7 | Build, test, vet, grep verification | all | T6 | 0 |

---

## Risks

- **None meaningful.** This is a rename + dead-field removal. No runtime behavior changes. The only risk is a missed reference, which `grep + go build` catches.

## Non-Goals

- No new config fields.
- No YAML format changes beyond removing `entry_point_query`.
- No changes to `pkg/db`, `pkg/ui`, `pkg/eventbus`, or any other package.
