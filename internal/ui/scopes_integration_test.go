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
	"github.com/simplyblock/postbrain/internal/db/compat"
	"github.com/simplyblock/postbrain/internal/principals"
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
	if _, err := compat.CreateToken(ctx, pool, userA.ID, hashSessionA, "a-session", nil, nil, nil); err != nil {
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

func TestScopesPage_MemberCannotAdminParentScope(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	parentPrincipal := testhelper.CreateTestPrincipal(t, pool, "team", "ui-scope-parent-"+uuid.NewString())
	childPrincipal := testhelper.CreateTestPrincipal(t, pool, "user", "ui-scope-child-"+uuid.NewString())
	parentScope := testhelper.CreateTestScope(t, pool, "project", "ui-parent-scope-"+uuid.NewString(), nil, parentPrincipal.ID)

	ms := principals.NewMembershipStore(pool)
	if err := ms.AddMembership(ctx, childPrincipal.ID, parentPrincipal.ID, "member", nil); err != nil {
		t.Fatalf("add membership child->parent: %v", err)
	}

	rawSessionChild, hashSessionChild, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("generate session token: %v", err)
	}
	if _, err := compat.CreateToken(ctx, pool, childPrincipal.ID, hashSessionChild, "child-session", nil, nil, nil); err != nil {
		t.Fatalf("create child session token: %v", err)
	}

	client, baseURL := loginUITestClient(t, pool, rawSessionChild)

	t.Run("member cannot delete parent scope", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodPost, baseURL+"/ui/scopes/"+parentScope.ID.String()+"/delete", nil)
		if err != nil {
			t.Fatalf("build delete request: %v", err)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("delete request: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if !strings.Contains(string(body), "scope admin required") {
			t.Fatalf("expected scope admin required error, got body=%q", string(body))
		}

		scopeAfter, err := compat.GetScopeByID(ctx, pool, parentScope.ID)
		if err != nil {
			t.Fatalf("get parent scope after delete attempt: %v", err)
		}
		if scopeAfter == nil {
			t.Fatalf("parent scope should not be deleted by non-admin member")
		}
	})

	t.Run("member does not get delete action for parent scope", func(t *testing.T) {
		resp, err := client.Get(baseURL + "/ui/scopes")
		if err != nil {
			t.Fatalf("GET /ui/scopes: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		deleteAction := "/ui/scopes/" + parentScope.ID.String() + "/delete"
		if strings.Contains(string(body), deleteAction) {
			t.Fatalf("did not expect delete action %q for non-admin member", deleteAction)
		}
		ownerAction := "openOwnerDialog('" + parentScope.ID.String() + "'"
		if strings.Contains(string(body), ownerAction) {
			t.Fatalf("did not expect owner action %q for non-admin member", ownerAction)
		}
	})

	t.Run("member cannot change owner of parent scope", func(t *testing.T) {
		form := url.Values{}
		form.Set("principal_id", childPrincipal.ID.String())
		req, err := http.NewRequest(http.MethodPost, baseURL+"/ui/scopes/"+parentScope.ID.String()+"/owner", strings.NewReader(form.Encode()))
		if err != nil {
			t.Fatalf("build owner request: %v", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("owner request: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if !strings.Contains(string(body), "scope admin required") {
			t.Fatalf("expected scope admin required error, got body=%q", string(body))
		}

		scopeAfter, err := compat.GetScopeByID(ctx, pool, parentScope.ID)
		if err != nil {
			t.Fatalf("get parent scope after owner change attempt: %v", err)
		}
		if scopeAfter == nil {
			t.Fatal("expected parent scope to exist")
		}
		if scopeAfter.PrincipalID != parentPrincipal.ID {
			t.Fatalf("owner changed unexpectedly: got %s want %s", scopeAfter.PrincipalID, parentPrincipal.ID)
		}
	})

	t.Run("member cannot create child scope under parent scope", func(t *testing.T) {
		form := url.Values{}
		form.Set("kind", "project")
		form.Set("external_id", "ui-member-denied-subscope-"+uuid.NewString())
		form.Set("name", "Denied Subscope")
		form.Set("principal_id", childPrincipal.ID.String())
		form.Set("parent_id", parentScope.ID.String())

		req, err := http.NewRequest(http.MethodPost, baseURL+"/ui/scopes", strings.NewReader(form.Encode()))
		if err != nil {
			t.Fatalf("build create request: %v", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("create request: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if !strings.Contains(string(body), "scope admin required") {
			t.Fatalf("expected scope admin required error, got body=%q", string(body))
		}

		created, err := compat.GetScopeByExternalID(ctx, pool, "project", form.Get("external_id"))
		if err != nil {
			t.Fatalf("lookup denied subscope: %v", err)
		}
		if created != nil {
			t.Fatalf("subscope should not be created by non-admin member")
		}
	})
}

func TestScopedSessionToken_IncludesParentScopesInDropdowns(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	user := testhelper.CreateTestPrincipal(t, pool, "user", "ui-scoped-parent-user-"+uuid.NewString())
	parentScope := testhelper.CreateTestScope(t, pool, "project", "ui-scoped-parent-"+uuid.NewString(), nil, user.ID)
	childScope := testhelper.CreateTestScope(t, pool, "project", "ui-scoped-child-"+uuid.NewString(), &parentScope.ID, user.ID)

	rawSession, hashSession, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("generate scoped session token: %v", err)
	}
	if _, err := compat.CreateToken(ctx, pool, user.ID, hashSession, "ui-scoped-parent-session", []uuid.UUID{childScope.ID}, nil, nil); err != nil {
		t.Fatalf("create scoped session token: %v", err)
	}

	client, baseURL := loginUITestClient(t, pool, rawSession)

	pages := []string{
		"/ui/memories",
		"/ui/query",
		"/ui/graph",
		"/ui/graph3d",
	}
	for _, path := range pages {
		path := path
		t.Run(path, func(t *testing.T) {
			resp, err := client.Get(baseURL + path)
			if err != nil {
				t.Fatalf("GET %s: %v", path, err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
			}
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			text := string(body)
			if !strings.Contains(text, parentScope.ExternalID) {
				t.Fatalf("expected parent scope %q in %s", parentScope.ExternalID, path)
			}
			if !strings.Contains(text, childScope.ExternalID) {
				t.Fatalf("expected child scope %q in %s", childScope.ExternalID, path)
			}
		})
	}
}

func TestScopesPage_ShowsAttachedRepositoryForProjectScope(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	user := testhelper.CreateTestPrincipal(t, pool, "user", "ui-scope-repo-user-"+uuid.NewString())
	scope := testhelper.CreateTestScope(t, pool, "project", "ui-scope-repo-"+uuid.NewString(), nil, user.ID)

	repoURL := "https://github.com/acme/repo.git"
	branch := "main"
	if _, err := compat.SetScopeRepo(ctx, pool, scope.ID, repoURL, branch); err != nil {
		t.Fatalf("set scope repo: %v", err)
	}

	rawSession, hashSession, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("generate session token: %v", err)
	}
	if _, err := compat.CreateToken(ctx, pool, user.ID, hashSession, "repo-session", nil, nil, nil); err != nil {
		t.Fatalf("create session token: %v", err)
	}

	client, baseURL := loginUITestClient(t, pool, rawSession)

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
	if !strings.Contains(bodyText, repoURL) {
		t.Fatalf("expected attached repository URL %q in scopes page", repoURL)
	}
	if !strings.Contains(bodyText, "branch: "+branch) {
		t.Fatalf("expected branch %q in scopes page", branch)
	}
}
