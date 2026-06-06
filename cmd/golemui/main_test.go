package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"GolemUI/pkg/db"
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

func TestRunBootstrap_MissingConfig(t *testing.T) {
	ctx := context.Background()
	_, err := RunBootstrap(ctx, "non_existent_config.lua", false, nil)
	if err == nil {
		t.Error("expected error due to missing configuration file, got nil")
	}
}

func TestRunBootstrap_DatabaseFailure(t *testing.T) {
	content := `
golemui_driver = {
    UIDB = {
        Host = "unreachable_core_db_host",
        Port = 5432,
        Database = "golemui_core",
        User = "postgres",
        Password = "password"
    },
    BusinessDB = {
        Host = "unreachable_biz_db_host",
        Port = 5432,
        Database = "negocio_production",
        User = "postgres",
        Password = "password"
    },
    EntryPointQuery = "SELECT * FROM golemui.layouts LIMIT 1"
}
`
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "golemui_driver.lua")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	ctx := context.Background()
	testApp := test.NewApp()

	_, err := RunBootstrap(ctx, tmpFile, false, testApp)
	if err == nil {
		t.Fatal("expected database connection failure, got nil")
	}

	if !strings.Contains(err.Error(), "core pool") && !strings.Contains(err.Error(), "ping") && !strings.Contains(err.Error(), "dial") {
		t.Errorf("expected error to relate to database connection, got: %v", err)
	}
}

func TestRunBootstrap_InvalidLuaConfigTable(t *testing.T) {
	content := `
-- Missing golemui_driver table
some_other_driver = {
    UIDB = {}
}
`
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "golemui_driver_invalid.lua")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	ctx := context.Background()
	testApp := test.NewApp()

	_, err := RunBootstrap(ctx, tmpFile, false, testApp)
	if !strings.Contains(err.Error(), "golemui_driver table not found") {
		t.Errorf("expected error to mention golemui_driver table, got: %v", err)
	}
}

func TestRunBootstrap_Success(t *testing.T) {
	// Reemplazar initDB por una función mock que retorna un mock db pool
	oldInitDB := initDB
	defer func() {
		initDB = oldInitDB
		ui.BusinessPool = nil
		ui.CorePool = nil
	}()

	coreMock := db.NewMockDBPool()
	bizMock := db.NewMockDBPool()

	// Register vista query so LoadScreen can load the home screen from DB
	coreMock.RegisterQuery(
		ui.DefaultLayoutQuery,
		[]string{"config_columnas"},
		[][]any{{`{"area":"home_root","component_ref":"container","layout":{"type":"vertical"},"children":[{"area":"header","component_ref":"label","label":"Welcome to GolemUI Desktop Client"}]}`}},
		nil,
	)

	initDB = func(ctx context.Context, coreCfg db.Config, bizCfg db.Config) (*db.DB, error) {
		return &db.DB{
			CorePool:     coreMock,
			BusinessPool: bizMock,
		}, nil
	}

	content := `
golemui_driver = {
    UIDB = {
        Host = "localhost",
        Port = 5432,
        Database = "golemui_core",
        User = "postgres",
        Password = "password"
    },
    BusinessDB = {
        Host = "localhost",
        Port = 5432,
        Database = "negocio_production",
        User = "postgres",
        Password = "password"
    },
    EntryPointQuery = "SELECT * FROM golemui.layouts LIMIT 1",
    EntryPointViewID = "home"
}
`
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "golemui_driver_success.lua")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	ctx := context.Background()
	testApp := test.NewApp()

	appInstance, err := RunBootstrap(ctx, tmpFile, false, testApp)
	if err != nil {
		t.Fatalf("unexpected bootstrap error: %v", err)
	}

	if appInstance == nil {
		t.Fatal("expected app instance, got nil")
	}

	if appInstance.DB == nil {
		t.Fatal("expected database instance inside app, got nil")
	}

	// Verificar que el pool sea el mock
	if _, ok := appInstance.DB.CorePool.(*db.MockDBPool); !ok {
		t.Error("expected CorePool to be MockDBPool")
	}

	if ui.BusinessPool != appInstance.DB.BusinessPool {
		t.Errorf("expected ui.BusinessPool to match DB.BusinessPool, got %v, want %v", ui.BusinessPool, appInstance.DB.BusinessPool)
	}

	// Verify ui.CorePool is wired to the core database pool
	if ui.CorePool != coreMock {
		t.Error("expected ui.CorePool to be wired to the core mock pool")
	}
}

func TestRunBootstrap_DefaultVistaID(t *testing.T) {
	// Verifies that when EntryPointViewID is absent from config, it defaults to "home"
	oldInitDB := initDB
	defer func() {
		initDB = oldInitDB
		ui.BusinessPool = nil
		ui.CorePool = nil
	}()

	coreMock := db.NewMockDBPool()
	bizMock := db.NewMockDBPool()

	// Register vista query for default "home" vista (no EntryPointViewID in config)
	coreMock.RegisterQuery(
		ui.DefaultLayoutQuery,
		[]string{"config_columnas"},
		[][]any{{`{"area":"default_root","component_ref":"container","layout":{"type":"vertical"},"children":[{"area":"title","component_ref":"label","label":"Default Home"}]}`}},
		nil,
	)

	initDB = func(ctx context.Context, coreCfg db.Config, bizCfg db.Config) (*db.DB, error) {
		return &db.DB{
			CorePool:     coreMock,
			BusinessPool: bizMock,
		}, nil
	}

	// Config WITHOUT EntryPointViewID — should default to "home"
	content := `
golemui_driver = {
    UIDB = {
        Host = "localhost",
        Port = 5432,
        Database = "golemui_core",
        User = "postgres",
        Password = "password"
    },
    BusinessDB = {
        Host = "localhost",
        Port = 5432,
        Database = "negocio_production",
        User = "postgres",
        Password = "password"
    },
    EntryPointQuery = "SELECT * FROM golemui.layouts LIMIT 1"
}
`
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "golemui_driver_default_vista.lua")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	ctx := context.Background()
	testApp := test.NewApp()

	appInstance, err := RunBootstrap(ctx, tmpFile, false, testApp)
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
	oldInitDB := initDB
	defer func() {
		initDB = oldInitDB
		ui.BusinessPool = nil
		ui.CorePool = nil
	}()

	coreMock := db.NewMockDBPool()
	bizMock := db.NewMockDBPool()

	// Register vista query to return ErrNoRows — simulates missing vista
	coreMock.RegisterQuery(
		ui.DefaultLayoutQuery,
		[]string{"config_columnas"},
		nil,
		pgx.ErrNoRows,
	)

	initDB = func(ctx context.Context, coreCfg db.Config, bizCfg db.Config) (*db.DB, error) {
		return &db.DB{
			CorePool:     coreMock,
			BusinessPool: bizMock,
		}, nil
	}

	content := `
golemui_driver = {
    UIDB = {
        Host = "localhost",
        Port = 5432,
        Database = "golemui_core",
        User = "postgres",
        Password = "password"
    },
    BusinessDB = {
        Host = "localhost",
        Port = 5432,
        Database = "negocio_production",
        User = "postgres",
        Password = "password"
    },
    EntryPointViewID = "nonexistent"
}
`
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "golemui_driver_failure.lua")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	ctx := context.Background()
	testApp := test.NewApp()

	appInstance, err := RunBootstrap(ctx, tmpFile, false, testApp)
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

func TestRunBootstrap_IntegrationWithLogs(t *testing.T) {
	oldInitDB := initDB
	defer func() {
		initDB = oldInitDB
		ui.BusinessPool = nil
		ui.CorePool = nil
	}()

	coreMock := db.NewMockDBPool()
	bizMock := db.NewMockDBPool()

	// Register layout query for transacciones_list
	layoutJSON := `{"area":"root","component_ref":"container","layout":{"type":"grid","columns":["1fr"],"rows":["30px","50px","1fr"],"gap":"10"},"children":[{"area":"header","component_ref":"label","label":"Listado de Transacciones"},{"area":"filters_container","component_ref":"container","layout":{"type":"grid","columns":["250px","200px","120px"],"rows":["40px"],"gap":"10"},"children":[{"area":"emp_cod_filter","component_ref":"text_input","placeholder":"Filtrar por Empresa (LIKE)","bind_to":"emp_cod"},{"area":"status_filter","component_ref":"text_input","placeholder":"Filtrar por Status (LIKE)","bind_to":"status"},{"area":"search_button","component_ref":"button","label":"Actualizar","submit_action":"search"}]},{"area":"transactions_grid","component_ref":"data_grid","filter_mode":"server","data_source":"SELECT id, emp_cod, monto, status FROM public.transacciones WHERE emp_cod LIKE $1 AND status LIKE $2","filter_keys":["emp_cod","status"]}]}`
	coreMock.RegisterQuery(
		ui.DefaultLayoutQuery,
		[]string{"config_columnas"},
		[][]any{{layoutJSON}},
		nil,
	)

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

	initDB = func(ctx context.Context, coreCfg db.Config, bizCfg db.Config) (*db.DB, error) {
		return &db.DB{
			CorePool:     coreMock,
			BusinessPool: bizMock,
		}, nil
	}

	content := `
golemui_driver = {
    UIDB = {
        Host = "localhost",
        Port = 5432,
        Database = "golemui_core",
        User = "postgres",
        Password = "password"
    },
    BusinessDB = {
        Host = "localhost",
        Port = 5432,
        Database = "negocio_production",
        User = "postgres",
        Password = "password"
    },
    EntryPointViewID = "transacciones_list"
}
`
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "golemui_driver_integration.lua")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	ctx := context.Background()
	testApp := test.NewApp()

	_, err := RunBootstrap(ctx, tmpFile, false, testApp)
	if err != nil {
		t.Fatalf("expected successful bootstrap, got error: %v", err)
	}
}




