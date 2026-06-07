# Design: nav-menu-schema

## 1. Insertion Point

**File:** `docker/init-db/02_init_core.sql`

**Anchor:** After the third `vistas_consulta` INSERT (query_runner), before `-- Vistas personalizadas guardadas por los usuarios`.

**Exact old text to locate:**

```text
ON CONFLICT (id) DO NOTHING;


-- Vistas personalizadas guardadas por los usuarios
```

**Replaced by:**

```sql
ON CONFLICT (id) DO NOTHING;

-- =====================================================================
-- Menú de Navegación Jerárquico
-- =====================================================================

-- Tabla de menú de navegación (árbol jerárquico auto-referenciado)
CREATE TABLE IF NOT EXISTS golemui.menu_navegacion (
    id VARCHAR(100) PRIMARY KEY,
    padre_id VARCHAR(100) REFERENCES golemui.menu_navegacion(id) ON DELETE CASCADE,
    titulo VARCHAR(150) NOT NULL,
    vista_id VARCHAR(100) REFERENCES golemui.vistas_consulta(id) ON DELETE SET NULL,
    orden INTEGER DEFAULT 0 NOT NULL
);

-- Nodo raíz del menú
INSERT INTO golemui.menu_navegacion (id, padre_id, titulo, vista_id, orden)
VALUES ('nav_principal', NULL, 'Menú Principal', NULL, 0)
ON CONFLICT (id) DO NOTHING;

-- Nodos hijos del menú principal
INSERT INTO golemui.menu_navegacion (id, padre_id, titulo, vista_id, orden) VALUES
('nav_home', 'nav_principal', 'Inicio', 'home', 1),
('nav_transacciones', 'nav_principal', 'Transacciones', 'transacciones_list', 2),
('nav_query_runner', 'nav_principal', 'Consola SQL', 'query_runner', 3)
ON CONFLICT (id) DO NOTHING;

-- Vistas personalizadas guardadas por los usuarios
```

## 2. Dependency Graph

```
golemui.vistas_consulta (seeded)
  │
  │  (FK: menu_navegacion.vista_id → vistas_consulta.id)
  ▼
golemui.menu_navegacion (NEW)
  │  (self-FK: padre_id → id)
  │     ├── Root: nav_principal (padre_id=NULL)
  │     └── Children: nav_home, nav_transacciones, nav_query_runner
  ▼
golemui.vistas_guardadas (unchanged)
```

## 3. Rollback Strategy

- Remove the inserted SQL block from `02_init_core.sql`
- `docker-compose down -v && docker-compose up -d`
- No migration scripts needed (ephemeral env)

## 4. Verification Queries

```sql
SELECT count(*) FROM golemui.menu_navegacion;  -- Expected: 4
SELECT * FROM golemui.menu_navegacion WHERE padre_id IS NULL;  -- Expected: 1 row
-- Orphan checks via LEFT JOINs
```
