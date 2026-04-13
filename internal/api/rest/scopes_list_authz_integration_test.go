//go:build integration

package rest_test

import (
	"context"
	"encoding/json"
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

func TestREST_ListScopes_RestrictedToEffectiveWritableScopes(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()
	cfg := &config.Config{}

	principalA := testhelper.CreateTestPrincipal(t, pool, "user", "rest-list-scope-a-"+uuid.NewString())
	principalB := testhelper.CreateTestPrincipal(t, pool, "user", "rest-list-scope-b-"+uuid.NewString())
	scopeA1 := testhelper.CreateTestScope(t, pool, "project", "rest-list-scope-a1-"+uuid.NewString(), nil, principalA.ID)
	scopeA2 := testhelper.CreateTestScope(t, pool, "project", "rest-list-scope-a2-"+uuid.NewString(), nil, principalA.ID)
	scopeB := testhelper.CreateTestScope(t, pool, "project", "rest-list-scope-b-"+uuid.NewString(), nil, principalB.ID)

	rawToken, hashToken, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	if _, err := compat.CreateToken(ctx, pool, principalA.ID, hashToken, "rest-list-scopes", nil, nil, nil); err != nil {
		t.Fatalf("create unrestricted token: %v", err)
	}

	handler := rest.NewRouter(pool, svc, cfg).Handler()
	srv := httptest.NewServer(handler)
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/v1/scopes", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+rawToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body struct {
		Scopes []db.Scope `json:"scopes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	got := map[uuid.UUID]bool{}
	for _, s := range body.Scopes {
		got[s.ID] = true
	}
	if !got[scopeA1.ID] || !got[scopeA2.ID] {
		t.Fatalf("expected own writable scopes in response, got=%v", got)
	}
	if got[scopeB.ID] {
		t.Fatalf("did not expect non-writable scope %s in response", scopeB.ID)
	}
}
