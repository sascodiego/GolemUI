# Explore: nav-menu-schema

> **Scope:** feasibility of adding `golemui.menu_navegacion` to `docker/init-db/02_init_core.sql` only. No implementation in this phase.

## 1. Current Schema State (`docker/init-db/02_init_core.sql`)

The file is a single sequential DDL + seed script. Tables appear in this order:

| # | Table | Outgoing FKs | Notes |
|---|---|---|---|
| 1 | `golemui.componentes` | — | UI component catalog; 10 seed rows |
| 2 | `golemui.estilos` | — | Semantic design tokens; 7 seed rows |
| 3 | `golemui.mapeo_interfaz` | — | Layer 3 overrides; composite PK |
| 4 | `golemui.sesion_borrador` | — | Draft forms |
| 5 | `golemui.vistas_consulta` | — | Screen catalog; **3 seed rows** |
| 6 | `golemui.vistas_guardadas` | `vista_consulta_id` → `vistas_consulta(id)` | Saved user views |

## 2. Seeded `vistas_consulta` IDs (verified)

- `home`
- `transacciones_list`
- `query_runner`

These are the only valid `vista_id` references for new `menu_navegacion` rows.

## 3. FK Dependency Analysis

- **`padre_id` (self-referencing):** PostgreSQL accepts self-FKs in a single `CREATE TABLE`; check is deferred to row-level. No DDL ordering constraint.
- **`vista_id` → `vistas_consulta(id)`:** Referenced table must exist at `CREATE TABLE` time, and non-null values must be present in `vistas_consulta`. Both satisfied after the third `INSERT INTO golemui.vistas_consulta`.

## 4. Recommended Insertion Point

**AFTER** the third `INSERT INTO golemui.vistas_consulta` (after `query_runner` seed), **BEFORE** `CREATE TABLE IF NOT EXISTS golemui.vistas_guardadas`.

This placement:
1. Satisfies FK dependency on `vistas_consulta`
2. Keeps screen-catalog cluster contiguous
3. Compatible with existing `IF NOT EXISTS` / `ON CONFLICT DO NOTHING` pattern
4. Preserves Clean Slate ephemeral-DB contract

## 5. Risks and Constraints

- **Self-FK in seed data:** Split seeds into multiple `INSERT` statements (roots first, then children) to avoid intra-statement FK violations.
- **`IF NOT EXISTS` + `ON CONFLICT`:** Consistent with rest of script; idempotent.
- **Layer separation preserved:** Table lives in `golemui_core`, not `negocio_production`.

### Out of scope
- No changes to other tables
- No changes outside `02_init_core.sql`
- No Go code changes

## 6. Feasibility Verdict

**Feasible, low-risk, narrowly scoped.** The schema drops into `02_init_core.sql` between `vistas_consulta` seeds and `vistas_guardadas` with no impact on existing tables.
