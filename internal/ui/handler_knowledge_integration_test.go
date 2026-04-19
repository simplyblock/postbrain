//go:build integration

package ui_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/db/compat"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

// TestHandleKnowledgeDetail_CrossScope_Returns404 verifies that an artifact
// belonging to scopeB cannot be read via scopeA's URL path. The handler must
// return 404 and not expose the artifact's contents.
func TestHandleKnowledgeDetail_CrossScope_Returns404(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	suffix := uuid.NewString()

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "kw-detail-user-"+suffix)
	scopeA := testhelper.CreateTestScope(t, pool, "project", "kw-detail-scope-a-"+suffix, nil, principal.ID)
	scopeB := testhelper.CreateTestScope(t, pool, "project", "kw-detail-scope-b-"+suffix, nil, principal.ID)

	artB := testhelper.CreateTestArtifact(t, pool, scopeB.ID, principal.ID, "SCOPEB_SECRET")

	rawToken, hashToken, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	if _, err := compat.CreateToken(t.Context(), pool, principal.ID, hashToken, "kw-detail-token-"+suffix, nil, nil, nil); err != nil {
		t.Fatalf("create token: %v", err)
	}

	client, baseURL := loginUITestClient(t, pool, rawToken)

	// Try to read scopeB's artifact via scopeA's path.
	url := baseURL + "/ui/" + scopeA.ID.String() + "/knowledge/" + artB.ID.String()
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET knowledge detail: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want %d (cross-scope artifact must not be readable)", resp.StatusCode, http.StatusNotFound)
	}
}

// TestHandleKnowledgeHistory_CrossScope_Returns404 verifies that the history
// page for a scopeB artifact cannot be accessed via scopeA's URL path.
func TestHandleKnowledgeHistory_CrossScope_Returns404(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	suffix := uuid.NewString()

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "kw-history-user-"+suffix)
	scopeA := testhelper.CreateTestScope(t, pool, "project", "kw-history-scope-a-"+suffix, nil, principal.ID)
	scopeB := testhelper.CreateTestScope(t, pool, "project", "kw-history-scope-b-"+suffix, nil, principal.ID)

	artB := testhelper.CreateTestArtifact(t, pool, scopeB.ID, principal.ID, "SCOPEB_HISTORY_SECRET")

	rawToken, hashToken, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	if _, err := compat.CreateToken(t.Context(), pool, principal.ID, hashToken, "kw-history-token-"+suffix, nil, nil, nil); err != nil {
		t.Fatalf("create token: %v", err)
	}

	client, baseURL := loginUITestClient(t, pool, rawToken)

	url := baseURL + "/ui/" + scopeA.ID.String() + "/knowledge/" + artB.ID.String() + "/history"
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET knowledge history: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want %d (cross-scope history must not be readable)", resp.StatusCode, http.StatusNotFound)
	}
}
