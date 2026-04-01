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
