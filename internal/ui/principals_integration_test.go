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

func TestPrincipalsPage_ShowsOnlyReachablePrincipals(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	userA := testhelper.CreateTestPrincipal(t, pool, "user", "ui-principals-user-a")
	teamA := testhelper.CreateTestPrincipal(t, pool, "team", "ui-principals-team-a")
	testhelper.CreateTestPrincipal(t, pool, "user", "ui-principals-user-b")

	if _, err := db.CreateMembership(ctx, pool, userA.ID, teamA.ID, "member", nil); err != nil {
		t.Fatalf("create membership userA->teamA: %v", err)
	}

	rawSessionA, hashSessionA, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("generate session token: %v", err)
	}
	if _, err := db.CreateToken(ctx, pool, userA.ID, hashSessionA, "a-session", nil, nil, nil); err != nil {
		t.Fatalf("create userA session token: %v", err)
	}

	client, baseURL := loginUITestClient(t, pool, rawSessionA)

	resp, err := client.Get(baseURL + "/ui/principals")
	if err != nil {
		t.Fatalf("principals request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("principals status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read principals body: %v", err)
	}
	bodyText := string(body)
	if !strings.Contains(bodyText, "ui-principals-user-a") {
		t.Fatalf("expected reachable principal userA in principals page")
	}
	if !strings.Contains(bodyText, "ui-principals-team-a") {
		t.Fatalf("expected reachable principal teamA in principals page")
	}
	if strings.Contains(bodyText, "ui-principals-user-b") {
		t.Fatalf("did not expect unrelated principal userB in principals page")
	}
}
