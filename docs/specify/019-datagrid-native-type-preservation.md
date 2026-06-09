### PERSONA

Desarrollador Senior de Software en Go y Arquitecto de Datos, experto en el mapeo de tipos físicos de bases de datos relacionales a estructuras lógicas y en la conservación de tipos primitivos en el frontend.

### CONTEXTO Y ANCLAJE

- **Iniciativa/Proyecto:** Motor de renderizado dinámico GolemUI.
- **Archivos de Referencia:**
  - `@pkg/ui/datasource.go` (Estructura `DataSet`)
  - `@pkg/dataaccess/sql_datasource.go` (Métodos `Fetch` y `FetchAll` de `SQLDataSource`)
  - `@pkg/ui/compositor.go` (Estructura `dataGridModel`, funciones `loadMasterBuffer`, `fetchGridDataAsync`, `filterMasterRows` y callback `OnSelected` de `widget.Table`)
  - `@pkg/dataaccess/format.go` (Función `FormatValue`)
- **Línea de Base:** Basándose estrictamente en la información técnica provista en los archivos referenciados, el modelo de transporte `DataSet` y el modelo interno `dataGridModel` almacenan las celdas como cadenas de texto (`[][]string`), degradando los tipos originales (enteros, booleanos, flotantes) mediante la conversión temprana en `FormatValue` durante la carga de base de datos en `SQLDataSource.Fetch`. Como consecuencia, al seleccionar una fila, el evento `publish_selection` transporta los valores formateados como cadenas de texto, perdiendo los tipos nativos requeridos para expresiones lógicas complejas en el cliente.

### TAREA (EL "QUÉ")

Preservar los tipos nativos en toda la cadena de transporte del grid, desde el origen de datos hasta la selección, delegando el formateo a string únicamente al momento de la renderización:

1. **Tipado Genérico en Transporte (`DataSet`):**
   - Modificar el campo `Rows` en la estructura `DataSet` (`pkg/ui/datasource.go`) para que sea de tipo `[][]any` en lugar de `[][]string`.
2. **Tipado Genérico en Almacenamiento Local (`dataGridModel`):**
   - Modificar los campos `rows` y `masterRows` de la estructura `dataGridModel` (`pkg/ui/compositor.go`) para que almacenen arreglos bidimensionales genéricos de tipo `[][]any`.
3. **Preservación de Tipos en DataAccess:**
   - Ajustar el método `Fetch` en `SQLDataSource` (`pkg/dataaccess/sql_datasource.go`) para poblar `DataSet.Rows` directamente con los valores nativos `[]any` leídos desde la base de datos (`rows.Values()`), omitiendo la llamada temprana a `FormatValue`.
4. **Formateo Visual Tardío (Renderizado):**
   - En el callback visual de actualización de celdas (`UpdateCell`) de `widget.Table` en `compositor.go`, invocar la función `dataaccess.FormatValue` sobre el valor de tipo `any` recuperado de `model.rows[id.Row][id.Col]` para convertirlo a string antes de asignarlo a la celda visual (`label.SetText`). Despachar estos seteos y refrescos de forma segura al hilo principal de la UI.
5. **Preservación de Tipos en Selección:**
   - En el callback `OnSelected` de `widget.Table` en `compositor.go`, construir el mapa `rowMap` utilizando los valores primitivos originales procedentes de `model.rows[id.Row]`, publicando tipos nativos (como `int64`, `float64`, `bool` o `string`) a través de `"publish_selection"`.
6. **Conversión Dinámica en Filtros:**
   - En la función `filterMasterRows` en `compositor.go`, realizar la conversión dinámica del valor de la celda (`row[col]`) mediante `dataaccess.FormatValue` para ejecutar la comparación de subcadenas sin alterar el tipo nativo en `masterRows`.

### DIRECTRICES EXCLUYENTES POSITIVAS (LÍMITES DE ALCANCE)

- **Enfoque Principal:** Limita el alcance exclusivamente a migrar las estructuras `DataSet.Rows`, `dataGridModel.rows` y `dataGridModel.masterRows` al tipo genérico `[][]any`, delegando la conversión a cadena de texto mediante `FormatValue` al dibujo visual y filtrados temporales.
- **Fuera de Alcance:**
  - Preserva intacta la firma y lógica de conversión interna de la función `dataaccess.FormatValue` para resolver tipos basados en `driver.Valuer`.
  - Limita el uso de tipos nativos en la selección a tipos primitivos básicos de Go de modo que el mapa de selección mantenga compatibilidad con serialización estándar.
  - Restringe la invocación de `FormatValue` en el compositor únicamente a las etapas visuales (dibujado de celdas y cabeceras) y comparaciones dinámicas del filtro de texto.
  - Asegura que todas las modificaciones de visualización de la tabla (`SetColumnWidth`, `Refresh`) ocurran bajo el hilo principal de la UI utilizando `fyne.Do`.

### CRITERIOS DE ACEPTACIÓN (VALIDACIÓN BINARIA)

- **Métrica de Éxito:** Las pruebas automatizadas en `@pkg/ui/compositor_test.go` y `@pkg/dataaccess/sql_datasource_test.go` deben validar que:
  1. La selección de una fila que contenga tipos enteros, booleanos y flotantes devuelva en el mapa publicado en `"publish_selection"` valores con sus tipos originales preservados (`int64`, `bool`, `float64`), en lugar de cadenas de texto.
  2. La representación en celdas de la tabla (las etiquetas visuales del `widget.Table`) muestre las cadenas formateadas de manera idéntica al comportamiento original del sistema.
