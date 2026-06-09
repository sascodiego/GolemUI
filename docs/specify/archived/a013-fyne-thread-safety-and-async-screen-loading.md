a013-fyne-thread-safety-and-async-screen-loading.md

### PERSONA

Desarrollador Senior de Interfaces de Usuario en Go y Arquitecto de Aplicaciones de Escritorio con Fyne, especializado en programación concurrente, seguridad de hilos y prevención de bloqueos visuales en el hilo principal.

### CONTEXTO Y ANCLAJE

- **Iniciativa/Proyecto:** Motor de renderizado dinámico GolemUI.
- **Archivos de Referencia:**
  - `@cmd/golemui/main.go` (Definición de `ui.Navigate`)
  - `@pkg/ui/compositor.go` (Carga y mutación de la interfaz gráfica y del componente `"data_grid"`)
- **Línea de Base:** Basándose estrictamente en la información técnica provista en los archivos referenciados, el motor de GolemUI ejecuta operaciones de red/base de datos (como `ui.LoadScreen` en `ui.Navigate`) directamente sobre el hilo principal de Fyne, lo que provoca congelamientos en la interfaz de usuario. Asimismo, las mutaciones directas sobre widgets visuales (`table.Refresh` y `table.SetColumnWidth`) desde goroutines secundarias (en `loadMasterBuffer` y `fetchGridDataAsync`) violan el modelo multihilo de Fyne.

### TAREA (EL "QUÉ")

Establecer la seguridad de hilos en Fyne y asincronía en la carga de pantallas:

1. **Mutaciones de UI Seguras en el Compositor:** Envolver cada actualización visual del widget `widget.Table` (específicamente las llamadas a `table.Refresh()` y `table.SetColumnWidth()`) dentro del compositor en la función segura de Fyne `fyne.Do()`.
2. **Navegación Asíncrona en Main:** Modificar la función `ui.Navigate` en `main.go` para que despache la ejecución de `ui.LoadScreen` y `ui.Compose` dentro de una nueva goroutine de fondo.
3. **Aplicación del Cambio en el Hilo Principal:** En `ui.Navigate`, una vez obtenida la nueva interfaz compuesta (`newUI`), invocar `win.SetContent(newUI)` dentro de un bloque `fyne.Do()` para garantizar la inserción correcta y segura en el hilo principal.
4. **Control de Errores Concurrente:** Capturar cualquier error surgido de `LoadScreen` o `Compose` dentro de la goroutine de fondo y registrar el log correspondiente, asegurando que la interfaz previa continúe respondiendo sin congelarse.

### DIRECTRICES EXCLUYENTES POSITIVAS (LÍMITES DE ALCANCE)

- **Enfoque Principal:** Limita el alcance exclusivamente a envolver las mutaciones del widget visual en `fyne.Do()` y a delegar la carga de pantallas a una goroutine de fondo en `ui.Navigate`.
- **Fuera de Alcance:**
  - Mantén intacta la lógica interna de base de datos de `LoadScreen` y los parámetros de consulta del archivo de configuración.
  - Restringe la manipulación de la interfaz gráfica únicamente a los componentes de navegación en `main.go` y la actualización del grid en `compositor.go`.
  - Enfócate estrictamente en conservar la firma actual de `ui.Navigate` y las propiedades existentes de `NodeMeta`.

### CRITERIOS DE ACEPTACIÓN (VALIDACIÓN BINARIA)

- **Métrica de Éxito:** Las pruebas de integración en `@cmd/golemui/main_test.go` o pruebas del cargador deben validar que:
  1. Al invocar `ui.Navigate`, la llamada retorne el control inmediatamente al hilo invocador sin bloquear la ejecución (comportamiento asíncrono).
  2. La actualización final de la ventana (`win.SetContent`) sea ejecutada bajo el despachador de hilos de `fyne.Do()`.
  3. Todas las llamadas a `table.Refresh()` y `table.SetColumnWidth()` dentro de goroutines en `compositor.go` se realicen encapsuladas en `fyne.Do()`.
