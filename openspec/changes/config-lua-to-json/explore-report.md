# GolemUI Lua-to-JSON Config Migration: Codebase Scout Report

## Executive Summary

The current config bootstrap uses gopher-lua v1.1.2 to execute `golemui_driver.lua` at startup, lift the global `golemui_driver` table, and map it into a Go `BootstrapConfig` struct. The migration to JSON requires replacing the Lua VM with `encoding/json`, renaming the config file, and updating all tests that synthesize Lua temp files.

---

## 1. `pkg/lua/loader.go` — Full Analysis

**Path:** `file:///src/GolemUI/pkg/lua/loader.go`

### Exported Types

```go
// loader.go:9-14
type ConfigConexion struct {
    Host     string
    Port     int
    Database string
    User     string
    Password string
}

// loader.go:16-23
type BootstrapConfig struct {
    UIDB             ConfigConexion
    BusinessDB       ConfigConexion
    EntryPointQuery  string
    EntryPointViewID string
    LayoutQuery      string
}
```

### Exported Functions

| Function | Signature | Location |
|---|---|---|
| `LoadConfig` | `func LoadConfig(path string) (*BootstrapConfig, error)` | loader.go:49 |

### Unexported Helpers

| Function | Signature | Location |
|---|---|---|
| `getStringField` | `func getStringField(tbl *lua.LTable, key string) string` | loader.go:25 |
| `getIntField` | `func getIntField(tbl *lua.LTable, key string) int` | loader.go:33 |

### Lua VM Lifecycle

```
lua.NewState()  ← loader.go:54
    ↓
L.DoFile(path)  ← loader.go:58 (executes golemui_driver.lua)
    ↓
L.GetGlobal("golemui_driver")  ← loader.go:62 (expects LTTable)
    ↓
tbl.RawGetString("UIDB") → parseConexion()  ← loader.go:72-97
tbl.RawGetString("BusinessDB") → parseConexion()  ← loader.go:99-101
tbl.RawGetString("EntryPointQuery") → getStringField()  ← loader.go:103
tbl.RawGetString("EntryPointViewID") → getStringField()  ← loader.go:104
tbl.RawGetString("LayoutQuery") → getStringField()  ← loader.go:105
    ↓
defer L.Close()  ← loader.go:55 (deferred after NewState)
```

### gopher-lua Dependency Surface

| Symbol | Package | Usage |
|---|---|---|
| `lua.NewState()` | `github.com/yuin/gopher-lua` | loader.go:54 — creates Lua VM state |
| `L.DoFile()` | `github.com/yuin/gopher-lua` | loader.go:58 — executes .lua script |
| `lua.LNil` | `github.com/yuin/gopher-lua` | loader.go:27, 35 — sentinel for missing key |
| `lua.LNumber` | `github.com/yuin/gopher-lua` | loader.go:38 — type assertion for port |
| `lua.LTTable` | `github.com/yuin/gopher-lua` | loader.go:62, 69 — type check constant |
| `lua.LTable` | `github.com/yuin/gopher-lua` | loader.go:25, 33, 65 — receiver type for helpers |
| `L.GetGlobal()` | `github.com/yuin/gopher-lua` | loader.go:62 — read global table |
| `L.Close()` | `github.com/yuin/gopher-lua` | loader.go:55 — VM teardown |

Import alias used: `lua "github.com/yuin/gopher-lua"` (loader.go:6).

### What Needs to Change for JSON Migration

- **Remove** import `"github.com/yuin/gopher-lua"` and alias `lua`.
- **Replace** `lua.NewState()` / `L.DoFile()` / `L.GetGlobal()` / `defer L.Close()` with `os.Open` + `json.NewDecoder`.
- **Replace** `getStringField`/`getIntField` helpers with `json:"..."` struct tags + direct field access.
- **Rename** the file (or create a new one): either `pkg/config/loader.go` or keep `pkg/lua/loader.go` but strip Lua references. The proposal spec (`docs/specify/004-migrate-bootstrap-config-to-json.md`) keeps `pkg/lua/loader.go` but changes its internals.
- **Keep** the exported type signatures (`BootstrapConfig`, `ConfigConexion`, `LoadConfig`) unchanged to preserve the `RunBootstrap` integration contract.
- **Validation logic** (required fields check in `parseConexion`) must be preserved.

---

## 2. `cmd/golemui/main.go` — Config Flow Analysis

**Path:** `file:///src/GolemUI/cmd/golemui/main.go`

### Key Struct

```go
// main.go:20
type App struct {
    Config   *lua.BootstrapConfig
    DB       *db.DB
    EventBus eventbus.EventBus
    FyneApp  fyne.App
    Window   fyne.Window
}
```

### CLI Flags

```go
// main.go:30-31
var (
    flagConfig = flag.String("config", "golemui_driver.lua", "Path to Lua configuration file")
    flagView   = flag.String("view", "", "Override entry point view ID (overrides Lua config EntryPointViewID)")
)
```

### `RunBootstrap` Config Flow

```
flagConfig (default: "golemui_driver.lua")
    ↓
lua.LoadConfig(configPath)  ← main.go:58 — returns *lua.BootstrapConfig
    ↓
db.Config{ Host, Port, Database, User, Password } × 2  ← main.go:64-71
    (maps cfg.UIDB and cfg.BusinessDB into pkg/db.Config)
    ↓
initDB(ctx, coreCfg, bizCfg)  ← main.go:74
    ↓
ui.BusinessPool = dbPool.BusinessPool  ← main.go:76
ui.CorePool = dbPool.CorePool           ← main.go:77
    ↓
cfg.LayoutQuery → ui.LoadScreen(..., cfg.LayoutQuery)  ← main.go:87 (home), main.go:107 (navigation)
    ↓
cfg.EntryPointViewID or viewOverride → vistaID → ui.LoadScreen  ← main.go:89-93
    ↓
ui.Compose(node, vistaID) → win.SetContent  ← main.go:96
```

### `viewOverride` Parameter

`RunBootstrap` receives `viewOverride string` (main.go:43). Resolution order:
1. If `viewOverride != ""`, use it (CLI `-view` flag wins).
2. Else if `cfg.EntryPointViewID != ""`, use it.
3. Else default to `"home"`.

This logic is in main.go:89-93 and is **independent of the config file format** — no Lua coupling here.

### What Needs to Change for JSON Migration

- Change default of `flagConfig` from `"golemui_driver.lua"` to `"golemui_driver.json"`.
- Update flag description from `"Path to Lua configuration file"` to `"Path to JSON configuration file"`.
- The `lua.` import alias and `lua.LoadConfig` call remain unchanged in signature — only the `pkg/lua/loader.go` internals change.
- No changes needed to the App struct, db.Config mapping, ui wiring, or viewOverride logic.

---

## 3. `cmd/golemui/main_test.go` — Test Catalog

**Path:** `file:///src/GolemUI/cmd/golemui/main_test.go`

### All Test Functions (16 total)

#### Locale Sanitization Tests (7) — No Lua files involved
| Test | Scenario | Temp File |
|---|---|---|
| `TestSanitizeLocale_LangC` | LANG=C, LC_ALL="" | None |
| `TestSanitizeLocale_LCAllPOSIX` | LANG="", LC_ALL=POSIX | None |
| `TestSanitizeLocale_BothEmpty` | LANG="", LC_ALL="" | None |
| `TestSanitizeLocale_ValidLangUntouched` | LANG=es_AR.UTF-8, LC_ALL="" | None |
| `TestSanitizeLocale_LCAllValid` | LANG="", LC_ALL=en_US.UTF-8 | None |
| `TestSanitizeLocale_LCAllCOverridesValidLang` | LANG=es_AR.UTF-8, LC_ALL=C | None |
| `TestSanitizeLocale_BothValidUntouched` | LANG=es_AR.UTF-8, LC_ALL=es_AR.UTF-8 | None |

#### Bootstrap Tests (9) — All use Lua temp files
| Test | Temp File | Lua Content Summary |
|---|---|---|
| `TestRunBootstrap_MissingConfig` | None (passes path to non-existent file) | N/A |
| `TestRunBootstrap_DatabaseFailure` | `golemui_driver.lua` | Full config with unreachable hosts |
| `TestRunBootstrap_InvalidLuaConfigTable` | `golemui_driver_invalid.lua` | Has `some_other_driver` table, missing `golemui_driver` |
| `TestRunBootstrap_Success` | `golemui_driver_success.lua` | Full valid config, EntryPointViewID="home" |
| `TestRunBootstrap_DefaultVistaID` | `golemui_driver_default_vista.lua` | Config without EntryPointViewID |
| `TestRunBootstrap_LoadScreenFailure` | `golemui_driver_failure.lua` | Valid config, mocked to return pgx.ErrNoRows |
| `TestRunBootstrap_ViewOverrideWins` | `golemui_driver_override.lua` | EntryPointViewID="dashboard" (viewOverride="settings" wins) |
| `TestRunBootstrap_EmptyOverrideFallsThrough` | `golemui_driver_config_vista.lua` | EntryPointViewID="transacciones_list", override="" |
| `TestRunBootstrap_BothEmptyDefaultsHome` | `golemui_driver_no_vista.lua` | Config without EntryPointViewID, override="" |
| `TestRunBootstrap_IntegrationWithLogs` | `golemui_driver_integration.lua` | Full config, EntryPointViewID="transacciones_list" |

### What Needs to Change for JSON Migration

- All 9 bootstrap tests that write `.lua` files must be rewritten to write `.json` files with equivalent content.
- `TestRunBootstrap_InvalidLuaConfigTable` should be renamed/replaced with a JSON equivalent (e.g., JSON syntax error or missing `golemui_driver` key).
- The 7 locale tests are unaffected.
- Test helper logic (`coreMock.RegisterQuery`, `initDB` mock, etc.) remains unchanged.
- The test package name is `main` and imports `"GolemUI/pkg/lua"` only via `RunBootstrap`.

---

## 4. `golemui_driver.lua` — Current Config File

**Path:** `file:///src/GolemUI/golemui_driver.lua`

```lua
golemui_driver = {
    UIDB = {
        Host = "localhost",
        Port = 5432,
        Database = "golemui_core",
        User = "golemui_core_engine",
        Password = "<REDACTED>"
    },
    BusinessDB = {
        Host = "localhost",
        Port = 5432,
        Database = "negocio_production",
        User = "golemui_render_engine",
        Password = "<REDACTED>"
    },
    EntryPointViewID = "transacciones_list",
    LayoutQuery = "SELECT config_columnas FROM golemui.vistas_consulta WHERE id = $1"
}
```

### Field Mapping to JSON

| Lua Key | Go Field | Type |
|---|---|---|
| `UIDB.Host` | `BootstrapConfig.UIDB.Host` | string |
| `UIDB.Port` | `BootstrapConfig.UIDB.Port` | int |
| `UIDB.Database` | `BootstrapConfig.UIDB.Database` | string |
| `UIDB.User` | `BootstrapConfig.UIDB.User` | string |
| `UIDB.Password` | `BootstrapConfig.UIDB.Password` | string |
| `BusinessDB.*` | `BootstrapConfig.BusinessDB.*` | same |
| `EntryPointQuery` | `BootstrapConfig.EntryPointQuery` | string (currently unused in Lua file, always empty) |
| `EntryPointViewID` | `BootstrapConfig.EntryPointViewID` | string |
| `LayoutQuery` | `BootstrapConfig.LayoutQuery` | string |

### What Needs to Change for JSON Migration

- Create `golemui_driver.json` with equivalent structure.
- Add `EntryPointQuery` field to JSON (currently absent in `.lua` file but defined in struct).
- Remove or archive `golemui_driver.lua` (or keep it for rollback).

---

## 5. `pkg/lua/loader_test.go` — Test Catalog

**Path:** `file:///src/GolemUI/pkg/lua/loader_test.go`

### All Test Functions (8 total)

| Test | Scenario | Temp File |
|---|---|---|
| `TestLoadConfig_MissingFile` | Non-existent file path | None |
| `TestLoadConfig_Success` | Full valid config, all fields | `golemui_driver_test.lua` |
| `TestLoadConfig_InvalidSyntax` | Lua syntax error (missing brace) | `golemui_driver_invalid.lua` |
| `TestLoadConfig_MissingFields` | UIDB missing "Host" field | `golemui_driver_missing.lua` |
| `TestLoadConfig_EntryPointViewID_Present` | EntryPointViewID="dashboard" | `golemui_driver_viewid.lua` |
| `TestLoadConfig_EntryPointViewID_Absent` | No EntryPointViewID key | `golemui_driver_no_viewid.lua` |
| `TestLoadConfig_LayoutQuery_Present` | LayoutQuery="SELECT col FROM tbl..." | `golemui_driver_layout_query.lua` |
| `TestLoadConfig_LayoutQuery_Absent` | No LayoutQuery key | `golemui_driver_no_layout_query.lua` |

### What Needs to Change for JSON Migration

- All 8 tests write `.lua` temp files → rewrite as `.json`.
- `TestLoadConfig_InvalidSyntax` → rename/replace with JSON parse error scenario (malformed JSON or wrong type).
- `TestLoadConfig_MissingFields` → rename to `InvalidJSONMissingFields` or similar, test with JSON missing required keys.
- `TestLoadConfig_Success` → JSON with all fields populated.
- The test file stays in `pkg/lua/loader_test.go` (package `lua_test`); no rename needed.
- The `lua.LoadConfig` call signature is unchanged, so test assertions remain identical.

---

## 6. `go.mod` — Dependency Confirmation

**Path:** `file:///src/GolemUI/go.mod`

```
github.com/yuin/gopher-lua v1.1.2
```

This is the **only** gopher-lua dependency in the project. All other imports are `fyne.io`, `github.com/jackc/pgx`, and stdlib.

### What Needs to Change for JSON Migration

**Keep** the dependency. Per the proposal spec:
> Mantener la biblioteca de scripting de Lua y su entorno intactos en el repositorio y dependencias (`go.mod`) para permitir ejecuciones de scripts dinámicos en runtime en futuras fases del sistema.

The dependency stays in `go.mod`. Only the import inside `pkg/lua/loader.go` is removed (or the file is refactored so that file no longer imports gopher-lua).

---

## 7. Other Files Referencing pkg/lua or LoadConfig

### Direct Importers

| File | Import | Usage |
|---|---|---|
| `cmd/golemui/main.go` | `"GolemUI/pkg/lua"` | `lua.LoadConfig()`, `lua.BootstrapConfig` |
| `cmd/golemui/main_test.go` | `"GolemUI/pkg/lua"` | Indirect via `RunBootstrap` (no direct calls) |

### References in OpenSpec / Docs (no code impact)

| File | Notes |
|---|---|
| `openspec/changes/layout-lua-decoupling-locale-fix/*` | Archived SDD artifacts referencing Lua loader |
| `openspec/changes/screen-loading-db/*` | Archived SDD referencing BootstrapConfig |
| `openspec/changes/config-lua-to-json/explore-report.md` | **This report** |
| `docs/specify/004-migrate-bootstrap-config-to-json.md` | Proposal spec (Spanish) — defines acceptance criteria |
| `docs/specify/archived/002-dynamic-layout-query-and-locale-fix.md` | Archived |
| `archive/` subdirectories | Old SDD phases, read-only |

### UI Package Surface (no pkg/lua dependency)

`pkg/ui/screen_loader.go` defines `DefaultLayoutQuery` constant (line 12):
```go
const DefaultLayoutQuery = "SELECT config_columnas FROM golemui.vistas_consulta WHERE id = $1"
```
This is the fallback `layoutQuery` used when `cfg.LayoutQuery` is empty. No Lua involvement.

---

## Architecture: Config Data Flow

```
CLI flag -config (default: golemui_driver.lua)
        │
        ▼
lua.LoadConfig(path) ──→ *lua.BootstrapConfig
    │                       ├─ UIDB: ConfigConexion
    │                       ├─ BusinessDB: ConfigConexion
    │                       ├─ EntryPointQuery: string
    │                       ├─ EntryPointViewID: string
    │                       └─ LayoutQuery: string
    │
    ▼
map to db.Config × 2
    │
    ▼
initDB(ctx, coreCfg, bizCfg) ──→ *db.DB
    │                              ├─ CorePool
    │                              └─ BusinessPool
    │
    ▼
ui.LoadScreen(ctx, CorePool, vistaID, cfg.LayoutQuery)
    │
    ▼
ui.Compose(node, vistaID) ──→ fyne widget tree
```

The data contract between `LoadConfig` and `RunBootstrap` is the `BootstrapConfig` struct. **This contract must not change.** Only the serialization format (Lua → JSON) changes.

---

## Migration Impact Summary

| File | Change Type | Scope |
|---|---|---|
| `pkg/lua/loader.go` | Rewrite internals | Remove gopher-lua VM, replace with `encoding/json` |
| `cmd/golemui/main.go` | Flag default + description | `"golemui_driver.lua"` → `"golemui_driver.json"` |
| `cmd/golemui/main_test.go` | 9 tests rewrite | All Lua temp files → JSON temp files |
| `pkg/lua/loader_test.go` | 8 tests rewrite | All Lua temp files → JSON temp files |
| `golemui_driver.lua` | Create new | New `golemui_driver.json` equivalent |
| `go.mod` | No change | Keep `gopher-lua` for future use |
| `pkg/ui/screen_loader.go` | No change | Unaffected |
| `pkg/db/db.go` | No change | `db.Config` struct unchanged |

### Breaking Change Risk

**Low.** The public API surface is:
- `LoadConfig(path string) (*BootstrapConfig, error)` — signature unchanged.
- `BootstrapConfig` and `ConfigConexion` structs — field names and types unchanged.

The JSON migration is a drop-in replacement at the serialization layer. The `pkg/lua` directory name may feel misleading post-migration; renaming to `pkg/config` is an optional polish step.

### Key Acceptance Criteria (from proposal)

1. All tests in `cmd/golemui/main_test.go` pass with JSON temp files.
2. `LoadConfig` does not call any `github.com/yuin/gopher-lua` symbol.
3. `golemui_driver.json` at project root starts the client correctly.

---

## Start Here

**Primary file for implementation:** `file:///src/GolemUI/pkg/lua/loader.go`

This is the single file where the Lua VM is instantiated and all gopher-lua symbols live. Start by:
1. Replacing the `gopher-lua` import block with `encoding/json` and `io`.
2. Implementing JSON unmarshal over `os.ReadFile`/`json.Unmarshal`.
3. Preserving field names and validation logic exactly.
4. Keeping `BootstrapConfig`, `ConfigConexion`, and `LoadConfig` signatures unchanged.

**Then update tests** in `pkg/lua/loader_test.go` and `cmd/golemui/main_test.go` to synthesize `.json` temp files.