//go:build integration

package db_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

func sha256Hex(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func TestOAuthConsumeCodeSingleUseAndExpiry(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	q := db.New(pool)
	ctx := context.Background()

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "oauth-code-user-"+time.Now().Format("150405.000000"))

	var clientRowID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO oauth_clients (client_id, name, redirect_uris, is_public)
		VALUES ('pb_client_code_test', 'Code Test Client', ARRAY['http://localhost/callback'], true)
		RETURNING id
	`).Scan(&clientRowID); err != nil {
		t.Fatalf("insert oauth client: %v", err)
	}

	t.Run("consume once", func(t *testing.T) {
		rawCode := "raw-auth-code-1"
		if _, err := pool.Exec(ctx, `
			INSERT INTO oauth_auth_codes (code_hash, client_id, principal_id, redirect_uri, scopes, code_challenge, expires_at)
			VALUES ($1, $2::uuid, $3::uuid, 'http://localhost/callback', ARRAY['memories:read'], 'challenge', now() + interval '10 minutes')
		`, sha256Hex(rawCode), clientRowID, principal.ID); err != nil {
			t.Fatalf("insert auth code: %v", err)
		}

		if _, err := q.ConsumeCode(ctx, sha256Hex(rawCode)); err != nil {
			t.Fatalf("ConsumeCode first call: %v", err)
		}
		if _, err := q.ConsumeCode(ctx, sha256Hex(rawCode)); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("ConsumeCode second call error = %v, want pgx.ErrNoRows", err)
		}
	})

	t.Run("expired rejected", func(t *testing.T) {
		rawCode := "raw-auth-code-expired"
		if _, err := pool.Exec(ctx, `
			INSERT INTO oauth_auth_codes (code_hash, client_id, principal_id, redirect_uri, scopes, code_challenge, expires_at)
			VALUES ($1, $2::uuid, $3::uuid, 'http://localhost/callback', ARRAY['memories:read'], 'challenge', now() - interval '1 minute')
		`, sha256Hex(rawCode), clientRowID, principal.ID); err != nil {
			t.Fatalf("insert expired auth code: %v", err)
		}

		if _, err := q.ConsumeCode(ctx, sha256Hex(rawCode)); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("ConsumeCode expired call error = %v, want pgx.ErrNoRows", err)
		}
	})
}

func TestOAuthConsumeStateSingleUse(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	q := db.New(pool)
	ctx := context.Background()

	rawState := "raw-oauth-state-1"
	if _, err := pool.Exec(ctx, `
		INSERT INTO oauth_states (state_hash, kind, payload, expires_at)
		VALUES ($1, 'social', '{}'::jsonb, now() + interval '10 minutes')
	`, sha256Hex(rawState)); err != nil {
		t.Fatalf("insert oauth state: %v", err)
	}

	if _, err := q.ConsumeState(ctx, sha256Hex(rawState)); err != nil {
		t.Fatalf("ConsumeState first call: %v", err)
	}
	if _, err := q.ConsumeState(ctx, sha256Hex(rawState)); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("ConsumeState second call error = %v, want pgx.ErrNoRows", err)
	}
}

func TestOAuthLookupClientExcludesRevoked(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	q := db.New(pool)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `
		INSERT INTO oauth_clients (client_id, name, redirect_uris, is_public)
		VALUES ('pb_client_active', 'Active Client', ARRAY['http://localhost/callback'], true)
	`); err != nil {
		t.Fatalf("insert active client: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO oauth_clients (client_id, name, redirect_uris, is_public, revoked_at)
		VALUES ('pb_client_revoked', 'Revoked Client', ARRAY['http://localhost/callback'], true, now())
	`); err != nil {
		t.Fatalf("insert revoked client: %v", err)
	}

	if _, err := q.LookupClient(ctx, "pb_client_active"); err != nil {
		t.Fatalf("LookupClient active: %v", err)
	}

	if _, err := q.LookupClient(ctx, "pb_client_revoked"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("LookupClient revoked error = %v, want pgx.ErrNoRows", err)
	}
}
