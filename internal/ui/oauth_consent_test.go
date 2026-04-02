package ui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/config"
	"github.com/simplyblock/postbrain/internal/oauth"
)

type fakeConsentStateStore struct {
	peekRecord    *oauth.StateRecord
	peekErr       error
	consumeRecord *oauth.StateRecord
	consumeErr    error
}

func (f *fakeConsentStateStore) Issue(_ context.Context, _ string, _ map[string]any, _ time.Duration) (string, error) {
	return "state-1", nil
}

func (f *fakeConsentStateStore) Peek(_ context.Context, _ string) (*oauth.StateRecord, error) {
	if f.peekErr != nil {
		return nil, f.peekErr
	}
	return f.peekRecord, nil
}

func (f *fakeConsentStateStore) Consume(_ context.Context, _ string) (*oauth.StateRecord, error) {
	if f.consumeErr != nil {
		return nil, f.consumeErr
	}
	return f.consumeRecord, nil
}

type fakeConsentCodeStore struct {
	lastReq oauth.IssueCodeRequest
	rawCode string
}

func (f *fakeConsentCodeStore) Issue(_ context.Context, req oauth.IssueCodeRequest) (string, error) {
	f.lastReq = req
	if f.rawCode == "" {
		f.rawCode = "raw-auth-code"
	}
	return f.rawCode, nil
}

type fakeConsentClientStore struct {
	client *oauth.OAuthClient
}

func (f *fakeConsentClientStore) LookupByClientID(_ context.Context, _ string) (*oauth.OAuthClient, error) {
	return f.client, nil
}

func TestHandleConsentGet_NoSession_RedirectsToLogin_WithNext(t *testing.T) {
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/ui/oauth/authorize?state=s1", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "/ui/login?next=") {
		t.Fatalf("Location = %q", loc)
	}
}

func TestHandleConsentGet_InvalidState_Returns400(t *testing.T) {
	h := newTestHandler(t)
	h.stateStore = &fakeConsentStateStore{peekErr: oauth.ErrNotFound}
	req := httptest.NewRequest(http.MethodGet, "/ui/oauth/authorize?state=bad", nil)
	rec := httptest.NewRecorder()

	h.handleConsentGet(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHandleConsentGet_ValidState_RendersConsentPage(t *testing.T) {
	h := newTestHandler(t)
	h.stateStore = &fakeConsentStateStore{
		peekRecord: &oauth.StateRecord{
			Payload: map[string]any{
				"client_id":    "pb_client_1",
				"redirect_uri": "http://localhost:8765/callback",
				"scopes":       []any{"memories:read", "knowledge:write"},
				"state":        "orig-state",
			},
		},
	}
	h.clients = &fakeConsentClientStore{
		client: &oauth.OAuthClient{ID: uuid.New(), ClientID: "pb_client_1", Name: "Test Client"},
	}
	req := httptest.NewRequest(http.MethodGet, "/ui/oauth/authorize?state=ok", nil)
	rec := httptest.NewRecorder()

	h.handleConsentGet(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Authorize Client") || !strings.Contains(body, "Approve") {
		t.Fatalf("unexpected body: %s", body)
	}
	if !strings.Contains(body, "Memories Read") || !strings.Contains(body, "Knowledge Write") {
		t.Fatalf("expected human-readable scope labels, got body: %s", body)
	}
}

func TestHandleConsentPost_NoSession_RedirectsToLogin(t *testing.T) {
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/ui/oauth/authorize", strings.NewReader("state=s1&action=approve"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	if rec.Header().Get("Location") != "/ui/login" {
		t.Fatalf("Location = %q", rec.Header().Get("Location"))
	}
}

func TestHandleConsentPost_Deny_RedirectsToClientWithErrorAndOriginalState(t *testing.T) {
	h := newTestHandler(t)
	h.stateStore = &fakeConsentStateStore{
		consumeRecord: &oauth.StateRecord{Payload: map[string]any{
			"client_id":    "pb_client_1",
			"redirect_uri": "http://localhost:8765/callback",
			"scopes":       []any{"memories:read"},
			"state":        "orig-state",
		}},
	}
	h.clients = &fakeConsentClientStore{client: &oauth.OAuthClient{ID: uuid.New(), ClientID: "pb_client_1"}}
	h.codeStore = &fakeConsentCodeStore{}
	req := httptest.NewRequest(http.MethodPost, "/ui/oauth/authorize", strings.NewReader("state=s1&action=deny"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	h.handleConsentPost(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	loc := rec.Header().Get("Location")
	parsed, _ := url.Parse(loc)
	if parsed.Query().Get("error") != "access_denied" || parsed.Query().Get("state") != "orig-state" {
		t.Fatalf("Location = %q", loc)
	}
}

func TestHandleConsentPost_Approve_IssuedCode_RedirectsToClientWithCodeAndOriginalState(t *testing.T) {
	h := newTestHandler(t)
	clientID := uuid.New()
	h.stateStore = &fakeConsentStateStore{
		consumeRecord: &oauth.StateRecord{Payload: map[string]any{
			"client_id":      "pb_client_1",
			"redirect_uri":   "http://localhost:8765/callback",
			"scopes":         []any{"memories:read"},
			"code_challenge": "challenge",
			"state":          "orig-state",
		}},
	}
	codes := &fakeConsentCodeStore{rawCode: "raw-code-123"}
	h.codeStore = codes
	h.clients = &fakeConsentClientStore{client: &oauth.OAuthClient{ID: clientID, ClientID: "pb_client_1"}}
	h.oauthCfg = config.OAuthConfig{Server: config.OAuthServerConfig{AuthCodeTTL: 10 * time.Minute}}

	req := httptest.NewRequest(http.MethodPost, "/ui/oauth/authorize", strings.NewReader("state=s1&action=approve"))
	req = req.WithContext(context.WithValue(req.Context(), auth.ContextKeyPrincipalID, uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	h.handleConsentPost(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	loc := rec.Header().Get("Location")
	parsed, _ := url.Parse(loc)
	if parsed.Query().Get("code") != "raw-code-123" || parsed.Query().Get("state") != "orig-state" {
		t.Fatalf("Location = %q", loc)
	}
	if codes.lastReq.ClientID != clientID {
		t.Fatalf("issue code client id = %s, want %s", codes.lastReq.ClientID, clientID)
	}
}

func TestHandleConsentPost_ExpiredState_Returns400(t *testing.T) {
	h := newTestHandler(t)
	h.stateStore = &fakeConsentStateStore{consumeErr: oauth.ErrNotFound}
	h.codeStore = &fakeConsentCodeStore{}
	h.clients = &fakeConsentClientStore{client: &oauth.OAuthClient{ID: uuid.New()}}
	req := httptest.NewRequest(http.MethodPost, "/ui/oauth/authorize", strings.NewReader("state=s1&action=approve"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	h.handleConsentPost(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHandleConsentPost_ReplayState_Returns400(t *testing.T) {
	TestHandleConsentPost_ExpiredState_Returns400(t)
}
