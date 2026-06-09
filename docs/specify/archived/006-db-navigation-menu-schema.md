### PERSONA

Actúa como un Consultor Senior de Base de Datos y Arquitectura de Persistencia de GolemUI, especializado en esquemas relacionales eficientes y persistencia desacoplada.

### CONTEXTO Y ANCLAJE

- **Iniciativa/Proyecto:** Barra lateral modular de navegación en GolemUI.
- **Archivos de Referencia:**
  - `@/src/GolemUI/docker/init-db/02_init_core.sql`: Archivo central de inicialización de la base de datos core de GolemUI (`golemui_core`) que contiene esquemas, estilos, componentes y layouts iniciales.
- **Línea de Base:** Basándose estrictamente en la información técnica provista en los archivos referenciados:
  - El script `02_init_core.sql` inicializa las tablas del esquema `golemui` pero carece de un modelo relacional y de datos semilla para soportar menús jerárquicos.
  - La navegación requiere una tabla estructurada para almacenar las jerarquías de menús asociadas a vistas de consulta existentes.

### TAREA (EL "QUÉ")

Definir e implementar el esquema físico de base de datos relacional para soportar menús de navegación jerárquicos y poblarlo con datos de prueba iniciales.

1. **Creación de Tabla:**
   Agregar en `02_init_core.sql` la definición de la tabla `golemui.menu_navegacion`:
   ```sql
   CREATE TABLE IF NOT EXISTS golemui.menu_navegacion (
       id VARCHAR(100) PRIMARY KEY,
       padre_id VARCHAR(100) REFERENCES golemui.menu_navegacion(id) ON DELETE CASCADE,
       titulo VARCHAR(150) NOT NULL,
       vista_id VARCHAR(100) REFERENCES golemui.vistas_consulta(id) ON DELETE SET NULL,
       orden INTEGER DEFAULT 0 NOT NULL
   );
   ```
2. **Inserción de Semillas:**
   Ingresar registros iniciales en la tabla `golemui.menu_navegacion` que representen una estructura jerárquica con al menos un nivel de anidamiento, vinculando las vistas `home`, `transacciones_list`, y `query_runner` a sus respectivos nodos hoja.

### DIRECTRICES EXCLUYENTES POSITIVAS (LÍMITES DE ALCANCE)

- **Enfoque Principal:** Diseñar la estructura SQL del menú jerárquico y poblar las tablas core en la fase de inicialización del docker local.
- **Fuera de Alcance:**
  - Limita el alcance exclusivamente a las definiciones de esquemas SQL e inserción de datos semilla en `02_init_core.sql`.
  - Enfócate estrictamente en mantener intactas las tablas preexistentes del esquema `golemui` tales como `componentes`, `estilos`, `mapeo_interfaz`, `sesion_borrador`, y `vistas_consulta`.
  - Restringe la manipulación del esquema físico únicamente al archivo `02_init_core.sql`.

### CRITERIOS DE ACEPTACIÓN (VALIDACIÓN BINARIA)

- **Métrica de Éxito:** Al inicializar el contenedor Docker (`docker-compose down && docker-compose up -d`), la base de datos `golemui_core` debe contener la tabla `golemui.menu_navegacion` con el esquema definido de forma exacta, y la consulta de todos sus registros debe retornar las filas semilla configuradas sin errores de integridad referencial.
