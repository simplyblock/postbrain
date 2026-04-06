//go:build integration

package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

func TestCreateScopeGrant(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	q := db.New(pool)
	ctx := context.Background()

	p := testhelper.CreateTestPrincipal(t, pool, "user", "sg-create-"+uuid.New().String())
	s := testhelper.CreateTestScope(t, pool, "project", "sg-create-"+uuid.New().String(), nil, p.ID)

	grant, err := q.CreateScopeGrant(ctx, db.CreateScopeGrantParams{
		PrincipalID: p.ID,
		ScopeID:     s.ID,
		Permissions: []string{"memories:read", "knowledge:read"},
		GrantedBy:   &p.ID,
		ExpiresAt:   nil,
	})
	if err != nil {
		t.Fatalf("CreateScopeGrant: %v", err)
	}
	if grant.ID == uuid.Nil {
		t.Error("expected non-nil grant ID")
	}
	if grant.PrincipalID != p.ID {
		t.Errorf("principal_id = %v, want %v", grant.PrincipalID, p.ID)
	}
	if grant.ScopeID != s.ID {
		t.Errorf("scope_id = %v, want %v", grant.ScopeID, s.ID)
	}
	if len(grant.Permissions) != 2 {
		t.Errorf("permissions len = %d, want 2", len(grant.Permissions))
	}
}

func TestGetScopeGrant(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	q := db.New(pool)
	ctx := context.Background()

	p := testhelper.CreateTestPrincipal(t, pool, "user", "sg-get-"+uuid.New().String())
	s := testhelper.CreateTestScope(t, pool, "project", "sg-get-"+uuid.New().String(), nil, p.ID)

	_, err := q.CreateScopeGrant(ctx, db.CreateScopeGrantParams{
		PrincipalID: p.ID,
		ScopeID:     s.ID,
		Permissions: []string{"memories:read"},
	})
	if err != nil {
		t.Fatalf("CreateScopeGrant: %v", err)
	}

	got, err := q.GetScopeGrant(ctx, db.GetScopeGrantParams{
		PrincipalID: p.ID,
		ScopeID:     s.ID,
	})
	if err != nil {
		t.Fatalf("GetScopeGrant: %v", err)
	}
	if got.PrincipalID != p.ID || got.ScopeID != s.ID {
		t.Error("GetScopeGrant returned wrong grant")
	}

	// Non-existent grant returns ErrNoRows
	other := testhelper.CreateTestPrincipal(t, pool, "user", "sg-get-other-"+uuid.New().String())
	_, err = q.GetScopeGrant(ctx, db.GetScopeGrantParams{
		PrincipalID: other.ID,
		ScopeID:     s.ID,
	})
	if err != pgx.ErrNoRows {
		t.Errorf("expected pgx.ErrNoRows for missing grant, got %v", err)
	}
}

func TestListScopeGrantsByPrincipal(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	q := db.New(pool)
	ctx := context.Background()

	p := testhelper.CreateTestPrincipal(t, pool, "user", "sg-list-p-"+uuid.New().String())
	s1 := testhelper.CreateTestScope(t, pool, "project", "sg-list-s1-"+uuid.New().String(), nil, p.ID)
	s2 := testhelper.CreateTestScope(t, pool, "project", "sg-list-s2-"+uuid.New().String(), nil, p.ID)

	for _, sid := range []uuid.UUID{s1.ID, s2.ID} {
		if _, err := q.CreateScopeGrant(ctx, db.CreateScopeGrantParams{
			PrincipalID: p.ID,
			ScopeID:     sid,
			Permissions: []string{"memories:read"},
		}); err != nil {
			t.Fatalf("CreateScopeGrant: %v", err)
		}
	}

	grants, err := q.ListScopeGrantsByPrincipal(ctx, p.ID)
	if err != nil {
		t.Fatalf("ListScopeGrantsByPrincipal: %v", err)
	}
	if len(grants) != 2 {
		t.Errorf("expected 2 grants, got %d", len(grants))
	}
}

func TestListScopeGrantsByPrincipal_ExcludesExpired(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	q := db.New(pool)
	ctx := context.Background()

	p := testhelper.CreateTestPrincipal(t, pool, "user", "sg-expired-"+uuid.New().String())
	s := testhelper.CreateTestScope(t, pool, "project", "sg-expired-"+uuid.New().String(), nil, p.ID)

	past := time.Now().Add(-time.Hour)
	if _, err := q.CreateScopeGrant(ctx, db.CreateScopeGrantParams{
		PrincipalID: p.ID,
		ScopeID:     s.ID,
		Permissions: []string{"memories:read"},
		ExpiresAt:   &past,
	}); err != nil {
		t.Fatalf("CreateScopeGrant (expired): %v", err)
	}

	grants, err := q.ListScopeGrantsByPrincipal(ctx, p.ID)
	if err != nil {
		t.Fatalf("ListScopeGrantsByPrincipal: %v", err)
	}
	if len(grants) != 0 {
		t.Errorf("expected 0 grants (all expired), got %d", len(grants))
	}
}

func TestListScopeGrantsByScope(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	q := db.New(pool)
	ctx := context.Background()

	owner := testhelper.CreateTestPrincipal(t, pool, "user", "sg-bysc-owner-"+uuid.New().String())
	s := testhelper.CreateTestScope(t, pool, "project", "sg-bysc-s-"+uuid.New().String(), nil, owner.ID)

	p1 := testhelper.CreateTestPrincipal(t, pool, "user", "sg-bysc-p1-"+uuid.New().String())
	p2 := testhelper.CreateTestPrincipal(t, pool, "user", "sg-bysc-p2-"+uuid.New().String())

	for _, pid := range []uuid.UUID{p1.ID, p2.ID} {
		if _, err := q.CreateScopeGrant(ctx, db.CreateScopeGrantParams{
			PrincipalID: pid,
			ScopeID:     s.ID,
			Permissions: []string{"memories:read"},
		}); err != nil {
			t.Fatalf("CreateScopeGrant: %v", err)
		}
	}

	grants, err := q.ListScopeGrantsByScope(ctx, s.ID)
	if err != nil {
		t.Fatalf("ListScopeGrantsByScope: %v", err)
	}
	if len(grants) != 2 {
		t.Errorf("expected 2 grants, got %d", len(grants))
	}
}

func TestUpdateScopeGrantPermissions(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	q := db.New(pool)
	ctx := context.Background()

	p := testhelper.CreateTestPrincipal(t, pool, "user", "sg-update-"+uuid.New().String())
	s := testhelper.CreateTestScope(t, pool, "project", "sg-update-"+uuid.New().String(), nil, p.ID)

	if _, err := q.CreateScopeGrant(ctx, db.CreateScopeGrantParams{
		PrincipalID: p.ID,
		ScopeID:     s.ID,
		Permissions: []string{"memories:read"},
	}); err != nil {
		t.Fatalf("CreateScopeGrant: %v", err)
	}

	updated, err := q.UpdateScopeGrantPermissions(ctx, db.UpdateScopeGrantPermissionsParams{
		PrincipalID: p.ID,
		ScopeID:     s.ID,
		Permissions: []string{"memories:read", "memories:write"},
	})
	if err != nil {
		t.Fatalf("UpdateScopeGrantPermissions: %v", err)
	}
	if len(updated.Permissions) != 2 {
		t.Errorf("expected 2 permissions after update, got %d", len(updated.Permissions))
	}
}

func TestDeleteScopeGrant(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	q := db.New(pool)
	ctx := context.Background()

	p := testhelper.CreateTestPrincipal(t, pool, "user", "sg-delete-"+uuid.New().String())
	s := testhelper.CreateTestScope(t, pool, "project", "sg-delete-"+uuid.New().String(), nil, p.ID)

	grant, err := q.CreateScopeGrant(ctx, db.CreateScopeGrantParams{
		PrincipalID: p.ID,
		ScopeID:     s.ID,
		Permissions: []string{"memories:read"},
	})
	if err != nil {
		t.Fatalf("CreateScopeGrant: %v", err)
	}

	if err := q.DeleteScopeGrant(ctx, grant.ID); err != nil {
		t.Fatalf("DeleteScopeGrant: %v", err)
	}

	_, err = q.GetScopeGrant(ctx, db.GetScopeGrantParams{PrincipalID: p.ID, ScopeID: s.ID})
	if err != pgx.ErrNoRows {
		t.Errorf("expected ErrNoRows after delete, got %v", err)
	}
}

func TestDeleteExpiredScopeGrants(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	q := db.New(pool)
	ctx := context.Background()

	p := testhelper.CreateTestPrincipal(t, pool, "user", "sg-delexp-"+uuid.New().String())
	s1 := testhelper.CreateTestScope(t, pool, "project", "sg-delexp-s1-"+uuid.New().String(), nil, p.ID)
	s2 := testhelper.CreateTestScope(t, pool, "project", "sg-delexp-s2-"+uuid.New().String(), nil, p.ID)

	past := time.Now().Add(-time.Hour)
	future := time.Now().Add(time.Hour)

	// s1: expired, s2: valid
	if _, err := q.CreateScopeGrant(ctx, db.CreateScopeGrantParams{
		PrincipalID: p.ID, ScopeID: s1.ID,
		Permissions: []string{"memories:read"}, ExpiresAt: &past,
	}); err != nil {
		t.Fatalf("create expired grant: %v", err)
	}
	if _, err := q.CreateScopeGrant(ctx, db.CreateScopeGrantParams{
		PrincipalID: p.ID, ScopeID: s2.ID,
		Permissions: []string{"memories:read"}, ExpiresAt: &future,
	}); err != nil {
		t.Fatalf("create valid grant: %v", err)
	}

	if err := q.DeleteExpiredScopeGrants(ctx); err != nil {
		t.Fatalf("DeleteExpiredScopeGrants: %v", err)
	}

	// s1 grant should be gone
	_, err := q.GetScopeGrant(ctx, db.GetScopeGrantParams{PrincipalID: p.ID, ScopeID: s1.ID})
	if err != pgx.ErrNoRows {
		t.Errorf("expected expired grant deleted, got %v", err)
	}

	// s2 grant should remain
	_, err = q.GetScopeGrant(ctx, db.GetScopeGrantParams{PrincipalID: p.ID, ScopeID: s2.ID})
	if err != nil {
		t.Errorf("valid grant should still exist, got %v", err)
	}
}
