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
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/db/compat"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

// TestApprovePromotion_IDOR_CrossScopeDenied is a regression test for the IDOR
// in POST /v1/promotions/{id}/approve.  Before the fix the handler called
// knwProm.Approve(id, …) without loading the promotion request first, so any
// authenticated principal could approve a promotion request belonging to another
// scope's target — mutating foreign knowledge state.
//
// After the fix the handler must load the request, enforce authorizeScopeAdmin
// on TargetScopeID, and return 403 when the caller is not an admin of that scope.
func TestApprovePromotion_IDOR_CrossScopeDenied(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewTestPool(t)
	cfg := &config.Config{}
	svc := testhelper.NewMockEmbeddingService()

	// user-b owns scope-b with a pending promotion request targeting scope-b.
	principalB := testhelper.CreateTestPrincipal(t, pool, "user", "idor-approve-b-"+uuid.New().String())
	scopeB := testhelper.CreateTestScope(t, pool, "project", "idor-approve-scope-b-"+uuid.New().String(), nil, principalB.ID)
	memB := testhelper.CreateTestMemory(t, pool, scopeB.ID, principalB.ID, "user-b memory for promotion")

	promReq, err := compat.CreatePromotionRequest(ctx, pool, &db.PromotionRequest{
		MemoryID:         memB.ID,
		RequestedBy:      principalB.ID,
		TargetScopeID:    scopeB.ID,
		TargetVisibility: "project",
	})
	if err != nil {
		t.Fatalf("create promotion request: %v", err)
	}

	// user-a owns scope-a and holds a token scoped to scope-a only.
	principalA := testhelper.CreateTestPrincipal(t, pool, "user", "idor-approve-a-"+uuid.New().String())
	scopeA := testhelper.CreateTestScope(t, pool, "project", "idor-approve-scope-a-"+uuid.New().String(), nil, principalA.ID)

	rawToken, hashToken, err := auth.GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	_, err = compat.CreateToken(ctx, pool, principalA.ID, hashToken, "idor-approve-token-a", []uuid.UUID{scopeA.ID}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	handler := rest.NewRouter(pool, svc, cfg).Handler()
	srv := httptest.NewServer(handler)
	defer srv.Close()

	req, err := http.NewRequest(
		http.MethodPost,
		srv.URL+"/v1/promotions/"+promReq.ID.String()+"/approve",
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

	// The promotion targets scope-b; user-a's token covers only scope-a.
	// authorizeScopeAdmin must reject this with 403.
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("IDOR: status = %d, want %d — user-a must not approve user-b's promotion",
			resp.StatusCode, http.StatusForbidden)
	}
}

// TestRejectPromotion_IDOR_CrossScopeDenied mirrors the approve IDOR test for
// POST /v1/promotions/{id}/reject.
func TestRejectPromotion_IDOR_CrossScopeDenied(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewTestPool(t)
	cfg := &config.Config{}
	svc := testhelper.NewMockEmbeddingService()

	principalB := testhelper.CreateTestPrincipal(t, pool, "user", "idor-reject-b-"+uuid.New().String())
	scopeB := testhelper.CreateTestScope(t, pool, "project", "idor-reject-scope-b-"+uuid.New().String(), nil, principalB.ID)
	memB := testhelper.CreateTestMemory(t, pool, scopeB.ID, principalB.ID, "user-b memory for reject test")

	promReq, err := compat.CreatePromotionRequest(ctx, pool, &db.PromotionRequest{
		MemoryID:         memB.ID,
		RequestedBy:      principalB.ID,
		TargetScopeID:    scopeB.ID,
		TargetVisibility: "project",
	})
	if err != nil {
		t.Fatalf("create promotion request: %v", err)
	}

	principalA := testhelper.CreateTestPrincipal(t, pool, "user", "idor-reject-a-"+uuid.New().String())
	scopeA := testhelper.CreateTestScope(t, pool, "project", "idor-reject-scope-a-"+uuid.New().String(), nil, principalA.ID)

	rawToken, hashToken, err := auth.GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	_, err = compat.CreateToken(ctx, pool, principalA.ID, hashToken, "idor-reject-token-a", []uuid.UUID{scopeA.ID}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	handler := rest.NewRouter(pool, svc, cfg).Handler()
	srv := httptest.NewServer(handler)
	defer srv.Close()

	req, err := http.NewRequest(
		http.MethodPost,
		srv.URL+"/v1/promotions/"+promReq.ID.String()+"/reject",
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

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("IDOR: status = %d, want %d — user-a must not reject user-b's promotion",
			resp.StatusCode, http.StatusForbidden)
	}
}