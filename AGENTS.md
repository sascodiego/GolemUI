# GolemUI: Guía de Contexto e Instrucciones para Agentes de IA

Este documento sirve como guía de onboarding y manual de directrices críticas para los agentes de Inteligencia Artificial que colaboren en el desarrollo, refactorización o extensión del proyecto GolemUI.

---

## 1. Sinopsis y Propósito del Proyecto

GolemUI es un motor de renderizado dinámico y reactivo (Data-Driven UI Engine) escrito en Go. Su propósito es renderizar interfaces gráficas en tiempo de ejecución basándose en metadatos y layouts almacenados en una base de datos PostgreSQL, eliminando la necesidad de programar y desplegar interfaces de usuario hardcodeadas en el cliente.

---

## 2. Arquitectura de Persistencia Segregada

El ecosistema de base de datos se separa físicamente en dos componentes para evitar la contaminación de los dominios:

1.  **Base de Datos Core (`golemui_core`)**: Contiene el esquema de sistema `golemui` con las definiciones de componentes, estilos semánticos, vistas de introspección, layouts de pantallas y configuraciones de presets.
2.  **Base de Datos de Negocio (`negocio_production`)**: Contiene las tablas y Stored Procedures transaccionales puros de la aplicación del usuario.

El cliente en Go administra **dos conexiones de red independientes** hacia estas bases de datos en caliente.

---

## 3. El Flujo de Desacoplamiento de Cuatro Capas

Cualquier cambio de código, base de datos o API debe respetar de forma estricta el modelo de cuatro capas:

*   **Capa 1: Datos y Esquema Físico (Origen de Datos)**: Resuelto por la base de datos de negocio o por un plugin `.so` externo. Solo entrega la data de negocio y su firma de tipos de datos físicos (`string`, `integer`, `boolean`, `object` o `array` recursivos). El origen es ciego a la interfaz de usuario.
*   **Capa 2: Mapeo Lógico Core (Postgres local `golemui_core`)**: Traduce el tipo físico obtenido de la Capa 1 a un **Componente Lógico de GolemUI** (ej. `string` $\rightarrow$ `text_input`).
*   **Capa 3: Overrides del Core (Postgres local `golemui_core`)**: La tabla `golemui.mapeo_interfaz` permite al desarrollador sobrescribir el componente por defecto (ej. forzar que un campo `string` se dibuje como un `dropdown_select` de FK).
*   **Capa 4: Renderizador Fyne (Cliente Go)**: El binario de Go lee los componentes lógicos e instancia los widgets físicos del toolkit gráfico **Fyne**.

---

## 4. Desarrollo Local y Flujo Efímero (Clean Slate)

Para garantizar que los desarrolladores e IAs trabajen siempre bajo un modelo de datos funcional y consistente sin arrastrar datos corruptos ni requerir migraciones manuales, el entorno local de Postgres se define como **completamente efímero** (sin volúmenes persistentes en disco).

### Ciclo de Vida del Entorno:
1.  **Arranque (`docker-compose up -d`)**: Postgres detecta el almacenamiento vacío e inicializa las bases de datos de cero ejecutando recursivamente los scripts de `/docker/init-db/`.
2.  **Destrucción (`docker-compose down`)**: Detiene y destruye el contenedor de base de datos, eliminando por completo todo el estado y los datos modificados.

### Estructura de Inicialización (`/docker/init-db/`):
*   `01_create_databases.sh`: Crea el usuario de negocio y la base de datos `negocio_production`.
*   `02_init_core.sql`: Inicializa el esquema `golemui`, estilos y componentes del core en `golemui_core`.
*   `03_load_transactions.sh`: Crea la tabla `public.transacciones` en la base de datos de negocio `negocio_production` e ingesta en caliente los datos de prueba del archivo `/data/transactions.json` mediante funciones nativas de Postgres.

---

## 5. Ecosistema de Plugins de Datos (`.so`)

Para conectar orígenes de datos heterogéneos (MSSQL, APIs REST, gRPC), se desarrollan plugins dinámicos que implementan la interfaz `dbplugin.DataConnector`.

### Reglas Críticas para Plugins:
*   Deben retornar tipos de datos puros y esquemas físicos (`GetSchema`) sin mencionar primitivas de UI.
*   Se compilan de forma unificada con el core para evitar problemas de compatibilidad del runtime de Go:
    ```bash
    go build -buildmode=plugin -o ./plugins/mssql.so ./plugins/mssql/main.go
    ```
*   La carga es dinámica en caliente mediante `plugin.Open` y lookup del símbolo exportado `Connector`.

---

## 6. Convenciones de Programación del Cliente Go (Fyne)

*   **Composición Recursiva**: El compositor en Go procesa layouts y contenedores a través de la estructura recursiva de árbol `NodeMeta` en memoria.
*   **Desacoplamiento del DataGrid**: Toda instancia de `data_grid` (`*widget.Table` de Fyne) debe encapsular su propio modelo de datos local (`DataGridWidget`) para evitar el compartido inseguro de variables en memoria.
*   **Asincronismo y UI Thread**: Las llamadas de red (`FetchData`) deben ejecutarse en goroutines de fondo. Al finalizar, la actualización de datos y el refresco de la pantalla (`table.Refresh()`) deben despacharse al hilo principal de la UI usando la abstracción segura de hilo del runtime de Fyne.
*   **Línea Exclusiva de UI**: Solo se permite el uso de **Fyne** como toolkit visual del core. No incluir dependencias ni menciones a otros frameworks de consola (como Bubbletea) o web en la lógica del cliente.

---

## 7. Reglas de Conducta Críticas para Agentes de IA

1.  **Restricción de Nombres Legales (Crítico)**: Está estrictamente prohibido escribir o hacer referencia a la antigua denominación del motor (por razones de cumplimiento legal de patentes/marcas). Se debe utilizar de manera obligatoria y exclusiva el término **GolemUI** en todo el código, base de datos y documentación.
2.  **Filosofía Production-First y Evitación de Migraciones**: Todo código, esquema de base de datos o API debe implementarse desde el primer día bajo estándares listos para producción (*Production-First*). No se permiten simplificaciones, mocks o parches temporales. Hasta la entrega del primer MVP, las modificaciones de esquemas físicos se realizan de forma directa y destructiva en los scripts de `/docker/init-db/` (aprovechando el entorno efímero Clean Slate), evitando la creación o acumulación de archivos de migración intermedios.
3.  **No acoplar UI en Datos**: Nunca inyectar lógica de renderizado ni alias visuales dentro de los plugins de transporte o scripts de conversión de datos (respetar la separación estricta de la Capa 1 y 2).
4.  **Transaccionalidad en Postgres**: Aprovechar la atomicidad implícita de las funciones de Postgres. Cualquier error de negocio debe forzarse mediante `RAISE EXCEPTION` para gatillar el rollback automático de todas las operaciones previas acumuladas en el borrador de sesión.
5.  **Enlaces de Documentación**: Al referenciar código, tablas o archivos, generar enlaces clicables válidos utilizando la sintaxis de esquema `file:///`.
