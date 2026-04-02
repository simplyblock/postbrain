-- name: UpsertSocialIdentity :one
INSERT INTO social_identities (
    principal_id,
    provider,
    provider_id,
    email,
    display_name,
    avatar_url,
    raw_profile
)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (provider, provider_id)
DO UPDATE SET
    email = EXCLUDED.email,
    display_name = EXCLUDED.display_name,
    avatar_url = EXCLUDED.avatar_url,
    raw_profile = EXCLUDED.raw_profile,
    updated_at = now()
RETURNING id, principal_id, provider, provider_id, email, display_name, avatar_url, raw_profile, created_at, updated_at;

-- name: FindPrincipalBySocialIdentity :one
SELECT principal_id
FROM social_identities
WHERE provider = $1
  AND provider_id = $2;
