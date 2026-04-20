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
	"github.com/simplyblock/postbrain/internal/config"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/db/compat"
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

	handler, err := uiapi.NewHandler(pool, svc, &config.Config{})
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

	queryURL := srv.URL + "/ui/" + project.ID.String() + "/query?q=DEPLOY_TOKEN&search_mode=hybrid&limit=10&layer_memory=1"
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

func TestQueryPlayground_SelectedScopeExcludesSiblingMemories(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()
	suffix := uuid.NewString()

	_ = testhelper.CreateTestEmbeddingModel(t, pool)
	svc := testhelper.NewMockEmbeddingService()

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "ui-query-sibling-user-"+suffix)
	company := testhelper.CreateTestScope(t, pool, "company", "ui-query-sibling-company-"+suffix, nil, principal.ID)
	marketingProject := testhelper.CreateTestScope(t, pool, "project", "ui-query-marketing-project-"+suffix, &company.ID, principal.ID)
	engineeringProject := testhelper.CreateTestScope(t, pool, "project", "ui-query-engineering-project-"+suffix, &company.ID, principal.ID)

	memStore := memory.NewStore(pool, svc)
	if _, err := memStore.Create(ctx, memory.CreateInput{
		MemoryType: "semantic",
		ScopeID:    marketingProject.ID,
		AuthorID:   principal.ID,
		Content:    "SHARED_QUERY_MARKETING marker",
	}); err != nil {
		t.Fatalf("create marketing memory: %v", err)
	}
	if _, err := memStore.Create(ctx, memory.CreateInput{
		MemoryType: "semantic",
		ScopeID:    engineeringProject.ID,
		AuthorID:   principal.ID,
		Content:    "SHARED_QUERY_ENGINEERING marker",
	}); err != nil {
		t.Fatalf("create engineering memory: %v", err)
	}

	rawToken, hashToken, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	if _, err := auth.NewTokenStore(pool).Create(ctx, principal.ID, hashToken, "ui-query-sibling-token", nil, nil, nil); err != nil {
		t.Fatalf("create token: %v", err)
	}

	handler, err := uiapi.NewHandler(pool, svc, &config.Config{})
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

	queryURL := srv.URL + "/ui/" + marketingProject.ID.String() + "/query?q=SHARED_QUERY&search_mode=hybrid&limit=10&layer_memory=1"
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

	if !strings.Contains(bodyText, "SHARED_QUERY_MARKETING marker") {
		t.Fatalf("expected selected project memory in query response")
	}
	if strings.Contains(bodyText, "SHARED_QUERY_ENGINEERING marker") {
		t.Fatalf("did not expect sibling project memory in selected-scope query response")
	}
}

func TestQueryPlayground_MemoryOnly_DoesNotLeakSiblingGraphContext(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()
	suffix := uuid.NewString()

	_ = testhelper.CreateTestEmbeddingModel(t, pool)
	svc := testhelper.NewMockEmbeddingService()

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "ui-query-graph-leak-user-"+suffix)
	company := testhelper.CreateTestScope(t, pool, "company", "ui-query-graph-leak-company-"+suffix, nil, principal.ID)
	selectedProject := testhelper.CreateTestScope(t, pool, "project", "ui-query-graph-leak-selected-"+suffix, &company.ID, principal.ID)
	siblingProject := testhelper.CreateTestScope(t, pool, "project", "ui-query-graph-leak-sibling-"+suffix, &company.ID, principal.ID)

	memStore := memory.NewStore(pool, svc)
	sourceRef := "file:src/auth/service.go:42"
	if _, err := memStore.Create(ctx, memory.CreateInput{
		MemoryType: "semantic",
		ScopeID:    selectedProject.ID,
		AuthorID:   principal.ID,
		Content:    "auth selected scope anchor",
		SourceRef:  &sourceRef,
	}); err != nil {
		t.Fatalf("create selected project memory: %v", err)
	}

	sourceEntity, err := compat.UpsertEntity(ctx, pool, &db.Entity{
		ScopeID:    selectedProject.ID,
		EntityType: "file",
		Name:       "service.go",
		Canonical:  "src/auth/service.go",
	})
	if err != nil {
		t.Fatalf("upsert source entity: %v", err)
	}
	neighborEntity, err := compat.UpsertEntity(ctx, pool, &db.Entity{
		ScopeID:    selectedProject.ID,
		EntityType: "function",
		Name:       "AuthFlow",
		Canonical:  "auth.AuthFlow",
	})
	if err != nil {
		t.Fatalf("upsert neighbor entity: %v", err)
	}
	if _, err := compat.UpsertRelation(ctx, pool, &db.Relation{
		ScopeID:   selectedProject.ID,
		SubjectID: sourceEntity.ID,
		Predicate: "uses",
		ObjectID:  neighborEntity.ID,
	}); err != nil {
		t.Fatalf("upsert relation: %v", err)
	}

	siblingMemory, err := memStore.Create(ctx, memory.CreateInput{
		MemoryType: "semantic",
		ScopeID:    siblingProject.ID,
		AuthorID:   principal.ID,
		Content:    "SIBLING_GRAPH_LEAK_AUTH_MARKER",
	})
	if err != nil {
		t.Fatalf("create sibling memory: %v", err)
	}
	if err := compat.LinkMemoryToEntity(ctx, pool, siblingMemory.MemoryID, neighborEntity.ID, "related"); err != nil {
		t.Fatalf("link sibling memory to selected-scope neighbor: %v", err)
	}

	rawToken, hashToken, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	if _, err := auth.NewTokenStore(pool).Create(ctx, principal.ID, hashToken, "ui-query-graph-leak-token", nil, nil, nil); err != nil {
		t.Fatalf("create token: %v", err)
	}

	handler, err := uiapi.NewHandler(pool, svc, &config.Config{})
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

	queryURL := srv.URL + "/ui/" + selectedProject.ID.String() + "/query?q=auth&search_mode=hybrid&limit=10&layer_memory=1"
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

	if !strings.Contains(bodyText, "auth") {
		t.Fatalf("expected selected-scope query response to include at least one auth-related result")
	}

	if strings.Contains(bodyText, "SIBLING_GRAPH_LEAK_AUTH_MARKER") {
		t.Fatalf("did not expect sibling graph-context memory in query response")
	}
}
