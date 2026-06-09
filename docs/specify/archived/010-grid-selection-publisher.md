### PERSONA

Desarrollador Senior de Interfaces de Usuario en Fyne y Diseñador de Sistemas Reactivos basados en Eventos, enfocado en robustez multihilo y sincronización segura de UI en Go.

### CONTEXTO Y ANCLAJE

- **Iniciativa/Proyecto:** Motor de renderizado dinámico GolemUI.
- **Archivos de Referencia:**
  - `@pkg/ui/compositor.go` (Componente `"data_grid"`, modelo `dataGridModel`)
  - `@pkg/eventbus/eventbus.go` (Publicación en el EventBus e interfaz `EventBus`)
- **Línea de Base:** Basándose estrictamente en la información técnica provista en los archivos referenciados, el componente `"data_grid"` utiliza un `widget.Table` de Fyne asociado a un modelo `dataGridModel` protegido por un mutex de lectura/escritura (`model.mu`). Sin embargo, carece de captura de selección de filas y de la posterior propagación de eventos al bus local.

### TAREA (EL "QUÉ")

Habilitar la captura interactiva de selección de filas dentro del widget `widget.Table` en el compositor y publicar el estado resultante en el bus de eventos:

1. **Configurar el Callback de Selección:** Asignar una función callback a la propiedad `OnSelected` del `widget.Table` instanciado para el caso `"data_grid"`.
2. **Validar Límites del Modelo:** Dentro del callback, obtener el ID de celda seleccionado (`id.Row`) y verificar que el índice de la fila esté dentro del rango válido de filas cargadas en memoria en `model.rows`.
3. **Mapeo Dinámico de Headers y Valores:** Emparejar de forma posicional cada columna de la fila seleccionada con su cabecera correspondiente de `model.headers`. Generar un mapa genérico `map[string]any` donde cada clave sea el nombre de la cabecera (header) y el valor sea el contenido formateado de la celda en esa posición.
4. **Publicación Segura Multihilo:** Publicar el mapa resultante en la instancia global de `LocalEventBus` bajo el canal de evento `"publish_selection"`. Garantizar la consistencia de los datos consultados mediante bloqueos de lectura de `model.mu`.

### DIRECTRICES EXCLUYENTES POSITIVAS (LÍMITES DE ALCANCE)

- **Enfoque Principal:** Limita el alcance exclusivamente a capturar la fila seleccionada, estructurar sus celdas bajo los headers en un mapa `map[string]any` y publicar dicho mapa en `"publish_selection"`.
- **Fuera de Alcance:**
  - Mantén intacta la lógica existente de carga asíncrona de datos desde el origen de datos (métodos `fetchGridDataAsync` y `loadMasterBuffer`).
  - Restringe la manipulación del bus de eventos únicamente al canal `"publish_selection"`.
  - Enfócate estrictamente en proteger el acceso al modelo de datos mediante el mutex `model.mu` para evitar condiciones de carrera.

### CRITERIOS DE ACEPTACIÓN (VALIDACIÓN BINARIA)

- **Métrica de Éxito:** La prueba automatizada unitaria en `@pkg/ui/compositor_test.go` debe pasar exitosamente. La prueba debe:
  1. Instanciar una vista que contenga un `"data_grid"`.
  2. Popular el `dataGridModel` con cabeceras `["id", "nombre", "monto"]` y al menos una fila de datos `["42", "Transaccion Test", "1000.50"]`.
  3. Ejecutar el callback `OnSelected` simulando la selección de la fila `0`.
  4. Verificar que se reciba en el canal `"publish_selection"` un evento con el payload exacto `map[string]any{"id": "42", "nombre": "Transaccion Test", "monto": "1000.50"}`.
