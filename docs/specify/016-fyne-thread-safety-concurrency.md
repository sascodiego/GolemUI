### PERSONA

Arquitecto Principal de Sistemas Go y Experto en Desarrollo Concurrente con Toolkits Gráficos (Fyne / UI Thread Safety).

### CONTEXTO Y ANCLAJE

- **Iniciativa/Proyecto:** Motor de renderizado dinámico GolemUI.
- **Archivos de Referencia:**
  - `@cmd/golemui/main.go` (Callback de navegación `ui.Navigate`)
  - `@pkg/ui/sidebar_widget.go` (Mecanismo de selección y apertura de ramas del árbol de navegación)
  - `@pkg/ui/compositor.go` (Composición de componentes `"data_grid"`, `"button"`, `"label"`)
  - `@pkg/eventbus/eventbus.go` (Ejecución de handlers del bus en goroutines de fondo)
- **Línea de Base:** Basándose estrictamente en la información técnica provista en los archivos referenciados, el motor de renderizado realiza mutaciones visuales y de estado de los widgets gráficos de Fyne de manera asíncrona dentro de goroutines secundarias (como la carga de pantallas en `Navigate`, la actualización de celdas y la redimensión de columnas en `loadMasterBuffer`/`fetchGridDataAsync`, y los callbacks del `EventBus` que se despachan concurrentemente). Esto desencadena advertencias de seguridad en el hilo de Fyne ("Error in Fyne call thread") y riesgos de colisión de concurrencia ("concurrent map writes").

### TAREA (EL "QUÉ")

Establecer un patrón estricto de seguridad de hilos (Thread-Safety) para toda interacción con el toolkit gráfico Fyne, garantizando que cualquier modificación estructural o visual se ejecute en el hilo principal de la UI:

1. **Sincronización en Navegación y Pantallas:**
   - En el callback `ui.Navigate` de `main.go`, asegurar que las modificaciones al contenedor principal (`mainContainer.Objects = ...`, `mainContainer.Refresh()`) y la sincronización visual del árbol (`navTree.SelectByVistaID(vID)`) se despachen mediante `fyne.Do(func() { ... })`.
2. **Sincronización de Selección de Árbol:**
   - En `SelectByVistaID` de `sidebar_widget.go`, asegurar que las operaciones de mutación del widget `Tree` (`nt.tree.OpenBranch` y `nt.tree.Select`) se realicen de manera segura en el hilo de la UI.
3. **Sincronización en Grillas y Tablas:**
   - En las funciones `loadMasterBuffer` y `fetchGridDataAsync` de `compositor.go`, envolver las llamadas a `table.SetColumnWidth` y `table.Refresh()` dentro de un bloque `fyne.Do(func() { ... })` al completarse la carga de datos en segundo plano.
   - En `filterMasterRows` de `compositor.go`, envolver la llamada a `table.Refresh()` dentro de un bloque `fyne.Do(func() { ... })`.
4. **Sincronización en Enlaces Reactivos y Botones:**
   - En los callbacks de suscripción del `EventBus` (que se ejecutan en goroutines independientes), asegurar que cualquier llamada que altere el estado de un widget visual (como `label.SetText` o `button.Enable`/`button.Disable`) se despache a través de `fyne.Do(func() { ... })`.

### DIRECTRICES EXCLUYENTES POSITIVAS (LÍMITES DE ALCANCE)

- **Enfoque Principal:** Limita el alcance exclusivamente a asegurar que todas las llamadas mutadoras de widgets en Fyne se ejecuten en el hilo principal utilizando `fyne.Do`, manteniendo la ejecución de consultas y lógica de persistencia en goroutines de fondo.
- **Fuera de Alcance:**
  - Restringe el uso de `fyne.Do` únicamente a operaciones que involucren el estado gráfico o estructural de la interfaz de usuario (ej. refrescos de widgets, seteos de texto, cambios de tamaño, habilitación y selección).
  - Mantén el procesamiento de obtención de datos desde bases de datos (`Fetch`, `FetchAll`) y parseos de datos estructurados de manera asíncrona en goroutines secundarias e independientes.
  - Conserva intactos los mecanismos de sincronización interna del estado en memoria basados en mutex (`sync.RWMutex`) en `dataGridModel` para proteger los datos en crudo.

### CRITERIOS DE ACEPTACIÓN (VALIDACIÓN BINARIA)

- **Métrica de Éxito:** La verificación del comportamiento concurrente se considerará exitosa si:
  1. El compilado de la aplicación se ejecuta y realiza transiciones de navegación y filtrado de tablas sin emitir la advertencia "Error in Fyne call thread, this should have been called in fyne.Do".
  2. Las pruebas unitarias concurrentes que simulan cargas rápidas en paralelo (como en `compositor_test.go`) finalizan exitosamente sin provocar pánicos por acceso concurrente no seguro de mapas ("concurrent map writes").
