### PERSONA

Actúa como un Ingeniero de Software Principal enfocado en arquitectura Go limpia, diseño de algoritmos seguros y control de ciclos recursivos en estructuras de datos arborescentes.

### CONTEXTO Y ANCLAJE

- **Iniciativa/Proyecto:** Barra lateral modular de navegación en GolemUI.
- **Archivos de Referencia:**
  - `@/src/GolemUI/pkg/db/db.go`: Define la interfaz `DatabasePool` y el pool de conexiones.
  - `@/src/GolemUI/pkg/ui/screen_loader.go`: Servicio cargador de pantallas que sirve como referencia para la interacción con la base de datos core.
- **Línea de Base:** Basándose estrictamente en la información técnica provista en los archivos referenciados:
  - GolemUI cuenta con conexiones de red independientes hacia las bases de datos core y negocio.
  - Se requiere un nuevo cargador que lea la configuración de menús e implemente validaciones estructurales contra recursión infinita en el cliente.

### TAREA (EL "QUÉ")

Implementar el cargador en Go para los elementos de menú desde la base de datos core, organizándolos jerárquicamente y validando la ausencia de ciclos relacionales recursivos.

1. **Estructura MenuItem:**
   Definir en el paquete `ui` la estructura `MenuItem`:
   ```go
   type MenuItem struct {
       ID       string
       PadreID  string
       Titulo   string
       VistaID  string
       Orden    int
   }
   ```
2. **Función de Carga:**
   Escribir la función `LoadNavigationMenu(ctx context.Context, pool db.DatabasePool) ([]MenuItem, error)` que realice un SELECT ordenado de la tabla `golemui.menu_navegacion`.
3. **Validación Antirrecursión (DFS):**
   Diseñar una rutina de validación basada en Búsqueda en Profundidad (DFS) que recorra las relaciones padre-hijo detectando ciclos en el árbol cargado.
4. **Manejo de Errores:**
   Interrumpir el inicio de la navegación y retornar un error detallado si se detecta un bucle de referencias circulares (ej. un nodo que sea su propio ancestro).

### DIRECTRICES EXCLUYENTES POSITIVAS (LÍMITES DE ALCANCE)

- **Enfoque Principal:** Desarrollar el servicio cargador de menús y la validación de ciclos en el paquete de lógica de Go.
- **Fuera de Alcance:**
  - Limita el alcance exclusivamente a la obtención de los menús desde la base de datos y la validación de ciclos en memoria.
  - Mantén intactos los comportamientos y métodos del cargador de pantallas `LoadScreen` y del compositor de widgets existente.
  - Restringe la manipulación del árbol de menús únicamente a operaciones de lectura y validación sin edición en tiempo de ejecución.

### CRITERIOS DE ACEPTACIÓN (VALIDACIÓN BINARIA)

- **Métrica de Éxito:** La prueba unitaria en `pkg/ui/sidebar_loader_test.go` debe pasar exitosamente, validando que:
  - Una jerarquía de menús válida y acíclica sea cargada y ordenada correctamente.
  - Una jerarquía de menús maliciosa con ciclos (ej. A -> B -> A) sea rechazada y dispare un error descriptivo de ciclo relacional.
