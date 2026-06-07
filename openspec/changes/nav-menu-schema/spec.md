# Nav Menu Schema Specification

## Purpose

Define the database schema and seed data for a hierarchical navigation menu in GolemUI Core, enabling the Go/Fyne client to render a data-driven sidebar or navigation panel without hardcoded screen lists.

## Requirements

### Requirement: menu_navegacion Table DDL

The system MUST create the table `golemui.menu_navegacion` in the `golemui_core` database with the exact DDL:

```sql
CREATE TABLE IF NOT EXISTS golemui.menu_navegacion (
    id VARCHAR(100) PRIMARY KEY,
    padre_id VARCHAR(100) REFERENCES golemui.menu_navegacion(id) ON DELETE CASCADE,
    titulo VARCHAR(150) NOT NULL,
    vista_id VARCHAR(100) REFERENCES golemui.vistas_consulta(id) ON DELETE SET NULL,
    orden INTEGER DEFAULT 0 NOT NULL
);
```

### Requirement: Seed Data — Root Nodes

```sql
INSERT INTO golemui.menu_navegacion (id, padre_id, titulo, vista_id, orden)
VALUES ('nav_principal', NULL, 'Menú Principal', NULL, 0)
ON CONFLICT (id) DO NOTHING;
```

### Requirement: Seed Data — Child Nodes

```sql
INSERT INTO golemui.menu_navegacion (id, padre_id, titulo, vista_id, orden) VALUES
('nav_home', 'nav_principal', 'Inicio', 'home', 1),
('nav_transacciones', 'nav_principal', 'Transacciones', 'transacciones_list', 2),
('nav_query_runner', 'nav_principal', 'Consola SQL', 'query_runner', 3)
ON CONFLICT (id) DO NOTHING;
```

### Requirement: Insertion Point

After the third `INSERT INTO golemui.vistas_consulta` (query_runner), before `CREATE TABLE IF NOT EXISTS golemui.vistas_guardadas`.

### Requirement: No Modification to Existing Tables

The addition MUST NOT alter, rename, or drop any existing table, column, constraint, index, or seed row.

## Acceptance Criteria

| # | Criterion | Verification |
|---|---|---|
| AC-1 | `docker-compose up -d` completes with exit code 0 | Run command on clean environment |
| AC-2 | `SELECT count(*) FROM golemui.menu_navegacion` returns `4` | SQL query |
| AC-3 | Root node `nav_principal` exists with `padre_id IS NULL` | SQL query returns exactly 1 row |
| AC-4 | Each child node references a valid `vista_id` in `vistas_consulta` | LEFT JOIN query returns 0 orphan rows |
| AC-5 | Each child node references a valid `padre_id` in `menu_navegacion` | LEFT JOIN query returns 0 orphan rows |
| AC-6 | No existing table, column, or row is altered or removed | Schema dump comparison |
| AC-7 | Re-running init script succeeds without error | Execute SQL a second time |

## Constraints

1. Split INSERTs required (roots before children) to avoid self-FK violations
2. `vista_id = NULL` for structural nodes; non-NULL for leaf nodes
3. Layer separation: `golemui_core` only, not `negocio_production`
4. Clean Slate only: no migration scripts
5. `ON DELETE CASCADE` on `padre_id` removes subtrees transitively
6. `ON DELETE SET NULL` on `vista_id` preserves menu structure if view is removed
