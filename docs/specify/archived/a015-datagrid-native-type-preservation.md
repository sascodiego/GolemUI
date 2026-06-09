a015-datagrid-native-type-preservation.md

### PERSONA

Desarrollador Senior de Software en Go y Arquitecto de Datos, experto en el mapeo de tipos físicos de bases de datos relacionales a estructuras lógicas y en la conservación de tipos primitivos en el frontend.

### CONTEXTO Y ANCLAJE

- **Iniciativa/Proyecto:** Motor de renderizado GolemUI.
- **Archivos de Referencia:**
  - `@pkg/ui/compositor.go` (Estructura `dataGridModel`, funciones `loadMasterBuffer`, `fetchGridDataAsync`, `filterMasterRows` y callback `OnSelected` de `widget.Table`)
- **Línea de Base:** Basándose estrictamente en la información técnica provista en los archivos referenciados, el modelo `dataGridModel` almacena los datos de las filas como cadenas de texto (`[][]string`), degradando los tipos originales (enteros, booleanos, flotantes) mediante la conversión temprana en `formatValue` durante la carga de base de datos. Como consecuencia, al seleccionar una fila, el evento `publish_selection` transporta los valores formateados como cadenas de texto, perdiendo los tipos nativos requeridos para expresiones lógicas complejas en el cliente.

### TAREA (EL "QUÉ")

Preservar los tipos nativos en el modelo de datos del grid:

1. **Tipado Genérico de Almacenamiento:** Modificar los campos `rows` y `masterRows` de la estructura `dataGridModel` para que almacenen arreglos bidimensionales genéricos de tipo `[][]any`.
2. **Inyección Directa de Datos de la BD:** En las funciones `loadMasterBuffer` y `fetchGridDataAsync`, almacenar directamente los valores leídos (`vals`) obtenidos de la fila de la base de datos (`rows.Values()`) dentro de la estructura `dataGridModel` en su formato original.
3. **Formateo Visual Tardío:** Mover la llamada a la función `formatValue` para que sea invocada exclusivamente dentro del callback visual de actualización de celdas (`UpdateCell` y cabeceras si corresponde) al momento de dibujar los datos en la pantalla.
4. **Preservación de Tipos en Selección:** En el callback `OnSelected` del `widget.Table`, estructurar el mapa `rowMap` utilizando los valores originales de tipo `any` procedentes de `model.rows[id.Row]`, enviando los tipos primitivos en su estado nativo a través de `"publish_selection"`.
5. **Conversión Dinámica en Filtro:** Ajustar el método `filterMasterRows` para realizar la conversión a cadena de texto de forma dinámica y realizar las comparaciones de texto ignorando mayúsculas/minúsculas sin alterar el almacenamiento de tipo `any`.

### DIRECTRICES EXCLUYENTES POSITIVAS (LÍMITES DE ALCANCE)

- **Enfoque Principal:** Limita el alcance exclusivamente a modificar los campos `rows` y `masterRows` a `[][]any` para resguardar los tipos originales hasta el dibujo visual y la publicación del mapa de selección.
- **Fuera de Alcance:**
  - Mantén intacta la firma del método `formatValue` y su lógica de conversión de tipos mediante `driver.Valuer`.
  - Restringe la manipulación de datos del grid a las funciones internas del compositor en `@pkg/ui/compositor.go`.
  - Enfócate estrictamente en conservar la firma del canal de publicación `"publish_selection"` como `map[string]any`.

### CRITERIOS DE ACEPTACIÓN (VALIDACIÓN BINARIA)

- **Métrica de Éxito:** Las pruebas en `@pkg/ui/compositor_test.go` deben validar que:
  1. La selección de una fila que contenga tipos enteros, booleanos y flotantes devuelva en `"publish_selection"` un mapa con sus tipos nativos correspondientes (por ejemplo, `int64`, `bool`, `float64`), en lugar de cadenas de texto.
  2. La visualización de la tabla mantenga el formateo visual a cadena de texto idéntico al comportamiento original.
