package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"GolemUI/pkg/config"
)

func TestLoadConfig_MissingFile(t *testing.T) {
	_, err := config.LoadConfig("non_existent_file_xyz.yaml")
	if err == nil {
		t.Error("Expected an error for non-existent config file, got nil")
	}
}

func TestLoadConfig_Success(t *testing.T) {
	content := `
uidb:
  host: "localhost"
  port: 5432
  database: "golemui_core"
  user: "postgres"
  password: "password123"
business_db:
  host: "127.0.0.1"
  port: 5433
  database: "negocio_production"
  user: "biz_user"
  password: "biz_password"
`
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "golemui_driver_test.yaml")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	config, err := config.LoadConfig(tmpFile)
	if err != nil {
		t.Fatalf("Expected successful load, got error: %v", err)
	}

	if config.UIDB.Host != "localhost" || config.UIDB.Port != 5432 || config.UIDB.Database != "golemui_core" || config.UIDB.User != "postgres" || config.UIDB.Password != "password123" {
		t.Errorf("UIDB values mismatched: %+v", config.UIDB)
	}

	if config.BusinessDB.Host != "127.0.0.1" || config.BusinessDB.Port != 5433 || config.BusinessDB.Database != "negocio_production" || config.BusinessDB.User != "biz_user" || config.BusinessDB.Password != "biz_password" {
		t.Errorf("BusinessDB values mismatched: %+v", config.BusinessDB)
	}

}

func TestLoadConfig_InvalidSyntax(t *testing.T) {
	// Invalid YAML: unclosed brace
	content := `{ invalid yaml: [`
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "golemui_driver_invalid.yaml")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	_, err := config.LoadConfig(tmpFile)
	if err == nil {
		t.Error("Expected parse error for invalid YAML syntax, but got no error")
	}
}

func TestLoadConfig_MissingFields(t *testing.T) {
	// Missing "host" in UIDB
	content := `
uidb:
  port: 5432
  database: "golemui_core"
  user: "postgres"
  password: "password123"
business_db:
  host: "127.0.0.1"
  port: 5433
  database: "negocio_production"
  user: "biz_user"
  password: "biz_password"
`
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "golemui_driver_missing.yaml")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	_, err := config.LoadConfig(tmpFile)
	if err == nil {
		t.Error("Expected error due to missing required connection fields, but got no error")
	}
}

func TestLoadConfig_EntryPointViewID_Present(t *testing.T) {
	content := `
uidb:
  host: "localhost"
  port: 5432
  database: "golemui_core"
  user: "postgres"
  password: "password123"
business_db:
  host: "127.0.0.1"
  port: 5433
  database: "negocio_production"
  user: "biz_user"
  password: "biz_password"
entry_point_view_id: "dashboard"
`
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "golemui_driver_viewid.yaml")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	config, err := config.LoadConfig(tmpFile)
	if err != nil {
		t.Fatalf("Expected successful load, got error: %v", err)
	}

	if config.EntryPointViewID != "dashboard" {
		t.Errorf("expected EntryPointViewID %q, got %q", "dashboard", config.EntryPointViewID)
	}
}

func TestLoadConfig_EntryPointViewID_Absent(t *testing.T) {
	content := `
uidb:
  host: "localhost"
  port: 5432
  database: "golemui_core"
  user: "postgres"
  password: "password123"
business_db:
  host: "127.0.0.1"
  port: 5433
  database: "negocio_production"
  user: "biz_user"
  password: "biz_password"
`
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "golemui_driver_no_viewid.yaml")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	config, err := config.LoadConfig(tmpFile)
	if err != nil {
		t.Fatalf("Expected successful load, got error: %v", err)
	}

	if config.EntryPointViewID != "" {
		t.Errorf("expected EntryPointViewID to be empty string when absent, got %q", config.EntryPointViewID)
	}
}

func TestLoadConfig_LayoutQuery_Present(t *testing.T) {
	content := `
uidb:
  host: "localhost"
  port: 5432
  database: "golemui_core"
  user: "postgres"
  password: "password123"
business_db:
  host: "127.0.0.1"
  port: 5433
  database: "negocio_production"
  user: "biz_user"
  password: "biz_password"
layout_query: "SELECT col FROM tbl WHERE id = $1"
`
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "golemui_driver_layout_query.yaml")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	config, err := config.LoadConfig(tmpFile)
	if err != nil {
		t.Fatalf("Expected successful load, got error: %v", err)
	}

	if config.LayoutQuery != "SELECT col FROM tbl WHERE id = $1" {
		t.Errorf("expected LayoutQuery %q, got %q", "SELECT col FROM tbl WHERE id = $1", config.LayoutQuery)
	}
}

func TestLoadConfig_LayoutQuery_Absent(t *testing.T) {
	content := `
uidb:
  host: "localhost"
  port: 5432
  database: "golemui_core"
  user: "postgres"
  password: "password123"
business_db:
  host: "127.0.0.1"
  port: 5433
  database: "negocio_production"
  user: "biz_user"
  password: "biz_password"
`
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "golemui_driver_no_layout_query.yaml")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	config, err := config.LoadConfig(tmpFile)
	if err != nil {
		t.Fatalf("Expected successful load, got error: %v", err)
	}

	if config.LayoutQuery != "" {
		t.Errorf("expected LayoutQuery to be empty string when absent, got %q", config.LayoutQuery)
	}
}
