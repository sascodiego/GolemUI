# Proposal: nav-menu-schema

> **Phase:** Proposal
> **Date:** 2026-06-07
> **Scope:** `docker/init-db/02_init_core.sql` only
> **Status:** Pending review

---

## 1. Problem Statement

GolemUI's core schema (`golemui_core`) currently defines screens through `golemui.vistas_consulta`, but there is **no mechanism to organize those screens into a navigable menu structure**. The Go/Fyne client has no persistent, database-driven way to know which views exist, how they are grouped, or what order they should appear in a sidebar or navigation panel.

Without this, any navigation UI must be hardcoded in the client — violating the Data-Driven UI principle that GolemUI is built on.

---

## 2. Proposed Solution

### 2.1 New Table: `golemui.menu_navegacion`

Add a single self-referencing hierarchical table to the `golemui` schema:

```sql
CREATE TABLE IF NOT EXISTS golemui.menu_navegacion (
    id VARCHAR(100) PRIMARY KEY,
    padre_id VARCHAR(100) REFERENCES golemui.menu_navegacion(id) ON DELETE CASCADE,
    titulo VARCHAR(150) NOT NULL,
    vista_id VARCHAR(100) REFERENCES golemui.vistas_consulta(id) ON DELETE SET NULL,
    orden INTEGER DEFAULT 0 NOT NULL
);
```

**Column semantics:**

| Column | Purpose |
|---|---|
| `id` | Stable identifier for the menu node |
| `padre_id` | Self-referencing FK; `NULL` = root node; `ON DELETE CASCADE` removes subtree |
| `titulo` | Human-readable label for the menu item |
| `vista_id` | FK to `vistas_consulta`; `NULL` for structural (non-leaf) nodes; `ON DELETE SET NULL` |
| `orden` | Sort order among siblings (ascending) |

### 2.2 Seed Data

Split into two `INSERT` statements — roots first, then children:

**Roots:**

| id | padre_id | titulo | vista_id | orden |
|---|---|---|---|---|
| `nav_principal` | NULL | Menú Principal | NULL | 0 |

**Children:**

| id | padre_id | titulo | vista_id | orden |
|---|---|---|---|---|
| `nav_home` | `nav_principal` | Inicio | `home` | 1 |
| `nav_transacciones` | `nav_principal` | Transacciones | `transacciones_list` | 2 |
| `nav_query_runner` | `nav_principal` | Consola SQL | `query_runner` | 3 |

All `INSERT`s use `ON CONFLICT (id) DO NOTHING`.

### 2.3 Insertion Point

After the third `INSERT INTO golemui.vistas_consulta` (query_runner), before `CREATE TABLE IF NOT EXISTS golemui.vistas_guardadas`.

---

## 3. Impact Analysis

| File | Change Type | Lines (est.) |
|---|---|---|
| `docker/init-db/02_init_core.sql` | Addition only | ~20 lines |

**No changes to any existing table or seed row.** Purely additive.

---

## 4. Rollback Plan

Remove the added DDL + seed block from `02_init_core.sql`, then `docker-compose down && up -d`. No migrations needed.

---

## 5. Success Criteria

- [ ] `docker-compose up -d` completes without error
- [ ] `SELECT count(*) FROM golemui.menu_navegacion` returns `4`
- [ ] All FK constraints validate
- [ ] Self-referencing hierarchy is queryable
- [ ] No existing table, row, or view is altered or removed

---

## 6. Non-Goals

- Go/Fyne client rendering of the menu
- Multi-level nesting beyond parent-child in seed data
- Role-based or permission-filtered menus
- Menu item icons, badges, or visual metadata
- Runtime CRUD API for menu items
- Changes to `vistas_consulta` or any other existing table

---

## 7. Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| Self-FK violation during seed | Low | Split INSERTs: roots first, children second |
| FK violation on `vista_id` | None | Insertion point after all `vistas_consulta` seeds |
| Breaking existing init script flow | None | Purely additive |
