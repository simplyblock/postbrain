package db

import (
	"strings"
	"testing"
)

func TestRunAGECypherSQL_UsesSchemaQualifiedAGEObjects(t *testing.T) {
	if !strings.Contains(runAGECypherSQL, "ag_catalog.cypher(") {
		t.Fatalf("runAGECypherSQL must use schema-qualified ag_catalog.cypher: %q", runAGECypherSQL)
	}
	if !strings.Contains(runAGECypherSQL, "ag_catalog.agtype") {
		t.Fatalf("runAGECypherSQL must use schema-qualified ag_catalog.agtype: %q", runAGECypherSQL)
	}
}
