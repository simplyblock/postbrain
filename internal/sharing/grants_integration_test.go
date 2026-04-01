//go:build integration

package sharing_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/sharing"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

func TestGrants_MemoryGrant_RoundTrip(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()
	s := sharing.NewStore(pool)

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "sharing-mem-"+uuid.New().String())
	granter := testhelper.CreateTestScope(t, pool, "project", "sharing-granter-"+uuid.New().String(), nil, principal.ID)
	grantee := testhelper.CreateTestScope(t, pool, "project", "sharing-grantee-"+uuid.New().String(), nil, principal.ID)
	mem := testhelper.CreateTestMemory(t, pool, granter.ID, principal.ID, "sharing grant test memory")

	memID := mem.ID
	g, err := s.Create(ctx, &sharing.Grant{
		MemoryID:       &memID,
		GranteeScopeID: grantee.ID,
		GrantedBy:      principal.ID,
		CanReshare:     false,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if g.ID == (uuid.UUID{}) {
		t.Error("expected non-zero grant ID")
	}
	if g.MemoryID == nil || *g.MemoryID != memID {
		t.Errorf("MemoryID = %v; want %v", g.MemoryID, memID)
	}
}

func TestGrants_ArtifactGrant_RoundTrip(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()
	s := sharing.NewStore(pool)

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "sharing-art-"+uuid.New().String())
	granter := testhelper.CreateTestScope(t, pool, "project", "sharing-art-granter-"+uuid.New().String(), nil, principal.ID)
	grantee := testhelper.CreateTestScope(t, pool, "project", "sharing-art-grantee-"+uuid.New().String(), nil, principal.ID)
	testhelper.CreateTestEmbeddingModel(t, pool)
	artifact := testhelper.CreateTestArtifact(t, pool, granter.ID, principal.ID, "sharing artifact")

	artID := artifact.ID
	g, err := s.Create(ctx, &sharing.Grant{
		ArtifactID:     &artID,
		GranteeScopeID: grantee.ID,
		GrantedBy:      principal.ID,
		CanReshare:     true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if g.ArtifactID == nil || *g.ArtifactID != artID {
		t.Errorf("ArtifactID = %v; want %v", g.ArtifactID, artID)
	}
	if !g.CanReshare {
		t.Error("expected CanReshare=true")
	}
}

func TestGrants_Revoke(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()
	s := sharing.NewStore(pool)

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "sharing-revoke-"+uuid.New().String())
	granter := testhelper.CreateTestScope(t, pool, "project", "sharing-rev-granter-"+uuid.New().String(), nil, principal.ID)
	grantee := testhelper.CreateTestScope(t, pool, "project", "sharing-rev-grantee-"+uuid.New().String(), nil, principal.ID)
	mem := testhelper.CreateTestMemory(t, pool, granter.ID, principal.ID, "revoke test")

	memID := mem.ID
	g, err := s.Create(ctx, &sharing.Grant{
		MemoryID:       &memID,
		GranteeScopeID: grantee.ID,
		GrantedBy:      principal.ID,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := s.Revoke(ctx, g.ID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	// Grant should no longer appear in the list.
	grants, err := s.List(ctx, grantee.ID, 10, 0)
	if err != nil {
		t.Fatalf("List after Revoke: %v", err)
	}
	for _, gr := range grants {
		if gr.ID == g.ID {
			t.Error("revoked grant still appears in List")
		}
	}
}

func TestGrants_List_Pagination(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()
	s := sharing.NewStore(pool)

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "sharing-list-"+uuid.New().String())
	granter := testhelper.CreateTestScope(t, pool, "project", "sharing-list-granter-"+uuid.New().String(), nil, principal.ID)
	grantee := testhelper.CreateTestScope(t, pool, "project", "sharing-list-grantee-"+uuid.New().String(), nil, principal.ID)

	// Create 3 memory grants.
	for i := 0; i < 3; i++ {
		mem := testhelper.CreateTestMemory(t, pool, granter.ID, principal.ID, "list test memory")
		memID := mem.ID
		if _, err := s.Create(ctx, &sharing.Grant{
			MemoryID:       &memID,
			GranteeScopeID: grantee.ID,
			GrantedBy:      principal.ID,
		}); err != nil {
			t.Fatalf("Create grant %d: %v", i, err)
		}
	}

	// Fetch all.
	all, err := s.List(ctx, grantee.ID, 10, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) < 3 {
		t.Errorf("expected at least 3 grants, got %d", len(all))
	}

	// Fetch with limit=1.
	page, err := s.List(ctx, grantee.ID, 1, 0)
	if err != nil {
		t.Fatalf("List page: %v", err)
	}
	if len(page) != 1 {
		t.Errorf("expected 1 grant with limit=1, got %d", len(page))
	}
}

func TestGrants_IsMemoryAccessible(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()
	s := sharing.NewStore(pool)

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "sharing-memaccess-"+uuid.New().String())
	granter := testhelper.CreateTestScope(t, pool, "project", "sharing-memacc-granter-"+uuid.New().String(), nil, principal.ID)
	grantee := testhelper.CreateTestScope(t, pool, "project", "sharing-memacc-grantee-"+uuid.New().String(), nil, principal.ID)
	other := testhelper.CreateTestScope(t, pool, "project", "sharing-memacc-other-"+uuid.New().String(), nil, principal.ID)
	mem := testhelper.CreateTestMemory(t, pool, granter.ID, principal.ID, "access test")

	memID := mem.ID
	if _, err := s.Create(ctx, &sharing.Grant{
		MemoryID:       &memID,
		GranteeScopeID: grantee.ID,
		GrantedBy:      principal.ID,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	ok, err := s.IsMemoryAccessible(ctx, mem.ID, grantee.ID)
	if err != nil {
		t.Fatalf("IsMemoryAccessible: %v", err)
	}
	if !ok {
		t.Error("expected memory to be accessible to grantee")
	}

	ok, err = s.IsMemoryAccessible(ctx, mem.ID, other.ID)
	if err != nil {
		t.Fatalf("IsMemoryAccessible other: %v", err)
	}
	if ok {
		t.Error("expected memory to be inaccessible to non-grantee scope")
	}
}

func TestGrants_IsMemoryAccessible_Expired(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()
	s := sharing.NewStore(pool)

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "sharing-expired-"+uuid.New().String())
	granter := testhelper.CreateTestScope(t, pool, "project", "sharing-exp-granter-"+uuid.New().String(), nil, principal.ID)
	grantee := testhelper.CreateTestScope(t, pool, "project", "sharing-exp-grantee-"+uuid.New().String(), nil, principal.ID)
	mem := testhelper.CreateTestMemory(t, pool, granter.ID, principal.ID, "expired grant test")

	past := time.Now().Add(-time.Hour)
	memID := mem.ID
	if _, err := s.Create(ctx, &sharing.Grant{
		MemoryID:       &memID,
		GranteeScopeID: grantee.ID,
		GrantedBy:      principal.ID,
		ExpiresAt:      &past,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	ok, err := s.IsMemoryAccessible(ctx, mem.ID, grantee.ID)
	if err != nil {
		t.Fatalf("IsMemoryAccessible: %v", err)
	}
	if ok {
		t.Error("expected expired grant to make memory inaccessible")
	}
}

func TestGrants_IsArtifactAccessible(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()
	s := sharing.NewStore(pool)

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "sharing-artacc-"+uuid.New().String())
	granter := testhelper.CreateTestScope(t, pool, "project", "sharing-artacc-granter-"+uuid.New().String(), nil, principal.ID)
	grantee := testhelper.CreateTestScope(t, pool, "project", "sharing-artacc-grantee-"+uuid.New().String(), nil, principal.ID)
	other := testhelper.CreateTestScope(t, pool, "project", "sharing-artacc-other-"+uuid.New().String(), nil, principal.ID)
	testhelper.CreateTestEmbeddingModel(t, pool)
	artifact := testhelper.CreateTestArtifact(t, pool, granter.ID, principal.ID, "artifact access test")

	artID := artifact.ID
	if _, err := s.Create(ctx, &sharing.Grant{
		ArtifactID:     &artID,
		GranteeScopeID: grantee.ID,
		GrantedBy:      principal.ID,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	ok, err := s.IsArtifactAccessible(ctx, artifact.ID, grantee.ID)
	if err != nil {
		t.Fatalf("IsArtifactAccessible: %v", err)
	}
	if !ok {
		t.Error("expected artifact to be accessible to grantee")
	}

	ok, err = s.IsArtifactAccessible(ctx, artifact.ID, other.ID)
	if err != nil {
		t.Fatalf("IsArtifactAccessible other: %v", err)
	}
	if ok {
		t.Error("expected artifact to be inaccessible to non-grantee scope")
	}
}
