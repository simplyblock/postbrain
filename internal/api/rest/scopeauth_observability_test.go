package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/api/scopeauth"
	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/db"
)

func TestWriteScopeAuthzError_LogsFieldsAndIncrementsMetric(t *testing.T) {
	t.Helper()

	principalID := uuid.New()
	tokenID := uuid.New()
	requestedScopeID := uuid.New()
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	req := httptest.NewRequest("GET", "/v1/test-scope-authz-observability", nil)
	req = req.WithContext(context.WithValue(req.Context(), loggerKey{}, logger))
	req = req.WithContext(context.WithValue(req.Context(), auth.ContextKeyPrincipalID, principalID))
	req = req.WithContext(context.WithValue(req.Context(), auth.ContextKeyToken, &db.Token{
		ID:          tokenID,
		PrincipalID: principalID,
	}))

	w := httptest.NewRecorder()
	writeScopeAuthzError(w, req, requestedScopeID, scopeauth.ErrTokenScopeDenied)

	if w.Code != 403 {
		t.Fatalf("status = %d, want 403", w.Code)
	}
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	errText, _ := out["error"].(string)
	if !strings.Contains(errText, "scope access denied") {
		t.Fatalf("error = %q, want scope access denied", errText)
	}

	logText := buf.String()
	for _, fragment := range []string{
		"scope access denied",
		"surface=rest",
		"endpoint=\"GET /v1/test-scope-authz-observability\"",
		"principal_id=" + principalID.String(),
		"token_id=" + tokenID.String(),
		"requested_scope_id=" + requestedScopeID.String(),
	} {
		if !strings.Contains(logText, fragment) {
			t.Fatalf("log missing %q in %q", fragment, logText)
		}
	}
}
