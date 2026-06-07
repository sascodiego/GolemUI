# Verify Report: Config Lua → YAML/Viper Migration

**Change:** `config-lua-to-json` (pivoted to Viper+YAML+pflag)  
**Phase:** Verify  
**Date:** 2026-06-06  
**Reviewer:** Fresh adversarial review (no prior context)

---

## Summary

The migration from gopher-lua config loading to Viper+YAML+pflag is **correct and complete**. All tests pass. No critical or major issues found. A handful of minor observations and nits are noted below.

**Verdict: PASS** (with minor nits — none blocking)

---

## Files Reviewed

| File | Action |
|------|--------|
| `pkg/lua/loader.go` | Rewrite: gopher-lua → Viper |
| `pkg/lua/loader_test.go` | Rewrite: 8 tests with YAML fixtures |
| `cmd/golemui/main.go` | Rewrite: `flag` → `pflag`+`viper`, `RunBootstrap` signature |
| `cmd/golemui/main_test.go` | Rewrite: struct-based config, mock DB helpers |
| `golemui_driver.yaml` | Created |
| `golemui_driver.lua` | Deleted |
| `go.mod` / `go.sum` | Added viper, pflag; kept gopher-lua |

---

## Build & Test Evidence

```
$ go vet ./pkg/lua/ ./cmd/golemui/   → PASS (no output)
$ go build ./...                       → PASS (no output)
$ go test ./pkg/lua/ -v               → 8/8 PASS
$ go test ./cmd/golemui/ -v           → 17/17 PASS (7 sanitize + 10 bootstrap)
```

---

## Checklist Results

### 1. Correctness — LoadConfig uses Viper, no gopher-lua imports ✅

- `pkg/lua/loader.go` imports only `fmt`, `os`, and `github.com/spf13/viper`.
- `grep 'gopher-lua' pkg/lua/loader.go` returns exit code 1 (not found).
- `grep 'gopher-lua' cmd/golemui/main.go` returns exit code 1 (not found).
- The `LoadConfig` function creates its own isolated `viper.New()`, sets config type to `"yaml"`, reads and unmarshals. Clean implementation.

### 2. Struct tags — `mapstructure` tags correct ✅

| Field | Tag | YAML key | Match? |
|-------|-----|----------|--------|
| `UIDB` | `mapstructure:"uidb"` | `uidb:` | ✅ |
| `BusinessDB` | `mapstructure:"business_db"` | `business_db:` | ✅ |
| `EntryPointQuery` | `mapstructure:"entry_point_query"` | `entry_point_query:` | ✅ |
| `EntryPointViewID` | `mapstructure:"entry_point_view_id"` | `entry_point_view_id:` | ✅ |
| `LayoutQuery` | `mapstructure:"layout_query"` | `layout_query:` | ✅ |
| `Host` | `mapstructure:"host"` | `host:` | ✅ |
| `Port` | `mapstructure:"port"` | `port:` | ✅ |
| `Database` | `mapstructure:"database"` | `database:` | ✅ |
| `User` | `mapstructure:"user"` | `user:` | ✅ |
| `Password` | `mapstructure:"password"` | `password:` | ✅ |

### 3. Validation — `validateConexion` correct ✅

Two-stage check:
1. **All-zero detection**: If all five fields are zero (`Host=="" && Port==0 && Database=="" && User=="" && Password==""`), returns `"sub-table %s not found or invalid"`. This correctly handles the case where Viper unmarshals a missing sub-object to a zero struct.
2. **Partial-field check**: If any required field (`Host`, `Port`, `Database`, `User`) is missing, returns `"missing required connection fields in %s"`.

Edge cases covered:
- All-zero struct → caught by stage 1 ✅
- Partial fields → caught by stage 2 ✅
- Fully valid struct → passes ✅

**Note:** `Password` is not checked in the partial-field test. An empty password with valid host/port/database/user would pass validation. This is arguably correct (some DBs allow empty passwords locally) but worth documenting.

### 4. RunBootstrap signature — accepts `*lua.BootstrapConfig` ✅

```go
func RunBootstrap(ctx context.Context, cfg *lua.BootstrapConfig, runWindow bool, fyneApp fyne.App) (*App, error)
```

- Accepts `*lua.BootstrapConfig` directly — no path string or view override parameter.
- View resolution is handled entirely in `main()` before calling `RunBootstrap`.
- `RunBootstrap` reads `cfg.EntryPointViewID` and `cfg.LayoutQuery` for screen loading.

### 5. main.go — pflag + Viper setup ✅

- `pflag.String("config", "golemui_driver.yaml", ...)` — correct default.
- `pflag.String("view", "", ...)` — correct empty default.
- Viper instance created for env resolution only: `SetEnvPrefix("GOLEMUI")`, `SetEnvKeyReplacer`, `AutomaticEnv()`.
- View override precedence: `pflag > env (GOLEMUI_ENTRY_POINT_VIEW_ID) > config file` — implemented correctly.

**Minor divergence from design spec:** The design calls for `v.BindPFlag("view", pflag.Lookup("view"))` and reading via `v.GetString("view")`. The implementation reads `pflag.CommandLine.GetString("view")` directly and only uses Viper for the env fallback. This is functionally equivalent and arguably simpler. Not a defect.

### 6. Tests — correct and comprehensive ✅

**`pkg/lua/loader_test.go`** (8 tests):
- `TestLoadConfig_MissingFile` — non-existent file → error ✅
- `TestLoadConfig_Success` — full config loaded, all fields asserted ✅
- `TestLoadConfig_InvalidSyntax` — malformed YAML → error ✅
- `TestLoadConfig_MissingFields` — missing `host` in UIDB → error ✅
- `TestLoadConfig_EntryPointViewID_Present` — optional field parsed ✅
- `TestLoadConfig_EntryPointViewID_Absent` — defaults to `""` ✅
- `TestLoadConfig_LayoutQuery_Present` — optional field parsed ✅
- `TestLoadConfig_LayoutQuery_Absent` — defaults to `""` ✅

**`cmd/golemui/main_test.go`** (10 bootstrap + 7 locale):
- Uses `testConfig()` helper building structs directly — correct.
- Uses `setupMockDB()` helper with `MockDBPool` — clean injection.
- `TestRunBootstrap_Success` — full happy path ✅
- `TestRunBootstrap_DefaultVistaID` — empty → `"home"` default ✅
- `TestRunBootstrap_LoadScreenFailure` — DB returns no rows → error ✅
- `TestRunBootstrap_ViewOverrideWins` — view set to `"settings"` ✅
- `TestRunBootstrap_EmptyOverrideFallsThrough` — config value used ✅
- `TestRunBootstrap_BothEmptyDefaultsHome` — both empty → `"home"` ✅
- `TestRunBootstrap_IntegrationWithLogs` — complex grid layout ✅
- `TestRunBootstrap_DatabaseFailure` — real initDB with unreachable host → error ✅

### 7. Config file — golemui_driver.yaml matches old .lua ✅

**Old `golemui_driver.lua`:**
```lua
golemui_driver = {
    UIDB = {
        Host = "localhost", Port = 5432,
        Database = "golemui_core", User = "golemui_core_engine",
        Password = "secret_password_for_ui"
    },
    BusinessDB = {
        Host = "localhost", Port = 5432,
        Database = "negocio_production", User = "golemui_render_engine",
        Password = "secret_password_for_business"
    },
    EntryPointViewID = "transacciones_list",
    LayoutQuery = "SELECT config_columnas FROM golemui.vistas_consulta WHERE id = $1"
}
```

**New `golemui_driver.yaml`:**
```yaml
uidb:
  host: "localhost"
  port: 5432
  database: "golemui_core"
  user: "golemui_core_engine"
  password: "secret_password_for_ui"
business_db:
  host: "localhost"
  port: 5432
  database: "negocio_production"
  user: "golemui_render_engine"
  password: "secret_password_for_business"
entry_point_view_id: "transacciones_list"
layout_query: "SELECT config_columnas FROM golemui.vistas_consulta WHERE id = $1"
```

All values match exactly. `EntryPointQuery` is absent in both (pre-existing state).

### 8. go.mod — dependencies correct ✅

```
github.com/spf13/pflag v1.0.10     → ADDED ✅
github.com/spf13/viper v1.21.0     → ADDED ✅
github.com/yuin/gopher-lua v1.1.2  → KEPT (REQ-004) ✅
```

### 9. Security — hardcoded secrets ✅ (pre-existing, not a regression)

The file `golemui_driver.yaml` contains `password: "secret_password_for_ui"` and `password: "secret_password_for_business"`. This is a **pre-existing issue** — the old `golemui_driver.lua` contained identical values. The migration preserves the same state. Not a regression introduced by this change.

**Recommendation (out of scope):** Move to env vars or a secrets manager in a future change. `GOLEMUI_UIDB_PASSWORD` and `GOLEMUI_BUSINESSDB_PASSWORD` env var support already works via Viper `AutomaticEnv()`, but `LoadConfig` would need env-override logic for full credential externalization.

### 10. Naming — leftover "Lua" references ⚠️ Nit

The following "Lua" references persist but are **intentional and acceptable** per the spec:

| Location | Reference | Assessment |
|----------|-----------|------------|
| Package name `pkg/lua/` | Package path | Intentional — spec says "package name stays unchanged" |
| Import `lua.LoadConfig` | Import alias | Intentional — package name preserved |
| Import `lua.BootstrapConfig` | Type reference | Intentional — types live in `pkg/lua` |
| `ConfigConexion` struct name | "Conexion" (Spanish) | Pre-existing naming convention |

No misleading references found. The only "Lua" in production code is the package path, which the spec explicitly preserves.

---

## Issues Found

### Critical — None

### Major — None

### Minor

| # | Severity | File | Description |
|---|----------|------|-------------|
| M1 | Minor | `pkg/lua/loader.go:26-32` | `validateConexion` does not check `Password`. An empty password with otherwise-valid fields passes validation. This may be intentional (local dev with trust auth) but is not documented. |
| M2 | Minor | `pkg/lua/loader.go:21` | `EntryPointQuery` field exists in the struct and is tested, but is never consumed in `main.go` or `RunBootstrap`. It is carried forward from before the migration and has no production caller. Not harmful, but dead code. |

### Nits

| # | Severity | File | Description |
|---|----------|------|-------------|
| N1 | Nit | `cmd/golemui/main.go:153-155` | Viper env instance is created after `LoadConfig`. The design spec suggested `BindPFlag` for the "view" flag; the implementation reads pflag directly instead. Functionally equivalent but diverges slightly from the design document. |
| N2 | Nit | `golemui_driver.yaml` | Pre-existing hardcoded passwords. Not introduced by this change but carried forward. Future opportunity: externalize via `GOLEMUI_UIDB_PASSWORD` env var. |

---

## Verdict

**PASS.** The migration is clean, complete, and well-tested. No critical or major issues. The two minor items (password validation gap, orphan `EntryPointQuery` field) are pre-existing characteristics of the codebase, not regressions. All tests pass, `go vet` is clean, and the config file values match the old Lua file exactly.
