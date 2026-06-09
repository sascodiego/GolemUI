# Design: code-quality-antipatterns

**Change ID:** `code-quality-antipatterns`
**Date:** 2026-06-07
**Status:** Approved
**Config:** `openspec/config.yaml` — strict TDD enabled

---

## 1. Overview

Two focused fixes in two files:

1. **§4.2** — Replace `bool` with `atomic.Bool` for `NavTree.navigating` in `pkg/ui/sidebar_widget.go`
2. **§4.3** — Add `win.SetOnClosed()` cleanup hook + `sync.Mutex` for `prevCleanup` in `cmd/golemui/main.go`

---

## 2. §4.2: Atomic Re-entrancy Guard

### 2.1 File: `pkg/ui/sidebar_widget.go`

#### Before (current — data race)

```go
import (
	"sort"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

type NavTree struct {
	tree        *widget.Tree
	vistaToNode map[string]string
	parentOf    map[string]string
	navigating  bool
}
```

Write site in `SelectByVistaID`:
```go
nt.navigating = true
defer func() { nt.navigating = false }()
```

Read site in `OnSelected`:
```go
if navTree.navigating {
    return
}
```

#### After (atomic.Bool — race-free)

```go
import (
	"sort"
	"sync/atomic"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

type NavTree struct {
	tree        *widget.Tree
	vistaToNode map[string]string
	parentOf    map[string]string
	navigating  atomic.Bool
}
```

Write site:
```go
nt.navigating.Store(true)
defer func() { nt.navigating.Store(false) }()
```

Read site:
```go
if navTree.navigating.Load() {
    return
}
```

#### Zero-value guarantee

`atomic.Bool` zero-value is `false`, identical to `bool`. No constructor changes needed. Existing `BuildNavTree` constructor continues to work without modification.

### 2.2 Test Design

| Test | File | Status |
|------|------|--------|
| `TestNavigating_InitialState` | `pkg/ui/sidebar_widget_test.go` | **NEW** — verifies `navigating` is `false` on fresh NavTree |
| `TestReentrancyGuardPreventsLoop` | `pkg/ui/sidebar_widget_test.go` | Unchanged — existing guard test |
| `TestSelectByVistaID_ValidSelectsNode` | `pkg/ui/sidebar_widget_test.go` | Unchanged |
| `TestSelectByVistaID_EmptyIsNoOp` | `pkg/ui/sidebar_widget_test.go` | Unchanged |
| `TestSelectByVistaID_UnknownIsNoOp` | `pkg/ui/sidebar_widget_test.go` | Unchanged |
| `TestBuildNavTree_LeafTriggersNavigate` | `pkg/ui/sidebar_widget_test.go` | Unchanged |

`TestNavigating_InitialState` creates a fresh NavTree with a leaf node that has a VistaID, sets a Navigate spy, and calls `OnSelected`. If `navigating` were incorrectly `true` (non-zero), the callback would suppress the Navigate call and the test would fail.

---

## 3. §4.3: Pool Cleanup + prevCleanup Mutex

### 3.1 File: `cmd/golemui/main.go`

#### Before (current — missing cleanup, data race on prevCleanup)

```go
import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

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
```

Navigate closure (unprotected prevCleanup):
```go
var prevCleanup func()

ui.Navigate = func(vID string) {
    log.Printf("[UI/Navigation] Navigating to screen %q", vID)
    go func() {
        if prevCleanup != nil {
            prevCleanup()
            prevCleanup = nil
        }
        // ... LoadScreen, Compose ...
        prevCleanup = cleanup
        // ... UI update ...
    }()
}
```

Window block (no cleanup hook):
```go
if runWindow {
    win.ShowAndRun()
}
```

#### After (mutex-protected prevCleanup + SetOnClosed hook)

```go
import (
	"context"
	"fmt"
	"log"
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
```

Navigate closure (mutex-protected):
```go
var cleanupMu sync.Mutex
var prevCleanup func()

ui.Navigate = func(vID string) {
    log.Printf("[UI/Navigation] Navigating to screen %q", vID)
    go func() {
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

Window block (with SetOnClosed hook):
```go
if runWindow {
    win.SetOnClosed(func() {
        cleanupMu.Lock()
        if prevCleanup != nil {
            prevCleanup()
            prevCleanup = nil
        }
        cleanupMu.Unlock()
        dbPool.Close()
    })
    win.ShowAndRun()
}
```

### 3.2 Architectural Constraints

- **REQ-ARCH-02**: Lock is NOT held during `LoadScreen` or `Compose` calls. Lock scope is limited to `prevCleanup` read/write/call only.
- **REQ-ARCH-03**: `dbPool.Close()` is called OUTSIDE the `cleanupMu` lock in `SetOnClosed`. Pool close may block on draining connections.
- The initial `prevCleanup = homeCleanup` assignment (before `if runWindow`) is in the main goroutine before any concurrent access begins. No mutex needed at that point.

### 3.3 Test Design

| Test | File | Status |
|------|------|--------|
| All existing `TestRunBootstrap_*` tests | `cmd/golemui/main_test.go` | Unchanged — use `runWindow=false` path |
| All existing `TestNavigate_*` tests | `cmd/golemui/main_test.go` | Unchanged |
| All existing `TestSanitizeLocale_*` tests | `cmd/golemui/main_test.go` | Unchanged |

No new test added for §4.3 because:
- The `SetOnClosed` callback only fires when `runWindow=true`, which blocks on `ShowAndRun()`.
- The existing test path (`runWindow=false`) is unaffected.
- The mutex logic is a standard Go pattern; `go test -race` validates correctness.

---

## 4. Dependency and Constraint Map

```
pkg/ui/sidebar_widget.go    ← §4.2 change (atomic.Bool)
pkg/ui/sidebar_widget_test.go ← §4.2 test addition (TestNavigating_InitialState)
cmd/golemui/main.go         ← §4.3 change (mutex + SetOnClosed)
```

No cross-file dependencies between §4.2 and §4.3. Both can be implemented independently.

---

## 5. Validation Plan

1. `go build ./...` — compiles without errors
2. `go test ./... -count=1` — all tests pass
3. `go test -race ./pkg/ui/ -count=1` — zero race detector warnings for §4.2
4. `go vet ./...` — no vet warnings
5. `gofmt -l .` — no formatting issues
