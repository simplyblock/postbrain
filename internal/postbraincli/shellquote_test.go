package postbraincli

import "testing"

func TestShellSingleQuote(t *testing.T) {
	t.Parallel()
	if got := shellSingleQuote("project:acme/api; rm -rf /"); got != "'project:acme/api; rm -rf /'" {
		t.Fatalf("quoted scope = %q", got)
	}
	if got := shellSingleQuote("project:acme/o'hare"); got != `'project:acme/o'"'"'hare'` {
		t.Fatalf("quoted scope with apostrophe = %q", got)
	}
}
