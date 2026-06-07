# Design: Layout Query Decoupling & Locale Fix

## Technical Approach

Two independent changes with zero overlap: (1) externalize the hardcoded SQL in `LoadScreen` into a `BootstrapConfig.LayoutQuery` field parsed from Lua, with a `DefaultLayoutQuery` fallback constant; (2) add a `sanitizeLocale()` function called before `app.New()` to prevent Fyne's `Error parsing user locale C`. Both follow existing patterns (`EntryPointViewID` for the Lua field, `t.Setenv` for env tests).

## Architecture Decisions

| Decision | Choice | Rejected | Rationale |
|----------|--------|----------|-----------|
| LayoutQuery storage | `BootstrapConfig.LayoutQuery string` field | Separate config file, env var | Mirrors existing `EntryPointQuery`/`EntryPointViewID` pattern in `pkg/lua/loader.go` |
| Fallback mechanism | Exported `DefaultLayoutQuery` constant in `pkg/ui` | Error on empty, panic | 9 existing test mocks register the hardcoded SQL string — constant keeps them working via reference |
| LoadScreen param addition | Append `layoutQuery string` as 4th param | Options struct, functional opts | Current signature is 3 params; one more is fine. No options pattern anywhere in codebase. |
| Locale sanitization | Pure function `sanitizeLocale()` in `cmd/golemui/main.go` | init(), separate package | Simplest placement. `os` + `strings` only — no new deps. |
| Locale check timing | Before `app.New()` | After, or in init() | Fyne reads `LANG` during app construction. Must run first. |
| Test env isolation | `t.Setenv` (Go 1.17+) | `os.Setenv` + manual restore | `t.Setenv` auto-restores and prevents parallel, avoiding races |

## Data Flow

### LayoutQuery Flow

```
golemui_driver.lua
    │  LayoutQuery = "SELECT ..."
    ▼
lua.LoadConfig()
    │  getStringField(tbl, "LayoutQuery")
    ▼
BootstrapConfig.LayoutQuery (string)
    │
    ▼
RunBootstrap() ──────────────────────┐
    │                                 │
    ▼                                 ▼
LoadScreen(ctx, pool, vID, cfg.LayoutQuery)
    │
    ├─ layoutQuery != ""  →  use it
    └─ layoutQuery == ""  →  DefaultLayoutQuery constant
```

### Locale Sanitization Flow

```
RunBootstrap() entry
    │
    ▼
sanitizeLocale()
    ├─ LANG empty/"C"/"POSIX" AND LC_ALL empty/"C"/"POSIX" → os.Setenv("LANG", "en_US.UTF-8")
    ├─ LC_ALL valid → LANG = LC_ALL
    └─ LANG already valid → no-op
    │
    ▼
lua.LoadConfig()
    │
    ▼
app.New()   ← Fyne reads LANG here
```

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `pkg/lua/loader.go` | Modify | Add `LayoutQuery string` field to `BootstrapConfig` (line 21). Add `getStringField(tbl, "LayoutQuery")` call in `LoadConfig` (after line 104). Populate field in return struct. |
| `pkg/ui/screen_loader.go` | Modify | Add `DefaultLayoutQuery` exported constant (line ~12). Change `LoadScreen` signature to `(ctx, pool, vistaID, layoutQuery string)`. Add fallback: `if layoutQuery == "" { layoutQuery = DefaultLayoutQuery }`. Remove hardcoded SQL on line 20. |
| `cmd/golemui/main.go` | Modify | Add `sanitizeLocale()` func. Call it at top of `RunBootstrap` (before `lua.LoadConfig`). Add `"os"` and `"strings"` imports. Pass `cfg.LayoutQuery` to both `LoadScreen` calls (lines 70, 89). |
| `golemui_driver.lua` | Modify | Add `LayoutQuery = "SELECT config_columnas FROM golemui.vistas_consulta WHERE id = $1"` to table. |
| `pkg/lua/loader_test.go` | Modify | Add `TestLoadConfig_LayoutQuery_Present` and `TestLoadConfig_LayoutQuery_Absent`. |
| `pkg/ui/screen_loader_test.go` | Modify | Update all `RegisterQuery` calls to use `ui.DefaultLayoutQuery` constant. Update `LoadScreen` calls to pass 4th arg. Add test for custom query override and empty fallback. |
| `cmd/golemui/main_test.go` | Modify | Update `LoadScreen` mock registrations. Add `TestSanitizeLocale_*` tests (C, POSIX, empty, valid, LC_ALL precedence). |

## Interfaces / Contracts

```go
// pkg/ui/screen_loader.go — new constant
const DefaultLayoutQuery = "SELECT config_columnas FROM golemui.vistas_consulta WHERE id = $1"

// Updated signature
func LoadScreen(ctx context.Context, pool db.DatabasePool, vistaID string, layoutQuery string) (NodeMeta, error)

// pkg/lua/loader.go — new field
type BootstrapConfig struct {
    UIDB             ConfigConexion
    BusinessDB       ConfigConexion
    EntryPointQuery  string
    EntryPointViewID string
    LayoutQuery      string  // NEW
}

// cmd/golemui/main.go — new function
func sanitizeLocale() {
    lang := strings.TrimSpace(os.Getenv("LANG"))
    lcAll := strings.TrimSpace(os.Getenv("LC_ALL"))
    isInvalid := func(v string) bool {
        return v == "" || v == "C" || v == "POSIX"
    }
    if isInvalid(lcAll) && isInvalid(lang) {
        os.Setenv("LANG", "en_US.UTF-8")
    } else if !isInvalid(lcAll) {
        os.Setenv("LANG", lcAll)
    }
}
```

## Testing Strategy

| Layer | What | Approach |
|-------|------|----------|
| Unit — Lua | `LayoutQuery` present → parsed; absent → `""` | Temp Lua file, `LoadConfig`, assert field |
| Unit — ScreenLoader | Empty query fallback, custom query override | `MockDBPool` with `DefaultLayoutQuery` or custom SQL registered |
| Unit — ScreenLoader | Existing 4 scenarios (happy, no rows, bad JSON, nil pool) | Update mocks to pass 4th arg; reference `ui.DefaultLayoutQuery` |
| Unit — sanitizeLocale | C→en_US, POSIX→en_US, both empty→en_US, valid untouched, LC_ALL precedence | `t.Setenv` to control env; assert `os.Getenv("LANG")` after call |
| Unit — sanitizeLocale | Ordering: runs before `app.New()` | Structural — `sanitizeLocale()` is first call in `RunBootstrap` |

## Migration / Rollout

No migration required. `DefaultLayoutQuery` equals the current hardcoded string — zero behavioral change when `LayoutQuery` is absent from Lua. Revert feature branch to rollback.

## Open Questions

None — all decisions resolved during exploration.
