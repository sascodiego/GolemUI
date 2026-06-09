a014-screen-lifecycle-cleanup-and-unsubscribe.md

### PERSONA

Arquitecto de Software Senior y Especialista en Gestión de Memoria en Go, con amplio conocimiento en el patrón EventBus y en la prevención de fugas de memoria (memory leaks) por retención de referencias en interfaces gráficas.

### CONTEXTO Y ANCLAJE

- **Iniciativa/Proyecto:** Motor de renderizado dinámico GolemUI.
- **Archivos de Referencia:**
  - `@pkg/eventbus/eventbus.go` (Suscripciones al EventBus e interfaz `EventBus`)
  - `@pkg/ui/compositor.go` (Instanciación de componentes e interacción con el `LocalEventBus`)
  - `@cmd/golemui/main.go` (Navegación e intercambio de vistas en `ui.Navigate`)
- **Línea de Base:** Basándose estrictamente en la información técnica provista en los archivos referenciados, las suscripciones a eventos de la pantalla previa (como el canal de submit) permanecen registradas en el `LocalEventBus` tras navegar a otra pantalla. Esto retiene en memoria los widgets de Fyne y sus controladores asociados, generando una fuga progresiva de memoria en cada navegación.

### TAREA (EL "QUÉ")

Diseñar la desuscripción automática del ciclo de vida de las pantallas:

1. **Modificación de la Firma de Suscripción:** Extender el método `Subscribe` en la interfaz `EventBus` y su implementación `InMemEventBus` para retornar dos valores: el identificador de suscripción (`string`) y una función de desuscripción de tipo `func()`.
2. **Retorno del Callback de Limpieza:** Implementar la función de desuscripción retornada para que invoque internamente a `Unsubscribe(channel, subID)` con los datos correspondientes de la suscripción creada.
3. **Registro de Suscripciones Activas:** Crear una variable de almacenamiento centralizado (por ejemplo, una porción de memoria o slice en el paquete `ui`, como `ActiveUnsubscribes []func()`) destinada a registrar las desuscripciones de la pantalla en pantalla.
4. **Acumulación durante el Compose:** Durante la composición de los widgets en `ui.Compose` (o subfunciones del compositor), registrar cada desuscripción obtenida al suscribirse al `LocalEventBus` dentro de la variable centralizada.
5. **Liberación en la Navegación:** Modificar la lógica de `ui.Navigate` para que, antes de comenzar el proceso de carga de la nueva pantalla, recorra y ejecute cada función registrada en la lista de desuscripciones activas de la pantalla saliente, y posteriormente limpie dicha lista.

### DIRECTRICES EXCLUYENTES POSITIVAS (LÍMITES DE ALCANCE)

- **Enfoque Principal:** Limita el alcance exclusivamente a liberar las suscripciones a eventos del `LocalEventBus` mediante funciones de desuscripción durante el cambio de pantalla en `ui.Navigate`.
- **Fuera de Alcance:**
  - Mantén intacta la implementación interna de los canales del bus de eventos y la estructura de sincronización (`sync.RWMutex`).
  - Restringe el ciclo de vida a la desuscripción de eventos del bus local en `ui.Navigate`.
  - Enfócate estrictamente en limpiar las suscripciones creadas en la composición visual de la pantalla.

### CRITERIOS DE ACEPTACIÓN (VALIDACIÓN BINARIA)

- **Métrica de Éxito:** La prueba unitaria en `@pkg/eventbus/eventbus_test.go` o un nuevo test de integración debe verificar que:
  1. La llamada a `Subscribe` retorne una función válida, y que la invocación de esta función reduzca a cero el número de suscriptores activos en el canal.
  2. Al simular una navegación mediante `ui.Navigate`, la lista de desuscripciones acumuladas sea ejecutada por completo y el número total de handlers registrados en el `LocalEventBus` para canales de submit previos se reduzca a cero.
