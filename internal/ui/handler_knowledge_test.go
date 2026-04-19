package ui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/simplyblock/postbrain/internal/config"
	"github.com/simplyblock/postbrain/internal/db"
)

// newTestHandler creates a Handler with nil pool (no DB) for unit testing.
// Templates are still parsed and rendered; DB-dependent data is simply absent.
func newTestHandler(t *testing.T) *Handler {
	t.Helper()
	h, err := NewHandler(nil, nil, &config.Config{})
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	return h
}

// TestHandleKnowledge_NoScope_Renders200 verifies that GET /ui/knowledge without
// a scope parameter renders successfully (zero UUID = all scopes).
func TestHandleKnowledge_NoScope_Renders200(t *testing.T) {
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/ui/knowledge", nil)
	w := httptest.NewRecorder()

	h.handleKnowledge(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("expected HTML content-type, got %q", ct)
	}
}

// TestHandleKnowledge_WithScope_Renders200 verifies that GET /ui/knowledge?scope=<uuid>
// renders successfully and does not panic with a nil pool.
func TestHandleKnowledge_WithScope_Renders200(t *testing.T) {
	h := newTestHandler(t)
	scopeID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/ui/knowledge?scope="+scopeID.String(), nil)
	w := httptest.NewRecorder()

	h.handleKnowledge(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// TestHandleKnowledge_QueryAndStatus_Renders200 verifies that GET /ui/knowledge
// with q and status params renders successfully without touching the DB.
func TestHandleKnowledge_QueryAndStatus_Renders200(t *testing.T) {
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/ui/knowledge?q=foo&status=published", nil)
	w := httptest.NewRecorder()

	h.handleKnowledge(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "name=\"artifact_kind\"") {
		t.Error("expected upload dialog to include artifact_kind field")
	}
}

// ── handleKnowledgeNew ────────────────────────────────────────────────────────

func TestHandleKnowledgeNew_Renders200(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/ui/knowledge/new", nil)
	w := httptest.NewRecorder()

	h.handleKnowledgeNew(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	if !strings.Contains(w.Body.String(), "name=\"artifact_kind\"") {
		t.Error("expected create form to include artifact_kind field")
	}
}

func TestHandleKnowledgeNew_UnauthenticatedViaRouter_RedirectsToLogin(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/ui/knowledge/new", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", w.Code, http.StatusSeeOther)
	}
	if loc := w.Header().Get("Location"); loc != "/ui/login" {
		t.Errorf("Location = %q, want /ui/login", loc)
	}
}

// ── handleCreateKnowledge ─────────────────────────────────────────────────────

func TestHandleCreateKnowledge_MissingTitle_RendersFormError(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/ui/knowledge",
		strings.NewReader("scope_id="+uuid.New().String()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	h.handleCreateKnowledge(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d (form should re-render)", w.Code, http.StatusOK)
	}
	if !strings.Contains(w.Body.String(), "title") {
		t.Error("expected form error mentioning 'title'")
	}
}

func TestHandleCreateKnowledge_MissingScope_RendersFormError(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/ui/knowledge",
		strings.NewReader("title=My+Article"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	h.handleCreateKnowledge(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if !strings.Contains(w.Body.String(), "scope") {
		t.Error("expected form error mentioning 'scope'")
	}
}

func TestHandleCreateKnowledge_InvalidScopeID_RendersFormError(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/ui/knowledge",
		strings.NewReader("title=My+Article&scope_id=not-a-uuid"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	h.handleCreateKnowledge(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if !strings.Contains(w.Body.String(), "scope") {
		t.Error("expected form error mentioning 'scope'")
	}
}

func TestHandleCreateKnowledge_InvalidArtifactKind_RendersFormError(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/ui/knowledge",
		strings.NewReader("title=My+Article&artifact_kind=banana"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// Inject scope into context as dispatchScopedRoute would.
	fakeScope := &db.Scope{ID: uuid.New()}
	req = req.WithContext(context.WithValue(req.Context(), ctxKeyCurrentScope, fakeScope))
	w := httptest.NewRecorder()

	h.handleCreateKnowledge(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if !strings.Contains(w.Body.String(), "invalid artifact kind") {
		t.Error("expected form error mentioning invalid artifact kind")
	}
}

// ── handleKnowledgeDetail ─────────────────────────────────────────────────────

func TestHandleKnowledgeDetail_InvalidUUID_Returns404(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/ui/knowledge/not-a-uuid", nil)
	w := httptest.NewRecorder()

	h.handleKnowledgeDetail(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleKnowledgeDetail_NilPool_Returns404(t *testing.T) {
	t.Parallel()
	// With nil pool the artifact cannot be fetched; handler must return 404
	// rather than passing a nil *db.KnowledgeArtifact to the template (which
	// would trigger a template-execution panic → 500).
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet,
		"/ui/knowledge/00000000-0000-0000-0000-000000000001", nil)
	w := httptest.NewRecorder()

	h.handleKnowledgeDetail(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleKnowledgeDetail_UnauthenticatedViaRouter_RedirectsToLogin(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet,
		"/ui/knowledge/00000000-0000-0000-0000-000000000001", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", w.Code, http.StatusSeeOther)
	}
	if loc := w.Header().Get("Location"); loc != "/ui/login" {
		t.Errorf("Location = %q, want /ui/login", loc)
	}
}

// ── handleKnowledgeReview (submit for review) ─────────────────────────────────

func TestHandleKnowledgeReview_InvalidUUID_Returns400(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/ui/knowledge/not-a-uuid/review", nil)
	w := httptest.NewRecorder()

	h.handleKnowledgeReview(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleKnowledgeReview_UnauthenticatedViaRouter_RedirectsToLogin(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost,
		"/ui/knowledge/00000000-0000-0000-0000-000000000001/review", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", w.Code, http.StatusSeeOther)
	}
	if loc := w.Header().Get("Location"); loc != "/ui/login" {
		t.Errorf("Location = %q, want /ui/login", loc)
	}
}

// ── handleKnowledgeRetract ────────────────────────────────────────────────────

func TestHandleKnowledgeRetract_InvalidUUID_Returns400(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/ui/knowledge/not-a-uuid/retract", nil)
	w := httptest.NewRecorder()

	h.handleKnowledgeRetract(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleKnowledgeRetract_NilPool_Returns503(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost,
		"/ui/knowledge/00000000-0000-0000-0000-000000000001/retract", nil)
	w := httptest.NewRecorder()

	h.handleKnowledgeRetract(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestHandleKnowledgeRetract_UnauthenticatedViaRouter_RedirectsToLogin(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost,
		"/ui/knowledge/00000000-0000-0000-0000-000000000001/retract", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", w.Code, http.StatusSeeOther)
	}
	if loc := w.Header().Get("Location"); loc != "/ui/login" {
		t.Errorf("Location = %q, want /ui/login", loc)
	}
}

// ── handleEndorseArtifact ─────────────────────────────────────────────────────

func TestHandleEndorseArtifact_InvalidUUID_Returns400(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/ui/knowledge/not-a-uuid/endorse", nil)
	w := httptest.NewRecorder()

	h.handleEndorseArtifact(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleEndorseArtifact_UnauthenticatedViaRouter_RedirectsToLogin(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost,
		"/ui/knowledge/00000000-0000-0000-0000-000000000001/endorse", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", w.Code, http.StatusSeeOther)
	}
	if loc := w.Header().Get("Location"); loc != "/ui/login" {
		t.Errorf("Location = %q, want /ui/login", loc)
	}
}

// ── handleKnowledgeDeprecate ──────────────────────────────────────────────────

func TestHandleKnowledgeDeprecate_InvalidUUID_Returns400(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/ui/knowledge/not-a-uuid/deprecate", nil)
	w := httptest.NewRecorder()

	h.handleKnowledgeDeprecate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleKnowledgeDeprecate_UnauthenticatedViaRouter_RedirectsToLogin(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost,
		"/ui/knowledge/00000000-0000-0000-0000-000000000001/deprecate", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", w.Code, http.StatusSeeOther)
	}
	if loc := w.Header().Get("Location"); loc != "/ui/login" {
		t.Errorf("Location = %q, want /ui/login", loc)
	}
}

// ── handleKnowledgeDelete ─────────────────────────────────────────────────────

func TestHandleKnowledgeDelete_InvalidUUID_Returns400(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/ui/knowledge/not-a-uuid/delete", nil)
	w := httptest.NewRecorder()

	h.handleKnowledgeDelete(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleKnowledgeDelete_UnauthenticatedViaRouter_RedirectsToLogin(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost,
		"/ui/knowledge/00000000-0000-0000-0000-000000000001/delete", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", w.Code, http.StatusSeeOther)
	}
	if loc := w.Header().Get("Location"); loc != "/ui/login" {
		t.Errorf("Location = %q, want /ui/login", loc)
	}
}
