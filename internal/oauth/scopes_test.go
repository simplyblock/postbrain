package oauth

import (
	"reflect"
	"testing"
)

func TestParseScopes_ValidSingle_OK(t *testing.T) {
	scopes, err := ParseScopes(ScopeMemoriesRead)
	if err != nil {
		t.Fatalf("ParseScopes: %v", err)
	}
	want := []string{ScopeMemoriesRead}
	if !reflect.DeepEqual(scopes, want) {
		t.Fatalf("ParseScopes single = %v, want %v", scopes, want)
	}
}

func TestParseScopes_ValidMultiple_OK(t *testing.T) {
	scopes, err := ParseScopes("memories:read knowledge:write")
	if err != nil {
		t.Fatalf("ParseScopes: %v", err)
	}
	want := []string{ScopeMemoriesRead, ScopeKnowledgeWrite}
	if !reflect.DeepEqual(scopes, want) {
		t.Fatalf("ParseScopes multiple = %v, want %v", scopes, want)
	}
}

func TestParseScopes_UnknownScope_ReturnsError(t *testing.T) {
	if _, err := ParseScopes("memories:read nope:scope"); err == nil {
		t.Fatal("ParseScopes unknown scope: expected error, got nil")
	}
}

func TestParseScopes_Empty_ReturnsError(t *testing.T) {
	if _, err := ParseScopes(""); err == nil {
		t.Fatal("ParseScopes empty: expected error, got nil")
	}
}

func TestScopeToPermissions_MapsCorrectly(t *testing.T) {
	input := []string{ScopeMemoriesRead, ScopeKnowledgeWrite}
	got := ScopeToPermissions(input)
	if !reflect.DeepEqual(got, input) {
		t.Fatalf("ScopeToPermissions = %v, want %v", got, input)
	}
}

// TestParseScopes_RejectsAdmin verifies that the legacy "admin" scope is no longer accepted.
func TestParseScopes_RejectsAdmin(t *testing.T) {
	if _, err := ParseScopes("admin"); err == nil {
		t.Fatal("ParseScopes: expected error for legacy admin scope, got nil")
	}
}

// TestParseScopes_AcceptsAllResourceOperationPairs verifies that every valid
// resource:operation combination from the full authz permission model is
// accepted as an OAuth scope.
func TestParseScopes_AcceptsAllResourceOperationPairs(t *testing.T) {
	validScopes := []string{
		// memories
		"memories:read", "memories:write", "memories:edit", "memories:delete",
		// knowledge
		"knowledge:read", "knowledge:write", "knowledge:edit", "knowledge:delete",
		// collections
		"collections:read", "collections:write", "collections:edit", "collections:delete",
		// skills
		"skills:read", "skills:write",
		// sessions (no delete operation)
		"sessions:read", "sessions:write",
		// graph
		"graph:read",
		// scopes
		"scopes:read", "scopes:write", "scopes:edit", "scopes:delete",
		// principals
		"principals:read", "principals:write", "principals:edit", "principals:delete",
		// tokens
		"tokens:read", "tokens:write", "tokens:edit", "tokens:delete",
		// sharing
		"sharing:read", "sharing:write", "sharing:delete",
		// promotions
		"promotions:read", "promotions:write",
	}
	for _, scope := range validScopes {
		scopes, err := ParseScopes(scope)
		if err != nil {
			t.Errorf("ParseScopes(%q): unexpected error: %v", scope, err)
			continue
		}
		if len(scopes) != 1 || scopes[0] != scope {
			t.Errorf("ParseScopes(%q) = %v, want [%s]", scope, scopes, scope)
		}
	}
}
