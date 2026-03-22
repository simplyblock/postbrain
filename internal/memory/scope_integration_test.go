//go:build integration

package memory_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/memory"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

func TestScopeFanOut_WalksAncestors(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	// Build hierarchy: company → department → team → project
	compPrincipal := testhelper.CreateTestPrincipal(t, pool, "company", "acme-co")
	deptPrincipal := testhelper.CreateTestPrincipal(t, pool, "department", "eng-dept")
	teamPrincipal := testhelper.CreateTestPrincipal(t, pool, "team", "platform-team")
	projPrincipal := testhelper.CreateTestPrincipal(t, pool, "user", "proj-owner")
	userPrincipal := testhelper.CreateTestPrincipal(t, pool, "user", "fanout-user")

	compScope := testhelper.CreateTestScope(t, pool, "company", "acme-co", nil, compPrincipal.ID)
	deptScope := testhelper.CreateTestScope(t, pool, "department", "acme-co_eng", &compScope.ID, deptPrincipal.ID)
	teamScope := testhelper.CreateTestScope(t, pool, "team", "acme-co_eng_plat", &deptScope.ID, teamPrincipal.ID)
	projScope := testhelper.CreateTestScope(t, pool, "project", "acme_api", &teamScope.ID, projPrincipal.ID)

	scopeIDs, err := memory.FanOutScopeIDs(ctx, pool, projScope.ID, userPrincipal.ID, 0, false)
	if err != nil {
		t.Fatalf("FanOutScopeIDs: %v", err)
	}

	want := map[uuid.UUID]bool{
		projScope.ID: true,
		teamScope.ID: true,
		deptScope.ID: true,
		compScope.ID: true,
	}
	for _, id := range scopeIDs {
		delete(want, id)
	}
	if len(want) > 0 {
		t.Errorf("fan-out missing scope IDs: %v", want)
	}
}

func TestScopeFanOut_StrictScope(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	p := testhelper.CreateTestPrincipal(t, pool, "user", "strict-user")
	s := testhelper.CreateTestScope(t, pool, "project", "strict_proj", nil, p.ID)

	scopeIDs, err := memory.FanOutScopeIDs(ctx, pool, s.ID, p.ID, 0, true)
	if err != nil {
		t.Fatalf("FanOutScopeIDs: %v", err)
	}
	if len(scopeIDs) != 1 || scopeIDs[0] != s.ID {
		t.Errorf("strict_scope=true: expected [%v], got %v", s.ID, scopeIDs)
	}
}
