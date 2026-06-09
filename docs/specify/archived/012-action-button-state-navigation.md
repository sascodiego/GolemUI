### PERSONA

Arquitecto de Software Senior y Especialista en Navegación Dinámica y Orquestación de Flujos de Trabajo en Aplicaciones Go/Fyne.

### CONTEXTO Y ANCLAJE

- **Iniciativa/Proyecto:** Motor de renderizado dinámico GolemUI.
- **Archivos de Referencia:**
  - `@pkg/ui/compositor.go` (Componente `"button"`, `ui.Navigate`)
  - `@pkg/ui/screen_state.go` (`ScreenState` y precarga de parámetros)
  - `@cmd/golemui/main.go` (Definición del callback `ui.Navigate`)
- **Línea de Base:** Basándose estrictamente en la información técnica provista en los archivos referenciados, el callback de navegación `ui.Navigate` en `main.go` recibe un `vistaID` plano, carga la pantalla y la compone de cero. El botón realiza la navegación de forma estática o envía un submit del Snapshot actual, pero no puede condicionar su habilitación al estado de selección del grid ni propagar parámetros dinámicos en la navegación.

### TAREA (EL "QUÉ")

Habilitar el control dinámico del estado de habilitación del botón en base a eventos de selección, mapear parámetros utilizando Dot-Notation y extender el enrutador para procesar query strings de navegación:

1. **Habilitación Reactiva por Selección:**
   - Permitir al componente `"button"` suscribirse al canal `"publish_selection"` (o al configurado en el nodo).
   - El botón debe inicializarse en estado deshabilitado (`button.Disable()`).
   - Al recibir un evento en dicho canal, si el payload representa una selección válida (un mapa no vacío), habilitar el botón (`button.Enable()`). Si el evento indica la pérdida o deselección, devolver el botón a su estado deshabilitado.
2. **Mapeo de Parámetros de Navegación (`param_mapping`):**
   - Permitir al nodo definir un mapeo de parámetros (ej. a través de un campo `"param_mapping"` en `NodeMeta` u otra vía configurable de mapeo).
   - Al hacer click en el botón, evaluar los caminos definidos en el mapeo contra la fila/objeto seleccionado usando la notación Dot-Notation recursiva (`resolvePath`).
   - Construir una URL de navegación con formato query string (ej. `navigate:detalle_transaccion?id=42&monto=1000.50`) a partir de los parámetros resueltos.
3. **Extensión del Enrutador `ui.Navigate`:**
   - Extender la callback de navegación `ui.Navigate` en `main.go` para que acepte formatos con query string (ej. `"pantalla_destino?clave1=valor1&clave2=valor2"`).
   - Implementar un parser que divida la URL en `vistaID` y la cadena de argumentos (`queryParams`).
   - El `vistaID` limpio se utilizará para cargar el layout de pantalla en la base de datos core.
   - Los argumentos del query string se deben inyectar en caliente dentro del nuevo objeto `ScreenState` asociado a la pantalla de destino, haciéndolos disponibles inmediatamente para la composición de sus widgets hijos (ej. como valores por defecto en text_inputs o parámetros para sus data_grids).

### DIRECTRICES EXCLUYENTES POSITIVAS (LÍMITES DE ALCANCE)

- **Enfoque Principal:** Limita el alcance exclusivamente a controlar el estado de habilitación/deshabilitación del botón según los eventos de selección, evaluar el mapeo de parámetros de navegación, dividir el query string de navegación y popular el estado de la pantalla destino.
- **Fuera de Alcance:**
  - Mantén intacta la lógica existente de carga del layout de pantalla desde `CorePool` utilizando el `vistaID` limpio.
  - Limita la propagación de parámetros del query string al estado de la pantalla destino mediante tipos de datos planos (`string` o `any`).
  - Enfócate estrictamente en procesar la deshabilitación/habilitación del botón a través del hilo principal de UI de Fyne.

### CRITERIOS DE ACEPTACIÓN (VALIDACIÓN BINARIA)

- **Métrica de Éxito:** Las pruebas automatizadas en `@pkg/ui/compositor_test.go` o `@pkg/ui/screen_state_test.go` deben validar con éxito:
  1. Que un botón configurado para escuchar eventos de selección cambie dinámicamente su estado de `Disabled` a `Enabled` tras recibir un mensaje en `"publish_selection"`.
  2. Que la invocación de `ui.Navigate` con la cadena `"detalle?id=99&tipo=debito"` resuelva correctamente `"detalle"` como el ID de la pantalla a cargar.
  3. Que el `ScreenState` creado para la pantalla `"detalle"` tenga precargados los valores `"99"` para la clave `"id"` y `"debito"` para la clave `"tipo"`, verificando la inyección exitosa de los parámetros de la URL.
