# Tasks: nav-menu-schema

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~25 |
| 400-line budget risk | Low |
| Chained PRs recommended | No |
| Delivery strategy | single PR |

---

## Implementation Tasks

### Task 1 — Insert DDL + DML block into 02_init_core.sql

- [ ] Replace the anchor region between the third `vistas_consulta` INSERT and `-- Vistas personalizadas` comment with the full menu_navegacion block (DDL + root seed + child seed).

**Old text:**
```sql
ON CONFLICT (id) DO NOTHING;


-- Vistas personalizadas guardadas por los usuarios
```

**New text:**
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

**Estimated lines:** ~25 added.

---

### Task 2 — End-to-end verification

- [ ] `docker-compose down -v` + `docker-compose up -d` exit code 0
- [ ] `SELECT count(*) FROM golemui.menu_navegacion` → 4
- [ ] Root node check: `padre_id IS NULL` → 1 row
- [ ] Orphan vista_id check → 0 rows
- [ ] Orphan padre_id check → 0 rows
- [ ] Existing tables untouched: componentes=10, vistas_consulta=3

---

## Summary

| Task | Action | Est. Lines |
|------|--------|------------|
| 1 | DDL + DML insert | ~25 |
| 2 | Verification | 0 |
| **Total** | | **~25** |
