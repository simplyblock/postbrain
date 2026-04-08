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
	if !strings.Contains(got, "MATCH (n:Entity {scope_id: 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'})") {
		t.Fatalf("buildScopedCypher missing scope anchor: %q", got)
	}
	if !strings.Contains(got, "RETURN n") {
		t.Fatalf("buildScopedCypher missing original cypher: %q", got)
	}
}

func TestRunCypherSQL_UsesSchemaQualifiedAGEObjects(t *testing.T) {
	if !strings.Contains(runCypherSQL, "ag_catalog.cypher(") {
		t.Fatalf("runCypherSQL must use schema-qualified ag_catalog.cypher: %q", runCypherSQL)
	}
	if !strings.Contains(runCypherSQL, "ag_catalog.agtype") {
		t.Fatalf("runCypherSQL must use schema-qualified ag_catalog.agtype: %q", runCypherSQL)
	}
}
