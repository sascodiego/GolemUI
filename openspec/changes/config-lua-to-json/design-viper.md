# Design: Migrate Bootstrap Config from Lua to YAML via Viper

**Change:** `config-lua-to-json` (pivoted to Viper/YAML)
**Phase:** Design (Viper revision)
**Date:** 2026-06-06

---

## 1. Architecture Overview

### 1.1 Before (Current — gopher-lua)

```
┌──────────────────┐     ┌─────────────────────────────┐     ┌───────────────┐
│ golemui_driver    │────▶│ pkg/lua/loader.go           │────▶│ BootstrapConfig│
│ .lua              │     │ LoadConfig(path)             │     │ (Go struct)    │
│                   │     │                              │     │               │
│ Lua global table  │     │ 1. os.Stat (file exists?)   │     │ UIDB          │
│ `golemui_driver`  │     │ 2. lua.NewState()           │     │ BusinessDB    │
│ with nested       │     │ 3. L.DoFile(path)           │     │ EntryPoint*   │
│ tables UIDB,      │     │ 4. L.GetGlobal()            │     │ LayoutQuery   │
│ BusinessDB        │     │ 5. Manual table iteration   │     └───────┬───────┘
│                   │     │ 6. Validation checks         │             │
└──────────────────┘     └─────────────────────────────┘             ▼
                                                             cmd/golemui/main.go
                                                             flag package
                                                             -config defaults to
                                                             golemui_driver.lua
                                                             -view defaults to ""
```

### 1.2 After (Target — Viper + YAML + pflag)

```
┌──────────────────────────────────────────────────────────────────────┐
│ Config Precedence (Viper)                                            │
│                                                                      │
│  pflag ──▶ env vars ──▶ YAML file ──▶ defaults                      │
│  (highest)                                    (lowest)               │
│                                                                      │
│  ┌──────────────────┐                                                │
│  │ CLI flags (pflag) │  -config golemui_driver.yaml                  │
│  │                   │  -view my_view                                 │
│  └────────┬─────────┘                                                │
│           │                                                          │
│  ┌────────▼─────────┐                                                │
│  │ Environment vars  │  GOLEMUI_UIDB_HOST                            │
│  │                   │  GOLEMUI_BUSINESSDB_PORT                       │
│  │                   │  GOLEMUI_ENTRYPOINTVIEWID                      │
│  └────────┬─────────┘                                                │
│           │                                                          │
│  ┌────────▼─────────┐     ┌─────────────────────────────┐            │
│  │ golemui_driver    │────▶│ pkg/lua/loader.go           │────────┐  │
│  │ .yaml             │     │ LoadConfig(path)             │        │  │
│  │                   │     │                              │        │  │
│  │ YAML with         │     │ 1. viper.New()               │        │  │
│  │ lowercase keys    │     │ 2. SetConfigFile(path)       │        │  │
│  │ uidb, business_db │     │ 3. ReadInConfig()            │        │  │
│  │ entry_point_*     │     │ 4. Unmarshal(&BootstrapCfg)  │        │  │
│  │                   │     │ 5. validateConexion ×2       │        │  │
│  └──────────────────┘     └─────────────────────────────┘        │  │
│                                                                  │  │
│                                                     ┌────────────▼─▼───┐
│                                                     │ BootstrapConfig   │
│                                                     │ (mapstructure tags│
│                                                     │  NOT json tags)   │
│                                                     │                   │
│                                                     │ UIDB              │
│                                                     │ BusinessDB        │
│                                                     │ EntryPoint*       │
│                                                     │ LayoutQuery       │
│                                                     └──────────────────┘
│
│  cmd/golemui/main.go
│  ┌─────────────────────────────────────────────────────┐
│  │ 1. pflag.Parse()                                    │
│  │ 2. viper.BindPFlags(pflag.CommandLine)              │
│  │ 3. viper.SetEnvPrefix("GOLEMUI")                    │
│  │ 4. viper.SetEnvKeyReplacer("." → "_")               │
│  │ 5. viper.AutomaticEnv()                             │
│  │ 6. cfg ← lua.LoadConfig(configPath)                 │
│  │    (LoadConfig creates its own Viper for file read)  │
│  │ 7. Apply env/flag overrides via viper.GetString()   │
│  │ 8. RunBootstrap(ctx, cfg, ...)                      │
│  └─────────────────────────────────────────────────────┘
```

### 1.3 Key Architectural Changes

| Aspect | Before | After |
|--------|--------|-------|
| Config format | Lua script | YAML file |
| Parser | gopher-lua VM | Viper (`github.com/spf13/viper`) |
| Env var support | None | `GOLEMUI_*` prefix via `AutomaticEnv()` |
| CLI flags | `flag` package | `pflag` package bound to Viper |
| Struct tags | None | `mapstructure:"..."` |
| Override hierarchy | CLI flag → file only | pflag → env → file → defaults |
| YAML key convention | PascalCase (Lua) | `snake_case` (YAML convention) |

The **gopher-lua** dependency remains in `go.mod` (REQ-004). `LoadConfig` no longer imports or calls any `lua` package functions.

---

## 2. Data Model

### 2.1 Structs with `mapstructure` Tags

```go
package lua

type ConfigConexion struct {
    Host     string `mapstructure:"host"`
    Port     int    `mapstructure:"port"`
    Database string `mapstructure:"database"`
    User     string `mapstructure:"user"`
    Password string `mapstructure:"password"`
}

type BootstrapConfig struct {
    UIDB             ConfigConexion `mapstructure:"uidb"`
    BusinessDB       ConfigConexion `mapstructure:"business_db"`
    EntryPointQuery  string         `mapstructure:"entry_point_query"`
    EntryPointViewID string         `mapstructure:"entry_point_view_id"`
    LayoutQuery      string         `mapstructure:"layout_query"`
}
```

**Key design decisions:**

1. **`mapstructure` tags, not `json` tags.** Viper uses `mapstructure` for `Unmarshal`. JSON tags are not used by Viper at all.
2. **`snake_case` YAML keys.** Matches YAML/Go convention. The `mapstructure` tag on each field uses the same snake_case key as the YAML file.
3. **Nested struct flattening for env vars.** Viper's env key replacer (`"." → "_"`) means `uidb.host` becomes `GOLEMUI_UIDB_HOST` automatically when using `AutomaticEnv()` with prefix `GOLEMUI`.
4. **Exported struct fields remain.** Field names (`Host`, `Port`, `Database`, etc.) are unchanged — only the tags change. All downstream consumers (`cfg.UIDB.Host`, `cfg.BusinessDB.Port`) compile identically.

### 2.2 YAML Schema (Canonical File: `golemui_driver.yaml`)

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

**Note:** `entry_point_query` is absent (was absent in the Lua file too) and defaults to `""`.

### 2.3 YAML Rules

| Rule | Detail |
|------|--------|
| Top-level | Must be a YAML mapping |
| `uidb`, `business_db` | Must be YAML mappings; validated by required-field check |
| `entry_point_query` | Optional string, defaults `""` |
| `entry_point_view_id` | Optional string, defaults `""` |
| `layout_query` | Optional string, defaults `""` |
| Extra fields | Ignored silently (forward-compatible) |
| Key casing | `snake_case` throughout |

---

## 3. Algorithm: LoadConfig with Viper

### 3.1 Step-by-Step Pseudocode

```
FUNCTION LoadConfig(path string) → (*BootstrapConfig, error)

  1. CHECK file existence
     stat ← os.Stat(path)
     IF os.IsNotExist(stat.err) THEN
       RETURN nil, fmt.Errorf("config file does not exist: %s", path)
     END IF

  2. CREATE Viper instance (internal, not shared)
     v ← viper.New()
     v.SetConfigFile(path)              // explicit file path with extension
     v.SetConfigType("yaml")            // format hint

  3. READ config file
     err ← v.ReadInConfig()
     IF err ≠ nil THEN
       RETURN nil, fmt.Errorf("failed to read config file: %w", err)
     END IF

  4. UNMARSHAL into BootstrapConfig
     var cfg BootstrapConfig
     err ← v.Unmarshal(&cfg)
     IF err ≠ nil THEN
       RETURN nil, fmt.Errorf("failed to parse config: %w", err)
     END IF

  5. VALIDATE UIDB connection
     err ← validateConexion(cfg.UIDB, "UIDB")
     IF err ≠ nil THEN
       RETURN nil, err
     END IF

  6. VALIDATE BusinessDB connection
     err ← validateConexion(cfg.BusinessDB, "BusinessDB")
     IF err ≠ nil THEN
       RETURN nil, err
     END IF

  7. RETURN
     RETURN &cfg, nil

END FUNCTION
```

### 3.2 Design Decision: Internal Viper Instance

`LoadConfig` creates its own `viper.New()` internally. Rationale:

- **Simple signature.** `LoadConfig(path string) (*BootstrapConfig, error)` stays unchanged. Callers don't need to know about Viper.
- **Testable.** Tests just pass file paths — no Viper setup needed in test code.
- **No global state.** Avoids the global `viper.Get()` singleton which can cause test pollution.
- **Caller env/flag overrides are applied separately** in `main.go` (see §4) after `LoadConfig` returns the file-based config.

### 3.3 Helper: validateConexion

Identical logic to the JSON design, preserved for error-message continuity:

```go
func validateConexion(c ConfigConexion, name string) error {
    if c.Host == "" && c.Port == 0 && c.Database == "" && c.User == "" && c.Password == "" {
        return fmt.Errorf("sub-table %s not found or invalid", name)
    }
    if c.Host == "" || c.Port == 0 || c.Database == "" || c.User == "" {
        return fmt.Errorf("missing required connection fields in %s", name)
    }
    return nil
}
```

### 3.4 What Gets Removed

| Removed Element | Reason |
|---|---|
| `import lua "github.com/yuin/gopher-lua"` | No Lua runtime interaction |
| `import "fmt"` | Only used by Lua helpers (re-check: `fmt.Errorf` still used in validation) |
| `func getStringField(tbl *lua.LTable, key string) string` | Lua-specific helper |
| `func getIntField(tbl *lua.LTable, key string) int` | Lua-specific helper |
| `L := lua.NewState()` / `defer L.Close()` | Lua VM lifecycle |
| `L.DoFile(path)` | Lua file execution |
| `L.GetGlobal("golemui_driver")` | Lua global lookup |
| `tbl.RawGetString(...)` calls | Lua table iteration |
| Inner `parseConexion` closure | Replaced by `validateConexion` |

### 3.5 What Gets Added

| Added Element | Reason |
|---|---|
| `import "github.com/spf13/viper"` | Viper for YAML parsing |
| `mapstructure:"..."` tags on both structs | Viper unmarshaling |
| `func validateConexion(c ConfigConexion, name string) error` | Extracted validation, testable |

### 3.6 Full Reference Implementation

```go
package lua

import (
    "fmt"
    "os"

    "github.com/spf13/viper"
)

type ConfigConexion struct {
    Host     string `mapstructure:"host"`
    Port     int    `mapstructure:"port"`
    Database string `mapstructure:"database"`
    User     string `mapstructure:"user"`
    Password string `mapstructure:"password"`
}

type BootstrapConfig struct {
    UIDB             ConfigConexion `mapstructure:"uidb"`
    BusinessDB       ConfigConexion `mapstructure:"business_db"`
    EntryPointQuery  string         `mapstructure:"entry_point_query"`
    EntryPointViewID string         `mapstructure:"entry_point_view_id"`
    LayoutQuery      string         `mapstructure:"layout_query"`
}

func validateConexion(c ConfigConexion, name string) error {
    if c.Host == "" && c.Port == 0 && c.Database == "" && c.User == "" && c.Password == "" {
        return fmt.Errorf("sub-table %s not found or invalid", name)
    }
    if c.Host == "" || c.Port == 0 || c.Database == "" || c.User == "" {
        return fmt.Errorf("missing required connection fields in %s", name)
    }
    return nil
}

func LoadConfig(path string) (*BootstrapConfig, error) {
    if _, err := os.Stat(path); os.IsNotExist(err) {
        return nil, fmt.Errorf("config file does not exist: %s", path)
    }

    v := viper.New()
    v.SetConfigFile(path)
    v.SetConfigType("yaml")

    if err := v.ReadInConfig(); err != nil {
        return nil, fmt.Errorf("failed to read config file: %w", err)
    }

    var cfg BootstrapConfig
    if err := v.Unmarshal(&cfg); err != nil {
        return nil, fmt.Errorf("failed to parse config: %w", err)
    }

    if err := validateConexion(cfg.UIDB, "UIDB"); err != nil {
        return nil, err
    }
    if err := validateConexion(cfg.BusinessDB, "BusinessDB"); err != nil {
        return nil, err
    }

    return &cfg, nil
}
```

---

## 4. Algorithm: Config Initialization in main.go (pflag + Viper Binding)

### 4.1 Step-by-Step

```
FUNCTION main()

  1. DEFINE pflags
     pflag.String("config", "golemui_driver.yaml", "Path to YAML configuration file")
     pflag.String("view", "", "Override entry point view ID")

  2. PARSE flags
     pflag.Parse()

  3. LOAD config from file (delegates to pkg/lua)
     configPath ← pflag lookup "config" (or viper.GetString("config"))
     cfg, err ← lua.LoadConfig(configPath)
     IF err ≠ nil THEN
       log.Fatalf("Bootstrap error: %v", err)
     END IF

  4. APPLY env/flag overrides via global Viper
     v ← viper.New()
     v.SetEnvPrefix("GOLEMUI")
     v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
     v.AutomaticEnv()
     v.BindPFlag("view", pflag.Lookup("view"))

     // Override entry_point_view_id if env or flag provides it
     viewOverride ← v.GetString("view")
     IF viewOverride ≠ "" THEN
       cfg.EntryPointViewID = viewOverride
     ELSE
       // Check env var GOLEMUI_ENTRY_POINT_VIEW_ID directly
       envViewID ← v.GetString("entry_point_view_id")
       IF envViewID ≠ "" THEN
         cfg.EntryPointViewID = envViewID
       END IF
     END IF

  5. RUN bootstrap
     RunBootstrap(ctx, cfg, true, nil)

END FUNCTION
```

### 4.2 Design Decision: Separate Viper for File vs. Overrides

`LoadConfig` creates its own Viper instance for file reading (isolated, testable). The `main.go` creates a **second** Viper instance for env var and flag overrides. This avoids coupling the file-reading Viper with the global env/flag Viper.

The override flow is:

```
1. LoadConfig reads YAML → populates BootstrapConfig from file
2. main.go checks pflag -view → if set, overrides cfg.EntryPointViewID
3. main.go checks GOLEMUI_ENTRY_POINT_VIEW_ID → if set, overrides cfg.EntryPointViewID
4. pflag has highest precedence, then env, then file
```

### 4.3 Updated main.go Reference (Relevant Sections)

```go
package main

import (
    "context"
    "flag"
    "log"
    "os"
    "strings"

    "github.com/spf13/pflag"
    "github.com/spf13/viper"

    "fyne.io/fyne/v2"
    "fyne.io/fyne/v2/app"
    "GolemUI/pkg/db"
    "GolemUI/pkg/eventbus"
    "GolemUI/pkg/lua"
    "GolemUI/pkg/ui"
)

type App struct {
    Config   *lua.BootstrapConfig
    DB       *db.DB
    EventBus eventbus.EventBus
    FyneApp  fyne.App
    Window   fyne.Window
}

var initDB = db.InitDB

func sanitizeLocale() { /* unchanged */ }

func RunBootstrap(ctx context.Context, cfg *lua.BootstrapConfig, runWindow bool, fyneApp fyne.App) (*App, error) {
    // 0. Sanitize locale
    sanitizeLocale()

    // 1. Convert lua ConfigConexion to db Config
    coreCfg := db.Config{
        Host:     cfg.UIDB.Host,
        Port:     cfg.UIDB.Port,
        Database: cfg.UIDB.Database,
        User:     cfg.UIDB.User,
        Password: cfg.UIDB.Password,
    }
    bizCfg := db.Config{
        Host:     cfg.BusinessDB.Host,
        Port:     cfg.BusinessDB.Port,
        Database: cfg.BusinessDB.Database,
        User:     cfg.BusinessDB.User,
        Password: cfg.BusinessDB.Password,
    }

    // 2. Database pool initialization
    dbPool, err := initDB(ctx, coreCfg, bizCfg)
    if err != nil {
        return nil, fmt.Errorf("failed to initialize database: %w", err)
    }
    ui.BusinessPool = dbPool.BusinessPool
    ui.CorePool = dbPool.CorePool

    // ... rest of bootstrap unchanged (event bus, Fyne, navigation, home screen) ...

    vistaID := cfg.EntryPointViewID
    if vistaID == "" {
        vistaID = "home"
    }

    // ... LoadScreen, Compose, etc. ...
}

func main() {
    // Define pflags
    pflag.String("config", "golemui_driver.yaml", "Path to YAML configuration file")
    pflag.String("view", "", "Override entry point view ID (overrides config entry_point_view_id)")
    pflag.Parse()

    // Importantly: pflag also handles -h/--help
    // Silence stdlib flag to avoid double-parse
    flag.CommandLine.Parse([]string{})

    configPath, _ := pflag.CommandLine.GetString("config")

    // 1. Load config from YAML file
    cfg, err := lua.LoadConfig(configPath)
    if err != nil {
        log.Fatalf("Bootstrap error: %v", err)
    }

    // 2. Apply env/flag overrides via Viper
    v := viper.New()
    v.SetEnvPrefix("GOLEMUI")
    v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
    v.AutomaticEnv()
    v.BindPFlag("view", pflag.Lookup("view"))

    // Flag override for view (highest precedence)
    viewOverride, _ := pflag.CommandLine.GetString("view")
    if viewOverride != "" {
        cfg.EntryPointViewID = viewOverride
    } else {
        // Env var override (e.g. GOLEMUI_ENTRY_POINT_VIEW_ID)
        envViewID := v.GetString("entry_point_view_id")
        if envViewID != "" {
            cfg.EntryPointViewID = envViewID
        }
    }

    ctx := context.Background()
    log.Printf("Starting GolemUI — config: %s, view: %s", configPath, cfg.EntryPointViewID)
    _, err = RunBootstrap(ctx, cfg, true, nil)
    if err != nil {
        log.Fatalf("Bootstrap error: %v", err)
    }
}
```

### 4.4 RunBootstrap Signature Change

The current signature is:

```go
func RunBootstrap(ctx context.Context, configPath string, runWindow bool, fyneApp fyne.App, viewOverride string) (*App, error)
```

After the Viper migration, it becomes:

```go
func RunBootstrap(ctx context.Context, cfg *lua.BootstrapConfig, runWindow bool, fyneApp fyne.App) (*App, error)
```

**Rationale:** Config loading and view override resolution now happen in `main()` before `RunBootstrap` is called. `RunBootstrap` receives a fully-resolved `BootstrapConfig` instead of a path and a view override string. This is cleaner because:

- `RunBootstrap` no longer needs to know about config file paths.
- View override logic is centralized in `main()` alongside flag/env handling.
- `RunBootstrap` becomes a pure bootstrap function, easier to test with a pre-built config struct.

---

## 5. Environment Variable Mapping Table

Viper's env var binding uses the prefix `GOLEMUI_` and replaces `.` with `_` in the key path. With `SetEnvKeyReplacer(strings.NewReplacer(".", "_"))`, nested keys like `uidb.host` become `GOLEMUI_UIDB_HOST`.

| Go Field Path | Viper Key | Env Var Name | Example Value |
|---|---|---|---|
| `cfg.UIDB.Host` | `uidb.host` | `GOLEMUI_UIDB_HOST` | `localhost` |
| `cfg.UIDB.Port` | `uidb.port` | `GOLEMUI_UIDB_PORT` | `5432` |
| `cfg.UIDB.Database` | `uidb.database` | `GOLEMUI_UIDB_DATABASE` | `golemui_core` |
| `cfg.UIDB.User` | `uidb.user` | `GOLEMUI_UIDB_USER` | `golemui_core_engine` |
| `cfg.UIDB.Password` | `uidb.password` | `GOLEMUI_UIDB_PASSWORD` | `secret` |
| `cfg.BusinessDB.Host` | `business_db.host` | `GOLEMUI_BUSINESSDB_HOST` | `localhost` |
| `cfg.BusinessDB.Port` | `business_db.port` | `GOLEMUI_BUSINESSDB_PORT` | `5432` |
| `cfg.BusinessDB.Database` | `business_db.database` | `GOLEMUI_BUSINESSDB_DATABASE` | `negocio_production` |
| `cfg.BusinessDB.User` | `business_db.user` | `GOLEMUI_BUSINESSDB_USER` | `golemui_render_engine` |
| `cfg.BusinessDB.Password` | `business_db.password` | `GOLEMUI_BUSINESSDB_PASSWORD` | `secret` |
| `cfg.EntryPointQuery` | `entry_point_query` | `GOLEMUI_ENTRYPOINTQUERY` | `SELECT ...` |
| `cfg.EntryPointViewID` | `entry_point_view_id` | `GOLEMUI_ENTRYPOINTVIEWID` | `transacciones_list` |
| `cfg.LayoutQuery` | `layout_query` | `GOLEMUI_LAYOUTQUERY` | `SELECT ...` |

**Note on key mapping:** The env key replacer replaces `.` with `_`, but does not affect `snake_case` within key segments. So `entry_point_view_id` (already snake_case) maps to `GOLEMUI_ENTRY_POINT_VIEW_ID`. However, Viper's `AutomaticEnv()` uppercases the key, so the actual env var is `GOLEMUI_ENTRY_POINT_VIEW_ID`.

Wait — correction. Viper's `AutomaticEnv()` transforms the **viper key** (e.g. `entry_point_view_id`) by:
1. Uppercasing it: `ENTRY_POINT_VIEW_ID`
2. Prepending the prefix: `GOLEMUI_ENTRY_POINT_VIEW_ID`
3. The replacer swaps `.` → `_`, but since `entry_point_view_id` has no dots, it stays as-is.

**Corrected table:**

| Go Field Path | Viper Key | Env Var Name | Example |
|---|---|---|---|
| `cfg.UIDB.Host` | `uidb.host` | `GOLEMUI_UIDB_HOST` | `localhost` |
| `cfg.UIDB.Port` | `uidb.port` | `GOLEMUI_UIDB_PORT` | `5432` |
| `cfg.UIDB.Database` | `uidb.database` | `GOLEMUI_UIDB_DATABASE` | `golemui_core` |
| `cfg.UIDB.User` | `uidb.user` | `GOLEMUI_UIDB_USER` | `golemui_core_engine` |
| `cfg.UIDB.Password` | `uidb.password` | `GOLEMUI_UIDB_PASSWORD` | `secret` |
| `cfg.BusinessDB.Host` | `business_db.host` | `GOLEMUI_BUSINESS_DB_HOST` | `localhost` |
| `cfg.BusinessDB.Port` | `business_db.port` | `GOLEMUI_BUSINESS_DB_PORT` | `5432` |
| `cfg.BusinessDB.Database` | `business_db.database` | `GOLEMUI_BUSINESS_DB_DATABASE` | `negocio_production` |
| `cfg.BusinessDB.User` | `business_db.user` | `GOLEMUI_BUSINESS_DB_USER` | `golemui_render_engine` |
| `cfg.BusinessDB.Password` | `business_db.password` | `GOLEMUI_BUSINESS_DB_PASSWORD` | `secret` |
| `cfg.EntryPointQuery` | `entry_point_query` | `GOLEMUI_ENTRY_POINT_QUERY` | `SELECT ...` |
| `cfg.EntryPointViewID` | `entry_point_view_id` | `GOLEMUI_ENTRY_POINT_VIEW_ID` | `transacciones_list` |
| `cfg.LayoutQuery` | `layout_query` | `GOLEMUI_LAYOUT_QUERY` | `SELECT ...` |

---

## 6. Flag Mapping Table

| Flag Name | Type | Default | pflag Key | Viper Key | Effect |
|---|---|---|---|---|---|
| `-config` | string | `golemui_driver.yaml` | `config` | N/A (used directly) | Path to YAML config file; passed to `LoadConfig` |
| `-view` | string | `""` | `view` | `view` | Overrides `EntryPointViewID` when non-empty |

### Precedence Rules

| Priority | Source | Example |
|---|---|---|
| 1 (highest) | pflag `-view` | `-view dashboard` → `cfg.EntryPointViewID = "dashboard"` |
| 2 | env var `GOLEMUI_ENTRY_POINT_VIEW_ID` | `export GOLEMUI_ENTRY_POINT_VIEW_ID=settings` |
| 3 | YAML file `entry_point_view_id` | `entry_point_view_id: "transacciones_list"` |
| 4 (lowest) | Go zero value | `""` if none of the above are set |

For connection fields (host, port, etc.), env vars override the YAML file:

| Priority | Source | Example |
|---|---|---|
| 1 (highest) | env var `GOLEMUI_UIDB_HOST` | `export GOLEMUI_UIDB_HOST=prod-db.example.com` |
| 2 | YAML file `uidb.host` | `host: "localhost"` |

Connection fields have no corresponding pflags (they come from the config file or env vars).

---

## 7. Validation Logic

### 7.1 Validation Checks

| # | Check | Trigger Condition | Error Message |
|---|---|---|---|
| V1 | File existence | `os.IsNotExist(err)` | `"config file does not exist: <path>"` |
| V2 | File readable / valid YAML | `v.ReadInConfig()` returns error | `"failed to read config file: <err>"` |
| V3 | Unmarshal success | `v.Unmarshal()` returns error | `"failed to parse config: <err>"` |
| V4 | UIDB sub-object present | All 5 fields zero-valued | `"sub-table UIDB not found or invalid"` |
| V5 | UIDB required fields | `host=="" \|\| port==0 \|\| database=="" \|\| user==""` | `"missing required connection fields in UIDB"` |
| V6 | BusinessDB sub-object present | All 5 fields zero-valued | `"sub-table BusinessDB not found or invalid"` |
| V7 | BusinessDB required fields | `host=="" \|\| port==0 \|\| database=="" \|\| user==""` | `"missing required connection fields in BusinessDB"` |

### 7.2 Error Message Preservation

| Scenario | Current Lua Error | New Viper Error | Match? |
|---|---|---|---|
| Missing file | `"config file does not exist: <path>"` | `"config file does not exist: <path>"` | Exact |
| Invalid syntax | `"failed to execute Lua config: <err>"` | `"failed to read config file: <err>"` or `"failed to parse config: <err>"` | Adapted |
| Missing/invalid sub-table | `"sub-table X not found or invalid"` | `"sub-table X not found or invalid"` | Exact |
| Missing required fields | `"missing required connection fields in X"` | `"missing required connection fields in X"` | Exact |
| Optional field absent | Returns `""` | Returns `""` (Go zero value) | Exact |

### 7.3 Env Var Validation Note

When env vars override config values, the validation in `LoadConfig` runs **before** env overrides are applied in `main()`. This means:

- `LoadConfig` validates the **file-based** config.
- If the file passes validation but env vars override a required field to empty, that's not caught by `LoadConfig`.
- This is acceptable because env vars are an operator-controlled override. If `GOLEMUI_UIDB_HOST=` is set to empty, the resulting DB connection will fail with a clear connection error — the validation gap is caught downstream.
- If stricter validation is needed, a post-override validation step can be added to `main()`. For now, this is not in scope.

---

## 8. YAML Config File (`golemui_driver.yaml`)

```yaml
# GolemUI Bootstrap Configuration
# This file is loaded by the GolemUI client at startup.
# Environment variables (GOLEMUI_*) override values here.

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

This carries the **exact same values** as the current `golemui_driver.lua`. Note that `entry_point_query` is absent (was absent in the Lua file too) and defaults to `""`.

---

## 9. File Change Specification

### 9.1 `pkg/lua/loader.go`

| Element | Action | Detail |
|---|---|---|
| `import lua "github.com/yuin/gopher-lua"` | **REMOVE** | No Lua runtime calls |
| `import "github.com/spf13/viper"` | **ADD** | Viper for YAML parsing |
| `ConfigConexion` struct tags | **MODIFY** | Replace no-tags with `mapstructure:"host"`, `mapstructure:"port"`, `mapstructure:"database"`, `mapstructure:"user"`, `mapstructure:"password"` |
| `BootstrapConfig` struct tags | **MODIFY** | Replace no-tags with `mapstructure:"uidb"`, `mapstructure:"business_db"`, `mapstructure:"entry_point_query"`, `mapstructure:"entry_point_view_id"`, `mapstructure:"layout_query"` |
| `func getStringField(...)` | **REMOVE** | Lua-specific |
| `func getIntField(...)` | **REMOVE** | Lua-specific |
| `func validateConexion(...)` | **ADD** | Extracted validation logic |
| `func LoadConfig(...)` body | **MODIFY** | Replace Lua VM with Viper: `viper.New()` → `SetConfigFile` → `ReadInConfig` → `Unmarshal` → `validateConexion` |

### 9.2 `pkg/lua/loader_test.go`

| Element | Action | Detail |
|---|---|---|
| All 8 test fixture strings | **MODIFY** | Replace Lua content with equivalent YAML content |
| File extensions in temp files | **MODIFY** | `.lua` → `.yaml` |
| Error assertions | **MODIFY** | Update from Lua-specific error text to YAML/Viper error text |
| Add new edge-case tests | **ADD** | Cover missing sub-objects, null sub-objects, partial fields (see §10) |

### 9.3 `cmd/golemui/main.go`

| Element | Action | Detail |
|---|---|---|
| `import "flag"` | **REMOVE** | Replaced by pflag |
| `import "github.com/spf13/pflag"` | **ADD** | pflag for CLI flags |
| `import "github.com/spf13/viper"` | **ADD** | Viper for env var binding |
| `import "strings"` | **KEEP** | Already imported; now also used for `NewReplacer` |
| `flagConfig` / `flagView` var block | **REMOVE** | Replaced by pflag definitions in `main()` |
| `flag.Parse()` | **REMOVE** | Replaced by `pflag.Parse()` |
| `func RunBootstrap(...)` signature | **MODIFY** | Accept `*lua.BootstrapConfig` instead of `configPath string` + `viewOverride string` |
| `main()` function | **MODIFY** | pflag setup → `LoadConfig` → env/flag overrides → `RunBootstrap` with resolved config |
| Log line | **MODIFY** | Update to show YAML config path |

### 9.4 `cmd/golemui/main_test.go`

| Element | Action | Detail |
|---|---|---|
| All test fixture strings | **MODIFY** | Replace Lua content with YAML content |
| `RunBootstrap` calls | **MODIFY** | Pass `*lua.BootstrapConfig` instead of path + viewOverride; build config struct directly in tests |
| Error assertions | **MODIFY** | Update expected error strings |
| Test names referencing "Lua" | **MODIFY** | Rename to reference "YAML" or "config" |

### 9.5 `golemui_driver.lua` (project root)

| Action | Detail |
|---|---|
| **DELETE** | Replaced by YAML equivalent |

### 9.6 `golemui_driver.yaml` (project root)

| Action | Detail |
|---|---|
| **CREATE** | YAML config with same values as the deleted Lua file (see §8) |

### 9.7 `go.mod`

| Action | Detail |
|---|---|
| **MODIFY** | Add `github.com/spf13/viper` and `github.com/spf13/pflag` as direct dependencies. Keep `github.com/yuin/gopher-lua` (REQ-004). |

Note: Viper likely pulls in several transitive dependencies. After adding the import and running `go mod tidy`, verify the resulting `go.mod` is clean.

---

## 10. Updated Migration Steps

Ordered implementation sequence. Each step is atomic and testable.

### Step 1: Add Viper and pflag dependencies

**Action:** Run `go get github.com/spf13/viper github.com/spf13/pflag`
**Files:** `go.mod`, `go.sum`
**Risk:** Low. Standard dependency addition.
**Verify:** `go build ./...` compiles.

### Step 2: Add `mapstructure` tags to structs

**File:** `pkg/lua/loader.go`
**Action:** Add `mapstructure:"..."` tags to all fields in `ConfigConexion` and `BootstrapConfig`.
**Risk:** None. Struct tags are inert for non-Viper usage.
**Verify:** `go vet ./pkg/lua/` passes; `go build ./pkg/lua/` compiles.

### Step 3: Replace `LoadConfig` implementation

**File:** `pkg/lua/loader.go`
**Action:**
- Remove `import lua "github.com/yuin/gopher-lua"`.
- Add `import "github.com/spf13/viper"`.
- Remove `getStringField` and `getIntField`.
- Rewrite `LoadConfig`: `os.Stat` → `viper.New()` → `SetConfigFile` → `ReadInConfig` → `Unmarshal` → validation.
- Add `validateConexion` helper.
**Risk:** Medium. Core logic change. All existing tests will break until Step 4.
**Verify:** `go build ./pkg/lua/` compiles. No `gopher-lua` import in `loader.go`.

### Step 4: Rewrite loader tests for YAML fixtures

**File:** `pkg/lua/loader_test.go`
**Action:**
- Replace all Lua fixture strings with equivalent YAML.
- Update error assertions to match Viper/validation error messages.
- Add edge-case tests for: missing sub-object, null sub-object, partial fields.
**Risk:** Low. Mechanical fixture swap.
**Verify:** `go test ./pkg/lua/ -v` — all tests pass green.

### Step 5: Update `cmd/golemui/main.go`

**File:** `cmd/golemui/main.go`
**Action:**
- Replace `flag` with `pflag` and `viper`.
- Change `RunBootstrap` signature: accept `*lua.BootstrapConfig` instead of `configPath + viewOverride`.
- Update `main()`: pflag setup → `LoadConfig` → env/flag overrides → `RunBootstrap(cfg, ...)`.
- Update log line and comments.
**Risk:** Medium. Signature change affects all callers and tests.
**Verify:** `go build ./cmd/golemui/` compiles.

### Step 6: Rewrite bootstrap tests

**File:** `cmd/golemui/main_test.go`
**Action:**
- Update all `RunBootstrap` calls to pass `*lua.BootstrapConfig` built directly.
- Replace Lua fixture strings with YAML.
- Update error assertions.
- Rename test functions referencing "Lua".
**Risk:** Medium. 10+ tests, signature change.
**Verify:** `go test ./cmd/golemui/ -v -run TestRunBootstrap` — all tests pass green.

### Step 7: Swap config file

**Files:** `golemui_driver.yaml` (create), `golemui_driver.lua` (delete)
**Action:**
- Create `golemui_driver.yaml` with values matching the current Lua file (see §8).
- Delete `golemui_driver.lua`.
**Risk:** Low.
**Verify:** `LoadConfig("golemui_driver.yaml")` succeeds with expected values. `test -f golemui_driver.lua` returns non-zero.

### Step 8: Final verification

**Action:**
- `go test ./pkg/lua/ ./cmd/golemui/ -v` — all pass.
- `go vet ./...` — clean.
- `grep 'gopher-lua' pkg/lua/loader.go` — returns exit 1 (not found).
- `grep 'github.com/yuin/gopher-lua' go.mod` — returns exit 0 (still present).
- `grep 'github.com/spf13/viper' go.mod` — returns exit 0 (added).
- `grep 'github.com/spf13/pflag' go.mod` — returns exit 0 (added).
- Manual test: set `GOLEMUI_UIDB_HOST=override` env var, run binary, confirm override works.

---

## Appendix A: Dependency Graph

```
Step 1 (deps) ──→ Step 2 (tags) ──→ Step 3 (LoadConfig) ──→ Step 4 (loader tests)
                                           │
                                           ├──→ Step 5 (main.go) ──→ Step 6 (bootstrap tests)
                                           │
                                           └──→ Step 7 (config file swap)

Step 4 + Step 6 + Step 7 ──→ Step 8 (final verification)
```

**Recommended execution order:** 1 → 2 → 3 → 4 + 5 (parallel) → 6 → 7 → 8

## Appendix B: New `go.mod` Dependencies

| Dependency | Version | Purpose |
|---|---|---|
| `github.com/spf13/viper` | latest | YAML config reading, env var binding, unmarshal |
| `github.com/spf13/pflag` | latest | POSIX/GNU-style CLI flags, Viper integration |

Both are well-maintained, widely-used Go packages with minimal transitive dependency surface.

**Note:** `gopkg.in/yaml.v3` is already an indirect dependency in `go.mod` (pulled by Fyne). Viper will use it for YAML parsing. No additional YAML library is needed.

## Appendix C: Backward Compatibility Notes

1. **No Lua config supported.** After this change, the binary only reads YAML. Users with existing `.lua` files must convert them to `.yaml`.
2. **Flag default changes.** `-config` now defaults to `golemui_driver.yaml`. Users who relied on the default must rename their config file.
3. **New env vars.** `GOLEMUI_*` env vars are now respected. This is additive — existing deployments without these env vars are unaffected.
4. **`flag` → `pflag`.** The `-config` and `-view` flags behave identically. pflag is a superset of `flag` and supports `--config` (double-dash) as well as `-config`.
