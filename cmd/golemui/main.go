package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"GolemUI/pkg/config"
	"GolemUI/pkg/dataaccess"
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

// parseNavigateTarget splits a vistaID string into a clean screen ID and
// optional query parameters. Returns the clean vistaID and a map of parsed
// key-value pairs (URL-decoded). Returns (vID, nil) if no query string.
func parseNavigateTarget(vID string) (string, map[string]string) {
	idx := strings.Index(vID, "?")
	if idx < 0 {
		return vID, nil
	}
	cleanID := vID[:idx]
	if cleanID == "" {
		return vID, nil // malformed — treat as plain vistaID
	}
	rawQuery := vID[idx+1:]
	if rawQuery == "" {
		return cleanID, nil
	}
	params := make(map[string]string)
	for _, pair := range strings.Split(rawQuery, "&") {
		if pair == "" {
			continue
		}
		kv := strings.SplitN(pair, "=", 2)
		key, err := url.QueryUnescape(kv[0])
		if err != nil {
			key = kv[0] // fallback to raw key
		}
		if len(kv) == 1 {
			params[key] = ""
		} else {
			val, err := url.QueryUnescape(kv[1])
			if err != nil {
				val = kv[1] // fallback to raw value
			}
			params[key] = val
		}
	}
	if len(params) == 0 {
		return cleanID, nil
	}
	return cleanID, params
}

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
	// Wire DataSource for business data queries (Layer 1 boundary)
	ui.DS = dataaccess.NewSQLDataSource(dbPool.BusinessPool)

	// Wire ColumnWidthResolver for Layer 2/3 metadata
	ui.CWR = dataaccess.NewColumnWidthResolver(dbPool.CorePool)

	// 3. Event bus setup (pkg/eventbus)
	eb := eventbus.NewEventBus()
	ui.LocalEventBus = eb

	// 3.5. Initialize Fyne app & Window
	if fyneApp == nil {
		fyneApp = app.New()
	}
	win := fyneApp.NewWindow("GolemUI Client")

	// Setup navigation menu and split layout
	menuItems, err := ui.LoadNavigationMenu(ctx, dbPool.CorePool)
	if err != nil {
		dbPool.Close()
		return nil, fmt.Errorf("failed to load navigation menu: %w", err)
	}
	navTree := ui.BuildNavTree(menuItems)

	// Create split layout: sidebar (left) + dynamic content area (right)
	mainContainer := container.NewMax()
	sidebarScroll := container.NewVScroll(navTree.Widget())
	split := container.NewHSplit(sidebarScroll, mainContainer)
	split.SetOffset(0.2)

	// Setup navigation callback — updates only the right panel
	var cleanupMu sync.Mutex
	var prevCleanup func()

	ui.Navigate = func(vID string) {
		log.Printf("[UI/Navigation] Navigating to screen %q", vID)
		ui.NavigateWG.Add(1)
		go func() {
			// Parse query string
			cleanVistaID, queryParams := parseNavigateTarget(vID)

			// Tear down previous screen before loading the new one
			cleanupMu.Lock()
			if prevCleanup != nil {
				prevCleanup()
				prevCleanup = nil
			}
			cleanupMu.Unlock()

			node, err := ui.LoadScreen(ctx, dbPool.CorePool, cleanVistaID, cfg.LayoutQuery)
			if err != nil {
				log.Printf("[UI/Navigation] Error loading screen %q: %v", cleanVistaID, err)
				ui.NavigateWG.Done()
				return
			}

			var newUI fyne.CanvasObject
			var cleanup func()
			if queryParams != nil {
				newUI, cleanup, err = ui.ComposeWithParams(node, cleanVistaID, queryParams)
			} else {
				newUI, cleanup, err = ui.Compose(node, cleanVistaID)
			}
			if err != nil {
				log.Printf("[UI/Navigation] Error composing screen %q: %v", cleanVistaID, err)
				ui.NavigateWG.Done()
				return
			}

			cleanupMu.Lock()
			prevCleanup = cleanup
			cleanupMu.Unlock()

			fyne.Do(func() {
				mainContainer.Objects = []fyne.CanvasObject{newUI}
				mainContainer.Refresh()
				navTree.SelectByVistaID(cleanVistaID)
				ui.NavigateWG.Done()
			})
		}()
	}

	// 4. Load home screen from core database (pkg/ui)
	vistaID := cfg.EntryPointViewID
	if vistaID == "" {
		vistaID = "home"
	}

	homeNode, err := ui.LoadScreen(ctx, dbPool.CorePool, vistaID, cfg.LayoutQuery)
	if err != nil {
		dbPool.Close()
		return nil, fmt.Errorf("failed to load screen %q: %w", vistaID, err)
	}

	homeUI, homeCleanup, err := ui.Compose(homeNode, vistaID)
	if err != nil {
		dbPool.Close()
		return nil, fmt.Errorf("failed to compose home UI: %w", err)
	}
	prevCleanup = homeCleanup

	// Place home screen into the right panel and set the split as window content
	mainContainer.Objects = []fyne.CanvasObject{homeUI}
	win.SetContent(split)

	a := &App{
		Config:   cfg,
		DB:       dbPool,
		EventBus: eb,
		FyneApp:  fyneApp,
		Window:   win,
	}

	win.SetOnClosed(func() {
		cleanupMu.Lock()
		if prevCleanup != nil {
			prevCleanup()
			prevCleanup = nil
		}
		cleanupMu.Unlock()
		dbPool.Close()
	})

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
