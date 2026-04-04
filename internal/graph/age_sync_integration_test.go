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

func TestSyncEntityToAGE_And_SyncRelationToAGE(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	if err := db.EnsureAGEOverlay(ctx, pool); err != nil {
		t.Fatalf("EnsureAGEOverlay: %v", err)
	}

	scopeID := uuid.New()
	subjectID := uuid.New()
	objectID := uuid.New()

	subject := &db.Entity{
		ID:         subjectID,
		ScopeID:    scopeID,
		EntityType: "function",
		Name:       "Authenticate",
		Canonical:  "auth.Authenticate",
	}
	object := &db.Entity{
		ID:         objectID,
		ScopeID:    scopeID,
		EntityType: "function",
		Name:       "IssueJWT",
		Canonical:  "auth.IssueJWT",
	}

	if !graph.DetectAGE(ctx, pool) {
		if err := graph.SyncEntityToAGE(ctx, pool, subject); !errors.Is(err, graph.ErrAGEUnavailable) {
			t.Fatalf("SyncEntityToAGE without AGE: err=%v, want ErrAGEUnavailable", err)
		}
		return
	}

	if err := graph.SyncEntityToAGE(ctx, pool, subject); err != nil {
		t.Fatalf("SyncEntityToAGE(subject): %v", err)
	}
	if err := graph.SyncEntityToAGE(ctx, pool, object); err != nil {
		t.Fatalf("SyncEntityToAGE(object): %v", err)
	}

	rel := &db.Relation{
		ScopeID:    scopeID,
		SubjectID:  subjectID,
		ObjectID:   objectID,
		Predicate:  "depends_on",
		Confidence: 0.93,
	}
	if err := graph.SyncRelationToAGE(ctx, pool, rel); err != nil {
		t.Fatalf("SyncRelationToAGE: %v", err)
	}

	nodes, err := graph.RunCypherQuery(ctx, pool, scopeID, fmt.Sprintf("MATCH (n:Entity {id: '%s'}) RETURN n", subjectID.String()))
	if err != nil {
		t.Fatalf("RunCypherQuery(entity verify): %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("entity verify row count = %d, want 1", len(nodes))
	}

	edges, err := graph.RunCypherQuery(ctx, pool, scopeID, fmt.Sprintf("MATCH (n)-[r:RELATION {predicate: 'depends_on'}]->(b:Entity {id: '%s'}) RETURN r", objectID.String()))
	if err != nil {
		t.Fatalf("RunCypherQuery(relation verify): %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("relation verify row count = %d, want 1", len(edges))
	}
}
