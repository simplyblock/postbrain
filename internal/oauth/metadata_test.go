package oauth

import "testing"

func TestServerMetadata_ContainsRequiredFields(t *testing.T) {
	md := ServerMetadata("https://postbrain.example.com")
	required := []string{
		"issuer",
		"authorization_endpoint",
		"token_endpoint",
		"registration_endpoint",
		"scopes_supported",
		"response_types_supported",
		"code_challenge_methods_supported",
	}
	for _, key := range required {
		if _, ok := md[key]; !ok {
			t.Fatalf("missing metadata key %q", key)
		}
	}
}

func TestServerMetadata_BaseURL_UsedForEndpoints(t *testing.T) {
	md := ServerMetadata("https://postbrain.example.com")

	if md["issuer"] != "https://postbrain.example.com" {
		t.Fatalf("issuer = %v", md["issuer"])
	}
	if md["authorization_endpoint"] != "https://postbrain.example.com/oauth/authorize" {
		t.Fatalf("authorization_endpoint = %v", md["authorization_endpoint"])
	}
	if md["token_endpoint"] != "https://postbrain.example.com/oauth/token" {
		t.Fatalf("token_endpoint = %v", md["token_endpoint"])
	}
	if md["registration_endpoint"] != "https://postbrain.example.com/oauth/register" {
		t.Fatalf("registration_endpoint = %v", md["registration_endpoint"])
	}
}

func TestServerMetadata_ScopesListed(t *testing.T) {
	md := ServerMetadata("https://postbrain.example.com")
	scopes, ok := md["scopes_supported"].([]string)
	if !ok {
		t.Fatalf("scopes_supported type = %T, want []string", md["scopes_supported"])
	}
	if len(scopes) == 0 {
		t.Fatal("scopes_supported is empty")
	}
	want := map[string]bool{
		ScopeMemoriesRead:   true,
		ScopeMemoriesWrite:  true,
		ScopeKnowledgeRead:  true,
		ScopeKnowledgeWrite: true,
		ScopeSkillsRead:     true,
		ScopeSkillsWrite:    true,
		ScopeAdmin:          true,
	}
	for _, s := range scopes {
		delete(want, s)
	}
	if len(want) != 0 {
		t.Fatalf("missing advertised scopes: %v", want)
	}
}
