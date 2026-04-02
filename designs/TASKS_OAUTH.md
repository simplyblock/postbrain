# Postbrain — OAuth Implementation Tasks

All tasks follow strict TDD: failing test written first, then implementation. See `designs/DESIGN_OAUTH.md` for the full specification.

---

## Phase 1 — Database & Configuration

### Migration

- [x] `internal/db/migrations/000011_oauth.up.sql` — create four tables:
  - `social_identities (id, principal_id FK, provider, provider_id, email citext, display_name, avatar_url, raw_profile JSONB, created_at, updated_at)` + `UNIQUE(provider, provider_id)` + `touch_updated_at` trigger
  - `oauth_clients (id, client_id TEXT UNIQUE, client_secret_hash, name, redirect_uris TEXT[], grant_types TEXT[], scopes TEXT[], is_public, meta JSONB, created_at, revoked_at)`
  - `oauth_auth_codes (id, code_hash TEXT UNIQUE, client_id FK, principal_id FK, redirect_uri, scopes TEXT[], code_challenge, expires_at, used_at, created_at)`
  - `oauth_states (id, state_hash TEXT UNIQUE, kind CHECK IN ('social','mcp_consent'), payload JSONB, expires_at, used_at, created_at)`
  - All indexes per design
- [x] `internal/db/migrations/000011_oauth.down.sql` — drop all four tables in reverse FK order
- [x] `internal/db/migrate_test.go` — extend migration integration test: verify all four tables exist and constraints hold after up; verify clean rollback on down

### Configuration

- [x] `internal/config/config.go` — add `OAuthConfig`, `ProviderConfig`, `OAuthServerConfig` structs; add `OAuth OAuthConfig` field to `Config`; add four `SetDefault` calls; wire `mapstructure` tags
- [x] `internal/config/config_test.go` — add tests: OAuth defaults load correctly; enabled provider fields round-trip from YAML; `auth_code_ttl` parses as `time.Duration`
- [x] `config.example.yaml` — add `oauth:` block with all keys documented

---

## Phase 2 — Database Query Layer

### sqlc query files (`internal/db/queries/`)

- [x] `oauth_clients.sql`:
  - `RegisterClient(client_id, client_secret_hash, name, redirect_uris, grant_types, scopes, is_public, meta)` — INSERT RETURNING
  - `LookupClient(client_id TEXT)` — SELECT WHERE revoked_at IS NULL
  - `RevokeClient(id UUID)` — UPDATE SET revoked_at=now()

- [x] `oauth_codes.sql`:
  - `IssueCode(code_hash, client_id, principal_id, redirect_uri, scopes, code_challenge, expires_at)` — INSERT RETURNING
  - `ConsumeCode(code_hash TEXT)` — `UPDATE oauth_auth_codes SET used_at=now() WHERE code_hash=$1 AND used_at IS NULL AND expires_at > now() RETURNING *` (atomic single-use)

- [x] `oauth_states.sql`:
  - `IssueState(state_hash, kind, payload, expires_at)` — INSERT RETURNING
  - `ConsumeState(state_hash TEXT)` — `UPDATE oauth_states SET used_at=now() WHERE state_hash=$1 AND used_at IS NULL AND expires_at > now() RETURNING *`

- [x] `social_identities.sql`:
  - `UpsertSocialIdentity(principal_id, provider, provider_id, email, display_name, avatar_url, raw_profile)` — `INSERT ... ON CONFLICT (provider, provider_id) DO UPDATE SET email=..., display_name=..., avatar_url=..., raw_profile=..., updated_at=now() RETURNING *`
  - `FindPrincipalBySocialIdentity(provider, provider_id TEXT)` — SELECT principal_id

- [x] Run `sqlc generate` and verify generated files compile
- [x] `internal/db/queries_test.go` (integration, build tag `integration`) — at minimum: `ConsumeCode` returns zero rows on second call; `ConsumeState` returns zero rows on second call; expired code is rejected; `LookupClient` excludes revoked clients

---

## Phase 3 — `internal/oauth` Package (Authorization Server Core)

### `internal/oauth/pkce.go` + test

- [x] `pkce_test.go` (failing first):
  - `TestVerifyS256_ValidPair_ReturnsTrue`
  - `TestVerifyS256_WrongVerifier_ReturnsFalse`
  - `TestVerifyS256_EmptyInputs_ReturnsFalse`
  - `TestGenerateChallenge_DeterministicForSameVerifier`
- [x] `pkce.go`:
  - `GenerateChallenge(verifier string) string` — `base64.RawURLEncoding.EncodeToString(sha256sum(verifier))`
  - `VerifyS256(verifier, challenge string) bool` — constant-time compare via `subtle.ConstantTimeCompare`

### `internal/oauth/scopes.go` + test

- [x] `scopes_test.go` (failing first):
  - `TestParseScopes_ValidSingle_OK`
  - `TestParseScopes_ValidMultiple_OK`
  - `TestParseScopes_UnknownScope_ReturnsError`
  - `TestParseScopes_Empty_ReturnsError`
  - `TestScopeToPermissions_MapsCorrectly`
- [x] `scopes.go`:
  - Constants: `ScopeMemoriesRead = "memories:read"`, `ScopeMemoriesWrite`, `ScopeKnowledgeRead`, `ScopeKnowledgeWrite`, `ScopeSkillsRead`, `ScopeSkillsWrite`, `ScopeAdmin`
  - `ParseScopes(raw string) ([]string, error)` — splits on space, validates each against known set
  - `ScopeToPermissions(scopes []string) []string` — identity mapping for now (scope strings are permission strings)

### `internal/oauth/states.go` + test

- [x] `states_test.go` (failing first):
  - `TestStateStore_Issue_RoundTrip` — issue then consume returns payload
  - `TestStateStore_Consume_SecondCall_ReturnsNotFound` — single-use enforced
  - `TestStateStore_Consume_Expired_ReturnsNotFound`
  - `TestStateStore_Issue_HashesRawState` — raw value is never stored
- [x] `states.go`:
  - `StateStore{pool}`, `NewStateStore(pool)`
  - `Issue(ctx, kind, payload map[string]any, ttl time.Duration) (rawState string, err error)` — generates 32 random bytes, hashes, calls `db.IssueState`
  - `Consume(ctx, rawState string) (*StateRecord, error)` — hashes, calls `db.ConsumeState`; returns `ErrNotFound` if zero rows

### `internal/oauth/clients.go` + test

- [x] `clients_test.go` (failing first):
  - `TestClientStore_Register_PublicClient_NoSecret`
  - `TestClientStore_Register_ConfidentialClient_HashesSecret`
  - `TestClientStore_LookupByClientID_Found`
  - `TestClientStore_LookupByClientID_Revoked_ReturnsNil`
  - `TestClientStore_ValidateRedirectURI_ExactMatch_OK`
  - `TestClientStore_ValidateRedirectURI_NoMatch_ReturnsError`
- [x] `clients.go`:
  - `ClientStore{pool}`, `NewClientStore(pool)`
  - `Register(ctx, req RegisterRequest) (*OAuthClient, rawSecret string, err error)` — generates `pb_client_<hex>` client_id; for confidential clients generates + hashes secret
  - `LookupByClientID(ctx, clientID string) (*OAuthClient, error)`
  - `ValidateRedirectURI(client *OAuthClient, redirectURI string) error` — exact string match against `client.RedirectURIs`
  - `Revoke(ctx, id uuid.UUID) error`

### `internal/oauth/codes.go` + test

- [x] `codes_test.go` (failing first):
  - `TestCodeStore_Issue_StoresHash_NotRaw`
  - `TestCodeStore_Consume_ValidCode_ReturnsRecord`
  - `TestCodeStore_Consume_AlreadyUsed_ReturnsError`
  - `TestCodeStore_Consume_Expired_ReturnsError`
  - `TestCodeStore_VerifyPKCE_ValidVerifier_OK`
  - `TestCodeStore_VerifyPKCE_InvalidVerifier_ReturnsError`
- [x] `codes.go`:
  - `CodeStore{pool}`, `NewCodeStore(pool)`
  - `Issue(ctx, req IssueCodeRequest) (rawCode string, err error)` — generates 32 random bytes, hashes, calls `db.IssueCode`
  - `Consume(ctx, rawCode string) (*AuthCode, error)` — hashes, calls `db.ConsumeCode`; `ErrNotFound` / `ErrCodeExpired` / `ErrCodeUsed` on failure
  - `VerifyPKCE(code *AuthCode, verifier string) error` — delegates to `pkce.VerifyS256`

### `internal/oauth/token_exchange.go` + test

- [x] `token_exchange_test.go` (failing first):
  - `TestIssue_TranslatesScopes_ToPermissions`
  - `TestIssue_CreatesTokenWithCorrectPrincipal`
  - `TestIssue_ZeroTTL_CreatesNonExpiringToken`
  - `TestIssue_NonZeroTTL_SetsExpiresAt`
- [x] `token_exchange.go`:
  - `Issuer{tokenStore *auth.TokenStore}`, `NewIssuer(tokenStore)`
  - `Issue(ctx, principalID uuid.UUID, scopes []string, ttl time.Duration) (rawToken string, err error)` — calls `auth.GenerateToken()`, `tokenStore.Create(...)`

### `internal/oauth/metadata.go` + test

- [x] `metadata_test.go` (failing first):
  - `TestServerMetadata_ContainsRequiredFields`
  - `TestServerMetadata_BaseURL_UsedForEndpoints`
  - `TestServerMetadata_ScopesListed`
- [x] `metadata.go`:
  - `ServerMetadata(baseURL string) map[string]any` — returns RFC 8414 JSON-serialisable map

### `internal/oauth/server.go`

- [x] `server.go`:
  - `Server{clients, codes, states, issuer, cfg}`, `NewServer(...)`
  - `HandleMetadata(w, r)` — `GET /.well-known/oauth-authorization-server` (RFC 8414 required fields)
  - `HandleAuthorize(w, r)` — `GET /oauth/authorize`: enforce `response_type=code`, `state`, `client_id`, exact `redirect_uri`, known `scope`, and PKCE requirements; store validated payload in `oauth_states`, redirect to consent
  - `HandleToken(w, r)` — `POST /oauth/token`: enforce `grant_type=authorization_code`, consume code, enforce exact `redirect_uri` + `client_id` match to code record, verify PKCE, verify `client_secret` for confidential clients, issue token
  - `HandleRegister(w, r)` — `POST /oauth/register`: gated by `cfg.DynamicRegistration`; rate-limit 10/IP/hour (in-memory sliding window)
  - `HandleRevoke(w, r)` — `POST /oauth/revoke` (RFC 7009 style; always 200 for known/unknown token)
- [x] `server_test.go` (failing first):
  - `TestHandleMetadata_Returns200_WithRequiredFields`
  - `TestHandleAuthorize_MissingClientID_Returns400`
  - `TestHandleAuthorize_MissingState_Returns400`
  - `TestHandleAuthorize_InvalidResponseType_Returns400`
  - `TestHandleAuthorize_UnknownClientID_Returns400`
  - `TestHandleAuthorize_BadRedirectURI_Returns400`
  - `TestHandleAuthorize_UnknownScope_Returns400`
  - `TestHandleAuthorize_MissingCodeChallenge_PublicClient_Returns400`
  - `TestHandleAuthorize_PlainCodeChallengeMethod_Returns400`
  - `TestHandleAuthorize_ValidRequest_RedirectsToConsentUI`
  - `TestHandleAuthorize_StoresValidatedPayloadInState`
  - `TestHandleToken_MissingCode_Returns400`
  - `TestHandleToken_MissingGrantType_Returns400`
  - `TestHandleToken_InvalidCode_Returns400`
  - `TestHandleToken_RedirectURIMismatch_Returns400`
  - `TestHandleToken_ClientIDMismatch_Returns400`
  - `TestHandleToken_PKCEMismatch_Returns400`
  - `TestHandleToken_ConfidentialClient_MissingSecret_Returns401`
  - `TestHandleToken_ConfidentialClient_WrongSecret_Returns401`
  - `TestHandleToken_ValidRequest_Returns200_WithToken`
  - `TestHandleToken_ReplayCode_Returns400`
  - `TestHandleRegister_ValidPublicClient_Returns201`
  - `TestHandleRegister_ConfidentialClient_ReturnsClientSecretOnce`
  - `TestHandleRegister_RateLimitExceeded_Returns429`
  - `TestHandleRegister_DynamicRegistrationDisabled_Returns404`
  - `TestHandleRevoke_ValidToken_Returns200`
  - `TestHandleRevoke_UnknownToken_Returns200`

---

## Phase 4 — `internal/social` Package (Social Login)

### `internal/social/provider.go`

- [ ] `provider.go`:
  - `UserInfo{ProviderID, Email, DisplayName, AvatarURL string, RawProfile []byte}`
  - `Provider` interface: `AuthURL(state string) string`, `Exchange(ctx context.Context, code string) (*UserInfo, error)`

### `internal/social/pkce.go` (state generation helper)

- [ ] Reuse `internal/oauth` state helpers; no separate file needed — social handlers call `oauth.StateStore` directly.

### `internal/social/github.go` + test

- [ ] `github_test.go` (failing first) — use `httptest.Server` to mock GitHub API:
  - `TestGitHubProvider_AuthURL_ContainsClientIDAndState`
  - `TestGitHubProvider_Exchange_ValidCode_ReturnsUserInfo`
  - `TestGitHubProvider_Exchange_APIError_ReturnsError`
  - `TestGitHubProvider_Exchange_UsesVerifiedPrimaryEmail`
- [ ] `github.go`:
  - `GitHubProvider{clientID, clientSecret string, scopes []string, httpClient *http.Client}`
  - `NewGitHubProvider(cfg config.ProviderConfig) *GitHubProvider`
  - `AuthURL(state string) string` — builds GitHub authorize URL
  - `Exchange(ctx, code string) (*UserInfo, error)` — POST to token endpoint, GET /user, GET /user/emails

### `internal/social/google.go` + test

- [ ] `google_test.go` (failing first) — mock OIDC discovery and token endpoint:
  - `TestGoogleProvider_AuthURL_ContainsClientIDAndState`
  - `TestGoogleProvider_Exchange_ValidIDToken_ReturnsUserInfo`
  - `TestGoogleProvider_Exchange_InvalidIDToken_ReturnsError`
- [ ] `google.go`:
  - `GoogleProvider{clientID, clientSecret string, scopes []string, httpClient *http.Client}`
  - `NewGoogleProvider(cfg config.ProviderConfig) *GoogleProvider`
  - `AuthURL(state string) string`
  - `Exchange(ctx, code string) (*UserInfo, error)` — exchanges code, parses `id_token` JWT claims (no signature verification in v1 — token comes directly from Google over TLS)

### `internal/social/gitlab.go` + test

- [ ] `gitlab_test.go` (failing first) — mock GitLab API:
  - `TestGitLabProvider_AuthURL_UsesInstanceURL`
  - `TestGitLabProvider_Exchange_ValidCode_ReturnsUserInfo`
- [ ] `gitlab.go`:
  - `GitLabProvider{instanceURL, clientID, clientSecret string, scopes []string, httpClient *http.Client}`
  - `NewGitLabProvider(cfg config.ProviderConfig) *GitLabProvider`
  - `AuthURL(state string) string` — uses `instanceURL`
  - `Exchange(ctx, code string) (*UserInfo, error)` — calls `{instanceURL}/oauth/token` then `{instanceURL}/api/v4/user`

### `internal/social/registry.go` + test

- [ ] `registry_test.go` (failing first):
  - `TestNewRegistry_OnlyEnabledProviders_Included`
  - `TestNewRegistry_DisabledProvider_Excluded`
  - `TestNewRegistry_UnknownProvider_Ignored`
- [ ] `registry.go`:
  - `NewRegistry(cfg config.OAuthConfig) map[string]Provider` — constructs providers for `github`, `google`, `gitlab` if `enabled: true`

### `internal/social/identity.go` + test

- [ ] `identity_test.go` (failing first, integration tag):
  - `TestIdentityStore_FindOrCreate_NewIdentity_CreatesPrincipalAndIdentity`
  - `TestIdentityStore_FindOrCreate_ExistingIdentity_UpdatesProfile_ReturnsSamePrincipal`
  - `TestIdentityStore_FindOrCreate_SlugCollision_AppendsProviderID`
- [ ] `identity.go`:
  - `IdentityStore{pool}`, `NewIdentityStore(pool)`
  - `FindOrCreate(ctx, provider string, info *UserInfo) (*db.Principal, error)` — single serializable transaction: lookup → update or create principal + identity

---

## Phase 5 — Web UI Handlers

### `internal/ui/oauth_social.go` + tests

- [ ] `oauth_social_test.go` (failing first):
  - `TestHandleSocialStart_UnknownProvider_Returns404`
  - `TestHandleSocialStart_DisabledProvider_Returns404`
  - `TestHandleSocialStart_ValidProvider_RedirectsToProviderURL`
  - `TestHandleSocialStart_SetsStateInDB`
  - `TestHandleSocialCallback_InvalidState_Returns400`
  - `TestHandleSocialCallback_ExpiredState_Returns400`
  - `TestHandleSocialCallback_ReplayState_Returns400`
  - `TestHandleSocialCallback_ProviderExchangeError_Returns502`
  - `TestHandleSocialCallback_Success_SetsCookie_RedirectsToUI`
- [ ] `oauth_social.go`:
  - `handleSocialStart(w, r)` — dispatched by provider name from URL path; creates state, redirects
  - `handleSocialCallback(w, r)` — validates state, exchanges code, calls `IdentityStore.FindOrCreate`, issues token, sets `pb_session` cookie
  - Set `Secure` on cookie when `cfg.OAuth.BaseURL` starts with `https://`

### `internal/ui/oauth_consent.go` + tests

- [ ] `oauth_consent_test.go` (failing first):
  - `TestHandleConsentGet_NoSession_RedirectsToLogin_WithNext`
  - `TestHandleConsentGet_InvalidState_Returns400`
  - `TestHandleConsentGet_ValidState_RendersConsentPage`
  - `TestHandleConsentPost_NoSession_RedirectsToLogin`
  - `TestHandleConsentPost_Deny_RedirectsToClientWithErrorAndOriginalState`
  - `TestHandleConsentPost_Approve_IssuedCode_RedirectsToClientWithCodeAndOriginalState`
  - `TestHandleConsentPost_ExpiredState_Returns400`
  - `TestHandleConsentPost_ReplayState_Returns400`
- [ ] `oauth_consent.go`:
  - `handleConsentGet(w, r)` — auth-guarded; renders `oauth_consent` template with client name and human-readable scope labels
  - `handleConsentPost(w, r)` — auth-guarded; consume state once; on approve: issue code, redirect to `redirect_uri` with `code` and original client `state`; on deny: redirect with `error=access_denied` and original client `state`

### Templates

- [ ] `internal/ui/web/templates/oauth_consent.html` — consent page: client name, scope list, Approve / Deny buttons
- [ ] `internal/ui/web/templates/login.html` — extend to show `<a href="/ui/auth/{provider}">Login with GitHub</a>` etc. for each enabled provider (passed in template data as `Providers []string`)

### Router wiring (`internal/ui/handler.go`)

- [ ] Add routes to `ServeHTTP`:
  - `strings.HasPrefix(path, "/ui/auth/") && method == GET` → dispatch to `handleSocialStart` or `handleSocialCallback` based on `strings.HasSuffix(path, "/callback")`
  - `path == "/ui/oauth/authorize" && method == GET` → `handleConsentGet`
  - `path == "/ui/oauth/authorize" && method == POST` → `handleConsentPost`
- [ ] Update `handleLogin` (GET) to pass `Providers []string` (enabled provider names) to template data
- [ ] Update `NewHandler` signature or config injection to receive `cfg config.OAuthConfig`, `providers map[string]social.Provider`, `stateStore *oauth.StateStore`, `codeStore *oauth.CodeStore`, `issuer *oauth.Issuer`

---

## Phase 6 — OAuth Server Route Registration

### `cmd/postbrain/main.go`

- [ ] Instantiate all OAuth/social dependencies in `runServe`: `StateStore`, `ClientStore`, `CodeStore`, `Issuer`, `social.Registry`, `IdentityStore`, `oauth.Server`
- [ ] Register on root mux:
  - `mux.HandleFunc("GET /.well-known/oauth-authorization-server", oauthServer.HandleMetadata)`
  - `mux.HandleFunc("GET /oauth/authorize", oauthServer.HandleAuthorize)`
  - `mux.HandleFunc("POST /oauth/token", oauthServer.HandleToken)`
  - `mux.HandleFunc("POST /oauth/register", oauthServer.HandleRegister)`
  - `mux.HandleFunc("POST /oauth/revoke", oauthServer.HandleRevoke)`
- [ ] Pass updated dependencies to `ui.NewHandler`

---

## Phase 7 — Integration & E2E Tests

### Social login integration test (`internal/social/identity_integration_test.go`, build tag `integration`)

Already covered in Phase 4 identity tests above.

### OAuth server integration test (`internal/oauth/server_integration_test.go`, build tag `integration`)

- [ ] Full Authorization Code + PKCE round-trip against a real DB:
  - Register client → authorize → consent → token exchange → verify `pb_*` token works on `/v1/memories/recall`
  - Replay attack: same code → `invalid_grant`
  - PKCE mismatch: wrong verifier → `invalid_grant`
  - Redirect URI mismatch on token exchange → `invalid_grant`
  - Confidential client with bad secret → `invalid_client`
  - Dynamic registration disabled in config → `/oauth/register` rejects

### Social login E2E test (`internal/ui/oauth_social_integration_test.go`, build tag `integration`)

- [ ] Mock provider server (httptest) + real DB:
  - Full flow: `GET /ui/auth/github` → mock GitHub → `GET /ui/auth/github/callback` → cookie set → `GET /ui` returns 200

---

## Phase 8 — Housekeeping

- [ ] `go.mod` / `go.sum` — no new external dependencies needed (all crypto is stdlib; HTTP calls use stdlib `net/http`)
- [ ] `config.example.yaml` — add `oauth:` block (already listed in Phase 1, verify it's complete)
- [ ] `TASKS.md` — add OAuth implementation section once all tasks complete
- [ ] Verify logs and API responses never include `social_identities.raw_profile` content
- [ ] Update `designs/DESIGN_OAUTH.md` if any decisions change during implementation
