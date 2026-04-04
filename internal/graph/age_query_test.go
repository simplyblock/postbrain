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
