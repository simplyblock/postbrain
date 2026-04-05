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
	var createdScopeID uuid.UUID

	t.Run("create memory returns 201", func(t *testing.T) {
		scope := testhelper.CreateTestScope(t, pool, "project", "e2e-project", nil, principal.ID)
		createdScopeID = scope.ID
		summary := "Stores persistence details for the API."
		body := map[string]any{
			"content":     "The API uses PostgreSQL for persistence",
			"summary":     summary,
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

		memID, err := uuid.Parse(createdMemoryID)
		if err != nil {
			t.Fatalf("invalid memory_id %q: %v", createdMemoryID, err)
		}
		mem, err := db.GetMemory(ctx, pool, memID)
		if err != nil {
			t.Fatalf("GetMemory: %v", err)
		}
		if mem == nil {
			t.Fatal("expected created memory to exist")
		}
		if mem.Summary == nil || *mem.Summary != summary {
			t.Fatalf("summary = %v, want %q", mem.Summary, summary)
		}
		var meta map[string]any
		if err := json.Unmarshal(mem.Meta, &meta); err != nil {
			t.Fatalf("memory meta is not valid JSON: %v", err)
		}
		if got, _ := meta["content_style"].(string); got != "long" {
			t.Fatalf("meta.content_style = %q, want %q", got, "long")
		}
	})

	t.Run("update memory persists summary and long style preference", func(t *testing.T) {
		if createdMemoryID == "" {
			t.Skip("memory not created")
		}
		updatedSummary := "Longer operational guidance for persistence ownership."
		body := map[string]any{
			"content":    "The API service owns PostgreSQL schema migrations and backups.",
			"importance": 0.9,
			"summary":    updatedSummary,
		}
		b, _ := json.Marshal(body)
		req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/v1/memories/"+createdMemoryID, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", authHeader)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		memID, err := uuid.Parse(createdMemoryID)
		if err != nil {
			t.Fatalf("invalid memory_id %q: %v", createdMemoryID, err)
		}
		mem, err := db.GetMemory(ctx, pool, memID)
		if err != nil {
			t.Fatalf("GetMemory: %v", err)
		}
		if mem == nil {
			t.Fatal("expected updated memory to exist")
		}
		if mem.ScopeID != createdScopeID {
			t.Fatalf("scope_id = %v, want %v", mem.ScopeID, createdScopeID)
		}
		if mem.Summary == nil || *mem.Summary != updatedSummary {
			t.Fatalf("summary = %v, want %q", mem.Summary, updatedSummary)
		}
		var meta map[string]any
		if err := json.Unmarshal(mem.Meta, &meta); err != nil {
			t.Fatalf("memory meta is not valid JSON: %v", err)
		}
		if got, _ := meta["content_style"].(string); got != "long" {
			t.Fatalf("meta.content_style = %q, want %q", got, "long")
		}
	})

	t.Run("recall returns results", func(t *testing.T) {
		if createdMemoryID == "" {
			t.Skip("memory not created")
		}
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/memories/recall?q=PostgreSQL&scope=project:e2e-project", nil)
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

func TestScopes_CRUD(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()
	cfg := &config.Config{}

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "scopes-e2e-user")
	otherPrincipal := testhelper.CreateTestPrincipal(t, pool, "user", "scopes-e2e-owner2")
	rawToken, hashToken, err := auth.GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.CreateToken(ctx, pool, principal.ID, hashToken, "scopes-test-token", nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	handler := rest.NewRouter(pool, svc, cfg).Handler()
	srv := httptest.NewServer(handler)
	defer srv.Close()

	authHeader := "Bearer " + rawToken

	var createdScopeID string

	t.Run("create scope returns 201", func(t *testing.T) {
		body := map[string]any{
			"kind":         "project",
			"external_id":  "e2e-proj",
			"name":         "E2E Project",
			"principal_id": principal.ID.String(),
		}
		b, _ := json.Marshal(body)
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/scopes", bytes.NewReader(b))
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
		json.NewDecoder(resp.Body).Decode(&result)
		if result["ID"] == nil {
			t.Error("expected ID in response")
		}
		createdScopeID, _ = result["ID"].(string)
	})

	t.Run("list scopes returns 200", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/scopes", nil)
		req.Header.Set("Authorization", authHeader)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
		var result map[string]any
		json.NewDecoder(resp.Body).Decode(&result)
		if result["scopes"] == nil {
			t.Error("expected scopes key in response")
		}
	})

	t.Run("get scope returns 200", func(t *testing.T) {
		if createdScopeID == "" {
			t.Skip("scope not created")
		}
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/scopes/"+createdScopeID, nil)
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

	t.Run("update scope returns 200", func(t *testing.T) {
		if createdScopeID == "" {
			t.Skip("scope not created")
		}
		body := map[string]any{"name": "E2E Project Updated"}
		b, _ := json.Marshal(body)
		req, _ := http.NewRequest(http.MethodPut, srv.URL+"/v1/scopes/"+createdScopeID, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
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

	t.Run("update scope owner returns 200", func(t *testing.T) {
		if createdScopeID == "" {
			t.Skip("scope not created")
		}
		body := map[string]any{"principal_id": otherPrincipal.ID.String()}
		b, _ := json.Marshal(body)
		req, _ := http.NewRequest(http.MethodPut, srv.URL+"/v1/scopes/"+createdScopeID+"/owner", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
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
		gotOwner, _ := result["PrincipalID"].(string)
		if gotOwner != otherPrincipal.ID.String() {
			t.Fatalf("PrincipalID = %q, want %q", gotOwner, otherPrincipal.ID.String())
		}
	})

	t.Run("get nonexistent scope returns 404", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/scopes/00000000-0000-0000-0000-000000000099", nil)
		req.Header.Set("Authorization", authHeader)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404, got %d", resp.StatusCode)
		}
	})

	t.Run("delete scope returns 403 after ownership transfer", func(t *testing.T) {
		if createdScopeID == "" {
			t.Skip("scope not created")
		}
		req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/v1/scopes/"+createdScopeID, nil)
		req.Header.Set("Authorization", authHeader)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403, got %d", resp.StatusCode)
		}
	})

	t.Run("sync nonexistent scope returns 404", func(t *testing.T) {
		nonexistentID := uuid.New().String()
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/scopes/"+nonexistentID+"/repo/sync", nil)
		req.Header.Set("Authorization", authHeader)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404, got %d", resp.StatusCode)
		}
	})
}
