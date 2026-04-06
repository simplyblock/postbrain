# Access Control Reference

This page is the canonical reference for Postbrain access control.

Use it when you need to answer questions like:

- Can this principal do this action in this scope?
- Why does this token fail with `scope access denied` or `insufficient permissions`?
- Which permission key should a token include?

## Core model

Every authorization decision is evaluated as:

- `(principal, resource:operation, scope)`

Examples:

- `memories:read` on `project:acme/platform/postbrain`
- `scopes:edit` on `team:acme/platform`
- `tokens:delete` on `company:acme`

`systemadmin` is a special principal-level bypass flag (`principals.is_system_admin = true`).

## Resources and operations

Permission keys are `{resource}:{operation}`.

Resources:

- `memories`
- `knowledge`
- `collections`
- `skills`
- `sessions`
- `graph`
- `scopes`
- `principals`
- `tokens`
- `sharing`
- `promotions`

Operations:

- `read`: view/list/search
- `write`: create/update content
- `edit`: structural/config changes
- `delete`: remove entities/content

Shorthand permissions:

- `read`, `write`, `edit`, `delete`

Each shorthand expands across all resources for that operation.

## Permission sources

A principal's effective permissions are additive from these sources:

1. `is_system_admin`: full access everywhere.
2. Direct scope ownership: full access on owned scopes and descendants.
3. Membership-derived role permissions (`member`, `admin`, `owner`) on scopes owned by the parent principal.
4. Direct scope grants via `scope_grants` for specific permissions on specific scopes.

No source can be used to escalate beyond what the granting principal already has.

## Scope inheritance rules

Scopes form a parent/child tree.

- Downward inheritance: permissions on scope `S` apply to descendants of `S`.
- Upward read: `*:read` on a child implies `*:read` on ancestors.
- No upward write/edit/delete: child permissions never grant ancestor mutation rights.

## Role summary

Membership roles grant progressively broader permission sets:

- `member`: read/write focused content permissions.
- `admin`: adds structural/edit capabilities.
- `owner`: adds delete capabilities.

Exact role permission sets are defined in [designs/DESIGN_PERMISSIONS.md](../designs/DESIGN_PERMISSIONS.md).

## Token model

Tokens are downscoped credentials issued on behalf of a principal.

Token effective access on a request is:

- principal effective permission on target scope
- intersected with token declared permissions
- intersected with token scope restrictions (`scope_ids`) when present

So a token can never exceed the principal's authority.

### Token permission restriction

Use `permissions` to limit allowed operations:

- broad read-only: `["read"]`
- targeted: `["memories:read","knowledge:read"]`
- mixed: `["memories:read","memories:write","knowledge:read"]`

### Token scope restriction

Use `scope_ids` to limit where token permissions apply:

- `scope_ids = NULL`: any scope the principal can access.
- `scope_ids = [A, B]`: only `A`/`B` and their descendants.

## Token self-service vs delegated token management

Self-service is always allowed:

- create/list/update scopes/revoke your own tokens

Managing another principal's tokens requires delegated authority:

- `principals:edit` on a scope that grants authority over that principal, or
- `systemadmin`

## Promotions permission paths

Promotion from memory to knowledge supports two paths:

1. Direct-write path:
- `memories:write` on source scope + `knowledge:write` on target scope.
2. Promotion-only path:
- `promotions:write` on source scope without direct `knowledge:write` on target.

Approval/rejection requires `promotions:edit` on the target scope.

## Quick troubleshooting map

If access fails:

1. Verify target scope is included by token `scope_ids` (or unrestricted).
2. Verify token declares required `{resource}:{operation}`.
3. Verify principal effectively has required permission in that scope via ownership/membership/scope grant.
4. Check whether request targets ancestor scope and assumes upward write/edit/delete (not allowed).

## Related references

- [Security and Access Model](./security.md)
- [API Auth Examples](./api-auth-examples.md)
- [Troubleshooting Playbook](./troubleshooting-playbook.md)
- [Permissions Design Source](../designs/DESIGN_PERMISSIONS.md)
