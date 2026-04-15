//go:build integration

package rest_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/api/rest"
	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/config"
	"github.com/simplyblock/postbrain/internal/db/compat"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

// TestPromoteMemory_IDOR_CrossScopeMemoryDenied is a regression test for the
// IDOR in POST /v1/memories/{id}/promote.  Before the fix, a principal with
// promotion permission in their own scope could nominate a memory that belongs
// to a different principal's scope by supplying an arbitrary {id} value.
//
// After the fix the handler must resolve the source memory and enforce
// authorizeObjectScope on its ScopeID before creating the promotion request,
// so the cross-scope attempt must return 403.
func TestPromoteMemory_IDOR_CrossScopeMemoryDenied(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewTestPool(t)
	cfg := &config.Config{}
	svc := testhelper.NewMockEmbeddingService()

	// user-a owns scope-a and holds a token scoped to scope-a.
	principalA := testhelper.CreateTestPrincipal(t, pool, "user", "idor-promote-a-"+uuid.New().String())
	scopeA := testhelper.CreateTestScope(t, pool, "project", "idor-promote-scope-a-"+uuid.New().String(), nil, principalA.ID)

	// user-b owns scope-b with a memory that user-a must NOT be able to promote.
	principalB := testhelper.CreateTestPrincipal(t, pool, "user", "idor-promote-b-"+uuid.New().String())
	scopeB := testhelper.CreateTestScope(t, pool, "project", "idor-promote-scope-b-"+uuid.New().String(), nil, principalB.ID)
	memB := testhelper.CreateTestMemory(t, pool, scopeB.ID, principalB.ID, "user-b private memory")

	// Issue a token for user-a scoped to scope-a only.
	rawToken, hashToken, err := auth.GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	_, err = compat.CreateToken(ctx, pool, principalA.ID, hashToken, "idor-promote-token-a", []uuid.UUID{scopeA.ID}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	handler := rest.NewRouter(pool, svc, cfg).Handler()
	srv := httptest.NewServer(handler)
	defer srv.Close()

	body := map[string]any{
		"target_scope":      "project:" + scopeA.ExternalID,
		"target_visibility": "project",
	}
	b, _ := json.Marshal(body)
	req, err := http.NewRequest(
		http.MethodPost,
		srv.URL+"/v1/memories/"+memB.ID.String()+"/promote",
		bytes.NewReader(b),
	)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+rawToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	// The source memory belongs to scope-b; user-a's token covers only scope-a.
	// The handler must reject the request with 403.
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("IDOR: status = %d, want %d — user-a must not promote user-b's memory",
			resp.StatusCode, http.StatusForbidden)
	}
}
