//go:build integration

package ui_test

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/db/compat"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

// TestHandleMemoryForget_CrossScope_Returns404 verifies that a principal with
// access to two scopes cannot delete a memory belonging to scopeB by posting
// to scopeA's forget URL. The response must be 404 regardless of whether the
// memory exists, to avoid cross-scope existence leakage.
func TestHandleMemoryForget_CrossScope_Returns404(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()
	suffix := uuid.NewString()

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "mem-forget-user-"+suffix)
	scopeA := testhelper.CreateTestScope(t, pool, "project", "mem-forget-scope-a-"+suffix, nil, principal.ID)
	scopeB := testhelper.CreateTestScope(t, pool, "project", "mem-forget-scope-b-"+suffix, nil, principal.ID)

	// Memory belongs to scopeB.
	memB := testhelper.CreateTestMemory(t, pool, scopeB.ID, principal.ID, "scopeB memory to protect")

	rawToken, hashToken, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	if _, err := compat.CreateToken(ctx, pool, principal.ID, hashToken, "mem-forget-token-"+suffix, nil, nil, nil); err != nil {
		t.Fatalf("create token: %v", err)
	}

	client, baseURL := loginUITestClient(t, pool, rawToken)

	// Attempt to delete scopeB's memory via scopeA's path.
	forgetURL := baseURL + "/ui/" + scopeA.ID.String() + "/memories/" + memB.ID.String() + "/forget"
	form := url.Values{}
	req, _ := http.NewRequest(http.MethodPost, forgetURL, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("forget request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want %d (cross-scope delete must be rejected)", resp.StatusCode, http.StatusNotFound)
	}

	// Verify the memory was NOT soft-deleted (IsActive must still be true).
	mem, err := compat.GetMemory(ctx, pool, memB.ID)
	if err != nil {
		t.Fatalf("GetMemory after cross-scope forget attempt: %v", err)
	}
	if mem == nil || !mem.IsActive {
		t.Error("memory was soft-deleted despite cross-scope forget attempt")
	}
}
