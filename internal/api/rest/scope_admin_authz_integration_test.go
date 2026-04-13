//go:build integration

package rest_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/api/rest"
	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/config"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/db/compat"
	"github.com/simplyblock/postbrain/internal/principals"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

func TestREST_ScopeAdminAuthz_MemberCannotAdminParentScope(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()
	cfg := &config.Config{}

	parentPrincipal := testhelper.CreateTestPrincipal(t, pool, "team", "rest-scope-admin-parent-"+uuid.NewString())
	childPrincipal := testhelper.CreateTestPrincipal(t, pool, "user", "rest-scope-admin-child-"+uuid.NewString())
	parentScope := testhelper.CreateTestScope(t, pool, "project", "rest-scope-admin-parent-scope-"+uuid.NewString(), nil, parentPrincipal.ID)

	ms := principals.NewMembershipStore(pool)
	if err := ms.AddMembership(ctx, childPrincipal.ID, parentPrincipal.ID, "member", nil); err != nil {
		t.Fatalf("add membership child->parent: %v", err)
	}

	rawToken, hashToken, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	if _, err := compat.CreateToken(ctx, pool, childPrincipal.ID, hashToken, "rest-scope-admin-member-token", nil, nil, nil); err != nil {
		t.Fatalf("create token: %v", err)
	}

	handler := rest.NewRouter(pool, svc, cfg).Handler()
	srv := httptest.NewServer(handler)
	defer srv.Close()

	t.Run("member cannot update parent scope", func(t *testing.T) {
		body := map[string]any{"name": "renamed-by-member"}
		payload, _ := json.Marshal(body)
		req, err := http.NewRequest(http.MethodPut, srv.URL+"/v1/scopes/"+parentScope.ID.String(), bytes.NewReader(payload))
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+rawToken)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("do request: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusForbidden)
		}
	})

	t.Run("member cannot delete parent scope", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodDelete, srv.URL+"/v1/scopes/"+parentScope.ID.String(), nil)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+rawToken)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("do request: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusForbidden)
		}
	})

	t.Run("member cannot create child scope under parent scope", func(t *testing.T) {
		body := map[string]any{
			"kind":         "project",
			"external_id":  "rest-member-denied-subscope-" + uuid.NewString(),
			"name":         "member denied subscope",
			"principal_id": childPrincipal.ID,
			"parent_id":    parentScope.ID,
		}
		payload, _ := json.Marshal(body)
		req, err := http.NewRequest(http.MethodPost, srv.URL+"/v1/scopes", bytes.NewReader(payload))
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+rawToken)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("do request: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusForbidden)
		}
	})
}
