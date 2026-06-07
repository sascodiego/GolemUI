## Estado actual del proyecto

### Lo que tenemos funcionando (verificado en código)

| Capa                        | Estado                                                  |
| --------------------------- | ------------------------------------------------------- |
| Capa 4 – Renderizador Fyne  | container , label , text_input , button , data_grid     |
| con headers ✅              |
| EventBus                    | text_input publica, data_grid suscribe y cancela        |
| contextos previos ✅        |
| Capa 1 – DB Pool            | Dos conexiones independientes ( CorePool , BusinessPool |
| ), interfaces testeables ✅ |
| Capa 2/3 – Scaffold         | Ausente — el mapeo tipo físico → componente lógico no   |
| existe                      |
| Bootstrap                   | El layout está hardcodeado en Go ( main.go líneas 61-   |
| 74)                         |

### El problema fundamental que todavía tenemos

El main.go arma el layout en código Go. Un NodeMeta construido a mano. Eso
contradice el propósito del motor: los layouts deben venir de la base de datos.

La tabla golemui.vistas_consulta ya existe en el schema con config_columnas y
config_filtros como JSONB. Pero nadie la lee todavía. La tabla golemui.mapeo_interfaz
también existe para los overrides de Capa 3. Tampoco se usa.
──────

## Qué debería ser la siguiente fase

Screen Loading desde la base de datos — que el bootstrap lea un layout real desde
golemui_core en lugar de construirlo hardcodeado.

Esto implica tres cosas concretas y acotadas:

1.  pkg/ui/screen_loader.go : una función que dado un vista_id consulte golemui.
    vistas_consulta , deserialice el JSONB de config_columnas y config_filtros y
    construya el árbol NodeMeta correspondiente.
2.  main.go : reemplazar el homeNode hardcodeado por una llamada al screen loader
    apuntando a una vista real en la DB core.
3.  docker/init-db/02_init_core.sql : insertar al menos una vista de muestra en
    golemui.vistas_consulta que defina la grilla de transacciones que ya tenemos datos en
    negocio_production .

## Lo que no se debe hacer y por que:

• Las features del EventBus y data_grid ya están listas, pero no tienen una salida
real a producción porque el layout es un mock Go.
• Conectar la DB core al compositor cierra el loop del motor: la primera vez que
arranques el binario y la pantalla venga de Postgres, GolemUI pasa de ser un
experimento a ser un motor real funcionando.
• Es incremental: no toca las capas ya testeadas, solo agrega la capa de lectura de
layout sobre lo que ya existe.

Lo que no haría todavía: el auto-scaffold de Capa 2 (mapeo tipo físico → componente).
Eso requiere introspección de schema y es el paso siguiente a tener pantallas desde DB
funcionando.
