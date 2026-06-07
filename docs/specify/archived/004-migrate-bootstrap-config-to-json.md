### PERSONA

Adopta el rol de un **Consultor Senior de Arquitectura** con sólida experiencia en modularización, optimización de runtimes y diseño de sistemas Go de alto rendimiento.

### CONTEXTO Y ANCLAJE

- **Iniciativa/Proyecto:** GolemUI Core Engine / Desacoplamiento de Configuración de Arranque.
- **Archivos de Referencia:**
  - @src/GolemUI/pkg/lua/loader.go (Implementación del cargador de configuración actual)
  - @src/GolemUI/cmd/golemui/main.go (Punto de entrada y bootstrap de la aplicación)
  - @src/GolemUI/cmd/golemui/main_test.go (Pruebas unitarias y de integración de bootstrap)
  - @src/GolemUI/golemui_driver.lua (Archivo de configuración local actual en formato Lua)

- **Línea de Base:** Basándose estrictamente en la información técnica provista en los archivos referenciados, el arranque de GolemUI está acoplado al runtime de Lua. La función `LoadConfig` en `pkg/lua/loader.go` levanta una VM de Lua (`lua.NewState()`), evalúa el script `golemui_driver.lua` y extrae la tabla global `golemui_driver` para instanciar la estructura `BootstrapConfig`. Esto genera una inicialización pesada e innecesaria para leer una configuración estática de desarrollo local.

### TAREA (EL "QUÉ")

Migrar el mecanismo de configuración de arranque local de GolemUI para que utilice un archivo JSON estático (`golemui_driver.json`) en lugar de evaluar código Lua dinámico en tiempo de inicialización. 

Esto requiere:
1. Reemplazar `golemui_driver.lua` en la raíz del proyecto por un equivalente estructurado en JSON (`golemui_driver.json`).
2. Refactorizar el cargador en `pkg/lua/loader.go` para leer y deserializar la configuración utilizando el paquete estándar `encoding/json` de Go.
3. Asegurar que la VM de Lua (`gopher-lua`) no sea instanciada durante la ejecución de `LoadConfig`, eliminando el uso de `lua.NewState()` en el flujo de arranque del driver.
4. Mantener la biblioteca de scripting de Lua y su entorno intactos en el repositorio y dependencias (`go.mod`) para permitir ejecuciones de scripts dinámicos en runtime en futuras fases del sistema.
5. Actualizar el flag por defecto de CLI `-config` en `cmd/golemui/main.go` para que apunte a `golemui_driver.json`.
6. Refactorizar la suite de pruebas unitarias en `cmd/golemui/main_test.go` para que operen con archivos JSON de prueba y validen adecuadamente los escenarios de error (JSON inválido, archivos faltantes) y éxito.

### DIRECTRICES EXCLUYENTES POSITIVAS (LÍMITES DE ALCANCE)

- **Enfoque Principal:**
  - Diseñar el parseo de configuración utilizando exclusivamente `encoding/json` en Go.
  - Mantener la integridad de la estructura de datos `BootstrapConfig` y su firma pública para evitar romper la integración con `RunBootstrap`.
  - Asegurar la retrocompatibilidad en el comportamiento del CLI si no se proveen argumentos de entrada específicos.
- **Fuera de Alcance:**
  - Limita el alcance exclusivamente a la migración de la configuración estática de arranque del driver de desarrollo local.
  - Enfócate estrictamente en eliminar la VM de Lua del bootstrap, manteniendo la dependencia `gopher-lua` disponible en el proyecto para futuras necesidades de scripting dinámico en runtime.
  - Restringe cualquier modificación de base de datos o lógica de renderizado gráfico de Fyne.

### CRITERIOS DE ACEPTACIÓN (VALIDACIÓN BINARIA)

- **Métrica de Éxito:**
  1. Todas las pruebas en `cmd/golemui/main_test.go` deben pasar con éxito tras migrar las configuraciones temporales de prueba a JSON.
  2. La función `LoadConfig` debe completarse exitosamente sin invocar de forma directa o indirecta funciones de `github.com/yuin/gopher-lua`.
  3. El cliente Go de GolemUI debe iniciar correctamente cargando el archivo `golemui_driver.json` ubicado en la raíz del proyecto.
