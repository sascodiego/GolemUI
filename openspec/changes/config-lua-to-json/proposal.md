# SDD Proposal: Migrate Bootstrap Config from Lua to JSON

## 1. Problem Statement

`pkg/lua/loader.go` currently bootstraps the application configuration by spinning up a full gopher-lua VM (`lua.NewState()`, `L.DoFile()`, table traversal) to evaluate a `.lua` file and extract a flat config struct. This introduces an unnecessary scripting runtime dependency for what is a static, declarative configuration with no dynamic logic. The Lua VM adds startup overhead, larger attack surface, and complexity for a task that pure JSON deserialization handles natively and safely.

## 2. Proposed Solution

Replace the Lua VM evaluation in `LoadConfig` with `encoding/json.Unmarshal`. The function signature and exported types (`BootstrapConfig`, `ConfigConexion`) remain unchanged. The config file format changes from executable Lua to declarative JSON:

```json
{
  "UIDB": {
    "Host": "localhost",
    "Port": 5432,
    "Database": "golemui_core",
    "User": "postgres",
    "Password": "password123"
  },
  "BusinessDB": {
    "Host": "127.0.0.1",
    "Port": 5433,
    "Database": "negocio_production",
    "User": "biz_user",
    "Password": "biz_password"
  },
  "EntryPointQuery": "SELECT * FROM golemui.layouts LIMIT 1",
  "EntryPointViewID": "dashboard",
  "LayoutQuery": "SELECT col FROM tbl WHERE id = $1"
}
```

The JSON structure maps directly to `BootstrapConfig` via struct tags. Validation (required connection fields) moves to a simple post-unmarshal check, preserving the same error messages callers rely on.

### Specific changes

1. **`pkg/lua/loader.go`** — Remove `lua.NewState()`, `DoFile`, `GetGlobal`, all helper functions (`getStringField`, `getIntField`). Add `encoding/json` import. `LoadConfig` reads the file with `os.ReadFile` and unmarshals into `BootstrapConfig`. Add struct tags for JSON keys. Port validation (`port != 0`) and required-field checks remain as explicit logic after unmarshal to match existing error semantics.
2. **`cmd/golemui/main.go`** — Change `-config` flag default from `"golemui_driver.lua"` to `"golemui_driver.json"`.
3. **`pkg/lua/loader_test.go`** — Rewrite all 8 test temp files from Lua syntax to JSON. Update error expectation strings where they reference "Lua" or "table".
4. **`cmd/golemui/main_test.go`** — Rewrite all 9 bootstrap test temp files from Lua syntax to JSON. Update error expectation strings (e.g., `"golemui_driver table not found"` → JSON-equivalent error message).
5. **`golemui_driver.json`** — Create the reference config file (replacing `golemui_driver.lua`).
6. **gopher-lua** — Remains in `go.mod`; only the import is removed from `loader.go`.

## 3. Scope

### In Scope
- `LoadConfig` implementation swap (Lua VM → JSON unmarshal).
- Struct tags on `BootstrapConfig` and `ConfigConexion` for JSON deserialization.
- CLI `-config` default value change.
- All test rewrites (17 tests total across 2 files).
- Reference `golemui_driver.json` config file.

### Out of Scope
- Database schema changes.
- Fyne rendering or UI composition logic.
- Runtime Lua scripting (gopher-lua stays in `go.mod` for future use).
- Any changes to public API signatures (`LoadConfig`, `BootstrapConfig`, `ConfigConexion`).
- Changes to `pkg/db`, `pkg/ui`, `pkg/eventbus`, or plugin system.

## 4. Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Existing `.lua` config files in deployed environments break silently | Low | Medium — app won't start | Error message explicitly states JSON is expected; migration is a mechanical key rename. Document the format change in commit message. |
| Error message strings change, breaking callers that parse them | Low | Low | Preserve existing error messages for file-not-found and missing-field validation. New JSON parse errors are strictly more descriptive. |
| Port `0` edge case: JSON `null`/missing `port` unmarshals to `0` (same as Lua behavior) | None | None | `int` zero-value already handled identically to current Lua code. |
| `EntryPointQuery`/`EntryPointViewID`/`LayoutQuery` absent in JSON → empty string (same as Lua) | None | None | Go zero-value semantics match current Lua `""` fallback exactly. |

## 5. Affected Files

| File | Change Type | Est. Lines |
|---|---|---|
| `pkg/lua/loader.go` | Rewrite LoadConfig body; remove Lua helpers, add JSON tags and validation | ~60 |
| `cmd/golemui/main.go` | Change `-config` default string | ~1 |
| `pkg/lua/loader_test.go` | Rewrite 8 test fixtures from Lua to JSON; update error assertions | ~30 |
| `cmd/golemui/main_test.go` | Rewrite 9 test fixtures from Lua to JSON; update error assertions | ~35 |
| `golemui_driver.json` | New file — reference config | ~15 |
| `golemui_driver.lua` | Delete (replaced by `.json`) | - |

## 6. Estimated Total Lines Changed

~141 lines changed (net, after deletions). Well within the 400-line review budget.
