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
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/config"
	"github.com/simplyblock/postbrain/internal/db/compat"
	"github.com/simplyblock/postbrain/internal/testhelper"
	uiapi "github.com/simplyblock/postbrain/internal/ui"
)

func TestTokensPage_ShowsOnlyCurrentPrincipalTokens(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	userA := testhelper.CreateTestPrincipal(t, pool, "user", "ui-token-user-a")
	userB := testhelper.CreateTestPrincipal(t, pool, "user", "ui-token-user-b")

	rawSessionA, hashSessionA, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("generate session token: %v", err)
	}
	if _, err := compat.CreateToken(ctx, pool, userA.ID, hashSessionA, "a-session", nil, nil, nil); err != nil {
		t.Fatalf("create userA session token: %v", err)
	}
	if _, err := compat.CreateToken(ctx, pool, userA.ID, auth.HashToken("pb_a_visible"), "a-visible", nil, nil, nil); err != nil {
		t.Fatalf("create userA visible token: %v", err)
	}
	if _, err := compat.CreateToken(ctx, pool, userB.ID, auth.HashToken("pb_b_hidden"), "b-hidden", nil, nil, nil); err != nil {
		t.Fatalf("create userB hidden token: %v", err)
	}

	client, baseURL := loginUITestClient(t, pool, rawSessionA)

	resp, err := client.Get(baseURL + "/ui/tokens")
	if err != nil {
		t.Fatalf("tokens request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("tokens status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read tokens body: %v", err)
	}
	bodyText := string(body)
	if !strings.Contains(bodyText, "a-visible") {
		t.Fatalf("expected userA token in tokens page")
	}
	if strings.Contains(bodyText, "b-hidden") {
		t.Fatalf("did not expect userB token in userA tokens page")
	}
}

func TestRevokeToken_OtherPrincipalToken_ReturnsForbidden(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	userA := testhelper.CreateTestPrincipal(t, pool, "user", "ui-revoke-user-a")
	userB := testhelper.CreateTestPrincipal(t, pool, "user", "ui-revoke-user-b")

	rawSessionA, hashSessionA, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("generate session token: %v", err)
	}
	if _, err := compat.CreateToken(ctx, pool, userA.ID, hashSessionA, "a-session", nil, nil, nil); err != nil {
		t.Fatalf("create userA session token: %v", err)
	}
	tokenB, err := compat.CreateToken(ctx, pool, userB.ID, auth.HashToken("pb_b_keep"), "b-keep", nil, nil, nil)
	if err != nil {
		t.Fatalf("create userB token: %v", err)
	}

	client, baseURL := loginUITestClient(t, pool, rawSessionA)

	req, err := http.NewRequest(http.MethodPost, baseURL+"/ui/tokens/"+tokenB.ID.String()+"/revoke", nil)
	if err != nil {
		t.Fatalf("build revoke request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("revoke request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("revoke status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}

	tokens, err := compat.ListTokens(ctx, pool, nil)
	if err != nil {
		t.Fatalf("list tokens: %v", err)
	}
	for _, tok := range tokens {
		if tok.ID == tokenB.ID {
			if tok.RevokedAt != nil {
				t.Fatalf("expected tokenB to remain active after unauthorized revoke")
			}
			return
		}
	}
	t.Fatalf("expected tokenB in token list")
}

func TestUpdateTokenScopes_OwnToken_UpdatesScopeIDs(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	user := testhelper.CreateTestPrincipal(t, pool, "user", "ui-update-scope-user")
	scopeA := testhelper.CreateTestScope(t, pool, "project", "ui-update-scope-a", nil, user.ID)
	scopeB := testhelper.CreateTestScope(t, pool, "project", "ui-update-scope-b", nil, user.ID)

	rawSession, hashSession, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("generate session token: %v", err)
	}
	if _, err := compat.CreateToken(ctx, pool, user.ID, hashSession, "session", nil, nil, nil); err != nil {
		t.Fatalf("create session token: %v", err)
	}

	tokenToEdit, err := compat.CreateToken(ctx, pool, user.ID, auth.HashToken("pb_edit_scope"), "editable", []uuid.UUID{scopeA.ID}, nil, nil)
	if err != nil {
		t.Fatalf("create editable token: %v", err)
	}

	client, baseURL := loginUITestClient(t, pool, rawSession)

	form := url.Values{}
	form.Add("scope_ids", scopeB.ID.String())
	req, err := http.NewRequest(http.MethodPost, baseURL+"/ui/tokens/"+tokenToEdit.ID.String()+"/scopes", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("build update scopes request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("update scopes request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusSeeOther)
	}

	tokens, err := compat.ListTokens(ctx, pool, &user.ID)
	if err != nil {
		t.Fatalf("list tokens: %v", err)
	}
	for _, tok := range tokens {
		if tok.ID != tokenToEdit.ID {
			continue
		}
		if len(tok.ScopeIds) != 1 || tok.ScopeIds[0] != scopeB.ID {
			t.Fatalf("scope_ids = %v, want [%s]", tok.ScopeIds, scopeB.ID)
		}
		return
	}
	t.Fatalf("editable token not found")
}

func TestUpdateTokenScopes_OtherPrincipalToken_ReturnsForbidden(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	userA := testhelper.CreateTestPrincipal(t, pool, "user", "ui-update-scope-a-user")
	userB := testhelper.CreateTestPrincipal(t, pool, "user", "ui-update-scope-b-user")

	rawSessionA, hashSessionA, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("generate session token: %v", err)
	}
	if _, err := compat.CreateToken(ctx, pool, userA.ID, hashSessionA, "a-session", nil, nil, nil); err != nil {
		t.Fatalf("create userA session token: %v", err)
	}

	scopeB := testhelper.CreateTestScope(t, pool, "project", "ui-update-scope-forbidden", nil, userB.ID)
	tokenB, err := compat.CreateToken(ctx, pool, userB.ID, auth.HashToken("pb_update_scope_other"), "b-editable", []uuid.UUID{scopeB.ID}, nil, nil)
	if err != nil {
		t.Fatalf("create userB token: %v", err)
	}

	client, baseURL := loginUITestClient(t, pool, rawSessionA)

	form := url.Values{}
	req, err := http.NewRequest(http.MethodPost, baseURL+"/ui/tokens/"+tokenB.ID.String()+"/scopes", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("build update scopes request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("update scopes request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
}

func TestTokensPage_UsesDialogForScopeEditing(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	user := testhelper.CreateTestPrincipal(t, pool, "user", "ui-token-dialog-user")
	scope := testhelper.CreateTestScope(t, pool, "project", "ui-token-dialog-scope", nil, user.ID)
	rawSession, hashSession, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("generate session token: %v", err)
	}
	if _, err := compat.CreateToken(ctx, pool, user.ID, hashSession, "session", nil, nil, nil); err != nil {
		t.Fatalf("create session token: %v", err)
	}
	if _, err := compat.CreateToken(ctx, pool, user.ID, auth.HashToken("pb_dialog_scope"), "editable", []uuid.UUID{scope.ID}, nil, nil); err != nil {
		t.Fatalf("create editable token: %v", err)
	}

	client, baseURL := loginUITestClient(t, pool, rawSession)
	resp, err := client.Get(baseURL + "/ui/tokens")
	if err != nil {
		t.Fatalf("GET /ui/tokens: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	bodyText := string(body)
	if !strings.Contains(bodyText, "id=\"dlg-token-scopes\"") {
		t.Fatalf("expected token scope dialog markup")
	}
	if !strings.Contains(bodyText, "openTokenScopesDialog(") {
		t.Fatalf("expected openTokenScopesDialog JS hook")
	}
	// Ensure scope editing uses the dialog button, not a <details> element inline.
	if strings.Contains(bodyText, "<details") && !strings.Contains(bodyText, "id=\"dlg-token-scopes\"") {
		t.Fatalf("did not expect inline <details> UI for scope editing without dialog")
	}
}

func TestCreateToken_UsesSelectedPermissions(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	user := testhelper.CreateTestPrincipal(t, pool, "user", "ui-token-create-perms-user")
	rawSession, hashSession, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("generate session token: %v", err)
	}
	if _, err := compat.CreateToken(ctx, pool, user.ID, hashSession, "session", nil, nil, nil); err != nil {
		t.Fatalf("create session token: %v", err)
	}

	client, baseURL := loginUITestClient(t, pool, rawSession)

	form := url.Values{}
	form.Set("name", "read-only-created")
	form.Add("permissions", "memories:read")
	req, err := http.NewRequest(http.MethodPost, baseURL+"/ui/tokens", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST /ui/tokens: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	tokens, err := compat.ListTokens(ctx, pool, &user.ID)
	if err != nil {
		t.Fatalf("list tokens: %v", err)
	}
	for _, tok := range tokens {
		if tok.Name != "read-only-created" {
			continue
		}
		if len(tok.Permissions) != 1 || tok.Permissions[0] != "memories:read" {
			t.Fatalf("permissions = %v, want [memories:read]", tok.Permissions)
		}
		return
	}
	t.Fatalf("created token not found")
}

func TestTokensPage_ShowsTokenPermissions(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	user := testhelper.CreateTestPrincipal(t, pool, "user", "ui-token-perms-visible-user")
	rawSession, hashSession, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("generate session token: %v", err)
	}
	if _, err := compat.CreateToken(ctx, pool, user.ID, hashSession, "session", nil, []string{"tokens:read"}, nil); err != nil {
		t.Fatalf("create session token: %v", err)
	}
	if _, err := compat.CreateToken(ctx, pool, user.ID, auth.HashToken("pb_token_perms_visible"), "permissions-visible", nil, []string{"memories:read", "memories:write"}, nil); err != nil {
		t.Fatalf("create visible token: %v", err)
	}

	client, baseURL := loginUITestClient(t, pool, rawSession)
	resp, err := client.Get(baseURL + "/ui/tokens")
	if err != nil {
		t.Fatalf("GET /ui/tokens: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	bodyText := string(body)
	if !strings.Contains(bodyText, "permissions-visible") {
		t.Fatalf("expected created token in page output")
	}
	if !strings.Contains(bodyText, "memories:read") {
		t.Fatalf("expected permissions column to include memories:read value")
	}
}

func TestTokensPage_EditScopes_ShowsPrincipalEffectiveScopes(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	user := testhelper.CreateTestPrincipal(t, pool, "user", "ui-token-edit-scope-effective-user")
	scopeA := testhelper.CreateTestScope(t, pool, "department", "ui-token-edit-scope-effective-a", nil, user.ID)
	scopeB := testhelper.CreateTestScope(t, pool, "department", "ui-token-edit-scope-effective-b", nil, user.ID)

	rawSession, hashSession, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("generate session token: %v", err)
	}
	if _, err := compat.CreateToken(ctx, pool, user.ID, hashSession, "session-scoped-a", []uuid.UUID{scopeA.ID}, []string{"tokens:read", "scopes:read"}, nil); err != nil {
		t.Fatalf("create scoped session token: %v", err)
	}
	if _, err := compat.CreateToken(ctx, pool, user.ID, auth.HashToken("pb_edit_scope_multi"), "editable-multi", []uuid.UUID{scopeA.ID, scopeB.ID}, []string{"memories:read", "memories:write"}, nil); err != nil {
		t.Fatalf("create editable token: %v", err)
	}

	client, baseURL := loginUITestClient(t, pool, rawSession)
	resp, err := client.Get(baseURL + "/ui/tokens")
	if err != nil {
		t.Fatalf("GET /ui/tokens: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	bodyText := string(body)
	if !strings.Contains(bodyText, "department:"+scopeA.ExternalID) {
		t.Fatalf("expected scope A in edit-scope options")
	}
	if !strings.Contains(bodyText, "department:"+scopeB.ExternalID) {
		t.Fatalf("expected scope B in edit-scope options")
	}
}

func loginUITestClient(t *testing.T, pool *pgxpool.Pool, rawSessionToken string) (*http.Client, string) {
	t.Helper()

	handler, err := uiapi.NewHandler(pool, nil, &config.Config{})
	if err != nil {
		t.Fatalf("new ui handler: %v", err)
	}
	mux := http.NewServeMux()
	mux.Handle("/ui", handler)
	mux.Handle("/ui/", handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

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
	form.Set("token", rawSessionToken)
	loginReq, err := http.NewRequest(http.MethodPost, srv.URL+"/ui/login", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("build login request: %v", err)
	}
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginResp, err := client.Do(loginReq)
	if err != nil {
		t.Fatalf("login request: %v", err)
	}
	defer loginResp.Body.Close()
	if loginResp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login status = %d, want %d", loginResp.StatusCode, http.StatusSeeOther)
	}

	return client, srv.URL
}
