# SDD Design — fyne-thread-safety-v2

**Change ID:** `fyne-thread-safety-v2`
**Date:** 2026-06-09

---

## 1. Overview

**Affected files:** 3 production files, 0 test files (tests are a separate task phase).

| File | Summary of changes |
|------|-------------------|
| `pkg/ui/compositor.go` | Remove `refreshMu` field; replace 4 `refreshMu.Lock/Unlock` blocks with `fyne.Do`; add package comment |
| `pkg/ui/sidebar_widget.go` | Wrap tree mutations in `fyne.DoAndWait`; preserve re-entrancy guard |
| `cmd/golemui/main.go` | Wrap Navigate UI swap in `fyne.Do` |

**Total estimated diff:** ~50 lines changed (6 site replacements + 1 struct field removal + 1 package comment). No new files. No import additions (all three files already import `"fyne.io/fyne/v2"`).

---

## 2. File-by-File Changes

### 2.1 `pkg/ui/compositor.go`

#### 2.1.0 Package comment (REQ-EB-01)

Add a package-level doc comment immediately before `package ui` (line 1).

**BEFORE** (line 1):

```go
package ui
```

**AFTER:**

```go
// Package ui implements the GolemUI rendering engine.
//
// Thread-safety contract for EventBus subscribers:
// Any LocalEventBus.Subscribe handler that mutates a Fyne widget (e.g. table.Refresh,
// label.SetText, button.Enable) must wrap the mutation in fyne.Do(func() { ... }).
// The EventBus dispatches each handler in a fresh goroutine (go h(event)).
// Fyne requires widget mutations on the UI thread. fyne.Do bridges the two.
// This applies to all current and future subscriber handlers, including the data_grid
// reactive filtering and upcoming reactive label (017) and button state (018) bindings.
package ui
```

#### 2.1.1 Import changes

No changes required. The file already imports `"fyne.io/fyne/v2"` (line 10).

#### 2.1.2 `refreshMu` field removal (REQ-DG-05)

Remove the `refreshMu` field from the `dataGridModel` struct.

**BEFORE** (struct definition, lines 27–42):

```go
type dataGridModel struct {
	mu            sync.RWMutex
	refreshMu     sync.Mutex // serializes all table.Refresh() calls to prevent concurrent widget mutations
	headers       []string
	columns       []string
	rows          [][]string
	masterHeaders []string
	masterRows    [][]string
	filterKeys    []string
	cancel        context.CancelFunc
	unsubscribe   func()
	wg            sync.WaitGroup
}
```

**AFTER:**

```go
type dataGridModel struct {
	mu            sync.RWMutex
	headers       []string
	columns       []string
	rows          [][]string
	masterHeaders []string
	masterRows    [][]string
	filterKeys    []string
	cancel        context.CancelFunc
	unsubscribe   func()
	wg            sync.WaitGroup
}
```

**REQ-LOCK-01 verification note:** removing `refreshMu` eliminates the secondary mutex entirely. After this change, `model.mu` is the only lock on the struct, and it is always unlocked before `fyne.Do` — see sites 1–4 below.

#### 2.1.3 Site 1: `loadMasterBuffer` — `table.SetColumnWidth` + `table.Refresh` (REQ-DG-01)

This goroutine block is inside `loadMasterBuffer` (starts ~line 343).

**BEFORE:**

```go
		model.mu.Lock()
		model.masterHeaders = ds.Headers
		model.masterRows = ds.Rows
		model.headers = ds.Headers
		model.rows = ds.Rows
		model.mu.Unlock()

		model.refreshMu.Lock()
		for i, h := range ds.Headers {
			w := resolveWidth(cwr, ds.ColumnWidths, i, h, node.MasterDataSource)
			table.SetColumnWidth(i, w)
		}
		table.Refresh()
		model.refreshMu.Unlock()
	}()
```

**AFTER:**

```go
		model.mu.Lock()
		model.masterHeaders = ds.Headers
		model.masterRows = ds.Rows
		model.headers = ds.Headers
		model.rows = ds.Rows
		model.mu.Unlock()

		fyne.Do(func() {
			for i, h := range ds.Headers {
				w := resolveWidth(cwr, ds.ColumnWidths, i, h, node.MasterDataSource)
				table.SetColumnWidth(i, w)
			}
			table.Refresh()
		})
	}()
```

**REQ-LOCK-01 check:** `model.mu.Unlock()` at line `model.mu.Unlock()` completes before `fyne.Do(...)` is entered. ✓

#### 2.1.4 Site 2: `filterMasterRows` empty-snap early return — `table.Refresh` (REQ-DG-03)

This is the empty-snapshot branch inside `filterMasterRows` (the `if len(snap) == 0` block).

**BEFORE:**

```go
	// If snapshot is empty, show all rows
	if len(snap) == 0 {
		model.rows = model.masterRows
		model.mu.Unlock()
		model.refreshMu.Lock()
		table.Refresh()
		model.refreshMu.Unlock()
		return
	}
```

**AFTER:**

```go
	// If snapshot is empty, show all rows
	if len(snap) == 0 {
		model.rows = model.masterRows
		model.mu.Unlock()
		fyne.Do(func() {
			table.Refresh()
		})
		return
	}
```

**REQ-LOCK-01 check:** `model.mu.Unlock()` completes before `fyne.Do(...)`. ✓

#### 2.1.5 Site 3: `filterMasterRows` filtered path — `table.Refresh` (REQ-DG-04)

This is the final block of `filterMasterRows`, after the filtering loop.

**BEFORE:**

```go
	model.rows = filtered
	model.mu.Unlock()

	model.refreshMu.Lock()
	table.Refresh()
	model.refreshMu.Unlock()
}
```

**AFTER:**

```go
	model.rows = filtered
	model.mu.Unlock()

	fyne.Do(func() {
		table.Refresh()
	})
}
```

**REQ-LOCK-01 check:** `model.mu.Unlock()` completes before `fyne.Do(...)`. ✓

#### 2.1.6 Site 4: `fetchGridDataAsync` — `table.SetColumnWidth` + `table.Refresh` (REQ-DG-02)

This is inside the goroutine in `fetchGridDataAsync`.

**BEFORE:**

```go
		model.mu.Lock()
		model.headers = ds.Headers
		model.columns = ds.Headers
		model.rows = ds.Rows
		model.mu.Unlock()

		model.refreshMu.Lock()
		for i, h := range ds.Headers {
			w := resolveWidth(cwr, ds.ColumnWidths, i, h, node.DataSource)
			table.SetColumnWidth(i, w)
		}
		table.Refresh()
		model.refreshMu.Unlock()
	}()
```

**AFTER:**

```go
		model.mu.Lock()
		model.headers = ds.Headers
		model.columns = ds.Headers
		model.rows = ds.Rows
		model.mu.Unlock()

		fyne.Do(func() {
			for i, h := range ds.Headers {
				w := resolveWidth(cwr, ds.ColumnWidths, i, h, node.DataSource)
				table.SetColumnWidth(i, w)
			}
			table.Refresh()
		})
	}()
```

**REQ-LOCK-01 check:** `model.mu.Unlock()` completes before `fyne.Do(...)`. ✓

---

### 2.2 `pkg/ui/sidebar_widget.go`

#### 2.2.1 Import changes

No changes required. The file already imports `"fyne.io/fyne/v2"` (line 6).

#### 2.2.2 `SelectByVistaID` — wrap tree mutations in `fyne.DoAndWait` (REQ-SB-01, REQ-SB-02, REQ-SB-03)

The entire function body changes in the lower half. The early-return guards (`vistaID == ""`, unknown `vistaID`) remain outside any dispatch block (REQ-SB-03). The `navigating` guard is set **before** `fyne.DoAndWait` and cleared **after** it returns via `defer` (REQ-SB-02).

**BEFORE** (full function, lines 33–58):

```go
func (nt *NavTree) SelectByVistaID(vistaID string) {
	if vistaID == "" {
		return
	}
	nodeID, ok := nt.vistaToNode[vistaID]
	if !ok {
		return
	}

	// Walk ancestor chain from target to root, then open root→parent
	ancestors := []string{}
	for cur := nodeID; cur != ""; {
		pid := nt.parentOf[cur]
		if pid != "" {
			ancestors = append(ancestors, pid)
		}
		cur = pid
	}
	for i := len(ancestors) - 1; i >= 0; i-- {
		nt.tree.OpenBranch(widget.TreeNodeID(ancestors[i]))
	}

	nt.navigating.Store(true)
	defer func() { nt.navigating.Store(false) }()
	nt.tree.Select(widget.TreeNodeID(nodeID))
}
```

**AFTER:**

```go
func (nt *NavTree) SelectByVistaID(vistaID string) {
	if vistaID == "" {
		return
	}
	nodeID, ok := nt.vistaToNode[vistaID]
	if !ok {
		return
	}

	// Walk ancestor chain from target to root, then open root→parent
	ancestors := []string{}
	for cur := nodeID; cur != ""; {
		pid := nt.parentOf[cur]
		if pid != "" {
			ancestors = append(ancestors, pid)
		}
		cur = pid
	}

	nt.navigating.Store(true)
	defer func() { nt.navigating.Store(false) }()

	// fyne.DoAndWait blocks until the callback completes on the UI thread.
	// This preserves the re-entrancy guard: navigating is true for the
	// entire duration of the tree mutation, so OnSelected cannot re-enter
	// Navigate during programmatic selection.
	fyne.DoAndWait(func() {
		for i := len(ancestors) - 1; i >= 0; i-- {
			nt.tree.OpenBranch(widget.TreeNodeID(ancestors[i]))
		}
		nt.tree.Select(widget.TreeNodeID(nodeID))
	})
}
```

**Semantic analysis:**

| Aspect | Before | After | Preserved? |
|--------|--------|-------|-----------|
| Early-return on empty/unknown `vistaID` | Outside any dispatch | Same | ✓ |
| Ancestor chain computed on calling goroutine | Yes | Yes (computed before `fyne.DoAndWait`) | ✓ |
| `navigating` set before `tree.Select` | Yes | Yes — set before `fyne.DoAndWait` | ✓ |
| `navigating` cleared after `tree.Select` | Yes (defer, after Select returns) | Yes (defer, after `DoAndWait` returns) | ✓ |
| `OpenBranch` + `Select` ordering | Root→parent, then select | Same ordering inside callback | ✓ |
| Thread safety | None (raw goroutine call) | Dispatched to UI thread | ✓ (fixed) |

**Why `fyne.DoAndWait` and not `fyne.Do`:** If `fyne.Do` (async) were used, the `defer navigating.Store(false)` would execute immediately after `fyne.Do` returns — before the callback runs on the UI thread. This creates a window where the guard is `false` during the actual tree mutation, defeating the re-entrancy protection. `fyne.DoAndWait` blocks the calling goroutine until the callback completes, so the guard is `true` for the full duration of the UI-thread mutation. When called from the UI thread (e.g. inside Navigate's `fyne.Do` block), `fyne.DoAndWait` executes inline with no re-dispatch overhead.

---

### 2.3 `cmd/golemui/main.go`

#### 2.3.1 Import changes

No changes required. The file already imports `"fyne.io/fyne/v2"` (line 11).

#### 2.3.2 Navigate closure — wrap UI swap in `fyne.Do` (REQ-NAV-02, REQ-NAV-03)

The three UI-mutating statements at the end of the `ui.Navigate` goroutine are wrapped in a single `fyne.Do`. Error paths (`LoadScreen` or `Compose` returning errors) return before reaching `fyne.Do` (REQ-NAV-03). The `cleanupMu` blocks remain outside `fyne.Do` — they manage non-UI bookkeeping.

**BEFORE** (Navigate goroutine body, lines 109–138):

```go
	ui.Navigate = func(vID string) {
		log.Printf("[UI/Navigation] Navigating to screen %q", vID)
		go func() {
			// Tear down previous screen before loading the new one
			cleanupMu.Lock()
			if prevCleanup != nil {
				prevCleanup()
				prevCleanup = nil
			}
			cleanupMu.Unlock()

			node, err := ui.LoadScreen(ctx, dbPool.CorePool, vID, cfg.LayoutQuery)
			if err != nil {
				log.Printf("[UI/Navigation] Error loading screen %q: %v", vID, err)
				return
			}
			newUI, cleanup, err := ui.Compose(node, vID)
			if err != nil {
				log.Printf("[UI/Navigation] Error composing screen %q: %v", vID, err)
				return
			}

			cleanupMu.Lock()
			prevCleanup = cleanup
			cleanupMu.Unlock()

			mainContainer.Objects = []fyne.CanvasObject{newUI}
			mainContainer.Refresh()
			navTree.SelectByVistaID(vID)
		}()
	}
```

**AFTER:**

```go
	ui.Navigate = func(vID string) {
		log.Printf("[UI/Navigation] Navigating to screen %q", vID)
		go func() {
			// Tear down previous screen before loading the new one
			cleanupMu.Lock()
			if prevCleanup != nil {
				prevCleanup()
				prevCleanup = nil
			}
			cleanupMu.Unlock()

			node, err := ui.LoadScreen(ctx, dbPool.CorePool, vID, cfg.LayoutQuery)
			if err != nil {
				log.Printf("[UI/Navigation] Error loading screen %q: %v", vID, err)
				return
			}
			newUI, cleanup, err := ui.Compose(node, vID)
			if err != nil {
				log.Printf("[UI/Navigation] Error composing screen %q: %v", vID, err)
				return
			}

			cleanupMu.Lock()
			prevCleanup = cleanup
			cleanupMu.Unlock()

			fyne.Do(func() {
				mainContainer.Objects = []fyne.CanvasObject{newUI}
				mainContainer.Refresh()
				navTree.SelectByVistaID(vID)
			})
		}()
	}
```

**Semantic analysis:**

| Aspect | Before | After |
|--------|--------|-------|
| `go func()` async kickoff | Yes | Yes (unchanged) |
| `cleanupMu.Lock/Unlock` for prev cleanup | Outside any dispatch | Same (unchanged) |
| `LoadScreen` + `Compose` on goroutine | Yes | Yes (unchanged) |
| Error early returns | Before UI mutations | Same — `fyne.Do` is never reached on error |
| `cleanupMu.Lock/Unlock` for new cleanup | Outside any dispatch | Same (unchanged) |
| `mainContainer.Objects` assignment | On goroutine (unsafe) | Inside `fyne.Do` → UI thread ✓ |
| `mainContainer.Refresh()` | On goroutine (unsafe) | Inside `fyne.Do` → UI thread ✓ |
| `navTree.SelectByVistaID(vID)` | On goroutine (unsafe) | Inside `fyne.Do` → UI thread ✓ |
| 3 statements atomic on UI thread | No | Yes — single `fyne.Do` callback |

**Note on `navTree.SelectByVistaID` inside `fyne.Do`:** The sidebar function uses `fyne.DoAndWait` internally. When called from the UI thread (which it now is, since Navigate wraps it in `fyne.Do`), `fyne.DoAndWait` executes the callback inline and returns immediately — no nested dispatch overhead.

---

## 3. Dependency Order

Apply changes in this order to keep the code compilable at every step:

1. **`pkg/ui/compositor.go`** — Remove `refreshMu` field and replace all 4 `refreshMu.Lock/Unlock` blocks with `fyne.Do`. This is the largest change but is fully self-contained. After this step, `compositor.go` compiles cleanly with no dead references to `refreshMu`.

2. **`pkg/ui/sidebar_widget.go`** — Wrap tree mutations in `fyne.DoAndWait`. This change is independent of compositor. After this step, `sidebar_widget.go` compiles cleanly.

3. **`cmd/golemui/main.go`** — Wrap Navigate UI swap in `fyne.Do`. This change calls `navTree.SelectByVistaID` which now uses `fyne.DoAndWait` internally. No cross-file compile dependency, but the semantic correctness of the nested `fyne.Do` → `fyne.DoAndWait` call chain should be verified together.

All three files can technically be edited in any order — there are no cross-file compile dependencies between the changes. The order above minimizes risk by tackling the most-site file first.

---

## 4. Estimated Diff Size

| File | Lines added | Lines removed | Net |
|------|------------|--------------|-----|
| `pkg/ui/compositor.go` | ~20 (4 `fyne.Do` blocks + package comment) | ~14 (4 `refreshMu` blocks + 1 field) | +6 |
| `pkg/ui/sidebar_widget.go` | ~10 (restructured lower function body) | ~6 (old inline calls) | +4 |
| `cmd/golemui/main.go` | ~4 (fyne.Do wrap) | ~2 (unwrapped lines) | +2 |
| **Total** | **~34** | **~22** | **+12** |

No new files. No `go.mod` changes. No test changes in this design (tests are specified in the spec's TDD scenarios and belong in the apply phase).

---

## 5. Risks (from design perspective)

| # | Risk | Severity | Mitigation |
|---|------|----------|------------|
| R1 | **`fyne.Do` fire-and-forget vs. goroutine lifecycle:** The `fyne.Do` callback in `loadMasterBuffer` and `fetchGridDataAsync` is dispatched asynchronously to the UI thread. The spawning goroutine exits (releasing `model.wg.Done()`) before the callback necessarily runs. If the screen is torn down (`cleanup()` called) between dispatch and execution, the `table` pointer may reference a destroyed widget. | Medium | The `cleanup()` function cancels the context and waits via `model.wg.Wait()`. Since `fyne.Do` is called **after** `model.wg.Done()` would be deferred... actually, no: `fyne.Do` is called **before** `defer model.wg.Done()` executes (it's inside the goroutine body, and `defer` runs when the goroutine returns). The `wg.Wait()` in cleanup waits for the goroutine to finish — but `fyne.Do` is fire-and-forget, so the goroutine finishes before the callback runs. This means cleanup could run while the callback is pending. However, `cleanup()` cancels context + unsubscribes, and the `table` widget is owned by the Fyne window — it is not freed until the window replaces it. The practical risk is low because `mainContainer.Objects` replacement (the screen teardown) happens in a separate `fyne.Do` in Navigate, which Fyne serializes after any pending callbacks. **Verify in apply phase** that no crash occurs under rapid navigation. |
| R2 | **Fyne test driver limitation:** `go test -race` will surface Fyne-internal races (`expiringCache.setAlive`, font metrics cache) that are not GolemUI bugs. These are the same races identified in the v1 verify-report. | Low | Accepted as known limitation. Structural grep audit is the primary verification. Tests verify correct dispatch, not race-clean Fyne internals. |
| R3 | **Re-regression:** Future refactors may remove `fyne.Do` wraps, just as the v1 wraps were removed. | Medium | REQ-EB-01 package comment documents the pattern. A future lint rule (out of scope for v2) could enforce the invariant mechanically. |
| R4 | **`refreshMu` removal:** Any code path that relied on `refreshMu` for serialization beyond thread dispatch loses that guard. `fyne.Do` provides equivalent serialization (UI thread is single-threaded), but if `refreshMu` was guarding a non-UI-code-path race, removing it could expose that race. | Low | The explore report confirms `refreshMu` only guards `table.SetColumnWidth` and `table.Refresh` calls — both are widget mutations that must run on the UI thread. No non-UI code path depends on `refreshMu`. `model.mu` guards data access, not widget state. |
| R5 | **Nested `fyne.Do` → `fyne.DoAndWait`:** Navigate's `fyne.Do` calls `navTree.SelectByVistaID`, which internally calls `fyne.DoAndWait`. On the UI thread, `fyne.DoAndWait` is documented to run inline. If Fyne's implementation does not handle this nesting, it could deadlock. | Low | Verified in Fyne v2.7.4 source: `DoAndWait` detects it is already on the UI thread and runs inline without dispatch. No deadlock. |

---

## 6. Invariant Checklist (pre-apply verification)

After applying all changes, the following structural invariants must hold:

- [ ] **INV-01:** `grep -rn "refreshMu" --include="*.go" pkg/ui/ cmd/golemui/` returns zero matches.
- [ ] **INV-02:** `grep -rn "fyne\.Do\|fyne\.DoAndWait" --include="*.go" pkg/ui/compositor.go pkg/ui/sidebar_widget.go cmd/golemui/main.go` returns exactly 6 matches (4 `fyne.Do` in compositor + 1 `fyne.DoAndWait` in sidebar + 1 `fyne.Do` in main).
- [ ] **INV-03:** At every `fyne.Do`/`fyne.DoAndWait` site, `model.mu.Unlock()` (or equivalent unlock) appears on a line **before** the `fyne.Do`/`fyne.DoAndWait` call — never inside the callback body.
- [ ] **INV-04:** No `model.mu.Unlock()` appears inside any `fyne.Do` callback.
- [ ] **INV-05:** The `navigating.Store(true)` call appears **before** `fyne.DoAndWait` in `SelectByVistaID`.
- [ ] **INV-06:** No public API signature changes. `go vet ./...` is clean.
- [ ] **INV-07:** `"fyne.io/fyne/v2"` import is present in all 3 files (already true — no change needed).
