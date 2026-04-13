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

func TestREST_PrincipalAdminAuthz_MutationsRequireAdmin(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()
	cfg := &config.Config{}

	actor := testhelper.CreateTestPrincipal(t, pool, "user", "rest-principal-admin-actor-"+uuid.NewString())
	target := testhelper.CreateTestPrincipal(t, pool, "user", "rest-principal-admin-target-"+uuid.NewString())
	other := testhelper.CreateTestPrincipal(t, pool, "user", "rest-principal-admin-other-"+uuid.NewString())

	rawToken, hashToken, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	if _, err := compat.CreateToken(ctx, pool, actor.ID, hashToken, "rest-principal-admin-actor-token", nil, nil, nil); err != nil {
		t.Fatalf("create token: %v", err)
	}

	handler := rest.NewRouter(pool, svc, cfg).Handler()
	srv := httptest.NewServer(handler)
	defer srv.Close()

	t.Run("non-admin update principal is forbidden", func(t *testing.T) {
		body := map[string]any{"display_name": "updated by non-admin"}
		payload, _ := json.Marshal(body)
		req, err := http.NewRequest(http.MethodPut, srv.URL+"/v1/principals/"+target.ID.String(), bytes.NewReader(payload))
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

	t.Run("non-admin create principal is forbidden", func(t *testing.T) {
		body := map[string]any{
			"kind":         "team",
			"slug":         "rest-principal-non-admin-create-" + uuid.NewString(),
			"display_name": "Denied Team",
		}
		payload, _ := json.Marshal(body)
		req, err := http.NewRequest(http.MethodPost, srv.URL+"/v1/principals", bytes.NewReader(payload))
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

	t.Run("admin update principal is allowed", func(t *testing.T) {
		adminHub := testhelper.CreateTestPrincipal(t, pool, "team", "rest-principal-admin-hub-"+uuid.NewString())
		ms := principals.NewMembershipStore(pool)
		if err := ms.AddMembership(ctx, actor.ID, adminHub.ID, "admin", nil); err != nil {
			t.Fatalf("add actor admin membership: %v", err)
		}
		if err := ms.AddMembership(ctx, target.ID, adminHub.ID, "member", nil); err != nil {
			t.Fatalf("add target membership to admin hub: %v", err)
		}

		body := map[string]any{"display_name": "updated by admin"}
		payload, _ := json.Marshal(body)
		req, err := http.NewRequest(http.MethodPut, srv.URL+"/v1/principals/"+target.ID.String(), bytes.NewReader(payload))
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
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
	})

	t.Run("admin can manage memberships", func(t *testing.T) {
		adminHub := testhelper.CreateTestPrincipal(t, pool, "department", "rest-principal-admin-membership-hub-"+uuid.NewString())
		ms := principals.NewMembershipStore(pool)
		if err := ms.AddMembership(ctx, actor.ID, adminHub.ID, "admin", nil); err != nil {
			t.Fatalf("add actor admin membership: %v", err)
		}
		if err := ms.AddMembership(ctx, target.ID, adminHub.ID, "member", nil); err != nil {
			t.Fatalf("add target membership: %v", err)
		}

		addBody := map[string]any{
			"member_id": other.ID.String(),
			"role":      "member",
		}
		payload, _ := json.Marshal(addBody)
		addReq, err := http.NewRequest(http.MethodPost, srv.URL+"/v1/principals/"+target.ID.String()+"/members", bytes.NewReader(payload))
		if err != nil {
			t.Fatalf("new add request: %v", err)
		}
		addReq.Header.Set("Authorization", "Bearer "+rawToken)
		addReq.Header.Set("Content-Type", "application/json")

		addResp, err := http.DefaultClient.Do(addReq)
		if err != nil {
			t.Fatalf("do add request: %v", err)
		}
		defer addResp.Body.Close()
		if addResp.StatusCode != http.StatusCreated {
			t.Fatalf("add status = %d, want %d", addResp.StatusCode, http.StatusCreated)
		}

		removeReq, err := http.NewRequest(http.MethodDelete, srv.URL+"/v1/principals/"+target.ID.String()+"/members/"+other.ID.String(), nil)
		if err != nil {
			t.Fatalf("new remove request: %v", err)
		}
		removeReq.Header.Set("Authorization", "Bearer "+rawToken)

		removeResp, err := http.DefaultClient.Do(removeReq)
		if err != nil {
			t.Fatalf("do remove request: %v", err)
		}
		defer removeResp.Body.Close()
		if removeResp.StatusCode != http.StatusNoContent {
			t.Fatalf("remove status = %d, want %d", removeResp.StatusCode, http.StatusNoContent)
		}
	})
}
