//go:build integration

package ui_test

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"slices"
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

func TestPrincipalsPage_SystemAdminCanAddMembership(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	systemAdmin := testhelper.CreateTestPrincipal(t, pool, "user", "ui-systemadmin-membership-actor-"+uuid.NewString())
	team := testhelper.CreateTestPrincipal(t, pool, "team", "ui-systemadmin-membership-team-"+uuid.NewString())
	member := testhelper.CreateTestPrincipal(t, pool, "user", "ui-systemadmin-membership-member-"+uuid.NewString())

	if _, err := pool.Exec(ctx, `UPDATE principals SET is_system_admin = true WHERE id = $1`, systemAdmin.ID); err != nil {
		t.Fatalf("set is_system_admin: %v", err)
	}

	rawSession, hashSession, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("generate session token: %v", err)
	}
	if _, err := db.CreateToken(ctx, pool, systemAdmin.ID, hashSession, "ui-systemadmin-membership-session", nil, nil, nil); err != nil {
		t.Fatalf("create session token: %v", err)
	}

	client, baseURL := loginUITestClient(t, pool, rawSession)
	form := url.Values{}
	form.Set("member_id", member.ID.String())
	form.Set("parent_id", team.ID.String())
	form.Set("role", "member")

	req, err := http.NewRequest(http.MethodPost, baseURL+"/ui/memberships", strings.NewReader(form.Encode()))
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

	memberships, err := db.GetMemberships(ctx, pool, member.ID)
	if err != nil {
		t.Fatalf("get memberships: %v", err)
	}
	var found bool
	for _, m := range memberships {
		if m.ParentID == team.ID && m.Role == "member" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected membership to be created by system admin")
	}
}

func TestPrincipalsPage_SystemAdminCanManageScopeGrants(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	systemAdmin := testhelper.CreateTestPrincipal(t, pool, "user", "ui-systemadmin-scopegrant-actor-"+uuid.NewString())
	owner := testhelper.CreateTestPrincipal(t, pool, "team", "ui-systemadmin-scopegrant-owner-"+uuid.NewString())
	grantee := testhelper.CreateTestPrincipal(t, pool, "user", "ui-systemadmin-scopegrant-grantee-"+uuid.NewString())

	if _, err := pool.Exec(ctx, `UPDATE principals SET is_system_admin = true WHERE id = $1`, systemAdmin.ID); err != nil {
		t.Fatalf("set is_system_admin: %v", err)
	}

	scope, err := db.CreateScope(
		ctx,
		pool,
		"project",
		"ui-scopegrant-scope-"+uuid.NewString(),
		"UI Scope Grant",
		nil,
		owner.ID,
		nil,
	)
	if err != nil {
		t.Fatalf("create scope: %v", err)
	}

	rawSession, hashSession, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("generate session token: %v", err)
	}
	if _, err := db.CreateToken(ctx, pool, systemAdmin.ID, hashSession, "ui-systemadmin-scopegrant-session", nil, nil, nil); err != nil {
		t.Fatalf("create session token: %v", err)
	}

	client, baseURL := loginUITestClient(t, pool, rawSession)

	createForm := url.Values{}
	createForm.Set("scope_id", scope.ID.String())
	createForm.Set("principal_id", grantee.ID.String())
	createForm.Add("permissions", "memories:read")
	createForm.Add("permissions", "knowledge:read")

	createReq, err := http.NewRequest(http.MethodPost, baseURL+"/ui/scope-grants", strings.NewReader(createForm.Encode()))
	if err != nil {
		t.Fatalf("build create request: %v", err)
	}
	createReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	createResp, err := client.Do(createReq)
	if err != nil {
		t.Fatalf("create request failed: %v", err)
	}
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create status = %d, want %d", createResp.StatusCode, http.StatusSeeOther)
	}

	q := db.New(pool)
	grant, err := q.GetScopeGrant(ctx, db.GetScopeGrantParams{PrincipalID: grantee.ID, ScopeID: scope.ID})
	if err != nil {
		t.Fatalf("get scope grant: %v", err)
	}
	if grant == nil {
		t.Fatal("expected scope grant to exist")
	}
	if !slices.Contains(grant.Permissions, "memories:read") || !slices.Contains(grant.Permissions, "knowledge:read") {
		t.Fatalf("unexpected grant permissions: %#v", grant.Permissions)
	}

	deleteForm := url.Values{}
	deleteForm.Set("scope_id", scope.ID.String())
	deleteForm.Set("grant_id", grant.ID.String())

	deleteReq, err := http.NewRequest(http.MethodPost, baseURL+"/ui/scope-grants/delete", strings.NewReader(deleteForm.Encode()))
	if err != nil {
		t.Fatalf("build delete request: %v", err)
	}
	deleteReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	deleteResp, err := client.Do(deleteReq)
	if err != nil {
		t.Fatalf("delete request failed: %v", err)
	}
	defer deleteResp.Body.Close()
	if deleteResp.StatusCode != http.StatusSeeOther {
		t.Fatalf("delete status = %d, want %d", deleteResp.StatusCode, http.StatusSeeOther)
	}

	grants, err := q.ListScopeGrantsByScope(ctx, scope.ID)
	if err != nil {
		t.Fatalf("list scope grants: %v", err)
	}
	if len(grants) != 0 {
		t.Fatalf("expected no grants after delete, got %d", len(grants))
	}
}
