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
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/db/compat"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

// TestREST_DirectScopeGrant_AllowsAccess verifies that a principal holding only a
// direct scope grant (no ownership, no membership) can access content in that scope.
func TestREST_DirectScopeGrant_AllowsAccess(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()
	cfg := &config.Config{}
	testhelper.CreateTestEmbeddingModel(t, pool)

	// Two unrelated principals.
	owner := testhelper.CreateTestPrincipal(t, pool, "user", "sg-owner-"+uuid.NewString())
	grantee := testhelper.CreateTestPrincipal(t, pool, "user", "sg-grantee-"+uuid.NewString())

	// Scope owned by owner, not grantee.
	scope := testhelper.CreateTestScope(t, pool, "project", "sg-scope-"+uuid.NewString(), nil, owner.ID)

	// Create a direct scope grant: grantee gets memories:write on scope.
	q := db.New(pool)
	if _, err := q.CreateScopeGrant(ctx, db.CreateScopeGrantParams{
		PrincipalID: grantee.ID,
		ScopeID:     scope.ID,
		Permissions: []string{"memories:write"},
	}); err != nil {
		t.Fatalf("CreateScopeGrant: %v", err)
	}

	// Token for grantee with memories:write permission (no scope_ids restriction).
	rawToken, hashToken, err := auth.GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := compat.CreateToken(ctx, pool, grantee.ID, hashToken, "sg-token", nil, []string{"memories:write"}, nil); err != nil {
		t.Fatal(err)
	}

	handler := rest.NewRouter(pool, svc, cfg).Handler()
	srv := httptest.NewServer(handler)
	defer srv.Close()

	// POST /v1/memories into the granted scope — should succeed (201).
	body := map[string]any{
		"content":     "direct grant access test",
		"scope":       "project:" + scope.ExternalID,
		"memory_type": "semantic",
		"importance":  0.5,
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/memories", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+rawToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 for direct scope grant access, got %d", resp.StatusCode)
	}
}

// TestREST_MembershipInAncestor_AllowsChildScopeAccess verifies that a principal
// that is a member of a parent principal can access scopes owned by that parent.
func TestREST_MembershipInAncestor_AllowsChildScopeAccess(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()
	cfg := &config.Config{}
	testhelper.CreateTestEmbeddingModel(t, pool)

	// Parent principal owns a scope; member principal belongs to parent.
	parent := testhelper.CreateTestPrincipal(t, pool, "team", "ma-parent-"+uuid.NewString())
	member := testhelper.CreateTestPrincipal(t, pool, "user", "ma-member-"+uuid.NewString())
	scope := testhelper.CreateTestScope(t, pool, "project", "ma-scope-"+uuid.NewString(), nil, parent.ID)

	// Add member as a member of parent.
	if _, err := compat.CreateMembership(ctx, pool, member.ID, parent.ID, "member", nil); err != nil {
		t.Fatalf("CreateMembership: %v", err)
	}

	rawToken, hashToken, err := auth.GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	// Token with memories:write (member role includes this permission).
	if _, err := compat.CreateToken(ctx, pool, member.ID, hashToken, "ma-token", nil, []string{"memories:write"}, nil); err != nil {
		t.Fatal(err)
	}

	handler := rest.NewRouter(pool, svc, cfg).Handler()
	srv := httptest.NewServer(handler)
	defer srv.Close()

	body := map[string]any{
		"content":     "membership ancestor access test",
		"scope":       "project:" + scope.ExternalID,
		"memory_type": "semantic",
		"importance":  0.5,
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/memories", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+rawToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 for membership ancestor access, got %d", resp.StatusCode)
	}
}

// TestREST_TokenScopeIDs_BlocksSiblingScope verifies that a token with scope_ids
// restricted to scope S cannot access sibling scope T, even though the principal can.
func TestREST_TokenScopeIDs_BlocksSiblingScope(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()
	cfg := &config.Config{}
	testhelper.CreateTestEmbeddingModel(t, pool)

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "ts-principal-"+uuid.NewString())
	allowedScope := testhelper.CreateTestScope(t, pool, "project", "ts-allowed-"+uuid.NewString(), nil, principal.ID)
	siblingScope := testhelper.CreateTestScope(t, pool, "project", "ts-sibling-"+uuid.NewString(), nil, principal.ID)

	rawToken, hashToken, err := auth.GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	// Token restricted to allowedScope only.
	if _, err := compat.CreateToken(ctx, pool, principal.ID, hashToken, "ts-token",
		[]uuid.UUID{allowedScope.ID}, []string{"memories:write"}, nil); err != nil {
		t.Fatal(err)
	}

	handler := rest.NewRouter(pool, svc, cfg).Handler()
	srv := httptest.NewServer(handler)
	defer srv.Close()

	authHeader := "Bearer " + rawToken

	t.Run("allowed scope succeeds", func(t *testing.T) {
		body := map[string]any{
			"content":     "token scope restriction test - allowed",
			"scope":       "project:" + allowedScope.ExternalID,
			"memory_type": "semantic",
			"importance":  0.5,
		}
		b, _ := json.Marshal(body)
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/memories", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", authHeader)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("expected 201 for allowed scope, got %d", resp.StatusCode)
		}
	})

	t.Run("sibling scope is blocked", func(t *testing.T) {
		body := map[string]any{
			"content":     "token scope restriction test - sibling",
			"scope":       "project:" + siblingScope.ExternalID,
			"memory_type": "semantic",
			"importance":  0.5,
		}
		b, _ := json.Marshal(body)
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/memories", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", authHeader)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("expected 403 for sibling scope, got %d", resp.StatusCode)
		}
	})
}

// TestREST_UpwardRead_ParentScopeInList verifies that a principal with read access
// to a child scope sees the parent scope in GET /v1/scopes (upward-read inheritance).
func TestREST_UpwardRead_ParentScopeInList(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()
	cfg := &config.Config{}

	parentOwner := testhelper.CreateTestPrincipal(t, pool, "team", "ur-parent-owner-"+uuid.NewString())
	childOwner := testhelper.CreateTestPrincipal(t, pool, "team", "ur-child-owner-"+uuid.NewString())
	grantee := testhelper.CreateTestPrincipal(t, pool, "user", "ur-grantee-"+uuid.NewString())

	// Parent scope owned by parentOwner; child scope is a child of parent.
	parentScope := testhelper.CreateTestScope(t, pool, "project", "ur-parent-"+uuid.NewString(), nil, parentOwner.ID)
	childScope := testhelper.CreateTestScope(t, pool, "project", "ur-child-"+uuid.NewString(), &parentScope.ID, childOwner.ID)

	// Grant grantee memories:read on the child scope only.
	q := db.New(pool)
	if _, err := q.CreateScopeGrant(ctx, db.CreateScopeGrantParams{
		PrincipalID: grantee.ID,
		ScopeID:     childScope.ID,
		Permissions: []string{"memories:read"},
	}); err != nil {
		t.Fatalf("CreateScopeGrant: %v", err)
	}

	rawToken, hashToken, err := auth.GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := compat.CreateToken(ctx, pool, grantee.ID, hashToken, "ur-token", nil, []string{"scopes:read", "memories:read"}, nil); err != nil {
		t.Fatal(err)
	}

	handler := rest.NewRouter(pool, svc, cfg).Handler()
	srv := httptest.NewServer(handler)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/scopes", nil)
	req.Header.Set("Authorization", "Bearer "+rawToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result struct {
		Scopes []db.Scope `json:"scopes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	got := map[uuid.UUID]bool{}
	for _, s := range result.Scopes {
		got[s.ID] = true
	}

	if !got[childScope.ID] {
		t.Errorf("expected child scope %s in list (has direct grant)", childScope.ID)
	}
	if !got[parentScope.ID] {
		t.Errorf("expected parent scope %s in list (upward-read from child grant)", parentScope.ID)
	}
}
