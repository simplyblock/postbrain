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

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/db"
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
	if _, err := db.CreateToken(ctx, pool, userA.ID, hashSessionA, "a-session", nil, nil, nil); err != nil {
		t.Fatalf("create userA session token: %v", err)
	}
	if _, err := db.CreateToken(ctx, pool, userA.ID, auth.HashToken("pb_a_visible"), "a-visible", nil, nil, nil); err != nil {
		t.Fatalf("create userA visible token: %v", err)
	}
	if _, err := db.CreateToken(ctx, pool, userB.ID, auth.HashToken("pb_b_hidden"), "b-hidden", nil, nil, nil); err != nil {
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
	if _, err := db.CreateToken(ctx, pool, userA.ID, hashSessionA, "a-session", nil, nil, nil); err != nil {
		t.Fatalf("create userA session token: %v", err)
	}
	tokenB, err := db.CreateToken(ctx, pool, userB.ID, auth.HashToken("pb_b_keep"), "b-keep", nil, nil, nil)
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

	tokens, err := db.ListTokens(ctx, pool, nil)
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

func loginUITestClient(t *testing.T, pool *pgxpool.Pool, rawSessionToken string) (*http.Client, string) {
	t.Helper()

	handler, err := uiapi.NewHandler(pool, nil)
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
