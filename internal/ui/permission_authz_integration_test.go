//go:build integration

package ui_test

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/db/compat"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

func TestUI_PermissionAuthz_ReadVsWrite(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()
	user := testhelper.CreateTestPrincipal(t, pool, "user", "ui-perm-user")

	rawRead, hashRead, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("generate read token: %v", err)
	}
	if _, err := compat.CreateToken(ctx, pool, user.ID, hashRead, "ui-read-session", nil, []string{"tokens:read"}, nil); err != nil {
		t.Fatalf("create read token: %v", err)
	}

	rawWrite, hashWrite, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("generate write token: %v", err)
	}
	if _, err := compat.CreateToken(ctx, pool, user.ID, hashWrite, "ui-write-session", nil, []string{"tokens:write", "memories:read", "memories:write"}, nil); err != nil {
		t.Fatalf("create write token: %v", err)
	}

	t.Run("read token can access pages", func(t *testing.T) {
		client, baseURL := loginUITestClient(t, pool, rawRead)
		resp, err := client.Get(baseURL + "/ui/tokens")
		if err != nil {
			t.Fatalf("GET /ui/tokens: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
	})

	t.Run("read token cannot perform writes", func(t *testing.T) {
		client, baseURL := loginUITestClient(t, pool, rawRead)
		form := url.Values{}
		form.Set("name", "should-not-create")
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
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusForbidden)
		}
	})

	t.Run("write token can perform writes", func(t *testing.T) {
		client, baseURL := loginUITestClient(t, pool, rawWrite)
		form := url.Values{}
		form.Set("name", "write-created")
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
	})
}
