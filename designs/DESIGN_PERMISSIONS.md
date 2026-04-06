# Postbrain — Permissions Design

---

## Goals

- A single, coherent model that replaces the current ad-hoc `read`/`write`/`admin` token flags.
- Permission checks are consistent across REST, MCP, and WebUI — no per-surface re-implementation.
- Tokens are downscoped credentials: a token can never exceed the issuing principal's effective permissions.
- Sharing is possible without granting full scope access.
- The model is derivable from stable sources of truth: scope ownership, principal memberships, and explicit grants.

---

## Permission Model: Three Dimensions

Every permission is a triple of **(principal, resource:operation, scope)**:

- **principal** — who is asking (derived from ownership / membership / direct grant)
- **resource:operation** — what they want to do (`memories:read`, `scopes:edit`, `knowledge:delete`, …)
- **scope** — where they want to do it (a specific scope, subject to inheritance rules)

There is one special permission outside this model: `systemadmin` — see below.

---

## Resources and Operations

Each resource supports a defined set of operations. Not every operation is meaningful on every resource.

| Resource | Operations | Covers |
|---|---|---|
| `memories` | `read`, `write`, `edit`, `delete` | memories table, consolidations |
| `knowledge` | `read`, `write`, `edit`, `delete` | knowledge_artifacts, history, endorsements, staleness_flags |
| `collections` | `read`, `write`, `edit`, `delete` | knowledge_collections, collection_items |
| `skills` | `read`, `write`, `edit`, `delete` | skills, endorsements, history |
| `sessions` | `read`, `write` | sessions, events |
| `graph` | `read` | entities, relations; graph is auto-managed as side effect of other writes |
| `scopes` | `read`, `write`, `edit`, `delete` | scope entities (create child, rename, attach repo, re-parent, delete) |
| `principals` | `read`, `write`, `edit`, `delete` | principals, principal_memberships |
| `tokens` | `read`, `write`, `edit`, `delete` | API tokens (list, create, update scopes, revoke) |
| `sharing` | `read`, `write`, `delete` | sharing_grants, scope_grants |
| `promotions` | `read`, `write`, `edit`, `delete` | promotion_requests |

### Operation semantics

| Operation | General meaning | Notable cases |
|---|---|---|
| `read` | View, list, search content | `graph:read` includes Cypher queries; `scopes:read` lets you see the scope exists and navigate ancestors |
| `write` | Create new content, update existing content | `promotions:write` = propose a promotion; `scopes:write` = create a child scope |
| `edit` | Modify structural or configuration properties of an entity | `scopes:edit` = rename, attach/detach repo, re-parent; `knowledge:edit` = change status (deprecate, publish), change visibility; `principals:edit` = manage memberships |
| `delete` | Remove content or the entity itself | `memories:delete` = forget (soft-delete); `scopes:delete` = delete the scope; `promotions:delete` = cancel a promotion request |

### Shorthand permissions

A bare permission with no resource prefix is shorthand for that operation across **all** resources:

| Shorthand | Equivalent |
|---|---|
| `read` | `memories:read`, `knowledge:read`, `collections:read`, `skills:read`, `sessions:read`, `graph:read`, `scopes:read`, `principals:read`, `tokens:read`, `sharing:read`, `promotions:read` |
| `write` | all `:write` permissions |
| `edit` | all `:edit` permissions |
| `delete` | all `:delete` permissions |

---

## `systemadmin`

`systemadmin` is a special flag on a principal that bypasses all permission checks. It is intended solely for the bootstrap/root system user created at installation time — the first account before any scopes or memberships exist.

- `systemadmin` is a boolean field on `principals` (`is_system_admin`), not a token permission.
- A `systemadmin` principal can perform any operation on any resource in any scope.
- `systemadmin` cannot be granted via membership or scope grants — it is set directly on the principal record.
- A `systemadmin` token still respects token-level downscoping (a `systemadmin` principal can issue a restricted read-only token).

---

## Sources of Permissions

A principal's effective permissions on a scope derive from four sources, evaluated additively.

### 1. `systemadmin` flag

If `principal.is_system_admin = true`, all resource:operation pairs are granted on all scopes. No further checks needed.

### 2. Direct Scope Ownership

A principal that owns a scope (`scopes.principal_id = principal.id`) has full permissions on that scope and all its descendants:

```
*:read, *:write, *:edit, *:delete  (all resources, all operations)
```

Ownership is assigned at scope creation and transferred via `PUT /v1/scopes/{id}/owner`.

### 3. Principal Membership Roles

A principal that is a member of another principal inherits permissions on all scopes owned by that principal (and their descendants). The membership `role` determines the permission set:

**`member`** — read and write access to content:

```
memories:read,    memories:write
knowledge:read,   knowledge:write
collections:read, collections:write
skills:read,      skills:write
sessions:write
graph:read
scopes:read
principals:read
tokens:read       (see self-service note below)
sharing:read
promotions:read,  promotions:write
```

**`admin`** — adds structural control:

```
everything member has, plus:
memories:edit,    memories:delete
knowledge:edit
collections:edit
skills:edit
scopes:edit,      scopes:write
principals:edit
sharing:write
promotions:edit
tokens:edit       (for principals under management)
```

**`owner`** — adds deletion rights:

```
everything admin has, plus:
knowledge:delete
collections:delete
skills:delete
scopes:delete
principals:delete
sharing:delete
promotions:delete
tokens:delete     (for principals under management)
```

### 4. Direct Scope Grants

A principal can be explicitly granted specific `{resource}:{operation}` permissions on a specific scope, independent of any ownership or membership relationship. This is the primary mechanism for the sharing feature.

```sql
CREATE TABLE scope_grants (
    id            UUID PRIMARY KEY DEFAULT uuidv7(),
    principal_id  UUID NOT NULL REFERENCES principals(id) ON DELETE CASCADE,
    scope_id      UUID NOT NULL REFERENCES scopes(id)     ON DELETE CASCADE,
    permissions   TEXT[] NOT NULL,   -- e.g. {"memories:read","knowledge:read"}
    granted_by    UUID NOT NULL REFERENCES principals(id),
    expires_at    TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (principal_id, scope_id)
);
```

Direct grants are additive with membership-derived permissions. A granting principal can only grant permissions they themselves hold on the scope — no privilege escalation.

---

## Scope Hierarchy & Inheritance

Scopes form a tree via `scopes.parent_id`, with `ltree` paths for efficient ancestor/descendant queries.

### Downward inheritance (always)

A permission on scope S is automatically inherited by all descendants of S. This applies to all permission sources.

```
scope Company  [principal has knowledge:write here]
  └── scope Team          ← knowledge:write inherited
        └── scope Project ← knowledge:write inherited
```

### Upward read (read-only)

If a principal has any `read` permission on scope S (for any resource), they also gain `{resource}:read` on all ancestors of S. This allows navigating up the tree for context — seeing the parent project when you have access to a team scope.

```
scope Company        ← :read granted on all resources P can read in Project
  └── scope Team     ← :read granted
        └── scope Project  ← P has knowledge:read here (direct or derived)
```

### No upward write/edit/delete

Having `write`, `edit`, or `delete` on a child scope does not grant those permissions on ancestor scopes. A team member cannot modify the company scope just because they can write to a team scope.

---

## Effective Permission Resolution

For principal P requesting `{resource}:{operation}` on scope S:

1. **`systemadmin`**: if `P.is_system_admin` → **granted**.

2. **Direct ownership**: if `scopes.principal_id = P.id` for scope S or any ancestor of S → **granted**.

3. **Membership derivation**: for each membership `(P is member of Q, role R)`:
   - Find scopes owned by Q (`scopes.principal_id = Q.id`)
   - If any such scope is S or an ancestor of S → check if role R grants `{resource}:{operation}` → **granted** if yes.

4. **Direct grants**: for each `scope_grants` row `(principal_id = P, scope_id = G)`:
   - If G is S or an ancestor of S → check if `permissions` includes `{resource}:{operation}` → **granted** if yes.

5. **Upward read**: if the request is `{resource}:read` and the above grants that same read on any descendant of S, the request is **granted** for S.

6. Otherwise → **denied**.

---

## Token Self-Service

Token management for a principal's **own** tokens is always permitted, regardless of any permission grant:

- A principal may always create, list, update scopes on, and revoke their own tokens.
- No `tokens:write`, `tokens:read`, `tokens:edit`, or `tokens:delete` permission is required for self-service.

Managing **another** principal's tokens requires:
- `principals:edit` (or higher) on a scope that grants authority over that principal, **or**
- `systemadmin`.

---

## Token Downscoping

Tokens are credentials issued on behalf of a principal. A token's effective permissions are always a **subset** of the issuing principal's effective permissions.

A token has two independent restriction axes:

### Axis 1: Permission restriction

The token's `permissions` field lists the maximum `{resource}:{operation}` pairs the token may exercise. At enforcement time:

```
effective token permission =
    principal's effective {resource}:{operation} on scope S
    ∩ token declared permissions
```

Examples:
- `["read"]` — shorthand for all `:read` operations, principal's full read access.
- `["memories:read", "knowledge:read"]` — read-only, memories and knowledge only.
- `["memories:read", "memories:write", "knowledge:read"]` — write memories, read knowledge.
- `["memories:read", "memories:write", "memories:delete"]` — full memory access, nothing else.

### Axis 2: Scope restriction

The token's `scope_ids` field lists the scopes the token may access. If NULL, the token may access all scopes the principal can access. If non-null, only those scopes and their descendants.

```
token accessible scopes =
    principal's effective accessible scopes
    ∩ (descendants of scope_ids, or all if scope_ids is NULL)
```

### Token cannot exceed principal

The enforcement layer verifies:
1. The principal has `{resource}:{operation}` on scope S, **and**
2. The token's declared permissions include `{resource}:{operation}`.

A `["memories:read"]` token issued by an `owner`-role principal cannot write memories.

---

## Promotions: Two Paths

A memory promotion request creates a `promotion_request` row pointing from a memory to a target knowledge artifact in a target scope. Two permission paths are supported:

### Path 1: Direct write (standard)

Principal has both `memories:write` on the source scope and `knowledge:write` on the target scope.

- The promotion is created with the configured standard `review_required` count (e.g. 1 endorsement).
- The principal effectively already has the authority to publish knowledge directly; the promotion is a formality for audit purposes.

### Path 2: Promotion-only (elevated review)

Principal has `promotions:write` on the source scope but **not** `knowledge:write` on the target scope.

- The promotion is created with an elevated `review_required` count (e.g. 2–3 endorsements).
- The principal proposes the promotion; knowledge holders must endorse before it is merged.
- This path exists because knowledge requires endorsement anyway — the lack of direct write access is compensated by requiring more reviewers.

Approving or rejecting a promotion requires `promotions:edit` on the target scope.

---

## Relationship to Existing `sharing_grants`

The existing `sharing_grants` table is a **content-item-level** sharing mechanism: it makes a specific memory or artifact visible to members of another scope. This is distinct from `scope_grants` (principal-level access control).

Both remain:

| Table | What it grants | Granularity |
|---|---|---|
| `scope_grants` | Principal P may access scope S with declared permissions | Principal × scope |
| `sharing_grants` | Members of scope B may see item X from scope A | Item × scope |

Use `scope_grants` for: "allow this external agent to read our team's knowledge".
Use `sharing_grants` for: "share this one memory with the platform team without giving them scope access".

---

## Permission Matrix

### REST Endpoints

| Endpoint | Method | Required Permission |
|---|---|---|
| `/v1/scopes` | GET | `scopes:read` |
| `/v1/scopes` | POST | `scopes:write` |
| `/v1/scopes/{id}` | GET | `scopes:read` |
| `/v1/scopes/{id}` | PUT | `scopes:edit` |
| `/v1/scopes/{id}/owner` | PUT | `scopes:edit` |
| `/v1/scopes/{id}` | DELETE | `scopes:delete` |
| `/v1/scopes/{id}/repo` | POST | `scopes:edit` |
| `/v1/scopes/{id}/repo/sync` | POST | `scopes:edit` |
| `/v1/memories` | POST | `memories:write` |
| `/v1/memories/recall` | GET | `memories:read` |
| `/v1/memories/{id}` | GET | `memories:read` |
| `/v1/memories/{id}` | PATCH | `memories:write` |
| `/v1/memories/{id}` | DELETE | `memories:delete` |
| `/v1/memories/{id}/promote` | POST | `memories:write` + `knowledge:write`, or `promotions:write` |
| `/v1/memories/summarize` | POST | `memories:write` |
| `/v1/knowledge` | POST | `knowledge:write` |
| `/v1/knowledge/search` | GET | `knowledge:read` |
| `/v1/knowledge/{id}` | GET | `knowledge:read` |
| `/v1/knowledge/{id}` | PATCH | `knowledge:write` |
| `/v1/knowledge/{id}` | DELETE | `knowledge:delete` |
| `/v1/knowledge/{id}/endorse` | POST | `knowledge:write` |
| `/v1/knowledge/{id}/deprecate` | POST | `knowledge:edit` |
| `/v1/knowledge/{id}/history` | GET | `knowledge:read` |
| `/v1/knowledge/{id}/sources` | GET | `knowledge:read` |
| `/v1/knowledge/{id}/digests` | GET | `knowledge:read` |
| `/v1/collections` | GET | `collections:read` |
| `/v1/collections` | POST | `collections:write` |
| `/v1/collections/{id}` | GET | `collections:read` |
| `/v1/collections/{id}` | PATCH | `collections:edit` |
| `/v1/collections/{id}` | DELETE | `collections:delete` |
| `/v1/collections/{id}/items` | POST | `collections:write` |
| `/v1/collections/{id}/items/{artifact_id}` | DELETE | `collections:write` |
| `/v1/skills/search` | GET | `skills:read` |
| `/v1/skills/{id}` | GET | `skills:read` |
| `/v1/skills` | POST | `skills:write` |
| `/v1/skills/{id}` | PATCH | `skills:write` |
| `/v1/skills/{id}` | DELETE | `skills:delete` |
| `/v1/skills/{id}/endorse` | POST | `skills:write` |
| `/v1/skills/{id}/deprecate` | POST | `skills:edit` |
| `/v1/skills/{id}/install` | POST | `skills:read` |
| `/v1/skills/{id}/invoke` | POST | `skills:read` |
| `/v1/sessions` | POST | `sessions:write` |
| `/v1/sessions/{id}` | PATCH | `sessions:write` |
| `/v1/context` | GET | `read` |
| `/v1/graph` | GET | `graph:read` |
| `/v1/graph/query` | POST | `graph:read` |
| `/v1/graph/callers` | GET | `graph:read` |
| `/v1/graph/callees` | GET | `graph:read` |
| `/v1/graph/deps` | GET | `graph:read` |
| `/v1/graph/dependents` | GET | `graph:read` |
| `/v1/sharing/grants` | GET | `sharing:read` |
| `/v1/sharing/grants` | POST | `sharing:write` |
| `/v1/sharing/grants/{id}` | DELETE | `sharing:delete` |
| `/v1/promotions` | GET | `promotions:read` |
| `/v1/promotions/{id}/approve` | POST | `promotions:edit` |
| `/v1/promotions/{id}/reject` | POST | `promotions:edit` |
| `/v1/principals` | GET | `principals:read` |
| `/v1/principals` | POST | `principals:write` |
| `/v1/principals/{id}` | GET | `principals:read` |
| `/v1/principals/{id}` | PUT | `principals:edit` |
| `/v1/principals/{id}` | DELETE | `principals:delete` |
| `/v1/principals/{id}/members` | GET | `principals:read` |
| `/v1/principals/{id}/members` | POST | `principals:edit` |
| `/v1/principals/{id}/members/{member_id}` | DELETE | `principals:edit` |
| `/v1/tokens` | GET | self-service or `tokens:read` |
| `/v1/tokens` | POST | self-service or `tokens:write` |
| `/v1/tokens/{id}/scopes` | POST | self-service or `tokens:edit` |
| `/v1/tokens/{id}/revoke` | POST | self-service or `tokens:delete` |

### MCP Tools

| Tool | Required Permission |
|---|---|
| `recall` | `memories:read` |
| `context` | `read` |
| `remember` | `memories:write` |
| `forget` | `memories:delete` |
| `summarize` | `memories:write` |
| `publish` | `knowledge:write` |
| `endorse` | `knowledge:write` or `skills:write` |
| `promote` | `memories:write` + `knowledge:write`, or `promotions:write` |
| `collect` | `collections:read` |
| `skill_search` | `skills:read` |
| `knowledge_detail` | `knowledge:read` |
| `skill_install` | `skills:read` |
| `skill_invoke` | `skills:read` |
| `list_scopes` | `scopes:read` |
| `graph_query` | `graph:read` |
| `session_begin` | `sessions:write` |
| `session_end` | `sessions:write` |
| `synthesize_topic` | `knowledge:write` |

---

## Required Schema Changes

### New: `scope_grants` table

```sql
CREATE TABLE scope_grants (
    id            UUID PRIMARY KEY DEFAULT uuidv7(),
    principal_id  UUID NOT NULL REFERENCES principals(id) ON DELETE CASCADE,
    scope_id      UUID NOT NULL REFERENCES scopes(id)     ON DELETE CASCADE,
    permissions   TEXT[] NOT NULL,
    granted_by    UUID NOT NULL REFERENCES principals(id),
    expires_at    TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (principal_id, scope_id)
);

CREATE INDEX scope_grants_principal_idx ON scope_grants (principal_id);
CREATE INDEX scope_grants_scope_idx     ON scope_grants (scope_id);
```

### Modified: `principals` table

Add `is_system_admin` boolean:

```sql
ALTER TABLE principals ADD COLUMN is_system_admin BOOLEAN NOT NULL DEFAULT false;
```

### Modified: `tokens` table

The `permissions` column changes from flat `{read,write,admin}` to `{resource}:{operation}` format. Bare `read`/`write` shorthand is preserved. `admin` is removed.

```sql
-- Valid permission values after migration:
-- Shorthand:          read, write, edit, delete
-- Resource-scoped:    memories:read, memories:write, memories:edit, memories:delete
--                     knowledge:read, knowledge:write, knowledge:edit, knowledge:delete
--                     collections:read, collections:write, collections:edit, collections:delete
--                     skills:read, skills:write, skills:edit, skills:delete
--                     sessions:read, sessions:write
--                     graph:read
--                     scopes:read, scopes:write, scopes:edit, scopes:delete
--                     principals:read, principals:write, principals:edit, principals:delete
--                     tokens:read, tokens:write, tokens:edit, tokens:delete
--                     sharing:read, sharing:write, sharing:delete
--                     promotions:read, promotions:write, promotions:edit, promotions:delete
```

Migration: existing `admin` tokens → `["read", "write", "edit", "delete"]`.

### Modified: `principal_memberships` table

No schema change. The `role` values (`member`, `admin`, `owner`) are unchanged; their semantics are now formally defined by the permission sets in this document.

---

## What Changes in Code

| Area | Change |
|---|---|
| `internal/auth/permissions.go` | Rewrite: resource×operation model, shorthand expansion, `systemadmin` check |
| `internal/oauth/scopes.go` | Align OAuth scopes with new `{resource}:{operation}` format |
| `internal/db/migrations/` | New migration: `scope_grants` table, `principals.is_system_admin`, token `admin` migration |
| `internal/api/rest/permissionauth.go` | Replace HTTP-method heuristic with explicit per-route `{resource}:{operation}` requirements |
| `internal/api/mcp/permissionauth.go` | Update tool permission assignments |
| `internal/api/mcp/server.go` | Update `withToolPermission` calls |
| `internal/ui/tokens.go` | Update token creation form to new permission set |
| `internal/api/scopeauth/scopeauth.go` | Replace `EffectiveScopeIDs` with full `(resource, operation, scope)` resolver |
| New: `internal/authz/` | Centralized effective-permission resolver used by all three surfaces (REST, MCP, WebUI) |