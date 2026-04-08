package graph

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/db"
)

func TestBuildEntityUpsertCypher_UsesMergeAndEscapesStrings(t *testing.T) {
	entity := &db.Entity{
		ID:         uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		ScopeID:    uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		EntityType: "function",
		Name:       "O'Brien",
		Canonical:  "auth.O'Brien",
	}

	cypher := buildEntityUpsertCypher(entity)
	if !strings.Contains(cypher, "MERGE (e:Entity {id: '11111111-1111-1111-1111-111111111111'})") {
		t.Fatalf("entity upsert must MERGE by id:\n%s", cypher)
	}
	if strings.Contains(cypher, "CREATE (e:Entity)") {
		t.Fatalf("entity upsert must not use standalone CREATE fallback:\n%s", cypher)
	}
	if !strings.Contains(cypher, "O\\'Brien") {
		t.Fatalf("entity upsert must escape single quotes:\n%s", cypher)
	}
}

func TestBuildRelationUpsertCypher_UsesMergeAndEscapesPredicate(t *testing.T) {
	rel := &db.Relation{
		ScopeID:    uuid.MustParse("33333333-3333-3333-3333-333333333333"),
		SubjectID:  uuid.MustParse("44444444-4444-4444-4444-444444444444"),
		ObjectID:   uuid.MustParse("55555555-5555-5555-5555-555555555555"),
		Predicate:  "depends_on'oops",
		Confidence: 0.95,
	}

	cypher := buildRelationUpsertCypher(rel)
	if !strings.Contains(cypher, "MERGE (a)-[r:RELATION {predicate: 'depends_on\\'oops'}]->(b)") {
		t.Fatalf("relation upsert must MERGE by subject/object/predicate:\n%s", cypher)
	}
	if strings.Contains(cypher, "CREATE (a)-[r:RELATION]->(b)") {
		t.Fatalf("relation upsert must not use standalone CREATE fallback:\n%s", cypher)
	}
	if !strings.Contains(cypher, "SET r.confidence = 0.95") {
		t.Fatalf("relation upsert must set confidence:\n%s", cypher)
	}
	if !strings.Contains(cypher, "r.scope_id = '33333333-3333-3333-3333-333333333333'") {
		t.Fatalf("relation upsert must set scope_id:\n%s", cypher)
	}
}
