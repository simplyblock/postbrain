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
