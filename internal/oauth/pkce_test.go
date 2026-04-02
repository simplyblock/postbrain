package oauth

import "testing"

func TestVerifyS256_ValidPair_ReturnsTrue(t *testing.T) {
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	challenge := "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	if !VerifyS256(verifier, challenge) {
		t.Fatal("VerifyS256(valid) = false, want true")
	}
}

func TestVerifyS256_WrongVerifier_ReturnsFalse(t *testing.T) {
	if VerifyS256("wrong-verifier", "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM") {
		t.Fatal("VerifyS256(wrong verifier) = true, want false")
	}
}

func TestVerifyS256_EmptyInputs_ReturnsFalse(t *testing.T) {
	tests := []struct {
		name      string
		verifier  string
		challenge string
	}{
		{name: "both empty", verifier: "", challenge: ""},
		{name: "empty verifier", verifier: "", challenge: "abc"},
		{name: "empty challenge", verifier: "abc", challenge: ""},
	}
	for _, tt := range tests {
		if VerifyS256(tt.verifier, tt.challenge) {
			t.Fatalf("%s: VerifyS256 = true, want false", tt.name)
		}
	}
}

func TestGenerateChallenge_DeterministicForSameVerifier(t *testing.T) {
	verifier := "some-random-verifier-value"
	a := GenerateChallenge(verifier)
	b := GenerateChallenge(verifier)
	if a == "" {
		t.Fatal("GenerateChallenge returned empty challenge")
	}
	if a != b {
		t.Fatalf("GenerateChallenge not deterministic: %q != %q", a, b)
	}
}
