package rest

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// ── createSkill ───────────────────────────────────────────────────────────────

func TestCreateSkill_MissingFields_Returns400(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		body string
	}{
		{"missing scope", `{"slug":"s","name":"n"}`},
		{"missing slug", `{"scope":"team:eng","name":"n"}`},
		{"missing name", `{"scope":"team:eng","slug":"s"}`},
		{"empty body", `{}`},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ro := &Router{}
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			ro.createSkill(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
			}
			assertJSONError(t, w)
		})
	}
}

// TestCreateSkill_TraversalSlug_Returns400 is a regression test: a slug
// containing path-traversal characters must be rejected at creation so it
// can never be persisted and later used in Install to escape the base dir.
func TestCreateSkill_TraversalSlug_Returns400(t *testing.T) {
	t.Parallel()
	dangerous := []string{
		"../../etc/passwd",
		"../evil",
		"/absolute/path",
		"has space",
		"has.dot",
		"UPPERCASE",
	}
	for _, slug := range dangerous {
		slug := slug
		t.Run(slug, func(t *testing.T) {
			t.Parallel()
			ro := &Router{}
			body := `{"scope":"team:eng","slug":"` + slug + `","name":"n"}`
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			ro.createSkill(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("createSkill with slug %q: status = %d, want 400", slug, w.Code)
			}
			assertJSONError(t, w)
		})
	}
}

func TestCreateSkill_InvalidScopeFormat_Returns400(t *testing.T) {
	t.Parallel()
	ro := &Router{}
	req := httptest.NewRequest(http.MethodPost, "/",
		strings.NewReader(`{"scope":"nocolon","slug":"s","name":"n"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	ro.createSkill(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	assertJSONError(t, w)
}

func TestCreateSkill_InvalidJSON_Returns400(t *testing.T) {
	t.Parallel()
	ro := &Router{}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`not-json`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	ro.createSkill(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestCreateSkill_NoAuth_Returns401(t *testing.T) {
	t.Parallel()
	r := newTestRouter()
	req := httptest.NewRequest(http.MethodPost, "/v1/skills",
		strings.NewReader(`{"scope":"team:eng","slug":"s","name":"n"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

// ── searchSkills ──────────────────────────────────────────────────────────────

func TestSearchSkills_InvalidScopeFormat_Returns400(t *testing.T) {
	t.Parallel()
	ro := &Router{}
	req := httptest.NewRequest(http.MethodGet, "/?scope=nocolon", nil)
	w := httptest.NewRecorder()

	ro.searchSkills(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	assertJSONError(t, w)
}

func TestSearchSkills_NoEmbeddingService_Returns503(t *testing.T) {
	t.Parallel()
	ro := &Router{} // svc is nil
	req := httptest.NewRequest(http.MethodGet, "/?q=something", nil)
	w := httptest.NewRecorder()

	ro.searchSkills(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
	assertJSONError(t, w)
}

func TestSearchSkills_NoAuth_Returns401(t *testing.T) {
	t.Parallel()
	r := newTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/v1/skills/search?q=test", nil)
	w := httptest.NewRecorder()

	r.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

// ── getSkill ──────────────────────────────────────────────────────────────────

func TestGetSkill_InvalidUUID_Returns400(t *testing.T) {
	t.Parallel()
	ro := &Router{}
	req := requestWithChiParam(t, "id", "not-a-uuid")
	w := httptest.NewRecorder()

	ro.getSkill(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	assertJSONError(t, w)
}

func TestGetSkill_NoAuth_Returns401(t *testing.T) {
	t.Parallel()
	r := newTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/v1/skills/"+uuid.New().String(), nil)
	w := httptest.NewRecorder()

	r.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

// ── updateSkill ───────────────────────────────────────────────────────────────

func TestUpdateSkill_InvalidUUID_Returns400(t *testing.T) {
	t.Parallel()
	ro := &Router{}
	req := requestWithChiParam(t, "id", "not-a-uuid")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	ro.updateSkill(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	assertJSONError(t, w)
}

func TestUpdateSkill_InvalidJSON_Returns400(t *testing.T) {
	t.Parallel()
	ro := &Router{}
	req := requestWithChiParam(t, "id", uuid.New().String())
	req = withBody(req, "not-json")
	w := httptest.NewRecorder()

	ro.updateSkill(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestUpdateSkill_NoAuth_Returns401(t *testing.T) {
	t.Parallel()
	r := newTestRouter()
	req := httptest.NewRequest(http.MethodPatch, "/v1/skills/"+uuid.New().String(),
		strings.NewReader(`{"body":"new body"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

// ── endorseSkill ──────────────────────────────────────────────────────────────

func TestEndorseSkill_InvalidUUID_Returns400(t *testing.T) {
	t.Parallel()
	ro := &Router{}
	req := requestWithChiParam(t, "id", "not-a-uuid")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	ro.endorseSkill(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	assertJSONError(t, w)
}

func TestEndorseSkill_NoAuth_Returns401(t *testing.T) {
	t.Parallel()
	r := newTestRouter()
	req := httptest.NewRequest(http.MethodPost, "/v1/skills/"+uuid.New().String()+"/endorse",
		strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

// ── deprecateSkill ────────────────────────────────────────────────────────────

func TestDeprecateSkill_InvalidUUID_Returns400(t *testing.T) {
	t.Parallel()
	ro := &Router{}
	req := requestWithChiParam(t, "id", "not-a-uuid")
	w := httptest.NewRecorder()

	ro.deprecateSkill(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	assertJSONError(t, w)
}

func TestDeprecateSkill_NoAuth_Returns401(t *testing.T) {
	t.Parallel()
	r := newTestRouter()
	req := httptest.NewRequest(http.MethodPost, "/v1/skills/"+uuid.New().String()+"/deprecate", nil)
	w := httptest.NewRecorder()

	r.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

// ── installSkill ──────────────────────────────────────────────────────────────

func TestInstallSkill_InvalidUUID_Returns400(t *testing.T) {
	t.Parallel()
	ro := &Router{}
	req := requestWithChiParam(t, "id", "not-a-uuid")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	ro.installSkill(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	assertJSONError(t, w)
}

func TestInstallSkill_NoAuth_Returns401(t *testing.T) {
	t.Parallel()
	r := newTestRouter()
	req := httptest.NewRequest(http.MethodPost, "/v1/skills/"+uuid.New().String()+"/install",
		strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

// ── invokeSkill ───────────────────────────────────────────────────────────────

func TestInvokeSkill_InvalidUUID_Returns400(t *testing.T) {
	t.Parallel()
	ro := &Router{}
	req := requestWithChiParam(t, "id", "not-a-uuid")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	ro.invokeSkill(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	assertJSONError(t, w)
}

func TestInvokeSkill_NoAuth_Returns401(t *testing.T) {
	t.Parallel()
	r := newTestRouter()
	req := httptest.NewRequest(http.MethodPost, "/v1/skills/"+uuid.New().String()+"/invoke",
		strings.NewReader(`{"params":{}}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}
