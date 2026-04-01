package rest

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// ── createCollection ──────────────────────────────────────────────────────────

func TestCreateCollection_MissingFields_Returns400(t *testing.T) {
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

			ro.createCollection(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
			}
			assertJSONError(t, w)
		})
	}
}

func TestCreateCollection_InvalidScopeFormat_Returns400(t *testing.T) {
	t.Parallel()
	ro := &Router{}
	req := httptest.NewRequest(http.MethodPost, "/",
		strings.NewReader(`{"scope":"nocolon","slug":"s","name":"n"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	ro.createCollection(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	assertJSONError(t, w)
}

func TestCreateCollection_InvalidJSON_Returns400(t *testing.T) {
	t.Parallel()
	ro := &Router{}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`not-json`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	ro.createCollection(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestCreateCollection_NoAuth_Returns401(t *testing.T) {
	t.Parallel()
	r := newTestRouter()
	req := httptest.NewRequest(http.MethodPost, "/v1/collections",
		strings.NewReader(`{"scope":"team:eng","slug":"s","name":"n"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

// ── listCollections ───────────────────────────────────────────────────────────

func TestListCollections_InvalidScopeFormat_Returns400(t *testing.T) {
	t.Parallel()
	ro := &Router{}
	req := httptest.NewRequest(http.MethodGet, "/?scope=nocolon", nil)
	w := httptest.NewRecorder()

	ro.listCollections(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	assertJSONError(t, w)
}

func TestListCollections_NoAuth_Returns401(t *testing.T) {
	t.Parallel()
	r := newTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/v1/collections", nil)
	w := httptest.NewRecorder()

	r.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

// ── getCollection ─────────────────────────────────────────────────────────────

func TestGetCollection_SlugWithoutScope_Returns400(t *testing.T) {
	t.Parallel()
	ro := &Router{}
	// "not-a-uuid" is treated as a slug; no ?scope param → 400.
	req := requestWithChiParam(t, "slug", "my-collection")
	w := httptest.NewRecorder()

	ro.getCollection(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	assertJSONError(t, w)
}

func TestGetCollection_SlugWithInvalidScopeFormat_Returns400(t *testing.T) {
	t.Parallel()
	ro := &Router{}
	req := requestWithChiParam(t, "slug", "my-collection")
	req.URL.RawQuery = "scope=nocolon"
	w := httptest.NewRecorder()

	ro.getCollection(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	assertJSONError(t, w)
}

func TestGetCollection_NoAuth_Returns401(t *testing.T) {
	t.Parallel()
	r := newTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/v1/collections/my-collection", nil)
	w := httptest.NewRecorder()

	r.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

// ── addCollectionItem ─────────────────────────────────────────────────────────

func TestAddCollectionItem_InvalidCollectionID_Returns400(t *testing.T) {
	t.Parallel()
	ro := &Router{}
	req := requestWithChiParam(t, "id", "not-a-uuid")
	req = withBody(req, `{"artifact_id":"`+uuid.New().String()+`"}`)
	w := httptest.NewRecorder()

	ro.addCollectionItem(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	assertJSONError(t, w)
}

func TestAddCollectionItem_InvalidArtifactID_Returns400(t *testing.T) {
	t.Parallel()
	ro := &Router{}
	req := requestWithChiParam(t, "id", uuid.New().String())
	req = withBody(req, `{"artifact_id":"not-a-uuid"}`)
	w := httptest.NewRecorder()

	ro.addCollectionItem(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	assertJSONError(t, w)
}

func TestAddCollectionItem_InvalidJSON_Returns400(t *testing.T) {
	t.Parallel()
	ro := &Router{}
	req := requestWithChiParam(t, "id", uuid.New().String())
	req = withBody(req, "not-json")
	w := httptest.NewRecorder()

	ro.addCollectionItem(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestAddCollectionItem_NoAuth_Returns401(t *testing.T) {
	t.Parallel()
	r := newTestRouter()
	req := httptest.NewRequest(http.MethodPost,
		"/v1/collections/"+uuid.New().String()+"/items",
		strings.NewReader(`{"artifact_id":"`+uuid.New().String()+`"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

// ── removeCollectionItem ──────────────────────────────────────────────────────

func TestRemoveCollectionItem_InvalidCollectionID_Returns400(t *testing.T) {
	t.Parallel()
	ro := &Router{}
	req := requestWithChiParam(t, "id", "not-a-uuid")
	// artifact_id param not set — UUID parse on id fails first.
	w := httptest.NewRecorder()

	ro.removeCollectionItem(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	assertJSONError(t, w)
}

func TestRemoveCollectionItem_InvalidArtifactID_Returns400(t *testing.T) {
	t.Parallel()
	ro := &Router{}
	// Valid collection id, invalid artifact_id param.
	rctx, req := twoChiParams(t, "id", uuid.New().String(), "artifact_id", "not-a-uuid")
	_ = rctx
	w := httptest.NewRecorder()

	ro.removeCollectionItem(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	assertJSONError(t, w)
}

func TestRemoveCollectionItem_NoAuth_Returns401(t *testing.T) {
	t.Parallel()
	r := newTestRouter()
	req := httptest.NewRequest(http.MethodDelete,
		"/v1/collections/"+uuid.New().String()+"/items/"+uuid.New().String(), nil)
	w := httptest.NewRecorder()

	r.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}
