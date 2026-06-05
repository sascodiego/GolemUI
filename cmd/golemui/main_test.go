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
)

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
	}()

	initDB = func(ctx context.Context, coreCfg db.Config, bizCfg db.Config) (*db.DB, error) {
		return &db.DB{
			CorePool:     db.NewMockDBPool(),
			BusinessPool: db.NewMockDBPool(),
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
    EntryPointQuery = "SELECT * FROM golemui.layouts LIMIT 1"
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
}



