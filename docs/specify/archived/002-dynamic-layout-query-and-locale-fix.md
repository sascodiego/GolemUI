### PERSONA

Adopta el rol de un Ingeniero de Sistemas Principal experto en localización (l10n), controladores gráficos GLFW/Fyne y desacoplamiento absoluto de capas de datos (Data-Driven UI).

### CONTEXTO Y ANCLAJE

- **Iniciativa/Proyecto:** GolemUI Client
- **Archivos de Referencia:**
  - @cmd/golemui/main.go
  - @pkg/lua/loader.go
  - @pkg/ui/screen_loader.go
  - @golemui_driver.lua
- **Línea de Base:** Basándose estrictamente en la información técnica de los archivos referenciados, el cargador de vistas de GolemUI tiene hardcodeada la query SQL física de carga. Además, las ejecuciones de Fyne bajo el locale de sistema genérico `C` generan un error de parseo de idioma (`Fyne error: Error parsing user locale C`), lo que provoca fallas de compatibilidad en la traducción de caracteres del teclado físico (ej. teclado en español con app funcionando bajo un locale mal estructurado).

### TAREA (EL "QUÉ")

1. **Configurar Consulta de Layout en Lua (Desacoplamiento):**
   - Incorporar en el archivo de configuración Lua `@golemui_driver.lua` una nueva propiedad `LayoutQuery` que contenga el query parametrizado para buscar la definición de pantallas en la base core.
   - Modificar la estructura `@pkg/lua/loader.go` para parsear y transferir la propiedad `LayoutQuery` a la configuración de arranque `BootstrapConfig`.
   - Modificar `@pkg/ui/screen_loader.go` para recibir y ejecutar dinámicamente la query inyectada, eliminando el string SQL hardcodeado y manteniéndolo únicamente como un fallback defensivo en caso de valor nulo.

2. **Resolución de Locale y Mapeo de Teclado:**
   - Implementar en la función inicial de arranque en `@cmd/golemui/main.go` un mecanismo automático que inspeccione las variables de entorno `LANG` y `LC_ALL` del sistema operativo.
   - Si se detecta que las variables están ausentes o configuradas con la opción por defecto `C`, forzar programáticamente su valor a un locale UTF-8 válido (como `en_US.UTF-8`) utilizando `os.Setenv`. Esto garantiza que Fyne pueda parsear un tag BCP-47 válido al arrancar el cliente y que el mapeo de caracteres del teclado del dispositivo coincida correctamente a través de GLFW.

### DIRECTRICES EXCLUYENTES POSITIVAS (LÍMITES DE ALCANCE)

- **Enfoque Principal:** 
  - Limita los cambios exclusivamente a la extracción de la query de layouts al archivo Lua y al saneamiento de las variables de entorno de localización en el arranque.
- **Fuera de Alcance:**
  - Evita modificar los componentes compositores de widgets visuales en `pkg/ui/compositor.go`.
  - Enfócate estrictamente en mantener la compatibilidad con el fallback de base de datos actual para que las pruebas de integración existentes sigan pasando limpiamente.
  - Restringe la manipulación del entorno de ejecución de idioma únicamente a las variables `LANG` y `LC_ALL` durante el bootstrap temprano.

### CRITERIOS DE ACEPTACIÓN (VALIDACIÓN BINARIA)

- **Métrica de Éxito:** 
  - El proyecto completo debe compilar sin advertencias y todos los tests unitarios (`go test ./...`) deben pasar exitosamente.
  - Al arrancar la aplicación (`./golemui_bin`), la consola no debe emitir el error `Fyne error: Error parsing user locale C`.
  - La query de recuperación de layouts se debe cargar desde la propiedad `LayoutQuery` del archivo Lua `@golemui_driver.lua`.
