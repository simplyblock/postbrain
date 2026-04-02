# Postbrain — OAuth Design

## Overview

This document specifies two complementary OAuth capabilities for Postbrain. First, social login for
the Web UI (Postbrain as an OAuth *client*): users authenticate via GitHub, Google, or GitLab, after
which Postbrain issues a standard `pb_*` bearer token and sets the `pb_session` cookie — no change
to the downstream auth path. Second, an OAuth 2.0 authorization server for MCP clients (Postbrain as
an OAuth *server*): Claude Desktop, VS Code Copilot Chat, and similar agents use Authorization Code +
PKCE to obtain a `pb_*` bearer token for subsequent MCP calls, satisfying the MCP 2024-11
specification.

---

## Database Changes

Migration `000011_oauth.up.sql`.

```sql
-- ─────────────────────────────────────────
-- 1. Social identity links
-- Connects a principals row to an external
-- provider identity.  One principal may have
-- multiple provider accounts (e.g. GitHub +
-- Google).
-- ─────────────────────────────────────────
CREATE TABLE social_identities (
    id             UUID        PRIMARY KEY DEFAULT uuidv7(),
    principal_id   UUID        NOT NULL REFERENCES principals(id) ON DELETE CASCADE,
    provider       TEXT        NOT NULL,  -- 'github' | 'google' | 'gitlab'
    provider_id    TEXT        NOT NULL,  -- provider's stable numeric/opaque user id
    email          citext,                -- may be NULL if provider withholds it
    display_name   TEXT,
    avatar_url     TEXT,
    raw_profile    JSONB       NOT NULL DEFAULT '{}',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (provider, provider_id)
);

CREATE INDEX social_identities_principal_idx ON social_identities (principal_id);

CREATE TRIGGER social_identities_updated_at BEFORE UPDATE ON social_identities
    FOR EACH ROW EXECUTE FUNCTION touch_updated_at();

-- ─────────────────────────────────────────
-- 2. OAuth clients (authorization server)
-- Registered MCP clients.  Public clients
-- (PKCE-only) have client_secret_hash NULL.
-- ─────────────────────────────────────────
CREATE TABLE oauth_clients (
    id                  UUID        PRIMARY KEY DEFAULT uuidv7(),
    client_id           TEXT        NOT NULL UNIQUE, -- opaque random string, handed to client
    client_secret_hash  TEXT,                        -- SHA-256 hex; NULL for public clients
    name                TEXT        NOT NULL,
    redirect_uris       TEXT[]      NOT NULL DEFAULT '{}',
    grant_types         TEXT[]      NOT NULL DEFAULT '{"authorization_code"}',
    scopes              TEXT[]      NOT NULL DEFAULT '{}',
    is_public           BOOLEAN     NOT NULL DEFAULT true,
    meta                JSONB       NOT NULL DEFAULT '{}',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at          TIMESTAMPTZ
);

CREATE INDEX oauth_clients_client_id_idx ON oauth_clients (client_id);

-- ─────────────────────────────────────────
-- 3. Authorization codes
-- Short-lived (10 min) single-use codes
-- produced during the Authorization Code flow.
-- ─────────────────────────────────────────
CREATE TABLE oauth_auth_codes (
    id               UUID        PRIMARY KEY DEFAULT uuidv7(),
    code_hash        TEXT        NOT NULL UNIQUE,  -- SHA-256 hex of the raw code
    client_id        UUID        NOT NULL REFERENCES oauth_clients(id) ON DELETE CASCADE,
    principal_id     UUID        NOT NULL REFERENCES principals(id) ON DELETE CASCADE,
    redirect_uri     TEXT        NOT NULL,
    scopes           TEXT[]      NOT NULL DEFAULT '{}',
    code_challenge   TEXT        NOT NULL,          -- S256 PKCE challenge (base64url)
    expires_at       TIMESTAMPTZ NOT NULL,
    used_at          TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX oauth_auth_codes_code_hash_idx ON oauth_auth_codes (code_hash);
CREATE INDEX oauth_auth_codes_expires_idx   ON oauth_auth_codes (expires_at)
    WHERE used_at IS NULL;

-- ─────────────────────────────────────────
-- 4. OAuth state parameters (CSRF guard)
-- Stored server-side for the duration of the
-- social-login and MCP-consent redirects.
-- ─────────────────────────────────────────
CREATE TABLE oauth_states (
    id          UUID        PRIMARY KEY DEFAULT uuidv7(),
    state_hash  TEXT        NOT NULL UNIQUE,  -- SHA-256 hex of the raw state value
    kind        TEXT        NOT NULL CHECK (kind IN ('social', 'mcp_consent')),
    payload     JSONB       NOT NULL DEFAULT '{}',
    expires_at  TIMESTAMPTZ NOT NULL,
    used_at     TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX oauth_states_state_hash_idx ON oauth_states (state_hash);
CREATE INDEX oauth_states_expires_idx    ON oauth_states (expires_at)
    WHERE used_at IS NULL;
```

All four tables are covered by a single down migration that drops them in reverse dependency order.

---

## Configuration Additions

```yaml
# ─────────────────────────────────────────────────────────────
# OAuth social login providers (Postbrain as OAuth *client*)
# ─────────────────────────────────────────────────────────────
oauth:
  # Base URL used to build redirect_uri values.
  # Must be reachable from the browser.  No trailing slash.
  base_url: "https://postbrain.example.com"

  providers:
    github:
      enabled:       true
      client_id:     "Iv1.abc123"
      client_secret: "ghsec_..."
      # scopes requested from GitHub — read:user + user:email is sufficient
      scopes:        ["read:user", "user:email"]

    google:
      enabled:       false
      client_id:     ""
      client_secret: ""
      scopes:        ["openid", "email", "profile"]

    gitlab:
      enabled:       false
      client_id:     ""
      client_secret: ""
      # self-hosted GitLab; omit or set to "https://gitlab.com" for SaaS
      instance_url:  "https://gitlab.com"
      scopes:        ["read_user"]

  # ─────────────────────────────────────────────────────────────
  # Authorization server settings (Postbrain as OAuth *server*)
  # ─────────────────────────────────────────────────────────────
  server:
    # Lifetime of authorization codes.
    auth_code_ttl:    10m
    # Lifetime of the oauth_states CSRF entries.
    state_ttl:        15m
    # Default token TTL issued via the OAuth flow (0 = no expiry).
    token_ttl:        0
    # Whether dynamic client registration is open (RFC 7591).
    # Set to false to require manual registration.
    dynamic_registration: true
```

Add the corresponding Go struct to `internal/config/config.go`:

```go
type OAuthConfig struct {
    BaseURL   string                    `mapstructure:"base_url"`
    Providers map[string]ProviderConfig `mapstructure:"providers"`
    Server    OAuthServerConfig         `mapstructure:"server"`
}

type ProviderConfig struct {
    Enabled      bool     `mapstructure:"enabled"`
    ClientID     string   `mapstructure:"client_id"`
    ClientSecret string   `mapstructure:"client_secret"`
    Scopes       []string `mapstructure:"scopes"`
    InstanceURL  string   `mapstructure:"instance_url"` // GitLab only
}

type OAuthServerConfig struct {
    AuthCodeTTL         time.Duration `mapstructure:"auth_code_ttl"`
    StateTTL            time.Duration `mapstructure:"state_ttl"`
    TokenTTL            time.Duration `mapstructure:"token_ttl"`
    DynamicRegistration bool          `mapstructure:"dynamic_registration"`
}
```

Add `OAuth OAuthConfig` to the top-level `Config` struct. Defaults:

```go
v.SetDefault("oauth.server.auth_code_ttl",          "10m")
v.SetDefault("oauth.server.state_ttl",              "15m")
v.SetDefault("oauth.server.token_ttl",              "0")
v.SetDefault("oauth.server.dynamic_registration",   true)
```

---

## New Packages and Files

### `internal/oauth` — authorization server core

| File | Responsibility |
|------|---------------|
| `server.go` | `Server` struct wiring all dependencies; `NewServer(pool, tokenStore, principalStore, cfg)`. |
| `clients.go` | `ClientStore`: CRUD for `oauth_clients`; `LookupByClientID`, `Register`, `Revoke`. |
| `codes.go` | `CodeStore`: `Issue`, `Consume` (marks `used_at`, returns principal + scopes), and S256 PKCE verification. |
| `states.go` | `StateStore`: `Issue` (stores hashed state + payload), `Consume` (lookup + mark used). |
| `pkce.go` | Pure functions: `VerifyS256(verifier, challenge string) bool`; `GenerateChallenge(verifier string) string`. |
| `scopes.go` | Scope constants (`ScopeMemoriesRead`, …); `ParseScopes(raw string) ([]string, error)` validates against the known set. |
| `token_exchange.go` | Converts an auth code into a `pb_*` bearer token via `auth.TokenStore.Create`; translates OAuth scopes to `permissions []string`. |
| `metadata.go` | Builds the RFC 8414 `/.well-known/oauth-authorization-server` JSON response from `cfg`. |

### `internal/social` — social login (OAuth client)

| File | Responsibility |
|------|---------------|
| `provider.go` | `Provider` interface: `AuthURL(state string) string`, `Exchange(ctx, code string) (*UserInfo, error)`. |
| `github.go` | `GitHubProvider` — calls `https://github.com/login/oauth/authorize` and `https://api.github.com/user`. |
| `google.go` | `GoogleProvider` — OIDC discovery; exchanges code for an `id_token` and verifies it. |
| `gitlab.go` | `GitLabProvider` — supports both `gitlab.com` and self-hosted via `InstanceURL`. |
| `registry.go` | `NewRegistry(cfg OAuthConfig) map[string]Provider` — constructs only enabled providers. |
| `identity.go` | `IdentityStore`: `FindOrCreate(ctx, provider, userInfo) (*db.Principal, error)` — upserts `social_identities` and `principals`. |

### `internal/db` — generated layer additions

Two new query files under `internal/db/queries/` regenerated with sqlc:

- `oauth_clients.sql` — `RegisterClient`, `LookupClient`, `RevokeClient`
- `oauth_codes.sql` — `IssueCode`, `ConsumeCode`
- `oauth_states.sql` — `IssueState`, `ConsumeState`
- `social_identities.sql` — `UpsertSocialIdentity`, `FindPrincipalBySocialIdentity`

### `internal/ui` additions

| File | Responsibility |
|------|---------------|
| `oauth_social.go` | Handlers for `GET /ui/auth/{provider}` (redirect) and `GET /ui/auth/{provider}/callback` (exchange + set cookie). |
| `oauth_consent.go` | `GET /ui/oauth/authorize` — renders the consent screen; `POST /ui/oauth/authorize` — records approval, issues auth code, redirects to client. |

---

## Route Inventory

All new routes are registered on the existing `http.NewServeMux` in `cmd/postbrain/main.go`.

### OAuth Authorization Server (`/oauth/*`)

These routes implement the RFC 6749 + RFC 7591 + RFC 8414 surface. They are unauthenticated at the
transport layer; each handler enforces its own validation.

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/.well-known/oauth-authorization-server` | RFC 8414 server metadata. Returns JSON with `issuer`, `authorization_endpoint`, `token_endpoint`, `registration_endpoint`, `scopes_supported`, `response_types_supported`, `code_challenge_methods_supported`. Required by MCP 2024-11. |
| `GET` | `/oauth/authorize` | Authorization endpoint. Validates `client_id`, `redirect_uri`, `response_type=code`, `code_challenge`, `code_challenge_method=S256`, `scope`, and `state`. Redirects the browser to `/ui/oauth/authorize` (consent screen) with the validated parameters preserved in a signed `oauth_states` entry. |
| `POST` | `/oauth/token` | Token endpoint. Accepts `grant_type=authorization_code`, exchanges a code for a `pb_*` bearer token. Validates PKCE `code_verifier`. For confidential clients also validates `client_secret`. Returns RFC 6749 §5.1 JSON. |
| `POST` | `/oauth/register` | RFC 7591 dynamic client registration. Accepts JSON body, creates an `oauth_clients` row, returns `client_id` (and `client_secret` if confidential). Disabled when `oauth.server.dynamic_registration: false`. |
| `POST` | `/oauth/revoke` | RFC 7009 token revocation. Accepts `token` (raw `pb_*` value). Hashes it and calls `auth.TokenStore.Revoke`. |

### Consent UI (`/ui/oauth/*`)

These routes are part of `internal/ui` and protected by the standard `pb_session` cookie check. A
user who arrives here without a session is redirected to `/ui/login`; the original authorization
request URL is preserved in the redirect target so the flow resumes automatically after login.

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/ui/oauth/authorize` | Consent screen. Reads the pending `oauth_states` entry, displays the client name, requested scopes, and approve/deny buttons. |
| `POST` | `/ui/oauth/authorize` | Consent submission. On approval: issues an `oauth_auth_codes` record, redirects to `redirect_uri?code=...&state=...`. On denial: redirects to `redirect_uri?error=access_denied`. |

### Social Login (`/ui/auth/*`)

Unauthenticated; no `pb_session` required.

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/ui/auth/{provider}` | Initiates social login. Generates a random state, stores it in `oauth_states` with `kind=social`, redirects the browser to the provider's authorization URL. Returns 404 if the provider is disabled in config. |
| `GET` | `/ui/auth/{provider}/callback` | OAuth callback from the provider. Validates `state` against `oauth_states`, exchanges the `code` for an access token, fetches the user profile, calls `social.IdentityStore.FindOrCreate`, issues a `pb_*` bearer token, sets `pb_session` cookie, and redirects to `/ui`. |

### Existing login page (unchanged)

`GET /ui/login` and `POST /ui/login` remain as-is. The login template gains social login buttons
(links to `/ui/auth/{provider}`) only when at least one provider is enabled; the token-paste form
is always present.

---

## Flows

### Flow 1 — Social Login (Web UI)

1. User opens `/ui/login`. The server renders the login page. For each provider with `enabled: true`
   in config a "Login with GitHub" (etc.) button is shown alongside the existing token-paste form.

2. User clicks "Login with GitHub". Browser sends `GET /ui/auth/github`.

3. Handler generates 32 random bytes as the raw `state` value, computes `sha256(state)`, inserts a
   row into `oauth_states` with `kind=social`, `expires_at=now()+state_ttl`, and `payload={}`. It
   then redirects the browser to
   `https://github.com/login/oauth/authorize?client_id=...&redirect_uri=.../ui/auth/github/callback&scope=read:user+user:email&state=<raw_state>`.

4. GitHub authenticates the user (or reuses an existing GitHub session) and redirects back to
   `GET /ui/auth/github/callback?code=...&state=<raw_state>`.

5. Handler hashes the `state`, calls `StateStore.Consume`. If not found, expired, or already used,
   returns `400 Bad Request`. Marks the state row `used_at=now()`.

6. Handler calls `GitHubProvider.Exchange(ctx, code)` which POSTs to
   `https://github.com/login/oauth/access_token`, then GETs `https://api.github.com/user` and
   (if the `user:email` scope was granted) `https://api.github.com/user/emails` to obtain a verified
   primary email. Returns `UserInfo{ProviderID, Email, DisplayName, AvatarURL, RawProfile}`.

7. Handler calls `IdentityStore.FindOrCreate(ctx, "github", userInfo)`. This runs a single
   transaction:
   - `SELECT principal_id FROM social_identities WHERE provider='github' AND provider_id=<id>`
   - If found: update `email`, `display_name`, `avatar_url`, `raw_profile`, `updated_at`; return
     the existing `principal_id`.
   - If not found: `INSERT INTO principals (kind='user', slug=<email or provider_id>, ...)` then
     `INSERT INTO social_identities (...)`. Return the new `principal_id`.

8. Handler calls `auth.GenerateToken()`, then `auth.TokenStore.Create(ctx, principalID, hash, name,
   nil, allScopes, expiresAt)` where `expiresAt` is derived from `oauth.server.token_ttl` (nil if
   zero).

9. Handler sets the `pb_session` cookie (`Path=/ui`, `HttpOnly`, `SameSite=Lax`; additionally
   `Secure` when `cfg.OAuth.BaseURL` begins with `https://`) and redirects to `/ui`.

10. All subsequent Web UI requests authenticate via the existing `ui.Handler.authenticated()` path —
    no change needed there.

---

### Flow 2 — MCP Authorization Code + PKCE

1. **Discovery.** Client fetches `GET /.well-known/oauth-authorization-server`. Postbrain returns:

   ```json
   {
     "issuer": "https://postbrain.example.com",
     "authorization_endpoint": "https://postbrain.example.com/oauth/authorize",
     "token_endpoint": "https://postbrain.example.com/oauth/token",
     "registration_endpoint": "https://postbrain.example.com/oauth/register",
     "scopes_supported": [
       "memories:read", "memories:write",
       "knowledge:read", "knowledge:write",
       "skills:read", "skills:write",
       "admin"
     ],
     "response_types_supported": ["code"],
     "code_challenge_methods_supported": ["S256"]
   }
   ```

2. **Dynamic registration (first run).** Client POSTs to `/oauth/register`:

   ```json
   {
     "client_name": "Claude Desktop",
     "redirect_uris": ["http://localhost:8765/callback"],
     "grant_types": ["authorization_code"],
     "token_endpoint_auth_method": "none"
   }
   ```

   Server validates fields, generates a random `client_id` (no secret for public clients), inserts
   into `oauth_clients`, and responds `201 Created` with:

   ```json
   { "client_id": "pb_client_...", "client_name": "Claude Desktop", ... }
   ```

   The client persists `client_id` for future sessions.

3. **PKCE setup.** Client generates a cryptographically random 32-byte `code_verifier` (base64url,
   no padding) and derives `code_challenge = BASE64URL(SHA256(ASCII(code_verifier)))`.

4. **Authorization request.** Client opens a browser to:

   ```
   GET /oauth/authorize
     ?response_type=code
     &client_id=pb_client_...
     &redirect_uri=http://localhost:8765/callback
     &scope=memories:read+memories:write
     &state=<random>
     &code_challenge=<challenge>
     &code_challenge_method=S256
   ```

   Server handler:
   - Validates `client_id` against `oauth_clients`, checks `redirect_uri` is in the registered list,
     parses and validates all scopes against the known set.
   - Stores an `oauth_states` row with `kind=mcp_consent` and `payload` containing the validated
     parameters (`client_id`, `redirect_uri`, `scopes`, `code_challenge`).
   - Redirects browser to `/ui/oauth/authorize?state=<raw_state>`.

5. **Consent screen.** `/ui/oauth/authorize` checks `pb_session`. If absent, redirects to
   `/ui/login?next=/ui/oauth/authorize%3Fstate=...`. After login the browser returns here.

   The handler looks up the `oauth_states` entry, reads the payload, and renders the consent page
   showing: client name, requested scopes (human-readable labels), and Approve / Deny buttons.

6. **Consent granted.** User clicks Approve. `POST /ui/oauth/authorize` handler:
   - Re-validates the state entry (not expired, not yet used).
   - Marks the state `used_at=now()`.
   - Generates a random 32-byte `code` value; computes `code_hash=sha256(code)`.
   - Inserts into `oauth_auth_codes` with the principal ID, `client_id`, `redirect_uri`, `scopes`,
     `code_challenge`, and `expires_at=now()+auth_code_ttl`.
   - Redirects to `redirect_uri?code=<raw_code>&state=<original_state_from_payload>`.

7. **Token exchange.** Client POSTs to `/oauth/token`:

   ```
   POST /oauth/token
   Content-Type: application/x-www-form-urlencoded

   grant_type=authorization_code
   &code=<raw_code>
   &redirect_uri=http://localhost:8765/callback
   &client_id=pb_client_...
   &code_verifier=<raw_verifier>
   ```

   Server handler:
   - Hashes `code`; calls `CodeStore.Consume` which atomically sets `used_at=now()` via
     `UPDATE ... WHERE code_hash=$1 AND used_at IS NULL RETURNING ...`. Zero rows → `invalid_grant`.
   - Verifies PKCE: `BASE64URL(SHA256(code_verifier)) == code_challenge` (constant-time comparison).
   - For confidential clients, also validates `client_secret` hash.
   - Calls `token_exchange.Issue(ctx, principalID, scopes, tokenTTL)` which translates OAuth scopes
     to `permissions []string`, generates a `pb_*` token via `auth.GenerateToken()`, stores it via
     `auth.TokenStore.Create`, and returns the raw token.
   - Responds `200 OK`:

     ```json
     {
       "access_token": "pb_...",
       "token_type": "Bearer",
       "expires_in": 0,
       "scope": "memories:read memories:write"
     }
     ```

8. **MCP calls.** Client sends `Authorization: Bearer pb_...` on all subsequent requests. The
   existing `BearerTokenMiddleware` validates these without modification.

---

## Security Considerations

**PKCE is mandatory for all public clients.** The `/oauth/authorize` handler rejects any request
from a client with `is_public=true` that does not include `code_challenge` and
`code_challenge_method=S256`. The `plain` method is not supported.

**State parameter is always required and server-side verified.** The raw `state` value is never
decoded — only hashed and looked up in `oauth_states`. State entries expire (`state_ttl`, default
15 min) and are single-use, preventing both CSRF and replay attacks.

**Authorization codes are single-use and short-lived.** `used_at` is set atomically in the same
SQL statement that returns the record. TTL defaults to 10 minutes.

**Redirect URI exact-match.** `oauth_clients.redirect_uris` stores the complete registered list.
Both the authorization and token endpoints require an exact string match — no prefix, wildcard, or
query-string stripping.

**Client secrets are stored only as SHA-256 hashes.** The same pattern as `tokens.token_hash`. The
plaintext secret is returned once at registration time and never persisted.

**Provider state round-trip prevents mix-up attacks.** The social login callback validates `state`
against `oauth_states` before making any token-exchange call to the provider.

**`pb_session` cookie security.** Social login sets the same `HttpOnly`, `SameSite=Lax` cookie as
the existing token-form path. When `cfg.OAuth.BaseURL` begins with `https://` the handler
additionally sets `Secure: true`.

**Token scope enforcement.** The `permissions` array on the issued token records the exact OAuth
scopes approved by the user. Existing per-handler guards apply without modification.

**Dynamic registration rate limiting.** `/oauth/register` is rate-limited to 10 registrations per
source IP per hour using an in-memory sliding window (v1). An operator can disable dynamic
registration entirely by setting `oauth.server.dynamic_registration: false`.

**Social profile data.** `social_identities.raw_profile` stores the full provider JSON for
auditability. It MUST NOT be logged or returned in any API response.

---

## Out of Scope for v1

- **Refresh tokens.** `pb_*` tokens are long-lived with configurable expiry. Refresh token
  issuance, rotation, and storage are deferred.

- **Token introspection (RFC 7662).** No `/oauth/introspect` endpoint.

- **OpenID Connect.** No `id_token` issuance, no `/userinfo` endpoint, no JWKS document. The
  authorization server issues opaque `pb_*` bearer tokens only.

- **Implicit and Client Credentials grant types.** Only Authorization Code + PKCE is implemented.
  Machine-to-machine integrations continue to use static `pb_*` tokens.

- **Per-scope consent granularity.** The consent screen is all-or-nothing; selective scope approval
  is deferred.

- **Token self-service revocation from consent screen.** Tokens appear in the existing `/ui/tokens`
  list and can be revoked there.

- **Admin UI for OAuth clients.** Client registration is via API only.

- **Provider-specific access restrictions.** GitHub org membership checks, Google Workspace domain
  restrictions, GitLab group enforcement — all deferred. All authenticated provider identities are
  accepted unconditionally.

- **PKCE for confidential clients.** Confidential clients may rely on `client_secret` alone in v1.
