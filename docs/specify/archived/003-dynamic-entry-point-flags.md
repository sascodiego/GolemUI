### PERSONA

Adopta el rol de un **Ingeniero de Plataforma y CLI Senior** con foco en la usabilidad y configuración robusta de binarios compilados de Go.

### CONTEXTO Y ANCLAJE

- **Iniciativa/Proyecto:** GolemUI Core Engine
- **Archivos de Referencia:**
  - @cmd/golemui/main.go (Punto de entrada actual)
  - @golemui_driver.lua (Archivo de configuración de prueba)

- **Línea de Base:** Basándose estrictamente en la información técnica provista en los archivos referenciados, el binario de GolemUI inicia leyendo siempre de forma estática la configuración `"golemui_driver.lua"` y ejecutando la vista indicada en `EntryPointViewID` (o `"home"` si está vacía). No existe capacidad en el binario para alternar la vista o el archivo de configuración en tiempo de ejecución mediante argumentos de línea de comandos.

### TAREA (EL "QUÉ")

Implementar un mecanismo de flags de línea de comandos en el binario principal de GolemUI para configurar de forma dinámica:

1. La ruta del archivo de configuración Lua (`-config`, por defecto `"golemui_driver.lua"`).
2. El ID de la vista de entrada inicial (`-view`, que pisa/sobrescribe la propiedad `EntryPointViewID` leída de la configuración en caso de estar presente).

### DIRECTRICES EXCLUYENTES POSITIVAS (LÍMITES DE ALCANCE)

- **Enfoque Principal:**
  - Analizar los flags de línea de comandos utilizando el paquete estándar `flag` de Go antes de invocar `RunBootstrap`.
  - Asegurar la propagación correcta de estos parámetros a `RunBootstrap`.
  - Mantener los valores por defecto históricos para preservar la retrocompatibilidad absoluta sin alterar el comportamiento por defecto si no se especifican los flags.
- **Fuera de Alcance:**
  - Limita el alcance exclusivamente al análisis e inicialización de flags de entrada en `cmd/golemui/main.go`.
  - Enfócate estrictamente en mantener la compatibilidad con el resto de la interfaz pública de `RunBootstrap` y la estructura `BootstrapConfig`.
  - Restringe la manipulación de variables de entorno a la lógica actual de localización ya existente en `sanitizeLocale`.

### CRITERIOS DE ACEPTACIÓN (VALIDACIÓN BINARIA)

- **Métrica de Éxito:** Las pruebas de integración en `cmd/golemui/main_test.go` deben validar que:
  1. Si se define el flag `-config` con un archivo inexistente, el programa retorna un error de archivo no encontrado o de carga de configuración.
  2. Si se define el flag `-view`, el bootstrap inicializa con ese ID de vista, ignorando lo que indique el archivo Lua.
  3. Si no se proveen argumentos, se inicia correctamente cargando `"golemui_driver.lua"` y su vista configurada.
