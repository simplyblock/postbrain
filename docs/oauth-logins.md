# OAuth Logins and Configuration

This page explains how to enable login and OAuth integrations in user-facing terms.

## What OAuth features Postbrain provides

Postbrain supports:

- social login for UI users (GitHub, Google, GitLab)
- OAuth authorization server endpoints for external clients and tools

## 1. Enable social login providers

Configure these keys:

- `oauth.base_url`
- `oauth.providers.github|google|gitlab.*`

Example:

```yaml
oauth:
  base_url: "https://postbrain.example.com"
  providers:
    github:
      enabled: true
      client_id: "Iv1.abc123"
      client_secret: "ghsec_..."
      scopes: [ "read:user", "user:email" ]
```

## 2. Configure provider callback URLs

In each provider's app settings, set callback URLs to:

- `https://<base>/ui/auth/github/callback`
- `https://<base>/ui/auth/google/callback`
- `https://<base>/ui/auth/gitlab/callback`

`<base>` must match `oauth.base_url`.

## 3. Configure OAuth server behavior

These control OAuth authorization-code flow behavior:

- `oauth.server.auth_code_ttl`
- `oauth.server.state_ttl`
- `oauth.server.token_ttl`
- `oauth.server.dynamic_registration`

## 4. Verify the setup

After restart, test:

1. `GET /.well-known/oauth-authorization-server`
2. `GET /ui/login` (social options should appear when provider enabled)
3. one end-to-end provider login flow

## 5. Common setup issues

- redirect URI mismatch between provider app and Postbrain callbacks
- missing or invalid client credentials
- expired OAuth state due to stale browser flow
- wrong public `oauth.base_url`

## 6. Security tips

- keep client secrets in secret managers, not source control
- keep provider scopes minimal
- prefer short auth-code/state TTLs suitable for your UX
- review audit logs for failed/denied auth flows
