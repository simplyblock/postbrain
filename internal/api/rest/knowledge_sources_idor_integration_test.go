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
	"github.com/simplyblock/postbrain/internal/db/compat"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

// TestGetArtifactSources_IDOR_CrossScopeDenied is a regression test for the
// IDOR in GET /v1/knowledge/{id}/sources.  Before the fix the handler called
// synth.ListSources with the user-supplied {id} and no scope authorization,
// so any authenticated caller could read provenance/source documents for
// artifacts in other tenants' scopes.
//
// After the fix the handler must resolve the digest artifact, enforce
// authorizeObjectScope on its OwnerScopeID, and return 403 for cross-scope reads.
func TestGetArtifactSources_IDOR_CrossScopeDenied(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewTestPool(t)
	cfg := &config.Config{}
	svc := testhelper.NewMockEmbeddingService()
	testhelper.CreateTestEmbeddingModel(t, pool)

	// user-a owns scope-a and holds a token scoped to scope-a.
	principalA := testhelper.CreateTestPrincipal(t, pool, "user", "idor-sources-a-"+uuid.New().String())
	scopeA := testhelper.CreateTestScope(t, pool, "project", "idor-sources-scope-a-"+uuid.New().String(), nil, principalA.ID)

	// user-b owns scope-b with a digest artifact whose sources user-a must NOT see.
	principalB := testhelper.CreateTestPrincipal(t, pool, "user", "idor-sources-b-"+uuid.New().String())
	scopeB := testhelper.CreateTestScope(t, pool, "project", "idor-sources-scope-b-"+uuid.New().String(), nil, principalB.ID)
	digestB := testhelper.CreateTestArtifact(t, pool, scopeB.ID, principalB.ID, "user-b private digest")

	// Issue a token for user-a scoped to scope-a only.
	rawToken, hashToken, err := auth.GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	_, err = compat.CreateToken(ctx, pool, principalA.ID, hashToken, "idor-sources-token-a", []uuid.UUID{scopeA.ID}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	handler := rest.NewRouter(pool, svc, cfg).Handler()
	srv := httptest.NewServer(handler)
	defer srv.Close()

	req, err := http.NewRequest(
		http.MethodGet,
		srv.URL+"/v1/knowledge/"+digestB.ID.String()+"/sources",
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

	// The digest belongs to scope-b; user-a's token covers only scope-a.
	// The handler must reject the request with 403.
	if resp.StatusCode != http.StatusForbidden {
		var body map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&body)
		t.Errorf("IDOR: status = %d, want %d — user-a must not read user-b digest sources (body: %v)",
			resp.StatusCode, http.StatusForbidden, body)
	}
}