//go:build integration

package rest_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/simplyblock/postbrain/internal/api/rest"
	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/config"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

func TestREST_E2E(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()
	cfg := &config.Config{}

	// Create principal and token.
	principal := testhelper.CreateTestPrincipal(t, pool, "user", "e2e-user")
	rawToken, hashToken, err := auth.GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.CreateToken(ctx, pool, principal.ID, hashToken, "test-token", nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Build httptest server.
	handler := rest.NewRouter(pool, svc, cfg).Handler()
	srv := httptest.NewServer(handler)
	defer srv.Close()

	authHeader := "Bearer " + rawToken

	t.Run("health returns 200", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/health")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("no auth returns 401", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/v1/memories/recall?query=test&scope=project:test")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", resp.StatusCode)
		}
	})

	var createdMemoryID string

	t.Run("create memory returns 201", func(t *testing.T) {
		scope := testhelper.CreateTestScope(t, pool, "project", "e2e-project", nil, principal.ID)
		body := map[string]any{
			"content":     "The API uses PostgreSQL for persistence",
			"scope":       "project:" + scope.ExternalID,
			"memory_type": "semantic",
			"importance":  0.7,
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
			t.Errorf("expected 201, got %d", resp.StatusCode)
		}
		var result map[string]any
		json.NewDecoder(resp.Body).Decode(&result)
		if result["memory_id"] == nil {
			t.Error("expected memory_id in response")
		}
		createdMemoryID, _ = result["memory_id"].(string)
	})

	t.Run("recall returns results", func(t *testing.T) {
		if createdMemoryID == "" {
			t.Skip("memory not created")
		}
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/memories/recall?query=PostgreSQL&scope=project:e2e-project", nil)
		req.Header.Set("Authorization", authHeader)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("delete memory returns 204 or 200", func(t *testing.T) {
		if createdMemoryID == "" {
			t.Skip("memory not created")
		}
		req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/v1/memories/"+createdMemoryID, nil)
		req.Header.Set("Authorization", authHeader)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
			t.Errorf("expected 204 or 200, got %d", resp.StatusCode)
		}
	})
}
