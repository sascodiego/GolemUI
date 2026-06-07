package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"GolemUI/pkg/config"
	"GolemUI/pkg/db"
	"GolemUI/pkg/eventbus"
	"GolemUI/pkg/ui"
)

type App struct {
	Config   *config.BootstrapConfig
	DB       *db.DB
	EventBus eventbus.EventBus
	FyneApp  fyne.App
	Window   fyne.Window
}

var initDB = db.InitDB

func sanitizeLocale() {
	lang := strings.TrimSpace(os.Getenv("LANG"))
	lcAll := strings.TrimSpace(os.Getenv("LC_ALL"))
	isInvalid := func(v string) bool {
		return v == "" || v == "C" || v == "POSIX"
	}

	if lcAll == "C" || lcAll == "POSIX" {
		os.Setenv("LC_ALL", "en_US.UTF-8")
	}

	if isInvalid(lang) && (isInvalid(lcAll) || lcAll == "en_US.UTF-8") {
		os.Setenv("LANG", "en_US.UTF-8")
	}
}

func RunBootstrap(ctx context.Context, cfg *config.BootstrapConfig, runWindow bool, fyneApp fyne.App) (*App, error) {
	// 0. Sanitize locale before Fyne initialization
	sanitizeLocale()

	// 1. Convert lua ConfigConexion to db Config
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

	// Setup navigation callback
	ui.Navigate = func(vID string) {
		log.Printf("[UI/Navigation] Navigating to screen %q", vID)
		node, err := ui.LoadScreen(ctx, ui.CorePool, vID, cfg.LayoutQuery)
		if err != nil {
			log.Printf("[UI/Navigation] Error loading screen %q: %v", vID, err)
			return
		}
		newUI, err := ui.Compose(node, vID)
		if err != nil {
			log.Printf("[UI/Navigation] Error composing screen %q: %v", vID, err)
			return
		}
		win.SetContent(newUI)
	}

	// 4. Load home screen from core database (pkg/ui)
	vistaID := cfg.EntryPointViewID
	if vistaID == "" {
		vistaID = "home"
	}

	homeNode, err := ui.LoadScreen(ctx, ui.CorePool, vistaID, cfg.LayoutQuery)
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
	pflag.String("config", "golemui_driver.yaml", "Path to YAML configuration file")
	pflag.String("view", "", "Override entry point view ID (overrides config and env)")
	pflag.Parse()

	configPath, _ := pflag.CommandLine.GetString("config")

	ctx := context.Background()
	log.Printf("Starting GolemUI — config: %s", configPath)

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Bootstrap error: %v", err)
	}

	// Env/flag overrides via Viper
	v := viper.New()
	v.SetEnvPrefix("GOLEMUI")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// View resolution: pflag > env > config file
	viewOverride, _ := pflag.CommandLine.GetString("view")
	if viewOverride != "" {
		cfg.EntryPointViewID = viewOverride
	} else if envView := v.GetString("entry_point_view_id"); envView != "" {
		cfg.EntryPointViewID = envView
	}

	_, err = RunBootstrap(ctx, cfg, true, nil)
	if err != nil {
		log.Fatalf("Bootstrap error: %v", err)
	}
}
