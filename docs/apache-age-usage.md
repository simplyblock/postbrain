# Apache AGE Usage and Operations

This page covers practical Apache AGE setup and troubleshooting for Postbrain.

## Scope

Use this guide when:
- `remember` fails in AGE dual-write paths.
- `graph_query` returns unexpected empty output.
- startup fails in `ensure age overlay`.

## 1. Identify Runtime DB/User

Use the exact connection string the server uses and verify identity:

```sql
SELECT current_user, session_user, current_database();
SHOW search_path;
```

All grants in this page must target the runtime role returned by `current_user`.

## 2. Required Grants (Parameterized by Username)

Run as a role that can grant privileges (owner/admin).

```sql
-- Replace once:
-- :pb_role => runtime app role (example: postgres or pb_app)
-- :pb_db   => target DB name (example: postgres)
\set pb_role 'postgres'
\set pb_db   'postgres'

-- Database
GRANT CONNECT, TEMP ON DATABASE :"pb_db" TO :"pb_role";

-- Public schema (regular Postbrain tables)
GRANT USAGE ON SCHEMA public TO :"pb_role";
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO :"pb_role";
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO :"pb_role";

-- AGE catalog
GRANT USAGE ON SCHEMA ag_catalog TO :"pb_role";
GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA ag_catalog TO :"pb_role";
GRANT USAGE ON TYPE ag_catalog.agtype TO :"pb_role";

-- AGE graph schema (graph name: postbrain)
GRANT USAGE, CREATE ON SCHEMA postbrain TO :"pb_role";
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA postbrain TO :"pb_role";
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA postbrain TO :"pb_role";
```

## 3. Default Privileges for Future Objects

If graph objects are created by a different owner role (for example managed provider role), set default privileges from that owner role:

```sql
-- Example: owner role 'vela' creates objects, runtime role is 'postgres'
ALTER DEFAULT PRIVILEGES FOR ROLE vela IN SCHEMA postbrain
  GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO postgres;
ALTER DEFAULT PRIVILEGES FOR ROLE vela IN SCHEMA postbrain
  GRANT USAGE, SELECT ON SEQUENCES TO postgres;
```

## 4. Ownership Checks and Fixes

Some AGE operations require owner-level behavior.

Check owners:

```sql
SELECT schemaname, tablename, tableowner
FROM pg_tables
WHERE schemaname = 'postbrain'
  AND tablename IN ('_ag_label_vertex', '_ag_label_edge');
```

If ownership is wrong and your platform allows it:

```sql
ALTER SCHEMA postbrain OWNER TO postgres;
ALTER TABLE postbrain._ag_label_vertex OWNER TO postgres;
ALTER TABLE postbrain._ag_label_edge OWNER TO postgres;
```

If managed platform disallows owner changes, keep startup checks read-only and rely on grants/default privileges for runtime writes.

## 5. search_path Requirement

AGE internals may require `ag_catalog` in `search_path` for runtime Cypher paths.

Recommended per-role DB setting:

```sql
ALTER ROLE postgres IN DATABASE postgres
  SET search_path = ag_catalog, "$user", public;
```

Validate in a new session:

```sql
SHOW search_path;
```

## 6. Health Checks

Run as runtime app role.

```sql
SELECT
  has_schema_privilege(current_user, 'ag_catalog', 'USAGE') AS ag_catalog_usage,
  has_type_privilege(current_user, 'ag_catalog.agtype', 'USAGE') AS agtype_usage,
  has_schema_privilege(current_user, 'postbrain', 'USAGE') AS postbrain_usage,
  has_schema_privilege(current_user, 'postbrain', 'CREATE') AS postbrain_create;

SELECT extname, extversion
FROM pg_extension
WHERE extname = 'age';

SELECT n.nspname, oc.opcname, am.amname
FROM pg_opclass oc
JOIN pg_namespace n ON n.oid = oc.opcnamespace
JOIN pg_am am ON am.oid = oc.opcmethod
WHERE n.nspname = 'ag_catalog' AND oc.opcname = 'graphid_ops';
```

Expected:
- all privilege checks above are `t`
- `age` extension exists
- `graphid_ops` exists in `ag_catalog`

## 7. Symptom -> Likely Cause -> Fix

- `permission denied for schema ag_catalog`
  - cause: missing AGE catalog grants for runtime role
  - fix: section 2 AGE catalog grants

- `permission denied for schema postbrain`
  - cause: missing graph schema grants
  - fix: section 2 graph schema grants (`USAGE, CREATE`, table/sequence grants)

- `must be owner of table _ag_label_vertex`
  - cause: owner-only path with mismatched object owner/runtime role
  - fix: align ownership (section 4) or ensure read-only startup probes and owner grants/default privileges

- `operator class "graphid_ops" does not exist for access method "btree"`
  - cause: commonly session `search_path` missing `ag_catalog` for AGE internals
  - fix: section 5 `ALTER ROLE ... SET search_path ...`; verify section 6 checks

- `a dollar-quoted string constant is expected`
  - cause: parameterized Cypher body in SQL call
  - fix: use literal dollar-quoted Cypher SQL in AGE call sites

## 8. Minimal Runtime Smoke Tests

```sql
-- Read-only AGE query
SELECT * FROM ag_catalog.cypher('postbrain', $$ RETURN 1 $$) AS (result ag_catalog.agtype);

-- Write path smoke test
SELECT * FROM ag_catalog.cypher('postbrain', $$
CREATE (n:Entity {id:'__pb_smoke__'}) RETURN n
$$) AS (result ag_catalog.agtype);

SELECT * FROM ag_catalog.cypher('postbrain', $$
MATCH (n:Entity {id:'__pb_smoke__'}) DETACH DELETE n RETURN 1
$$) AS (result ag_catalog.agtype);
```

## 9. Notes for Managed PostgreSQL

- Platform owner roles (for example `vela`) may own AGE objects even if app runs as another role.
- `GRANT` statements do not require `pg_reload_conf()`.
- If extension/object ownership cannot be changed, use role/default-privilege strategy and avoid owner-only startup probes.
