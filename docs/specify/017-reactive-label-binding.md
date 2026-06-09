### PERSONA

Desarrollador Senior de Motores de Renderizado en Go y Especialista en Parsing/Compilación de Expresiones Dinámicas para Interfaces de Usuario Reactivas.

### CONTEXTO Y ANCLAJE

- **Iniciativa/Proyecto:** Motor de renderizado dinámico GolemUI.
- **Archivos de Referencia:**
  - `@pkg/ui/compositor.go` (Componente `"label"`)
  - `@pkg/eventbus/eventbus.go` (Suscripción y manejo de callbacks de eventos)
- **Línea de Base:** Basándose estrictamente en la información técnica provista en los archivos referenciados, el componente `"label"` actualmente se compone como un widget estático `widget.NewLabel(node.Label)`. No posee capacidad de suscripción a canales del EventBus ni de resolución dinámica de plantillas en base a payloads estructurados (JSON/mapas).

### TAREA (EL "QUÉ")

Implementar un mecanismo de enlace reactivo para el widget `Label` basado en plantillas dinámicas y suscripciones al EventBus en runtime, asegurando compatibilidad de hilos:

1. **Detección de Enlace Reactivo:** Comprobar si el `NodeMeta` del componente `"label"` define un canal de suscripción en su propiedad de metadatos JSON `DataSource` (ej. si inicia con el formato `"event:"` o define un nombre de canal como `"publish_selection"`).
2. **Suscripción al EventBus:** Si se detecta un canal válido en la propiedad `DataSource`, suscribir el widget `Label` a dicho canal en `LocalEventBus` al momento de la composición.
3. **Resolución de Caminos Anidados (Dot-Notation):** Diseñar y utilizar una función auxiliar recursiva en Go:
   `func resolvePath(data any, path string) any`
   Esta función debe recibir un objeto genérico (como `map[string]any`) y un camino separado por puntos (ej. `"cliente.direccion.calle"`). Debe navegar jerárquicamente por el mapa y retornar el valor final, controlando accesos seguros e interfaces vacías.
4. **Procesamiento de la Plantilla (Template Parsing):** Al recibir un evento en el canal, tomar el string original de `node.Label` como una plantilla (ej. `"Monto: {monto} - Cliente: {cliente.nombre}"`). Identificar todos los tokens entre llaves `{...}`, extraer el camino interior, resolverlo contra el payload del evento usando `resolvePath` y reemplazar el token con el valor string resultante.
5. **Actualización Segura en UI:** Actualizar el widget gráfico de Fyne mediante `label.SetText(resolvedText)`. Despachar esta actualización al hilo principal de la UI utilizando `fyne.Do(func() { ... })` para garantizar la estabilidad del toolkit visual, ya que los callbacks del `EventBus` corren de forma asíncrona.

### DIRECTRICES EXCLUYENTES POSITIVAS (LÍMITES DE ALCANCE)

- **Enfoque Principal:** Limita el alcance exclusivamente a suscribir el widget Label al canal provisto en la propiedad `DataSource` de `NodeMeta`, parsear tokens `{}` en el campo `node.Label` mediante navegación Dot-Notation y actualizar el widget en el hilo principal de la UI mediante `fyne.Do`.
- **Fuera de Alcance:**
  - Mantén intacta la instanciación estática del widget `Label` cuando la propiedad `DataSource` de `NodeMeta` se encuentre vacía.
  - Limita el procesamiento recursivo de `resolvePath` estrictamente a tipos `map[string]any` y valores escalares convertibles a string.
  - Restringe la suscripción del ciclo de vida del widget para que se desuscriba adecuadamente al destruirse la pantalla o destruirse el contenedor, retornando una función de limpieza (cleanup) desde la composición del componente `"label"`, evitando fugas de memoria en el EventBus.
  - Preserva separada la propiedad JSON `node.DataSource` del backend global de datos representado por la interfaz Go `ui.DataSource`.

### CRITERIOS DE ACEPTACIÓN (VALIDACIÓN BINARIA)

- **Métrica de Éxito:** La prueba automatizada unitaria en `@pkg/ui/compositor_test.go` (o un nuevo archivo de test `@pkg/ui/label_binding_test.go`) debe completarse con éxito. La prueba debe:
  1. Validar la función `resolvePath` con un mapa estructurado anidado:
     `{"transaccion": {"id": 101, "detalles": {"moneda": "USD", "valor": 500.0}}}`
     Verificar que `resolvePath(data, "transaccion.detalles.valor")` retorne exactamente `500.0`.
  2. Verificar que una plantilla `"Monto: {transaccion.detalles.valor} {transaccion.detalles.moneda}"` se resuelva exactamente como `"Monto: 500 USD"` al procesarse el evento del bus.
  3. Comprobar que el widget `Label` refleje el cambio de texto esperado tras el disparo del evento dentro del hilo de Fyne.
