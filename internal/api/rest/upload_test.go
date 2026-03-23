package rest

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUploadKnowledge_Unauthorized(t *testing.T) {
	ro := NewRouter(nil, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/knowledge/upload", nil)
	rec := httptest.NewRecorder()
	ro.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}
