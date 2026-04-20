//go:build integration

package ui_test

import (
	"net/http"
	"net/url"
	"strings"
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

// TestHandleKnowledgeDelete_CrossScope_Returns404 verifies that a principal
// with access to both scopes cannot delete a scopeB artifact via scopeA's URL.
func TestHandleKnowledgeDelete_CrossScope_Returns404(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	suffix := uuid.NewString()

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "kw-delete-user-"+suffix)
	scopeA := testhelper.CreateTestScope(t, pool, "project", "kw-delete-scope-a-"+suffix, nil, principal.ID)
	scopeB := testhelper.CreateTestScope(t, pool, "project", "kw-delete-scope-b-"+suffix, nil, principal.ID)

	artB := testhelper.CreateTestArtifact(t, pool, scopeB.ID, principal.ID, "SCOPEB_PROTECTED")

	rawToken, hashToken, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	if _, err := compat.CreateToken(t.Context(), pool, principal.ID, hashToken, "kw-delete-token-"+suffix, nil, nil, nil); err != nil {
		t.Fatalf("create token: %v", err)
	}

	client, baseURL := loginUITestClient(t, pool, rawToken)

	// Attempt cross-scope delete via scopeA path.
	deleteURL := baseURL + "/ui/" + scopeA.ID.String() + "/knowledge/" + artB.ID.String() + "/delete"
	form := url.Values{}
	req, _ := http.NewRequest(http.MethodPost, deleteURL, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST knowledge delete: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want %d (cross-scope delete must be rejected)", resp.StatusCode, http.StatusNotFound)
	}

	// Confirm the artifact was not deleted.
	art, err := compat.GetArtifact(t.Context(), pool, artB.ID)
	if err != nil {
		t.Fatalf("GetArtifact after cross-scope delete attempt: %v", err)
	}
	if art == nil {
		t.Error("artifact was deleted despite cross-scope delete attempt")
	}
}

// TestHandleKnowledgeRetract_CrossScope_Returns404 verifies that retract is
// also guarded, including the scopedArtifact check that now precedes the
// knwLife nil-check.
func TestHandleKnowledgeRetract_CrossScope_Returns404(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	suffix := uuid.NewString()

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "kw-retract-user-"+suffix)
	scopeA := testhelper.CreateTestScope(t, pool, "project", "kw-retract-scope-a-"+suffix, nil, principal.ID)
	scopeB := testhelper.CreateTestScope(t, pool, "project", "kw-retract-scope-b-"+suffix, nil, principal.ID)

	artB := testhelper.CreateTestArtifact(t, pool, scopeB.ID, principal.ID, "SCOPEB_RETRACT_TARGET")

	rawToken, hashToken, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	if _, err := compat.CreateToken(t.Context(), pool, principal.ID, hashToken, "kw-retract-token-"+suffix, nil, nil, nil); err != nil {
		t.Fatalf("create token: %v", err)
	}

	client, baseURL := loginUITestClient(t, pool, rawToken)

	retractURL := baseURL + "/ui/" + scopeA.ID.String() + "/knowledge/" + artB.ID.String() + "/retract"
	form := url.Values{}
	req, _ := http.NewRequest(http.MethodPost, retractURL, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST knowledge retract: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want %d (cross-scope retract must be rejected)", resp.StatusCode, http.StatusNotFound)
	}
}
