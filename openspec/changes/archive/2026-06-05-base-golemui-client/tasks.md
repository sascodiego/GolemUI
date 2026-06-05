# Tasks Breakdown: base-golemui-client

## Review Workload Forecast
Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: stacked-to-main
400-line budget risk: High

## PR 1: Core Bootstrapping & DB Connectivity
- [x] **Task 1.1: Go Module Initialization**
  - Path: [go.mod](file:///src/GolemUI/go.mod)
  - Action: Initialize the Go module `golemui` and add third-party dependencies (`fyne.io/fyne/v2`, `github.com/jackc/pgx/v5`, `github.com/yuin/gopher-lua`).
- [x] **Task 1.2: Database Connection Pools**
  - Path: [pkg/db/db.go](file:///src/GolemUI/pkg/db/db.go)
  - Action: Implement separated pgx connection pools for `golemui_core` and `negocio_production` databases with connection health checks.
  - Tests: Write integration tests in [pkg/db/db_test.go](file:///src/GolemUI/pkg/db/db_test.go) asserting connection readiness and error handling.
- [x] **Task 1.3: Lua Configuration Loader**
  - Path: [pkg/lua/loader.go](file:///src/GolemUI/pkg/lua/loader.go)
  - Action: Implement config bootstrapping via an ephemeral Gopher-Lua VM, parsing db credentials. Ensure clean release of VM resources with defer L.Close().
  - Tests: Add unit tests in [pkg/lua/loader_test.go](file:///src/GolemUI/pkg/lua/loader_test.go) with mock configuration files.

## PR 2: Reactive Event Bus
- [x] **Task 2.1: Concurrency-Safe Event Bus**
  - Path: [pkg/eventbus/eventbus.go](file:///src/GolemUI/pkg/eventbus/eventbus.go)
  - Action: Implement the `EventBus` interface to support thread-safe publishing and subscription mechanisms using channels and sync.Map.
  - Tests: Concurrency unit tests in [pkg/eventbus/eventbus_test.go](file:///src/GolemUI/pkg/eventbus/eventbus_test.go) asserting 50+ simultaneous readers/writers and leak-free operations.

## PR 3: UI Rendering Engine
- [x] **Task 3.1: Fractional Layout Engine**
  - Path: [pkg/ui/layout.go](file:///src/GolemUI/pkg/ui/layout.go)
  - Action: Build custom Fyne Layout calculating fractional layout sizing (e.g. `fr` and `auto`).
  - Tests: Assert bounds and size calculations in [pkg/ui/layout_test.go](file:///src/GolemUI/pkg/ui/layout_test.go).
- [x] **Task 3.2: Recursive Compositor**
  - Path: [pkg/ui/compositor.go](file:///src/GolemUI/pkg/ui/compositor.go)
  - Action: Implement dynamic construction of visual Fyne component trees recursively from `NodeMeta` configurations.
  - Tests: Test structural rendering of nested components in [pkg/ui/compositor_test.go](file:///src/GolemUI/pkg/ui/compositor_test.go).
- [x] **Task 3.3: Main Client Application Entrypoint**
  - Path: [cmd/golemui/main.go](file:///src/GolemUI/cmd/golemui/main.go)
  - Action: Develop main Go entrypoint to initialize configuration load, connect databases, instanciate the reactive EventBus, and spin up the main Fyne GUI thread.
