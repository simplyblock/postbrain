//go:build integration

package ui_test

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/testhelper"
	uiapi "github.com/simplyblock/postbrain/internal/ui"
)

func TestLoginPOST_WithNext_RedirectsToNext(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	user := testhelper.CreateTestPrincipal(t, pool, "user", "ui-login-next-user")

	rawToken, hashToken, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	_, err = db.CreateToken(context.Background(), pool, user.ID, hashToken, "ui-login-next", nil, []string{"read"}, nil)
	if err != nil {
		t.Fatalf("create token: %v", err)
	}

	h, err := uiapi.NewHandler(pool, nil)
	if err != nil {
		t.Fatalf("new handler: %v", err)
	}
	mux := http.NewServeMux()
	mux.Handle("/ui", h)
	mux.Handle("/ui/", h)
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
	form.Set("next", "/ui/oauth/authorize?state=resume-state")
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/ui/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("login request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", resp.StatusCode)
	}
	if got := resp.Header.Get("Location"); got != "/ui/oauth/authorize?state=resume-state" {
		t.Fatalf("Location = %q, want %q", got, "/ui/oauth/authorize?state=resume-state")
	}
}

func TestLogoutPOST_ClearsSessionAndRequiresLoginAgain(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	user := testhelper.CreateTestPrincipal(t, pool, "user", "ui-logout-user")

	rawToken, hashToken, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	_, err = db.CreateToken(context.Background(), pool, user.ID, hashToken, "ui-logout", nil, []string{"read"}, nil)
	if err != nil {
		t.Fatalf("create token: %v", err)
	}

	h, err := uiapi.NewHandler(pool, nil)
	if err != nil {
		t.Fatalf("new handler: %v", err)
	}
	mux := http.NewServeMux()
	mux.Handle("/ui", h)
	mux.Handle("/ui/", h)
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

	// Login first.
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

	// Logout.
	logoutReq, _ := http.NewRequest(http.MethodPost, srv.URL+"/ui/logout", nil)
	logoutResp, err := client.Do(logoutReq)
	if err != nil {
		t.Fatalf("logout request: %v", err)
	}
	defer logoutResp.Body.Close()
	if logoutResp.StatusCode != http.StatusSeeOther {
		t.Fatalf("logout status = %d, want %d", logoutResp.StatusCode, http.StatusSeeOther)
	}
	if got := logoutResp.Header.Get("Location"); got != "/ui/login" {
		t.Fatalf("logout Location = %q, want %q", got, "/ui/login")
	}
	if setCookie := logoutResp.Header.Get("Set-Cookie"); !strings.Contains(setCookie, "pb_session=") {
		t.Fatalf("logout should clear session cookie, Set-Cookie=%q", setCookie)
	}

	// Access a protected page after logout; should require login again.
	protectedResp, err := client.Get(srv.URL + "/ui/tokens")
	if err != nil {
		t.Fatalf("protected request: %v", err)
	}
	defer protectedResp.Body.Close()
	if protectedResp.StatusCode != http.StatusSeeOther {
		t.Fatalf("protected status after logout = %d, want %d", protectedResp.StatusCode, http.StatusSeeOther)
	}
	if got := protectedResp.Header.Get("Location"); got != "/ui/login" {
		t.Fatalf("protected Location after logout = %q, want %q", got, "/ui/login")
	}
}
