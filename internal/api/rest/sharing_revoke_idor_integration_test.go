//go:build integration

package rest_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/api/rest"
	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/config"
	"github.com/simplyblock/postbrain/internal/db/compat"
	"github.com/simplyblock/postbrain/internal/sharing"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

// TestRevokeGrant_IDOR_CrossPrincipalDenied is a regression test for the IDOR
// in DELETE /v1/sharing/grants/{id}.  Before the fix the handler called
// ro.sharing.Revoke(id) using only the caller-supplied {id}, with no check
// that the caller created the grant.  Any authenticated principal could delete
// any grant by ID.
//
// After the fix the handler must load the grant, verify the caller is the
// principal who created it (grant.GrantedBy == callerPrincipalID), and return
// 403 when they differ.
func TestRevokeGrant_IDOR_CrossPrincipalDenied(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewTestPool(t)
	cfg := &config.Config{}
	svc := testhelper.NewMockEmbeddingService()

	// user-b creates a grant (GrantedBy = principalB).
	principalB := testhelper.CreateTestPrincipal(t, pool, "user", "idor-revoke-b-"+uuid.New().String())
	scopeB := testhelper.CreateTestScope(t, pool, "project", "idor-revoke-scope-b-"+uuid.New().String(), nil, principalB.ID)
	memB := testhelper.CreateTestMemory(t, pool, scopeB.ID, principalB.ID, "user-b memory")

	granteePrincipal := testhelper.CreateTestPrincipal(t, pool, "user", "idor-revoke-grantee-"+uuid.New().String())
	granteeScope := testhelper.CreateTestScope(t, pool, "project", "idor-revoke-grantee-scope-"+uuid.New().String(), nil, granteePrincipal.ID)

	sharingStore := sharing.NewStore(pool)
	grant, err := sharingStore.Create(ctx, &sharing.Grant{
		MemoryID:       &memB.ID,
		GranteeScopeID: granteeScope.ID,
		GrantedBy:      principalB.ID,
	})
	if err != nil {
		t.Fatalf("create grant: %v", err)
	}

	// user-a has their own scope and token — they did NOT create the grant.
	principalA := testhelper.CreateTestPrincipal(t, pool, "user", "idor-revoke-a-"+uuid.New().String())
	scopeA := testhelper.CreateTestScope(t, pool, "project", "idor-revoke-scope-a-"+uuid.New().String(), nil, principalA.ID)

	rawToken, hashToken, err := auth.GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	_, err = compat.CreateToken(ctx, pool, principalA.ID, hashToken, "idor-revoke-token-a", []uuid.UUID{scopeA.ID}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	handler := rest.NewRouter(pool, svc, cfg).Handler()
	srv := httptest.NewServer(handler)
	defer srv.Close()

	req, err := http.NewRequest(
		http.MethodDelete,
		srv.URL+"/v1/sharing/grants/"+grant.ID.String(),
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+rawToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	// user-a did not create this grant; the handler must return 403.
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("IDOR: status = %d, want %d — user-a must not revoke user-b's grant",
			resp.StatusCode, http.StatusForbidden)
	}

	// Verify the grant still exists (was not deleted).
	remaining, err := sharingStore.GetByID(ctx, grant.ID)
	if err != nil {
		t.Fatalf("get grant after failed revoke: %v", err)
	}
	if remaining == nil {
		t.Error("IDOR: grant was deleted by unauthorized caller")
	}
}
