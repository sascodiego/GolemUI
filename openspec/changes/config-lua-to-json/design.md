# Design: Migrate Bootstrap Config from Lua to JSON

**Change:** `config-lua-to-json`
**Phase:** Design
**Date:** 2026-06-06

---

## 1. Architecture Overview

### 1.1 Before (Current)

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
│                   │     │    getStringField/getIntField│             │
└──────────────────┘     │ 6. Validation checks         │             ▼
                         └─────────────────────────────┘     cmd/golemui/main.go
                                                             flag -config defaults
                                                             to golemui_driver.lua
```

### 1.2 After (Target)

```
┌──────────────────┐     ┌─────────────────────────────┐     ┌───────────────┐
│ golemui_driver    │────▶│ pkg/lua/loader.go           │────▶│ BootstrapConfig│
│ .json             │     │ LoadConfig(path)             │     │ (Go struct)    │
│                   │     │                              │     │               │
│ JSON object       │     │ 1. os.Stat (file exists?)   │     │ UIDB          │
│ with nested       │     │ 2. os.ReadFile(path)        │     │ BusinessDB    │
│ objects UIDB,     │     │ 3. json.Unmarshal into       │     │ EntryPoint*   │
│ BusinessDB        │     │    intermediate raw struct   │     │ LayoutQuery   │
│                   │     │ 4. Validation checks         │     └───────┬───────┘
└──────────────────┘     └─────────────────────────────┘             │
                                                                     ▼
                                                             cmd/golemui/main.go
                                                             flag -config defaults
                                                             to golemui_driver.json
```

### 1.3 Key Architectural Change

The **Lua runtime dependency is removed from the config-loading path**. The `gopher-lua` dependency remains in `go.mod` (REQ-004) because other parts of the system may use it, but `LoadConfig` no longer imports or calls any `lua` package functions.

The Go struct types (`BootstrapConfig`, `ConfigConexion`) and the function signature `LoadConfig(path string) (*BootstrapConfig, error)` remain **identical** (REQ-003), so all callers (`cmd/golemui/main.go`) require only the flag default change.

---

## 2. Data Model Changes

### 2.1 Current Structs (Unchanged Exported Signatures)

```go
// pkg/lua/loader.go — exported types stay identical
type ConfigConexion struct {
    Host     string
    Port     int
    Database string
    User     string
    Password string
}

type BootstrapConfig struct {
    UIDB             ConfigConexion
    BusinessDB       ConfigConexion
    EntryPointQuery  string
    EntryPointViewID string
    LayoutQuery      string
}
```

### 2.2 JSON Struct Tags (Added to Existing Types)

Both structs gain `json` struct tags to support direct deserialization. Fields remain exported (they already are).

```go
type ConfigConexion struct {
    Host     string `json:"Host"`
    Port     int    `json:"Port"`
    Database string `json:"Database"`
    User     string `json:"User"`
    Password string `json:"Password"`
}

type BootstrapConfig struct {
    UIDB             ConfigConexion `json:"UIDB"`
    BusinessDB       ConfigConexion `json:"BusinessDB"`
    EntryPointQuery  string         `json:"EntryPointQuery"`
    EntryPointViewID string         `json:"EntryPointViewID"`
    LayoutQuery      string         `json:"LayoutQuery"`
}
```

**Design decision:** JSON keys use PascalCase (`"Host"`, `"Port"`, `"UIDB"`) to match the existing Lua key names exactly. This preserves visual parity with the old `.lua` file and avoids a confusing migration where every key casing changes.

### 2.3 Intermediate Raw Struct (Unexported, for Validation)

We cannot rely on `json.Unmarshal` returning partial zero-value fields to distinguish "field missing" from "field present but empty". However, the current Lua code does not make this distinction for string fields either — it defaults absent strings to `""`. For the validation of required connection fields, the post-unmarshal check (`host == "" || port == 0 || ...`) already works because:

- Missing JSON string field → Go zero value `""` → caught by validation.
- Missing JSON int field → Go zero value `0` → caught by validation.
- Explicit empty string `""` → also `""` → caught by validation.

Therefore, **no intermediate raw struct is needed**. We unmarshal directly into `BootstrapConfig` and run the same validation checks as the Lua version.

### 2.4 JSON Schema (Canonical File: `golemui_driver.json`)

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

This carries the **exact same values** as the current `golemui_driver.lua` (REQ-011). Note that `EntryPointQuery` is absent (was absent in the Lua file too) and defaults to `""`.

### 2.5 JSON Rules

| Rule | Detail |
|------|--------|
| Top-level | Must be a JSON object |
| `UIDB`, `BusinessDB` | Must be JSON objects, validated by required-field check |
| `EntryPointQuery` | Optional string, defaults `""` |
| `EntryPointViewID` | Optional string, defaults `""` |
| `LayoutQuery` | Optional string, defaults `""` |
| Extra fields | Ignored silently (forward-compatible) |
| Trailing commas | Not permitted by `encoding/json`; parse error if present |

---

## 3. Algorithm: LoadConfig New Implementation

### 3.1 Step-by-Step Pseudocode

```
FUNCTION LoadConfig(path string) → (*BootstrapConfig, error)

  1. CHECK file existence
     stat ← os.Stat(path)
     IF os.IsNotExist(stat.err) THEN
       RETURN nil, fmt.Errorf("config file does not exist: %s", path)
     END IF

  2. READ file bytes
     data, err ← os.ReadFile(path)
     IF err ≠ nil THEN
       RETURN nil, fmt.Errorf("failed to read config file: %w", err)
     END IF

  3. UNMARSHAL JSON
     var cfg BootstrapConfig
     err ← json.Unmarshal(data, &cfg)
     IF err ≠ nil THEN
       RETURN nil, fmt.Errorf("failed to parse JSON config: %w", err)
     END IF

  4. VALIDATE UIDB connection
     err ← validateConexion(cfg.UIDB, "UIDB")
     IF err ≠ nil THEN
       RETURN nil, err
     END IF

  5. VALIDATE BusinessDB connection
     err ← validateConexion(cfg.BusinessDB, "BusinessDB")
     IF err ≠ nil THEN
       RETURN nil, err
     END IF

  6. RETURN
     RETURN &cfg, nil

END FUNCTION
```

### 3.2 Helper: validateConexion

Extracted from the inline closure in the current code. Pure function, testable independently.

```
FUNCTION validateConexion(c ConfigConexion, name string) → error

  IF c.Host == "" || c.Port == 0 || c.Database == "" || c.User == "" THEN
    RETURN fmt.Errorf("missing required connection fields in %s", name)
  END IF

  RETURN nil

END FUNCTION
```

### 3.3 What Gets Removed

| Removed Element | Reason |
|---|---|
| `import lua "github.com/yuin/gopher-lua"` | No Lua runtime interaction (REQ-002) |
| `func getStringField(tbl *lua.LTable, key string) string` | Lua-specific helper |
| `func getIntField(tbl *lua.LTable, key string) int` | Lua-specific helper |
| `L := lua.NewState()` / `defer L.Close()` | Lua VM lifecycle |
| `L.DoFile(path)` | Lua file execution |
| `L.GetGlobal("golemui_driver")` | Lua global lookup |
| `tbl.RawGetString(...)` calls | Lua table iteration |
| Inner `parseConexion` closure | Replaced by `validateConexion` |

### 3.4 What Gets Added

| Added Element | Reason |
|---|---|
| `import "encoding/json"` | JSON parsing (REQ-002) |
| `json:"..."` struct tags on both structs | Structured deserialization |
| `func validateConexion(c ConfigConexion, name string) error` | Extracted validation, testable |

---

## 4. Algorithm: Validation Logic

### 4.1 Validation Checks and Error Messages

| # | Check | Trigger Condition | Error Message |
|---|---|---|---|
| V1 | File existence | `os.IsNotExist(err)` | `"config file does not exist: <path>"` |
| V2 | File readable | `os.ReadFile` returns error | `"failed to read config file: <err>"` |
| V3 | JSON syntax | `json.Unmarshal` returns `*json.SyntaxError` or other parse error | `"failed to parse JSON config: <err>"` |
| V4 | UIDB sub-object present/valid | After unmarshal, `cfg.UIDB` has all-zero values → caught by required fields check; but if the JSON key `UIDB` is missing entirely, the struct field is zero-valued, which means `Host=""` and `Port=0` → the required-fields check fires. However, we need a separate check for **the sub-object not being a JSON object at all** (e.g. `"UIDB": null` or `"UIDB": "string"`). The `encoding/json` unmarshaler will handle `null` as zero-value and non-object types as an error. | For `null`: falls through to required-fields → `"missing required connection fields in UIDB"`. For wrong type: caught by V3 parse error. |
| V5 | UIDB required fields | `host=="" || port==0 || database=="" || user==""` | `"missing required connection fields in UIDB"` |
| V6 | BusinessDB sub-object | Same logic as V4 for `BusinessDB` | `"missing required connection fields in BusinessDB"` |
| V7 | BusinessDB required fields | `host=="" || port==0 || database=="" || user==""` | `"missing required connection fields in BusinessDB"` |

### 4.2 Error Message Preservation

| REQ | Scenario | Current Lua Error | New JSON Error | Match? |
|---|---|---|---|---|
| REQ-006 | Missing file | `"config file does not exist: <path>"` | `"config file does not exist: <path>"` | Exact |
| REQ-007 | Invalid syntax | `"failed to execute Lua config: <err>"` | `"failed to parse JSON config: <err>"` | Adapted (different parser, same structure) |
| REQ-008 | Missing/invalid sub-table | `"sub-table X not found or invalid"` | `"missing required connection fields in X"` | **See note** |
| REQ-009 | Missing required fields | `"missing required connection fields in X"` | `"missing required connection fields in X"` | Exact |
| REQ-010 | Optional string absent | Returns `""` | Returns `""` (Go zero value) | Exact |

**Note on REQ-008:** In the Lua version, a missing sub-table (e.g. `UIDB` key absent or non-table) produces `"sub-table X not found or invalid"`. In the JSON version, a missing key `UIDB` results in a zero-valued `ConfigConexion` struct where `Host=""` and `Port=0`, which triggers the `"missing required connection fields in UIDB"` error instead. This is acceptable because:
1. The error is still descriptive and identifies the sub-object name.
2. REQ-008 states the error should be `"sub-table X not found or invalid"` — we preserve this by adding an explicit check **before** the required-fields check. If the entire sub-object struct is zero-valued (all 5 fields are zero), we emit the `"sub-table X not found or invalid"` message. Otherwise, if some fields are present but required ones are missing, we emit `"missing required connection fields in X"`.

### 4.3 Refined Validation Logic for Sub-Object Presence

```
FUNCTION validateConexion(c ConfigConexion, name string) → error

  // Distinguish "sub-object entirely absent/null" from "sub-object present but incomplete"
  IF c is entirely zero-valued (Host=="", Port==0, Database=="", User=="", Password=="") THEN
    RETURN fmt.Errorf("sub-table %s not found or invalid", name)
  END IF

  // Check required fields
  IF c.Host == "" || c.Port == 0 || c.Database == "" || c.User == "" THEN
    RETURN fmt.Errorf("missing required connection fields in %s", name)
  END IF

  RETURN nil

END FUNCTION
```

This preserves the exact error messages from REQ-008 and REQ-009.

**Edge case:** What if someone writes `"UIDB": { "Password": "x" }` — one field present, but not the required ones? Then it is not "entirely zero-valued", so it correctly hits `"missing required connection fields in UIDB"`.

**Edge case:** What if someone writes `"UIDB": { "Host": "h", "Port": 1, "Database": "d", "User": "u" }` — all required present, Password absent? Then Password defaults to `""`, which is correct (REQ-010).

---

## 5. Test Design

The existing test suite has **8 test functions**. The spec requires **17 tests rewritten for JSON fixtures** (REQ-012). The additional tests cover edge cases that were previously implicit or missing.

### 5.1 Test Table

| # | Test Name | Fixture Content | Expected Result | Validates |
|---|---|---|---|---|
| 1 | `TestLoadConfig_MissingFile` | *(no fixture — path to non-existent file)* | Error containing `"config file does not exist"` | REQ-006 |
| 2 | `TestLoadConfig_Success` | Full valid JSON with both connections, EntryPointQuery | Valid `BootstrapConfig` with all fields matched | REQ-001, REQ-003 |
| 3 | `TestLoadConfig_InvalidJSON_Syntax` | `{ "UIDB": { ` *(truncated, no closing brace)* | Error containing `"failed to parse JSON config"` | REQ-007 |
| 4 | `TestLoadConfig_InvalidJSON_TrailingComma` | `{ "UIDB": {}, "BusinessDB": {}, }` *(trailing comma)* | Error containing `"failed to parse JSON config"` | REQ-007 |
| 5 | `TestLoadConfig_InvalidJSON_NotObject` | `[1, 2, 3]` *(top-level array)* | Error containing `"failed to parse JSON config"` | REQ-007 |
| 6 | `TestLoadConfig_MissingUIDB` | Valid BusinessDB, no UIDB key | Error `"sub-table UIDB not found or invalid"` | REQ-008 |
| 7 | `TestLoadConfig_MissingBusinessDB` | Valid UIDB, no BusinessDB key | Error `"sub-table BusinessDB not found or invalid"` | REQ-008 |
| 8 | `TestLoadConfig_UIDBNull` | `"UIDB": null` | Error `"sub-table UIDB not found or invalid"` | REQ-008 |
| 9 | `TestLoadConfig_MissingHost` | UIDB missing Host, BusinessDB valid | Error `"missing required connection fields in UIDB"` | REQ-009 |
| 10 | `TestLoadConfig_MissingPort` | UIDB with Port absent (defaults 0), rest valid | Error `"missing required connection fields in UIDB"` | REQ-009 |
| 11 | `TestLoadConfig_MissingDatabase` | UIDB missing Database | Error `"missing required connection fields in UIDB"` | REQ-009 |
| 12 | `TestLoadConfig_MissingUser` | UIDB missing User | Error `"missing required connection fields in UIDB"` | REQ-009 |
| 13 | `TestLoadConfig_PasswordAbsent` | Both connections valid, Password key absent | Success, `Password == ""` | REQ-010 |
| 14 | `TestLoadConfig_EntryPointViewID_Present` | Valid config with `"EntryPointViewID": "dashboard"` | Success, `EntryPointViewID == "dashboard"` | REQ-001 |
| 15 | `TestLoadConfig_EntryPointViewID_Absent` | Valid config, no EntryPointViewID key | Success, `EntryPointViewID == ""` | REQ-010 |
| 16 | `TestLoadConfig_LayoutQuery_Present` | Valid config with explicit LayoutQuery | Success, LayoutQuery matched | REQ-001 |
| 17 | `TestLoadConfig_LayoutQuery_Absent` | Valid config, no LayoutQuery key | Success, `LayoutQuery == ""` | REQ-010 |

### 5.2 Test Implementation Notes

- All fixture content is written as inline string constants in the test functions (same pattern as current tests).
- Each test creates a temp file via `t.TempDir()` + `os.WriteFile`, now with `.json` extension.
- The file extension does not matter to `LoadConfig` — only the path argument matters.
- Tests that check specific error substrings use `strings.Contains(err.Error(), expected)`.
- The `TestLoadConfig_Success` test validates **every field** of both connection objects plus the optional fields.

### 5.3 Test Fixture Helpers

The tests share a common "valid base" JSON that can be minimally mutated:

```go
const validBaseJSON = `{
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
  "EntryPointQuery": "SELECT * FROM golemui.layouts LIMIT 1"
}`
```

Individual tests override specific fields or remove keys as needed.

---

## 6. Migration Steps

Ordered implementation sequence. Each step is atomic and testable.

### Step 1: Add JSON struct tags to `ConfigConexion` and `BootstrapConfig`

**File:** `pkg/lua/loader.go`
**Action:** Add `json:"..."` tags to all fields in both structs.
**Risk:** None. Struct tags are inert for non-JSON usage.

### Step 2: Replace `LoadConfig` implementation

**File:** `pkg/lua/loader.go`
**Action:**
- Remove `import lua "github.com/yuin/gopher-lua"`.
- Add `import "encoding/json"`.
- Remove `getStringField` and `getIntField` functions.
- Rewrite `LoadConfig` body: `os.ReadFile` → `json.Unmarshal` → validation.
- Add unexported `validateConexion` helper.
**Risk:** This is the core change. All existing tests will break (Lua fixtures → need JSON fixtures).

### Step 3: Rewrite all tests for JSON fixtures

**File:** `pkg/lua/loader_test.go`
**Action:**
- Replace all Lua fixture strings with equivalent JSON fixture strings.
- Add 9 new test functions to reach 17 total (see §5.1).
- Change `lua.LoadConfig("non_existent_file_xyz.lua")` to `lua.LoadConfig("non_existent_file_xyz.json")` (the filename is cosmetic but should match the new convention).
**Risk:** Test logic is mechanical translation. Low risk.

### Step 4: Update CLI default flag

**File:** `cmd/golemui/main.go`
**Action:** Change `flagConfig` default from `"golemui_driver.lua"` to `"golemui_driver.json"`.
**Risk:** Minimal. Users who pass `-config` explicitly are unaffected.

### Step 5: Create `golemui_driver.json`, delete `golemui_driver.lua`

**Files:** `golemui_driver.json` (create), `golemui_driver.lua` (delete)
**Action:**
- Create the JSON file with values matching the current Lua file (see §2.4).
- Delete the Lua file.
**Risk:** If anyone is running the binary without `-config`, it will now look for `.json` by default. The new file must exist before the old one is deleted.

### Step 6: Verify `gopher-lua` remains in `go.mod`

**File:** `go.mod`
**Action:** No change required. `gopher-lua` stays as a dependency (REQ-004). Do **not** run `go mod tidy` if it would remove `gopher-lua` while other code still imports it. If no other code imports it after this change, it can be removed from `go.mod` — but the spec says it must stay. Verify with `grep -r 'gopher-lua' pkg/ cmd/` and, if no other references exist, add a comment to `go.mod` or keep the import in some other file.

---

## 7. File Change Specification

### 7.1 `pkg/lua/loader.go`

| Element | Action | Detail |
|---|---|---|
| `import lua "github.com/yuin/gopher-lua"` | **REMOVE** | No Lua runtime calls in LoadConfig |
| `import "encoding/json"` | **ADD** | JSON deserialization |
| `ConfigConexion` struct tags | **MODIFY** | Add `json:"Host"`, `json:"Port"`, `json:"Database"`, `json:"User"`, `json:"Password"` |
| `BootstrapConfig` struct tags | **MODIFY** | Add `json:"UIDB"`, `json:"BusinessDB"`, `json:"EntryPointQuery"`, `json:"EntryPointViewID"`, `json:"LayoutQuery"` |
| `func getStringField(...)` | **REMOVE** | Lua-specific, no longer used |
| `func getIntField(...)` | **REMOVE** | Lua-specific, no longer used |
| `func validateConexion(c ConfigConexion, name string) error` | **ADD** | Extracted validation logic (see §4.3) |
| `func LoadConfig(path string) (*BootstrapConfig, error)` body | **MODIFY** | Replace Lua VM logic with ReadFile + json.Unmarshal + validateConexion (see §3.1) |

### 7.2 `pkg/lua/loader_test.go`

| Element | Action | Detail |
|---|---|---|
| Package import `lua` | **KEEP** | Tests still import `GolemUI/pkg/lua` |
| All 8 existing test functions | **MODIFY** | Replace Lua fixture strings with JSON fixture strings |
| 9 new test functions | **ADD** | Tests 3–5, 6–8, 9–12, 13 from §5.1 table |

### 7.3 `cmd/golemui/main.go`

| Element | Action | Detail |
|---|---|---|
| `flagConfig` default value | **MODIFY** | `"golemui_driver.lua"` → `"golemui_driver.json"` |
| Comment on flagConfig | **MODIFY** | `"Path to Lua configuration file"` → `"Path to JSON configuration file"` |

### 7.4 `golemui_driver.lua` (project root)

| Action | Detail |
|---|---|
| **DELETE** | Replaced by JSON equivalent (REQ-011) |

### 7.5 `golemui_driver.json` (project root)

| Action | Detail |
|---|---|
| **CREATE** | JSON config with same values as the deleted Lua file (see §2.4) |

### 7.6 `go.mod`

| Action | Detail |
|---|---|
| **NO CHANGE** | `gopher-lua` v1.1.2 stays in `go.mod` (REQ-004) |

---

## Appendix A: Full Replacement `LoadConfig` Implementation (Reference)

```go
package lua

import (
    "encoding/json"
    "fmt"
    "os"
)

type ConfigConexion struct {
    Host     string `json:"Host"`
    Port     int    `json:"Port"`
    Database string `json:"Database"`
    User     string `json:"User"`
    Password string `json:"Password"`
}

type BootstrapConfig struct {
    UIDB             ConfigConexion `json:"UIDB"`
    BusinessDB       ConfigConexion `json:"BusinessDB"`
    EntryPointQuery  string         `json:"EntryPointQuery"`
    EntryPointViewID string         `json:"EntryPointViewID"`
    LayoutQuery      string         `json:"LayoutQuery"`
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

    data, err := os.ReadFile(path)
    if err != nil {
        return nil, fmt.Errorf("failed to read config file: %w", err)
    }

    var cfg BootstrapConfig
    if err := json.Unmarshal(data, &cfg); err != nil {
        return nil, fmt.Errorf("failed to parse JSON config: %w", err)
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

## Appendix B: Full `golemui_driver.json` Content

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
