package codegraph

import "testing"

func TestCanonicalForSymbol_DoesNotDoublePrefixPackage(t *testing.T) {
	t.Parallel()

	sym := Symbol{Name: "longpkg.Target", Package: "longpkg"}
	if got := canonicalForSymbol(sym); got != "longpkg.Target" {
		t.Fatalf("canonical = %q, want %q", got, "longpkg.Target")
	}
}
