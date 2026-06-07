# Proposal: sidebar-hsplit-integration

**Change ID:** sidebar-hsplit-integration  
**Status:** proposal  
**Date:** 2026-06-07  
**Layer:** Capa 4 — Renderizador Fyne  

---

## 1. Problem Statement

The current window management in `cmd/golemui/main.go` replaces the **entire window content** on every navigation event via `win.SetContent(newUI)`. This has three concrete problems:

1. **No persistent sidebar.** The `widget.Tree` built by `BuildNavTree` is never rendered. Users have no persistent navigation affordance — they must rely entirely on buttons within each screen to move between views.

2. **Full-window flash on navigation.** `win.SetContent()` destroys and recreates the entire Fyne widget tree on each navigation call. This causes a visible flash/rebuild that includes every widget, not just the content area that changed.

3. **No bidirectional sync.** When a user clicks a button with `navigate:` prefix (e.g., "Volver al Listado"), the sidebar has no way to know navigation occurred. The tree selection becomes stale or nonexistent — the user sees the correct view on the right but the sidebar shows no active item.

These problems make the UI feel stateless and disconnected rather than a cohesive desktop application with a persistent navigation chrome.

---

## 2. Proposed Solution

### 2.1 HSplit Layout

Replace the single `win.SetContent(screenUI)` pattern with a persistent split layout:

```
┌──────────────┬──────────────────────────────────┐
│              │                                  │
│  Sidebar     │      mainContainer               │
│  (Tree)      │      (container.NewMax)          │
│  ~220px      │      current screen view         │
│              │                                  │
│              │                                  │
└──────────────┴──────────────────────────────────┘
     Leading                  Trailing
       (HSplit)
```

- **Leading**: `container.NewVScroll(tree)` — scrollable sidebar with fixed-width tree.
- **Trailing**: `container.NewMax()` — fills remaining space, holds the active screen.
- Window content becomes the `HSplit`, set **once** at bootstrap and never replaced.

### 2.2 Partial Navigation

Change `ui.Navigate` to update **only** the right-side `mainContainer`:

```go
mainContainer.Objects = []fyne.CanvasObject{newUI}
mainContainer.Refresh()
```

The `*fyne.Container` reference is captured via closure in the `Navigate` callback assigned in `RunBootstrap`. No `win.SetContent()` call on navigation — only at initial window setup.

### 2.3 Bidirectional Sync

Introduce a `NavTree` struct in `pkg/ui/sidebar_widget.go` that wraps `*widget.Tree` and exposes:

- **`SelectByVistaID(vistaID string)`**: walks an internal `vistaID → treeNodeID` map, calls `tree.OpenBranch()` for each ancestor, then `tree.Select()` for the target leaf.

A `*NavTree` reference is captured via closure in the `Navigate` callback, so after `Compose` succeeds, the sidebar updates its selection.

**Re-entrancy guard**: Since `tree.Select()` calls `OnSelected`, which calls `Navigate`, a `navigating` boolean flag prevents infinite recursion. The guard is set before `SelectByVistaID` and cleared after.

---

## 3. Scope Boundaries

### 3.1 In Scope

| Item | Files |
|------|-------|
| HSplit layout construction in `RunBootstrap` | `cmd/golemui/main.go` |
| Partial navigation (mainContainer update) | `cmd/golemui/main.go` |
| `NavTree` struct with `SelectByVistaID` | `pkg/ui/sidebar_widget.go` |
| Sidebar loading (`LoadNavigationMenu` + `BuildNavTree`) wired into bootstrap | `cmd/golemui/main.go` |
| Tests for `NavTree.SelectByVistaID` | `pkg/ui/sidebar_widget_test.go` |
| Updated bootstrap tests for HSplit layout | `cmd/golemui/main_test.go` |

### 3.2 Out of Scope

- Database schema changes (Capa 1-3) — `menu_navegacion`, `vistas_consulta` untouched.
- `LoadNavigationMenu` or `MenuItem` changes — already complete from prior SDD change.
- Plugin system, data sources, event bus — no changes.
- Window title, icon, or theme changes.
- Multi-window support — single main window only.
- Sidebar collapse/expand toggle — future enhancement.
- Sidebar width persistence across sessions — future enhancement.
- Configuration file changes — `golemui_driver.yaml` untouched.

---

## 4. Key Decisions

### Decision 1: Where to store tree reference and vistaID→nodeID map

**Choice: `NavTree` struct in `pkg/ui/sidebar_widget.go`**

```go
type NavTree struct {
    tree       *widget.Tree
    vistaToNode map[string]string   // vistaID → menu item ID
    parentOf   map[string]string   // nodeID → parent nodeID
}
```

**Rationale:**
- Keeps tree internals (parent-to-children index, id-to-item map) encapsulated.
- `BuildNavTree` is refactored to return `*NavTree` instead of `*widget.Tree`.
- The caller (`main.go`) only needs `*NavTree` — never accesses `*widget.Tree` directly for selection.
- The `*widget.Tree` is accessible via a `Widget() *widget.Tree` method for adding to containers.

**Alternatives considered:**
- (A) Export `idToItem` and `parentToChildren` maps from current `BuildNavTree` → leaks internals, couples caller.
- (B) Build the vistaID→nodeID map in `main.go` → splits knowledge across packages.
- (C) Use a channel/event for sync → over-engineered for synchronous Fyne callbacks.

### Decision 2: `BuildNavTree` return type change

**Choice: Change signature from `BuildNavTree(items []MenuItem) *widget.Tree` to `BuildNavTree(items []MenuItem) *NavTree`**

**Rationale:**
- Breaking change but `BuildNavTree` is only called in `main.go` (one call site) and tested in `sidebar_widget_test.go`.
- The test file accesses `tree.ChildUIDs`, `tree.IsBranch`, `tree.UpdateNode`, `tree.OnSelected` — these need updating to go through `navTree.Widget()`.
- Cleaner than adding a separate `NewNavTree` constructor that wraps an already-built tree.

**Migration:** Tests call `nt.Widget()` to get the `*widget.Tree` for assertion access. The `NavTree` type provides `SelectByVistaID` for new tests.

### Decision 3: Thread safety strategy for Navigate callback

**Choice: Direct mutation — no goroutine dispatch needed**

**Rationale:**
- `Navigate` is called from two sources:
  1. `tree.OnSelected` — called by Fyne on the **UI thread** (user click).
  2. `widget.NewButton` with `navigate:` — called by Fyne on the **UI thread** (user click).
- Both callers are already on the Fyne UI thread. No background goroutine dispatches `Navigate`.
- `mainContainer.Refresh()` is safe on the UI thread.
- `tree.Select()` is safe on the UI thread.
- The `dataGridModel` in `compositor.go` uses goroutines for data loading, but those dispatch `table.Refresh()` correctly — and they don't call `Navigate`.

**Future consideration:** If a background event bus subscriber ever calls `Navigate`, a `fyne.CurrentApp().Send()` wrapper would be needed. For this change, all call sites are on the UI thread, so no additional dispatch is required.

### Decision 4: `ui.Navigate` signature

**Choice: Keep `func(vistaID string)` unchanged**

**Rationale:**
- The closure in `RunBootstrap` already captures `ctx`, `cfg.LayoutQuery`, `mainContainer`, and `navTree`.
- Adding parameters would require changing every call site in `compositor.go` (button with `navigate:` prefix).
- Package-level variable `ui.Navigate` remains the contract — internal closure state changes, external API does not.

### Decision 5: Re-entrancy guard for bidirectional sync

**Choice: Boolean flag inside `NavTree`**

```go
type NavTree struct {
    // ...
    navigating bool  // set during SelectByVistaID to prevent OnSelected → Navigate loop
}
```

When `SelectByVistaID` runs, it sets `nt.navigating = true` before calling `tree.Select()`. The `OnSelected` callback checks this flag and skips the `Navigate` call when already navigating. This prevents:
```
Navigate("home") → SelectByVistaID("home") → tree.Select("nav_home")
  → OnSelected("nav_home") → Navigate("home") → ... infinite loop
```

---

## 5. Impact Assessment

### Files Changed

| File | Change Type | Estimated Lines |
|------|-------------|-----------------|
| `cmd/golemui/main.go` | Modify: `RunBootstrap` body, add `container` import | ~40 changed, ~15 new |
| `pkg/ui/sidebar_widget.go` | Modify: `BuildNavTree` returns `*NavTree`, add `NavTree` struct + `SelectByVistaID` | ~50 new |
| `pkg/ui/sidebar_widget_test.go` | Modify: update tests to use `NavTree`, add bidirectional sync tests | ~60 new |
| `cmd/golemui/main_test.go` | Modify: update bootstrap tests for HSplit layout | ~20 changed |

**Total estimated:** ~170 lines changed/added. Well within 400-line review budget.

### Risk Level: **Medium**

- **Low risk**: HSplit construction is straightforward Fyne API usage.
- **Medium risk**: Re-entrancy guard must be correct — infinite loop would hang the UI.
- **Low risk**: `BuildNavTree` signature change — controlled migration, one call site.
- **Low risk**: Thread safety — confirmed all call sites are on UI thread.

### Rollback Plan

Since this is Capa 4 (Fyne client only) with no database changes:
1. `git revert` the commit restores `win.SetContent(fullUI)` behavior.
2. No database migration to roll back.
3. No plugin interface changes.
4. The `NavTree` struct addition is backward-compatible — old code simply doesn't use it.

---

## 6. Non-Goals

This change explicitly does **NOT**:

1. **Add sidebar collapse/expand toggle** — the sidebar is always visible at a fixed width.
2. **Persist sidebar width across sessions** — `Split.SetOffset()` is not saved/loaded.
3. **Add breadcrumbs or navigation history** — no back/forward stack.
4. **Support multiple windows** — single main window only.
5. **Change the database schema** — `menu_navegacion` and `vistas_consulta` are untouched.
6. **Modify the event bus or data plugin system** — no Capa 1-3 changes.
7. **Add keyboard shortcuts for navigation** — future enhancement.
8. **Style or theme the sidebar** — uses default Fyne widget styling.
9. **Handle deep linking or external navigation triggers** — all navigation originates from Fyne widget callbacks.
10. **Add loading indicators during screen transitions** — synchronous compose, no async render gap.

---

## 7. Dependency on Prior Work

This change depends on the following completed SDD changes:

| Change | Artifact | Status |
|--------|----------|--------|
| `nav-menu-schema` | `docker/init-db/02_init_core.sql` — `menu_navegacion` table + seed data | ✅ Applied & Verified |
| `nav-menu-loader` | `pkg/ui/sidebar_loader.go` — `LoadNavigationMenu`, `MenuItem` | ✅ Applied & Verified |
| `fyne-sidebar-tree` | `pkg/ui/sidebar_widget.go` — `BuildNavTree` | ✅ Applied & Verified |

---

## 8. Acceptance Criteria Summary

1. ✅ Window shows HSplit: sidebar (left) + content (right) on launch.
2. ✅ Clicking a sidebar leaf renders the corresponding view on the right; sidebar stays fixed.
3. ✅ Clicking a `navigate:` button (e.g., "Volver al Listado") renders the target view on the right AND auto-selects + auto-expands the corresponding sidebar node.
4. ✅ No full-window flash/rebuild on navigation — only the right panel updates.
5. ✅ All existing tests pass; new tests cover `SelectByVistaID` and re-entrancy guard.
6. ✅ `go build ./...` and `go vet ./...` clean.
