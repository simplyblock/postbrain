package db

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestRunAGECypherSQL_UsesSchemaQualifiedAGEObjects(t *testing.T) {
	sql := buildAGECypherSQLLiteral("RETURN 1")
	if !strings.Contains(sql, "ag_catalog.cypher(") {
		t.Fatalf("AGE dual-write SQL must use schema-qualified ag_catalog.cypher: %q", sql)
	}
	if !strings.Contains(sql, "ag_catalog.agtype") {
		t.Fatalf("AGE dual-write SQL must use schema-qualified ag_catalog.agtype: %q", sql)
	}
	if !strings.Contains(sql, "$postbrain$RETURN 1$postbrain$") {
		t.Fatalf("AGE dual-write SQL must embed cypher in dollar-quoted literal: %q", sql)
	}
	if strings.Contains(sql, "$1") {
		t.Fatalf("AGE dual-write SQL must not use bind params for cypher body: %q", sql)
	}
}

func TestBuildAGECypherSQLLiteral_DollarTagCollisionGetsEscaped(t *testing.T) {
	sql := buildAGECypherSQLLiteral("RETURN '$postbrain$'")
	if strings.Contains(sql, "$postbrain$RETURN '$postbrain$'$postbrain$") {
		t.Fatalf("expected builder to avoid delimiter collision: %q", sql)
	}
}

func TestBuildEntityUpsertCypher_UsesMerge(t *testing.T) {
	entity := &Entity{
		ID:         uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"),
		ScopeID:    uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"),
		EntityType: "function",
		Name:       "O'Brien",
		Canonical:  "auth.O'Brien",
	}

	cypher := buildEntityUpsertCypher(entity)
	if !strings.Contains(cypher, "MERGE (e:Entity {id: 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'})") {
		t.Fatalf("entity dual-write must MERGE by id: %q", cypher)
	}
	if strings.Contains(cypher, "CREATE (e:Entity)") {
		t.Fatalf("entity dual-write must not use standalone CREATE fallback: %q", cypher)
	}
	if !strings.Contains(cypher, "O\\'Brien") {
		t.Fatalf("entity dual-write should escape single quotes: %q", cypher)
	}
}

func TestBuildRelationUpsertCypher_UsesMerge(t *testing.T) {
	rel := &Relation{
		ScopeID:    uuid.MustParse("cccccccc-cccc-cccc-cccc-cccccccccccc"),
		SubjectID:  uuid.MustParse("dddddddd-dddd-dddd-dddd-dddddddddddd"),
		ObjectID:   uuid.MustParse("eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee"),
		Predicate:  "depends_on'oops",
		Confidence: 0.91,
	}
	cypher := buildRelationUpsertCypher(rel)
	if !strings.Contains(cypher, "MERGE (a)-[r:RELATION {predicate: 'depends_on\\'oops'}]->(b)") {
		t.Fatalf("relation dual-write must MERGE by subject/object/predicate: %q", cypher)
	}
	if strings.Contains(cypher, "CREATE (a)-[r:RELATION]->(b)") {
		t.Fatalf("relation dual-write must not use standalone CREATE fallback: %q", cypher)
	}
}
