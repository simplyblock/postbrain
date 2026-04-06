//go:build integration

package rest_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/api/rest"
	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/config"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

func TestREST_ScopeGrants_CRUD(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()
	cfg := &config.Config{}

	owner := testhelper.CreateTestPrincipal(t, pool, "user", "sg-crud-owner-"+uuid.NewString())
	grantee := testhelper.CreateTestPrincipal(t, pool, "user", "sg-crud-grantee-"+uuid.NewString())
	scope := testhelper.CreateTestScope(t, pool, "project", "sg-crud-scope-"+uuid.NewString(), nil, owner.ID)

	// Token for owner with sharing:write, sharing:read, sharing:delete permissions.
	rawToken, hashToken, err := auth.GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateToken(ctx, pool, owner.ID, hashToken, "sg-crud-token",
		nil, []string{"sharing:write", "sharing:read", "sharing:delete", "memories:write", "memories:read"}, nil); err != nil {
		t.Fatal(err)
	}

	handler := rest.NewRouter(pool, svc, cfg).Handler()
	srv := httptest.NewServer(handler)
	defer srv.Close()

	authHeader := "Bearer " + rawToken
	scopeURL := srv.URL + "/v1/scopes/" + scope.ID.String() + "/grants"

	var createdGrantID string

	t.Run("POST creates scope grant", func(t *testing.T) {
		body := map[string]any{
			"principal_id": grantee.ID.String(),
			"permissions":  []string{"memories:read"},
		}
		b, _ := json.Marshal(body)
		req, _ := http.NewRequest(http.MethodPost, scopeURL, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", authHeader)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("expected 201, got %d", resp.StatusCode)
		}

		var result map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		id, _ := result["id"].(string)
		if id == "" {
			t.Fatal("expected id in response")
		}
		createdGrantID = id
	})

	t.Run("GET lists scope grants", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, scopeURL, nil)
		req.Header.Set("Authorization", authHeader)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		var result map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		grants, _ := result["grants"].([]any)
		if len(grants) == 0 {
			t.Fatal("expected at least one grant in response")
		}
	})

	t.Run("DELETE revokes scope grant", func(t *testing.T) {
		if createdGrantID == "" {
			t.Skip("grant not created")
		}
		req, _ := http.NewRequest(http.MethodDelete,
			srv.URL+"/v1/scopes/"+scope.ID.String()+"/grants/"+createdGrantID, nil)
		req.Header.Set("Authorization", authHeader)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("expected 204, got %d", resp.StatusCode)
		}
	})

	t.Run("GET after DELETE shows no grants", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, scopeURL, nil)
		req.Header.Set("Authorization", authHeader)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		var result map[string]any
		json.NewDecoder(resp.Body).Decode(&result)
		grants, _ := result["grants"].([]any)
		if len(grants) != 0 {
			t.Fatalf("expected 0 grants after delete, got %d", len(grants))
		}
	})
}

func TestREST_ScopeGrants_RequiresPermissions(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()
	cfg := &config.Config{}

	owner := testhelper.CreateTestPrincipal(t, pool, "user", "sg-perm-owner-"+uuid.NewString())
	grantee := testhelper.CreateTestPrincipal(t, pool, "user", "sg-perm-grantee-"+uuid.NewString())
	scope := testhelper.CreateTestScope(t, pool, "project", "sg-perm-scope-"+uuid.NewString(), nil, owner.ID)

	// Token with no sharing permissions.
	rawToken, hashToken, err := auth.GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateToken(ctx, pool, owner.ID, hashToken, "sg-perm-token",
		nil, []string{"memories:read"}, nil); err != nil {
		t.Fatal(err)
	}

	handler := rest.NewRouter(pool, svc, cfg).Handler()
	srv := httptest.NewServer(handler)
	defer srv.Close()

	authHeader := "Bearer " + rawToken
	scopeURL := srv.URL + "/v1/scopes/" + scope.ID.String() + "/grants"

	t.Run("POST without sharing:write returns 403", func(t *testing.T) {
		body := map[string]any{
			"principal_id": grantee.ID.String(),
			"permissions":  []string{"memories:read"},
		}
		b, _ := json.Marshal(body)
		req, _ := http.NewRequest(http.MethodPost, scopeURL, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", authHeader)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("expected 403, got %d", resp.StatusCode)
		}
	})

	t.Run("GET without sharing:read returns 403", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, scopeURL, nil)
		req.Header.Set("Authorization", authHeader)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("expected 403, got %d", resp.StatusCode)
		}
	})
}

func TestREST_ScopeGrants_AntiEscalation(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()
	cfg := &config.Config{}

	owner := testhelper.CreateTestPrincipal(t, pool, "user", "sg-esc-owner-"+uuid.NewString())
	grantee := testhelper.CreateTestPrincipal(t, pool, "user", "sg-esc-grantee-"+uuid.NewString())
	scope := testhelper.CreateTestScope(t, pool, "project", "sg-esc-scope-"+uuid.NewString(), nil, owner.ID)

	// Token with sharing:write but only memories:read (cannot grant memories:write).
	rawToken, hashToken, err := auth.GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateToken(ctx, pool, owner.ID, hashToken, "sg-esc-token",
		nil, []string{"sharing:write", "sharing:read", "memories:read"}, nil); err != nil {
		t.Fatal(err)
	}

	handler := rest.NewRouter(pool, svc, cfg).Handler()
	srv := httptest.NewServer(handler)
	defer srv.Close()

	authHeader := "Bearer " + rawToken
	scopeURL := srv.URL + "/v1/scopes/" + scope.ID.String() + "/grants"

	t.Run("cannot grant permissions caller does not hold", func(t *testing.T) {
		body := map[string]any{
			"principal_id": grantee.ID.String(),
			"permissions":  []string{"memories:write"}, // caller only has memories:read
		}
		b, _ := json.Marshal(body)
		req, _ := http.NewRequest(http.MethodPost, scopeURL, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", authHeader)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("expected 403 for escalation attempt, got %d", resp.StatusCode)
		}
	})

	t.Run("can grant permissions caller holds", func(t *testing.T) {
		body := map[string]any{
			"principal_id": grantee.ID.String(),
			"permissions":  []string{"memories:read"}, // caller has this
		}
		b, _ := json.Marshal(body)
		req, _ := http.NewRequest(http.MethodPost, scopeURL, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", authHeader)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("expected 201 for valid grant, got %d", resp.StatusCode)
		}
	})
}

func TestREST_ScopeGrants_ExpiredGrantExcluded(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()
	cfg := &config.Config{}

	owner := testhelper.CreateTestPrincipal(t, pool, "user", "sg-exp-owner-"+uuid.NewString())
	grantee := testhelper.CreateTestPrincipal(t, pool, "user", "sg-exp-grantee-"+uuid.NewString())
	scope := testhelper.CreateTestScope(t, pool, "project", "sg-exp-scope-"+uuid.NewString(), nil, owner.ID)

	// Insert an expired grant directly.
	q := db.New(pool)
	past := time.Now().Add(-time.Hour)
	if _, err := q.CreateScopeGrant(ctx, db.CreateScopeGrantParams{
		PrincipalID: grantee.ID,
		ScopeID:     scope.ID,
		Permissions: []string{"memories:read"},
		ExpiresAt:   &past,
	}); err != nil {
		t.Fatalf("CreateScopeGrant: %v", err)
	}

	rawToken, hashToken, err := auth.GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateToken(ctx, pool, owner.ID, hashToken, "sg-exp-token",
		nil, []string{"sharing:read"}, nil); err != nil {
		t.Fatal(err)
	}

	handler := rest.NewRouter(pool, svc, cfg).Handler()
	srv := httptest.NewServer(handler)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet,
		srv.URL+"/v1/scopes/"+scope.ID.String()+"/grants", nil)
	req.Header.Set("Authorization", "Bearer "+rawToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	grants, _ := result["grants"].([]any)
	if len(grants) != 0 {
		t.Fatalf("expected 0 grants (expired excluded), got %d", len(grants))
	}
}

func TestREST_ScopeGrants_SystemAdminCanCreate(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()
	cfg := &config.Config{}

	admin := testhelper.CreateTestPrincipal(t, pool, "user", "sg-sa-admin-"+uuid.NewString())
	owner := testhelper.CreateTestPrincipal(t, pool, "user", "sg-sa-owner-"+uuid.NewString())
	grantee := testhelper.CreateTestPrincipal(t, pool, "user", "sg-sa-grantee-"+uuid.NewString())
	scope := testhelper.CreateTestScope(t, pool, "project", "sg-sa-scope-"+uuid.NewString(), nil, owner.ID)

	// Set admin as system admin.
	if _, err := pool.Exec(ctx, `UPDATE principals SET is_system_admin = true WHERE id = $1`, admin.ID); err != nil {
		t.Fatalf("set system admin: %v", err)
	}

	rawToken, hashToken, err := auth.GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateToken(ctx, pool, admin.ID, hashToken, "sg-sa-token",
		nil, []string{"sharing:write"}, nil); err != nil {
		t.Fatal(err)
	}

	handler := rest.NewRouter(pool, svc, cfg).Handler()
	srv := httptest.NewServer(handler)
	defer srv.Close()

	body := map[string]any{
		"principal_id": grantee.ID.String(),
		"permissions":  []string{"memories:write", "knowledge:write"},
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost,
		srv.URL+"/v1/scopes/"+scope.ID.String()+"/grants", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+rawToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 for system admin, got %d", resp.StatusCode)
	}
}
