### PERSONA

Adopta la perspectiva de un Arquitecto de Software Senior con más de 15 años de experiencia, experto en patrones de diseño para interfaces de usuario reactivas (Data-Driven UI), desacoplamiento de estado y optimización de flujos de datos asíncronos en Go.

### CONTEXTO Y ANCLAJE

- **Iniciativa/Proyecto:** GolemUI Desktop Client.
- **Archivos de Referencia:**
  - `@pkg/ui/compositor.go` (composición de componentes de la UI)
  - `@pkg/ui/screen_loader.go` (cargador de layouts y metadatos)
  - `@pkg/eventbus/eventbus.go` (mecanismo de mensajería reactiva local)
- **Línea de Base:** Basándose estrictamente en la información técnica provista en los archivos referenciados, el motor actual mapea `"text_input"` para que publiquen cambios inmediatamente en el EventBus, y `"data_grid"` se suscribe al cambio de forma directa y uno a uno. Debemos evolucionar esto a un almacenamiento de estado genérico.

### TAREA (EL "QUÉ")

1. **Diseñar e implementar el ScreenState Store:** Crear una estructura atómica en el cliente para almacenar y centralizar el estado actual de los inputs de una pantalla (un mapa llave-valor de filtros configurados).
2. **Definir el flujo de publicación indirecta:** Modificar los widgets de entrada de datos (inputs) para que al cambiar no gatillen el refresco de la grilla directamente, sino que actualicen el ScreenState Store de la pantalla local.
3. **Implementar el evento consolidado de SUBMIT:** Configurar el widget `"button"` con acción de actualizar/buscar para que, al ser presionado, publique en el EventBus un evento unificado `SUBMIT` llevando como payload la totalidad del estado acumulado en el ScreenState Store.
4. **Desarrollar el polimorfismo de filtrado en la Grilla:** Configurar el `"data_grid"` para suscribirse al evento `SUBMIT` y procesar el payload del filtro en base a dos modos de ejecución:
   - **Modo Server-side:** La grilla toma las claves del estado y las pasa directamente como parámetros posicionales en la consulta parametrizada a la base de datos a través de `BusinessPool`.
   - **Modo Client-side:** La grilla mantiene en memoria un búfer de datos maestros completos (cargado una única vez en el bootstrap inicial) y aplica filtros en caliente localmente en memoria usando las claves del estado, sin re-consultar a la base de datos.

### DIRECTRICES EXCLUYENTES POSITIVAS (LÍMITES DE ALCANCE)

- **Enfoque Principal:** Abstracción del estado de los inputs, mensajería mediante evento consolidado de Submit, y lógica polimórfica (servidor/cliente) para la grilla de datos.
- **Fuera de Alcance:**
  - Limita el alcance exclusivamente a la grilla y sus inputs asociados.
  - Enfócate estrictamente en mantener la compatibilidad con el sistema de renderizado recursivo existente de Fyne.
  - Restringe la manipulación de datos en memoria del cliente únicamente a los escenarios donde el modo de filtrado de la vista esté explícitamente configurado como local o cliente.

### CRITERIOS DE ACEPTACIÓN (VALIDACIÓN BINARIA)

- **Métrica de Éxito:** Pruebas unitarias automatizadas completas en `@pkg/ui/screen_loader_test.go` (o un nuevo archivo de tests de comportamiento de filtros en `@pkg/ui`) que validen exitosamente:
  1. Que la escritura en múltiples inputs actualice correctamente el mapa único de estado.
  2. Que el botón de actualizar consolide y emita dicho mapa completo en el evento de Submit.
  3. Que la grilla filtre correctamente de forma local (Client-side) sobre un conjunto de datos cargados previamente.
  4. Que la grilla gatille consultas parametrizadas con variables múltiples ($1, $2, etc.) en modo Server-side.
