package db

import (
	"strings"
	"testing"
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
