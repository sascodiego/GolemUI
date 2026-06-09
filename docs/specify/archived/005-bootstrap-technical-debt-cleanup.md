### PERSONA

Adopta el rol de un **Ingeniero de Plataforma Senior** con foco en legibilidad, cohesión y limpieza arquitectónica de bases de código escritas en Go.

### CONTEXTO Y ANCLAJE

- **Iniciativa/Proyecto:** GolemUI Core Engine / Limpieza de Deuda Técnica del Bootstrap.
- **Archivos de Referencia:**
  - @[loader.go](file:///src/GolemUI/pkg/lua/loader.go) (Definición actual de la estructura de configuración y cargador Viper)
  - @[loader_test.go](file:///src/GolemUI/pkg/lua/loader_test.go) (Suite de pruebas del cargador)
  - @[main.go](file:///src/GolemUI/cmd/golemui/main.go) (Punto de entrada de la aplicación)
  - @[main_test.go](file:///src/GolemUI/cmd/golemui/main_test.go) (Suite de pruebas de bootstrap)
- **Línea de Base:** Basándose estrictamente en la información técnica provista en los archivos referenciados, el paquete de configuración local de arranque heredó el nombre de módulo `pkg/lua` a pesar de haber sido migrado a Viper y YAML, lo cual genera confusión cognitiva. Asimismo, el struct `BootstrapConfig` retiene la propiedad en desuso `EntryPointQuery` (código muerto).

### TAREA (EL "QUÉ")

Ejecutar la limpieza y refactorización estructural de la configuración de arranque de GolemUI para resolver la deuda técnica acumulada de nomenclatura y código muerto. 

Esto requiere:
1. Renombrar el directorio `pkg/lua` a `pkg/config` (y el nombre del paquete Go dentro de sus archivos a `config`).
2. Actualizar todas las rutas de importación de `GolemUI/pkg/lua` a `GolemUI/pkg/config` en los archivos `cmd/golemui/main.go` y `cmd/golemui/main_test.go`.
3. Eliminar la propiedad `EntryPointQuery` de la estructura `BootstrapConfig` en `loader.go` y remover sus respectivas etiquetas `mapstructure`.
4. Limpiar los archivos de prueba en `loader_test.go` para eliminar cualquier aserción o inicialización de la propiedad en desuso `EntryPointQuery`.

### DIRECTRICES EXCLUYENTES POSITIVAS (LÍMITES DE ALCANCE)

- **Enfoque Principal:**
  - Enfócate estrictamente en renombrar el paquete, actualizar los imports del proyecto y remover la propiedad de código muerto.
  - Asegurar que todas las firmas públicas de interfaces del nuevo paquete `config` queden alineadas con los consumidores en `main.go`.
- **Fuera de Alcance:**
  - Limita el alcance exclusivamente a la limpieza del módulo de configuración estática del cliente de desarrollo local.
  - Mantén sin alteraciones el validador de conexiones `validateConexion` (no agregues validaciones de contraseñas obligatorias).
  - Restringe la manipulación del archivo `golemui_driver.yaml` únicamente a la remoción del campo `entry_point_query` (las credenciales de prueba deben permanecer intactas).
  - Mantén intactas las dependencias de `gopher-lua` en el archivo `go.mod`.

### CRITERIOS DE ACEPTACIÓN (VALIDACIÓN BINARIA)

- **Métrica de Éxito:**
  1. No debe quedar ninguna referencia literal a la ruta o paquete `pkg/lua` dentro de la base de código Go (`grep -rn "pkg/lua" .` no debe retornar coincidencias en archivos fuente).
  2. La suite de pruebas de Go (`go test ./...`) debe completarse con éxito (100% verde) tras la refactorización.
  3. El binario debe compilar limpiamente mediante `go build -o golemui_bin cmd/golemui/main.go`.
