package rest

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// ── createSession ─────────────────────────────────────────────────────────────

func TestCreateSession_MissingScope_Returns400(t *testing.T) {
	t.Parallel()
	ro := &Router{}
	req := httptest.NewRequest(http.MethodPost, "/",
		strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	ro.createSession(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	assertJSONError(t, w)
}

func TestCreateSession_InvalidScopeFormat_Returns400(t *testing.T) {
	t.Parallel()
	ro := &Router{}
	req := httptest.NewRequest(http.MethodPost, "/",
		strings.NewReader(`{"scope":"nocolon"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	ro.createSession(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	assertJSONError(t, w)
}

func TestCreateSession_InvalidJSON_Returns400(t *testing.T) {
	t.Parallel()
	ro := &Router{}
	req := httptest.NewRequest(http.MethodPost, "/",
		strings.NewReader(`not-json`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	ro.createSession(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestCreateSession_NoAuth_Returns401(t *testing.T) {
	t.Parallel()
	r := newTestRouter()
	req := httptest.NewRequest(http.MethodPost, "/v1/sessions",
		strings.NewReader(`{"scope":"team:eng"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

// ── updateSession ─────────────────────────────────────────────────────────────

func TestUpdateSession_InvalidUUID_Returns400(t *testing.T) {
	t.Parallel()
	ro := &Router{}
	req := requestWithChiParam(t, "id", "not-a-uuid")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	ro.updateSession(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	assertJSONError(t, w)
}

func TestUpdateSession_InvalidJSON_Returns400(t *testing.T) {
	t.Parallel()
	ro := &Router{}
	req := requestWithChiParam(t, "id", uuid.New().String())
	req = withBody(req, "not-json")
	w := httptest.NewRecorder()

	ro.updateSession(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestUpdateSession_InvalidEndedAt_Returns400(t *testing.T) {
	t.Parallel()
	ro := &Router{}
	req := requestWithChiParam(t, "id", uuid.New().String())
	req = withBody(req, `{"ended_at":"not-a-timestamp"}`)
	w := httptest.NewRecorder()

	ro.updateSession(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	assertJSONError(t, w)
}

func TestUpdateSession_NoAuth_Returns401(t *testing.T) {
	t.Parallel()
	r := newTestRouter()
	req := httptest.NewRequest(http.MethodPatch, "/v1/sessions/"+uuid.New().String(),
		strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}
