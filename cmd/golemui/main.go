package main

import (
	"context"
	"fmt"
	"log"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"GolemUI/pkg/db"
	"GolemUI/pkg/eventbus"
	"GolemUI/pkg/lua"
	"GolemUI/pkg/ui"
)

type App struct {
	Config   *lua.BootstrapConfig
	DB       *db.DB
	EventBus eventbus.EventBus
	FyneApp  fyne.App
	Window   fyne.Window
}

var initDB = db.InitDB

func RunBootstrap(ctx context.Context, configPath string, runWindow bool, fyneApp fyne.App) (*App, error) {
	// 1. Configuration loading (pkg/lua)
	cfg, err := lua.LoadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config from %s: %w", configPath, err)
	}

	// Convert lua ConfigConexion to db Config
	coreCfg := db.Config{
		Host:     cfg.UIDB.Host,
		Port:     cfg.UIDB.Port,
		Database: cfg.UIDB.Database,
		User:     cfg.UIDB.User,
		Password: cfg.UIDB.Password,
	}
	bizCfg := db.Config{
		Host:     cfg.BusinessDB.Host,
		Port:     cfg.BusinessDB.Port,
		Database: cfg.BusinessDB.Database,
		User:     cfg.BusinessDB.User,
		Password: cfg.BusinessDB.Password,
	}

	// 2. Database pool initialization (pkg/db)
	dbPool, err := initDB(ctx, coreCfg, bizCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}
	ui.BusinessPool = dbPool.BusinessPool
	ui.CorePool = dbPool.CorePool

	// 3. Event bus setup (pkg/eventbus)
	eb := eventbus.NewEventBus()
	ui.LocalEventBus = eb

	// 3.5. Initialize Fyne app & Window
	if fyneApp == nil {
		fyneApp = app.New()
	}
	win := fyneApp.NewWindow("GolemUI Client")

	// 4. Load home screen from core database (pkg/ui)
	vistaID := cfg.EntryPointViewID
	if vistaID == "" {
		vistaID = "home"
	}

	homeNode, err := ui.LoadScreen(ctx, ui.CorePool, vistaID)
	if err != nil {
		dbPool.Close()
		return nil, fmt.Errorf("failed to load screen %q: %w", vistaID, err)
	}

	homeUI, err := ui.Compose(homeNode, vistaID)
	if err != nil {
		dbPool.Close()
		return nil, fmt.Errorf("failed to compose home UI: %w", err)
	}

	win.SetContent(homeUI)

	a := &App{
		Config:   cfg,
		DB:       dbPool,
		EventBus: eb,
		FyneApp:  fyneApp,
		Window:   win,
	}

	if runWindow {
		win.ShowAndRun()
	}

	return a, nil
}

func main() {
	ctx := context.Background()
	log.Println("Starting GolemUI desktop client bootstrap...")
	_, err := RunBootstrap(ctx, "golemui_driver.lua", true, nil)
	if err != nil {
		log.Fatalf("Bootstrap error: %v", err)
	}
}

