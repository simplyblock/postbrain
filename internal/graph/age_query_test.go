package graph

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestBuildScopedCypher_PrependsScopeAnchor(t *testing.T) {
	scopeID := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	cypher := "RETURN n"

	got := buildScopedCypher(scopeID, cypher)
	if !strings.Contains(got, "MATCH (n:Entity)") {
		t.Fatalf("buildScopedCypher missing entity match: %q", got)
	}
	if !strings.Contains(got, "WHERE n.scope_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'") {
		t.Fatalf("buildScopedCypher missing scope anchor: %q", got)
	}
	if strings.Contains(got, "{scope_id:") {
		t.Fatalf("buildScopedCypher must avoid map-pattern filter for AGE compatibility: %q", got)
	}
	if !strings.Contains(got, "RETURN n") {
		t.Fatalf("buildScopedCypher missing original cypher: %q", got)
	}
}

func TestRunCypherSQL_UsesSchemaQualifiedAGEObjects(t *testing.T) {
	sql := buildAGECypherSQL("RETURN 1")
	if !strings.Contains(sql, "ag_catalog.cypher(") {
		t.Fatalf("AGE query SQL must use schema-qualified ag_catalog.cypher: %q", sql)
	}
	if !strings.Contains(sql, "ag_catalog.agtype") {
		t.Fatalf("AGE query SQL must use schema-qualified ag_catalog.agtype: %q", sql)
	}
	if !strings.Contains(sql, "$postbrain$RETURN 1$postbrain$") {
		t.Fatalf("AGE query SQL must embed cypher in dollar-quoted literal: %q", sql)
	}
	if strings.Contains(sql, "$1") {
		t.Fatalf("AGE query SQL must not use bind params for cypher body: %q", sql)
	}
}

func TestBuildAGECypherSQL_DollarTagCollisionGetsEscaped(t *testing.T) {
	sql := buildAGECypherSQL("RETURN '$postbrain$'")
	if strings.Contains(sql, "$postbrain$RETURN '$postbrain$'$postbrain$") {
		t.Fatalf("expected builder to avoid delimiter collision: %q", sql)
	}
}
