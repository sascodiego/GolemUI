## Exploration: Code Audit Remediation

### Current State
The codebase currently exhibits several technical debt issues and architectural violations identified in the audit report:
1. **Thread-Safety Violations**: Mutating Fyne UI components directly from background goroutines in [main.go](file:///src/GolemUI/cmd/golemui/main.go#L123-L125) and [compositor.go](file:///src/GolemUI/pkg/ui/compositor.go#L410-L413) creates data races. Programmable sidebar navigation in [sidebar_widget.go](file:///src/GolemUI/pkg/ui/sidebar_widget.go#L32-L57) mutates widgets directly, and the re-entrancy guard is a simple unsynchronized boolean.
2. **Screen Navigation Memory Leaks**: Cleanups of old screens are executed prematurely at the start of async navigation, leading to "zombie screens" on failure. Concurrent navigation calls overwrite the package-level `prevCleanup` closure, leaking event bus subscriptions and database contexts.
3. **Database Driver Coupling**: The renderer in [compositor.go](file:///src/GolemUI/pkg/ui/compositor.go#L5) imports `database/sql/driver` to flat-format values, directly queries the connection pool, and has hardcoded layout widths. The grid model also degrades database values to strings, losing native types.
4. **Go Idioms and Connection Leaks**: Mutable package-level global pools exist in [compositor.go](file:///src/GolemUI/pkg/ui/compositor.go#L19-L22). Additionally, the database connection pool is not closed on exit in [main.go](file:///src/GolemUI/cmd/golemui/main.go#L167-L200) when the window closes.

### Affected Areas
- [main.go](file:///src/GolemUI/cmd/golemui/main.go) — Needs thread-safe navigation updates, navigation sequence tracking, deferred cleanup, and connection shutdown on exit.
- [compositor.go](file:///src/GolemUI/pkg/ui/compositor.go) — Needs Fyne thread-safety wrapping (`fyne.Do`), removal of database driver import, migration of values storage to `[][]any`, and dynamic layouts.
- [sidebar_widget.go](file:///src/GolemUI/pkg/ui/sidebar_widget.go) — Needs atomic type for the re-entrancy guard, and programmatic updates must run on the main UI thread.
- [db.go](file:///src/GolemUI/pkg/db/db.go) — Needs database value formatting utility relocation to decouple the renderer layer.

### Approaches

#### 1. Thread-Safety & Re-entrancy
- **Option A: Fyne UI Thread Dispatch & Atomic Guards (Recommended)**
  - Wrap UI-modifying code in `fyne.Do(func() { ... })` and use `atomic.Bool` for re-entrancy checks.
  - Pros: Conforms to Fyne multithreading model; fully resolves data races.
  - Cons: None.
  - Effort: Medium
- **Option B: Mutex-based UI Lockups**
  - Lock Fyne rendering methods using `sync.Mutex`.
  - Pros: Easy implementation.
  - Cons: Fyne widgets are not thread-safe; locking them blocks the main thread causing deadlocks.
  - Effort: Low

#### 2. Screen Navigation & Leaks
- **Option A: Sequence-Validated UI-Thread Cleanup (Recommended)**
  - Defers cleanup of the old screen to execute inside the `fyne.Do` block *only* when the new screen has successfully composed.
  - Spawns navigation with an atomic sequence number; discards outdated navigations and immediately releases their resources if a newer one was started.
  - Pros: Solves zombie screens, prevents out-of-order rendering, and stops memory leaks.
  - Cons: Requires tracking sequence and triggering early cleanup on mismatch.
  - Effort: Medium
- **Option B: Synchronized Global Cleanup**
  - Uses a mutex to serialize cleanup calls.
  - Pros: Simple.
  - Cons: Leaves zombie screens on composition failure.
  - Effort: Low

#### 3. Database Driver Decoupling & Native Type Preservation
- **Option A: Abstract Data Fetcher +Late Cell Formatting +Metadata Overrides (Recommended)**
  - Relocates value conversions (`formatValue` and `driver.Valuer` support) to `pkg/db`.
  - Changes grid rows storage from `[][]string` to `[][]any` to preserve native types (`int64`, `bool`, `float64`) for selection events.
  - Invokes cell formatting dynamically inside the table renderer callbacks.
  - Adds layout metrics mapping to table components dynamically.
  - Pros: Decouples Layer 4 from driver imports; preserves native types.
  - Cons: Modifies query mocks in unit tests.
  - Effort: High
- **Option B: Move Formatting Helper Only**
  - Moves `formatValue` to the `db` package but keeps raw row queries and string mapping inside the compositor.
  - Pros: Lower effort.
  - Cons: Keeps Layer 4 coupled to raw query/scanning mechanics.
  - Effort: Low

#### 4. Go Idioms & Database Cleanup
- **Option A: Struct-encapsulated Context & Defer Close (Recommended)**
  - Relocates package-level globals to a configuration context structure and ensures `app.DB.Close()` is called in `main.go` on graceful exit.
  - Pros: Correct idiomatic Go; prevents database connection leaks.
  - Cons: Requires refactoring globals throughout the ui package.
  - Effort: High
- **Option B: Graceful Close on Exit & Incremental Refactoring**
  - Calls `app.DB.Close()` in `main()` after `RunBootstrap` returns, delaying structural context rewrite to subsequent phases.
  - Pros: Fixes the connection leak immediately with minimal regression risk.
  - Cons: Mutable globals remain temporarily.
  - Effort: Low

### Recommendation
Implement Option A for all four areas to achieve architectural compliance with GolemUI rules:
1. Wrap Fyne widget mutations in `fyne.Do` and convert the navigation guard to `atomic.Bool`.
2. Implement sequence-validated screen navigation to cleanly free resources and avoid zombie screens.
3. Migrate `dataGridModel` to `[][]any`, defer string formatting to cell drawing, and move database types (`driver.Valuer`) to the database package.
4. Cleanly close database connection pools on application shutdown in `main.go`.

### Risks
- **Deadlock risk**: Calling `fyne.Do` while holding model write locks will block the UI thread. Mitigation: Always release `model.mu.Unlock()` before scheduling `fyne.Do` updates.
- **Mock DB test regressions**: Upgrading the data formats and selection event types from string to native types requires updating query registration and assertion blocks in [compositor_test.go](file:///src/GolemUI/pkg/ui/compositor_test.go).

### Ready for Proposal
Yes — The codebase exploration confirms the viability of remediation. We are ready to transition to the Proposal phase.
