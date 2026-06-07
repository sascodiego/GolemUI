package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"GolemUI/pkg/db"
	"GolemUI/pkg/lua"
	"GolemUI/pkg/ui"
	"fyne.io/fyne/v2/test"
	"github.com/jackc/pgx/v5"
)

func TestSanitizeLocale_LangC(t *testing.T) {
	t.Setenv("LANG", "C")
	t.Setenv("LC_ALL", "")
	sanitizeLocale()
	if os.Getenv("LANG") != "en_US.UTF-8" {
		t.Errorf("expected LANG to be %q, got %q", "en_US.UTF-8", os.Getenv("LANG"))
	}
	if os.Getenv("LC_ALL") != "" {
		t.Errorf("expected LC_ALL to remain empty, got %q", os.Getenv("LC_ALL"))
	}
}

func TestSanitizeLocale_LCAllPOSIX(t *testing.T) {
	t.Setenv("LANG", "")
	t.Setenv("LC_ALL", "POSIX")
	sanitizeLocale()
	if os.Getenv("LANG") != "en_US.UTF-8" {
		t.Errorf("expected LANG to be %q, got %q", "en_US.UTF-8", os.Getenv("LANG"))
	}
	if os.Getenv("LC_ALL") != "en_US.UTF-8" {
		t.Errorf("expected LC_ALL to be %q, got %q", "en_US.UTF-8", os.Getenv("LC_ALL"))
	}
}

func TestSanitizeLocale_BothEmpty(t *testing.T) {
	t.Setenv("LANG", "")
	t.Setenv("LC_ALL", "")
	sanitizeLocale()
	if os.Getenv("LANG") != "en_US.UTF-8" {
		t.Errorf("expected LANG to be %q, got %q", "en_US.UTF-8", os.Getenv("LANG"))
	}
	if os.Getenv("LC_ALL") != "" {
		t.Errorf("expected LC_ALL to remain empty, got %q", os.Getenv("LC_ALL"))
	}
}

func TestSanitizeLocale_ValidLangUntouched(t *testing.T) {
	t.Setenv("LANG", "es_AR.UTF-8")
	t.Setenv("LC_ALL", "")
	sanitizeLocale()
	if os.Getenv("LANG") != "es_AR.UTF-8" {
		t.Errorf("expected LANG to remain %q, got %q", "es_AR.UTF-8", os.Getenv("LANG"))
	}
	if os.Getenv("LC_ALL") != "" {
		t.Errorf("expected LC_ALL to remain empty, got %q", os.Getenv("LC_ALL"))
	}
}

func TestSanitizeLocale_LCAllValid(t *testing.T) {
	t.Setenv("LANG", "")
	t.Setenv("LC_ALL", "en_US.UTF-8")
	sanitizeLocale()
	if os.Getenv("LANG") != "en_US.UTF-8" {
		t.Errorf("expected LANG to be set to %q since it was empty, got %q", "en_US.UTF-8", os.Getenv("LANG"))
	}
	if os.Getenv("LC_ALL") != "en_US.UTF-8" {
		t.Errorf("expected LC_ALL to remain %q, got %q", "en_US.UTF-8", os.Getenv("LC_ALL"))
	}
}

func TestSanitizeLocale_LCAllCOverridesValidLang(t *testing.T) {
	t.Setenv("LANG", "es_AR.UTF-8")
	t.Setenv("LC_ALL", "C")
	sanitizeLocale()
	if os.Getenv("LANG") != "es_AR.UTF-8" {
		t.Errorf("expected LANG to remain %q, got %q", "es_AR.UTF-8", os.Getenv("LANG"))
	}
	if os.Getenv("LC_ALL") != "en_US.UTF-8" {
		t.Errorf("expected LC_ALL to be %q, got %q", "en_US.UTF-8", os.Getenv("LC_ALL"))
	}
}

func TestSanitizeLocale_BothValidUntouched(t *testing.T) {
	t.Setenv("LANG", "es_AR.UTF-8")
	t.Setenv("LC_ALL", "es_AR.UTF-8")
	sanitizeLocale()
	if os.Getenv("LANG") != "es_AR.UTF-8" {
		t.Errorf("expected LANG to remain %q, got %q", "es_AR.UTF-8", os.Getenv("LANG"))
	}
	if os.Getenv("LC_ALL") != "es_AR.UTF-8" {
		t.Errorf("expected LC_ALL to remain %q, got %q", "es_AR.UTF-8", os.Getenv("LC_ALL"))
	}
}

// helper: build a valid config struct for tests
func testConfig() *lua.BootstrapConfig {
	return &lua.BootstrapConfig{
		UIDB:       lua.ConfigConexion{Host: "localhost", Port: 5432, Database: "golemui_core", User: "postgres", Password: "password"},
		BusinessDB: lua.ConfigConexion{Host: "localhost", Port: 5432, Database: "negocio_production", User: "postgres", Password: "password"},
	}
}

// helper: write YAML config to temp file and load it via LoadConfig
func loadTestYAML(t *testing.T, content string) *lua.BootstrapConfig {
	t.Helper()
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "golemui_driver_test.yaml")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}
	cfg, err := lua.LoadConfig(tmpFile)
	if err != nil {
		t.Fatalf("failed to load test config: %v", err)
	}
	return cfg
}

// helper: setup mock DB pools and inject initDB
func setupMockDB(t *testing.T, layoutJSON string, layoutErr error) (*db.MockDBPool, *db.MockDBPool) {
	t.Helper()

	oldInitDB := initDB
	t.Cleanup(func() {
		initDB = oldInitDB
		ui.BusinessPool = nil
		ui.CorePool = nil
	})

	coreMock := db.NewMockDBPool()
	bizMock := db.NewMockDBPool()

	if layoutJSON != "" || layoutErr != nil {
		coreMock.RegisterQuery(
			ui.DefaultLayoutQuery,
			[]string{"config_columnas"},
			[][]any{{layoutJSON}},
			layoutErr,
		)
	}

	initDB = func(ctx context.Context, coreCfg db.Config, bizCfg db.Config) (*db.DB, error) {
		return &db.DB{
			CorePool:     coreMock,
			BusinessPool: bizMock,
		}, nil
	}

	return coreMock, bizMock
}

func TestRunBootstrap_MissingConfig(t *testing.T) {
	_, err := lua.LoadConfig("non_existent_config.yaml")
	if err == nil {
		t.Error("expected error due to missing configuration file, got nil")
	}
}

func TestRunBootstrap_DatabaseFailure(t *testing.T) {
	content := `
uidb:
  host: "unreachable_core_db_host"
  port: 5432
  database: "golemui_core"
  user: "postgres"
  password: "password"
business_db:
  host: "unreachable_biz_db_host"
  port: 5432
  database: "negocio_production"
  user: "postgres"
  password: "password"
entry_point_query: "SELECT * FROM golemui.layouts LIMIT 1"
`
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "golemui_driver.yaml")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	ctx := context.Background()
	testApp := test.NewApp()

	cfg, err := lua.LoadConfig(tmpFile)
	if err != nil {
		t.Fatalf("failed to load valid config: %v", err)
	}

	// Use real initDB (not mocked) to trigger DB connection failure
	_, err = RunBootstrap(ctx, cfg, false, testApp)
	if err == nil {
		t.Fatal("expected database connection failure, got nil")
	}

	if !strings.Contains(err.Error(), "database") {
		t.Errorf("expected error to relate to database, got: %v", err)
	}
}

func TestRunBootstrap_InvalidConfigMissingFields(t *testing.T) {
	// YAML missing required UIDB fields — LoadConfig should fail
	content := `
uidb:
  host: "localhost"
business_db:
  host: "localhost"
  port: 5432
  database: "negocio_production"
  user: "postgres"
  password: "password"
`
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "golemui_driver_invalid.yaml")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	_, err := lua.LoadConfig(tmpFile)
	if err == nil {
		t.Error("expected error for config with missing required fields, got nil")
	}
}

func TestRunBootstrap_Success(t *testing.T) {
	coreMock, _ := setupMockDB(t, `{"area":"home_root","component_ref":"container","layout":{"type":"vertical"},"children":[{"area":"header","component_ref":"label","label":"Welcome to GolemUI Desktop Client"}]}`, nil)

	cfg := testConfig()
	cfg.EntryPointViewID = "home"

	ctx := context.Background()
	testApp := test.NewApp()

	appInstance, err := RunBootstrap(ctx, cfg, false, testApp)
	if err != nil {
		t.Fatalf("unexpected bootstrap error: %v", err)
	}

	if appInstance == nil {
		t.Fatal("expected app instance, got nil")
	}

	if appInstance.DB == nil {
		t.Fatal("expected database instance inside app, got nil")
	}

	if _, ok := appInstance.DB.CorePool.(*db.MockDBPool); !ok {
		t.Error("expected CorePool to be MockDBPool")
	}

	if ui.BusinessPool != appInstance.DB.BusinessPool {
		t.Errorf("expected ui.BusinessPool to match DB.BusinessPool")
	}

	if ui.CorePool != coreMock {
		t.Error("expected ui.CorePool to be wired to the core mock pool")
	}
}

func TestRunBootstrap_DefaultVistaID(t *testing.T) {
	// Verifies that when EntryPointViewID is empty, it defaults to "home"
	coreMock, _ := setupMockDB(t, `{"area":"default_root","component_ref":"container","layout":{"type":"vertical"},"children":[{"area":"title","component_ref":"label","label":"Default Home"}]}`, nil)

	cfg := testConfig()
	// EntryPointViewID left empty → should default to "home"

	ctx := context.Background()
	testApp := test.NewApp()

	appInstance, err := RunBootstrap(ctx, cfg, false, testApp)
	if err != nil {
		t.Fatalf("expected bootstrap to succeed with default vistaID 'home', got error: %v", err)
	}

	if appInstance == nil {
		t.Fatal("expected app instance, got nil")
	}

	if ui.CorePool != coreMock {
		t.Error("expected ui.CorePool to be wired to the core mock pool")
	}
}

func TestRunBootstrap_LoadScreenFailure(t *testing.T) {
	_, _ = setupMockDB(t, "", pgx.ErrNoRows)

	cfg := testConfig()
	cfg.EntryPointViewID = "nonexistent"

	ctx := context.Background()
	testApp := test.NewApp()

	appInstance, err := RunBootstrap(ctx, cfg, false, testApp)
	if err == nil {
		t.Fatal("expected error when LoadScreen fails, got nil")
	}

	if !strings.Contains(err.Error(), "LoadScreen") {
		t.Errorf("expected error to mention LoadScreen, got: %v", err)
	}

	if appInstance != nil {
		t.Error("expected nil app instance on LoadScreen failure")
	}
}

func TestRunBootstrap_ViewOverrideWins(t *testing.T) {
	// Verifies that cfg.EntryPointViewID = "settings" is used directly
	// (view override is resolved in main(), so tests set the config directly)
	_, _ = setupMockDB(t, `{"area":"settings_root","component_ref":"container","layout":{"type":"vertical"},"children":[{"area":"title","component_ref":"label","label":"Settings Page"}]}`, nil)

	cfg := testConfig()
	cfg.EntryPointViewID = "settings" // "settings" wins over what config file might have said

	ctx := context.Background()
	testApp := test.NewApp()

	appInstance, err := RunBootstrap(ctx, cfg, false, testApp)
	if err != nil {
		t.Fatalf("expected bootstrap to succeed with viewOverride='settings', got error: %v", err)
	}

	if appInstance == nil {
		t.Fatal("expected app instance, got nil")
	}
}

func TestRunBootstrap_EmptyOverrideFallsThrough(t *testing.T) {
	// Verifies that cfg.EntryPointViewID from config is used when no override
	_, _ = setupMockDB(t, `{"area":"root","component_ref":"container","layout":{"type":"vertical"},"children":[{"area":"header","component_ref":"label","label":"Transacciones"}]}`, nil)

	cfg := testConfig()
	cfg.EntryPointViewID = "transacciones_list"

	ctx := context.Background()
	testApp := test.NewApp()

	appInstance, err := RunBootstrap(ctx, cfg, false, testApp)
	if err != nil {
		t.Fatalf("expected bootstrap to succeed using config's EntryPointViewID, got error: %v", err)
	}

	if appInstance == nil {
		t.Fatal("expected app instance, got nil")
	}
}

func TestRunBootstrap_BothEmptyDefaultsHome(t *testing.T) {
	// Verifies that when EntryPointViewID is empty, defaults to "home"
	_, _ = setupMockDB(t, `{"area":"home_root","component_ref":"container","layout":{"type":"vertical"},"children":[{"area":"title","component_ref":"label","label":"Default Home"}]}`, nil)

	cfg := testConfig()
	// EntryPointViewID left empty → should default to "home"

	ctx := context.Background()
	testApp := test.NewApp()

	appInstance, err := RunBootstrap(ctx, cfg, false, testApp)
	if err != nil {
		t.Fatalf("expected bootstrap to succeed with default vistaID 'home', got error: %v", err)
	}

	if appInstance == nil {
		t.Fatal("expected app instance, got nil")
	}
}

func TestRunBootstrap_IntegrationWithLogs(t *testing.T) {
	coreMock, bizMock := setupMockDB(t, `{"area":"root","component_ref":"container","layout":{"type":"grid","columns":["1fr"],"rows":["30px","50px","1fr"],"gap":"10"},"children":[{"area":"header","component_ref":"label","label":"Listado de Transacciones"},{"area":"filters_container","component_ref":"container","layout":{"type":"grid","columns":["250px","200px","120px"],"rows":["40px"],"gap":"10"},"children":[{"area":"emp_cod_filter","component_ref":"text_input","placeholder":"Filtrar por Empresa (LIKE)","bind_to":"emp_cod"},{"area":"status_filter","component_ref":"text_input","placeholder":"Filtrar por Status (LIKE)","bind_to":"status"},{"area":"search_button","component_ref":"button","label":"Actualizar","submit_action":"search"}]},{"area":"transactions_grid","component_ref":"data_grid","filter_mode":"server","data_source":"SELECT id, emp_cod, monto, status FROM public.transacciones WHERE emp_cod LIKE $1 AND status LIKE $2","filter_keys":["emp_cod","status"]}]}`, nil)

	// Register query for the grid data (initial call with empty arguments)
	gridCols := []string{"id", "emp_cod", "monto", "status"}
	gridRows := [][]any{
		{1, "GETNET", 1500.50, "APROBADA"},
		{2, "EMP01", 350.00, "RECHAZADA"},
	}
	bizMock.RegisterQuery(
		"SELECT id, emp_cod, monto, status FROM public.transacciones WHERE emp_cod LIKE $1 AND status LIKE $2",
		gridCols,
		gridRows,
		nil,
	)

	_ = coreMock // just to avoid unused warning

	cfg := testConfig()
	cfg.EntryPointViewID = "transacciones_list"

	ctx := context.Background()
	testApp := test.NewApp()

	_, err := RunBootstrap(ctx, cfg, false, testApp)
	if err != nil {
		t.Fatalf("expected successful bootstrap, got error: %v", err)
	}
}
