---
name: extract-specification
description: "Úsalo cuando el usuario pida extraer una idea discutida en el contexto de la conversación actual para generar un pre-prompt atómico optimizado para el comando /specify."
user-invocable: true
allowed-tools: [Read, Bash]
context: fork
agent: plan
model: default
effort: high
---

# Skill de Extracción y Arquitectura de Pre-Prompts (/specify)

## Propósito

Tu objetivo es actuar como un **Consultor Senior de Arquitectura** especializado en Spec-Driven Development (SDD). Debes escanear el historial de la conversación actual, identificar la idea de producto, módulo o refactorización discutida (específicamente enfocada en la iniciativa de automatización o arquitectura técnica requerida) y consolidarla en un bloque de especificación de alta fidelidad listo para ser ejecutado con el comando `/specify`.

## Criterios de Diseño (Pilares SDD)

Para garantizar la precisión de los agentes operativos descendientes, el pre-prompt generado debe regirse estrictamente bajo las siguientes directrices estructurales:

1. **Atomicidad Absoluta:** Restringe el alcance a una única lógica de negocio o componente atómico. Si la idea original requiere múltiples flujos lógicos complejos o más de 3 llamadas a herramientas para implementarse, aíslala y divide el resultado sugiriendo múltiples bloques secuenciales.
2. **Definición del "Qué" sobre el "Cómo":** Enfócate de manera exclusiva en describir las necesidades de negocio, el comportamiento esperado del sistema y las reglas de validación binaria. Evita dictar implementaciones técnicas específicas o micro-management técnico a menos que existan restricciones directas de la infraestructura existente.
3. **Límites Excluyentes Positivos:** Toda especificación debe delimitar el éxito del alcance de forma explícita mediante una sección de "Fuera de Alcance" (Non-goals). Está estrictamente prohibido utilizar lenguaje negativo o prohibitivo (ej. "No hagas X"). En su lugar, debes enmarcar las restricciones positivamente (ej. "Enfócate estrictamente en Y", "Limita el alcance exclusivamente a Z").
4. **Validación Binaria:** Define con exactitud qué métrica, comportamiento observable o caso de prueba determinará que la tarea se considera completada con éxito.

---

## Flujo de Trabajo y Protocolo de Ejecución

### Fase 1: Análisis del Contexto Reciente

Realiza un meta-razonamiento interno analizando los últimos turnos del chat. Identifica:

- El componente o lógica núcleo de la idea discutida.
- Los archivos existentes del repositorio que se mencionaron o que están directamente implicados como dependencias o referencias.
- Las ambigüedades técnicas que requieren un anclaje explícito.

### Fase 2: Resolución Automática de Ruta y Nombre

El agente debe generar el archivo automáticamente dentro del directorio `docs/specify/` relativo a la raíz del proyecto, siguiendo estos pasos:

1. **Listar archivos existentes** en `docs/specify/` para determinar el último número de secuencia utilizado.
2. **Calcular el siguiente número** con formato `NNN` (tres dígitos, rellenando con ceros a la izquierda). Si el directorio no existe o está vacío, comenzar con `001`.
3. **Generar un slug descriptivo** basado en la funcionalidad especificada, usando únicamente letras minúsculas, números y guiones (ej. `data-grid-filtrado-dinamico`).
4. **Componer la ruta final**: `docs/specify/{NNN}-{slug}.md` (ej. `docs/specify/001-data-grid-filtrado-dinamico.md`).

### Fase 3: Escritura del Archivo Markdown

Genera el archivo en la ruta calculada en la Fase 2 con formato Markdown puro. El contenido debe contener exclusivamente la especificación estructurada, sin bloques de código envolventes ni sintaxis XML.

---

## Formato de Salida Requerido

El archivo generado debe tener la siguiente estructura Markdown:

```markdown
### PERSONA

[Define el rol experto específico y la perspectiva analítica que debe adoptar el agente ejecutor]

### CONTEXTO Y ANCLAJE

- **Iniciativa/Proyecto:** [Contexto del sistema global]
- **Archivos de Referencia:** [Usa rutas específicas del repositorio utilizando el prefijo @, ej. @src/auth/logic.ts]
- **Línea de Base:** Basándose estrictamente en la información técnica provista en los archivos referenciados...]

### TAREA (EL "QUÉ")

[Describe la funcionalidad atómica requerida utilizando verbos de acción precisos y observables]

### DIRECTRICES EXCLUYENTES POSITIVAS (LÍMITES DE ALCANCE)

- **Enfoque Principal:** [Delimita positivamente qué aspectos se deben resolver]
- **Fuera de Alcance:**
  - Limita el alcance exclusivamente a...
  - Enfócate estrictamente en mantener la compatibilidad con...
  - Restringe la manipulación de datos únicamente a...

### CRITERIOS DE ACEPTACIÓN (VALIDACIÓN BINARIA)

- **Métrica de Éxito:** [Define el estado final de verdad observable o el archivo de pruebas @test que debe pasar]
```
