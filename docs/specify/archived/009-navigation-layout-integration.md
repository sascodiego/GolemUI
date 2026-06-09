### PERSONA

Actúa como un Consultor Senior de Integración de Sistemas y UI en Fyne, con foco en sincronía de flujos asíncronos y desacoplamiento de componentes de interfaz.

### CONTEXTO Y ANCLAJE

- **Iniciativa/Proyecto:** Barra lateral modular de navegación en GolemUI.
- **Archivos de Referencia:**
  - `@/src/GolemUI/cmd/golemui/main.go`: Punto de entrada de la aplicación donde se inicializan las ventanas y layouts de Fyne.
  - `@/src/GolemUI/pkg/ui/compositor.go`: Contiene el callback global `ui.Navigate` y componentes del sistema.
- **Línea de Base:** Basándose estrictamente en la información técnica provista en los archivos referenciados:
  - El flujo en `main.go` reemplaza la ventana completa (`win.SetContent(newUI)`) al cambiar de pantalla.
  - Se requiere rediseñar el contenedor principal con un split layout (`HSplit`) para albergar la barra lateral fija y un contenedor derecho dinámico.

### TAREA (EL "QUÉ")

Refactorizar el contenedor de ventana principal en `main.go` para incorporar el panel lateral de navegación de forma persistente y sincronizar bidireccionalmente los eventos de navegación.

1. **Estructura de Ventana y HSplit:**
   - Crear un contenedor principal derecho (`mainContainer`) del tipo `*fyne.Container` con un Layout Max (`container.NewMax()`).
   - Instanciar el widget Tree de la barra lateral, envolverlo en un scroll vertical (`container.NewVScroll`) y asignarle un ancho fijo.
   - Definir un split layout horizontal (`container.NewHSplit`) asignando el Sidebar Scroll a la izquierda y el `mainContainer` a la derecha.
   - Reemplazar el contenido de la ventana principal (`win.SetContent(split)`).
2. **Navegación Asíncrona Parcial:**
   - Modificar la función global `ui.Navigate` para que al invocarse recupere y dibuje la vista correspondiente.
   - La actualización visual en `ui.Navigate` debe reemplazar únicamente los objetos dentro de `mainContainer` (`mainContainer.Objects = []fyne.CanvasObject{newUI}`) y ejecutar el refresco (`mainContainer.Refresh()`) de manera asíncrona dentro del hilo principal de la UI.
3. **Sincronización Bidireccional:**
   - Diseñar e implementar un mecanismo para que cuando ocurra una navegación iniciada por una acción externa (ej. botón "Volver al Listado" en la vista activa), el Sidebar Tree seleccione visualmente y expanda de manera automática el nodo correspondiente al nuevo `vista_id` activo.

### DIRECTRICES EXCLUYENTES POSITIVAS (LÍMITES DE ALCANCE)

- **Enfoque Principal:** Rediseñar la ventana principal de la aplicación, soportando el layout dividido y la sincronización bidireccional de navegación en el cliente de Go.
- **Fuera de Alcance:**
  - Limita el alcance exclusivamente a la integración de layout HSplit, la navegación asíncrona parcial y la sincronización de selección del sidebar.
  - Mantén intactas las configuraciones del archivo de configuración inicial y de conexión a bases de datos.
  - Restringe la manipulación de ventanas a la ventana principal de la aplicación.

### CRITERIOS DE ACEPTACIÓN (VALIDACIÓN BINARIA)

- **Métrica de Éxito:** Al ejecutar el cliente GolemUI, la ventana principal muestra un panel dividido. Al hacer click en una hoja del árbol lateral, la vista correspondiente se dibuja a la derecha mientras la barra lateral mantiene su selección y estado. Al accionar un botón de navegación externa en el panel derecho (ej. "Volver al Listado"), la pantalla derecha cambia a la vista de destino y el menú lateral resalta automáticamente el nodo asociado a dicho destino.
