# SDD Spec: Migrate Bootstrap Config from Lua to JSON

**Change ID:** `config-lua-to-json`
**Phase:** Spec
**Status:** Draft
**Date:** 2026-06-06

---

## 1. Introduction

GolemUI currently loads its bootstrap configuration by evaluating a Lua script (`golemui_driver.lua`) through the `gopher-lua` VM. This approach introduces unnecessary attack surface (arbitrary code execution) and a heavy dependency for what is fundamentally static configuration data.

This change replaces Lua VM evaluation with static JSON file parsing using Go's standard library `encoding/json`. The function signature `LoadConfig(path string) (*BootstrapConfig, error)` and all exported types (`BootstrapConfig`, `ConfigConexion`) remain unchanged. The configuration file format changes from `.lua` to `.json`.

### Scope

- **In scope:** `pkg/lua/loader.go`, `pkg/lua/loader_test.go`, `cmd/golemui/main.go`, `cmd/golemui/main_test.go`, `golemui_driver.lua` → `golemui_driver.json`.
- **Out of scope:** Removing `gopher-lua` from `go.mod` (kept per requirement). Any changes to `BootstrapConfig` or `ConfigConexion` field names or types. Any changes to downstream consumers beyond adapting test fixtures.

### Affected Files

| File | Change Type |
|------|-------------|
| `pkg/lua/loader.go` | Rewrite: replace Lua VM with JSON unmarshal |
| `pkg/lua/loader_test.go` | Rewrite: 8 tests adapted from Lua fixtures to JSON fixtures |
| `cmd/golemui/main.go` | Edit: CLI flag default and description |
| `cmd/golemui/main_test.go` | Edit: 10 tests adapted from Lua fixtures to JSON fixtures |
| `golemui_driver.lua` | Delete |
| `golemui_driver.json` | Create: equivalent JSON config |

### Estimated Diff

~141 lines changed across 6 files.

---

## 2. Requirements

### REQ-001: JSON config file format

**ID:** `REQ-001`
The bootstrap configuration file shall use JSON format with the following structure:

```json
{
  "UIDB": {
    "Host": "localhost",
    "Port": 5432,
    "Database": "golemui_core",
    "User": "golemui_core_engine",
    "Password": "secret_password_for_ui"
  },
  "BusinessDB": {
    "Host": "localhost",
    "Port": 5432,
    "Database": "negocio_production",
    "User": "golemui_render_engine",
    "Password": "secret_password_for_business"
  },
  "EntryPointViewID": "transacciones_list",
  "LayoutQuery": "SELECT config_columnas FROM golemui.vistas_consulta WHERE id = $1"
}
```

The top-level object maps directly to `BootstrapConfig`. All fields are optional at the JSON level; validation is performed in Go code after unmarshalling.

**Testable:** A valid JSON file matching this schema is accepted by `LoadConfig` and returns a fully populated `BootstrapConfig`.

---

### REQ-002: LoadConfig uses encoding/json exclusively

**ID:** `REQ-002`
`LoadConfig` shall use `os.ReadFile` + `encoding/json.Unmarshal` to parse the config file. It shall never call `lua.NewState()`, `L.DoFile()`, `L.GetGlobal()`, or any `gopher-lua` API. The helper functions `getStringField` and `getIntField` shall be removed.

**Testable:** After the change, `pkg/lua/loader.go` contains zero imports from `github.com/yuin/gopher-lua` and zero references to `lua.NewState`.

---

### REQ-003: Exported type signatures unchanged

**ID:** `REQ-003`
`ConfigConexion` and `BootstrapConfig` struct definitions shall remain identical in field names, field types, and exported visibility. No new exported fields shall be added. The `LoadConfig` function signature remains `func LoadConfig(path string) (*BootstrapConfig, error)`.

**Testable:** All existing callers (`cmd/golemui/main.go` referencing `cfg.UIDB.Host`, `cfg.BusinessDB.Port`, etc.) compile and pass without modification to those access patterns.

---

### REQ-004: gopher-lua remains in go.mod

**ID:** `REQ-004`
The `github.com/yuin/gopher-lua` dependency shall remain in `go.mod` and `go.sum`. No dependency removal.

**Testable:** `go list -m github.com/yuin/gopher-lua` succeeds after the change.

---

### REQ-005: CLI flag default updated to JSON

**ID:** `REQ-005`
The `-config` CLI flag in `cmd/golemui/main.go` shall default to `"golemui_driver.json"` and its description shall reference "JSON" instead of "Lua".

**Testable:** Running the binary without `-config` flag attempts to load `golemui_driver.json`. The help text (`-h`) shows "JSON" in the description.

---

### REQ-006: Missing config file error preserved

**ID:** `REQ-006`
When the config file does not exist, `LoadConfig` shall return an error matching the pattern `"config file does not exist: <path>"`. This preserves the existing error message.

**Testable:** Calling `LoadConfig("non_existent_file.json")` returns a non-nil error whose message contains `"config file does not exist"`.

---

### REQ-007: Invalid JSON syntax error

**ID:** `REQ-007`
When the config file contains invalid JSON (malformed syntax), `LoadConfig` shall return an error whose message contains descriptive information from `json.Unmarshal`. The error wraps the standard JSON parse error.

**Testable:** A file containing `{invalid json` produces a non-nil error. The error message contains information indicating a JSON parse failure (e.g., "invalid character", "unexpected", or similar from the `encoding/json` package).

---

### REQ-008: Missing top-level fields validation

**ID:** `REQ-008`
When `UIDB` or `BusinessDB` sub-objects are missing from the JSON (zero-valued after unmarshal), `LoadConfig` shall return an error matching the pattern `"sub-table <name> not found or invalid"` where `<name>` is `"UIDB"` or `"BusinessDB"`.

A sub-object is considered "missing or invalid" when its `Host` field is empty (the zero-value check). Since `encoding/json` populates struct fields from the JSON, an absent key leaves the struct at its zero value, which naturally has `Host == ""`.

**Testable:** A JSON config omitting the `"UIDB"` key produces an error containing `"sub-table UIDB not found or invalid"`.

---

### REQ-009: Missing required connection fields validation

**ID:** `REQ-009`
When any of `Host`, `Port`, or `Database`, or `User` are missing or zero-valued within a connection sub-object, `LoadConfig` shall return an error matching `"missing required connection fields in <name>"` where `<name>` is `"UIDB"` or `"BusinessDB"`.

The validation rules are:
- `Host != ""`
- `Port != 0`
- `Database != ""`
- `User != ""`

`Password` is NOT required (may be empty).

**Testable:** A JSON config where `UIDB.Host` is `""` (absent or explicitly empty) produces an error containing `"missing required connection fields in UIDB"`.

---

### REQ-010: Optional top-level string fields

**ID:** `REQ-010`
`EntryPointQuery`, `EntryPointViewID`, and `LayoutQuery` are optional string fields. When absent from JSON, they shall default to `""` (Go zero value for string). No validation error shall be raised for their absence.

**Testable:**
- A config without `"EntryPointViewID"` yields `config.EntryPointViewID == ""`.
- A config with `"EntryPointViewID": "dashboard"` yields `config.EntryPointViewID == "dashboard"`.
- Same pattern for `EntryPointQuery` and `LayoutQuery`.

---

### REQ-011: Production config file replaced

**ID:** `REQ-011`
The file `golemui_driver.lua` shall be deleted and replaced with `golemui_driver.json` containing the same configuration values as the current Lua file.

Current Lua values to preserve in JSON:
- `UIDB.Host`: `"localhost"`
- `UIDB.Port`: `5432`
- `UIDB.Database`: `"golemui_core"`
- `UIDB.User`: `"golemui_core_engine"`
- `UIDB.Password`: `"secret_password_for_ui"`
- `BusinessDB.Host`: `"localhost"`
- `BusinessDB.Port`: `5432`
- `BusinessDB.Database`: `"negocio_production"`
- `BusinessDB.User`: `"golemui_render_engine"`
- `BusinessDB.Password`: `"secret_password_for_business"`
- `EntryPointViewID`: `"transacciones_list"`
- `LayoutQuery`: `"SELECT config_columnas FROM golemui.vistas_consulta WHERE id = $1"`

**Testable:** The new `golemui_driver.json` is valid JSON. `LoadConfig("golemui_driver.json")` succeeds and returns the expected values.

---

### REQ-012: All tests rewritten for JSON

**ID:** `REQ-012`
All tests in `pkg/lua/loader_test.go` (8 tests) and config-dependent tests in `cmd/golemui/main_test.go` (10 tests) shall be rewritten to use JSON test fixtures instead of Lua test fixtures. The test logic (assertions, error checks) remains semantically equivalent.

**Testable:** `go test ./pkg/lua/ ./cmd/golemui/` passes with all tests using JSON fixtures.

---

## 3. Scenarios

### Scenario: SC-001 — Successful config load

```
GIVEN a file "test_config.json" exists with valid JSON containing
      complete UIDB and BusinessDB connection objects
  AND UIDB has Host="localhost", Port=5432, Database="golemui_core",
      User="postgres", Password="password123"
  AND BusinessDB has Host="127.0.0.1", Port=5433, Database="negocio_production",
      User="biz_user", Password="biz_password"
  AND EntryPointQuery="SELECT * FROM golemui.layouts LIMIT 1"
 WHEN LoadConfig("test_config.json") is called
 THEN the returned BootstrapConfig has UIDB.Host="localhost", UIDB.Port=5432,
      UIDB.Database="golemui_core", UIDB.User="postgres", UIDB.Password="password123"
  AND BusinessDB.Host="127.0.0.1", BusinessDB.Port=5433,
      BusinessDB.Database="negocio_production", BusinessDB.User="biz_user",
      BusinessDB.Password="biz_password"
  AND EntryPointQuery="SELECT * FROM golemui.layouts LIMIT 1"
  AND error is nil
```

### Scenario: SC-002 — Config file does not exist

```
GIVEN no file exists at path "non_existent_file_xyz.json"
 WHEN LoadConfig("non_existent_file_xyz.json") is called
 THEN the returned error is non-nil
  AND the error message contains "config file does not exist: non_existent_file_xyz.json"
```

### Scenario: SC-003 — Invalid JSON syntax

```
GIVEN a file "bad_config.json" exists containing "{invalid json"
 WHEN LoadConfig("bad_config.json") is called
 THEN the returned error is non-nil
  AND the error message indicates a JSON parse failure
```

### Scenario: SC-004 — Missing UIDB connection fields

```
GIVEN a file "missing_host.json" exists with valid JSON where
      UIDB is missing the "Host" field (or Host="")
  AND all other fields are valid
 WHEN LoadConfig("missing_host.json") is called
 THEN the returned error is non-nil
  AND the error message contains "missing required connection fields in UIDB"
```

### Scenario: SC-005 — Missing BusinessDB connection fields

```
GIVEN a file "missing_biz_port.json" exists with valid JSON where
      BusinessDB is present but Port is 0 (absent or explicitly 0)
  AND all other fields are valid
 WHEN LoadConfig("missing_biz_port.json") is called
 THEN the returned error is non-nil
  AND the error message contains "missing required connection fields in BusinessDB"
```

### Scenario: SC-006 — EntryPointViewID present

```
GIVEN a valid config JSON with "EntryPointViewID": "dashboard"
 WHEN LoadConfig is called
 THEN config.EntryPointViewID equals "dashboard"
```

### Scenario: SC-007 — EntryPointViewID absent

```
GIVEN a valid config JSON without an "EntryPointViewID" key
 WHEN LoadConfig is called
 THEN config.EntryPointViewID equals ""
  AND no error is returned
```

### Scenario: SC-008 — LayoutQuery present

```
GIVEN a valid config JSON with "LayoutQuery": "SELECT col FROM tbl WHERE id = $1"
 WHEN LoadConfig is called
 THEN config.LayoutQuery equals "SELECT col FROM tbl WHERE id = $1"
```

### Scenario: SC-009 — LayoutQuery absent

```
GIVEN a valid config JSON without a "LayoutQuery" key
 WHEN LoadConfig is called
 THEN config.LayoutQuery equals ""
  AND no error is returned
```

### Scenario: SC-010 — Bootstrap integration: missing config file

```
GIVEN no file exists at "non_existent_config.json"
 WHEN RunBootstrap is called with path "non_existent_config.json"
 THEN the returned error is non-nil
  AND the error message mentions the config file failure
```

### Scenario: SC-011 — Bootstrap integration: database connection failure

```
GIVEN a valid config JSON pointing to unreachable database hosts
 WHEN RunBootstrap is called
 THEN the returned error is non-nil
  AND the error message indicates a database connection failure
```

### Scenario: SC-012 — Bootstrap integration: invalid top-level structure

```
GIVEN a JSON file where the top-level object is empty `{}`
 WHEN LoadConfig is called
 THEN the returned error is non-nil
  AND the error message contains "sub-table UIDB not found or invalid"
```

### Scenario: SC-013 — Bootstrap integration: successful full bootstrap

```
GIVEN a valid config JSON with valid connection parameters
  AND a mocked database that returns a valid screen layout
 WHEN RunBootstrap is called with runWindow=false
 THEN the returned App is non-nil
  AND App.Config is populated with the config values
  AND App.DB is populated with the database pools
```

### Scenario: SC-014 — Bootstrap integration: default vista ID

```
GIVEN a valid config JSON without EntryPointViewID
  AND a mocked database that returns a valid screen layout for "home"
 WHEN RunBootstrap is called with viewOverride=""
 THEN bootstrap succeeds using the default vista ID "home"
```

### Scenario: SC-015 — Bootstrap integration: view override wins

```
GIVEN a valid config JSON with EntryPointViewID="dashboard"
  AND a mocked database that returns a valid screen layout for "settings"
 WHEN RunBootstrap is called with viewOverride="settings"
 THEN bootstrap succeeds
  AND the loaded screen corresponds to "settings" (not "dashboard")
```

### Scenario: SC-016 — Bootstrap integration: LoadScreen failure

```
GIVEN a valid config JSON with EntryPointViewID="nonexistent"
  AND a mocked database that returns ErrNoRows for that vista
 WHEN RunBootstrap is called
 THEN the returned error is non-nil
  AND the error message mentions LoadScreen
  AND the returned App is nil
```

### Scenario: SC-017 — Bootstrap integration: empty override falls through to config

```
GIVEN a valid config JSON with EntryPointViewID="transacciones_list"
  AND a mocked database that returns a valid screen layout
 WHEN RunBootstrap is called with viewOverride=""
 THEN bootstrap succeeds
  AND the loaded screen corresponds to "transacciones_list"
```

---

## 4. Design Constraints

1. **No gopher-lua removal:** The dependency stays in `go.mod` to avoid a large diff in dependency management that is orthogonal to this change.
2. **Package name unchanged:** The package remains `lua` even though it no longer uses Lua. Renaming the package is out of scope and would require a broader refactor of import paths.
3. **Error message continuity:** Existing error messages for missing files, missing fields, and validation failures are preserved in their current wording to avoid breaking any external tooling or documentation that matches against them. The only error message that changes substantively is the one previously reading `"failed to execute Lua config: ..."` which becomes a JSON parse error.
4. **JSON struct tags:** `ConfigConexion` and `BootstrapConfig` will receive `json:"..."` struct tags matching the current Lua table key names exactly (e.g., `json:"Host"`, `json:"Port"`, `json:"UIDB"`, `json:"BusinessDB"`).
5. **No schema versioning:** The JSON file has no version field. This is a v1 artifact; versioning can be added later if the schema evolves.

---

## 5. Open Questions

None. All requirements are derived from the current code behavior and the approved proposal.
