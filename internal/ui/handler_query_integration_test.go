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
	"github.com/simplyblock/postbrain/internal/memory"
	"github.com/simplyblock/postbrain/internal/testhelper"
	uiapi "github.com/simplyblock/postbrain/internal/ui"
)

func TestQueryPlayground_SelectedScopeIncludesAncestorMemories(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()
	suffix := uuid.NewString()

	_ = testhelper.CreateTestEmbeddingModel(t, pool)
	svc := testhelper.NewMockEmbeddingService()

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "ui-query-user-"+suffix)
	company := testhelper.CreateTestScope(t, pool, "company", "ui-query-company-"+suffix, nil, principal.ID)
	project := testhelper.CreateTestScope(t, pool, "project", "ui-query-project-"+suffix, &company.ID, principal.ID)

	memStore := memory.NewStore(pool, svc)
	if _, err := memStore.Create(ctx, memory.CreateInput{
		MemoryType: "semantic",
		ScopeID:    company.ID,
		AuthorID:   principal.ID,
		Content:    "DEPLOY_TOKEN ancestor memory",
	}); err != nil {
		t.Fatalf("create ancestor memory: %v", err)
	}
	if _, err := memStore.Create(ctx, memory.CreateInput{
		MemoryType: "semantic",
		ScopeID:    project.ID,
		AuthorID:   principal.ID,
		Content:    "DEPLOY_TOKEN project memory",
	}); err != nil {
		t.Fatalf("create project memory: %v", err)
	}

	rawToken, hashToken, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	if _, err := auth.NewTokenStore(pool).Create(ctx, principal.ID, hashToken, "ui-query-token", nil, nil, nil); err != nil {
		t.Fatalf("create token: %v", err)
	}

	handler, err := uiapi.NewHandler(pool, svc)
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

	queryURL := srv.URL + "/ui/query?q=DEPLOY_TOKEN&scope_id=" + project.ID.String() + "&search_mode=hybrid&limit=10&layer_memory=1"
	resp, err := client.Get(queryURL)
	if err != nil {
		t.Fatalf("query request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("query status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read query body: %v", err)
	}
	bodyText := string(body)

	if !strings.Contains(bodyText, "DEPLOY_TOKEN project memory") {
		t.Fatalf("expected project memory in query response")
	}
	if !strings.Contains(bodyText, "DEPLOY_TOKEN ancestor memory") {
		t.Fatalf("expected ancestor memory in selected-scope query response")
	}
}
