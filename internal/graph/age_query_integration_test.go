//go:build integration

package graph_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/graph"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

func TestRunCypherQuery_AGEAvailabilityModes(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	if err := db.EnsureAGEOverlay(ctx, pool); err != nil {
		t.Fatalf("EnsureAGEOverlay: %v", err)
	}

	if !graph.DetectAGE(ctx, pool) {
		_, err := graph.RunCypherQuery(ctx, pool, uuid.New(), "RETURN n")
		if !errors.Is(err, graph.ErrAGEUnavailable) {
			t.Fatalf("RunCypherQuery without AGE: err=%v, want ErrAGEUnavailable", err)
		}
		return
	}

	scopeID := uuid.New()
	otherScopeID := uuid.New()

	insertScopeNode := func(id, scope uuid.UUID) {
		t.Helper()
		q := fmt.Sprintf(`
SELECT * FROM cypher('postbrain', $$
  MERGE (e:Entity {id: '%s'})
  SET e.scope_id = '%s', e.name = 'age-test'
  RETURN e
$$) AS (result agtype);`, id.String(), scope.String())
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatalf("insert AGE test node: %v", err)
		}
	}
	insertScopeNode(uuid.New(), scopeID)
	insertScopeNode(uuid.New(), otherScopeID)

	rows, err := graph.RunCypherQuery(ctx, pool, scopeID, "RETURN n")
	if err != nil {
		t.Fatalf("RunCypherQuery with AGE: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("RunCypherQuery row count = %d, want 1", len(rows))
	}
}
