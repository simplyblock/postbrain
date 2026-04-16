package rest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestCreateMemory_MalformedJSON_Returns400 verifies that a syntactically
// invalid JSON body causes createMemory to return 400.
func TestCreateMemory_MalformedJSON_Returns400(t *testing.T) {
	ro := &Router{}

	req := httptest.NewRequest(http.MethodPost, "/v1/memories",
		strings.NewReader(`{not valid json`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	ro.createMemory(w, req)

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

// TestPromoteMemory_InvalidMemoryID_Returns400 verifies that a non-UUID path
// parameter causes promoteMemory to return 400 before any DB access.
func TestPromoteMemory_InvalidMemoryID_Returns400(t *testing.T) {
	ro := &Router{}
	req := requestWithChiParam(t, "id", "not-a-uuid")
	req = withBody(req, `{"target_scope":"project:x","target_visibility":"team"}`)
	w := httptest.NewRecorder()

	ro.promoteMemory(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	assertJSONError(t, w)
}

// TestPromoteMemory_MalformedJSON_Returns400 verifies that a syntactically
// invalid JSON body causes promoteMemory to return 400.
func TestPromoteMemory_MalformedJSON_Returns400(t *testing.T) {
	ro := &Router{}
	req := requestWithChiParam(t, "id", "11111111-1111-1111-1111-111111111111")
	req = withBody(req, `{not valid json`)
	w := httptest.NewRecorder()

	ro.promoteMemory(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	assertJSONError(t, w)
}

// TestPromoteMemory_MissingTargetFields_Returns400 verifies that a request
// without target_scope and target_visibility returns 400.
func TestPromoteMemory_MissingTargetFields_Returns400(t *testing.T) {
	ro := &Router{}
	req := requestWithChiParam(t, "id", "11111111-1111-1111-1111-111111111111")
	req = withBody(req, `{}`)
	w := httptest.NewRecorder()

	ro.promoteMemory(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	assertJSONError(t, w)
}

// TestRecallMemories_MissingQ_Returns400 verifies that a recall request
// without the required q parameter returns 400.
func TestRecallMemories_MissingQ_Returns400(t *testing.T) {
	ro := &Router{}

	req := httptest.NewRequest(http.MethodGet, "/v1/memories/recall", nil)
	w := httptest.NewRecorder()

	ro.recallMemories(w, req)

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
