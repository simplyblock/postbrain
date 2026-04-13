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
	"github.com/simplyblock/postbrain/internal/graph"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

func TestGraphQuery_AGEAwareBehavior(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()
	cfg := &config.Config{}

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "graph-query-user-"+uuid.NewString())
	scope := testhelper.CreateTestScope(t, pool, "project", "graph-query-scope-"+uuid.NewString(), nil, principal.ID)
	rawToken, hashToken, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	if _, err := compat.CreateToken(ctx, pool, principal.ID, hashToken, "graph-query-token", nil, nil, nil); err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	if err := db.EnsureAGEOverlay(ctx, pool); err != nil {
		t.Fatalf("EnsureAGEOverlay: %v", err)
	}

	scopeID := scope.ID
	if graph.DetectAGE(ctx, pool) {
		if err := graph.SyncEntityToAGE(ctx, pool, &db.Entity{
			ID:         uuid.New(),
			ScopeID:    scopeID,
			EntityType: "file",
			Name:       "auth.go",
			Canonical:  "src/auth.go",
		}); err != nil {
			t.Fatalf("SyncEntityToAGE: %v", err)
		}
	}

	handler := rest.NewRouter(pool, svc, cfg).Handler()
	srv := httptest.NewServer(handler)
	defer srv.Close()

	reqBody := map[string]any{
		"scope_id": scopeID.String(),
		"cypher":   "RETURN n",
	}
	buf, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/graph/query", bytes.NewReader(buf))
	req.Header.Set("Authorization", "Bearer "+rawToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("query request: %v", err)
	}
	defer resp.Body.Close()

	if !graph.DetectAGE(ctx, pool) {
		if resp.StatusCode != http.StatusNotImplemented {
			t.Fatalf("status without AGE = %d, want %d", resp.StatusCode, http.StatusNotImplemented)
		}
		return
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status with AGE = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	rows, ok := payload["rows"].([]any)
	if !ok {
		t.Fatalf("rows missing or wrong type: %T", payload["rows"])
	}
	if len(rows) == 0 {
		t.Fatalf("rows = 0, want >= 1")
	}
}

func TestGraphQuery_DeniesTokenScopeMismatch(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()
	cfg := &config.Config{}

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "graph-query-deny-user-"+uuid.NewString())
	allowed := testhelper.CreateTestScope(t, pool, "project", "graph-query-allowed-"+uuid.NewString(), nil, principal.ID)
	denied := testhelper.CreateTestScope(t, pool, "project", "graph-query-denied-"+uuid.NewString(), nil, principal.ID)

	rawToken, hashToken, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	if _, err := compat.CreateToken(ctx, pool, principal.ID, hashToken, "graph-query-deny-token", []uuid.UUID{allowed.ID}, nil, nil); err != nil {
		t.Fatalf("CreateToken restricted: %v", err)
	}

	handler := rest.NewRouter(pool, svc, cfg).Handler()
	srv := httptest.NewServer(handler)
	defer srv.Close()

	reqBody := map[string]any{
		"scope_id": denied.ID.String(),
		"cypher":   "RETURN n",
	}
	buf, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/graph/query", bytes.NewReader(buf))
	req.Header.Set("Authorization", "Bearer "+rawToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("query request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
}
