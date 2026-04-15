//go:build integration

package rest_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/api/rest"
	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/config"
	"github.com/simplyblock/postbrain/internal/db/compat"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

// TestUpdateSession_IDOR_CrossPrincipalDenied is a regression test for the
// IDOR in PATCH /v1/sessions/{id}.  Before the fix the handler called
// compat.EndSession(id, meta) using only the caller-supplied {id}, with no
// ownership check.  Any authenticated principal could end or alter another
// principal's session record by supplying an arbitrary {id}.
//
// After the fix the handler must load the session, verify
// session.PrincipalID == callerPrincipalID, and return 403 when they differ.
func TestUpdateSession_IDOR_CrossPrincipalDenied(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewTestPool(t)
	cfg := &config.Config{}
	svc := testhelper.NewMockEmbeddingService()

	// user-b creates a session under their own scope.
	principalB := testhelper.CreateTestPrincipal(t, pool, "user", "idor-sess-b-"+uuid.New().String())
	scopeB := testhelper.CreateTestScope(t, pool, "project", "idor-sess-scope-b-"+uuid.New().String(), nil, principalB.ID)

	sessionB, err := compat.CreateSession(ctx, pool, scopeB.ID, principalB.ID, nil)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	// user-a has their own scope and token — they do NOT own sessionB.
	principalA := testhelper.CreateTestPrincipal(t, pool, "user", "idor-sess-a-"+uuid.New().String())
	scopeA := testhelper.CreateTestScope(t, pool, "project", "idor-sess-scope-a-"+uuid.New().String(), nil, principalA.ID)

	rawToken, hashToken, err := auth.GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	_, err = compat.CreateToken(ctx, pool, principalA.ID, hashToken, "idor-sess-token-a", []uuid.UUID{scopeA.ID}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	handler := rest.NewRouter(pool, svc, cfg).Handler()
	srv := httptest.NewServer(handler)
	defer srv.Close()

	req, err := http.NewRequest(
		http.MethodPatch,
		srv.URL+"/v1/sessions/"+sessionB.ID.String(),
		strings.NewReader(`{"meta":{"closed_by":"attacker"}}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+rawToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	// user-a does not own sessionB; the handler must return 403.
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("IDOR: status = %d, want %d — user-a must not end user-b's session",
			resp.StatusCode, http.StatusForbidden)
	}

	// Verify sessionB was not ended.
	after, err := compat.GetSession(ctx, pool, sessionB.ID)
	if err != nil {
		t.Fatalf("get session after failed update: %v", err)
	}
	if after == nil {
		t.Fatal("session disappeared unexpectedly")
	}
	if after.EndedAt != nil {
		t.Error("IDOR: session was ended by unauthorized caller")
	}
}