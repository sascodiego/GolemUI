### PERSONA

Actúa como un Diseñador data-driven ui Frontend de Escritorio Senior especializado en Fyne (Go), enfocado en la construcción de layouts modulares, renderizado dinámico y binding de eventos estructurados.

### CONTEXTO Y ANCLAJE

- **Iniciativa/Proyecto:** Barra lateral modular de navegación en GolemUI.
- **Archivos de Referencia:**
  - `@/src/GolemUI/pkg/ui/compositor.go`: Contiene la definición de `NodeMeta` y el callback global `Navigate`.
- **Línea de Base:** Basándose estrictamente en la información técnica provista en los archivos referenciados:
  - El compositor recibe y procesa estructuras `NodeMeta` y delega la navegación a través del callback `Navigate` configurado globalmente.
  - Se requiere crear un componente de navegación persistente tipo árbol que llame a este flujo de navegación al interactuar con las hojas.

### TAREA (EL "QUÉ")

Construir y configurar el widget interactivo `widget.Tree` de Fyne a partir de la lista de `MenuItem` recuperada de la base de datos core.

1. **Estructuras Auxiliares:**
   Procesar la lista plana de `MenuItem` para generar:
   - `parentToChildren map[string][]string`: Mapea el ID de un padre (o `""` para raíces) a la lista ordenada de IDs de sus hijos directos.
   - `idToItem map[string]MenuItem`: Mapea cada ID de ítem a su struct `MenuItem`.
2. **Definición del Widget Tree:**
   Instanciar y configurar el widget `widget.NewTree` usando las funciones:
   - **ChildUIDs:** `func(uid string) []string` que retorne la lista de IDs del mapa `parentToChildren`.
   - **IsBranch:** `func(uid string) bool` que determine si el UID tiene registros hijos en `parentToChildren`.
   - **CreateNode:** `func(branch bool) fyne.CanvasObject` que instancie el widget visual del nodo (ej. un widget label o contenedor de texto e ícono).
   - **UpdateNode:** `func(uid string, branch bool, obj fyne.CanvasObject)` que configure la información visual con el título correcto desde `idToItem`.
   - **OnSelected:** `func(uid string)` que identifique si el elemento seleccionado es una hoja (no es rama) y posee un `vista_id` configurado, ejecutando el callback global `ui.Navigate(vistaID)`.

### DIRECTRICES EXCLUYENTES POSITIVAS (LÍMITES DE ALCANCE)

- **Enfoque Principal:** Construir el widget Tree interactivo de Fyne y mapear sus eventos con el cargador de pantallas.
- **Fuera de Alcance:**
  - Limita el alcance exclusivamente a la construcción, mapeo de datos e interacción del widget Tree.
  - Enfócate estrictamente en mantener intacta la jerarquía de layouts y composición de los widgets del panel derecho de la aplicación.
  - Restringe la manipulación visual del Tree únicamente a la lectura del modelo de datos de menú cargado.

### CRITERIOS DE ACEPTACIÓN (VALIDACIÓN BINARIA)

- **Métrica de Éxito:** Un test o verificación visual que demuestre que el widget Tree se instancia correctamente usando el modelo de prueba, puebla los nodos con los títulos correspondientes y que la invocación del callback `OnSelected` en una hoja activa correctamente la función `ui.Navigate` con el ID de vista correspondiente.
