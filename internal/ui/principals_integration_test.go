//go:build integration

package ui_test

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/principals"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

func TestPrincipalsPage_ShowsOnlyReachablePrincipals(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	userA := testhelper.CreateTestPrincipal(t, pool, "user", "ui-principals-user-a")
	teamA := testhelper.CreateTestPrincipal(t, pool, "team", "ui-principals-team-a")
	testhelper.CreateTestPrincipal(t, pool, "user", "ui-principals-user-b")
	adminHub := testhelper.CreateTestPrincipal(t, pool, "team", "ui-principals-admin-hub")

	if _, err := db.CreateMembership(ctx, pool, userA.ID, teamA.ID, "member", nil); err != nil {
		t.Fatalf("create membership userA->teamA: %v", err)
	}
	if _, err := db.CreateMembership(ctx, pool, userA.ID, adminHub.ID, "admin", nil); err != nil {
		t.Fatalf("create admin membership userA->adminHub: %v", err)
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

func TestPrincipalsPage_RequiresAdminRole(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	user := testhelper.CreateTestPrincipal(t, pool, "user", "ui-principals-requires-admin-user")
	rawSession, hashSession, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("generate session token: %v", err)
	}
	if _, err := db.CreateToken(ctx, pool, user.ID, hashSession, "ui-principals-requires-admin-session", nil, nil, nil); err != nil {
		t.Fatalf("create session token: %v", err)
	}

	client, baseURL := loginUITestClient(t, pool, rawSession)
	resp, err := client.Get(baseURL + "/ui/principals")
	if err != nil {
		t.Fatalf("GET /ui/principals: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
}

func TestSidebar_HidesPrincipalsForNonAdmin(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	user := testhelper.CreateTestPrincipal(t, pool, "user", "ui-sidebar-principals-user")
	rawSession, hashSession, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("generate session token: %v", err)
	}
	if _, err := db.CreateToken(ctx, pool, user.ID, hashSession, "ui-sidebar-principals-session", nil, nil, nil); err != nil {
		t.Fatalf("create session token: %v", err)
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
	if strings.Contains(string(body), "href=\"/ui/principals\"") {
		t.Fatalf("did not expect principals nav entry for non-admin")
	}
}

func TestSidebar_ShowsPrincipalsForAdmin(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	user := testhelper.CreateTestPrincipal(t, pool, "user", "ui-sidebar-principals-admin-user")
	adminHub := testhelper.CreateTestPrincipal(t, pool, "team", "ui-sidebar-principals-admin-hub")
	if _, err := db.CreateMembership(ctx, pool, user.ID, adminHub.ID, "admin", nil); err != nil {
		t.Fatalf("create admin membership user->adminHub: %v", err)
	}

	rawSession, hashSession, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("generate session token: %v", err)
	}
	if _, err := db.CreateToken(ctx, pool, user.ID, hashSession, "ui-sidebar-principals-admin-session", nil, nil, nil); err != nil {
		t.Fatalf("create session token: %v", err)
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
	if !strings.Contains(string(body), "href=\"/ui/principals\"") {
		t.Fatalf("expected principals nav entry for admin")
	}
}

func TestPrincipalsPage_PrincipalSlugChangeRequiresAdmin(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	actor := testhelper.CreateTestPrincipal(t, pool, "user", "ui-principal-admin-actor-"+uuid.NewString())
	target := testhelper.CreateTestPrincipal(t, pool, "user", "ui-principal-admin-target-"+uuid.NewString())
	adminHub := testhelper.CreateTestPrincipal(t, pool, "team", "ui-principal-admin-hub-"+uuid.NewString())

	rawSessionActor, hashSessionActor, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("generate session token: %v", err)
	}
	if _, err := db.CreateToken(ctx, pool, actor.ID, hashSessionActor, "ui-principal-admin-session", nil, nil, nil); err != nil {
		t.Fatalf("create actor session token: %v", err)
	}

	client, baseURL := loginUITestClient(t, pool, rawSessionActor)

	form := url.Values{}
	form.Set("slug", "ui-principal-slug-updated-"+uuid.NewString())
	form.Set("display_name", "Updated By Non-Admin")

	t.Run("non-admin slug update is denied", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodPost, baseURL+"/ui/principals/"+target.ID.String(), strings.NewReader(form.Encode()))
		if err != nil {
			t.Fatalf("build request: %v", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusForbidden)
		}

		after, err := db.GetPrincipalByID(ctx, pool, target.ID)
		if err != nil {
			t.Fatalf("get principal after denied update: %v", err)
		}
		if after == nil {
			t.Fatal("expected target principal to exist")
		}
		if after.Slug == form.Get("slug") {
			t.Fatalf("slug changed without admin privileges")
		}
	})

	t.Run("admin slug update is allowed", func(t *testing.T) {
		ms := principals.NewMembershipStore(pool)
		if err := ms.AddMembership(ctx, actor.ID, adminHub.ID, "admin", nil); err != nil {
			t.Fatalf("add actor admin membership: %v", err)
		}
		if err := ms.AddMembership(ctx, target.ID, adminHub.ID, "member", nil); err != nil {
			t.Fatalf("add target membership: %v", err)
		}

		req, err := http.NewRequest(http.MethodPost, baseURL+"/ui/principals/"+target.ID.String(), strings.NewReader(form.Encode()))
		if err != nil {
			t.Fatalf("build request: %v", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusSeeOther {
			t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusSeeOther)
		}

		after, err := db.GetPrincipalByID(ctx, pool, target.ID)
		if err != nil {
			t.Fatalf("get principal after update: %v", err)
		}
		if after == nil {
			t.Fatal("expected target principal to exist")
		}
		if after.Slug != form.Get("slug") {
			t.Fatalf("slug = %q, want %q", after.Slug, form.Get("slug"))
		}
	})
}
