# GolemUI Codebase Audit Report

This document contains a comprehensive audit of all Go source code files inside `/src/GolemUI/pkg/` and `/src/GolemUI/cmd/`. The audit has been performed to evaluate thread-safety with Fyne widgets, screen lifecycle memory leaks, compliance with the 4-layer architecture model, and general Go code quality.

---

## 1. Thread-Safety Violations & Concerns in Fyne UI Updates

Under Fyne's API rules, any concurrent update to canvas objects or widget properties must be performed in a thread-safe manner. Since `fyne.Do()` is not available, all operations that mutate UI widgets or containers directly from background goroutines are highly unsafe and can lead to data races, layout corruption, or runtime panics.

### 1.1 Navigation Screen Swapping
* **Location**: [main.go:L103-127](file:///src/GolemUI/cmd/golemui/main.go#L103-L127)
* **Description**: The package-level global navigation function `ui.Navigate` is executed on a background goroutine. Within this goroutine, the code directly mutates the main window's container objects and refreshes it:
  ```go
  mainContainer.Objects = []fyne.CanvasObject{newUI}
  mainContainer.Refresh()
  navTree.SelectByVistaID(vID)
  ```
* **Impact**: Mutating `mainContainer.Objects` and calling `Refresh()` concurrently with Fyne's main layout and drawing loop can cause layout/rendering data races, visual corruption, or application panics.

### 1.2 Sidebar Selection (Programmatic)
* **Location**: [sidebar_widget.go:L32-57](file:///src/GolemUI/pkg/ui/sidebar_widget.go#L32-L57)
* **Description**: `SelectByVistaID` runs on the background goroutine spawned by `Navigate`. Within this function, the navigation tree branches are expanded and selections are made:
  ```go
  nt.tree.OpenBranch(widget.TreeNodeID(ancestors[i]))
  // ...
  nt.tree.Select(widget.TreeNodeID(nodeID))
  ```
* **Impact**: These calls directly mutate the visual tree state of the tree widget without synchronization, running concurrently with Fyne's UI interaction loop.

### 1.3 DataGrid Asynchronous Loading and Refreshes
* **Location**: [compositor.go:L409-414](file:///src/GolemUI/pkg/ui/compositor.go#L409-L414) and [compositor.go:L578-583](file:///src/GolemUI/pkg/ui/compositor.go#L578-L583)
* **Description**: Both `loadMasterBuffer` and `fetchGridDataAsync` run database queries in background goroutines. Upon completion, they update column widths and refresh the table:
  ```go
  model.refreshMu.Lock()
  for i := 0; i < len(headers); i++ {
      table.SetColumnWidth(i, 150)
  }
  table.Refresh()
  model.refreshMu.Unlock()
  ```
* **Impact**: While `model.refreshMu` prevents concurrent refreshes between different background threads, it does **not** protect against concurrent reads/writes from Fyne's UI rendering thread. Fyne's layout pass reads column widths and table structures concurrently, exposing the application to data races.

---

## 2. Memory and Goroutine Leaks in Screen Lifecycle

Screen changes must clean up resource bindings (cancellations of database query contexts, event unsubscribing) to prevent memory leaks and goroutine leaks.

### 2.1 Concurrently Overwritten Cleanup Closures
* **Location**: [main.go:L103-127](file:///src/GolemUI/cmd/golemui/main.go#L103-L127)
* **Description**: The `prevCleanup` variable is a shared package-level closure variable:
  ```go
  var prevCleanup func()
  ```
  If the user triggers multiple navigations in quick succession, multiple background goroutines run concurrently. 
  1. Goroutine 1 starts, executes `prevCleanup()`, then calls `Compose(ScreenB)`. It gets `cleanupB`.
  2. Before Goroutine 1 completes writing `prevCleanup = cleanupB`, Goroutine 2 starts and performs the check `prevCleanup != nil`.
  3. The goroutines can interleave such that `prevCleanup` is overwritten with `cleanupC` (for Screen C) without `cleanupB` (for Screen B) ever being invoked.
* **Impact**: The event subscriptions (`model.unsubscribe`) and the database cancel functions (`model.cancel`) for Screen B are never executed. This causes Screen B's composed widget tree to remain referenced by `LocalEventBus`, resulting in a permanent memory leak and keeping in-flight database query goroutines alive.

### 2.2 Premature Cleanup and Zombie Screens
* **Location**: [main.go:L106-110](file:///src/GolemUI/cmd/golemui/main.go#L106-L110)
* **Description**: In `Navigate`, the previous screen's cleanup function (`prevCleanup()`) is executed at the very beginning of the background goroutine, before the new screen's metadata is loaded or composed:
  ```go
  if prevCleanup != nil {
      prevCleanup()
      prevCleanup = nil
  }
  ```
  If `LoadScreen` (which fetches layouts from the DB core) fails or if UI composition fails, the navigation handler exits early.
* **Impact**: The previous screen's event bindings and query contexts are already cancelled, but its widgets remain visible in the container because `mainContainer.Objects` was never updated. This leaves the user on a non-responsive "zombie screen" where UI elements are broken or unresponsive.
* **Recommendation**: Cleanup of the old screen should only occur *after* the new screen is successfully loaded and composed, immediately prior to updating `mainContainer.Objects`.

---

## 3. Violations of the 4-Layer Architecture Model

GolemUI specifies a strict 4-layer model to segregate physical schemas and business logic from the UI client.

### 3.1 Direct SQL & DB Driver Awareness in Layer 4 (Fyne Renderer)
* **Location**: [compositor.go:L364](file:///src/GolemUI/pkg/ui/compositor.go#L364), [L534](file:///src/GolemUI/pkg/ui/compositor.go#L534), and [L587-603](file:///src/GolemUI/pkg/ui/compositor.go#L587-L603)
* **Description**: 
  - The compositor package directly accesses database connection pools (`BusinessPool` and `CorePool`) and runs raw SQL queries.
  - The compositor imports `database/sql/driver` and manually processes database-specific types using the `driver.Valuer` interface:
    ```go
    if valuer, ok := val.(driver.Valuer); ok { ... }
    ```
* **Impact**: The Go renderer (Layer 4) is coupled to database driver details and SQL-specific data formats, violating the boundary where the UI should consume clean abstractions and logical UI data objects.

### 3.2 Hardcoded Layout Override Details
* **Location**: [compositor.go:L411](file:///src/GolemUI/pkg/ui/compositor.go#L411) and [L580](file:///src/GolemUI/pkg/ui/compositor.go#L580)
* **Description**: The grid column width is hardcoded to `150` pixels (`table.SetColumnWidth(i, 150)`) inside the rendering loops.
* **Impact**: Layout override sizes are hardcoded in the Go binary. Column sizing behavior should ideally be read dynamically from Layer 2/3 metadata overrides, allowing design customizations to remain purely database-driven.

---

## 4. Code Quality, Anti-patterns, and Go Idioms

### 4.1 Global Mutable Package State
* **Location**: [compositor.go:L19-22](file:///src/GolemUI/pkg/ui/compositor.go#L19-L22)
* **Description**: Global package-level variables are utilized to store database pools, event buses, and the global navigation function:
  ```go
  var BusinessPool db.DatabasePool
  var CorePool db.DatabasePool
  var LocalEventBus eventbus.EventBus
  var Navigate func(vistaID string)
  ```
* **Impact**: This mutable package state creates synchronization hazards, prevents running concurrent tests in parallel, and makes it impossible to instantiate multiple independent UI shells/windows. These should be encapsulated into a dedicated context struct.

### 4.2 Non-Atomic Re-entrancy Guard (Data Race)
* **Location**: [sidebar_widget.go:L18](file:///src/GolemUI/pkg/ui/sidebar_widget.go#L18), [L54-55](file:///src/GolemUI/pkg/ui/sidebar_widget.go#L54-L55), and [L135-137](file:///src/GolemUI/pkg/ui/sidebar_widget.go#L135-L137)
* **Description**: `nt.navigating` is a standard Go boolean used to guard against re-entrant calls when programmatic selection occurs:
  ```go
  nt.navigating = true
  defer func() { nt.navigating = false }()
  nt.tree.Select(...)
  ```
  However, this boolean is written to in `SelectByVistaID` (executing on the background goroutine) and read in the tree's `OnSelected` callback (executing on the main UI thread).
* **Impact**: This concurrent read and write constitutes a classic data race. The guard should be implemented using a thread-safe primitive like `atomic.Bool` or protected by a mutex.

### 4.3 Missing Cleanup of Database Pools
* **Location**: [main.go:L167-200](file:///src/GolemUI/cmd/golemui/main.go#L167-L200)
* **Description**: While early bootstrap error paths close the database pool, there is no shutdown hook or defer statement in the `main` entry point to cleanly close the database connection pools (`dbPool.Close()`) on graceful window closures.
* **Impact**: Connections are abruptly terminated on exit rather than closed gracefully.
a015-datagrid-native-type-preservation.md

### PERSONA

Desarrollador Senior de Software en Go y Arquitecto de Datos, experto en el mapeo de tipos físicos de bases de datos relacionales a estructuras lógicas y en la conservación de tipos primitivos en el frontend.

### CONTEXTO Y ANCLAJE

- **Iniciativa/Proyecto:** Motor de renderizado GolemUI.
- **Archivos de Referencia:**
  - `@pkg/ui/compositor.go` (Estructura `dataGridModel`, funciones `loadMasterBuffer`, `fetchGridDataAsync`, `filterMasterRows` y callback `OnSelected` de `widget.Table`)
- **Línea de Base:** Basándose estrictamente en la información técnica provista en los archivos referenciados, el modelo `dataGridModel` almacena los datos de las filas como cadenas de texto (`[][]string`), degradando los tipos originales (enteros, booleanos, flotantes) mediante la conversión temprana en `formatValue` durante la carga de base de datos. Como consecuencia, al seleccionar una fila, el evento `publish_selection` transporta los valores formateados como cadenas de texto, perdiendo los tipos nativos requeridos para expresiones lógicas complejas en el cliente.

### TAREA (EL "QUÉ")

Preservar los tipos nativos en el modelo de datos del grid:

1. **Tipado Genérico de Almacenamiento:** Modificar los campos `rows` y `masterRows` de la estructura `dataGridModel` para que almacenen arreglos bidimensionales genéricos de tipo `[][]any`.
2. **Inyección Directa de Datos de la BD:** En las funciones `loadMasterBuffer` y `fetchGridDataAsync`, almacenar directamente los valores leídos (`vals`) obtenidos de la fila de la base de datos (`rows.Values()`) dentro de la estructura `dataGridModel` en su formato original.
3. **Formateo Visual Tardío:** Mover la llamada a la función `formatValue` para que sea invocada exclusivamente dentro del callback visual de actualización de celdas (`UpdateCell` y cabeceras si corresponde) al momento de dibujar los datos en la pantalla.
4. **Preservación de Tipos en Selección:** En el callback `OnSelected` del `widget.Table`, estructurar el mapa `rowMap` utilizando los valores originales de tipo `any` procedentes de `model.rows[id.Row]`, enviando los tipos primitivos en su estado nativo a través de `"publish_selection"`.
5. **Conversión Dinámica en Filtro:** Ajustar el método `filterMasterRows` para realizar la conversión a cadena de texto de forma dinámica y realizar las comparaciones de texto ignorando mayúsculas/minúsculas sin alterar el almacenamiento de tipo `any`.

### DIRECTRICES EXCLUYENTES POSITIVAS (LÍMITES DE ALCANCE)

- **Enfoque Principal:** Limita el alcance exclusivamente a modificar los campos `rows` y `masterRows` a `[][]any` para resguardar los tipos originales hasta el dibujo visual y la publicación del mapa de selección.
- **Fuera de Alcance:**
  - Mantén intacta la firma del método `formatValue` y su lógica de conversión de tipos mediante `driver.Valuer`.
  - Restringe la manipulación de datos del grid a las funciones internas del compositor en `@pkg/ui/compositor.go`.
  - Enfócate estrictamente en conservar la firma del canal de publicación `"publish_selection"` como `map[string]any`.

### CRITERIOS DE ACEPTACIÓN (VALIDACIÓN BINARIA)

- **Métrica de Éxito:** Las pruebas en `@pkg/ui/compositor_test.go` deben validar que:
  1. La selección de una fila que contenga tipos enteros, booleanos y flotantes devuelva en `"publish_selection"` un mapa con sus tipos nativos correspondientes (por ejemplo, `int64`, `bool`, `float64`), en lugar de cadenas de texto.
  2. La visualización de la tabla mantenga el formateo visual a cadena de texto idéntico al comportamiento original.
