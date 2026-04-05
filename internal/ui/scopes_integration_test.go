//go:build integration

package ui_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

func TestScopesPage_ShowsOnlyWritableScopesForPrincipal(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	userA := testhelper.CreateTestPrincipal(t, pool, "user", "ui-scope-user-a")
	userB := testhelper.CreateTestPrincipal(t, pool, "user", "ui-scope-user-b")

	scopeA := testhelper.CreateTestScope(t, pool, "project", "ui-scope-a", nil, userA.ID)
	scopeB := testhelper.CreateTestScope(t, pool, "project", "ui-scope-b", nil, userB.ID)

	rawSessionA, hashSessionA, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("generate session token: %v", err)
	}
	if _, err := db.CreateToken(ctx, pool, userA.ID, hashSessionA, "a-session", nil, nil, nil); err != nil {
		t.Fatalf("create userA session token: %v", err)
	}

	client, baseURL := loginUITestClient(t, pool, rawSessionA)

	resp, err := client.Get(baseURL + "/ui/scopes")
	if err != nil {
		t.Fatalf("scopes request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("scopes status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read scopes body: %v", err)
	}
	bodyText := string(body)
	if !strings.Contains(bodyText, scopeA.ExternalID) {
		t.Fatalf("expected writable scope %q in scopes page", scopeA.ExternalID)
	}
	if strings.Contains(bodyText, scopeB.ExternalID) {
		t.Fatalf("did not expect non-writable scope %q in scopes page", scopeB.ExternalID)
	}
}
