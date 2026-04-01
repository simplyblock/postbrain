package rest

import (
	"bytes"
	"mime/multipart"
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

// multipartUpload builds a multipart/form-data request with an optional file
// and optional extra form fields. Pass an empty filename to omit the file part.
func multipartUpload(t *testing.T, filename, fileContent string, fields map[string]string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	if filename != "" {
		fw, err := mw.CreateFormFile("file", filename)
		if err != nil {
			t.Fatalf("CreateFormFile: %v", err)
		}
		if _, err := fw.Write([]byte(fileContent)); err != nil {
			t.Fatalf("write file content: %v", err)
		}
	}
	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			t.Fatalf("WriteField %s: %v", k, err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

// ── uploadKnowledge ───────────────────────────────────────────────────────────

func TestUploadKnowledge_NotMultipart_Returns400(t *testing.T) {
	t.Parallel()
	ro := &Router{}
	req := httptest.NewRequest(http.MethodPost, "/",
		bytes.NewBufferString("not-a-multipart-body"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	ro.uploadKnowledge(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	assertJSONError(t, w)
}

func TestUploadKnowledge_MissingFileField_Returns400(t *testing.T) {
	t.Parallel()
	ro := &Router{}
	// Multipart form with no "file" part.
	req := multipartUpload(t, "", "", map[string]string{"scope": "team:eng"})
	w := httptest.NewRecorder()

	ro.uploadKnowledge(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	assertJSONError(t, w)
}

func TestUploadKnowledge_UnsupportedFileType_Returns400(t *testing.T) {
	t.Parallel()
	ro := &Router{}
	req := multipartUpload(t, "notes.xyz", "some content", map[string]string{"scope": "team:eng"})
	w := httptest.NewRecorder()

	ro.uploadKnowledge(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	assertJSONError(t, w)
}

func TestUploadKnowledge_EmptyTextContent_Returns400(t *testing.T) {
	t.Parallel()
	ro := &Router{}
	// .txt file with blank content → extracted text is empty.
	req := multipartUpload(t, "notes.txt", "   \n\t  ", map[string]string{"scope": "team:eng"})
	w := httptest.NewRecorder()

	ro.uploadKnowledge(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	assertJSONError(t, w)
}

func TestUploadKnowledge_MissingScope_Returns400(t *testing.T) {
	t.Parallel()
	ro := &Router{}
	req := multipartUpload(t, "notes.txt", "hello world", nil)
	w := httptest.NewRecorder()

	ro.uploadKnowledge(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	assertJSONError(t, w)
}

func TestUploadKnowledge_InvalidScopeFormat_Returns400(t *testing.T) {
	t.Parallel()
	ro := &Router{}
	req := multipartUpload(t, "notes.txt", "hello world", map[string]string{"scope": "nocolon"})
	w := httptest.NewRecorder()

	ro.uploadKnowledge(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	assertJSONError(t, w)
}
