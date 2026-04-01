package rest

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// ── synthesizeKnowledge ───────────────────────────────────────────────────────

func TestSynthesizeKnowledge_InvalidJSON_Returns400(t *testing.T) {
	t.Parallel()
	ro := &Router{}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`not-json`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	ro.synthesizeKnowledge(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestSynthesizeKnowledge_MissingScopeID_Returns400(t *testing.T) {
	t.Parallel()
	ro := &Router{}
	body := `{"source_ids":["` + uuid.New().String() + `","` + uuid.New().String() + `"]}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	ro.synthesizeKnowledge(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	assertJSONError(t, w)
}

func TestSynthesizeKnowledge_InvalidScopeID_Returns400(t *testing.T) {
	t.Parallel()
	ro := &Router{}
	body := `{"scope_id":"not-a-uuid","source_ids":["` + uuid.New().String() + `","` + uuid.New().String() + `"]}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	ro.synthesizeKnowledge(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	assertJSONError(t, w)
}

func TestSynthesizeKnowledge_TooFewSourceIDs_Returns400(t *testing.T) {
	t.Parallel()
	ro := &Router{}
	body := `{"scope_id":"` + uuid.New().String() + `","source_ids":["` + uuid.New().String() + `"]}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	ro.synthesizeKnowledge(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	assertJSONError(t, w)
}

func TestSynthesizeKnowledge_InvalidSourceID_Returns400(t *testing.T) {
	t.Parallel()
	ro := &Router{}
	body := `{"scope_id":"` + uuid.New().String() + `","source_ids":["` + uuid.New().String() + `","not-a-uuid"]}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	ro.synthesizeKnowledge(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	assertJSONError(t, w)
}

func TestSynthesizeKnowledge_NoAuth_Returns401(t *testing.T) {
	t.Parallel()
	r := newTestRouter()
	body := `{"scope_id":"` + uuid.New().String() + `","source_ids":["` + uuid.New().String() + `","` + uuid.New().String() + `"]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/knowledge/synthesize", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

// ── getArtifactSources ────────────────────────────────────────────────────────

func TestGetArtifactSources_InvalidUUID_Returns400(t *testing.T) {
	t.Parallel()
	ro := &Router{}
	req := requestWithChiParam(t, "id", "not-a-uuid")
	w := httptest.NewRecorder()

	ro.getArtifactSources(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	assertJSONError(t, w)
}

func TestGetArtifactSources_NoAuth_Returns401(t *testing.T) {
	t.Parallel()
	r := newTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/v1/knowledge/"+uuid.New().String()+"/sources", nil)
	w := httptest.NewRecorder()

	r.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

// ── getArtifactDigests ────────────────────────────────────────────────────────

func TestGetArtifactDigests_InvalidUUID_Returns400(t *testing.T) {
	t.Parallel()
	ro := &Router{}
	req := requestWithChiParam(t, "id", "not-a-uuid")
	w := httptest.NewRecorder()

	ro.getArtifactDigests(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	assertJSONError(t, w)
}

func TestGetArtifactDigests_NoAuth_Returns401(t *testing.T) {
	t.Parallel()
	r := newTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/v1/knowledge/"+uuid.New().String()+"/digests", nil)
	w := httptest.NewRecorder()

	r.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}
