package rest

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// ── createPrincipal ───────────────────────────────────────────────────────────

func TestCreatePrincipal_MissingSlug_Returns400(t *testing.T) {
	t.Parallel()
	ro := &Router{}
	req := httptest.NewRequest(http.MethodPost, "/",
		strings.NewReader(`{"kind":"user","display_name":"Alice"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	ro.createPrincipal(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	assertJSONError(t, w)
}

func TestCreatePrincipal_MissingKind_Returns400(t *testing.T) {
	t.Parallel()
	ro := &Router{}
	req := httptest.NewRequest(http.MethodPost, "/",
		strings.NewReader(`{"slug":"alice","display_name":"Alice"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	ro.createPrincipal(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	assertJSONError(t, w)
}

func TestCreatePrincipal_InvalidJSON_Returns400(t *testing.T) {
	t.Parallel()
	ro := &Router{}
	req := httptest.NewRequest(http.MethodPost, "/",
		strings.NewReader(`not-json`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	ro.createPrincipal(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// ── listPrincipals ────────────────────────────────────────────────────────────

func TestListPrincipals_NoAuth_Returns401(t *testing.T) {
	t.Parallel()
	r := newTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/v1/principals", nil)
	w := httptest.NewRecorder()

	r.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

// ── getPrincipal ──────────────────────────────────────────────────────────────

func TestGetPrincipal_InvalidUUID_Returns400(t *testing.T) {
	t.Parallel()
	ro := &Router{}
	req := requestWithChiParam(t, "id", "not-a-uuid")
	w := httptest.NewRecorder()

	ro.getPrincipal(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	assertJSONError(t, w)
}

// ── updatePrincipal ───────────────────────────────────────────────────────────

func TestUpdatePrincipal_InvalidUUID_Returns400(t *testing.T) {
	t.Parallel()
	ro := &Router{}
	req := requestWithChiParam(t, "id", "not-a-uuid")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	ro.updatePrincipal(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	assertJSONError(t, w)
}

func TestUpdatePrincipal_InvalidJSON_Returns400(t *testing.T) {
	t.Parallel()
	ro := &Router{}
	req := requestWithChiParam(t, "id", uuid.New().String())
	req = withBody(req, "not-json")
	w := httptest.NewRecorder()

	ro.updatePrincipal(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// ── deletePrincipal ───────────────────────────────────────────────────────────

func TestDeletePrincipal_InvalidUUID_Returns400(t *testing.T) {
	t.Parallel()
	ro := &Router{}
	req := requestWithChiParam(t, "id", "not-a-uuid")
	w := httptest.NewRecorder()

	ro.deletePrincipal(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	assertJSONError(t, w)
}

// ── listMembers ───────────────────────────────────────────────────────────────

func TestListMembers_InvalidUUID_Returns400(t *testing.T) {
	t.Parallel()
	ro := &Router{}
	req := requestWithChiParam(t, "id", "not-a-uuid")
	w := httptest.NewRecorder()

	ro.listMembers(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	assertJSONError(t, w)
}

func TestListMembers_NoAuth_Returns401(t *testing.T) {
	t.Parallel()
	r := newTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/v1/principals/"+uuid.New().String()+"/members", nil)
	w := httptest.NewRecorder()

	r.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}
