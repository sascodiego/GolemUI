# Exploration: GolemUI Go Client Base Structure

This document explores the architectural design and implementation approach for the **GolemUI** Go desktop/mobile client base structure. The client is a reactive, data-driven engine utilizing Fyne (Capa 4) that translates dynamic logical UI component specifications from PostgreSQL (Capa 2 and 3) into concrete native canvas interfaces.

---

## Current State

The current repository represents a greenfield workspace for the Go client binary. The Postgres database schema and initialization scripts reside in `/docker/init-db/` and data payloads are in `/data/`. There is no existing Go module, directory hierarchy, layout parser, event system, or Lua bootstrap integration. This allows us to establish a production-first, clean architectural foundation from scratch.

---

## Affected Areas

1. **Project Root & Module Definition**: Go module initialization (`go.mod`) and package layout (`cmd/`, `pkg/`).
2. **Layout Compositor Layer (`pkg/ui`)**: Recursive composite layout engine for nested grid, flex, and tab nodes utilizing Fyne widgets.
3. **Reactivity Engine (`pkg/eventbus`)**: In-memory Event Bus for fast client-side widget reactivity using `bind_to` specifications.
4. **Bootstrapping Layer (`pkg/lua`)**: Gopher-Lua virtual machine integration to load connection metadata and entrypoint queries.
5. **Database Manager (`pkg/db`)**: Core and Business segregated connection pools.

---

## Approaches

### 1. Module Initialization & Hierarchy

We must layout the project to support clean-architecture decoupling. We evaluate two structural patterns:

#### Approach 1.A: Flat/Monolithic Package Structure
- All files in a single main package or flat package structure under `/src/GolemUI`.
- **Pros**: Zero import cycle risks; fast to implement initially.
- **Cons**: High coupling; lacks structural division for distinct components like UI, Lua driver, DB connections, and Event Bus.
- **Effort**: Low (1-2 hours)

#### Approach 1.B: Decoupled Domain Directory Hierarchy (Recommended)
- Standard Go project layout dividing the entry point (`cmd/golemui/`) from the reusable packages (`pkg/ui`, `pkg/eventbus`, `pkg/lua`, `pkg/db`).
- **Pros**: Strict boundaries prevent circular dependencies; scales cleanly as plugins and complex widgets are added.
- **Cons**: Requires careful planning of internal APIs to avoid dependency loops.
- **Effort**: Medium (2-3 hours)

---

### 2. Recursive Layout Parser & Custom Fyne Compositor

Fyne's standard containers do not support fractional widths or heights directly (e.g. `"2fr, 1.2fr, 1.2fr"`). We explore layout approaches:

#### Approach 2.A: Standard Fyne Grid & Flex Fallbacks
- Fallback to standard equal-width grid columns (`container.NewGridWithColumns`) and simple HBox/VBox.
- **Pros**: Built-in Fyne containers, low complexity.
- **Cons**: Violates GolemUI spec (incapable of rendering fractional grids as designed in the database).
- **Effort**: Low (2 hours)

#### Approach 2.B: Custom Fractional Grid & Flex Layout implementation (Recommended)
- Write a custom Go structure implementing `fyne.Layout`. It parses column/row metric slices (`"2fr"`, `"1.2fr"`, `"auto"`) and arranges components in their allocated bounding boxes.
- **Pros**: Fits the layout engine specifications perfectly; enables complex responsive dashboards.
- **Cons**: Higher rendering/sizing complexity when calculating minimum bounds.
- **Effort**: High (6-8 hours)

```go
// FractionalGridLayout computes coordinate sizing dynamically
type FractionalGridLayout struct {
    Columns []string
    Rows    []string
    Gap     float64
}

func (l *FractionalGridLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
    // Custom allocation of bounds to each object based on fractional sizes...
}
```

---

### 3. Local Reactivity Event Bus

Reactivity must bridge inputs (e.g., numeric pad key presses) and displays (e.g., cash register cart viewer) without server round-trips.

#### Approach 3.A: Direct Reference Coupling
- Hardcode references between widget structs (e.g., keypad has a pointer to the numeric text input).
- **Pros**: Simple, zero overhead.
- **Cons**: Destroys the dynamic database-driven nature of GolemUI; impossible to build dynamic layouts from JSON.
- **Effort**: Medium (but unacceptable design)

#### Approach 3.B: Pub/Sub Event Bus Broker (Recommended)
- Thread-safe pub/sub broker handling channels using Go channels and sync locks.
- **Pros**: Total decoupling; widgets subscribe to channels via `"bind_to"` metadata and publish asynchronously.
- **Cons**: Must manage channel lifetimes and teardown properly to prevent memory leaks when screens change.
- **Effort**: Medium (3-4 hours)

```go
type Event struct {
    Channel string
    Payload interface{}
}

type EventBus struct {
    mu          sync.RWMutex
    subscribers map[string][]chan Event
}
```

---

### 4. Lua Bootstrapper Connection Loader

#### Approach 4.A: Persistent Lua VM
- Keep the Lua VM alive during client execution to run scripts or handle logic updates.
- **Pros**: Allows running custom Lua event handlers later.
- **Cons**: High memory overhead; prone to leaks in GUI applications; complex synchronization.
- **Effort**: High (5 hours)

#### Approach 4.B: Ephemeral VM Loading & Immediate Teardown (Recommended)
- Load `golemui_driver.lua` into `gopher-lua`, read connections (`ui_db` and `business_db`), parse entrypoint query, and immediately close VM (`L.Close()`).
- **Pros**: Light memory footprint; aligns perfectly with specifications; clean lifecycle.
- **Cons**: Static configs loaded once at startup.
- **Effort**: Medium (2-3 hours)

---

## Recommendation

Implement **Approaches 1.B, 2.B, 3.B, and 4.B**. 
They establish a high-performance, decoupled, and strictly spec-compliant core that matches GolemUI's architecture. A custom `FractionalGridLayout` is the only viable path to deliver the responsive, database-defined user layouts specified in the technical documents.

---

## Risks

1. **Circular Package Dependencies**: The UI compositor, widgets, and Event Bus may naturally reference each other. Packages must be designed so widgets receive event bus triggers via callbacks/interfaces, never referencing the layout manager directly.
2. **Lua VM Memory Management**: Failing to close the Lua State or clean up its resources would leak memory on bootstrap. Ensure strict `defer L.Close()` wrappers around state evaluation.
3. **Fyne Thread Safety**: Goroutine queries fetching database records must not manipulate Fyne UI objects directly outside the main UI thread loop. Data grid updates must rely on the thread-safe `Refresh()` methods after thread-safe dataset mutations.

---

## Ready for Proposal

**Yes**. The architecture matches the technical documentation and provides a clear, scalable roadmap.
