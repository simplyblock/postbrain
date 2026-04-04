//go:build integration

package ui_test

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/testhelper"
	uiapi "github.com/simplyblock/postbrain/internal/ui"
)

func TestPromotionsPage_FiltersByScopeID(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()
	suffix := uuid.NewString()

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "ui-promotions-user-"+suffix)
	scopeA := testhelper.CreateTestScope(t, pool, "project", "ui-promotions-scope-a-"+suffix, nil, principal.ID)
	scopeB := testhelper.CreateTestScope(t, pool, "project", "ui-promotions-scope-b-"+suffix, nil, principal.ID)

	memoryA := testhelper.CreateTestMemory(t, pool, scopeA.ID, principal.ID, "scope A memory")
	memoryB := testhelper.CreateTestMemory(t, pool, scopeB.ID, principal.ID, "scope B memory")

	if _, err := db.CreatePromotionRequest(ctx, pool, &db.PromotionRequest{
		MemoryID:         memoryA.ID,
		RequestedBy:      principal.ID,
		TargetScopeID:    scopeA.ID,
		TargetVisibility: "project",
	}); err != nil {
		t.Fatalf("create promotion for scope A: %v", err)
	}
	if _, err := db.CreatePromotionRequest(ctx, pool, &db.PromotionRequest{
		MemoryID:         memoryB.ID,
		RequestedBy:      principal.ID,
		TargetScopeID:    scopeB.ID,
		TargetVisibility: "project",
	}); err != nil {
		t.Fatalf("create promotion for scope B: %v", err)
	}

	rawToken, hashToken, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	if _, err := db.CreateToken(ctx, pool, principal.ID, hashToken, "ui-promotions-token", nil, nil, nil); err != nil {
		t.Fatalf("create token: %v", err)
	}

	handler, err := uiapi.NewHandler(pool, nil)
	if err != nil {
		t.Fatalf("new ui handler: %v", err)
	}
	mux := http.NewServeMux()
	mux.Handle("/ui", handler)
	mux.Handle("/ui/", handler)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	form := url.Values{}
	form.Set("token", rawToken)
	loginReq, _ := http.NewRequest(http.MethodPost, srv.URL+"/ui/login", strings.NewReader(form.Encode()))
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginResp, err := client.Do(loginReq)
	if err != nil {
		t.Fatalf("login request: %v", err)
	}
	defer loginResp.Body.Close()
	if loginResp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login status = %d, want %d", loginResp.StatusCode, http.StatusSeeOther)
	}

	resp, err := client.Get(srv.URL + "/ui/promotions?scope_id=" + scopeA.ID.String())
	if err != nil {
		t.Fatalf("promotions request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("promotions status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read promotions body: %v", err)
	}
	bodyText := string(body)

	if !strings.Contains(bodyText, memoryA.ID.String()) {
		t.Fatalf("expected scope A memory ID %s in promotions page", memoryA.ID)
	}
	if strings.Contains(bodyText, memoryB.ID.String()) {
		t.Fatalf("did not expect scope B memory ID %s in scope-filtered promotions page", memoryB.ID)
	}
}

func TestPromotionsPage_ShowsApprovedWhenStatusAll(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()
	suffix := uuid.NewString()

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "ui-promotions-status-user-"+suffix)
	scope := testhelper.CreateTestScope(t, pool, "project", "ui-promotions-status-scope-"+suffix, nil, principal.ID)
	memory := testhelper.CreateTestMemory(t, pool, scope.ID, principal.ID, "approved promotion memory")

	prom, err := db.CreatePromotionRequest(ctx, pool, &db.PromotionRequest{
		MemoryID:         memory.ID,
		RequestedBy:      principal.ID,
		TargetScopeID:    scope.ID,
		TargetVisibility: "project",
	})
	if err != nil {
		t.Fatalf("create promotion: %v", err)
	}
	queries := db.New(pool)
	if err := queries.UpdatePromotionRequest(ctx, db.UpdatePromotionRequestParams{
		ID:         prom.ID,
		Status:     "approved",
		ReviewerID: &principal.ID,
	}); err != nil {
		t.Fatalf("approve promotion: %v", err)
	}

	rawToken, hashToken, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	if _, err := db.CreateToken(ctx, pool, principal.ID, hashToken, "ui-promotions-status-token", nil, nil, nil); err != nil {
		t.Fatalf("create token: %v", err)
	}

	handler, err := uiapi.NewHandler(pool, nil)
	if err != nil {
		t.Fatalf("new ui handler: %v", err)
	}
	mux := http.NewServeMux()
	mux.Handle("/ui", handler)
	mux.Handle("/ui/", handler)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	form := url.Values{}
	form.Set("token", rawToken)
	loginReq, _ := http.NewRequest(http.MethodPost, srv.URL+"/ui/login", strings.NewReader(form.Encode()))
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginResp, err := client.Do(loginReq)
	if err != nil {
		t.Fatalf("login request: %v", err)
	}
	defer loginResp.Body.Close()
	if loginResp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login status = %d, want %d", loginResp.StatusCode, http.StatusSeeOther)
	}

	resp, err := client.Get(srv.URL + "/ui/promotions")
	if err != nil {
		t.Fatalf("promotions request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("promotions status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read promotions body: %v", err)
	}
	bodyText := string(body)

	if !strings.Contains(bodyText, memory.ID.String()) {
		t.Fatalf("expected approved promotion memory ID %s in promotions page", memory.ID)
	}
}
