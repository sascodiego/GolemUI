# Proposal: base-golemui-client

## Intent
Establish a decoupled, production-ready foundation for the GolemUI Go client using Clean Architecture, a custom fractional layout engine, and thread-safe reactivity.

## Capabilities

### New Capabilities
- `client-bootstrap`: Gopher-Lua bootstrapper to parse connection configs and database metadata at startup.
- `composite-layout-engine`: Custom layout parser & arranger for fractional grids, flex containers, and tab pages in Fyne.
- `client-reactivity-broker`: Thread-safe, in-memory Pub/Sub event bus to dispatch dynamic UI updates without network loops.

### Modified Capabilities
- None (greenfield project).

## Affected Areas
- [go.mod](file:///src/GolemUI/go.mod): Core module definition & Fyne/Lua dependencies.
- [cmd/golemui/main.go](file:///src/GolemUI/cmd/golemui/main.go): Application entrypoint.
- [pkg/db/db.go](file:///src/GolemUI/pkg/db/db.go): Segregated DB connection manager.
- [pkg/lua/loader.go](file:///src/GolemUI/pkg/lua/loader.go): Ephemeral VM lifecycle and driver parser.
- [pkg/eventbus/eventbus.go](file:///src/GolemUI/pkg/eventbus/eventbus.go): Reactive event broker.
- [pkg/ui/compositor.go](file:///src/GolemUI/pkg/ui/compositor.go): Recursive layout tree compositor.
- [pkg/ui/layout.go](file:///src/GolemUI/pkg/ui/layout.go): Custom fractional grid layout.

## Approach
1. **Decoupled Architecture**: Structure the codebase into separate packages (`cmd/`, `pkg/db`, `pkg/lua`, `pkg/eventbus`, `pkg/ui`) to prevent circular dependencies.
2. **Custom Layout Composer**: Build a custom compositor implementing `fyne.Layout` to parse responsive row/column fraction ratios.
3. **Pub/Sub Broker**: Implement an in-memory event bus with `sync.RWMutex` to handle reactive UI bindings cleanly.
4. **Ephemeral VM**: Boot the Lua VM to load connections and shut it down immediately with `defer L.Close()`.

## Risks & Mitigations
- **UI Thread Safety**: Mutations outside Fyne's main loop trigger panics. *Mitigation*: Perform DB queries in background goroutines, applying results via Fyne's thread-safe update methods.
- **Lua VM Leaks**: Persistent VMs consume memory. *Mitigation*: Terminate VM immediately after initialization.
- **Circular Imports**: Event bus and UI elements reference each other. *Mitigation*: Design decoupled interfaces for message callbacks.

## Success Criteria
- Executing `go test ./...` succeeds.
- Binary parses connections and layouts from PostgreSQL.
- Local reactive event propagation latency remains under 5ms.

## Rollback Plan
- Clean the database using `docker-compose down && docker-compose up -d`.
- Remove the generated packages (`cmd/`, `pkg/`) and clean up `go.mod`/`go.sum`.
