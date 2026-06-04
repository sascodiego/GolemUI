# GolemUI: Especificación del Ecosistema de Plugins de Datos

Este documento define la arquitectura y especificación técnica para la extensión de GolemUI mediante plugins dinámicos. Aunque el esquema de sistema de UI (`golemui`) y la administración de los overrides visuales residen de forma inamovible en una base de datos centralizada de UI PostgreSQL (`golemui_core`), el motor permite la conexión de orígenes de datos de negocio heterogéneos (Microsoft SQL Server, APIs REST, servicios gRPC, etc.) utilizando Shared Objects compilados dinámicamente (`.so`).

---

## 1. El Rol del Plugin en el Modelo de Cuatro Capas

Para garantizar un diseño desacoplado (SOLID), GolemUI implementa una arquitectura de cuatro capas operativas. En esta arquitectura, **el conector de datos del plugin opera exclusivamente en la Capa 1 (Datos y Esquema Físico)**, aislando por completo al core de GolemUI y su base de datos de metadatos (`golemui_core`) de los orígenes externos.

```text
[ Capa 1: Datos y Esquema Físico ] (Resuelto por el Plugin .so en Go)
                 │
                 ▼ (Inferencia de Tipos de Datos Puros: string, integer, object, etc.)
[ Capa 2: Mapeo Lógico Core ] (Resuelto por golemui_core de Postgres)
                 │
                 ▼ (Mapeo por defecto de Tipo Físico -> Componente Lógico de GolemUI)
[ Capa 3: Overrides del Core ] (Configurado en golemui_core: golemui.mapeo_interfaz)
                 │
                 ▼ (Sobrescribe componentes lógicos de GolemUI, ej: dropdown para FKs)
[ Capa 4: Renderizador Fyne ] (Cliente GolemUI en Go usando Fyne)
```

### Directivas de Aislamiento del Plugin:
1.  **Agnóstico de Interfaz**: El conector no tiene dependencias visuales de Fyne ni de los componentes de GolemUI (`text_input`, `data_grid`). Solo entrega datos y metadatos de tipos físicos.
2.  **Responsabilidad de Introspección (Capa 1)**: El conector lee la estructura del origen remoto (ej. firmas de columnas, campos JSON) y expone de forma recursiva los tipos nativos puros.
3.  **Transporte Directo en Memoria**: El plugin transmite los bytes de respuesta JSON directamente al cliente GolemUI de Go en runtime, evitando contaminar la base de datos `golemui_core` con escrituras de tablas temporales.

---

## 2. La Interfaz Unificada del Plugin (`DataConnector`)

Cada conector externo debe compilarse como un paquete `main` independiente, implementar la interfaz `DataConnector` y exponerla a través del símbolo público de variable `Connector`.

```go
package dbplugin

import "context"

// ActionResult representa el resultado de la ejecución de una acción transaccional externa
type ActionResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"` // Mensaje de éxito o error propagado al modal de la UI
	Data    []byte `json:"data"`    // Payload JSON opcional retornado por el servicio externo
}

// DataConnector define el contrato que deben cumplir los plugins de datos (.so)
type DataConnector interface {
	// Init inicializa el conector con su configuración encriptada que se extrae de golemui_core
	Init(configJSON string) error

	// GetSchema retorna el esquema físico del recurso de forma recursiva (Capa 1)
	// - resource: Identificador del recurso (Ej: 'dbo.Clientes', '/v1/alertas', 'Patient/List')
	// Retorna un JSON Schema con la estructura y tipos de datos nativos puros
	GetSchema(ctx context.Context, resource string) ([]byte, error)

	// FetchData consulta un conjunto de registros (Lectura)
	// - resource: Identificador del recurso
	// - queryParams: Filtros aplicados por el usuario en la grilla visual de GolemUI
	// Retorna un slice JSON crudo que describe el dataset obtenido
	FetchData(ctx context.Context, resource string, queryParams map[string]interface{}) ([]byte, error)

	// ExecuteAction ejecuta una transacción o comando de negocio externo (Escritura)
	// - action: Identificador de la mutación (Ej: 'sp_pagar_cuenta', '/v1/facturar', 'Billing/Pay')
	// - payload: Los datos recolectados de los inputs del formulario en GolemUI
	ExecuteAction(ctx context.Context, action string, payload map[string]interface{}) (ActionResult, error)

	// Close libera pools de conexión, canales o descriptores de archivos del plugin
	Close() error
}
```

---

## 3. Inferencia de Esquema y Recursividad en Capa 1

Cuando el recurso mapeado por el plugin es dinámico (ej. un JSON complejo de una API REST o un objeto gRPC), el método `GetSchema` del plugin debe inferir de forma recursiva los tipos de datos físicos del payload:

### Ejemplo de Estructura de Data Schema
Si la API externa retorna un objeto con datos primitivos y colecciones anidadas (ej. un array de recetas médicas dentro de una consulta), el conector estructura la firma de tipos de datos en un JSON Schema recursivo:

```json
{
  "type": "object",
  "properties": {
    "id": { "type": "integer" },
    "nombre": { "type": "string" },
    "detalles_ingreso": {
      "type": "object",
      "properties": {
        "fecha": { "type": "string", "format": "date-time" },
        "activo": { "type": "boolean" }
      }
    },
    "recetas": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "medicamento": { "type": "string" },
          "dosis": { "type": "string" }
        }
      }
    }
  }
}
```

### Intercepción y Transformación Opcional (Lua Middleware)
Si el formato JSON de una API es incompatible, se puede registrar un script de Lua (almacenado en `golemui_core`) que actúa como middleware de Capa 1 en el cliente Go:
1. El cliente de Go carga el script de Lua desde `golemui_core.golemui.vistas_consulta.adaptador_lua`.
2. Le pasa los datos e inferencias crudas del plugin al script de Lua.
3. El script de Lua normaliza el árbol de datos y el esquema dinámicamente de forma recursiva y los retorna a Go.

---

## 4. Cargador Dinámico de Plugins en Go

El core de GolemUI en Go utiliza el paquete estándar `plugin` para abrir los Shared Objects y enlazar el conector en caliente:

```go
package loader

import (
	"errors"
	"plugin"
	"golemui/dbplugin" // Contrato de la interfaz
)

// CargarPlugin abre el Shared Object y mapea el conector de datos
func CargarPlugin(path string) (dbplugin.DataConnector, error) {
	p, err := plugin.Open(path)
	if err != nil {
		return nil, err
	}

	sym, err := p.Lookup("Connector")
	if err != nil {
		return nil, err
	}

	connector, ok := sym.(dbplugin.DataConnector)
	if !ok {
		return nil, errors.New("el simbolo 'Connector' no implementa la interfaz DataConnector")
	}

	return connector, nil
}
```

---

## 5. Compilación y Restricciones de Build

Para mitigar los acoplamientos y fallas de carga dinámica en tiempo de ejecución (gotchas de compatibilidad del runtime de Go), la compilación de los plugins debe coordinarse de forma estricta:

1.  **Misma versión de compilador**: Tanto el binario principal de GolemUI como los archivos `.so` de los plugins deben compilarse con la misma versión exacta del SDK de Go.
2.  **Mismos flags de build**: Se deben compartir los mismos flags y dependencias comunes (definidas en el `go.mod` del proyecto).
3.  **Límite de plataforma**: Al usar el modo `-buildmode=plugin`, la solución está restringida a plataformas tipo Unix (Linux, macOS, FreeBSD). No es compatible con Windows.

### Comando de compilación del Shared Object:
```bash
go build -buildmode=plugin -o ./plugins/mssql.so ./plugins/mssql/main.go
```
