//go:build integration

package ui_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

func TestUI_Pages_FilterScopesAndScopedData(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	userA := testhelper.CreateTestPrincipal(t, pool, "user", "ui-filter-user-a")
	userB := testhelper.CreateTestPrincipal(t, pool, "user", "ui-filter-user-b")
	scopeA := testhelper.CreateTestScope(t, pool, "project", "ui-filter-scope-a", nil, userA.ID)
	scopeB := testhelper.CreateTestScope(t, pool, "project", "ui-filter-scope-b", nil, userB.ID)

	// Seed scoped content across pages.
	_ = testhelper.CreateTestMemory(t, pool, scopeA.ID, userA.ID, "VISIBLE_MEMORY")
	_ = testhelper.CreateTestMemory(t, pool, scopeB.ID, userB.ID, "HIDDEN_MEMORY")
	visibleArtifact := testhelper.CreateTestArtifact(t, pool, scopeA.ID, userA.ID, "VISIBLE_ARTIFACT")
	hiddenArtifact := testhelper.CreateTestArtifact(t, pool, scopeB.ID, userB.ID, "HIDDEN_ARTIFACT")

	if _, err := db.CreateCollection(ctx, pool, &db.KnowledgeCollection{
		ScopeID:    scopeA.ID,
		OwnerID:    userA.ID,
		Slug:       "visible-collection",
		Name:       "VISIBLE_COLLECTION",
		Visibility: "team",
	}); err != nil {
		t.Fatalf("create visible collection: %v", err)
	}
	if _, err := db.CreateCollection(ctx, pool, &db.KnowledgeCollection{
		ScopeID:    scopeB.ID,
		OwnerID:    userB.ID,
		Slug:       "hidden-collection",
		Name:       "HIDDEN_COLLECTION",
		Visibility: "team",
	}); err != nil {
		t.Fatalf("create hidden collection: %v", err)
	}

	now := time.Now().UTC()
	if _, err := db.CreateSkill(ctx, pool, &db.Skill{
		ScopeID:        scopeA.ID,
		AuthorID:       userA.ID,
		Slug:           "visible-skill",
		Name:           "VISIBLE_SKILL",
		Description:    "visible",
		AgentTypes:     []string{"copilot"},
		Body:           "visible body",
		Parameters:     []byte("{}"),
		Visibility:     "team",
		Status:         "published",
		PublishedAt:    &now,
		ReviewRequired: 0,
		Version:        1,
	}); err != nil {
		t.Fatalf("create visible skill: %v", err)
	}
	if _, err := db.CreateSkill(ctx, pool, &db.Skill{
		ScopeID:        scopeB.ID,
		AuthorID:       userB.ID,
		Slug:           "hidden-skill",
		Name:           "HIDDEN_SKILL",
		Description:    "hidden",
		AgentTypes:     []string{"copilot"},
		Body:           "hidden body",
		Parameters:     []byte("{}"),
		Visibility:     "team",
		Status:         "published",
		PublishedAt:    &now,
		ReviewRequired: 0,
		Version:        1,
	}); err != nil {
		t.Fatalf("create hidden skill: %v", err)
	}

	if _, err := db.CreatePromotionRequest(ctx, pool, &db.PromotionRequest{
		MemoryID:         testhelper.CreateTestMemory(t, pool, scopeA.ID, userA.ID, "visible promotion memory").ID,
		RequestedBy:      userA.ID,
		TargetScopeID:    scopeA.ID,
		TargetVisibility: "team",
		Status:           "pending",
	}); err != nil {
		t.Fatalf("create visible promotion: %v", err)
	}
	if _, err := db.CreatePromotionRequest(ctx, pool, &db.PromotionRequest{
		MemoryID:         testhelper.CreateTestMemory(t, pool, scopeB.ID, userB.ID, "hidden promotion memory").ID,
		RequestedBy:      userB.ID,
		TargetScopeID:    scopeB.ID,
		TargetVisibility: "team",
		Status:           "pending",
	}); err != nil {
		t.Fatalf("create hidden promotion: %v", err)
	}

	if _, err := db.InsertStalenessFlag(ctx, pool, &db.StalenessFlag{
		ArtifactID: visibleArtifact.ID,
		Signal:     "low_access_age",
		Confidence: 0.8,
		Evidence:   []byte("{}"),
		Status:     "open",
		FlaggedAt:  now,
		ReviewedBy: nil,
		ReviewedAt: nil,
		ReviewNote: nil,
	}); err != nil {
		t.Fatalf("insert visible staleness flag: %v", err)
	}
	if _, err := db.InsertStalenessFlag(ctx, pool, &db.StalenessFlag{
		ArtifactID: hiddenArtifact.ID,
		Signal:     "low_access_age",
		Confidence: 0.8,
		Evidence:   []byte("{}"),
		Status:     "open",
		FlaggedAt:  now,
	}); err != nil {
		t.Fatalf("insert hidden staleness flag: %v", err)
	}

	rawSessionA, hashSessionA, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("generate session token: %v", err)
	}
	if _, err := db.CreateToken(ctx, pool, userA.ID, hashSessionA, "ui-filter-session", nil, nil, nil); err != nil {
		t.Fatalf("create session token: %v", err)
	}

	client, baseURL := loginUITestClient(t, pool, rawSessionA)

	pages := []struct {
		path           string
		mustContain    []string
		mustNotContain []string
	}{
		{path: "/ui/memories", mustContain: []string{"ui-filter-scope-a"}, mustNotContain: []string{"ui-filter-scope-b"}},
		{path: "/ui/query", mustContain: []string{"ui-filter-scope-a"}, mustNotContain: []string{"ui-filter-scope-b"}},
		{path: "/ui/knowledge", mustContain: []string{"VISIBLE_ARTIFACT"}, mustNotContain: []string{"ui-filter-scope-b", "HIDDEN_ARTIFACT"}},
		{path: "/ui/collections", mustContain: []string{"VISIBLE_COLLECTION"}, mustNotContain: []string{"HIDDEN_COLLECTION"}},
		{path: "/ui/promotions", mustContain: []string{"ui-filter-scope-a"}, mustNotContain: []string{"ui-filter-scope-b"}},
		{path: "/ui/staleness", mustContain: []string{visibleArtifact.ID.String()}, mustNotContain: []string{hiddenArtifact.ID.String()}},
		{path: "/ui/skills", mustContain: []string{"VISIBLE_SKILL"}, mustNotContain: []string{"HIDDEN_SKILL"}},
		{path: "/ui/graph", mustContain: []string{"ui-filter-scope-a"}, mustNotContain: []string{"ui-filter-scope-b"}},
		{path: "/ui/graph3d", mustContain: []string{"ui-filter-scope-a"}, mustNotContain: []string{"ui-filter-scope-b"}},
	}

	for _, tc := range pages {
		tc := tc
		t.Run(tc.path, func(t *testing.T) {
			resp, err := client.Get(baseURL + tc.path)
			if err != nil {
				t.Fatalf("GET %s: %v", tc.path, err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
			}
			bodyBytes := make([]byte, 0)
			buf := make([]byte, 4096)
			for {
				n, readErr := resp.Body.Read(buf)
				if n > 0 {
					bodyBytes = append(bodyBytes, buf[:n]...)
				}
				if readErr != nil {
					break
				}
			}
			body := string(bodyBytes)
			for _, s := range tc.mustContain {
				if !strings.Contains(body, s) {
					t.Fatalf("expected %q in %s response", s, tc.path)
				}
			}
			for _, s := range tc.mustNotContain {
				if strings.Contains(body, s) {
					t.Fatalf("did not expect %q in %s response", s, tc.path)
				}
			}
		})
	}
}
