package lua_test

import (
	"os"
	"path/filepath"
	"testing"
	"GolemUI/pkg/lua"
)

func TestLoadConfig_MissingFile(t *testing.T) {
	_, err := lua.LoadConfig("non_existent_file_xyz.lua")
	if err == nil {
		t.Error("Expected an error for non-existent config file, got nil")
	}
}

func TestLoadConfig_Success(t *testing.T) {
	// Create a temporary Lua configuration file
	content := `
golemui_driver = {
    UIDB = {
        Host = "localhost",
        Port = 5432,
        Database = "golemui_core",
        User = "postgres",
        Password = "password123"
    },
    BusinessDB = {
        Host = "127.0.0.1",
        Port = 5433,
        Database = "negocio_production",
        User = "biz_user",
        Password = "biz_password"
    },
    EntryPointQuery = "SELECT * FROM golemui.layouts LIMIT 1"
}
`
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "golemui_driver_test.lua")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	config, err := lua.LoadConfig(tmpFile)
	if err != nil {
		t.Fatalf("Expected successful load, got error: %v", err)
	}

	if config.UIDB.Host != "localhost" || config.UIDB.Port != 5432 || config.UIDB.Database != "golemui_core" || config.UIDB.User != "postgres" || config.UIDB.Password != "password123" {
		t.Errorf("UIDB values mismatched: %+v", config.UIDB)
	}

	if config.BusinessDB.Host != "127.0.0.1" || config.BusinessDB.Port != 5433 || config.BusinessDB.Database != "negocio_production" || config.BusinessDB.User != "biz_user" || config.BusinessDB.Password != "biz_password" {
		t.Errorf("BusinessDB values mismatched: %+v", config.BusinessDB)
	}

	if config.EntryPointQuery != "SELECT * FROM golemui.layouts LIMIT 1" {
		t.Errorf("EntryPointQuery mismatched: %q", config.EntryPointQuery)
	}
}

func TestLoadConfig_InvalidSyntax(t *testing.T) {
	// Invalid syntax: missing brace
	content := `
golemui_driver = {
    UIDB = {
        Host = "localhost"
-- missing closing braces
`
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "golemui_driver_invalid.lua")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	_, err := lua.LoadConfig(tmpFile)
	if err == nil {
		t.Error("Expected compile error for invalid Lua syntax, but got no error")
	}
}

func TestLoadConfig_MissingFields(t *testing.T) {
	// Missing "Host" in UIDB
	content := `
golemui_driver = {
    UIDB = {
        Port = 5432,
        Database = "golemui_core",
        User = "postgres",
        Password = "password123"
    },
    BusinessDB = {
        Host = "127.0.0.1",
        Port = 5433,
        Database = "negocio_production",
        User = "biz_user",
        Password = "biz_password"
    },
    EntryPointQuery = "SELECT * FROM golemui.layouts LIMIT 1"
}
`
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "golemui_driver_missing.lua")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	_, err := lua.LoadConfig(tmpFile)
	if err == nil {
		t.Error("Expected error due to missing required connection fields, but got no error")
	}
}

func TestLoadConfig_EntryPointViewID_Present(t *testing.T) {
	content := `
golemui_driver = {
    UIDB = {
        Host = "localhost",
        Port = 5432,
        Database = "golemui_core",
        User = "postgres",
        Password = "password123"
    },
    BusinessDB = {
        Host = "127.0.0.1",
        Port = 5433,
        Database = "negocio_production",
        User = "biz_user",
        Password = "biz_password"
    },
    EntryPointQuery = "SELECT * FROM golemui.layouts LIMIT 1",
    EntryPointViewID = "dashboard"
}
`
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "golemui_driver_viewid.lua")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	config, err := lua.LoadConfig(tmpFile)
	if err != nil {
		t.Fatalf("Expected successful load, got error: %v", err)
	}

	if config.EntryPointViewID != "dashboard" {
		t.Errorf("expected EntryPointViewID %q, got %q", "dashboard", config.EntryPointViewID)
	}
}

func TestLoadConfig_EntryPointViewID_Absent(t *testing.T) {
	content := `
golemui_driver = {
    UIDB = {
        Host = "localhost",
        Port = 5432,
        Database = "golemui_core",
        User = "postgres",
        Password = "password123"
    },
    BusinessDB = {
        Host = "127.0.0.1",
        Port = 5433,
        Database = "negocio_production",
        User = "biz_user",
        Password = "biz_password"
    },
    EntryPointQuery = "SELECT * FROM golemui.layouts LIMIT 1"
}
`
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "golemui_driver_no_viewid.lua")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	config, err := lua.LoadConfig(tmpFile)
	if err != nil {
		t.Fatalf("Expected successful load, got error: %v", err)
	}

	if config.EntryPointViewID != "" {
		t.Errorf("expected EntryPointViewID to be empty string when absent, got %q", config.EntryPointViewID)
	}
}
