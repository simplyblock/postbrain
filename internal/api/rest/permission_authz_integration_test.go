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

func TestREST_PermissionAuthz_ReadVsWrite(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()
	cfg := &config.Config{}

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "rest-perm-user-"+uuid.NewString())
	scope := testhelper.CreateTestScope(t, pool, "project", "rest-perm-scope-"+uuid.NewString(), nil, principal.ID)
	testhelper.CreateTestEmbeddingModel(t, pool)

	rawReadToken, hashReadToken, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("generate read token: %v", err)
	}
	if _, err := compat.CreateToken(ctx, pool, principal.ID, hashReadToken, "read-only", nil, []string{"scopes:read", "memories:read"}, nil); err != nil {
		t.Fatalf("create read token: %v", err)
	}

	rawWriteToken, hashWriteToken, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("generate write token: %v", err)
	}
	if _, err := compat.CreateToken(ctx, pool, principal.ID, hashWriteToken, "write-token", nil, []string{"memories:write"}, nil); err != nil {
		t.Fatalf("create write token: %v", err)
	}

	handler := rest.NewRouter(pool, svc, cfg).Handler()
	srv := httptest.NewServer(handler)
	defer srv.Close()

	t.Run("scopes:read token can GET /v1/scopes", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, srv.URL+"/v1/scopes", nil)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+rawReadToken)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("do request: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
	})

	t.Run("memories:read token cannot POST /v1/memories", func(t *testing.T) {
		body := map[string]any{
			"content":     "permission gate",
			"scope":       "project:" + scope.ExternalID,
			"memory_type": "semantic",
			"importance":  0.5,
		}
		payload, _ := json.Marshal(body)
		req, err := http.NewRequest(http.MethodPost, srv.URL+"/v1/memories", bytes.NewReader(payload))
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+rawReadToken)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("do request: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusForbidden)
		}
	})

	t.Run("memories:write token can POST /v1/memories", func(t *testing.T) {
		body := map[string]any{
			"content":     "permission gate write",
			"scope":       "project:" + scope.ExternalID,
			"memory_type": "semantic",
			"importance":  0.5,
		}
		payload, _ := json.Marshal(body)
		req, err := http.NewRequest(http.MethodPost, srv.URL+"/v1/memories", bytes.NewReader(payload))
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+rawWriteToken)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("do request: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusCreated)
		}
	})
}
