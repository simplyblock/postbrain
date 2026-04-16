package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/codegraph"
	"github.com/simplyblock/postbrain/internal/config"
)

// TestListScopes_NoAuth_Returns401 verifies unauthenticated requests to GET /v1/scopes return 401.
func TestListScopes_NoAuth_Returns401(t *testing.T) {
	r := newTestRouter()
	handler := r.Handler()

	req := httptest.NewRequest(http.MethodGet, "/v1/scopes", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("could not parse body: %v", err)
	}
	if _, ok := body["error"]; !ok {
		t.Error("expected 'error' key in 401 response")
	}
}

// TestCreateScope_NoAuth_Returns401 verifies unauthenticated POST /v1/scopes returns 401.
func TestCreateScope_NoAuth_Returns401(t *testing.T) {
	r := newTestRouter()
	handler := r.Handler()

	req := httptest.NewRequest(http.MethodPost, "/v1/scopes", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// TestGetScope_InvalidUUID_Returns400 verifies that a non-UUID {id} is rejected before DB access.
func TestGetScope_InvalidUUID_Returns400(t *testing.T) {
	t.Parallel()
	ro := &Router{}
	req := requestWithChiParam(t, "id", "not-a-uuid")
	w := httptest.NewRecorder()

	ro.getScope(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	assertJSONError(t, w)
}

// TestGetScope_NoAuth_Returns401 verifies unauthenticated GET /v1/scopes/{id} returns 401.
func TestGetScope_NoAuth_Returns401(t *testing.T) {
	r := newTestRouter()
	handler := r.Handler()

	req := httptest.NewRequest(http.MethodGet, "/v1/scopes/00000000-0000-0000-0000-000000000001", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// TestUpdateScope_NoAuth_Returns401 verifies unauthenticated PUT /v1/scopes/{id} returns 401.
func TestUpdateScope_NoAuth_Returns401(t *testing.T) {
	r := newTestRouter()
	handler := r.Handler()

	req := httptest.NewRequest(http.MethodPut, "/v1/scopes/00000000-0000-0000-0000-000000000001", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// TestDeleteScope_NoAuth_Returns401 verifies unauthenticated DELETE /v1/scopes/{id} returns 401.
func TestDeleteScope_NoAuth_Returns401(t *testing.T) {
	r := newTestRouter()
	handler := r.Handler()

	req := httptest.NewRequest(http.MethodDelete, "/v1/scopes/00000000-0000-0000-0000-000000000001", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// TestUpdateScopeOwner_NoAuth_Returns401 verifies unauthenticated
// PUT /v1/scopes/{id}/owner returns 401.
func TestUpdateScopeOwner_NoAuth_Returns401(t *testing.T) {
	r := newTestRouter()
	handler := r.Handler()

	req := httptest.NewRequest(http.MethodPut, "/v1/scopes/00000000-0000-0000-0000-000000000001/owner", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// TestSetScopeRepo_MissingRepoURL_Returns400 verifies that omitting repo_url
// in the POST /v1/scopes/:id/repo body returns 400 before any DB call.
func TestSetScopeRepo_MissingRepoURL_Returns400(t *testing.T) {
	ro := &Router{}

	req := httptest.NewRequest(http.MethodPost, "/",
		strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", uuid.New().String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()

	ro.setScopeRepo(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if _, ok := body["error"]; !ok {
		t.Error("expected 'error' key in 400 response body")
	}
}

// TestGetSyncStatus_InvalidUUID_Returns400 verifies that a non-UUID {id} returns 400.
func TestGetSyncStatus_InvalidUUID_Returns400(t *testing.T) {
	t.Parallel()
	ro := &Router{syncer: codegraph.NewSyncer(config.CodeGraphConfig{})}
	req := requestWithChiParam(t, "id", "not-a-uuid")
	w := httptest.NewRecorder()

	ro.getSyncStatus(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	assertJSONError(t, w)
}

// TestGetSyncStatus_NoAuth_Returns401 verifies that unauthenticated requests
// to GET /v1/scopes/{id}/repo/sync return 401.
func TestGetSyncStatus_NoAuth_Returns401(t *testing.T) {
	t.Parallel()
	r := newTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/v1/scopes/"+uuid.New().String()+"/repo/sync", nil)
	w := httptest.NewRecorder()

	r.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

// TestUpdateScopeOwner_MissingPrincipalID_Returns400 verifies that omitting
// principal_id in PUT /v1/scopes/:id/owner returns 400 before DB access.
func TestUpdateScopeOwner_MissingPrincipalID_Returns400(t *testing.T) {
	ro := &Router{}

	req := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", uuid.New().String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()

	ro.updateScopeOwner(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if _, ok := body["error"]; !ok {
		t.Error("expected 'error' key in 400 response body")
	}
}
