package ui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// ── handleLogin ───────────────────────────────────────────────────────────────

func TestLoginGET_RendersForm(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/ui/login", nil)
	w := httptest.NewRecorder()

	h.handleLogin(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
}

func TestLoginGET_DoesNotRenderAppSidebar(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/ui/login", nil)
	w := httptest.NewRecorder()

	h.handleLogin(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if strings.Contains(body, "<nav>") {
		t.Fatalf("login page should not render app sidebar nav, got body: %s", body)
	}
	if !strings.Contains(body, "<h1>Sign in to Postbrain</h1>") {
		t.Fatalf("expected login heading in body, got: %s", body)
	}
}

func TestLoginGET_UsesStyledStandaloneLayout(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/ui/login", nil)
	w := httptest.NewRecorder()

	h.handleLogin(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if !strings.Contains(body, "/ui/static/pico.min.css") {
		t.Fatalf("expected login page to include stylesheet link, got: %s", body)
	}
	if !strings.Contains(body, "class=\"auth-body\"") {
		t.Fatalf("expected standalone auth layout body class, got: %s", body)
	}
}

func TestLoginGET_WithNext_RendersHiddenNextField(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/ui/login?next=%2Fui%2Foauth%2Fauthorize%3Fstate%3Dabc", nil)
	w := httptest.NewRecorder()

	h.handleLogin(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if !strings.Contains(body, "name=\"next\"") {
		t.Fatal("expected hidden next input in login form")
	}
	if !strings.Contains(body, "value=\"/ui/oauth/authorize?state=abc\"") {
		t.Fatalf("expected decoded next value in response body, got: %s", body)
	}
}

func TestLoginPOST_MissingToken_RendersFormWithError(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	// POST with no token field — must re-render the form (200), not 500.
	req := httptest.NewRequest(http.MethodPost, "/ui/login",
		strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	h.handleLogin(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d (should re-render form)", w.Code, http.StatusOK)
	}
	if !strings.Contains(w.Body.String(), "Token is required") {
		t.Error("expected 'Token is required' error in response body")
	}
}

func TestLoginPOST_EmptyToken_RendersFormWithError(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/ui/login",
		strings.NewReader("token="))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	h.handleLogin(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if !strings.Contains(w.Body.String(), "Token is required") {
		t.Error("expected 'Token is required' error in response body")
	}
}

func TestLoginPOST_NilPool_RendersServiceUnavailable(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t) // nil pool
	req := httptest.NewRequest(http.MethodPost, "/ui/login",
		strings.NewReader("token=pb_sometoken"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	h.handleLogin(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if !strings.Contains(w.Body.String(), "unavailable") {
		t.Error("expected service unavailable message in response body")
	}
}

// ── /ui/tokens — unauthenticated access ──────────────────────────────────────

func TestTokensGET_NoCookie_RedirectsToLogin(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/ui/tokens", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", w.Code, http.StatusSeeOther)
	}
	if loc := w.Header().Get("Location"); loc != "/ui/login" {
		t.Errorf("Location = %q, want %q", loc, "/ui/login")
	}
}

// ── handleCreateToken ─────────────────────────────────────────────────────────

func TestCreateToken_MissingName_RendersFormError(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/ui/tokens",
		strings.NewReader("name="))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	h.handleCreateToken(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d (should re-render form)", w.Code, http.StatusOK)
	}
	if !strings.Contains(w.Body.String(), "name is required") {
		t.Error("expected 'name is required' error in response body")
	}
}

func TestCreateToken_InvalidScopeID_RendersFormError(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/ui/tokens",
		strings.NewReader("name=mytoken&scope_ids=not-a-uuid"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	h.handleCreateToken(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if !strings.Contains(w.Body.String(), "invalid scope id") {
		t.Error("expected 'invalid scope id' error in response body")
	}
}

func TestCreateToken_InvalidExpiryDate_RendersFormError(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/ui/tokens",
		strings.NewReader("name=mytoken&expires_at=not-a-date"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	h.handleCreateToken(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if !strings.Contains(w.Body.String(), "invalid expiry date") {
		t.Error("expected 'invalid expiry date' error in response body")
	}
}

// ── handleRevokeToken ─────────────────────────────────────────────────────────

func TestRevokeToken_InvalidUUID_Returns400(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/ui/tokens/not-a-uuid/revoke", nil)
	w := httptest.NewRecorder()

	h.handleRevokeToken(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestRevokeToken_NilPool_Returns503(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t) // nil pool
	req := httptest.NewRequest(http.MethodPost,
		"/ui/tokens/"+uuid.New().String()+"/revoke", nil)
	w := httptest.NewRecorder()

	h.handleRevokeToken(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}
